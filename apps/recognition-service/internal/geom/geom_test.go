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

func TestRenderPanelsFiltersForbiddenLabels(t *testing.T) {
	// 端到端：图上只保留代数式/结论类文字；数值长度、坐标、位置关系说明都不出现。
	spec := `{"canvas":{"width":100,"height":70},` +
		`"points":[{"name":"A","x":25,"y":60},{"name":"C","x":25,"y":20},` +
		`{"name":"x1","x":6,"y":60}],` +
		`"segments":[{"from":"A","to":"C"}],` +
		`"labels":[{"text":"AG∥CE","x":50,"y":40},{"text":"2.6m","x":50,"y":30},` +
		`{"text":"A(-4,0)","x":50,"y":50},{"text":"AD=2BD","x":50,"y":60}]}`
	results, err := RenderPanels(spec)
	if err != nil || len(results) != 1 {
		t.Fatalf("RenderPanels: %v, results=%+v", err, results)
	}
	svg := string(results[0].SVG)
	for _, bad := range []string{"AG∥CE", "2.6m", "A(-4,0)", ">x1<"} {
		if strings.Contains(svg, bad) {
			t.Fatalf("forbidden label %q should not be rendered: %s", bad, svg)
		}
	}
	if !strings.Contains(svg, "AD=2BD") {
		t.Fatalf("algebraic label should be kept: %s", svg)
	}
}

func TestParsePanelsExplicitEmptyPanels(t *testing.T) {
	// 模型显式返回 {"panels":[]}（判定输入不是几何示意图）→ 返回空结果、不算错误。
	panels, err := ParsePanels(`{"panels":[]}`)
	if err != nil {
		t.Fatalf("ParsePanels empty panels: %v", err)
	}
	if len(panels) != 0 {
		t.Fatalf("expected 0 panels, got %d", len(panels))
	}

	// 单子图形态的内容为空 → 仍视为垃圾输出报错（走回喂重试）。
	if _, err := ParsePanels(`{"title":"图1"}`); err == nil {
		t.Fatal("expected error for empty single-panel spec")
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

func axisSpec() *Spec {
	return &Spec{
		Canvas: Canvas{Width: 100, Height: 80},
		Points: []Point{
			{Name: "O", X: 10, Y: 70},
			{Name: "A", X: 10, Y: 25},
			{Name: "B", X: 65, Y: 25},
			{Name: "C", X: 65, Y: 70},
			{Name: "XA", X: 95, Y: 70, Hidden: true},
			{Name: "YA", X: 10, Y: 6, Hidden: true},
		},
		Segments: []Segment{
			{From: "O", To: "A"}, {From: "A", To: "B"},
			{From: "B", To: "C"}, {From: "C", To: "O"},
		},
		Axes: &Axes{
			Origin: "O",
			X:      Axis{To: "XA", Label: "x"},
			Y:      Axis{To: "YA", Label: "y"},
		},
		Curves: []Curve{{Through: []string{"A", "B", "C"}, Smooth: true}},
	}
}

func TestRenderAxesArrowAndCurve(t *testing.T) {
	s := axisSpec()
	s.Normalize()
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	str := string(out)
	for _, want := range []string{">x<", ">y<", " C ", "Z", "stroke-width=\"1\""} {
		if !strings.Contains(str, want) {
			t.Fatalf("axes svg missing %q: %s", want, str)
		}
	}
	// 轴端点是纯构造点，不应输出点名标签。
	for _, bad := range []string{">XA<", ">YA<"} {
		if strings.Contains(str, bad) {
			t.Fatalf("axis construction point label should be suppressed, got %q: %s", bad, str)
		}
	}
	// 图上不允许出现数值标注（刻度数字会随手写批注被误抄）。
	for _, bad := range []string{">1<", ">13<", ">24<"} {
		if strings.Contains(str, bad) {
			t.Fatalf("unexpected numeric label %q: %s", bad, str)
		}
	}
}

func TestRenderKeepsFigurePointUsedAsAxisEnd(t *testing.T) {
	// 模型误把图形点 C 当 y 轴端点：C 仍必须画点并显示点名，不能整点消失。
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 20, Y: 80},
			{Name: "C", X: 20, Y: 20},
			{Name: "D", X: 60, Y: 80},
			{Name: "XA", X: 92, Y: 80, Hidden: true},
		},
		Segments: []Segment{{From: "O", To: "C"}, {From: "O", To: "D"}},
		Axes:     &Axes{Origin: "O", X: Axis{To: "XA"}, Y: Axis{To: "C"}},
	}
	s.Normalize()
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	str := string(out)
	if !strings.Contains(str, ">C<") {
		t.Fatalf("figure point C used as axis end lost its label: %s", str)
	}
}

func TestRenderAxesOnlyIsStructured(t *testing.T) {
	// 只有坐标系、没有线段时也必须走结构化渲染，而不是退回 svg 兜底。
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 20, Y: 80, Hidden: true},
			{Name: "XA", X: 95, Y: 80, Hidden: true},
			{Name: "YA", X: 20, Y: 8, Hidden: true},
		},
		Axes: &Axes{Origin: "O", X: Axis{To: "XA"}, Y: Axis{To: "YA"}},
	}
	s.Normalize()
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	str := string(out)
	for _, want := range []string{"<svg ", ">x<", ">y<", "<line"} {
		if !strings.Contains(str, want) {
			t.Fatalf("axes-only svg missing %q: %s", want, str)
		}
	}
}

func TestNormalizeAxesAndCurveFiltering(t *testing.T) {
	// 原点不存在 → 整组坐标系丢弃，避免画出半截轴。
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{{Name: "A", X: 10, Y: 10}, {Name: "B", X: 20, Y: 20}},
		Axes:   &Axes{Origin: "O", X: Axis{To: "A"}, Y: Axis{To: "B"}},
	}
	s.Normalize()
	if s.Axes != nil {
		t.Fatalf("axes with undefined origin should be dropped, got %+v", s.Axes)
	}

	// 曲线：悬空引用被过滤，有效点不足 2 个整条丢弃。
	s2 := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 5, Y: 90}, {Name: "XA", X: 95, Y: 90},
			{Name: "YA", X: 5, Y: 5}, {Name: "T", X: 30, Y: 90},
		},
		Axes: &Axes{
			Origin: "O",
			X:      Axis{To: "XA", Label: " x "},
			Y:      Axis{To: "YA"},
		},
		Curves: []Curve{{Through: []string{"O", "GONE"}}, {Through: []string{"O", "T"}}},
	}
	s2.Normalize()
	if s2.Axes == nil || s2.Axes.X.Label != "x" {
		t.Fatalf("unexpected axes after normalize: %+v", s2.Axes)
	}
	if len(s2.Curves) != 1 || len(s2.Curves[0].Through) != 2 {
		t.Fatalf("unexpected curves after normalize: %+v", s2.Curves)
	}
}

func TestValidateAxesIssues(t *testing.T) {
	// 轴端点缺失可自动补齐，不算结构问题；原点缺失才是问题。
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{{Name: "O", X: 5, Y: 90}},
		Axes: &Axes{
			Origin: "GONE",
			X:      Axis{To: "NOPE"},
			Y:      Axis{To: "O"},
		},
		Curves: []Curve{{Through: []string{"O"}}},
	}
	joined := strings.Join(s.Validate(), "\n")
	for _, want := range []string{"axes 的原点引用了未定义的点", "曲线有效点不足"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected issue %q, got %s", want, joined)
		}
	}
	if strings.Contains(joined, "轴端点") {
		t.Fatalf("axis endpoints should be auto-filled, not reported: %s", joined)
	}
}

func TestNormalizeAutoFillsAxisEndpoints(t *testing.T) {
	// 模型常见错误：axes 引用了 xax/yax 却没在 points 里定义。
	// 应自动按画布边缘补齐隐藏端点，而不是丢弃坐标系或回喂重试。
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 80},
		Points: []Point{{Name: "O", X: 10, Y: 70}},
		Axes:   &Axes{Origin: "O", X: Axis{To: "xax"}, Y: Axis{To: "yax"}},
	}
	s.Normalize()
	if s.Axes == nil {
		t.Fatal("axes should be kept with auto-filled endpoints")
	}
	xEnd, okX := s.PointByName(s.Axes.X.To)
	yEnd, okY := s.PointByName(s.Axes.Y.To)
	if !okX || !okY {
		t.Fatalf("auto endpoints missing: %+v", s.Points)
	}
	if !xEnd.Hidden || !yEnd.Hidden {
		t.Fatalf("auto endpoints must be hidden: %+v / %+v", xEnd, yEnd)
	}
	if xEnd.X != 96 || xEnd.Y != 70 {
		t.Fatalf("x endpoint should extend to right edge along origin y, got %+v", xEnd)
	}
	if yEnd.X != 10 || yEnd.Y != 4 {
		t.Fatalf("y endpoint should extend to top edge along origin x, got %+v", yEnd)
	}
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	str := string(out)
	if !strings.Contains(str, ">x<") || !strings.Contains(str, ">y<") {
		t.Fatalf("axis labels missing: %s", str)
	}
	if strings.Contains(str, ">xax_end<") || strings.Contains(str, ">yax_end<") {
		t.Fatalf("auto endpoint names must not be rendered: %s", str)
	}
}

func TestNormalizeKeepsValidAxisEndpoints(t *testing.T) {
	// 已经合法存在的端点（即便落在图形外一点点）不应被改写。
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 10, Y: 90},
			{Name: "XA", X: 88, Y: 90, Hidden: true},
			{Name: "YA", X: 10, Y: 12, Hidden: true},
		},
		Axes: &Axes{Origin: "O", X: Axis{To: "XA"}, Y: Axis{To: "YA"}},
	}
	s.Normalize()
	if s.Axes.X.To != "XA" || s.Axes.Y.To != "YA" {
		t.Fatalf("valid endpoints should be preserved, got %+v", s.Axes)
	}
	if len(s.Points) != 3 {
		t.Fatalf("no extra points should be appended, got %+v", s.Points)
	}
}

func TestRenderAxisExtendsToNegativeSide(t *testing.T) {
	// A(-4,0)、D 落在 x 轴负半轴：轴线必须向左延伸覆盖到它们之外。
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 75},
		Points: []Point{
			{Name: "O", X: 32, Y: 62},
			{Name: "XA", X: 96, Y: 62, Hidden: true},
			{Name: "YA", X: 32, Y: 8, Hidden: true},
			{Name: "A", X: 12, Y: 62},
			{Name: "D", X: 22, Y: 62},
			{Name: "C", X: 12, Y: 42},
			{Name: "B", X: 32, Y: 42},
		},
		Segments: []Segment{{From: "A", To: "C"}, {From: "C", To: "B"}},
		Axes:     &Axes{Origin: "O", X: Axis{To: "XA"}, Y: Axis{To: "YA"}},
	}
	s.Normalize()
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	str := string(out)
	// 负方向伸到最远点 A(x=12) 之外一段（至少 4 个单位）→ x=8，
	// 让左侧轴线明显可见，而不是贴着 A 收尾。
	if !strings.Contains(str, `x1="32" y1="62" x2="8" y2="62"`) {
		t.Fatalf("x axis should extend to negative side through A/D: %s", str)
	}
}

func TestRenderAxisNoNegativeExtensionWhenNoPoints(t *testing.T) {
	// 原点在图形角落、负方向没有点时，不应凭空添线（保持 2 条轴射线）。
	s := axisSpec()
	s.Normalize()
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if n := strings.Count(string(out), `stroke-width="1"`); n != 2 {
		t.Fatalf("expected exactly 2 axis ray lines, got %d: %s", n, string(out))
	}
}

func TestNormalizeDropsRelationLabels(t *testing.T) {
	// "AG∥CE" / "AB⊥CD" / "AG//CE" / "A、B、C三点共线" 这类位置关系说明不画到图上；
	// 代数式仍保留。
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{{Name: "A", X: 10, Y: 90}, {Name: "C", X: 10, Y: 20}},
		Labels: []Label{
			{X: 30, Y: 40, Text: "AG∥CE"},
			{X: 30, Y: 50, Text: "AB ⊥ CD"},
			{X: 30, Y: 60, Text: "AG//CE"},
			{X: 30, Y: 70, Text: "AG 平行于 CE"},
			{X: 30, Y: 72, Text: "A、B、C三点共线"},
			{X: 30, Y: 74, Text: "AB∥CD且AG⊥CE"},
			{X: 30, Y: 80, Text: "AD=2BD"},
		},
	}
	s.Normalize()
	if len(s.Labels) != 1 || s.Labels[0].Text != "AD=2BD" {
		t.Fatalf("relation labels should be dropped, got %+v", s.Labels)
	}
}

func TestNormalizeDropsCoordinateLabels(t *testing.T) {
	// 坐标属于数值信息，同样不画到图上；代数式/结论保留。
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{{Name: "A", X: 10, Y: 90}},
		Labels: []Label{
			{X: 30, Y: 40, Text: "A(-4,0)"},
			{X: 30, Y: 50, Text: "(-4, 0)"},
			{X: 30, Y: 60, Text: "AD=2BD"},
		},
	}
	s.Normalize()
	if len(s.Labels) != 1 || s.Labels[0].Text != "AD=2BD" {
		t.Fatalf("coordinate labels should be dropped, got %+v", s.Labels)
	}
}

func TestNormalizeHidesAxisAuxPoints(t *testing.T) {
	// 模型自造的 x1/y1 轴辅助点若没有被任何图元引用，应隐藏：不画实心点、不写点名。
	// 坐标要保留，轴线仍可据此延伸到图形之外（原图的负半轴不能丢）。
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "O", X: 45, Y: 70},
			{Name: "A", X: 25, Y: 70},
			{Name: "XA", X: 95, Y: 70, Hidden: true},
			{Name: "YA", X: 45, Y: 12, Hidden: true},
			{Name: "x1", X: 8, Y: 70},
			{Name: "y1", X: 45, Y: 96},
		},
		Segments: []Segment{{From: "O", To: "A"}},
		Axes: &Axes{
			Origin: "O",
			X:      Axis{To: "XA"},
			Y:      Axis{To: "YA"},
		},
	}
	s.Normalize()
	for _, name := range []string{"x1", "y1"} {
		p, ok := s.PointByName(name)
		if !ok || !p.Hidden {
			t.Fatalf("axis aux point %s should be hidden after normalize, got %+v", name, p)
		}
	}
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	str := string(out)
	for _, bad := range []string{">x1<", ">y1<"} {
		if strings.Contains(str, bad) {
			t.Fatalf("axis aux point label should be suppressed, got %q: %s", bad, str)
		}
	}
	// 辅助点仍参与轴线延伸；最远点就是模型给定的轴端点（aux）时按它收尾，不再外扩：
	// x 轴负方向画到 x=8，y 轴负方向画到 y=96。
	for _, want := range []string{
		`x1="45" y1="70" x2="8" y2="70"`,
		`x1="45" y1="70" x2="45" y2="96"`,
	} {
		if !strings.Contains(str, want) {
			t.Fatalf("axis should still extend through aux points, missing %q: %s", want, str)
		}
	}
}

// TestRenderExtendedSegmentReachesCanvasEdge extend 的线段要画成贯穿图框的直线，
// 而不是只比线段长一点——原图里的函数图象都是从一侧穿到另一侧。
func TestRenderExtendedSegmentReachesCanvasEdge(t *testing.T) {
	s := &Spec{
		Canvas: Canvas{Width: 110, Height: 80},
		Points: []Point{
			{Name: "A", X: 30, Y: 22},
			{Name: "B", X: 92, Y: 62},
		},
		Segments: []Segment{{From: "A", To: "B", Extend: true}},
	}
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	str := string(out)
	// 左边界 1.5 进入、右边界 108.5 出去。
	if !strings.Contains(str, `x1="1.5"`) || !strings.Contains(str, `x2="108.5"`) {
		t.Fatalf("extended segment should span the canvas edges: %s", str)
	}
}

// TestRenderCollinearCurveExtendsToCanvasEdges 模型用 curves 表达的函数直线（三点共线）
// 要延长到图框边界，否则只是一段线段。
func TestRenderCollinearCurveExtendsToCanvasEdges(t *testing.T) {
	s := &Spec{
		Canvas: Canvas{Width: 110, Height: 80},
		Points: []Point{
			{Name: "P", X: 30, Y: 40},
			{Name: "Q", X: 50, Y: 30},
			{Name: "R", X: 60, Y: 25},
		},
		Curves: []Curve{{Through: []string{"P", "Q", "R"}}},
	}
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if str := string(out); !strings.Contains(str, `M 1.5 `) {
		t.Fatalf("collinear curve should start at the canvas edge: %s", str)
	}
}

// TestRenderNonCollinearCurveStaysInside 真正的曲线（抛物线）不能被延长。
func TestRenderNonCollinearCurveStaysInside(t *testing.T) {
	s := &Spec{
		Canvas: Canvas{Width: 110, Height: 80},
		Points: []Point{
			{Name: "P", X: 20, Y: 60},
			{Name: "Q", X: 50, Y: 20},
			{Name: "R", X: 80, Y: 60},
		},
		Curves: []Curve{{Through: []string{"P", "Q", "R"}}},
	}
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if str := string(out); !strings.Contains(str, `M 20 60`) {
		t.Fatalf("non-collinear curve should keep its endpoints: %s", str)
	}
}

func TestRenderSuppressesConstructionPointLabels(t *testing.T) {
	// hidden:true 是构造点（如轴辅助点、半圆圆心）：不画实心点，也不要点名。
	s := &Spec{
		Canvas: Canvas{Width: 100, Height: 100},
		Points: []Point{
			{Name: "A", X: 20, Y: 80},
			{Name: "B", X: 80, Y: 80},
			{Name: "M", X: 50, Y: 80, Hidden: true},
		},
		Segments: []Segment{{From: "A", To: "B"}},
	}
	s.Normalize()
	out, err := Render(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	str := string(out)
	if strings.Contains(str, ">M<") {
		t.Fatalf("hidden construction point label should be suppressed: %s", str)
	}
	for _, want := range []string{">A<", ">B<"} {
		if !strings.Contains(str, want) {
			t.Fatalf("figure point label %q missing: %s", want, str)
		}
	}
}
