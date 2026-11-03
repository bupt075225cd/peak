package geom

import (
	"strings"
	"testing"
)

// 任务 53 的现象：l₂:y=5x 只有 O、C 两个标注点被连起来，模型没写 extend，
// 重绘出来就成了 O→C 的一截线段，与原图贯穿坐标系的直线不符。
// 图上 l₂ 名称紧贴这条线，按名称标签把它延长为直线。
func TestReconcileExtendsLineNamedByLabel(t *testing.T) {
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 50, Y: 60},
			{Name: "C", X: 58, Y: 20},
			{Name: "Xp", X: 96, Y: 60, Hidden: true},
			{Name: "Yp", X: 50, Y: 4, Hidden: true},
		},
		Segments: []Segment{{From: "O", To: "C"}},
		Axes:     &Axes{Origin: "O", X: Axis{To: "Xp"}, Y: Axis{To: "Yp"}},
		Labels:   []Label{{X: 62, Y: 22, Text: "l₂"}},
	}
	spec.Normalize()
	notes := spec.Reconcile()
	if !spec.Segments[0].Extend {
		t.Fatalf("l₂ 应被延长为直线，notes=%v", notes)
	}
	svg, err := Render(spec)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// 延长后应贯穿图框：两端贴到画布上下边界（内缩 canvasInset=1.5）。
	out := string(svg)
	if !strings.Contains(out, `y1="98.5"`) || !strings.Contains(out, `y2="1.5"`) {
		t.Fatalf("直线未贯穿图框，svg=%s", out)
	}
}

// 标签离所有未延长线段都很远时保守跳过：不能凭空把某条边拉成直线。
func TestReconcileKeepsShortSegmentWhenLabelFar(t *testing.T) {
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 50, Y: 60},
			{Name: "C", X: 58, Y: 20},
			{Name: "Xp", X: 96, Y: 60, Hidden: true},
			{Name: "Yp", X: 50, Y: 4, Hidden: true},
		},
		Segments: []Segment{{From: "O", To: "C"}},
		Axes:     &Axes{Origin: "O", X: Axis{To: "Xp"}, Y: Axis{To: "Yp"}},
		Labels:   []Label{{X: 10, Y: 90, Text: "l₁"}},
	}
	spec.Normalize()
	spec.Reconcile()
	if spec.Segments[0].Extend {
		t.Fatal("标签离线段太远时不应延长")
	}
}

// 多边形的边是图形轮廓，即使旁边有 l₁ 之类的名称也不延长。
func TestReconcileDoesNotExtendPolygonEdge(t *testing.T) {
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 20, Y: 80},
			{Name: "A", X: 80, Y: 80},
			{Name: "B", X: 50, Y: 20},
			{Name: "Xp", X: 96, Y: 80, Hidden: true},
			{Name: "Yp", X: 20, Y: 4, Hidden: true},
		},
		Segments: []Segment{{From: "O", To: "A"}},
		Polygons: []Polygon{{Points: []string{"O", "A", "B"}}},
		Axes:     &Axes{Origin: "O", X: Axis{To: "Xp"}, Y: Axis{To: "Yp"}},
		Labels:   []Label{{X: 50, Y: 86, Text: "l₁"}},
	}
	spec.Normalize()
	spec.Reconcile()
	if spec.Segments[0].Extend {
		t.Fatal("多边形的边不应被延长为直线")
	}
}

// 两端都是 hidden 构造点的线段：模型用它表达"一条线"却漏写 extend，
// 渲染时按直线贯穿图框，避免画出漂浮在图中间的一小截。
func TestRenderExtendsHiddenConstructSegment(t *testing.T) {
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "L1a", X: 20, Y: 80, Hidden: true},
			{Name: "L1b", X: 80, Y: 20, Hidden: true},
		},
		Segments: []Segment{{From: "L1a", To: "L1b"}},
	}
	svg, err := Render(spec)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := string(svg)
	if !strings.Contains(out, `x1="1.5"`) || !strings.Contains(out, `x2="98.5"`) {
		t.Fatalf("构造点线段未贯穿图框，svg=%s", out)
	}
}

// 端到端走模型原始 JSON：l₂ 只写了 O→C 两个点、没有 extend，
// 但图上标了 l₂，解析阶段应把它改造成贯穿的直线。
func TestParsePanelsExtendsNamedLineFromModelJSON(t *testing.T) {
	raw := `{"title":"备用图","canvas":{"width":100,"height":80},
		"points":[{"name":"O","x":58,"y":52},{"name":"A","x":22,"y":52},{"name":"B","x":55,"y":15},
		          {"name":"C","x":62,"y":10},
		          {"name":"xend","x":88,"y":52,"hidden":true},{"name":"yend","x":58,"y":6,"hidden":true}],
		"segments":[{"from":"A","to":"C","extend":true},{"from":"O","to":"C"}],
		"axes":{"origin":"O","x":{"to":"xend"},"y":{"to":"yend"}},
		"labels":[{"x":66,"y":8,"text":"l₂"}]}`
	panels, err := ParsePanels(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(panels) != 1 {
		t.Fatalf("want 1 panel, got %d", len(panels))
	}
	found := false
	for _, seg := range panels[0].Spec.Segments {
		if seg.From == "O" && seg.To == "C" {
			found = true
			if !seg.Extend {
				t.Fatalf("O→C 应被延长为直线，got %+v", panels[0].Spec.Segments)
			}
		}
	}
	if !found {
		t.Fatalf("O→C 线段不应被丢弃，got %+v", panels[0].Spec.Segments)
	}
}

// 只有两个点的 curves 就是一条直线：模型常用它表达 y=kx+b 的函数图象，
// 不延长同样会画成一截线段。
func TestRenderExtendsTwoPointCurve(t *testing.T) {
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 50, Y: 60},
			{Name: "C", X: 58, Y: 20},
		},
		Curves: []Curve{{Through: []string{"O", "C"}}},
	}
	svg, err := Render(spec)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := string(svg)
	if !strings.Contains(out, `M 42.3 98.5`) || !strings.Contains(out, `L 61.7 1.5`) {
		t.Fatalf("两点曲线未延长为直线，svg=%s", out)
	}
}

// 三点以上不共线的折线（如 y=|x| 的 V 形）必须保持原样，不能被拉成直线。
func TestRenderKeepsNonCollinearPolyline(t *testing.T) {
	spec := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "A", X: 20, Y: 20},
			{Name: "B", X: 50, Y: 70},
			{Name: "C", X: 80, Y: 20},
		},
		Curves: []Curve{{Through: []string{"A", "B", "C"}}},
	}
	svg, err := Render(spec)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(string(svg), `M 20 20 L 50 70 L 80 20`) {
		t.Fatalf("折线不应被延长，svg=%s", svg)
	}
}
