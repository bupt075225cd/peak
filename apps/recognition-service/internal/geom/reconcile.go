package geom

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// 渲染前的几何对齐容差（画布单位，坐标量级 100）。
const (
	// axisSnapTol 标注点离坐标轴这么近时，直接吸附到轴上。
	axisSnapTol = 1.6
	// lineAnchorTol 直线找"锚点"（它该穿过的标注点）的容差，按画布短边取比例。
	// 收紧到 0.06：过宽会让"最近标注点"选择对采样漂移过于敏感，把无字母点直线
	// （如 l₃）错误拉到 O/B/C 上、改变斜率甚至变成贴轴线段（过原点/穿错象限的来源）。
	// 锚点不足或不可信时宁可不重拟合，保持模型给的原始方向，交给修正回路兜底。
	lineAnchorTol = 0.06
	// pointSecondTol 普通标注点（非交点）找第二条约束的容差。
	pointSecondTol = 0.08
	// junctionTol 交点类标注点（同时是某条直线的端点）找第二条约束的容差。
	// 这类点本来就应该落在两条线的交叉处，可以容忍更大的初始偏差。
	junctionTol = 0.20
	// axisSecondTol 以坐标轴作为"第二条约束"时的容差上限。
	axisSecondTol = 3.0
	// exactOnLineTol 判定"确切落在某条线上"的容差。
	exactOnLineTol = 1.0
	// projectTol 单条线上的投影上限：只修 2 个单位以内的目测误差。
	projectTol = 2.5
)

// isFigureLabel 判断点名是否是图上标注的点（A、B、C…），
// 用于把标注点和模型自造的构造点（hidden 点、x1/p1 之类）区分开。
func isFigureLabel(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(name)
	return r >= 'A' && r <= 'Z'
}

// line2 是无限长的直线（两点式）。
type line2 struct{ a, b vec }

func (l line2) valid() bool { return dist(l.a, l.b) > 1e-6 }

// distToLine 点到无限长直线的垂直距离。
func distToLine(p vec, l line2) float64 {
	if !l.valid() {
		return math.Inf(1)
	}
	return math.Abs(cross(sub(l.b, l.a), sub(p, l.a))) / dist(l.a, l.b)
}

// projectOnLine 把点投影到无限长直线上。
func projectOnLine(p vec, l line2) vec {
	if !l.valid() {
		return p
	}
	d := sub(l.b, l.a)
	t := dot(sub(p, l.a), d) / dot(d, d)
	return add(l.a, mul(d, t))
}

// lineCross 求两条直线的交点；平行或退化时返回 false。
func lineCross(l1, l2 line2) (vec, bool) {
	d1 := sub(l1.b, l1.a)
	d2 := sub(l2.b, l2.a)
	den := cross(d1, d2)
	if math.Abs(den) < 1e-9 {
		return vec{}, false
	}
	t := cross(sub(l2.a, l1.a), d2) / den
	return add(l1.a, mul(d1, t)), true
}

func dot(a, b vec) float64   { return a.X*b.X + a.Y*b.Y }
func cross(a, b vec) float64 { return a.X*b.Y - a.Y*b.X }

func (s *Spec) pointIndex(name string) int {
	name = strings.TrimSpace(name)
	for i := range s.Points {
		if strings.TrimSpace(s.Points[i].Name) == name {
			return i
		}
	}
	return -1
}

func (s *Spec) pt(i int) vec { return vec{s.Points[i].X, s.Points[i].Y} }

func (s *Spec) setPt(i int, v vec) {
	s.Points[i].X, s.Points[i].Y = v.X, v.Y
}

// isLabelPoint 判断点是否是图上标注的真实点（非 hidden、非构造点）。
func (s *Spec) isLabelPoint(i int) bool {
	return i >= 0 && !s.Points[i].Hidden && isFigureLabel(s.Points[i].Name)
}

// junctionNames 收集"某条直线的端点"的点名：这类点多半是两条线的交点。
func (s *Spec) junctionNames() map[string]bool {
	out := make(map[string]bool, len(s.Segments)*2)
	for _, seg := range s.Segments {
		if !seg.Extend {
			continue
		}
		out[strings.TrimSpace(seg.From)] = true
		out[strings.TrimSpace(seg.To)] = true
	}
	return out
}

// primitive 是一个可用于对齐的"线"：坐标轴或延长线。
type primitive struct {
	line line2
	axis bool
}

// sameLine 判断两条直线是否重合（方向相同且端点距离足够近）。
func sameLine(a, b line2) bool {
	if !a.valid() || !b.valid() {
		return false
	}
	da, db := sub(a.b, a.a), sub(b.b, b.a)
	if math.Abs(cross(unit(da), unit(db))) > 0.02 {
		return false // 方向差超过约 1°
	}
	return distToLine(b.a, a) <= 0.8 && distToLine(b.b, a) <= 0.8
}

// primitives 收集当前的坐标轴与延长线（同一位置的线只保留一条：
// 模型常把坐标轴同时用 axes 和一条 extend 线段表达）。
func (s *Spec) primitives() []primitive {
	out := make([]primitive, 0, 2+len(s.Segments))
	add := func(p primitive) {
		if !p.line.valid() {
			return
		}
		for _, e := range out {
			if sameLine(e.line, p.line) {
				return
			}
		}
		out = append(out, p)
	}
	if a := s.Axes; a != nil {
		oi := s.pointIndex(a.Origin)
		if oi >= 0 {
			o := s.pt(oi)
			if xi := s.pointIndex(a.X.To); xi >= 0 {
				add(primitive{line: line2{a: o, b: s.pt(xi)}, axis: true})
			}
			if yi := s.pointIndex(a.Y.To); yi >= 0 {
				add(primitive{line: line2{a: o, b: s.pt(yi)}, axis: true})
			}
		}
	}
	for _, seg := range s.Segments {
		if !seg.Extend {
			continue
		}
		pi, pj := s.pointIndex(seg.From), s.pointIndex(seg.To)
		if pi < 0 || pj < 0 {
			continue
		}
		add(primitive{line: line2{a: s.pt(pi), b: s.pt(pj)}})
	}
	return out
}

// lineLabelTol 直线名称标签（l₁/l₂…）与它所属直线的距离容忍度，按画布短边取比例。
const lineLabelTol = 0.12

// isNamedLineLabel 判断文字是否为直线名称标签（l、l₁、l2、L 3）。
func isNamedLineLabel(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	r, size := utf8.DecodeRuneInString(t)
	if r != 'l' && r != 'L' {
		return false
	}
	rest := strings.TrimSpace(t[size:])
	if rest == "" {
		return true // 教材里也常直接写 l 表示直线
	}
	for _, c := range rest {
		if c >= '0' && c <= '9' {
			continue
		}
		if _, ok := subscriptDigits[c]; !ok {
			return false
		}
	}
	return true
}

// isPolygonEdge 判断线段是否为某个多边形的边（多边形的边是图形轮廓，不是函数图象）。
func (s *Spec) isPolygonEdge(from, to string) bool {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	for _, poly := range s.Polygons {
		for i := range poly.Points {
			a := strings.TrimSpace(poly.Points[i])
			b := strings.TrimSpace(poly.Points[(i+1)%len(poly.Points)])
			if (a == from && b == to) || (a == to && b == from) {
				return true
			}
		}
	}
	return false
}

// extendLinesNamedByLabels 让"图上标了直线名称（l₁/l₂/l₃…）的线段"真正贯穿图框。
// 模型常把正比例/一次函数图象 l₂ 只画成 O→C 两个标注点之间的一截（忘了写
// extend:true），重绘出来就成了线段，与原图贯穿坐标系的直线不符。这里以名称标签
// 为线索：标签紧贴哪条线段，那条线就是被命名的直线，应延长；标签离所有未延长
// 线段都很远时保守跳过。
func (s *Spec) extendLinesNamedByLabels() []string {
	if s.Axes == nil || len(s.Labels) == 0 {
		return nil
	}
	short := math.Min(s.Canvas.Width, s.Canvas.Height)
	if short <= 0 {
		return nil
	}
	tol := lineLabelTol * short
	var notes []string
	for _, l := range s.Labels {
		if !isNamedLineLabel(l.Text) {
			continue
		}
		lp := vec{X: l.X, Y: l.Y}
		best, bestD := -1, math.Inf(1)
		for i := range s.Segments {
			seg := s.Segments[i]
			if seg.Extend || seg.Dashed || s.isPolygonEdge(seg.From, seg.To) {
				continue
			}
			pi, pj := s.pointIndex(seg.From), s.pointIndex(seg.To)
			if pi < 0 || pj < 0 || pi == pj {
				continue
			}
			if d := pointSegDistance(lp, s.pt(pi), s.pt(pj)); d < bestD {
				best, bestD = i, d
			}
		}
		if best < 0 || bestD > tol {
			continue
		}
		s.Segments[best].Extend = true
		notes = append(notes, fmt.Sprintf("%s→%s 按图上直线名称 %s 延长为直线",
			s.Segments[best].From, s.Segments[best].To, strings.TrimSpace(l.Text)))
	}
	return notes
}

// Reconcile 在渲染前把"点—直线—坐标轴"对齐，修掉模型常见的几何不自洽：
//  0. 图上标了直线名称（l₁/l₂…）却只画成两个点之间的线段 → 延长为贯穿的直线；
//  1. 标注点几乎落在坐标轴上却没落准 → 吸附到轴上；
//  2. 延长线（segments.extend）另造了一对构造点、与图上标注点各画各的，
//     导致 C 明明该在 l₁ 与 l₂ 的交点上却"漂浮"在直线旁边 → 让直线穿过它该穿过的标注点；
//  3. 已经确切落在某条线上的标注点（尤其是直线端点/交点），若附近还有另一条线，
//     移动到两线交点（"点应该落在交叉点上"），而不是各自偏一点。
//
// 只做小幅吸附，不改变图元的拓扑关系；返回修复说明供日志排查。
func (s *Spec) Reconcile() []string {
	if s == nil || s.Canvas.Width <= 0 || s.Canvas.Height <= 0 {
		return nil
	}
	var notes []string
	notes = append(notes, s.extendLinesNamedByLabels()...)
	notes = append(notes, s.snapPointsToAxes()...)
	// 两轮足够收敛：先让直线穿过锚点，再把交点类标注点挪到交叉点上。
	for round := 0; round < 2; round++ {
		before := len(notes)
		notes = append(notes, s.anchorExtendedLines()...)
		notes = append(notes, s.snapPointsToLines()...)
		if len(notes) == before {
			break
		}
	}
	return notes
}

// snapPointsToAxes 把几乎落在坐标轴上的标注点吸附到轴上。
func (s *Spec) snapPointsToAxes() []string {
	a := s.Axes
	if a == nil {
		return nil
	}
	oi := s.pointIndex(a.Origin)
	if oi < 0 {
		return nil
	}
	origin := s.pt(oi)
	horizontal, vertical := false, false
	if xi := s.pointIndex(a.X.To); xi >= 0 {
		horizontal = math.Abs(s.Points[xi].Y-origin.Y) <= 0.5
	}
	if yi := s.pointIndex(a.Y.To); yi >= 0 {
		vertical = math.Abs(s.Points[yi].X-origin.X) <= 0.5
	}
	if !horizontal && !vertical {
		return nil
	}
	var notes []string
	for i := range s.Points {
		if !s.isLabelPoint(i) {
			continue
		}
		p := &s.Points[i]
		if horizontal {
			if d := math.Abs(p.Y - origin.Y); d > 1e-6 && d <= axisSnapTol {
				notes = append(notes, fmt.Sprintf("%s 吸附到 x 轴(%s)", p.Name, num(d)))
				p.Y = origin.Y
			}
		}
		if vertical {
			if d := math.Abs(p.X - origin.X); d > 1e-6 && d <= axisSnapTol {
				notes = append(notes, fmt.Sprintf("%s 吸附到 y 轴(%s)", p.Name, num(d)))
				p.X = origin.X
			}
		}
	}
	return notes
}

// anchorExtendedLines 让延长线真正穿过它该穿过的标注点。
// 模型常给直线另造一对构造点（只保证大致斜率），标注点则是另估的位置，
// 于是点浮在直线旁边：如 l₁ 明明过 A、C，画出来的直线却偏了 5~9 个单位。
func (s *Spec) anchorExtendedLines() []string {
	short := math.Min(s.Canvas.Width, s.Canvas.Height)
	tol := lineAnchorTol * short
	if tol <= 0 {
		return nil
	}
	var notes []string
	for i := range s.Segments {
		seg := &s.Segments[i]
		if !seg.Extend {
			continue
		}
		pi, pj := s.pointIndex(seg.From), s.pointIndex(seg.To)
		if pi < 0 || pj < 0 || pi == pj {
			continue
		}
		var anchors, free []int
		for _, idx := range []int{pi, pj} {
			if s.isLabelPoint(idx) {
				anchors = append(anchors, idx)
			} else {
				free = append(free, idx)
			}
		}
		if len(anchors) == 2 {
			// 两端都是图上标注点：模型显式用真实点定义这条线，保持不动。
			continue
		}
		cur := line2{a: s.pt(pi), b: s.pt(pj)}
		if !cur.valid() {
			continue
		}
		if len(anchors) == 1 {
			// 已有一个端点锚点，再找离这条线最近的另一个标注点。
			best, bestD := -1, tol
			for idx := range s.Points {
				if idx == anchors[0] || !s.isLabelPoint(idx) {
					continue
				}
				if d := distToLine(s.pt(idx), cur); d <= bestD {
					best, bestD = idx, d
				}
			}
			if best < 0 {
				continue
			}
			anchors = append(anchors, best)
		} else {
			type cand struct {
				idx int
				d   float64
			}
			cands := make([]cand, 0, len(s.Points))
			for idx := range s.Points {
				if !s.isLabelPoint(idx) {
					continue
				}
				if d := distToLine(s.pt(idx), cur); d <= tol {
					cands = append(cands, cand{idx: idx, d: d})
				}
			}
			if len(cands) < 2 {
				continue
			}
			sort.SliceStable(cands, func(x, y int) bool { return cands[x].d < cands[y].d })
			anchors = append(anchors, cands[0].idx, cands[1].idx)
		}
		if len(anchors) < 2 {
			continue
		}
		fitted := line2{a: s.pt(anchors[0]), b: s.pt(anchors[1])}
		if !fitted.valid() || dist(fitted.a, fitted.b) < 1.0 {
			continue
		}
		if s.duplicatesExtendedLine(i, fitted) {
			continue
		}
		// 重拟合后的直线不得与坐标轴重合（如被拉到 O→B 竖线、O→A 水平线）：
		// 这种线画出来会被轴线盖住，而且会误导结构校验报"与轴重合"、浪费修正轮。
		// 这里直接放弃拟合，保留模型给的原方向（它至少不是沿轴的退化线段）。
		if s.coincidentWithAxis(fitted) {
			continue
		}
		moved := false
		for _, idx := range free {
			np := projectOnLine(s.pt(idx), fitted)
			if dist(np, s.pt(idx)) > 1e-6 {
				s.setPt(idx, np)
				moved = true
			}
		}
		if moved {
			notes = append(notes, fmt.Sprintf("%s→%s 改为穿过 %s、%s",
				seg.From, seg.To, s.Points[anchors[0]].Name, s.Points[anchors[1]].Name))
		}
	}
	return notes
}

// duplicatesExtendedLine 判断把第 idx 条延长线改到 fitted 后是否与另一条已有的
// 延长线重合：模型给的 l₂、l₃ 常被"找锚点"拉到同一条线上，重绘后两条线叠在一起
// 只剩一条（实测三线题只画出两条）。重合时保留模型给的原始位置。
func (s *Spec) duplicatesExtendedLine(idx int, fitted line2) bool {
	for j := range s.Segments {
		if j == idx || !s.Segments[j].Extend {
			continue
		}
		pi, pj := s.pointIndex(s.Segments[j].From), s.pointIndex(s.Segments[j].To)
		if pi < 0 || pj < 0 || pi == pj {
			continue
		}
		if sameLine(line2{a: s.pt(pi), b: s.pt(pj)}, fitted) {
			return true
		}
	}
	return false
}

// coincidentWithAxis 判断一条直线是否与坐标轴重合（方向接近水平/竖直且过原点），
// 画出来会被轴线盖住。anchorExtendedLines 用它阻止把构造直线重拟合成贴轴线段。
func (s *Spec) coincidentWithAxis(l line2) bool {
	a := s.Axes
	if a == nil || !l.valid() {
		return false
	}
	oi := s.pointIndex(a.Origin)
	if oi < 0 {
		return false
	}
	o := s.pt(oi)
	// 直线须通过原点（距离足够近）才算"与轴重合"；仅平行轴但偏移的线不会被轴盖住。
	if distToLine(o, l) > 0.8 {
		return false
	}
	dx, dy := l.b.X-l.a.X, l.b.Y-l.a.Y
	length := math.Hypot(dx, dy)
	if length < 1e-6 {
		return false
	}
	// 与水平轴（x 轴）重合：方向接近水平。
	if math.Abs(dy) <= 0.02*length {
		return true
	}
	// 与竖直轴（y 轴）重合：方向接近竖直。
	if math.Abs(dx) <= 0.02*length {
		return true
	}
	return false
}

// coincidesWithOtherPoint 判断把点 except 移到 q 后是否会与其它已有点重合（距离 < 0.8）。
// snapPointsToLines 用它避免把 C 这类交点搬到 O（或其它点）上、把直线/点退化掉。
func (s *Spec) coincidesWithOtherPoint(except int, q vec) bool {
	for j := range s.Points {
		if j == except {
			continue
		}
		if dist(s.pt(j), q) < 0.8 {
			return true
		}
	}
	return false
}

// snapPointsToLines 处理"点该落在交叉点上"：对已经确切落在某条线上的标注点，
// 若附近还有另一条线，就移动到两线交点（例如 C 同时在 l₁、l₂ 上）。
func (s *Spec) snapPointsToLines() []string {
	prims := s.primitives()
	if len(prims) == 0 {
		return nil
	}
	short := math.Min(s.Canvas.Width, s.Canvas.Height)
	junctions := s.junctionNames()
	originIdx := -1
	if s.Axes != nil {
		originIdx = s.pointIndex(s.Axes.Origin)
	}
	var notes []string
	for i := range s.Points {
		if !s.isLabelPoint(i) || i == originIdx {
			continue
		}
		pv := s.pt(i)
		// 已经落在坐标轴上的标注点只允许沿轴微调：把"直线与轴的交点"搬到别的线的
		// 交点（例如把 y 轴上的 B 搬到 l₁ 与 l₂ 的交点 C），整条直线的方向就变了
		// （实测 l₁ 被拉成一条水平线）。这类点直接跳过跨线搬移。
		if s.Axes != nil && originIdx >= 0 {
			origin := s.pt(originIdx)
			if math.Abs(pv.Y-origin.Y) <= 0.5 || math.Abs(pv.X-origin.X) <= 0.5 {
				continue
			}
		}
		type hit struct {
			idx int
			d   float64
		}
		hits := make([]hit, 0, len(prims))
		for pi, pr := range prims {
			if !pr.line.valid() {
				continue
			}
			if d := distToLine(pv, pr.line); d <= projectTol || d <= junctionTol*short {
				hits = append(hits, hit{idx: pi, d: d})
			}
		}
		if len(hits) == 0 {
			continue
		}
		sort.SliceStable(hits, func(x, y int) bool { return hits[x].d < hits[y].d })
		if hits[0].d > exactOnLineTol {
			// 只是"接近"某条线：最多做一个投影，不跨线搬点。
			if hits[0].d <= projectTol {
				if q := projectOnLine(pv, prims[hits[0].idx].line); dist(pv, q) > 1e-6 {
					s.setPt(i, q)
					notes = append(notes, fmt.Sprintf("%s 投影到直线(%s)", s.Points[i].Name, num(hits[0].d)))
				}
			}
			continue
		}
		if len(hits) == 1 {
			// 只贴着一个图元：把这点误差投影掉。
			if q := projectOnLine(pv, prims[hits[0].idx].line); dist(pv, q) > 1e-6 {
				s.setPt(i, q)
				notes = append(notes, fmt.Sprintf("%s 投影到直线(%s)", s.Points[i].Name, num(hits[0].d)))
			}
			continue
		}
		secTol := pointSecondTol * short
		if junctions[strings.TrimSpace(s.Points[i].Name)] {
			secTol = junctionTol * short
		}
		moved := false
		for _, h := range hits[1:] {
			limit := secTol
			if prims[h.idx].axis {
				limit = math.Min(limit, axisSecondTol)
			}
			if h.d > limit {
				continue
			}
			q, ok := lineCross(prims[hits[0].idx].line, prims[h.idx].line)
			if !ok {
				continue // 平行（如 x 轴与它上面那条重复的线段已被去重，这里兜底）
			}
			d := dist(pv, q)
			if d <= 1e-6 {
				// 点已精确落在这两条线的交点上（如 C 同时是 l₁、l₂ 的交点），无需再移动；
				// 继续找更远的第三条线反而会把点拉离正确交点（实测 C 被拉到 l₁∩l₃）。
				break
			}
			if d > secTol*1.4 {
				continue
			}
			// 交点与其它标注点重合时放弃搬移：把 C 搬到 O（l₂ 与 l₃ 的交点被拉到原点）
			// 会让直线退化、触发"与轴重合"误报，破坏原本正确的交点。
			if s.coincidesWithOtherPoint(i, q) {
				continue
			}
			s.setPt(i, q)
			notes = append(notes, fmt.Sprintf("%s 移到两线交点(%s)", s.Points[i].Name, num(d)))
			moved = true
			break
		}
		if !moved {
			// 附近没有第二条可信的线：把这点小误差投影掉即可（不跨线移动）。
			if q := projectOnLine(pv, prims[hits[0].idx].line); dist(pv, q) > 1e-6 {
				s.setPt(i, q)
				notes = append(notes, fmt.Sprintf("%s 投影到直线(%s)", s.Points[i].Name, num(hits[0].d)))
			}
		}
	}
	return notes
}
