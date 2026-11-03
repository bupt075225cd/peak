package geom

import (
	"math"
	"testing"
)

// 复现任务 54 的第一张图：模型给两条直线各造了一对构造点（斜率目测），
// 标注点 A/B/C/D 是另一套坐标，结果 C 浮在 l₁ 下方、B 也不在线上。
const task54Panel1 = `{
  "title":"图1","canvas":{"width":110,"height":75},
  "points":[
    {"name":"O","x":60,"y":55},{"name":"A","x":28,"y":55},{"name":"B","x":60,"y":23},
    {"name":"C","x":68,"y":15},{"name":"D","x":44,"y":39},
    {"name":"xp","x":106,"y":55,"hidden":true},{"name":"yp","x":60,"y":6,"hidden":true},
    {"name":"P1","x":2.556,"y":73.5,"hidden":true},{"name":"P2","x":93.222,"y":1.5,"hidden":true},
    {"name":"Q1","x":55.138,"y":73.5,"hidden":true},{"name":"Q2","x":72.862,"y":1.5,"hidden":true}
  ],
  "segments":[
    {"from":"P1","to":"P2","extend":true},
    {"from":"Q1","to":"Q2","extend":true}
  ],
  "axes":{"origin":"O","x":{"to":"xp"},"y":{"to":"yp"}}
}`

func segLine(t *testing.T, s *Spec, idx int) line2 {
	t.Helper()
	p, ok1 := s.PointByName(s.Segments[idx].From)
	q, ok2 := s.PointByName(s.Segments[idx].To)
	if !ok1 || !ok2 {
		t.Fatalf("segment %d endpoint missing", idx)
	}
	return line2{a: vec{p.X, p.Y}, b: vec{q.X, q.Y}}
}

func assertOnLine(t *testing.T, s *Spec, name string, l line2, tol float64) {
	t.Helper()
	p, ok := s.PointByName(name)
	if !ok {
		t.Fatalf("point %s missing", name)
	}
	if d := distToLine(vec{p.X, p.Y}, l); d > tol {
		t.Fatalf("point %s should lie on the line, distance=%.3f", name, d)
	}
}

// 对齐后：l₁ 穿过 A、B、C、D，l₂ 穿过 O、C，C 正好是两线交点。
func TestReconcileAnchorsExtendedLines(t *testing.T) {
	panels, err := ParsePanels(task54Panel1)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	spec := &panels[0].Spec
	l1 := segLine(t, spec, 0)
	l2 := segLine(t, spec, 1)
	for _, name := range []string{"A", "B", "C", "D"} {
		assertOnLine(t, spec, name, l1, 0.05)
	}
	for _, name := range []string{"O", "C"} {
		assertOnLine(t, spec, name, l2, 0.05)
	}
	c, _ := spec.PointByName("C")
	q, ok := lineCross(l1, l2)
	if !ok {
		t.Fatal("lines should intersect")
	}
	if d := math.Hypot(c.X-q.X, c.Y-q.Y); d > 0.05 {
		t.Fatalf("C should sit exactly at the intersection, off by %.3f", d)
	}
}

// 吸附不能把点拖离坐标轴：A 在 x 轴上、B 在 y 轴上，对齐后仍应在轴上。
func TestReconcileKeepsAxisPointsOnAxis(t *testing.T) {
	panels, err := ParsePanels(task54Panel1)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	spec := &panels[0].Spec
	o, _ := spec.PointByName("O")
	a, _ := spec.PointByName("A")
	b, _ := spec.PointByName("B")
	if math.Abs(a.Y-o.Y) > 1e-6 {
		t.Fatalf("A should stay on the x axis, got y=%v (axis y=%v)", a.Y, o.Y)
	}
	if math.Abs(b.X-o.X) > 1e-6 {
		t.Fatalf("B should stay on the y axis, got x=%v (axis x=%v)", b.X, o.X)
	}
}

// 第二张测试图的真实形态：l₁ 只挂了 A 一个标注点、l₂ 只挂了 C，
// 交点 C 偏在 l₁ 外 9 个单位。对齐后 C 应落在两线交点，B 落在 l₁ 与 y 轴交点。
const task45Panel1 = `{
  "title":"图1","canvas":{"width":100,"height":70},
  "points":[
    {"name":"O","x":18,"y":55},{"name":"C","x":29,"y":22},
    {"name":"A","x":60,"y":55},{"name":"B","x":18,"y":30},
    {"name":"xend","x":96,"y":55,"hidden":true},{"name":"yend","x":18,"y":6,"hidden":true},
    {"name":"l1p1","x":8,"y":19,"hidden":true},
    {"name":"l2p1","x":12,"y":68,"hidden":true}
  ],
  "segments":[
    {"from":"l1p1","to":"A","extend":true},
    {"from":"l2p1","to":"C","extend":true}
  ],
  "axes":{"origin":"O","x":{"to":"xend"},"y":{"to":"yend"}}
}`

func TestReconcilePutsIntersectionOnBothLines(t *testing.T) {
	panels, err := ParsePanels(task45Panel1)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	spec := &panels[0].Spec
	l1 := segLine(t, spec, 0)
	l2 := segLine(t, spec, 1)
	for _, name := range []string{"A", "B", "C"} {
		assertOnLine(t, spec, name, l1, 0.05)
	}
	for _, name := range []string{"O", "C"} {
		assertOnLine(t, spec, name, l2, 0.05)
	}
	// C 必须正好是两条直线的交点。
	c, _ := spec.PointByName("C")
	q, ok := lineCross(l1, l2)
	if !ok {
		t.Fatal("lines should intersect")
	}
	if d := math.Hypot(c.X-q.X, c.Y-q.Y); d > 0.05 {
		t.Fatalf("C should sit at the intersection, off by %.3f", d)
	}
	// B 既在 y 轴上又在 l₁ 上。
	o, _ := spec.PointByName("O")
	b, _ := spec.PointByName("B")
	if math.Abs(b.X-o.X) > 1e-6 {
		t.Fatalf("B should be on the y axis, got x=%v", b.X)
	}
}

// 已经穿点正确的直线不应被改动（幂等且无副作用）。
func TestReconcileIdempotent(t *testing.T) {
	panels, err := ParsePanels(task54Panel1)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	spec := &panels[0].Spec
	if notes := spec.Reconcile(); len(notes) != 0 {
		t.Fatalf("second reconcile should be a no-op, got %v", notes)
	}
}

// 模型把点放偏一点点（≤lineSnapTol）时，投影到直线上。
func TestReconcileSnapsNearlyOnLinePoint(t *testing.T) {
	raw := `{"canvas":{"width":100,"height":100},
		"points":[{"name":"A","x":10,"y":80},{"name":"B","x":80,"y":20},
		          {"name":"P","x":45,"y":51}],
		"segments":[{"from":"A","to":"B","extend":true}]}`
	panels, err := ParsePanels(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	spec := &panels[0].Spec
	l := segLine(t, spec, 0)
	assertOnLine(t, spec, "P", l, 1e-6)
}

// 明显不在直线上的点（远离所有线）不能被拽过去。
func TestReconcileLeavesFarPointsAlone(t *testing.T) {
	raw := `{"canvas":{"width":100,"height":100},
		"points":[{"name":"A","x":10,"y":80},{"name":"B","x":80,"y":20},
		          {"name":"Q","x":50,"y":20}],
		"segments":[{"from":"A","to":"B","extend":true}]}`
	panels, err := ParsePanels(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	spec := &panels[0].Spec
	q, _ := spec.PointByName("Q")
	if q.X != 50 || q.Y != 20 {
		t.Fatalf("far point should not be moved, got (%v,%v)", q.X, q.Y)
	}
}
