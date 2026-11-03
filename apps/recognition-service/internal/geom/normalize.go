package geom

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// maxValidateIssues 单次结构校验最多报告的问题条数（避免回喂文本过长）。
const maxValidateIssues = 12

// coordRefRe 匹配"把端点直接写成坐标"的引用（如 "10,15"、"10.5, 15"）。
var coordRefRe = regexp.MustCompile(`^\s*(-?\d+(?:\.\d+)?)\s*,\s*(-?\d+(?:\.\d+)?)\s*$`)

// RepairCoordRefs 把"用坐标字符串当点名"的引用补成隐藏构造点，返回修复说明。
// 网格图上模型常把格线端点写成 {"from":"10,15","to":"70,15"}，结构校验会判
// "引用了未定义的点"而丢掉整条线；这里按坐标造点，保住模型已经定好的位置。
func (s *Spec) RepairCoordRefs() []string {
	if s == nil {
		return nil
	}
	seen := make(map[string]bool, len(s.Points))
	for _, p := range s.Points {
		seen[strings.TrimSpace(p.Name)] = true
	}
	var notes []string
	fix := func(ref *string) {
		raw := strings.TrimSpace(*ref)
		if raw == "" || seen[raw] {
			return
		}
		m := coordRefRe.FindStringSubmatch(raw)
		if m == nil {
			return
		}
		x, errX := strconv.ParseFloat(m[1], 64)
		y, errY := strconv.ParseFloat(m[2], 64)
		if errX != nil || errY != nil {
			return
		}
		name := fmt.Sprintf("p%d", len(s.Points)+1)
		for seen[name] {
			name += "_"
		}
		seen[name] = true
		s.Points = append(s.Points, Point{Name: name, X: x, Y: y, Hidden: true})
		notes = append(notes, raw+"→"+name)
		*ref = name
	}
	for i := range s.Segments {
		fix(&s.Segments[i].From)
		fix(&s.Segments[i].To)
	}
	for i := range s.Polygons {
		for j := range s.Polygons[i].Points {
			fix(&s.Polygons[i].Points[j])
		}
	}
	for i := range s.Curves {
		for j := range s.Curves[i].Through {
			fix(&s.Curves[i].Through[j])
		}
	}
	return notes
}

// numericLabelRe 纯数值长度标注（如 "2.6"、"2.6m"、"2.6米"）。
// 已知长度以题干文字为准，图上不标数值长度，渲染前直接剔除。
var numericLabelRe = regexp.MustCompile(
	`(?i)^[≈≈~]?\s*\d+(?:\.\d+)?\s*(mm|cm|dm|km|m|米|厘米|毫米|分米|千米)?[。.。]?$`)

// relationLabelRe 纯位置关系标注：如 "AG∥CE"、"AB ⊥ CD"、"AG//CE"、"AG平行于CE"、
// "A、B、C三点共线"（可用 、，；且 连接多条关系）。
// 平行、垂直、共线等关系应体现在图形本身，图上不写这类说明文字。
var relationLabelRe = regexp.MustCompile(
	`^(?:\s*(?:` +
		`[A-Z][0-9]?[A-Z][0-9]?\s*(?:∥|//|⊥|平行于|垂直于|平行|垂直)\s*[A-Z][0-9]?[A-Z][0-9]?` +
		`|(?:[A-Z][0-9]?\s*[、,，]\s*){2,}[A-Z][0-9]?\s*(?:三点)?\s*共线` +
		`|(?:[A-Z][0-9]?){3}\s*(?:三点)?\s*共线` +
		`)\s*(?:[，,、且；;。.]\s*)*)+$`)

// coordLabelRe 纯坐标标注（如 "A(-4,0)"、"(-4, 0)"）。
// 坐标属于数值信息，按"图上不标数值"的规则同样不画到图上。
var coordLabelRe = regexp.MustCompile(
	`^\s*(?:[A-Z][0-9]?\s*)?[（(]\s*[-−－+]?\s*\d+(?:\.\d+)?\s*[,，]\s*[-−－+]?\s*\d+(?:\.\d+)?\s*[)）]\s*$`)

// axisAuxPointRe 匹配模型自造的坐标轴辅助点（如 x1/y1、x_axis/yax）。
// 这类小写点名只用于画轴，不是题图中的点名。
var axisAuxPointRe = regexp.MustCompile(`^[xy](?:[0-9]+|_?(?:ax(?:is)?|end)[0-9]*)$`)

// isAxisAuxPoint 判断点名是否是坐标轴辅助点。
func isAxisAuxPoint(name string) bool {
	return axisAuxPointRe.MatchString(strings.TrimSpace(name))
}

// Normalize 对模型输出做健壮性处理：补齐画布、裁剪坐标、剔除引用了不存在点的图元。
func (s *Spec) Normalize() {
	if s == nil {
		return
	}
	if s.Canvas.Width <= 0 || !isFinite(s.Canvas.Width) {
		s.Canvas.Width = DefaultCanvasSize
	}
	if s.Canvas.Height <= 0 || !isFinite(s.Canvas.Height) {
		s.Canvas.Height = DefaultCanvasSize
	}
	w, h := s.Canvas.Width, s.Canvas.Height
	maxR := maxRadiusRatio * math.Max(w, h)

	// 网格背景：参数非法就整体丢弃（装饰性元素，不值得回喂修正），
	// 合法时把范围收进画布、行列数收进上限。
	if g := s.Grid; g != nil {
		if !isFinite(g.Cell) || g.Cell <= 0 || g.Cols <= 0 || g.Rows <= 0 {
			s.Grid = nil
		} else {
			g.Cols = minInt(g.Cols, maxGridCols)
			g.Rows = minInt(g.Rows, maxGridRows)
			g.X = clampF(g.X, 0, w)
			g.Y = clampF(g.Y, 0, h)
		}
	}

	// 点：去重、裁剪坐标。
	seen := make(map[string]bool, len(s.Points))
	points := s.Points[:0]
	for _, p := range s.Points {
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" || seen[p.Name] {
			continue
		}
		seen[p.Name] = true
		p.X = clampF(p.X, 0, w)
		p.Y = clampF(p.Y, 0, h)
		points = append(points, p)
	}
	s.Points = points

	exists := func(name string) bool { return seen[strings.TrimSpace(name)] }

	// 线段：剔除悬空引用与自环。
	segments := s.Segments[:0]
	for _, seg := range s.Segments {
		if exists(seg.From) && exists(seg.To) && seg.From != seg.To {
			segments = append(segments, seg)
		}
	}
	s.Segments = segments

	// 多边形：过滤不存在的顶点，剩余不足 3 个则整条丢弃。
	polygons := s.Polygons[:0]
	for _, poly := range s.Polygons {
		nodes := make([]string, 0, len(poly.Points))
		for _, name := range poly.Points {
			if exists(name) {
				nodes = append(nodes, name)
			}
		}
		if len(nodes) >= 3 {
			poly.Points = nodes
			polygons = append(polygons, poly)
		}
	}
	s.Polygons = polygons

	// 圆：半径缺失时用 center→through 距离补齐，并限制在合理范围。
	circles := s.Circles[:0]
	for _, c := range s.Circles {
		if !exists(c.Center) {
			continue
		}
		if c.Radius <= 0 || !isFinite(c.Radius) {
			c.Radius = 0
		}
		if c.Radius == 0 && exists(c.Through) && c.Through != c.Center {
			c.Radius = s.distance(c.Center, c.Through)
		}
		if c.Radius <= 0 {
			continue
		}
		c.Radius = clampF(c.Radius, 0.5, maxR)
		circles = append(circles, c)
	}
	s.Circles = circles

	// 弧：角度归一化到 [0,360)，起止相同则丢弃。
	arcs := s.Arcs[:0]
	for _, a := range s.Arcs {
		if !exists(a.Center) || a.Radius <= 0 || !isFinite(a.Radius) {
			continue
		}
		a.Radius = clampF(a.Radius, 0.5, maxR)
		a.StartAngle = normalizeAngle(a.StartAngle)
		a.EndAngle = normalizeAngle(a.EndAngle)
		if a.StartAngle == a.EndAngle {
			continue
		}
		arcs = append(arcs, a)
	}
	s.Arcs = arcs

	// 坐标系：原点必须存在，否则整组丢弃（没有原点就无法确定轴的位置）。
	// 轴端点缺失/引用未定义的点/与原点或彼此重合时，自动按"从原点延伸到画布边缘"
	// 生成隐藏构造点补齐——这类问题模型经常犯，补齐比丢弃或回喂重试都划算。
	if a := s.Axes; a != nil {
		a.Origin = strings.TrimSpace(a.Origin)
		a.X.To = strings.TrimSpace(a.X.To)
		a.Y.To = strings.TrimSpace(a.Y.To)
		a.X.Label = strings.TrimSpace(a.X.Label)
		a.Y.Label = strings.TrimSpace(a.Y.Label)
		origin, okOrigin := s.PointByName(a.Origin)
		if a.Origin == "" || !okOrigin {
			s.Axes = nil
		} else {
			take := func(name string, horizontal bool, other string) string {
				if name != "" && exists(name) && name != a.Origin && name != other {
					return name
				}
				p := s.autoAxisEnd(origin, horizontal)
				p.Name = s.uniquePointName(axisEndName(horizontal), seen)
				seen[p.Name] = true
				s.Points = append(s.Points, p)
				return p.Name
			}
			a.X.To = take(a.X.To, true, "")
			a.Y.To = take(a.Y.To, false, a.X.To)
		}
	}

	// 坐标轴辅助点（模型常用 x1/y1 这类小写点表示轴的另一端）不是题图要素：
	// 未被任何图元引用时一律隐藏，避免被渲染成带 "x1"/"y1" 名字的点与标签。
	// 保留坐标是有意的：轴线仍按这些点延伸到图形之外，原图的负半轴不会丢。
	used := figurePointNames(s)
	for i := range s.Points {
		if p := &s.Points[i]; !used[p.Name] && isAxisAuxPoint(p.Name) {
			p.Hidden = true
		}
	}

	// 曲线：过滤引用了不存在点的节点，剩余不足 2 个则整条丢弃。
	if len(s.Curves) > 0 {
		curves := s.Curves[:0]
		for _, c := range s.Curves {
			nodes := make([]string, 0, len(c.Through))
			for _, name := range c.Through {
				if exists(name) {
					nodes = append(nodes, name)
				}
			}
			if len(nodes) < 2 {
				continue
			}
			c.Through = nodes
			curves = append(curves, c)
		}
		s.Curves = curves
	}

	rightAngles := s.RightAngles[:0]
	for _, ra := range s.RightAngles {
		if exists(ra.Vertex) && exists(ra.A) && exists(ra.B) &&
			ra.A != ra.B && ra.A != ra.Vertex && ra.B != ra.Vertex {
			rightAngles = append(rightAngles, ra)
		}
	}
	s.RightAngles = rightAngles

	// 角的弧线标记忠实原图：保留引用合法的标记（提示词要求"原图没有就不要输出"），
	// 引用未定义点的仍按结构问题上报，走回喂修正。
	angleMarks := s.AngleMarks[:0]
	for _, am := range s.AngleMarks {
		if exists(am.Vertex) && exists(am.A) && exists(am.B) &&
			am.A != am.B && am.A != am.Vertex && am.B != am.Vertex {
			angleMarks = append(angleMarks, am)
		}
	}
	s.AngleMarks = angleMarks

	ticks := s.Ticks[:0]
	for _, m := range s.Ticks {
		if m.From != m.To && exists(m.From) && exists(m.To) {
			m.Count = clampCount(m.Count)
			ticks = append(ticks, m)
		}
	}
	s.Ticks = ticks

	parallels := s.Parallels[:0]
	for _, m := range s.Parallels {
		if m.From != m.To && exists(m.From) && exists(m.To) {
			m.Count = clampCount(m.Count)
			parallels = append(parallels, m)
		}
	}
	s.Parallels = parallels

	labels := s.Labels[:0]
	for _, l := range s.Labels {
		l.Text = strings.TrimSpace(l.Text)
		if l.Text == "" {
			continue
		}
		// 图上不标数值长度：纯数值（可带单位）、纯坐标标注直接剔除，保留代数式/结论等文字。
		if numericLabelRe.MatchString(l.Text) || coordLabelRe.MatchString(l.Text) {
			continue
		}
		// 点名由 points 自动标注：labels 里重复写点名会在图上出现两个同名标签。
		if seen[l.Text] {
			continue
		}
		// 平行/垂直/共线等位置关系由图形本身表达，图上不写 "AG∥CE" 这类说明文字。
		if relationLabelRe.MatchString(l.Text) {
			continue
		}
		l.X = clampF(l.X, 0, w)
		l.Y = clampF(l.Y, 0, h)
		switch l.Anchor {
		case "start", "middle", "end":
		default:
			l.Anchor = "middle"
		}
		labels = append(labels, l)
	}
	s.Labels = labels

	s.SVG = strings.TrimSpace(s.SVG)
}

// axisEndName 自动生成的轴端点基础点名（不会被渲染成标签）。
func axisEndName(horizontal bool) string {
	if horizontal {
		return "xax_end"
	}
	return "yax_end"
}

// uniquePointName 返回不与已有点重名的点名（必要时追加序号）。
func (s *Spec) uniquePointName(base string, seen map[string]bool) string {
	name := base
	for i := 2; seen[name]; i++ {
		name = base + strconv.Itoa(i)
	}
	return name
}

// autoAxisEnd 为缺失的轴端点生成兜底坐标：从原点延伸到画布边缘。
// 约定 x 轴向右、y 轴向上；原点已靠右/靠上时自动反向，保证轴始终伸出图形之外。
func (s *Spec) autoAxisEnd(origin Point, horizontal bool) Point {
	const margin = 4.0
	w, h := s.Canvas.Width, s.Canvas.Height
	if horizontal {
		x := w - margin
		if origin.X > w*0.55 {
			x = margin
		}
		return Point{X: clampF(x, 0, w), Y: origin.Y, Hidden: true}
	}
	y := margin
	if origin.Y < h*0.45 {
		y = h - margin
	}
	return Point{X: origin.X, Y: clampF(y, 0, h), Hidden: true}
}

// Empty 判断该图形描述是否没有任何可绘制内容。
// 仅有 svg 兜底字段（无结构化图元）不算空。
func (s *Spec) Empty() bool {
	if s == nil {
		return true
	}
	if len(s.Points) == 0 && len(s.Segments) == 0 && len(s.Polygons) == 0 &&
		len(s.Circles) == 0 && len(s.Arcs) == 0 && len(s.Curves) == 0 && s.Axes == nil {
		return strings.TrimSpace(s.SVG) == ""
	}
	return false
}

// PointByName 按名称查找点。
func (s *Spec) PointByName(name string) (Point, bool) {
	if s == nil {
		return Point{}, false
	}
	name = strings.TrimSpace(name)
	for _, p := range s.Points {
		if p.Name == name {
			return p, true
		}
	}
	return Point{}, false
}

// distance 计算两个命名点之间的距离（任一点不存在返回 0）。
func (s *Spec) distance(from, to string) float64 {
	a, okA := s.PointByName(from)
	b, okB := s.PointByName(to)
	if !okA || !okB {
		return 0
	}
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}

// Validate 结构校验：替代旧方案的"约束残差"判据，作为回喂修正与是否自洽的依据。
//
// 应在 Normalize 之前对模型原始输出调用，用于发现结构问题；
// 在 Normalize 之后调用则只报告无法自动修复的残留问题。
// 返回可读问题列表，空表示结构完整、可直接渲染。
func (s *Spec) Validate() []string {
	if s == nil {
		return []string{"几何描述为空"}
	}
	issues := make([]string, 0, 4)
	add := func(format string, args ...any) {
		if len(issues) < maxValidateIssues {
			issues = append(issues, fmt.Sprintf(format, args...))
		}
	}

	if s.Canvas.Width <= 0 || !isFinite(s.Canvas.Width) ||
		s.Canvas.Height <= 0 || !isFinite(s.Canvas.Height) {
		add("canvas 尺寸缺失或非法（需为正数）")
	}

	hasSVG := strings.TrimSpace(s.SVG) != ""
	if hasSVG && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(s.SVG)), "<svg") {
		add("svg 兜底字段不是 <svg> 文档")
	}
	if len(s.Points) == 0 && !hasSVG {
		add("points 为空，至少需要一个点")
	}

	seen := make(map[string]bool, len(s.Points))
	for i, p := range s.Points {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			add("第 %d 个点缺少 name", i+1)
			continue
		}
		if seen[name] {
			add("点 %s 重复定义", name)
			continue
		}
		seen[name] = true
		if !isFinite(p.X) || !isFinite(p.Y) {
			add("点 %s 的坐标不是有限数值", name)
		}
	}
	exists := func(name string) bool { return seen[strings.TrimSpace(name)] }

	for i, seg := range s.Segments {
		if !exists(seg.From) || !exists(seg.To) {
			add("第 %d 条线段引用了未定义的点（%s→%s）", i+1, seg.From, seg.To)
		} else if seg.From == seg.To {
			add("第 %d 条线段起止点相同（%s）", i+1, seg.From)
		}
	}
	for i, poly := range s.Polygons {
		n := 0
		for _, name := range poly.Points {
			if exists(name) {
				n++
			}
		}
		if n < 3 {
			add("第 %d 个多边形有效顶点不足 3 个", i+1)
		}
	}
	for i, c := range s.Circles {
		if !exists(c.Center) {
			add("第 %d 个圆引用了未定义的圆心 %s", i+1, c.Center)
			continue
		}
		if (c.Radius <= 0 || !isFinite(c.Radius)) &&
			(c.Through == "" || c.Through == c.Center || !exists(c.Through)) {
			add("第 %d 个圆（圆心 %s）既无有效 radius 也无可用 through", i+1, c.Center)
		}
	}
	for i, a := range s.Arcs {
		if !exists(a.Center) {
			add("第 %d 条弧引用了未定义的圆心 %s", i+1, a.Center)
			continue
		}
		if a.Radius <= 0 || !isFinite(a.Radius) {
			add("第 %d 条弧（圆心 %s）radius 非法", i+1, a.Center)
		} else if normalizeAngle(a.StartAngle) == normalizeAngle(a.EndAngle) {
			add("第 %d 条弧（圆心 %s）起止角度相同", i+1, a.Center)
		}
	}
	for i, ra := range s.RightAngles {
		if !exists(ra.Vertex) || !exists(ra.A) || !exists(ra.B) {
			add("第 %d 个直角标记引用了未定义的点（%s/%s/%s）", i+1, ra.Vertex, ra.A, ra.B)
		} else if ra.A == ra.B || ra.A == ra.Vertex || ra.B == ra.Vertex {
			add("第 %d 个直角标记顶点/边点重合", i+1)
		}
	}
	for i, am := range s.AngleMarks {
		if !exists(am.Vertex) || !exists(am.A) || !exists(am.B) {
			add("第 %d 个角标记引用了未定义的点（%s/%s/%s）", i+1, am.Vertex, am.A, am.B)
		} else if am.A == am.B || am.A == am.Vertex || am.B == am.Vertex {
			add("第 %d 个角标记顶点/边点重合", i+1)
		}
	}
	for i, m := range s.Ticks {
		if !exists(m.From) || !exists(m.To) {
			add("第 %d 个等长标记引用了未定义的点（%s→%s）", i+1, m.From, m.To)
		}
	}
	for i, m := range s.Parallels {
		if !exists(m.From) || !exists(m.To) {
			add("第 %d 个平行标记引用了未定义的点（%s→%s）", i+1, m.From, m.To)
		}
	}
	for i, l := range s.Labels {
		if strings.TrimSpace(l.Text) == "" {
			add("第 %d 个文字标注内容为空", i+1)
		}
	}
	if a := s.Axes; a != nil {
		// 轴端点缺失/未定义/与原点重合都可自动补齐（见 Normalize.autoAxisEnd），
		// 只有原点缺失无法修复，才作为问题回喂。
		if strings.TrimSpace(a.Origin) == "" {
			add("axes 缺少 origin（原点点名）")
		} else if !exists(a.Origin) {
			add("axes 的原点引用了未定义的点 %s", a.Origin)
		}
	}
	for i, c := range s.Curves {
		n := 0
		for _, name := range c.Through {
			if exists(name) {
				n++
			}
		}
		if n < 2 {
			add("第 %d 条曲线有效点不足 2 个", i+1)
		}
	}

	if !hasSVG && len(s.Points) == 0 && len(s.Segments) == 0 && len(s.Polygons) == 0 &&
		len(s.Circles) == 0 && len(s.Arcs) == 0 && len(s.Curves) == 0 && s.Axes == nil {
		add("没有任何可绘制的图元")
	}
	if len(issues) >= maxValidateIssues {
		issues = append(issues[:maxValidateIssues], "…其余问题省略")
	}
	return issues
}
