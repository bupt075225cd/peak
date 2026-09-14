package geom

import (
	"fmt"
	"math"
	"strings"
)

// maxValidateIssues 单次结构校验最多报告的问题条数（避免回喂文本过长）。
const maxValidateIssues = 12

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

	rightAngles := s.RightAngles[:0]
	for _, ra := range s.RightAngles {
		if exists(ra.Vertex) && exists(ra.A) && exists(ra.B) &&
			ra.A != ra.B && ra.A != ra.Vertex && ra.B != ra.Vertex {
			rightAngles = append(rightAngles, ra)
		}
	}
	s.RightAngles = rightAngles

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

// Empty 判断该图形描述是否没有任何可绘制内容。
// 仅有 svg 兜底字段（无结构化图元）不算空。
func (s *Spec) Empty() bool {
	if s == nil {
		return true
	}
	if len(s.Points) == 0 && len(s.Segments) == 0 && len(s.Polygons) == 0 &&
		len(s.Circles) == 0 && len(s.Arcs) == 0 {
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

	if !hasSVG && len(s.Points) == 0 && len(s.Segments) == 0 && len(s.Polygons) == 0 &&
		len(s.Circles) == 0 && len(s.Arcs) == 0 {
		add("没有任何可绘制的图元")
	}
	if len(issues) >= maxValidateIssues {
		issues = append(issues[:maxValidateIssues], "…其余问题省略")
	}
	return issues
}
