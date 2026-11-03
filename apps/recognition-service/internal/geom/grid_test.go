package geom

import (
	"strings"
	"testing"
)

// TestRenderGrid 验证 grid 被画成浅灰细线：(cols+1) 条竖线 + (rows+1) 条横线。
func TestRenderGrid(t *testing.T) {
	s := &Spec{
		Canvas:   Canvas{Width: 100, Height: 100},
		Grid:     &Grid{X: 10, Y: 10, Cell: 10, Cols: 4, Rows: 3},
		Points:   []Point{{Name: "A", X: 10, Y: 40}, {Name: "B", X: 50, Y: 10}},
		Segments: []Segment{{From: "A", To: "B"}},
	}
	s.Normalize()
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	str := string(out)
	if n, want := strings.Count(str, `stroke="`+colorGrid+`"`), (4+1)+(3+1); n != want {
		t.Fatalf("grid lines = %d, want %d:\n%s", n, want, str)
	}
	// 网格线要细，不能盖住图形主线条。
	if !strings.Contains(str, `stroke-width="0.35"`) {
		t.Fatalf("expected thin grid stroke: %s", str)
	}
}

// TestNormalizeDropsInvalidGrid 参数非法（cell<=0）时整体丢弃网格，不影响其它图元渲染。
func TestNormalizeDropsInvalidGrid(t *testing.T) {
	s := &Spec{
		Canvas:   Canvas{Width: 100, Height: 100},
		Grid:     &Grid{X: 10, Y: 10, Cell: 0, Cols: 4, Rows: 3},
		Points:   []Point{{Name: "A", X: 10, Y: 40}, {Name: "B", X: 50, Y: 10}},
		Segments: []Segment{{From: "A", To: "B"}},
	}
	s.Normalize()
	if s.Grid != nil {
		t.Fatalf("invalid grid should be dropped, got %+v", s.Grid)
	}
	if _, err := Render(s); err != nil {
		t.Fatalf("render: %v", err)
	}
}

// TestRepairCoordRefs 验证把"端点写成坐标"的线段补成隐藏点，且修复后结构校验通过。
func TestRepairCoordRefs(t *testing.T) {
	s := &Spec{
		Canvas:   Canvas{Width: 100, Height: 100},
		Points:   []Point{{Name: "A", X: 10, Y: 10}},
		Segments: []Segment{{From: "A", To: "40,10"}},
	}
	notes := s.RepairCoordRefs()
	if len(notes) != 1 {
		t.Fatalf("expected 1 repair, got %v", notes)
	}
	if issues := s.Validate(); len(issues) != 0 {
		t.Fatalf("validate after repair should pass, got %v", issues)
	}
	p, ok := s.PointByName(s.Segments[0].To)
	if !ok || !p.Hidden || p.X != 40 || p.Y != 10 {
		t.Fatalf("repaired point = %+v (ok=%v)", p, ok)
	}
	s.Normalize()
	if _, err := Render(s); err != nil {
		t.Fatalf("render: %v", err)
	}
}
