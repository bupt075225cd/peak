package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"peak/apps/recognition-service/internal/geom"
	"peak/apps/recognition-service/internal/provider"
)

// 回归用题干（与测试图片上的印刷文字一致）。题干是重绘的重要输入：
// 它决定了直线代号（l₁/l₂/l₃）、已知点与三角形等约束。
const (
	stemTask18 = "18. 如图，在平面直角坐标系xOy中，直线l1：y=kx+b与x轴，y轴分别交于A（-4，0）、B两点，" +
		"与直线l2：y=5x交于点C，点C的横坐标为1，点D（m，n）为直线l1上一动点。" +
		"（1）求直线l1的函数表达式；（2）若△COD的面积为6，求线段AD的长；" +
		"（3）直线l3：y=mx+2n-2与x轴交于点E，当点B到直线l3距离最大时，求点D到BE的距离。"
	stemTask22 = "22. 如图，在平面直角坐标系xOy中，一次函数y=nx+7的图象l1分别与x，y轴交于A，B两点，" +
		"与正比例函数y=mx的图象l2交于点C（2，6），若一次函数y=kx+k的图象l3与直线l1、l2不能围成三角形，" +
		"则k的值为____"
)

// TestManualRedrawRegression 两张真实测试图的完整重绘回归：
// 识别（bbox/朝向）→ 裁剪区域 → 转正 → 几何描述提取（含语义校验回喂，最多 3 轮）→ 渲染。
// 产物写入 /tmp/redraw_regression/<任务>_attempt<N>_<图号>.svg 与 _best_<图号>.svg，
// 供人工核对"重绘图形与原图一致"。仅 GEOM_MANUAL=1 时执行。
func TestManualRedrawRegression(t *testing.T) {
	if os.Getenv("GEOM_MANUAL") == "" {
		t.Skip("manual only")
	}
	cases := []struct{ name, path, stem string }{
		{"task18", "/home/git/test-data/math/微信图片_20261031170521_43_176.jpg", stemTask18},
		{"task22", "/home/git/test-data/math/微信图片_20261031170523_45_176.jpg", stemTask22},
	}
	outDir := "/tmp/redraw_regression"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 思考强度可用 GEOM_EFFORT 覆盖（low/high/max，仅作用于几何提取调用），便于对比不同强度下的提取质量。
	prov := provider.NewZhipuProvider(provider.ZhipuConfig{
		APIKey:         os.Getenv("ZHIPU_API_KEY"),
		GeometryEffort: os.Getenv("GEOM_EFFORT"),
	})

	only := os.Getenv("REDRAW_ONLY")
	for _, c := range cases {
		if only != "" && only != c.name {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 900*time.Second)
		t.Logf("=== %s: %s ===", c.name, filepath.Base(c.path))

		raw, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatalf("read %s: %v", c.path, err)
		}
		img, _ := normalizeImageOrientation(raw)
		geo, gerr := prov.RecognizeGeometry(ctx, img)
		if gerr != nil || geo.BoundingBox == nil {
			t.Fatalf("%s recognize: %v", c.name, gerr)
		}
		t.Logf("图形描述：%s；原图印刷的直线名称：%v", geo.Description, geo.LineLabels)

		extract := img
		if small, plan, cerr := cropAndScaleRegionPlanned(img, geo.BoundingBox, geometryRegionMaxDim); cerr == nil {
			extract = small
			t.Logf("区域裁剪：%s", plan)
		}
		rotation := geo.Rotation.FixDegrees()
		if rotation != 0 {
			if rot, rerr := rotateImageData(extract, rotation); rerr == nil {
				extract = rot
			}
			t.Logf("几何区域转正：%d°", rotation)
		}

		var best []geom.PanelResult
		var bestIssues []string
		bestScore := -1
		correction := ""
		oppositeTried := false
		wholeTried := false
		// 与线上链路（service.go redrawGeometry）同构：截断/JSON 不合法/空 panels 等
		// 无效轮不消耗修正预算，只受独立的基础设施重试上限约束。
		const maxInfraRetries = 3
		infraRetries := 0
		attempt := 0
		for valid := 0; valid < 4 && infraRetries <= maxInfraRetries; {
			attempt++
			spec, serr := prov.ExtractGeometrySpec(ctx, extract, c.stem, correction)
			if serr != nil {
				infraRetries++
				t.Logf("[%s#%d] 提取失败：%v", c.name, attempt, serr)
				correction = "上一轮输出不是合法的 JSON。请只输出一个完整的 JSON 对象，不要 Markdown 代码块与解释文字。"
				continue
			}
			_ = os.WriteFile(filepath.Join(outDir, fmt.Sprintf("%s_attempt%d.json", c.name, attempt)), []byte(spec), 0o644)

			panels, perr := geom.ParsePanels(spec)
			if perr != nil {
				infraRetries++
				t.Logf("[%s#%d] 解析失败：%v", c.name, attempt, perr)
				correction = fmt.Sprintf("上一轮输出的 JSON 无法解析（%v）。请重新输出完整 JSON。", perr)
				continue
			}
			if len(panels) == 0 {
				infraRetries++
				// 侧躺图转反了会被判成"没有几何图形"，反向再试一次。
				if rotation != 0 && !oppositeTried {
					oppositeTried = true
					opp := (360 - rotation) % 360
					if rot, rerr := rotateImageData(extract, opp); rerr == nil {
						extract = rot
						rotation = opp
						t.Logf("[%s#%d] 空结果，改用反向旋转 %d° 重试", c.name, attempt, opp)
						correction = "上一轮输出 {\"panels\":[]}，但图片中确实有几何图形，可能是朝向转错；" +
							"请按文字可读方向理解图形并完整输出几何描述 JSON。"
						continue
					}
				}
				// 真实链路会先回退整图（regionFallback）再重试，这里同样处理。
				if !wholeTried {
					wholeTried = true
					extract = img
					t.Logf("[%s#%d] 模型返回空 panels，回退整图重试", c.name, attempt)
					correction = "上一轮输出 {\"panels\":[]}，但图片中确实有几何图形；请完整输出几何描述 JSON。"
					continue
				}
				t.Logf("[%s#%d] 模型返回空 panels，重试", c.name, attempt)
				correction = "上一轮输出 {\"panels\":[]}，但图片中确实有几何图形；" +
					"请按文字可读方向理解图形并完整输出几何描述 JSON。"
				continue
			}

			// 与线上链路一致的修复与语义核对。印刷直线名称取识别与描述两阶段共识。
			printedLineNames := geom.PrintedLineNamesConsensus(geo.LineLabels, panels)
			geom.StripUnprintedLineNames(panels, printedLineNames)
			geom.CheckFigureCount(panels, c.stem+"\n"+geo.Description)
			geom.CheckNamedLines(panels, c.stem, geo.Description, printedLineNames)
			geom.CheckAxisCoincidentSegments(panels)
			geom.CheckFloatingPoints(panels)
			geom.CheckTriangleEdges(panels, c.stem)

			results := make([]geom.PanelResult, 0, len(panels))
			for i := range panels {
				svg, rerr := geom.Render(&panels[i].Spec)
				if rerr != nil {
					t.Fatalf("%s: render: %v", c.name, rerr)
				}
				results = append(results, geom.PanelResult{
					Title: panels[i].Title, SVG: svg, Issues: panels[i].Issues,
				})
				_ = os.WriteFile(filepath.Join(outDir, fmt.Sprintf("%s_attempt%d_%d.svg", c.name, attempt, i+1)), svg, 0o644)
			}
			issues := geom.CollectIssues(results)
			t.Logf("[%s#%d] 子图=%d 问题=%d %v", c.name, attempt, len(results), len(issues), issues)

			valid++ // 本轮产出了完整可校验的结果，消耗一次修正预算
			if best == nil || len(issues) < bestScore {
				best, bestIssues, bestScore = results, issues, len(issues)
			}
			if len(issues) == 0 {
				break
			}
			correction = strings.Join(issues, "\n")
		}
		if best == nil {
			t.Fatalf("%s: 未产出任何重绘结果", c.name)
		}
		var report strings.Builder
		fmt.Fprintf(&report, "题目：%s\n图形描述：%s\n剩余问题（%d）：\n%s\n", c.name, geo.Description, len(bestIssues), strings.Join(bestIssues, "\n"))
		for i, p := range best {
			_ = os.WriteFile(filepath.Join(outDir, fmt.Sprintf("%s_best_%d.svg", c.name, i+1)), p.SVG, 0o644)
			fmt.Fprintf(&report, "- %s\n", p.Title)
		}
		_ = os.WriteFile(filepath.Join(outDir, c.name+"_report.txt"), []byte(report.String()), 0o644)
		t.Logf("[%s] 最终子图数=%d，剩余问题=%v", c.name, len(best), bestIssues)
		cancel()
	}
}
