package geom

import "testing"

func semicirclePanels(circles, arcs int) []Panel {
	p := Panel{Title: "图1"}
	for i := 0; i < circles; i++ {
		p.Spec.Circles = append(p.Spec.Circles, Circle{Center: "O", Radius: 10})
	}
	for i := 0; i < arcs; i++ {
		p.Spec.Arcs = append(p.Spec.Arcs, Arc{Center: "O", Radius: 10, StartAngle: 180, EndAngle: 360})
	}
	return []Panel{p}
}

func TestCheckSemicircleFlagsCircleOnly(t *testing.T) {
	// 题干含"半圆"、只有整圆无弧 → 追加修正问题。
	panels := semicirclePanels(1, 0)
	CheckSemicircle(panels, "储藏室的截面，下面是长方形ABCD，上面是半圆形")
	if len(panels[0].Issues) == 0 {
		t.Fatalf("expected semicircle issue, got none")
	}
}

func TestCheckSemicirclePassesWithArc(t *testing.T) {
	// 已用 arcs 表达半圆 → 不报问题。
	panels := semicirclePanels(1, 1)
	CheckSemicircle(panels, "上面是半圆形")
	for i := range panels {
		if len(panels[i].Issues) != 0 {
			t.Fatalf("unexpected issues: %v", panels[i].Issues)
		}
	}
}

func TestCheckSemicircleStemWithoutSemicircle(t *testing.T) {
	// 题干未提半圆 → 即使只有整圆也不报（避免误报）。
	panels := semicirclePanels(1, 0)
	CheckSemicircle(panels, "如图，PQ//MN，求角的度数")
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestCheckSemicircleNoCircleNoArc(t *testing.T) {
	// 无 circles（模型未画圆）→ 保守不报。
	panels := semicirclePanels(0, 0)
	CheckSemicircle(panels, "上面是半圆形")
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestCheckSemicircleMultiPanel(t *testing.T) {
	// 多子图：只给含圆的子图追加问题。
	withCircle := Panel{Title: "图1"}
	withCircle.Spec.Circles = []Circle{{Center: "O", Radius: 10}}
	noCircle := Panel{Title: "图2"}
	noCircle.Spec.Segments = []Segment{{From: "A", To: "B"}}
	panels := []Panel{withCircle, noCircle}
	CheckSemicircle(panels, "上面是半圆形")
	if len(panels[0].Issues) == 0 {
		t.Fatalf("expected issue on panel 1")
	}
	if len(panels[1].Issues) != 0 {
		t.Fatalf("unexpected issue on panel 2: %v", panels[1].Issues)
	}
}

// rectanglePanel 构造一个矩形 ABCD 子图（A 左下、B 右下、C 右上、D 左上）。
func rectanglePanel(w, h float64) Panel {
	p := Panel{Title: "图1"}
	p.Spec.Canvas = Canvas{Width: 100, Height: 100}
	x0, x1 := 20.0, 20.0+w
	y0, y1 := 70.0, 70.0-h
	p.Spec.Points = []Point{
		{Name: "A", X: x0, Y: y0},
		{Name: "B", X: x1, Y: y0},
		{Name: "C", X: x1, Y: y1},
		{Name: "D", X: x0, Y: y1},
	}
	p.Spec.Segments = []Segment{
		{From: "A", To: "B"}, {From: "B", To: "C"}, {From: "C", To: "D"}, {From: "D", To: "A"},
	}
	return p
}

const stemAB2_3BC2_6 = "17.某储藏室的截面下面是长方形ABCD，上面是半圆形，其中AB=2.3m，BC=2.6m，一辆车能通过吗？"

func TestCheckStemLengthsFlagsWrongProportions(t *testing.T) {
	// 题干 AB=2.3、BC=2.6，但图把宽边（AB）画得比高边（BC）长 → 报比例矛盾。
	panels := []Panel{rectanglePanel(60, 35)}
	CheckStemLengths(panels, stemAB2_3BC2_6)
	if len(panels[0].Issues) == 0 {
		t.Fatalf("expected proportion issue, got none")
	}
}

func TestCheckStemLengthsPassesConsistentProportions(t *testing.T) {
	// 画出长度比例与题干数值一致（AB 略短于 BC）→ 不报。
	panels := []Panel{rectanglePanel(29, 35)}
	CheckStemLengths(panels, stemAB2_3BC2_6)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestCheckStemLengthsFlagsMisplacedLabel(t *testing.T) {
	// 比例一致，但 "2.6m" 标注贴在 AB 旁，而题干 2.6 是 BC 的长度 → 报标注错位。
	p := rectanglePanel(29, 35)
	p.Spec.Labels = []Label{{Text: "2.6m", X: 34, Y: 74}}
	panels := []Panel{p}
	CheckStemLengths(panels, stemAB2_3BC2_6)
	if len(panels[0].Issues) == 0 {
		t.Fatalf("expected misplaced label issue, got none")
	}
}

func TestCheckStemLengthsPassesCorrectLabel(t *testing.T) {
	// "2.6m" 标在 BC 旁、"2.3m" 标在 AB 旁 → 不报。
	p := rectanglePanel(29, 35)
	p.Spec.Labels = []Label{
		{Text: "2.3m", X: 34, Y: 74},
		{Text: "2.6m", X: 55, Y: 52},
	}
	panels := []Panel{p}
	CheckStemLengths(panels, stemAB2_3BC2_6)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestCheckStemLengthsLabelWithSegmentName(t *testing.T) {
	// 标注自带线段名（如 "AB=2.3"）→ 位置不再强求，视为正确。
	p := rectanglePanel(29, 35)
	p.Spec.Labels = []Label{{Text: "AB=2.3", X: 34, Y: 74}}
	panels := []Panel{p}
	CheckStemLengths(panels, "其中AB=2.3m")
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestCheckStemLengthsIgnoresAngleNames(t *testing.T) {
	// ∠ABC=60° 不是线段长度；题干无数值线段约束 → 保守不报。
	panels := []Panel{rectanglePanel(60, 35)}
	CheckStemLengths(panels, "在△ABC中，∠ABC=60°，求证")
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestCheckStemLengthsMultiPanel(t *testing.T) {
	// 多子图：只给包含对应点且矛盾的子图报问题。
	bad := rectanglePanel(60, 35)
	empty := Panel{Title: "图2"}
	panels := []Panel{bad, empty}
	CheckStemLengths(panels, stemAB2_3BC2_6)
	if len(panels[0].Issues) == 0 {
		t.Fatalf("expected issue on panel 1")
	}
	if len(panels[1].Issues) != 0 {
		t.Fatalf("unexpected issue on panel 2: %v", panels[1].Issues)
	}
}

func TestParseStemLengths(t *testing.T) {
	cases := []struct {
		stem string
		want []stemLengthConstraint
	}{
		{"AB=2.3m，BC=2.6m", []stemLengthConstraint{{"A", "B", 2.3}, {"B", "C", 2.6}}},
		{"其中AB＝2.3", []stemLengthConstraint{{"A", "B", 2.3}}},
		{"∠ABC=60°", nil},        // 角名不是线段
		{"2AB=6", nil},           // 系数表达不是线段长度
		{"AE=AD，AB=5", []stemLengthConstraint{{"A", "B", 5}}}, // 符号关系不计，只留数值项
		{"AB=2.3，AB=2.3", []stemLengthConstraint{{"A", "B", 2.3}}}, // 去重
	}
	for _, c := range cases {
		got := parseStemLengths(c.stem)
		if len(got) != len(c.want) {
			t.Fatalf("parseStemLengths(%q) = %v, want %d constraints", c.stem, got, len(c.want))
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("parseStemLengths(%q)[%d] = %v, want %v", c.stem, i, got[i], c.want[i])
			}
		}
	}
}

const stemSemicircleTop = "17.储藏室的截面下面是长方形ABCD，上面是半圆形，其中AB=2.3m，BC=2.6m"

func semicircleDirectionPanel(start, end float64) Panel {
	p := Panel{Title: "图1"}
	p.Spec.Canvas = Canvas{Width: 100, Height: 100}
	// 竖直直径 AD（右侧），圆心 O 为中点：弧朝向由 start/end 决定。
	p.Spec.Points = []Point{
		{Name: "A", X: 60, Y: 20},
		{Name: "D", X: 60, Y: 70},
		{Name: "O", X: 60, Y: 45, Hidden: true},
	}
	p.Spec.Segments = []Segment{{From: "A", To: "D"}}
	p.Spec.Arcs = []Arc{{Center: "O", Radius: 25, StartAngle: start, EndAngle: end}}
	return p
}

func TestCheckSemicircleDirectionFlagsSideArc(t *testing.T) {
	// 题干"上面是半圆形"，弧却朝右（270→90，竖直直径）→ 报朝向问题。
	panels := []Panel{semicircleDirectionPanel(270, 90)}
	CheckSemicircleDirection(panels, stemSemicircleTop)
	if len(panels[0].Issues) == 0 {
		t.Fatalf("expected direction issue, got none")
	}
}

func TestCheckSemicircleDirectionPassesTopArc(t *testing.T) {
	// 弧朝上（180→360，水平直径）→ 不报。
	panels := []Panel{semicircleDirectionPanel(180, 360)}
	CheckSemicircleDirection(panels, stemSemicircleTop)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestCheckSemicircleDirectionBottom(t *testing.T) {
	// 题干"下面是半圆形"：弧朝下（0→180）→ 不报；弧朝上（180→360）→ 报。
	down := []Panel{semicircleDirectionPanel(0, 180)}
	CheckSemicircleDirection(down, "储藏室的截面上方是长方形，下面是半圆形")
	if len(down[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", down[0].Issues)
	}
	up := []Panel{semicircleDirectionPanel(180, 360)}
	CheckSemicircleDirection(up, "储藏室的截面上方是长方形，下面是半圆形")
	if len(up[0].Issues) == 0 {
		t.Fatalf("expected direction issue, got none")
	}
}

func TestCheckSemicircleDirectionUnknownSkips(t *testing.T) {
	// 题干未说明半圆在上/下 → 保守不报。
	panels := []Panel{semicircleDirectionPanel(270, 90)}
	CheckSemicircleDirection(panels, "储藏室的截面由半圆形构成")
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestNormalizeDropsNumericLengthLabels(t *testing.T) {
	// 图上不标数值长度：纯数值（可带单位）标注剔除，代数式/结论保留。
	s := Spec{Canvas: Canvas{Width: 100, Height: 100}}
	s.Labels = []Label{
		{Text: "2.6m", X: 50, Y: 80},
		{Text: "2.3", X: 10, Y: 40},
		{Text: "3厘米", X: 20, Y: 40},
		{Text: "AD=2BD", X: 30, Y: 40},
		{Text: "a²+b²=c²", X: 40, Y: 40},
	}
	s.Normalize()
	var got []string
	for _, l := range s.Labels {
		got = append(got, l.Text)
	}
	want := []string{"AD=2BD", "a²+b²=c²"}
	if len(got) != len(want) {
		t.Fatalf("labels after normalize = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("labels after normalize = %v, want %v", got, want)
		}
	}
}
