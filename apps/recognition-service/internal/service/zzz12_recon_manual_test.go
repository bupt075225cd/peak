package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"peak/apps/recognition-service/internal/geom"
	"peak/apps/recognition-service/internal/provider"
)

// TestManualReconcileTwoImages 用两张真实测试图跑完整"识别→提取→渲染"链路，
// 输出 SVG 与"标注点到直线距离"报告，用于验证点线对齐效果。
func TestManualReconcileTwoImages(t *testing.T) {
	if os.Getenv("GEOM_MANUAL") == "" {
		t.Skip("manual only")
	}
	images := []string{
		"/home/git/test-data/math/微信图片_20261031170521_43_176.jpg",
		"/home/git/test-data/math/微信图片_20261031170523_45_176.jpg",
	}
	prov := provider.NewZhipuProvider(provider.ZhipuConfig{APIKey: os.Getenv("ZHIPU_API_KEY")})
	ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)
	defer cancel()
	outDir := "/tmp/recon_test"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, path := range images {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		img, _ := normalizeImageOrientation(raw)
		geo, gerr := prov.RecognizeGeometry(ctx, img)
		if gerr != nil || geo.BoundingBox == nil {
			t.Fatalf("recognize %s: %v", path, gerr)
		}
		extract := img
		if small, _, cerr := cropAndScaleRegionPlanned(img, geo.BoundingBox, geometryRegionMaxDim); cerr == nil {
			extract = small
		}
		if fix := geo.Rotation.FixDegrees(); fix != 0 {
			if rot, rerr := rotateImageData(extract, fix); rerr == nil {
				extract = rot
			}
		}
		spec, serr := prov.ExtractGeometrySpec(ctx, extract, "", "")
		if serr != nil {
			t.Fatalf("extract %s: %v", path, serr)
		}
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		_ = os.WriteFile(filepath.Join(outDir, base+"_spec.json"), []byte(spec), 0o644)

		// 对齐前的原始坐标用于对比。
		var before geom.Spec
		_ = json.Unmarshal([]byte(specNormalizePanels(spec)), &before)

		panels, perr := geom.ParsePanels(spec)
		if perr != nil {
			t.Fatalf("parse %s: %v", path, perr)
		}
		t.Logf("=== %s panels=%d ===", base, len(panels))
		for i := range panels {
			svg, rerr := geom.Render(&panels[i].Spec)
			if rerr != nil {
				t.Fatalf("render: %v", rerr)
			}
			name := fmt.Sprintf("%s_%d.svg", base, i+1)
			_ = os.WriteFile(filepath.Join(outDir, name), svg, 0o644)
			t.Logf("[%s] %s\n%s", name, panels[i].Title, pointLineReport(&panels[i].Spec))
		}
		if len(panels) == 1 {
			t.Logf("对齐前：\n%s", pointLineReport(&before))
		}
	}
}

// specNormalizePanels 把多子图 spec 转成单子图（仅取第一张），便于对比原始坐标。
func specNormalizePanels(spec string) string {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(spec), &probe); err != nil {
		return spec
	}
	if p, ok := probe["panels"]; ok {
		var list []json.RawMessage
		if json.Unmarshal(p, &list) == nil && len(list) > 0 {
			return string(list[0])
		}
	}
	return spec
}

type pt struct{ x, y float64 }

// pointLineReport 输出每个标注点到各条延长线（直线）的距离，用于判断点是否浮在直线外。
func pointLineReport(s *geom.Spec) string {
	var lines []struct {
		name string
		a, b pt
	}
	byName := map[string]pt{}
	for _, p := range s.Points {
		byName[p.Name] = pt{p.X, p.Y}
	}
	for i, seg := range s.Segments {
		if !seg.Extend {
			continue
		}
		a, okA := byName[seg.From]
		b, okB := byName[seg.To]
		if !okA || !okB {
			continue
		}
		lines = append(lines, struct {
			name string
			a, b pt
		}{fmt.Sprintf("seg%d(%s-%s)", i+1, seg.From, seg.To), a, b})
	}
	var sb strings.Builder
	for _, p := range s.Points {
		if p.Hidden {
			continue
		}
		fmt.Fprintf(&sb, "  %s(%s,%s):", p.Name, num2(p.X), num2(p.Y))
		for _, l := range lines {
			d := ptLineDist(pt{p.X, p.Y}, l.a, l.b)
			mark := ""
			if d <= 0.2 {
				mark = " ✅"
			}
			fmt.Fprintf(&sb, " %s=%.2f%s", l.name, d, mark)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func ptLineDist(p, a, b pt) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	if l < 1e-9 {
		return math.Inf(1)
	}
	return math.Abs(dx*(p.y-a.y)-dy*(p.x-a.x)) / l
}

func num2(v float64) string { return fmt.Sprintf("%.1f", v) }
