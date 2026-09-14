package geom

import (
	"strings"
	"testing"
)

func TestNormalizeClampAndDedupe(t *testing.T) {
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 80},
		Points: []Point{
			{Name: "A", X: 200, Y: -50},
			{Name: "A", X: 1, Y: 1},
			{Name: "", X: 2, Y: 2},
			{Name: "B", X: 10, Y: 10},
		},
		Segments: []Segment{{From: "A", To: "C"}, {From: "A", To: "A"}, {From: "A", To: "B"}},
	}
	s.Normalize()
	if len(s.Points) != 2 {
		t.Fatalf("expected 2 points after dedupe, got %d", len(s.Points))
	}
	if s.Points[0].X != 100 || s.Points[0].Y != 0 {
		t.Fatalf("expected clamped coords (100,0), got (%v,%v)", s.Points[0].X, s.Points[0].Y)
	}
	if len(s.Segments) != 1 || s.Segments[0].To != "B" {
		t.Fatalf("expected only the valid segment kept, got %+v", s.Segments)
	}
}

func TestNormalizeCanvasDefault(t *testing.T) {
	s := &Spec{}
	s.Normalize()
	if s.Canvas.Width != DefaultCanvasSize || s.Canvas.Height != DefaultCanvasSize {
		t.Fatalf("expected default canvas, got %+v", s.Canvas)
	}
}

func TestNormalizeCircleRadiusFromThrough(t *testing.T) {
	s := &Spec{
		Canvas:  Canvas{Width: 100, Height: 100},
		Points:  []Point{{Name: "O", X: 50, Y: 50}, {Name: "A", X: 50, Y: 20}},
		Circles: []Circle{{Center: "O", Through: "A"}},
	}
	s.Normalize()
	if len(s.Circles) != 1 || s.Circles[0].Radius != 30 {
		t.Fatalf("expected radius 30 from through, got %+v", s.Circles)
	}
}

func TestNormalizeLabelAnchorFallback(t *testing.T) {
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{{Name: "A", X: 1, Y: 1}},
		Labels: []Label{{X: 50, Y: 50, Text: "AB=2", Anchor: "bogus"}, {X: 1, Y: 1, Text: "  "}},
	}
	s.Normalize()
	if len(s.Labels) != 1 || s.Labels[0].Anchor != "middle" {
		t.Fatalf("expected anchor fallback to middle and empty label dropped, got %+v", s.Labels)
	}
}

func TestValidateReportsIssues(t *testing.T) {
	s := &Spec{Segments: []Segment{{From: "A", To: "B"}}}
	issues := s.Validate()
	if len(issues) == 0 {
		t.Fatal("expected validation issues for empty spec")
	}
	joined := strings.Join(issues, "\n")
	if !strings.Contains(joined, "points 为空") || !strings.Contains(joined, "未定义的点") {
		t.Fatalf("unexpected issues: %v", issues)
	}
}

func TestValidateClean(t *testing.T) {
	s := &Spec{
		Canvas:   Canvas{Width: 100, Height: 100},
		Points:   []Point{{Name: "A", X: 1, Y: 1}, {Name: "B", X: 2, Y: 2}},
		Segments: []Segment{{From: "A", To: "B"}},
	}
	if issues := s.Validate(); len(issues) != 0 {
		t.Fatalf("expected no issues, got %v", issues)
	}
}

func TestMeasureTextWidth(t *testing.T) {
	cases := []struct {
		in   string
		size float64
		want float64
	}{
		{"中", 10, 10},
		{"A", 10, 5.5},
		{"", 10, 0},
		{"中A", 10, 15.5},
	}
	for _, c := range cases {
		if got := MeasureTextWidth(c.in, c.size); got != c.want {
			t.Fatalf("MeasureTextWidth(%q,%v) = %v, want %v", c.in, c.size, got, c.want)
		}
	}
}

func TestRenderBasic(t *testing.T) {
	s := &Spec{
		Canvas:   Canvas{Width: 100, Height: 100},
		Points:   []Point{{Name: "A", X: 10, Y: 10}, {Name: "B", X: 90, Y: 10}},
		Segments: []Segment{{From: "A", To: "B"}},
		Polygons: []Polygon{{Points: []string{"A", "B"}}}, // 顶点不足，应被 Normalize 丢弃
	}
	s.Normalize()
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	str := string(out)
	for _, want := range []string{
		`<svg `, `viewBox="0 0 100 100"`, `width="100"`, `height="100"`,
		`<rect`, `fill="#ffffff"`, `<line`, `stroke="#111827"`, "<text", ">A<", ">B<",
	} {
		if !strings.Contains(str, want) {
			t.Fatalf("rendered svg missing %q: %s", want, str)
		}
	}
	if strings.Contains(str, "<polygon") {
		t.Fatalf("invalid polygon should be dropped: %s", str)
	}
	// 文字不应带白色描边底（paint-order:stroke），字母须为纯色。
	if strings.Contains(str, "paint-order") || strings.Contains(str, `stroke="#ffffff"`) {
		t.Fatalf("text should not have white halo: %s", str)
	}
}

func TestRenderAllPrimitives(t *testing.T) {
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "A", X: 20, Y: 70}, {Name: "B", X: 80, Y: 70}, {Name: "C", X: 50, Y: 20},
			{Name: "O", X: 50, Y: 70},
		},
		Segments:    []Segment{{From: "A", To: "B", Extend: true, Dashed: true}},
		Polygons:    []Polygon{{Points: []string{"A", "B", "C"}, Fill: true}},
		Circles:     []Circle{{Center: "O", Through: "A"}},
		Arcs:        []Arc{{Center: "C", Radius: 10, StartAngle: 200, EndAngle: 340}},
		RightAngles: []RightAngle{{Vertex: "C", A: "A", B: "B"}},
		AngleMarks:  []AngleMark{{Vertex: "A", A: "B", B: "C", Count: 2}},
		Ticks:       []TickMark{{From: "A", To: "C", Count: 2}},
		Parallels:   []ParallelMark{{From: "A", To: "B", Count: 1}},
		Labels:      []Label{{X: 50, Y: 90, Text: "AB=AC", Anchor: "middle"}},
	}
	s.Normalize()
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	str := string(out)
	for _, want := range []string{
		"<polygon", "<circle", "<path", "<line", "<text", "AB=AC",
		"stroke-dasharray", "rgba(37,99,235,0.10)",
	} {
		if !strings.Contains(str, want) {
			t.Fatalf("rendered svg missing %q: %s", want, str)
		}
	}
}

func TestRenderEmptyError(t *testing.T) {
	if _, err := Render(&Spec{}); err == nil {
		t.Fatal("expected error for empty spec")
	}
}

func TestRenderSvgFallbackSanitized(t *testing.T) {
	s := &Spec{SVG: `<svg viewBox="0 0 10 10" onload="alert(1)">` +
		`<circle cx="1" cy="1" r="1"/><script>alert(1)</script>` +
		`<foreignObject><body>x</body></foreignObject></svg>`}
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	str := string(out)
	if strings.Contains(str, "script") || strings.Contains(str, "onload") || strings.Contains(str, "foreignObject") {
		t.Fatalf("svg fallback not sanitized: %s", str)
	}
	if !strings.Contains(str, "<circle") {
		t.Fatalf("safe content lost: %s", str)
	}
}

func TestSanitizeSVG(t *testing.T) {
	in := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">` +
		`<script>alert(1)</script>` +
		`<a xlink:href="javascript:alert(1)"><text x="1" y="1">x</text></a>` +
		`<image href="https://evil.example/x.png"/>` +
		`<circle cx="1" cy="1" r="1" onload="x()" fill="url(#g)"/></svg>`
	out := SanitizeSVG(in)
	if out == "" {
		t.Fatal("expected sanitized output")
	}
	for _, bad := range []string{"script", "onload", "javascript", "image", "xlink"} {
		if strings.Contains(strings.ToLower(out), bad) {
			t.Fatalf("sanitized svg still contains %q: %s", bad, out)
		}
	}
	if !strings.Contains(out, "<circle") || !strings.Contains(out, `fill="url(#g)"`) {
		t.Fatalf("safe content lost: %s", out)
	}
	if !strings.HasPrefix(out, `<svg xmlns="http://www.w3.org/2000/svg"`) {
		t.Fatalf("missing xmlns doc header: %s", out)
	}
}

func TestSanitizeSVGInvalid(t *testing.T) {
	for _, in := range []string{"", "not svg", "<div></div>"} {
		if out := SanitizeSVG(in); out != "" {
			t.Fatalf("expected empty for %q, got %q", in, out)
		}
	}
}

func TestRenderPanelsMultiAndSingle(t *testing.T) {
	multi := `{"panels":[` +
		`{"title":"图1","canvas":{"width":100,"height":80},` +
		`"points":[{"name":"A","x":15,"y":60},{"name":"B","x":85,"y":60},{"name":"C","x":50,"y":20}],` +
		`"segments":[{"from":"A","to":"B"}]},` +
		`{"title":"图2","canvas":{"width":100,"height":80},` +
		`"points":[{"name":"O","x":50,"y":40},{"name":"P","x":80,"y":40}],` +
		`"circles":[{"center":"O","through":"P"}]}` +
		`]}`
	results, err := RenderPanels(multi)
	if err != nil {
		t.Fatalf("RenderPanels: %v", err)
	}
	if len(results) != 2 || results[0].Title != "图1" || results[1].Title != "图2" {
		t.Fatalf("unexpected panels: %+v", results)
	}
	if !AllConsistent(results) {
		t.Fatalf("expected consistent, got issues %v", CollectIssues(results))
	}

	// 单子图（无 panels 包装），缺省标题为图1。
	single := `{"canvas":{"width":100,"height":100},` +
		`"points":[{"name":"A","x":10,"y":10},{"name":"B","x":90,"y":10}],` +
		`"segments":[{"from":"A","to":"B"}]}`
	one, err := RenderPanels(single)
	if err != nil {
		t.Fatalf("RenderPanels single: %v", err)
	}
	if len(one) != 1 || one[0].Title != "图1" {
		t.Fatalf("unexpected single panel: %+v", one)
	}

	// 引用不存在的点 → 结构校验应报问题。
	bad := `{"canvas":{"width":100,"height":100},` +
		`"points":[{"name":"A","x":10,"y":10}],` +
		`"segments":[{"from":"A","to":"B"}]}`
	badResults, err := RenderPanels(bad)
	if err != nil {
		t.Fatalf("RenderPanels bad: %v", err)
	}
	if AllConsistent(badResults) || len(CollectIssues(badResults)) == 0 {
		t.Fatalf("expected validation issues, got %+v", badResults)
	}

	if _, err := RenderPanels("not json"); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestIsAngleText(t *testing.T) {
	yes := []string{"", "40°", "∠ABC", "α", "(β)", "【γ】"}
	no := []string{"AB=2", "AD=2BD", "x"}
	for _, s := range yes {
		if !isAngleText(s) {
			t.Fatalf("isAngleText(%q) should be true", s)
		}
	}
	for _, s := range no {
		if isAngleText(s) {
			t.Fatalf("isAngleText(%q) should be false", s)
		}
	}
}
