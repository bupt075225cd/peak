package geom

import (
	"os"
	"testing"
)

func TestZZReplay(t *testing.T) {
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 80},
		Points: []Point{
			{Name: "O", X: 18, Y: 72},
			{Name: "XA", X: 88, Y: 72, Hidden: true},
			{Name: "YA", X: 18, Y: 8, Hidden: true},
			{Name: "A", X: 68, Y: 72},
			{Name: "B", X: 18, Y: 40},
			{Name: "C", X: 33, Y: 48},
		},
		Segments: []Segment{
			{From: "B", To: "A"},
			{From: "B", To: "C"},
		},
		Axes: &Axes{Origin: "O", X: Axis{To: "XA"}, Y: Axis{To: "YA"}},
	}
	s.Normalize()
	out, err := Render(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/tmp/replay.svg", out, 0o644); err != nil {
		t.Fatal(err)
	}
}
