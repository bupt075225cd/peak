package geom

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// 函数图象"定点纠偏"：把模型画歪的直线按题干的代数约束确定性地重新定位，
// 不依赖再回喂 VLM。只处理"必须经过某个可解析定点"的直线：
//   - 正比例函数 y=mx           → 必过原点 O(0,0)
//   - y=kx+k（系数与常数项相同） → 恒过 (-1,0)，且不得过原点
//   - y=kx+b（b 为数字）        → 必过 (0,b)
//   - 题干给出坐标的交点         → 按坐标定位
//
// 与 CheckFunctionGraphPoints 的分工：本文件负责"能在程序里算出来就直接改对"，
// 改不动（信息不足、需要大范围翻修）时才留给回喂路径报问题。两者用同一套
// "是否已经满足约束"的判据，避免"改完仍被判错 → 又白回喂一次"。
//
// 安全边界：
//   - 标注点（A/B/C/O…）的坐标绝不移动；直线需要脱离标注点时，
//     为该端点克隆一个 hidden 构造点承接新位置，其余图元不受影响。
//   - 定点所需的比例（1 个数学单位对应多少画布单位）从"题干坐标点 + 图中同名点"
//     估计；估计不出时只做"保证 (-1,0) 落在 O 左侧"的最小修正。
//   - 平移幅度超过画布短边 25% 视为"不是小幅纠偏"，放弃修改。

// stemPointCoordRe 匹配题干里的点坐标："C（2，6）"、"A(-4, 0)"。
var stemPointCoordRe = regexp.MustCompile(
	`([A-Z])\s*[（(]\s*(-?\d+(?:\.\d+)?)\s*[，,]\s*(-?\d+(?:\.\d+)?)\s*[）)]`)

// stemYInterceptRe 匹配 "y=nx+7 的图象 l₁" 里的数字 y 截距与直线名。
var stemYInterceptRe = regexp.MustCompile(
	`y\s*=\s*[a-zA-Z]?\s*x\s*[+＋]\s*(\d+(?:\.\d+)?)\s*(?:的)?\s*(?:图象|图像|图)?\s*(l\s*[0-9₀-₉])`)

// fixPointKind 定点的语义：决定"是否已满足"用哪套判据。
type fixPointKind int

const (
	fixPointCoord        fixPointKind = iota // 普通定点：必须落在直线上
	fixPointOrigin                           // 正比例必过原点 O
	fixPointLeftOfOrigin                     // y=kx+k 恒过 (-1,0)：x 截距须在 O 左侧
)

// fixPoint 直线必须满足的一个定点（数学坐标）。
type fixPoint struct {
	math vec
	kind fixPointKind
}

// functionFixTarget 一条可由定点约束定位的直线。
type functionFixTarget struct {
	line    string // 归一化直线名 l3
	display string
	points  []fixPoint
}

// parseStemPointCoords 提取题干中显式给出的点坐标（数学坐标）。
func parseStemPointCoords(stemText string) map[string]vec {
	out := make(map[string]vec, 4)
	for _, m := range stemPointCoordRe.FindAllStringSubmatch(stemText, -1) {
		x, err1 := strconvFloat(m[2])
		y, err2 := strconvFloat(m[3])
		if err1 != nil || err2 != nil {
			continue
		}
		if _, dup := out[m[1]]; !dup { // 同名点保留首个
			out[m[1]] = vec{X: x, Y: y}
		}
	}
	return out
}

// parseFunctionFixTargets 汇总每条直线的定点约束（解析式定点优先，其次题干坐标点）。
func parseFunctionFixTargets(stemText string) []functionFixTarget {
	cons := parseFunctionLineConstraints(stemText)
	if len(cons) == 0 {
		return nil
	}
	coords := parseStemPointCoords(stemText)
	intercepts := parseStemYIntercepts(stemText)
	out := make([]functionFixTarget, 0, len(cons))
	for _, c := range cons {
		t := functionFixTarget{line: c.line, display: c.display}
		if c.proportional {
			t.points = appendUniqueFixPoint(t.points, fixPoint{math: vec{X: 0, Y: 0}, kind: fixPointOrigin})
		}
		if c.fixedNegativeX {
			t.points = appendUniqueFixPoint(t.points, fixPoint{math: vec{X: -1, Y: 0}, kind: fixPointLeftOfOrigin})
		}
		if b, ok := intercepts[c.line]; ok {
			t.points = appendUniqueFixPoint(t.points, fixPoint{math: vec{X: 0, Y: b}, kind: fixPointCoord})
		}
		for _, pn := range c.points {
			if p, ok := coords[pn]; ok {
				t.points = appendUniqueFixPoint(t.points, fixPoint{math: p, kind: fixPointCoord})
			}
		}
		if len(t.points) > 0 {
			out = append(out, t)
		}
	}
	return out
}

// parseStemYIntercepts 提取 "y=nx+7 的图象 l₁" 里的 b → 直线名。
func parseStemYIntercepts(stemText string) map[string]float64 {
	out := make(map[string]float64, 4)
	for _, m := range stemYInterceptRe.FindAllStringSubmatch(stemText, -1) {
		b, err := strconvFloat(m[1])
		if err != nil {
			continue
		}
		if name := normalizeLineName(m[2]); name != "" {
			out[name] = b
		}
	}
	return out
}

func appendUniqueFixPoint(list []fixPoint, items ...fixPoint) []fixPoint {
	for _, it := range items {
		dup := false
		for _, e := range list {
			if dist(e.math, it.math) < 1e-9 {
				dup = true
				break
			}
		}
		if !dup {
			list = append(list, it)
		}
	}
	return list
}

func strconvFloat(s string) (float64, error) {
	var v float64
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%g", &v)
	if err != nil || !isFinite(v) {
		return 0, fmt.Errorf("invalid number %q", s)
	}
	return v, nil
}

// estimateUnit 估计"1 个数学单位对应多少画布单位"：
// 用题干给出的坐标点与图中同名标注点的距离比，取中位数抗噪。
// 估计不出（没有同名点、或都落在原点附近）时返回 false。
func estimateUnit(s *Spec, coords map[string]vec) (float64, bool) {
	if s == nil || len(coords) == 0 || s.Axes == nil {
		return 0, false
	}
	oi := s.pointIndex(s.Axes.Origin)
	if oi < 0 {
		return 0, false
	}
	o := s.pt(oi)
	samples := make([]float64, 0, len(coords))
	for name, m := range coords {
		p, ok := s.PointByName(name)
		if !ok || p.Hidden {
			continue
		}
		dMath := math.Hypot(m.X, m.Y)
		dSVG := dist(vec{X: p.X, Y: p.Y}, o)
		if dMath < 0.5 || dSVG < 1 {
			continue
		}
		samples = append(samples, dSVG/dMath)
	}
	if len(samples) == 0 {
		return 0, false
	}
	sort.Float64s(samples)
	return samples[len(samples)/2], true
}

// mathToSVG 把数学坐标映射为画布坐标（用坐标轴方向，兼容轴不水平的图）。
func mathToSVG(s *Spec, scale float64, m vec) (vec, bool) {
	a := s.Axes
	if a == nil || scale <= 0 {
		return vec{}, false
	}
	oi, xi, yi := s.pointIndex(a.Origin), s.pointIndex(a.X.To), s.pointIndex(a.Y.To)
	if oi < 0 || xi < 0 || yi < 0 {
		return vec{}, false
	}
	o, xp, yp := s.pt(oi), s.pt(xi), s.pt(yi)
	xDir, yDir := unit(sub(xp, o)), unit(sub(yp, o))
	if (xDir.X == 0 && xDir.Y == 0) || (yDir.X == 0 && yDir.Y == 0) {
		return vec{}, false
	}
	return add(o, add(mul(xDir, m.X*scale), mul(yDir, m.Y*scale))), true
}

// interceptOffset 直线与 x 轴交点相对原点的有符号距离（沿 x 轴正方向，>0 在 O 右侧）。
func interceptOffset(s *Spec, ln line2) (float64, bool) {
	if s == nil || s.Axes == nil {
		return 0, false
	}
	axis, ok := xAxisLine(s)
	if !ok {
		return 0, false
	}
	q, ok := lineCross(ln, axis)
	if !ok {
		return 0, false
	}
	o, ok := s.PointByName(s.Axes.Origin)
	if !ok {
		return 0, false
	}
	dir := unit(sub(axis.b, axis.a))
	if dir.X == 0 && dir.Y == 0 {
		return 0, false
	}
	return dot(sub(q, vec{X: o.X, Y: o.Y}), dir), true
}

// fixedPointToSVG 把"必须经过的定点（数学坐标）"换算成画布坐标。
// 原点不需要比例尺（直接取图上的原点），其余定点都需要 scale；
// 没有 scale 时只对 y=kx+k 的 (-1,0) 退化为"O 左侧一小段"。
func fixedPointToSVG(s *Spec, scale float64, hasScale bool, m vec, short float64) (vec, bool) {
	if s == nil || s.Axes == nil {
		return vec{}, false
	}
	if math.Abs(m.X) < 1e-9 && math.Abs(m.Y) < 1e-9 {
		o, ok := s.PointByName(s.Axes.Origin)
		if !ok {
			return vec{}, false
		}
		return vec{X: o.X, Y: o.Y}, true
	}
	if hasScale {
		return mathToSVG(s, scale, m)
	}
	if math.Abs(m.X+1) > 1e-9 || math.Abs(m.Y) > 1e-9 {
		return vec{}, false
	}
	// 无比例尺：只能保证"与 x 轴交于 O 的左侧"，避免出现"恒过 (-1,0) 却过原点"。
	o, ok := s.PointByName(s.Axes.Origin)
	if !ok {
		return vec{}, false
	}
	axis, ok := xAxisLine(s)
	if !ok {
		return vec{}, false
	}
	dir := unit(sub(axis.b, axis.a))
	if dir.X == 0 && dir.Y == 0 {
		return vec{}, false
	}
	return sub(vec{X: o.X, Y: o.Y}, mul(dir, 0.06*short)), true
}

// translateLineTo 保持斜率，把直线整体平移到经过 p。
func translateLineTo(l line2, p vec) line2 {
	if !l.valid() {
		return l
	}
	d := sub(p, projectOnLine(p, l))
	return line2{a: add(l.a, d), b: add(l.b, d)}
}

// lineThroughPoints 用前两个足够分离的点确定直线（两点定线 = 数学上唯一解）。
func lineThroughPoints(pts []vec, minSep float64) (line2, bool) {
	for i := 0; i < len(pts); i++ {
		for j := i + 1; j < len(pts); j++ {
			if dist(pts[i], pts[j]) > minSep {
				ln := line2{a: pts[i], b: pts[j]}
				return ln, ln.valid()
			}
		}
	}
	return line2{}, false
}

// maxFixSlopeSin 允许的最大斜率改变量（单位向量叉积 ≈ sin 夹角）：约 20°。
// 超过这个量说明不是"画歪了一点"，而是题干数据或图形结构有问题，交给回喂路径。
const maxFixSlopeSin = 0.34

// slopeChangedMuch 判断把 cur 校正成 target 是否会大幅改变斜率。
func slopeChangedMuch(cur, target line2) bool {
	if !cur.valid() || !target.valid() {
		return true
	}
	d1, d2 := unit(sub(cur.b, cur.a)), unit(sub(target.b, target.a))
	return math.Abs(cross(d1, d2)) > maxFixSlopeSin
}

// segmentLine 取线段所在直线。
func (s *Spec) segmentLine(i int) (line2, bool) {
	if i < 0 || i >= len(s.Segments) {
		return line2{}, false
	}
	pi, pj := s.pointIndex(s.Segments[i].From), s.pointIndex(s.Segments[i].To)
	if pi < 0 || pj < 0 || pi == pj {
		return line2{}, false
	}
	ln := line2{a: s.pt(pi), b: s.pt(pj)}
	return ln, ln.valid()
}

// extendedSegmentByLabelName 用图上印刷的直线名称标签找到对应线段下标。
func (s *Spec) extendedSegmentByLabelName(name string) (int, bool) {
	want := normalizeLineName(name)
	if want == "" || len(s.Labels) == 0 {
		return -1, false
	}
	var lp vec
	found := false
	for _, l := range s.Labels {
		if normalizeLineName(l.Text) == want {
			lp = vec{X: l.X, Y: l.Y}
			found = true
			break
		}
	}
	if !found {
		return -1, false
	}
	short := math.Min(s.Canvas.Width, s.Canvas.Height)
	if short <= 0 {
		return -1, false
	}
	best, bestD := -1, math.Inf(1)
	for i := range s.Segments {
		if !s.Segments[i].Extend {
			continue
		}
		ln, ok := s.segmentLine(i)
		if !ok {
			continue
		}
		if d := distToLine(lp, ln); d < bestD {
			best, bestD = i, d
		}
	}
	if best < 0 || bestD > lineLabelTol*short {
		return -1, false
	}
	return best, true
}

// pointUseCount 统计某点被图元引用的次数（跳过 skipSeg 这条线段自身的引用）。
// 用于判断"移动这个构造点会不会连带改动别的图元"。
func pointUseCount(s *Spec, name string, skipSeg int) int {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0
	}
	hit := func(pairs ...string) int {
		for _, p := range pairs {
			if strings.TrimSpace(p) == name {
				return 1
			}
		}
		return 0
	}
	n := 0
	for i := range s.Segments {
		if i == skipSeg {
			continue
		}
		n += hit(s.Segments[i].From, s.Segments[i].To)
	}
	for _, c := range s.Curves {
		n += hit(c.Through...)
	}
	for _, p := range s.Polygons {
		n += hit(p.Points...)
	}
	if a := s.Axes; a != nil {
		n += hit(a.Origin, a.X.To, a.Y.To)
	}
	for _, c := range s.Circles {
		n += hit(c.Center, c.Through)
	}
	for _, a := range s.Arcs {
		n += hit(a.Center)
	}
	for _, r := range s.RightAngles {
		n += hit(r.Vertex, r.A, r.B)
	}
	for _, m := range s.AngleMarks {
		n += hit(m.Vertex, m.A, m.B)
	}
	for _, t := range s.Ticks {
		n += hit(t.From, t.To)
	}
	for _, p := range s.Parallels {
		n += hit(p.From, p.To)
	}
	return n
}

// addHiddenPoint 追加一个只用于作图的隐藏构造点，返回唯一名字。
// 名称以下划线开头，避免被当成图上标注的点（isFigureLabel 只认 A-Z）。
func (s *Spec) addHiddenPoint(base string, p vec) string {
	name := "__" + base
	for i := 1; s.pointIndex(name) >= 0; i++ {
		name = fmt.Sprintf("__%s%d", base, i)
	}
	s.Points = append(s.Points, Point{Name: name, X: p.X, Y: p.Y, Hidden: true})
	return name
}

// applyLineToSegment 把线段的两端挪到目标直线上（只做垂直投影，斜率不变）。
// 端点若为标注点、或被别的图元共用，则克隆一个隐藏构造点承接新位置，
// 保证 A/B/C/O 这类点的坐标与其它图形元素一律不受影响。
func applyLineToSegment(s *Spec, segIdx int, ln line2, maxMove float64) bool {
	if segIdx < 0 || segIdx >= len(s.Segments) || !ln.valid() {
		return false
	}
	names := [2]string{s.Segments[segIdx].From, s.Segments[segIdx].To}
	type plan struct {
		idx   int
		pos   vec
		clone bool
	}
	plans := make([]plan, 0, 2)
	for _, name := range names {
		idx := s.pointIndex(name)
		if idx < 0 {
			return false
		}
		p := s.pt(idx)
		np := projectOnLine(p, ln)
		d := dist(np, p)
		if d > maxMove {
			return false // 幅度过大：宁可不改，交回回喂路径
		}
		clone := false
		if d > 1e-9 { // 只有真要动这个点时才考虑"是不是不能动"
			clone = s.isLabelPoint(idx) || pointUseCount(s, name, segIdx) > 0
		}
		plans = append(plans, plan{idx: idx, pos: np, clone: clone})
	}
	newNames := [2]string{names[0], names[1]}
	for k, pl := range plans {
		if dist(pl.pos, s.pt(pl.idx)) <= 1e-9 {
			continue // 已经在目标直线上
		}
		if pl.clone {
			newNames[k] = s.addHiddenPoint(names[k], pl.pos)
			continue
		}
		s.setPt(pl.idx, pl.pos)
	}
	if newNames[0] == newNames[1] {
		return false
	}
	s.Segments[segIdx].From, s.Segments[segIdx].To = newNames[0], newNames[1]
	return true
}

// followLineLabel 让直线名称标签跟着直线做同样的垂直位移。
// 标签本来就不在这条线附近（可能属于别的线或误判）时不动。
func followLineLabel(s *Spec, name string, from, to line2, tol float64) {
	want := normalizeLineName(name)
	if want == "" {
		return
	}
	for i := range s.Labels {
		if normalizeLineName(s.Labels[i].Text) != want {
			continue
		}
		lp := vec{X: s.Labels[i].X, Y: s.Labels[i].Y}
		if !from.valid() || !to.valid() || distToLine(lp, from) > tol {
			return
		}
		np := add(lp, sub(projectOnLine(lp, to), projectOnLine(lp, from)))
		s.Labels[i].X, s.Labels[i].Y = np.X, np.Y
		return
	}
}

// requirement 一条待校验的定点要求（已换算到画布坐标）。
type requirement struct {
	pos  vec
	kind fixPointKind
}

// requirementSatisfied 判断直线是否已满足某个定点要求，判据与 CheckFunctionGraphPoints 一致。
func requirementSatisfied(s *Spec, ln line2, r requirement, tol float64) bool {
	if r.kind == fixPointLeftOfOrigin {
		if t, ok := interceptOffset(s, ln); ok {
			return t <= -functionOriginTolRatio*tol // 交点必须在 O 左侧（含安全余量）
		}
	}
	return distToLine(r.pos, ln) <= tol
}

// fixOneLine 对一条直线做定点纠偏；返回修正说明。
func fixOneLine(spec *Spec, t functionFixTarget, scale float64, hasScale bool, short float64) string {
	segIdx, ok := spec.extendedSegmentByLabelName(t.line)
	if !ok {
		return ""
	}
	cur, ok := spec.segmentLine(segIdx)
	if !ok {
		return ""
	}
	tol := functionPointTolRatio * short
	reqs := make([]requirement, 0, len(t.points))
	desc := make([]string, 0, len(t.points))
	for _, fp := range t.points {
		p, ok := fixedPointToSVG(spec, scale, hasScale, fp.math, short)
		if !ok {
			continue
		}
		reqs = append(reqs, requirement{pos: p, kind: fp.kind})
		desc = append(desc, fmt.Sprintf("(%s,%s)", num(fp.math.X), num(fp.math.Y)))
	}
	if len(reqs) == 0 {
		return ""
	}
	satisfied := true
	for _, r := range reqs {
		if !requirementSatisfied(spec, cur, r, tol) {
			satisfied = false
			break
		}
	}
	if satisfied {
		return "" // 已经满足全部定点约束，不动
	}
	pts := make([]vec, 0, len(reqs))
	for _, r := range reqs {
		pts = append(pts, r.pos)
	}
	target := translateLineTo(cur, reqs[0].pos)
	if ln, ok := lineThroughPoints(pts, tol); ok {
		consistent := true
		for _, p := range pts {
			if distToLine(p, ln) > 1.5*tol {
				consistent = false // 题干给的定点互相矛盾（多半是转录有误）：退回单点平移
				break
			}
		}
		if consistent {
			if slopeChangedMuch(cur, ln) {
				return "" // 斜率要动太多：多半是题干坐标转录有误，不盲目重排
			}
			target = ln
		}
	}
	if !applyLineToSegment(spec, segIdx, target, 0.25*short) {
		return ""
	}
	followLineLabel(spec, t.line, cur, target, 0.4*short)
	return fmt.Sprintf("%s 校正为过定点%s", t.display, strings.Join(desc, ""))
}

// FixFunctionGraphs 对"偏离题干定点约束"的函数图象做确定性纠偏（不依赖模型回喂）。
// 返回修正说明（供日志排查）；没有可修正项时返回 nil。
//
// 处理范围：正比例必过 O、y=kx+k 恒过 (-1,0)、y=kx+b 过 (0,b)、题干坐标交点。
// 比例（scale）估计失败时，对 y=kx+k 退化为"把 x 截距移到 O 左侧一小段"的最小修正，
// 至少保证不出现"恒过 (-1,0) 的直线却穿过原点"这种与解析式矛盾的结果。
func FixFunctionGraphs(panels []Panel, stemText string) []string {
	targets := parseFunctionFixTargets(stemText)
	if len(targets) == 0 {
		return nil
	}
	coords := parseStemPointCoords(stemText)
	var notes []string
	for i := range panels {
		spec := &panels[i].Spec
		if spec.Axes == nil {
			continue
		}
		short := math.Min(spec.Canvas.Width, spec.Canvas.Height)
		if short <= 0 {
			continue
		}
		scale, hasScale := estimateUnit(spec, coords)
		for _, t := range targets {
			if note := fixOneLine(spec, t, scale, hasScale, short); note != "" {
				notes = append(notes, note)
			}
		}
	}
	return notes
}
