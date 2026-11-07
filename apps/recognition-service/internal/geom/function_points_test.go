package geom

import (
	"strings"
	"testing"
)

const (
	stemFuncPoints22 = "22. 如图，在平面直角坐标系 xOy 中，一次函数 y=nx+7 的图象 l₁ 分别与 x，y 轴交于 A，B 两点，" +
		"与正比例函数 y=mx 的图象 l₂ 交于点 C（2，6），若一次函数 y=kx+k 的图象 l₃ 与直线 l₁、l₂ 不能围成三角形，" +
		"则 k 的值为________."
	stemFuncPoints18 = "18. 如图. 在平面直角坐标系 xOy 中，直线 l₁: y=kx+b 与 x 轴，y 轴分别交于 A(-4, 0)，B 两点，" +
		"与直线 l₂: y=5x 交于点 C，点 C 的横坐标为 1，点 D(m, n) 为直线 l₁ 上一动点."
)

// findConstraint 按归一化直线名取约束。
func findConstraint(cons []functionLineConstraint, name string) (functionLineConstraint, bool) {
	for _, c := range cons {
		if c.line == name {
			return c, true
		}
	}
	return functionLineConstraint{}, false
}

func TestParseFunctionLineConstraints_task22(t *testing.T) {
	cons := parseFunctionLineConstraints(stemFuncPoints22)
	l1, ok := findConstraint(cons, "l1")
	if !ok {
		t.Fatalf("应解析出 l1，实际 %+v", cons)
	}
	for _, want := range []string{"A", "B", "C"} {
		if !strings.Contains(strings.Join(l1.points, ","), want) {
			t.Fatalf("l1 应经过 %s，实际 points=%v", want, l1.points)
		}
	}
	if l1.proportional {
		t.Fatal("l1 是一次函数，不应被判成正比例")
	}
	l2, ok := findConstraint(cons, "l2")
	if !ok || !l2.proportional {
		t.Fatalf("l2 应被判定为正比例函数，实际 %+v", l2)
	}
	l3, ok := findConstraint(cons, "l3")
	if !ok || !l3.fixedNegativeX {
		t.Fatalf("l3 应被判定为 y=kx+k 型（恒过 -1,0），实际 %+v", l3)
	}
}

func TestParseFunctionLineConstraints_task18(t *testing.T) {
	cons := parseFunctionLineConstraints(stemFuncPoints18)
	l1, ok := findConstraint(cons, "l1")
	if !ok {
		t.Fatalf("应解析出 l1，实际 %+v", cons)
	}
	for _, want := range []string{"A", "B", "C"} {
		if !strings.Contains(strings.Join(l1.points, ","), want) {
			t.Fatalf("l1 应经过 %s，实际 points=%v", want, l1.points)
		}
	}
	l2, ok := findConstraint(cons, "l2")
	if !ok || !strings.Contains(strings.Join(l2.points, ","), "C") {
		t.Fatalf("l2 应经过 C，实际 %+v", l2)
	}
}

// funcPointsSpec 构造几何完全自洽的坐标系（scale=4，O(20,60)）：
// l₁=A→B 过 A(76,60)、B(20,32)、C(28,36)；l₂=O→C 过原点；l₃ 过 (-1,0) 的 (16,60) 且不过 O。
func funcPointsSpec(bx, by float64, l1From, l1To, l3From, l3To string) *Spec {
	return &Spec{
		Canvas: Canvas{Width: 100, Height: 80},
		Points: []Point{
			{Name: "O", X: 20, Y: 60},
			{Name: "A", X: 76, Y: 60},
			{Name: "B", X: bx, Y: by},
			{Name: "C", X: 28, Y: 36},
			{Name: "XE", X: 95, Y: 60, Hidden: true},
			{Name: "YE", X: 20, Y: 5, Hidden: true},
			{Name: "P", X: 16, Y: 60, Hidden: true},
			{Name: "Q", X: 20, Y: 48, Hidden: true},
		},
		Segments: []Segment{
			{From: l1From, To: l1To, Extend: true},
			{From: "O", To: "C", Extend: true},
			{From: l3From, To: l3To, Extend: true},
		},
		Axes: &Axes{Origin: "O", X: Axis{To: "XE", Label: "x"}, Y: Axis{To: "YE", Label: "y"}},
		Labels: []Label{
			{X: 85, Y: 65, Text: "l₁"},
			{X: 42, Y: 2, Text: "l₂"},
			{X: 14, Y: 58, Text: "l₃"},
		},
	}
}

func TestCheckFunctionGraphPoints_correct(t *testing.T) {
	panels := []Panel{{Title: "图1", Spec: *funcPointsSpec(20, 32, "A", "B", "P", "Q")}}
	CheckFunctionGraphPoints(panels, stemFuncPoints22)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("几何自洽的图不应报问题，实际 %v", panels[0].Issues)
	}
}

func TestCheckFunctionGraphPoints_lineMissesPoint(t *testing.T) {
	// B 挪离 l₁（l₁ 改由 A→C 定义）；l₃ 改成 O→N（过原点）。
	s := funcPointsSpec(20, 50, "A", "C", "P", "Q")
	s.Points = append(s.Points, Point{Name: "N", X: 50, Y: 20, Hidden: true})
	s.Segments[2] = Segment{From: "O", To: "N", Extend: true}
	s.Labels[2] = Label{X: 52, Y: 18, Text: "l₃"}
	panels := []Panel{{Title: "图1", Spec: *s}}
	CheckFunctionGraphPoints(panels, stemFuncPoints22)
	issues := strings.Join(panels[0].Issues, "\n")
	if !strings.Contains(issues, "B") || !strings.Contains(issues, "l₁") {
		t.Fatalf("应报告 l₁ 没有穿过 B，实际 %v", panels[0].Issues)
	}
	if !strings.Contains(issues, "过原点") || !strings.Contains(issues, "l₃") {
		t.Fatalf("应报告 l₃:y=kx+k 过原点的问题，实际 %v", panels[0].Issues)
	}
}

func TestCheckFunctionGraphPoints_proportionalNotThroughOrigin(t *testing.T) {
	s := funcPointsSpec(20, 32, "A", "B", "P", "Q")
	// l₂ 改成 C→N（两端都不是 O），线明显偏离原点（距离约 4.2 > 容差 3.2）。
	s.Points = append(s.Points, Point{Name: "N", X: 44, Y: 6, Hidden: true})
	s.Segments[1] = Segment{From: "C", To: "N", Extend: true}
	s.Labels[1] = Label{X: 44, Y: 6, Text: "l₂"}
	panels := []Panel{{Title: "图1", Spec: *s}}
	CheckFunctionGraphPoints(panels, stemFuncPoints22)
	issues := strings.Join(panels[0].Issues, "\n")
	if !strings.Contains(issues, "正比例函数") {
		t.Fatalf("应报告正比例函数不过原点，实际 %v", panels[0].Issues)
	}
}
