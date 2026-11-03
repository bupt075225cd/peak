package geom

import (
	"strings"
	"testing"
)

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
		{"∠ABC=60°", nil}, // 角名不是线段
		{"2AB=6", nil},    // 系数表达不是线段长度
		{"AE=AD，AB=5", []stemLengthConstraint{{"A", "B", 5}}},      // 符号关系不计，只留数值项
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

func TestCheckStemPassPointsFlagsNonCollinear(t *testing.T) {
	// 题干"边C₁D₁恰好经过点A"，图中 A 远离 C1D1 → 报共线矛盾。
	p := Panel{Title: "图1"}
	p.Spec.Canvas = Canvas{Width: 100, Height: 100}
	p.Spec.Points = []Point{
		{Name: "A", X: 30, Y: 55},
		{Name: "B", X: 90, Y: 55},
		{Name: "D1", X: 15, Y: 30},
		{Name: "C1", X: 75, Y: 12},
	}
	panels := []Panel{p}
	CheckStemPassPoints(panels, "23.在长方形ABCD中…将长方形ABCD沿BE翻折…且边C₁D₁恰好经过点A，再将△ABC₁沿AB翻折…")
	if len(panels[0].Issues) == 0 {
		t.Fatalf("expected collinearity issue, got none")
	}
}

func TestCheckStemPassPointsPassesCollinear(t *testing.T) {
	// A 在 C1D1 所在直线上（允许小偏差）→ 不报。
	p := Panel{Title: "图1"}
	p.Spec.Canvas = Canvas{Width: 100, Height: 100}
	p.Spec.Points = []Point{
		{Name: "A", X: 45, Y: 21},
		{Name: "B", X: 90, Y: 55},
		{Name: "D1", X: 15, Y: 30},
		{Name: "C1", X: 75, Y: 12},
	}
	panels := []Panel{p}
	CheckStemPassPoints(panels, "且边C₁D₁恰好经过点A")
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestCheckStemPassPointsSkipsWithoutPattern(t *testing.T) {
	// 题干无"经过点"表述 → 保守不报。
	p := Panel{Title: "图1"}
	p.Spec.Canvas = Canvas{Width: 100, Height: 100}
	p.Spec.Points = []Point{
		{Name: "A", X: 30, Y: 55},
		{Name: "D1", X: 15, Y: 30},
		{Name: "C1", X: 75, Y: 12},
	}
	panels := []Panel{p}
	CheckStemPassPoints(panels, "将△ABC₁沿AB翻折，点C₁落在点C₂处，连接EC₂")
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestParseStemPassPoints(t *testing.T) {
	cases := []struct {
		stem string
		want []collinearityConstraint
	}{
		{"且边C₁D₁恰好经过点A", []collinearityConstraint{{"C1", "D1", "A"}}},
		{"线段DE经过点F", []collinearityConstraint{{"D", "E", "F"}}},
		{"直线AB经过点C，AB经过点C", []collinearityConstraint{{"A", "B", "C"}}}, // 去重
		{"点C落在点C₁处", nil}, // 无"经过点"
	}
	for _, c := range cases {
		got := parseStemPassPoints(c.stem)
		if len(got) != len(c.want) {
			t.Fatalf("parseStemPassPoints(%q) = %v, want %d constraints", c.stem, got, len(c.want))
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("parseStemPassPoints(%q)[%d] = %v, want %v", c.stem, i, got[i], c.want[i])
			}
		}
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

func edgePanel(py float64, px float64) []Panel {
	return []Panel{{Title: "图1", Spec: Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 20, Y: 80},
			{Name: "A", X: 20, Y: 20},
			{Name: "C", X: 80, Y: 80},
			{Name: "P", X: px, Y: py},
		},
	}}}
}

const stemOnEdge = "点D在OC边上，OD=13，点P在OA边上，将矩形OABC沿直线PD翻折"

func TestCheckPointOnEdgeFlagsExtension(t *testing.T) {
	// P 画在 A 的外侧（OA 延长线上）→ 必须报"延长线"问题。
	panels := edgePanel(8, 20)
	CheckPointOnEdge(panels, stemOnEdge)
	if len(panels[0].Issues) == 0 {
		t.Fatal("expected issue for point on edge extension")
	}
	if !strings.Contains(panels[0].Issues[0], "延长线") {
		t.Fatalf("expected extension issue, got %v", panels[0].Issues)
	}
}

func TestCheckPointOnEdgePassesInside(t *testing.T) {
	// P 落在 O 与 A 之间 → 不报。
	panels := edgePanel(50, 20)
	CheckPointOnEdge(panels, stemOnEdge)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestCheckPointOnEdgeFlagsFarOffLine(t *testing.T) {
	// P 远离线段 OA → 报偏离问题。
	panels := edgePanel(50, 60)
	CheckPointOnEdge(panels, stemOnEdge)
	if len(panels[0].Issues) == 0 || !strings.Contains(panels[0].Issues[0], "偏离") {
		t.Fatalf("expected off-line issue, got %v", panels[0].Issues)
	}
}

func TestCheckPointOnEdgeSkipsMissingPoints(t *testing.T) {
	// 图里没有 P（模型漏点）→ 保守跳过，交给结构校验处理。
	panels := []Panel{{Title: "图1", Spec: Spec{
		Points: []Point{{Name: "O", X: 20, Y: 80}, {Name: "A", X: 20, Y: 20}},
	}}}
	CheckPointOnEdge(panels, stemOnEdge)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestExpectedFigureCount(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"如图1，点P在OA边上；（2）如图2，点M从点D出发", 2},
		{"三幅带x轴、y轴箭头的子图（图1、图2、备用图）", 3},
		{"如图1，如图2，备用图", 3},
		{"图1/图2/图3", 3},
		{"如图，求阴影面积", 0},
	}
	for _, c := range cases {
		if got := expectedFigureCount(c.text); got != c.want {
			t.Fatalf("expectedFigureCount(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}

func TestCheckFigureCountFlagsMissingPanel(t *testing.T) {
	panels := []Panel{{Title: "图1"}, {Title: "图2"}}
	hint := "如图1；（2）如图2\n三幅带x轴、y轴箭头的子图（图1、图2、备用图）"
	CheckFigureCount(panels, hint)
	if len(panels[0].Issues) == 0 || !strings.Contains(panels[0].Issues[0], "3 张子图") {
		t.Fatalf("expected missing-panel issue, got %v", panels[0].Issues)
	}

	// 数量已齐 → 不报。
	ok := []Panel{{Title: "图1"}, {Title: "图2"}, {Title: "备用图"}}
	CheckFigureCount(ok, hint)
	if len(ok[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", ok[0].Issues)
	}
}

func trianglePanel(withEM bool) []Panel {
	p := Panel{Title: "图2", Spec: Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 10, Y: 90, Hidden: true},
			{Name: "XA", X: 92, Y: 90, Hidden: true},
			{Name: "YA", X: 10, Y: 5, Hidden: true},
			{Name: "D", X: 45, Y: 90},
			{Name: "E", X: 30, Y: 20},
			{Name: "M", X: 70, Y: 90},
		},
		Segments: []Segment{{From: "D", To: "E"}, {From: "D", To: "M"}},
		Axes: &Axes{
			Origin: "O",
			X:      Axis{To: "XA", Label: "x"},
			Y:      Axis{To: "YA", Label: "y"},
		},
	}}
	if withEM {
		p.Spec.Segments = append(p.Spec.Segments, Segment{From: "E", To: "M"})
	}
	return []Panel{p}
}

const stemTriangle = "以点 D、E、M 为顶点的三角形的面积记为 S"

func TestCheckTriangleEdgesFlagsMissingEdge(t *testing.T) {
	// 只有 DE 与 DM（DM 与 x 轴重合被轴覆盖），缺 EM → 报缺失。
	panels := trianglePanel(false)
	CheckTriangleEdges(panels, stemTriangle)
	if len(panels[0].Issues) == 0 || !strings.Contains(panels[0].Issues[0], "EM") {
		t.Fatalf("expected missing EM issue, got %v", panels[0].Issues)
	}
}

func TestCheckTriangleEdgesPassesWithAllEdges(t *testing.T) {
	panels := trianglePanel(true)
	CheckTriangleEdges(panels, stemTriangle)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestCheckTriangleEdgesSkipsPanelWithoutVertices(t *testing.T) {
	// 图里没有 M（如折叠前的图1）→ 保守跳过，不误报。
	panels := []Panel{{Title: "图1", Spec: Spec{
		Points:   []Point{{Name: "D", X: 45, Y: 90}, {Name: "E", X: 30, Y: 20}},
		Segments: []Segment{{From: "D", To: "E"}},
	}}}
	CheckTriangleEdges(panels, stemTriangle)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestNormalizeDropsLabelEqualToPointName(t *testing.T) {
	// 模型在 labels 里重复写点名 → 剔除，避免图上出现两个 M。
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{{Name: "M", X: 70, Y: 90}},
		Labels: []Label{{X: 74, Y: 92, Text: "M"}, {X: 30, Y: 40, Text: "AB=2BD"}},
	}
	s.Normalize()
	if len(s.Labels) != 1 || s.Labels[0].Text != "AB=2BD" {
		t.Fatalf("duplicate point-name label should be dropped, got %+v", s.Labels)
	}
}

func TestCheckPointOnEdgeHandlesIntersectionPhrase(t *testing.T) {
	// 题干"交CD于点G"：G 必须落在线段 CD 上。
	build := func(gx, gy float64) []Panel {
		return []Panel{{Title: "图1", Spec: Spec{
			Points: []Point{
				{Name: "C", X: 20, Y: 20}, {Name: "D", X: 60, Y: 80},
				{Name: "G", X: gx, Y: gy},
			},
		}}}
	}
	stem := "过点A作AG∥CE交CD于点G，当线段OG的值最小时，则运动时间t的值为____"

	bad := build(60, 30) // 远离 CD
	CheckPointOnEdge(bad, stem)
	if len(bad[0].Issues) == 0 {
		t.Fatalf("expected G-on-CD issue, got none")
	}

	good := build(40, 50) // 恰在 CD 中点
	CheckPointOnEdge(good, stem)
	if len(good[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", good[0].Issues)
	}
}

func motionPanel(px, py float64) []Panel {
	return []Panel{{Title: "图1", Spec: Spec{
		Canvas: Canvas{Width: 100, Height: 75},
		Points: []Point{
			{Name: "A", X: 18, Y: 55},
			{Name: "D", X: px, Y: py},
			{Name: "B", X: 46, Y: 27},
			{Name: "E", X: px, Y: py},
		},
	}}}
}

const stemMotion = "动点D从A点出发，以每秒1个单位的速度水平向右运动，动点E从点B出发，以每秒1个单位的速度竖直向上运动"

func TestCheckMotionConstraintsFlagsWrongLine(t *testing.T) {
	// D 被画在竖直边 AC 上（x 与 A 不同、y 变了），E 也被画到同一处 → 两条都报。
	panels := motionPanel(18, 44.5)
	CheckMotionConstraints(panels, stemMotion)
	if len(panels[0].Issues) < 2 {
		t.Fatalf("expected issues for D (not horizontal from A) and E (not vertical from B), got %v", panels[0].Issues)
	}
}

func TestCheckMotionConstraintsPassesCorrectPositions(t *testing.T) {
	// D 在过 A 的水平线上（向右），E 在过 B 的竖直线上（向上）。
	panels := []Panel{{Title: "图1", Spec: Spec{
		Canvas: Canvas{Width: 100, Height: 75},
		Points: []Point{
			{Name: "A", X: 18, Y: 55},
			{Name: "D", X: 30, Y: 55},
			{Name: "B", X: 46, Y: 27},
			{Name: "E", X: 46, Y: 13},
		},
	}}}
	CheckMotionConstraints(panels, stemMotion)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", panels[0].Issues)
	}
}

func TestCheckParallelRelations(t *testing.T) {
	stem := "过点A作AG∥CE交CD于点G"
	// AG 竖直、CE 水平 → 不平行，报问题。
	bad := []Panel{{Title: "图1", Spec: Spec{Points: []Point{
		{Name: "A", X: 10, Y: 50}, {Name: "G", X: 10, Y: 20},
		{Name: "C", X: 20, Y: 30}, {Name: "E", X: 60, Y: 30},
	}}}}
	CheckParallelRelations(bad, stem)
	if len(bad[0].Issues) == 0 {
		t.Fatalf("expected non-parallel issue")
	}

	// 两段都竖直 → 平行，不报。
	good := []Panel{{Title: "图1", Spec: Spec{Points: []Point{
		{Name: "A", X: 10, Y: 50}, {Name: "G", X: 15, Y: 20},
		{Name: "C", X: 20, Y: 30}, {Name: "E", X: 25, Y: 5},
	}}}}
	CheckParallelRelations(good, stem)
	if len(good[0].Issues) != 0 {
		t.Fatalf("unexpected issues: %v", good[0].Issues)
	}
}
