package geom

import (
	"strings"
	"testing"
)

// 原图上没印直线名称时，即使题干一直在用 l₁/l₂ 这些代号，也不要求把名称画到图上。
func TestCheckNamedLinesNoLabelWhenNotPrinted(t *testing.T) {
	stem := "直线 l₁：y=kx+b 与 x 轴，y 轴分别交于 A（-4，0），B 两点，与直线 l₂：y=5x 交于点 C"
	desc := "平面直角坐标系xOy中两条直线相交的函数图像，主图标注点A(-4,0)、B、C(1,5)、D"
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 50, Y: 60}, {Name: "A", X: 18, Y: 60}, {Name: "B", X: 50, Y: 26},
			{Name: "C", X: 58, Y: 22}, {Name: "D", X: 32, Y: 45},
			{Name: "Xp", X: 96, Y: 60, Hidden: true}, {Name: "Yp", X: 50, Y: 6, Hidden: true},
		},
		Segments: []Segment{{From: "A", To: "C", Extend: true}, {From: "O", To: "C", Extend: true}},
		Axes:     &Axes{Origin: "O", X: Axis{To: "Xp"}, Y: Axis{To: "Yp"}},
	}
	panels := []Panel{{Title: "图1", Spec: *spec}}
	CheckNamedLines(panels, stem, desc, nil)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("原图未印名称时不应要求 labels，got %v", panels[0].Issues)
	}
}

// 两条直线只画出一条（或没延长）时要报出来，并指明用 extend:true。
func TestCheckNamedLinesReportsShortLineCount(t *testing.T) {
	stem := "直线 l₁：y=kx+b 与直线 l₂：y=5x 交于点 C"
	desc := "平面直角坐标系xOy中两条直线相交的函数图像"
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 50, Y: 60}, {Name: "A", X: 18, Y: 60}, {Name: "C", X: 58, Y: 22},
			{Name: "Xp", X: 96, Y: 60, Hidden: true}, {Name: "Yp", X: 50, Y: 6, Hidden: true},
		},
		// 只有 A→C 延长；O→C 是一截线段（模型漏写 extend）。
		Segments: []Segment{{From: "A", To: "C", Extend: true}, {From: "O", To: "C"}},
		Axes:     &Axes{Origin: "O", X: Axis{To: "Xp"}, Y: Axis{To: "Yp"}},
	}
	panels := []Panel{{Title: "图1", Spec: *spec}}
	CheckNamedLines(panels, stem, desc, nil)
	issues := strings.Join(panels[0].Issues, "\n")
	if !strings.Contains(issues, "只画出 1 条贯穿的直线") || !strings.Contains(issues, "extend:true") {
		t.Fatalf("应报直线数量不足并提示 extend，got %q", issues)
	}
}

// 原图没印的名称要从 labels 里删掉；原图印了的保留。
func TestStripUnprintedLineNames(t *testing.T) {
	spec := Spec{
		Labels: []Label{
			{X: 30, Y: 30, Text: "l₁"},
			{X: 70, Y: 70, Text: "l₂"},
			{X: 50, Y: 50, Text: "AD=2BD"},
		},
	}
	panels := []Panel{{Title: "图1", Spec: spec}}
	StripUnprintedLineNames(panels, []string{"l1"})
	got := panels[0].Spec.Labels
	var texts []string
	for _, l := range got {
		texts = append(texts, l.Text)
	}
	joined := strings.Join(texts, ",")
	if !strings.Contains(joined, "l₁") || strings.Contains(joined, "l₂") {
		t.Fatalf("只应保留原图印了的 l₁，got %q", joined)
	}
	if !strings.Contains(joined, "AD=2BD") {
		t.Fatalf("非直线名称的标注不应被删除，got %q", joined)
	}
}

// 识别阶段没返回印刷名称时按"原图没印"处理：名称全删，但非名称标注保留。
func TestStripUnprintedLineNamesRemovesAllWithoutLabels(t *testing.T) {
	spec := Spec{Labels: []Label{
		{X: 30, Y: 30, Text: "l₁"},
		{X: 70, Y: 70, Text: "l₂"},
		{X: 50, Y: 50, Text: "AD=2BD"},
	}}
	panels := []Panel{{Title: "图1", Spec: spec}}
	StripUnprintedLineNames(panels, nil)
	got := panels[0].Spec.Labels
	if len(got) != 1 || got[0].Text != "AD=2BD" {
		t.Fatalf("应只保留非名称标注，got %v", got)
	}
}
