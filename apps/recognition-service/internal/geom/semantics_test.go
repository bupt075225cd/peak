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
