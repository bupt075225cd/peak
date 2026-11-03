package geom

import (
	"strings"
	"testing"
)

// task51IssueSpec 复现任务 51 的问题规格：l₃ 没有画出来，
// 模型改用 O→A、O→B 两截与坐标轴重合的线段代替（不可见）。
func task51IssueSpec() *Spec {
	return &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 20, Y: 80},
			{Name: "A", X: 76, Y: 80},
			{Name: "B", X: 20, Y: 52},
			{Name: "C", X: 28, Y: 56},
			{Name: "Xp", X: 92, Y: 80, Hidden: true},
			{Name: "Yp", X: 20, Y: 10, Hidden: true},
		},
		Segments: []Segment{
			{From: "O", To: "A"},
			{From: "O", To: "B"},
			{From: "B", To: "A", Extend: true},
		},
		Axes: &Axes{Origin: "O", X: Axis{To: "Xp"}, Y: Axis{To: "Yp"}},
	}
}

// 压在坐标轴上的孤立线段必须被报出来（画出来会被轴线盖住）。
func TestCheckAxisCoincidentSegments(t *testing.T) {
	panels := []Panel{{Title: "图1", Spec: *task51IssueSpec()}}
	CheckAxisCoincidentSegments(panels)
	issues := strings.Join(panels[0].Issues, "\n")
	if !strings.Contains(issues, "O→A") || !strings.Contains(issues, "O→B") {
		t.Fatalf("expected both axis-coincident segments reported, got %q", issues)
	}
	if strings.Contains(issues, "B→A") {
		t.Fatalf("real line should not be reported, got %q", issues)
	}
}

// 三角形/矩形与坐标轴重合的边是真实边，不应误报。
func TestCheckAxisCoincidentSegmentsIgnoresClosedShapeEdges(t *testing.T) {
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 20, Y: 80},
			{Name: "A", X: 70, Y: 80},
			{Name: "B", X: 20, Y: 40},
			{Name: "Xp", X: 95, Y: 80, Hidden: true},
			{Name: "Yp", X: 20, Y: 8, Hidden: true},
		},
		// OA 落在 x 轴上、OB 落在 y 轴上，都是三角形的边。
		Segments: []Segment{{From: "O", To: "A"}, {From: "A", To: "B"}, {From: "B", To: "O"}},
		Axes:     &Axes{Origin: "O", X: Axis{To: "Xp"}, Y: Axis{To: "Yp"}},
	}
	panels := []Panel{{Title: "图1", Spec: *spec}}
	CheckAxisCoincidentSegments(panels)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("closed-shape edges on axes should be ignored, got %v", panels[0].Issues)
	}
}

// 题干点名 l₁/l₂/l₃、图形描述也确认有三条直线，但图里缺标注时必须报出来。
func TestCheckNamedLines(t *testing.T) {
	stem := "一次函数 y=nx+7 的图象 l₁ 分别与 x，y 轴交于 A，B 两点，与正比例函数 y=mx 的图象 l₂ " +
		"交于点 C（2，6），若一次函数 y=kx+k 的图象 l₃ 与直线 l₁、l₂ 不能围成三角形，则 k 的值为____"
	desc := "平面直角坐标系中三条直线l1、l2、l3的图像，l1与l2交于点C(2,6)"
	panels := []Panel{{Title: "图1", Spec: *task51IssueSpec()}}
	CheckNamedLines(panels, stem, desc, nil)
	issues := strings.Join(panels[0].Issues, "\n")
	for _, want := range []string{"l₃", "l₁", "只画出 1 条贯穿的直线"} {
		if !strings.Contains(issues, want) {
			t.Fatalf("expected %q in issues, got %q", want, issues)
		}
	}
}

// 题干小问里才引入的直线（原图没有画）不能被要求补画——
// 任务 53 的 l₃ 就是这种情况，误报会把 l₃ 逼着画出来。
func TestCheckNamedLinesSkipsLinesAbsentFromFigure(t *testing.T) {
	stem := "直线 l₁：y=kx+b 与 x 轴，y 轴分别交于 A（-4，0），B 两点，与直线 l₂：y=5x 交于点 C；" +
		"（3）直线 l₃：y=mx+2n-2 与 x 轴交于点 E"
	desc := "平面直角坐标系xOy中两条直线相交的函数图像，主图标注点A(-4,0)、B、C(1,5)、D"
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "A", X: 20, Y: 80}, {Name: "B", X: 55, Y: 48}, {Name: "O", X: 55, Y: 72},
			{Name: "Xp", X: 92, Y: 72, Hidden: true}, {Name: "Yp", X: 55, Y: 10, Hidden: true},
		},
		Segments: []Segment{{From: "A", To: "B", Extend: true}, {From: "O", To: "B", Extend: true}},
		Labels:   []Label{{X: 40, Y: 40, Text: "l₁"}, {X: 60, Y: 60, Text: "l₂"}},
		Axes:     &Axes{Origin: "O", X: Axis{To: "Xp"}, Y: Axis{To: "Yp"}},
	}
	panels := []Panel{{Title: "图1", Spec: *spec}}
	CheckNamedLines(panels, stem, desc, nil)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("l₃ only appears in the stem, should not be required, got %v", panels[0].Issues)
	}
}

// 名称已写进 labels 时不再重复报问题（l₁/l₂ 画了也标了名，只有 l₃ 缺）。
func TestCheckNamedLinesPartial(t *testing.T) {
	spec := task51IssueSpec()
	spec.Labels = []Label{
		{X: 60, Y: 60, Text: "l₁"},
		{X: 40, Y: 40, Text: "l₂"},
	}
	panels := []Panel{{Title: "图1", Spec: *spec}}
	CheckNamedLines(panels, "直线 l₁、l₂ 与 l₃ 不能围成三角形", "三条直线l1、l2、l3相交于坐标系中", []string{"l1", "l2", "l3"})
	issues := strings.Join(panels[0].Issues, "\n")
	if !strings.Contains(issues, "印刷了直线名称 l₃") || strings.Contains(issues, "印刷了直线名称 l₁") {
		t.Fatalf("only l₃ should be reported missing, got %q", issues)
	}
}

// 题干没有点名任何直线时不触发（普通几何题）。
func TestCheckNamedLinesNoNames(t *testing.T) {
	panels := []Panel{{Title: "图1", Spec: *task51IssueSpec()}}
	CheckNamedLines(panels, "在矩形ABCD中，AB=3，BC=4，求对角线AC的长", "矩形ABCD", nil)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("no named lines in stem, got %v", panels[0].Issues)
	}
}

// 备用图不参与三角形完整性核对：原图备用图本就没有 △COD 的边。
func TestCheckTriangleEdgesSkipsBackupPanel(t *testing.T) {
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "C", X: 61, Y: 42}, {Name: "O", X: 55, Y: 72}, {Name: "D", X: 43, Y: 60},
		},
		Segments: []Segment{{From: "O", To: "C"}},
	}
	panels := []Panel{{Title: "备用图", Spec: *spec}}
	CheckTriangleEdges(panels, "（2）若△COD 的面积为 6，求线段 AD 的长")
	if len(panels[0].Issues) != 0 {
		t.Fatalf("backup panel should be skipped, got %v", panels[0].Issues)
	}
	panels[0].Title = "图1"
	// 已经画出两条边（OC、CD）时才认为原图有这个三角形，应补出缺的 OD。
	panels[0].Spec.Segments = append(panels[0].Spec.Segments, Segment{From: "C", To: "D"})
	CheckTriangleEdges(panels, "（2）若△COD 的面积为 6，求线段 AD 的长")
	issues := strings.Join(panels[0].Issues, "\n")
	if !strings.Contains(issues, "OD") {
		t.Fatalf("main panel should report missing edge OD, got %q", issues)
	}
}

// 只画了一条边时不要补画：那条"边"常常只是函数图象恰好穿过两个点。
func TestCheckTriangleEdgesSkipsWhenOnlyOneEdge(t *testing.T) {
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "C", X: 61, Y: 42}, {Name: "O", X: 55, Y: 72}, {Name: "D", X: 43, Y: 60},
		},
		Segments: []Segment{{From: "O", To: "C"}},
	}
	panels := []Panel{{Title: "图1", Spec: *spec}}
	CheckTriangleEdges(panels, "（2）若△COD 的面积为 6，求线段 AD 的长")
	if len(panels[0].Issues) != 0 {
		t.Fatalf("只画出一条边时不应补画三角形，got %v", panels[0].Issues)
	}
}

// 函数图象（extend 直线）穿过三个顶点，不等于原图画了这个三角形：
// 题干里的"△COD 的面积为 6"只是计算条件，不能据此补画原图没有的边。
func TestCheckTriangleEdgesIgnoresExtendedLines(t *testing.T) {
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "C", X: 60, Y: 30}, {Name: "O", X: 45, Y: 70}, {Name: "D", X: 35, Y: 55},
			{Name: "L1a", X: 5, Y: 95, Hidden: true}, {Name: "L1b", X: 95, Y: 5, Hidden: true},
		},
		Segments: []Segment{{From: "L1a", To: "L1b", Extend: true}},
	}
	panels := []Panel{{Title: "图2", Spec: *spec}}
	CheckTriangleEdges(panels, "（2）若△COD 的面积为 6，求线段 AD 的长")
	if len(panels[0].Issues) != 0 {
		t.Fatalf("函数图象穿过顶点不应被当作三角形的边，got %v", panels[0].Issues)
	}
}

// 标注点漂浮（不在任何线或轴上）必须被报出来：实测 B 该是 l₁ 与 y 轴的交点，却成了孤点。
func TestCheckFloatingPoints(t *testing.T) {
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 50, Y: 70}, {Name: "A", X: 20, Y: 70}, {Name: "C", X: 80, Y: 30},
			{Name: "B", X: 62, Y: 20},
			{Name: "Xp", X: 96, Y: 70, Hidden: true}, {Name: "Yp", X: 50, Y: 6, Hidden: true},
		},
		Segments: []Segment{{From: "A", To: "C", Extend: true}},
		Axes:     &Axes{Origin: "O", X: Axis{To: "Xp"}, Y: Axis{To: "Yp"}},
	}
	panels := []Panel{{Title: "图1", Spec: *spec}}
	CheckFloatingPoints(panels)
	issues := strings.Join(panels[0].Issues, "\n")
	if !strings.Contains(issues, "点 B") {
		t.Fatalf("expected floating point B reported, got %q", issues)
	}
	if strings.Contains(issues, "点 A") || strings.Contains(issues, "点 C") || strings.Contains(issues, "点 O") {
		t.Fatalf("points on lines should not be reported, got %q", issues)
	}
}

// 圆心不在圆周上，不能算漂浮。
func TestCheckFloatingPointsSkipsCircleCenter(t *testing.T) {
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 50, Y: 50}, {Name: "A", X: 80, Y: 50},
			{Name: "Xp", X: 96, Y: 50, Hidden: true}, {Name: "Yp", X: 50, Y: 6, Hidden: true},
		},
		Circles: []Circle{{Center: "O", Through: "A"}},
		Axes:    &Axes{Origin: "O", X: Axis{To: "Xp"}, Y: Axis{To: "Yp"}},
	}
	panels := []Panel{{Title: "图1", Spec: *spec}}
	CheckFloatingPoints(panels)
	if len(panels[0].Issues) != 0 {
		t.Fatalf("circle center should be skipped, got %v", panels[0].Issues)
	}
}
