package service

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"peak/apps/recognition-service/internal/geom"
	"peak/apps/recognition-service/internal/provider"
)

// 金标准斜率：由题干解析式直接推得的三条直线数学斜率。
//   l₁: y=nx+7 过 C(2,6) → n=-0.5
//   l₂: y=mx   过 C(2,6) → m=3（正比例函数，必过原点 O）
//   l₃: y=kx+k 与 l₁、l₂ 不能围成三角形 → 过 l₁∩l₂=C(2,6) → k=2
// SVG 坐标 y 向下，故"数学斜率 = -(Δy_svg/Δx_svg)"。
var goldenSlopes = []float64{-0.5, 3, 2}

// slopeTol 斜率匹配容差：三条金标准斜率两两差距都 > 0.5，0.35 不会交叉误配。
const slopeTol = 0.35

// lineMetric 是一条 extend 直线（函数图象）的可测属性。
type lineMetric struct {
	from, to     string
	slope        float64 // 数学斜率；竖直线为 +Inf
	passesOrigin bool    // 是否过原点 O
}

// extendLineMetrics 提取一张子图里所有 extend 线段的数学斜率与"是否过原点"。
func extendLineMetrics(s *geom.Spec) []lineMetric {
	if s == nil || s.Axes == nil {
		return nil
	}
	origin, ok := s.PointByName(s.Axes.Origin)
	if !ok {
		return nil
	}
	short := math.Min(s.Canvas.Width, s.Canvas.Height)
	if short <= 0 {
		short = 100
	}
	var out []lineMetric
	for _, seg := range s.Segments {
		if !seg.Extend {
			continue
		}
		p1, ok1 := s.PointByName(seg.From)
		p2, ok2 := s.PointByName(seg.To)
		if !ok1 || !ok2 {
			continue
		}
		m := lineMetric{from: seg.From, to: seg.To}
		dx, dy := p2.X-p1.X, p2.Y-p1.Y
		if math.Abs(dx) < 1e-6 {
			// 竖直线：数学斜率无穷大；过原点当且仅当直线落在 y 轴上。
			m.slope = math.Inf(1)
			m.passesOrigin = math.Abs(p1.X-origin.X) < 0.8
		} else {
			m.slope = -dy / dx
			// 过原点：O 到直线的垂直距离 < 画布短边的 1.5%。
			d := math.Abs((p2.X-p1.X)*(p1.Y-origin.Y)-(p1.X-origin.X)*(p2.Y-p1.Y)) / math.Hypot(dx, dy)
			m.passesOrigin = d <= 0.015*short
		}
		out = append(out, m)
	}
	return out
}

// TestManualLineStability 同一张坐标系题图跑 N 次几何描述提取，统计三条直线
// （一次函数 l₁、正比例函数 l₂、一次函数 l₃）的数学斜率与"是否过原点"的漂移，
// 验证"同一条线有时过原点、有时穿第二/第四象限"这类随机性是否被收敛。
// 仅 GEOM_MANUAL=1 时执行；GEOM_STABILITY_N 控制采样次数（默认 3，每次约 40~120s）。
// 产物：/tmp/redraw_stability/report.txt（每次采样的斜率/过原点明细 + 汇总）。
func TestManualLineStability(t *testing.T) {
	if os.Getenv("GEOM_MANUAL") == "" {
		t.Skip("manual only")
	}
	n := 3
	if v := os.Getenv("GEOM_STABILITY_N"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			n = parsed
		}
	}

	imgPath := "/home/git/test-data/math/微信图片_20261031170523_45_176.jpg"
	raw, err := os.ReadFile(imgPath)
	if err != nil {
		t.Fatalf("read %s: %v", imgPath, err)
	}
	prov := provider.NewZhipuProvider(provider.ZhipuConfig{
		APIKey:         os.Getenv("ZHIPU_API_KEY"),
		GeometryEffort: os.Getenv("GEOM_EFFORT"),
	})

	// 一次识别定朝向/区域；之后的采样只做几何描述提取（朝向不影响"直线位置漂移"测量）。
	img, _ := normalizeImageOrientation(raw)
	ctx0, cancel0 := context.WithTimeout(context.Background(), 240*time.Second)
	geo, gerr := prov.RecognizeGeometry(ctx0, img)
	cancel0()
	if gerr != nil || geo.BoundingBox == nil {
		t.Fatalf("recognize geometry: %v", gerr)
	}
	extract := img
	if small, _, cerr := cropAndScaleRegionPlanned(img, geo.BoundingBox, geometryRegionMaxDim); cerr == nil {
		extract = small
	}
	rotation := geo.Rotation.FixDegrees()
	if rotation != 0 {
		if rot, rerr := rotateImageData(extract, rotation); rerr == nil {
			extract = rot
		}
	}

	// 探路一次：90/270 判反会得到空 panels，此时反向旋转后再正式采样。
	extractOnce := func() []lineMetric {
		ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
		spec, serr := prov.ExtractGeometrySpec(ctx, extract, stemTask22, "")
		cancel()
		if serr != nil {
			return nil
		}
		panels, perr := geom.ParsePanels(spec)
		if perr != nil {
			return nil
		}
		var ms []lineMetric
		for i := range panels {
			ms = append(ms, extendLineMetrics(&panels[i].Spec)...)
		}
		return ms
	}
	if probe := extractOnce(); len(probe) == 0 && rotation != 0 {
		opp := (360 - rotation) % 360
		if rot, rerr := rotateImageData(extract, opp); rerr == nil {
			extract = rot
			t.Logf("探路采样为空，反向旋转 %d° 后正式采样", opp)
		}
	}

	outDir := "/tmp/redraw_stability"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var rep strings.Builder
	fmt.Fprintf(&rep, "题目：task22（l₁:y=-0.5x+7, l₂:y=3x 过原点, l₃:y=2x+2）\n金标准斜率：%v\n\n", goldenSlopes)

	originPassCount := 0  // 正比例线（斜率≈3 且过原点）被正确识别的采样数
	fullHitCount := 0     // 三条金标准斜率全部命中的采样数
	for i := 1; i <= n; i++ {
		ms := extractOnce()
		hit := make([]bool, len(goldenSlopes))
		used := make([]bool, len(ms))
		for gi, g := range goldenSlopes {
			for mi, m := range ms {
				if used[mi] {
					continue
				}
				if math.Abs(m.slope-g) <= slopeTol {
					hit[gi], used[mi] = true, true
					break
				}
			}
		}
		hitCount, originOK := 0, false
		for _, h := range hit {
			if h {
				hitCount++
			}
		}
		for _, m := range ms {
			if math.Abs(m.slope-3) <= slopeTol && m.passesOrigin {
				originOK = true
			}
		}
		if originOK {
			originPassCount++
		}
		if hitCount == len(goldenSlopes) {
			fullHitCount++
		}
		fmt.Fprintf(&rep, "采样#%d（%d 条直线）：", i, len(ms))
		for _, m := range ms {
			fmt.Fprintf(&rep, "[%s→%s 斜率=%.2f 过原点=%v] ", m.from, m.to, m.slope, m.passesOrigin)
		}
		if len(ms) == 0 {
			fmt.Fprintf(&rep, "(空/提取失败)")
		}
		fmt.Fprintf(&rep, "→ 命中金标准 %d/3，正比例过原点=%v\n", hitCount, originOK)
	}
	fmt.Fprintf(&rep, "\n汇总（N=%d）：正比例线过原点 %d/%d；三条斜率全命中 %d/%d\n",
		n, originPassCount, n, fullHitCount, n)

	reportPath := filepath.Join(outDir, "report.txt")
	if err := os.WriteFile(reportPath, []byte(rep.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("稳定性报告已写入 %s\n%s", reportPath, rep.String())
}
