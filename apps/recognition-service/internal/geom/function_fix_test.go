package geom

import (
	"math"
	"strings"
	"testing"
)

// visiblePoints 记录所有图上标注点（非 hidden）的坐标，用于断言"标注点绝不被移动"。
func visiblePoints(s *Spec) map[string]vec {
	out := make(map[string]vec, len(s.Points))
	for _, p := range s.Points {
		if p.Hidden {
			continue
		}
		out[p.Name] = vec{X: p.X, Y: p.Y}
	}
	return out
}

func samePoints(t *testing.T, want, got map[string]vec) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("标注点数量变化：%v → %v", want, got)
	}
	for name, p := range want {
		if q, ok := got[name]; !ok || dist(p, q) > 1e-9 {
			t.Fatalf("标注点 %s 被移动：%v → %v", name, p, got[name])
		}
	}
}

// l3ThroughOriginSpec 把 l₃ 造成"过原点"（与 TestCheckFunctionGraphPoints_lineMissesPoint 同一场景）。
func l3ThroughOriginSpec() *Spec {
	s := funcPointsSpec(20, 32, "A", "B", "P", "Q")
	s.Points = append(s.Points, Point{Name: "N", X: 50, Y: 20, Hidden: true})
	s.Segments[2] = Segment{From: "O", To: "N", Extend: true}
	s.Labels[2] = Label{X: 52, Y: 18, Text: "l₃"}
	return s
}

func xIntercept(t *testing.T, s *Spec, seg int) vec {
	t.Helper()
	ln, ok := s.segmentLine(seg)
	if !ok {
		t.Fatalf("线段 %d 不可用", seg)
	}
	axis, ok := xAxisLine(s)
	if !ok {
		t.Fatal("x 轴不可用")
	}
	q, ok := lineCross(ln, axis)
	if !ok {
		t.Fatalf("线段 %d 与 x 轴平行", seg)
	}
	return q
}

func TestFixFunctionGraphs_kxPlusKOffOrigin(t *testing.T) {
	// scale=4（C(2,6) 在画布上离 O 25.3），所以 (-1,0) → (16,60)。
	s := l3ThroughOriginSpec()
	panels := []Panel{{Title: "图1", Spec: *s}}
	before := visiblePoints(&panels[0].Spec)

	notes := FixFunctionGraphs(panels, stemFuncPoints22)
	if len(notes) != 1 || !strings.Contains(notes[0], "l₃") {
		t.Fatalf("应只修正 l₃，实际 %v", notes)
	}
	samePoints(t, before, visiblePoints(&panels[0].Spec))

	sp := &panels[0].Spec
	if q := xIntercept(t, sp, 2); math.Abs(q.X-16) > 0.5 || math.Abs(q.Y-60) > 0.5 {
		t.Fatalf("l₃ 与 x 轴交点应约在 (-1,0) 即 (16,60)，实际 %+v", q)
	}
	if o, _ := sp.PointByName("O"); distToLine(vec{X: o.X, Y: o.Y}, mustLine(t, sp, 2)) <= 1.5 {
		t.Fatal("l₃ 修正后仍穿过原点")
	}
	// 修正后语义核对不应再报 l₃ 的问题（否则又会触发一次回喂）。
	CheckFunctionGraphPoints(panels, stemFuncPoints22)
	if issued := strings.Join(panels[0].Issues, "\n"); strings.Contains(issued, "l₃") {
		t.Fatalf("修正后仍报 l₃ 问题：%s", issued)
	}
}

func TestFixFunctionGraphs_proportionalAnchoredToOrigin(t *testing.T) {
	// l₂ 被画成 C→N（不过原点），而题干要求它是正比例函数且过 C(2,6)。
	s := funcPointsSpec(20, 32, "A", "B", "P", "Q")
	s.Points = append(s.Points, Point{Name: "N", X: 44, Y: 6, Hidden: true})
	s.Segments[1] = Segment{From: "C", To: "N", Extend: true}
	s.Labels[1] = Label{X: 44, Y: 6, Text: "l₂"}
	panels := []Panel{{Title: "图1", Spec: *s}}
	before := visiblePoints(&panels[0].Spec)

	notes := FixFunctionGraphs(panels, stemFuncPoints22)
	if len(notes) != 1 || !strings.Contains(notes[0], "l₂") {
		t.Fatalf("应只修正 l₂，实际 %v", notes)
	}
	samePoints(t, before, visiblePoints(&panels[0].Spec))

	sp := &panels[0].Spec
	o, _ := sp.PointByName("O")
	ln := mustLine(t, sp, 1)
	if d := distToLine(vec{X: o.X, Y: o.Y}, ln); d > 1e-6 {
		t.Fatalf("l₂ 修正后应正好过原点，偏 %v", d)
	}
	c, _ := sp.PointByName("C")
	if d := distToLine(vec{X: c.X, Y: c.Y}, ln); d > 1e-6 {
		t.Fatalf("l₂ 修正后应仍过 C，偏 %v", d)
	}
}

func TestFixFunctionGraphs_yIntercept(t *testing.T) {
	// l₁ 被画成 A→D（D 偏离正确直线），题干 y=nx+7 要求过 (0,7) 与 C(2,6)。
	s := funcPointsSpec(20, 32, "A", "D", "P", "Q")
	s.Points = append(s.Points, Point{Name: "D", X: 60, Y: 46, Hidden: true})
	panels := []Panel{{Title: "图1", Spec: *s}}
	before := visiblePoints(&panels[0].Spec)

	notes := FixFunctionGraphs(panels, stemFuncPoints22)
	if len(notes) != 1 || !strings.Contains(notes[0], "l₁") {
		t.Fatalf("应只修正 l₁，实际 %v", notes)
	}
	samePoints(t, before, visiblePoints(&panels[0].Spec))

	sp := &panels[0].Spec
	ln := mustLine(t, sp, 0)
	// (0,7) → O(20,60) + 7*(-4) 在 y 方向 = (20,32)
	if d := distToLine(vec{X: 20, Y: 32}, ln); d > 0.5 {
		t.Fatalf("l₁ 修正后应过 (0,7)=(20,32)，偏 %v", d)
	}
	if d := distToLine(vec{X: 28, Y: 36}, ln); d > 0.5 {
		t.Fatalf("l₁ 修正后应过 C(2,6)=(28,36)，偏 %v", d)
	}
}

func TestFixFunctionGraphs_noChangeWhenCorrect(t *testing.T) {
	s := funcPointsSpec(20, 32, "A", "B", "P", "Q")
	panels := []Panel{{Title: "图1", Spec: *s}}
	before := visiblePoints(&panels[0].Spec)

	if notes := FixFunctionGraphs(panels, stemFuncPoints22); len(notes) != 0 {
		t.Fatalf("几何已自洽时不应修改，实际 %v", notes)
	}
	samePoints(t, before, visiblePoints(&panels[0].Spec))
	if len(panels[0].Spec.Points) != len(s.Points) {
		t.Fatalf("不应追加构造点，%d → %d", len(s.Points), len(panels[0].Spec.Points))
	}
	for i := range s.Segments {
		if panels[0].Spec.Segments[i] != s.Segments[i] {
			t.Fatalf("线段 %d 被无故改动：%+v → %+v", i, s.Segments[i], panels[0].Spec.Segments[i])
		}
	}
}

func TestFixFunctionGraphs_withoutScale(t *testing.T) {
	// 题干没给任何坐标 → 估不出比例尺，只能做"交点落在 O 左侧"的最小修正。
	const stem = "如图，一次函数 y=kx+k 的图象 l₃ 与 x 轴交于点 E，求 k 的值。"
	s := funcPointsSpec(20, 32, "A", "B", "P", "Q")
	s.Points = append(s.Points, Point{Name: "N", X: 50, Y: 20, Hidden: true})
	s.Segments[2] = Segment{From: "O", To: "N", Extend: true}
	s.Labels[2] = Label{X: 52, Y: 18, Text: "l₃"}
	panels := []Panel{{Title: "图1", Spec: *s}}
	before := visiblePoints(&panels[0].Spec)

	notes := FixFunctionGraphs(panels, stem)
	if len(notes) != 1 {
		t.Fatalf("无比例尺时仍应做最小修正，实际 %v", notes)
	}
	samePoints(t, before, visiblePoints(&panels[0].Spec))

	sp := &panels[0].Spec
	o, _ := sp.PointByName("O")
	q := xIntercept(t, sp, 2)
	if !(q.X < o.X-1) {
		t.Fatalf("l₃ 与 x 轴交点应落在 O 左侧，O=%+v 交点=%+v", o, q)
	}
}

// mustLine 取线段所在直线（测试辅助）。
func mustLine(t *testing.T, s *Spec, seg int) line2 {
	t.Helper()
	ln, ok := s.segmentLine(seg)
	if !ok {
		t.Fatalf("线段 %d 无法构成直线", seg)
	}
	return ln
}
