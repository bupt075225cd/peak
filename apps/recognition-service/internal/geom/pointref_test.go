package geom

import (
	"strings"
	"testing"
)

// 模型常把 segments 的 from/to 直接写成坐标（对象或数组），
// 解析必须容忍，并把坐标补成隐藏构造点，而不是让整次解析失败。
func TestParsePanelsAcceptsCoordinateEndpoints(t *testing.T) {
	raw := `{"title":"图1","canvas":{"width":100,"height":70},
		"points":[{"name":"O","x":20,"y":55}],
		"segments":[
			{"from":{"x":15,"y":50.5},"to":{"x":60,"y":18.5},"extend":true},
			{"from":[22,58],"to":"O"},
			{"from":{"l1":[40,30]},"to":{"x":70,"y":55}}
		]}`
	panels, err := ParsePanels(raw)
	if err != nil {
		t.Fatalf("parse panels: %v", err)
	}
	if len(panels) != 1 || len(panels[0].Spec.Segments) != 3 {
		t.Fatalf("expected 1 panel with 3 segments, got %+v", panels)
	}
	spec := panels[0].Spec
	want := []struct {
		name string
		x, y float64
	}{
		{"", 15, 50.5},
		{"", 22, 58},
		{"", 40, 30},
	}
	for i, w := range want {
		p, ok := spec.PointByName(spec.Segments[i].From)
		if !ok {
			t.Fatalf("segment %d from=%q should be backed by a generated point", i, spec.Segments[i].From)
		}
		if p.X != w.x || p.Y != w.y {
			t.Fatalf("segment %d coordinate endpoint: got (%v,%v) want (%v,%v)", i, p.X, p.Y, w.x, w.y)
		}
		if !p.Hidden {
			t.Fatalf("generated coordinate point %s should be hidden", p.Name)
		}
	}
	out, err := Render(&spec)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if n := strings.Count(string(out), "stroke-width=\"0.95\""); n != 3 {
		t.Fatalf("expected 3 segment lines, got %d: %s", n, out)
	}
}

// points 被写成对象（模型常见违规）时也要能解析。
func TestParsePanelsAcceptsPointsObject(t *testing.T) {
	raw := `{"canvas":{"width":100,"height":70},
		"points":{"A":{"x":10,"y":20},"B":[40,50],"C":{"x":60,"y":30,"hidden":true}},
		"segments":[{"from":"A","to":"B"}]}`
	panels, err := ParsePanels(raw)
	if err != nil {
		t.Fatalf("parse panels: %v", err)
	}
	spec := panels[0].Spec
	a, ok := spec.PointByName("A")
	if !ok || a.X != 10 || a.Y != 20 {
		t.Fatalf("point A should be (10,20), got %+v", a)
	}
	b, ok := spec.PointByName("B")
	if !ok || b.X != 40 || b.Y != 50 {
		t.Fatalf("point B should be (40,50), got %+v", b)
	}
	if c, ok := spec.PointByName("C"); ok && !c.Hidden {
		t.Fatalf("point C should stay hidden, got %+v", c)
	}
}

// 点名写法必须保持原样解析（不能被坐标容错逻辑带偏）。
func TestParsePanelsCharacterNamesUnaffected(t *testing.T) {
	raw := `{"canvas":{"width":100,"height":70},
		"points":[{"name":"A","x":10,"y":10},{"name":"B","x":40,"y":40}],
		"segments":[{"from":"A","to":"B"}]}`
	panels, err := ParsePanels(raw)
	if err != nil {
		t.Fatalf("parse panels: %v", err)
	}
	seg := panels[0].Spec.Segments[0]
	if seg.From != "A" || seg.To != "B" {
		t.Fatalf("unexpected segment refs: %+v", seg)
	}
	if len(panels[0].Spec.Points) != 2 {
		t.Fatalf("no extra points should be created, got %+v", panels[0].Spec.Points)
	}
}
