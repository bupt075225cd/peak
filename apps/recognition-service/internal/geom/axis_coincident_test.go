package geom

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestCheckNamedLinesIgnoresAxisCoincidentLines 复现生产 task 72：
// 应画 3 条直线，模型把其中一条画成与 x 轴重合的 O→A（延长到画布边缘）。
// 条数核对不能被这条废线段骗过，否则漏画的 l₃ 不会触发回喂重试。
func TestCheckNamedLinesIgnoresAxisCoincidentLines(t *testing.T) {
	s := funcPointsSpec(20, 32, "A", "B", "P", "Q")
	s.Segments[2] = Segment{From: "O", To: "A", Extend: true} // l₃ 退化到 x 轴上
	s.Labels = []Label{{X: 85, Y: 65, Text: "l₁"}, {X: 42, Y: 2, Text: "l₂"}}
	panels := []Panel{{Title: "图1", Spec: *s}}

	issues := CheckNamedLines(panels, stemFuncPoints22,
		"平面直角坐标系中三条直线l1、l2、l3相交", []string{"l1", "l2", "l3"})
	joined := strings.Join(issues, "\n")
	if !strings.Contains(joined, "只画出 2 条贯穿的直线") {
		t.Fatalf("与轴重合的线段不应计入已画直线数，应报漏画，实际 %v", issues)
	}
}

// TestRenderSkipsAxisCoincidentExtendSegment 复现生产反馈的"轴线超过箭头"：
// 与坐标轴重合的 extend 线段被延长到画布边缘，画到轴箭头之外。
// 渲染应跳过这类线段（它本来就被轴线盖住，等于没画）。
func TestRenderSkipsAxisCoincidentExtendSegment(t *testing.T) {
	s := funcPointsSpec(20, 32, "A", "B", "P", "Q")
	s.Segments[2] = Segment{From: "O", To: "A", Extend: true}
	// 生产场景里模型没有印刷任何线名标签（l₃ 整条缺失），这里保持一致。
	s.Labels = []Label{{X: 85, Y: 65, Text: "l₁"}, {X: 42, Y: 2, Text: "l₂"}}
	s.Normalize()
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	lineRe := regexp.MustCompile(`<line x1="([-\d.]+)" y1="([-\d.]+)" x2="([-\d.]+)" y2="([-\d.]+)" stroke="#111827" stroke-width="0.95"`)
	for _, m := range lineRe.FindAllStringSubmatch(string(out), -1) {
		y1, _ := strconv.ParseFloat(m[2], 64)
		y2, _ := strconv.ParseFloat(m[4], 64)
		if y1 == 60 && y2 == 60 { // O(20,60)-A(76,60) 所在的 x 轴线
			t.Fatalf("与 x 轴重合的线段不应被绘制：%s", m[0])
		}
	}
	// 真正的函数直线（如 l₁）不受影响，仍应画出。
	if !strings.Contains(string(out), `stroke-width="0.95"`) {
		t.Fatal("其余函数直线应正常绘制")
	}
}
