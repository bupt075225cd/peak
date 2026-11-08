package geom

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// 统一配色与字体（线段/文字用近黑色，保证打印与深浅底都清晰）。
const (
	svgNS            = "http://www.w3.org/2000/svg"
	fontStack        = `"Noto Sans SC","PingFang SC","Microsoft YaHei",Arial,sans-serif`
	colorLine        = "#111827"
	colorMark        = "#b45309"
	colorAccent      = "#2563eb"
	colorPoint       = "#111827"
	colorText        = "#0f172a"
	colorWhite       = "#ffffff"
	colorGrid        = "#c8d2e0" // 网格线：比图形线浅得多，不能抢主体
	dashArray        = "3 2.4"
	gridDashArray    = "1.6 1.6"
	defaultLabelSize = 3.6
	// 网格行列上限：防止模型给出离谱的行列数把 SVG 撑爆。
	maxGridCols = 40
	maxGridRows = 40
)

var (
	textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	attrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
)

// greekAngleRe 匹配"纯希腊字母角名"，如 α、(β)、【γ】。
var greekAngleRe = regexp.MustCompile(`^[（(\[【]?\s*[α-ωΑ-Ω]+\s*[)）\]】]?$`)

// isAngleText 判断是否为"角度类"文字（度数、∠ 表达式、纯希腊字母角名）。
// 按产品要求重绘时不在图上标注任何角度信息，这类文字一律跳过。
func isAngleText(t string) bool {
	s := strings.TrimSpace(t)
	if s == "" {
		return true
	}
	if strings.Contains(s, "°") || strings.Contains(s, "∠") {
		return true
	}
	return greekAngleRe.MatchString(s)
}

// stripInvalidXML 移除 XML 1.0 不允许的控制字符。
func stripInvalidXML(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return r
		}
		if r < 0x20 || r == 0xFFFE || r == 0xFFFF {
			return -1
		}
		return r
	}, s)
}

func escapeText(s string) string { return textEscaper.Replace(stripInvalidXML(s)) }

func escapeAttr(s string) string { return attrEscaper.Replace(stripInvalidXML(s)) }

// svgAttr 是一个有序 SVG 属性（用切片而非 map，保证输出顺序稳定）。
type svgAttr struct{ k, v string }

func attr(k, v string) svgAttr { return svgAttr{k: k, v: v} }

// writeAttrs 输出属性列表；值为空的属性被跳过（与参考实现的 mk 行为一致，
// 例如未指定 dashed 时不输出 stroke-dasharray）。
func writeAttrs(b *strings.Builder, attrs []svgAttr) {
	for _, a := range attrs {
		if a.v == "" {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(a.k)
		b.WriteString(`="`)
		b.WriteString(escapeAttr(a.v))
		b.WriteByte('"')
	}
}

// writeVoid 输出一个自闭合元素。
func writeVoid(b *strings.Builder, name string, attrs ...svgAttr) {
	b.WriteByte('<')
	b.WriteString(name)
	writeAttrs(b, attrs)
	b.WriteString("/>")
}

// dashAttr 虚线属性值。
func dashAttr(dashed bool) string {
	if dashed {
		return dashArray
	}
	return ""
}

// subscriptRe 匹配文字中的 Unicode 下标串（如 l₁ 的 "₁"）。
var subscriptRe = regexp.MustCompile(`[₀-₉]+`)

// writeTextContent 输出转义后的文字内容，并把 Unicode 下标字符改写为
// <tspan> 下标（普通数字 + 下沉小号字）。部分字体/栅格化器缺下标字形，
// 会把 "l₁" 渲染成只剩 "l"，改用 tspan 后任何渲染器都能完整显示。
func writeTextContent(b *strings.Builder, content string) {
	last := 0
	sub := false
	for _, loc := range subscriptRe.FindAllStringIndex(content, -1) {
		if loc[0] > last {
			if sub {
				// 上一段是下标，后续正文需要把基线抬回来。
				b.WriteString(`<tspan dy="-1">`)
				b.WriteString(escapeText(content[last:loc[0]]))
				b.WriteString(`</tspan>`)
			} else {
				b.WriteString(escapeText(content[last:loc[0]]))
			}
		}
		digits := make([]rune, 0, loc[1]-loc[0])
		for _, r := range content[loc[0]:loc[1]] {
			digits = append(digits, r-'₀'+'0')
		}
		b.WriteString(`<tspan dy="1" font-size="70%">`)
		b.WriteString(escapeText(string(digits)))
		b.WriteString(`</tspan>`)
		sub = true
		last = loc[1]
	}
	if last < len(content) {
		if sub {
			b.WriteString(`<tspan dy="-1">`)
			b.WriteString(escapeText(content[last:]))
			b.WriteString(`</tspan>`)
		} else {
			b.WriteString(escapeText(content[last:]))
		}
	}
}

// writeTextEl 输出文字元素：无描边底色，纯色文字（配合白色画布底清晰可读）。
func writeTextEl(b *strings.Builder, pos vec, content string, size float64, weight int, anchor string) {
	if anchor == "" {
		anchor = "middle"
	}
	b.WriteString("<text")
	writeAttrs(b, []svgAttr{
		attr("x", num(pos.X)),
		attr("y", num(pos.Y)),
		attr("font-size", num(size)),
		attr("font-family", fontStack),
		attr("font-weight", strconv.Itoa(weight)),
		attr("text-anchor", anchor),
		attr("dominant-baseline", "middle"),
		attr("fill", colorText),
	})
	b.WriteByte('>')
	writeTextContent(b, content)
	b.WriteString("</text>")
}

// Render 把单张子图渲染为独立 SVG 文档（内部完成文字避让）。
// 当描述只有 svg 兜底字段时，直接返回安全清洗后的该 SVG。
func Render(s *Spec) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("geom: nil spec")
	}
	if s.Empty() {
		return nil, fmt.Errorf("geom: nothing to draw")
	}
	structured := len(s.Points) > 0 || len(s.Segments) > 0 || len(s.Polygons) > 0 ||
		len(s.Circles) > 0 || len(s.Arcs) > 0 || len(s.Curves) > 0 || s.Axes != nil
	if !structured {
		out := SanitizeSVG(s.SVG)
		if out == "" {
			return nil, fmt.Errorf("geom: invalid svg fallback")
		}
		return []byte(out), nil
	}
	return []byte(drawStructured(s)), nil
}

// drawStructured 按图元类型依次绘制（与参考实现 drawGeometry 的 z 序一致），
// 文字统一放到最后做避让布局。
func drawStructured(s *Spec) string {
	w, h := s.Canvas.Width, s.Canvas.Height
	if w <= 0 || !isFinite(w) {
		w = DefaultCanvasSize
	}
	if h <= 0 || !isFinite(h) {
		h = DefaultCanvasSize
	}

	pts := make(map[string]Point, len(s.Points))
	for _, p := range s.Points {
		pts[p.Name] = p
	}
	at := func(name string) (Point, bool) {
		p, ok := pts[strings.TrimSpace(name)]
		return p, ok
	}

	layout := newTextLayout(w, h)
	pending := make([]textItem, 0, len(s.Points)+len(s.Labels))
	var b strings.Builder
	// 显式 width/height：作为 <img> 加载时浏览器可据此得到固有尺寸，缩略图才能正常显示。
	b.WriteString(`<svg xmlns="` + svgNS + `" viewBox="0 0 ` + num(w) + ` ` + num(h) +
		`" width="` + num(w) + `" height="` + num(h) + `" preserveAspectRatio="xMidYMid meet">`)
	b.WriteByte('\n')
	// 白色纸面底：在深色底（如放大查看器遮罩）上以"纸面"呈现，黑色线与文字清晰可读。
	writeVoid(&b, "rect",
		attr("x", "0"), attr("y", "0"),
		attr("width", num(w)), attr("height", num(h)),
		attr("fill", colorWhite))

	// 0) 网格背景（"在如图所示的网格中"这类题）：浅灰细线，画在所有图元之下。
	// 网格不登记为文字避让障碍——格子太密，登记后点标签会被挤到格外。
	drawGrid(&b, s)

	// 0) 坐标系与函数曲线（z 序最底：轴画在图形下面，避免压住图形边线）。
	// 轴端点是构造点：仅当它没有被图形（线段/圆/多边形等）引用时才隐去点与点名，
	// 否则会出现"题干里的点 A/C/D 整点消失、只剩轴上的数字"这种错误。
	axisOnly := make(map[string]bool, 4)
	if s.Axes != nil {
		used := figurePointNames(s)
		for _, name := range []string{s.Axes.X.To, s.Axes.Y.To} {
			if name != "" && !used[name] {
				axisOnly[name] = true
			}
		}
	}
	drawAxes(&b, s, at, layout, w, h)
	drawCurves(&b, s, at, layout)

	// 1) 多边形（可填充）。
	for _, poly := range s.Polygons {
		nodes := make([]Point, 0, len(poly.Points))
		for _, name := range poly.Points {
			if p, ok := at(name); ok {
				nodes = append(nodes, p)
			}
		}
		if len(nodes) < 3 {
			continue
		}
		coords := make([]string, 0, len(nodes))
		vs := make([]vec, 0, len(nodes))
		for _, p := range nodes {
			coords = append(coords, num(p.X)+","+num(p.Y))
			vs = append(vs, vec{X: p.X, Y: p.Y})
		}
		fill := "none"
		attrs := []svgAttr{
			attr("points", strings.Join(coords, " ")),
			attr("fill", fill),
			attr("stroke", colorLine),
			attr("stroke-width", "0.9"),
			attr("stroke-linejoin", "round"),
			attr("stroke-dasharray", dashAttr(poly.Dashed)),
		}
		if poly.Fill {
			// 半透明填充拆成 rgb + fill-opacity：oksvg 的 ParseSVGColor
			// 不支持 rgba()，且该解析错误不受 IgnoreErrorMode 兜底，
			// 会导致导出时光栅化整图失败。
			attrs = append(attrs, attr("fill", "rgb(37,99,235)"), attr("fill-opacity", "0.10"))
		}
		writeVoid(&b, "polygon", attrs...)
		layout.addPolygon(vs)
	}

	// 2) 圆。
	for _, c := range s.Circles {
		o, ok := at(c.Center)
		if !ok {
			continue
		}
		o2 := vec{X: o.X, Y: o.Y}
		r := c.Radius
		if r <= 0 && c.Through != "" {
			if t, ok2 := at(c.Through); ok2 {
				r = dist(o2, vec{X: t.X, Y: t.Y})
			}
		}
		if r <= 0 {
			continue
		}
		writeVoid(&b, "circle",
			attr("cx", num(o2.X)), attr("cy", num(o2.Y)), attr("r", num(r)),
			attr("fill", "none"),
			attr("stroke", colorLine),
			attr("stroke-width", "0.9"),
			attr("stroke-dasharray", dashAttr(c.Dashed)),
		)
		layout.addCircle(o2, r)
	}

	// 3) 圆弧。
	for _, a := range s.Arcs {
		o, ok := at(a.Center)
		if !ok || a.Radius <= 0 {
			continue
		}
		o2 := vec{X: o.X, Y: o.Y}
		start := a.StartAngle
		delta := math.Mod(a.EndAngle-start, 360)
		if delta < 0 {
			delta += 360
		}
		if delta == 0 {
			delta = 360
		}
		p1 := polar(o2, a.Radius, start)
		p2 := polar(o2, a.Radius, start+delta)
		large := 0
		if delta > 180 {
			large = 1
		}
		writeVoid(&b, "path",
			attr("d", fmt.Sprintf("M %s %s A %s %s 0 %d 1 %s %s",
				num(p1.X), num(p1.Y), num(a.Radius), num(a.Radius), large, num(p2.X), num(p2.Y))),
			attr("fill", "none"),
			attr("stroke", colorLine),
			attr("stroke-width", "0.9"),
			attr("stroke-dasharray", dashAttr(a.Dashed)),
		)
		// 圆弧按采样折线登记，避免文字压在弧上。
		steps := clampIntRange(math.Abs(delta)/12, 6, 1000)
		arcPts := make([]vec, 0, steps+1)
		for i := 0; i <= steps; i++ {
			arcPts = append(arcPts, polar(o2, a.Radius, start+delta*float64(i)/float64(steps)))
		}
		layout.addPolyline(arcPts)
	}

	// 4) 线段（extend 时向两端延长，用于表达直线/射线）。
	labeledSegs := s.labeledExtendSegments()
	for i, seg := range s.Segments {
		pa, okA := at(seg.From)
		pb, okB := at(seg.To)
		if !okA || !okB {
			continue
		}
		p1 := vec{X: pa.X, Y: pa.Y}
		p2 := vec{X: pb.X, Y: pb.Y}
		// 两端都是 hidden 构造点的线段，模型只是在用两个定位点表达"一条线"，
		// 若忘了写 extend 就会画成漂浮在图中间的一小截，这里按直线延长。
		if seg.Extend || (pa.Hidden && pb.Hidden && !seg.Dashed) {
			// 与坐标轴完全重合、又没挂直线名称标签的延长线段：延长到画布边缘后
			// 会盖在轴线上、一直画过轴箭头（视觉上"轴线超长"）。它本来就被轴线
			// 盖住等于没画，检查器（CheckAxisCoincidentSegments）已提示模型修正，
			// 这里不重复画。挂着名称标签的照常绘制——真实函数图象也可能与轴共线。
			if s.axisCoincidentSegment(seg) && !labeledSegs[i] {
				continue
			}
			u := unit(sub(p2, p1))
			if u.X != 0 || u.Y != 0 {
				// 直线要贯穿图框：原图里的函数图象都是从一侧穿到另一侧，
				// 按固定 22% 延长会画成"比线段长一点的线段"，缺少延伸感。
				p1 = rayToCanvasEdge(p1, mul(u, -1), w, h)
				p2 = rayToCanvasEdge(p2, u, w, h)
			}
		}
		writeVoid(&b, "line",
			attr("x1", num(p1.X)), attr("y1", num(p1.Y)),
			attr("x2", num(p2.X)), attr("y2", num(p2.Y)),
			attr("stroke", colorLine),
			attr("stroke-width", "0.95"),
			attr("stroke-linecap", "round"),
			attr("stroke-dasharray", dashAttr(seg.Dashed)),
		)
		layout.addSegment(p1, p2)
	}

	// 5) 等长标记。
	for _, t := range s.Ticks {
		pa, okA := at(t.From)
		pb, okB := at(t.To)
		if !okA || !okB {
			continue
		}
		a1 := vec{X: pa.X, Y: pa.Y}
		b1 := vec{X: pb.X, Y: pb.Y}
		m := mid(a1, b1)
		u := unit(sub(b1, a1))
		nrm := vec{X: -u.Y, Y: u.X}
		count := clampCount(t.Count)
		for i := 0; i < count; i++ {
			c := add(m, mul(u, (float64(i)-float64(count-1)/2)*1.15))
			q1 := add(c, mul(nrm, 1.35))
			q2 := add(c, mul(nrm, -1.35))
			writeVoid(&b, "line",
				attr("x1", num(q1.X)), attr("y1", num(q1.Y)),
				attr("x2", num(q2.X)), attr("y2", num(q2.Y)),
				attr("stroke", colorMark),
				attr("stroke-width", "0.85"),
				attr("stroke-linecap", "round"),
			)
		}
	}

	// 6) 平行标记（"V"形箭头）。
	for _, t := range s.Parallels {
		pa, okA := at(t.From)
		pb, okB := at(t.To)
		if !okA || !okB {
			continue
		}
		a1 := vec{X: pa.X, Y: pa.Y}
		b1 := vec{X: pb.X, Y: pb.Y}
		m := mid(a1, b1)
		u := unit(sub(b1, a1))
		nrm := vec{X: -u.Y, Y: u.X}
		count := clampCount(t.Count)
		for i := 0; i < count; i++ {
			c := add(m, mul(u, (float64(i)-float64(count-1)/2)*2.2))
			tip := add(c, mul(u, 1.3))
			w1 := add(c, add(mul(u, -0.9), mul(nrm, 1.25)))
			w2 := add(c, add(mul(u, -0.9), mul(nrm, -1.25)))
			writeVoid(&b, "path",
				attr("d", fmt.Sprintf("M %s %s L %s %s L %s %s",
					num(w1.X), num(w1.Y), num(tip.X), num(tip.Y), num(w2.X), num(w2.Y))),
				attr("fill", "none"),
				attr("stroke", colorMark),
				attr("stroke-width", "0.85"),
				attr("stroke-linejoin", "round"),
				attr("stroke-linecap", "round"),
			)
		}
	}

	// 7) 直角标记。
	for _, ra := range s.RightAngles {
		v, okV := at(ra.Vertex)
		pa, okA := at(ra.A)
		pb, okB := at(ra.B)
		if !okV || !okA || !okB {
			continue
		}
		vo := vec{X: v.X, Y: v.Y}
		ao := vec{X: pa.X, Y: pa.Y}
		bo := vec{X: pb.X, Y: pb.Y}
		ua := unit(sub(ao, vo))
		ub := unit(sub(bo, vo))
		la := dist(vo, ao)
		lb := dist(vo, bo)
		size := ra.Size
		if size <= 0 || !isFinite(size) {
			size = math.Min(3.2, math.Max(1.6, math.Min(la, lb)*0.2))
		}
		p1 := add(vo, mul(ua, size))
		p2 := add(add(vo, mul(ua, size)), mul(ub, size))
		p3 := add(vo, mul(ub, size))
		writeVoid(&b, "path",
			attr("d", fmt.Sprintf("M %s %s L %s %s L %s %s",
				num(p1.X), num(p1.Y), num(p2.X), num(p2.Y), num(p3.X), num(p3.Y))),
			attr("fill", "none"),
			attr("stroke", colorAccent),
			attr("stroke-width", "0.85"),
			attr("stroke-linejoin", "round"),
		)
	}

	// 8) 角标记（只画弧线，不写度数/角名）。
	// 忠实原图：仅当 VLM 判定原图确实画有弧线标记时 spec 才会含 AngleMarks
	// （提示词明确"原图没有就不要输出"），渲染器按 spec 如实绘制。
	for _, am := range s.AngleMarks {
		v, okV := at(am.Vertex)
		pa, okA := at(am.A)
		pb, okB := at(am.B)
		if !okV || !okA || !okB {
			continue
		}
		vo := vec{X: v.X, Y: v.Y}
		ao := vec{X: pa.X, Y: pa.Y}
		bo := vec{X: pb.X, Y: pb.Y}
		la := dist(vo, ao)
		lb := dist(vo, bo)
		r := am.Radius
		if r <= 0 || !isFinite(r) {
			r = math.Min(la, lb) * 0.26
		}
		r = math.Min(math.Max(r, 2.6), 14)

		a0 := deg(math.Atan2(ao.Y-vo.Y, ao.X-vo.X))
		a1 := deg(math.Atan2(bo.Y-vo.Y, bo.X-vo.X))
		delta := math.Mod(a1-a0+540, 360) - 180 // 取小于 180° 的一侧

		count := clampCount(am.Count)
		for i := 0; i < count; i++ {
			rr := r - float64(i)*1.3
			if rr <= 0.8 {
				break
			}
			p1 := polar(vo, rr, a0)
			p2 := polar(vo, rr, a0+delta)
			large := 0
			if math.Abs(delta) > 180 {
				large = 1
			}
			sweep := 0
			if delta > 0 {
				sweep = 1
			}
			writeVoid(&b, "path",
				attr("d", fmt.Sprintf("M %s %s A %s %s 0 %d %d %s %s",
					num(p1.X), num(p1.Y), num(rr), num(rr), large, sweep, num(p2.X), num(p2.Y))),
				attr("fill", "none"),
				attr("stroke", colorAccent),
				attr("stroke-width", "0.8"),
				attr("stroke-linecap", "round"),
			)
		}
	}

	// 9) 点与点标签。
	for _, p := range s.Points {
		pv := vec{X: p.X, Y: p.Y}
		if !p.Hidden && !axisOnly[p.Name] {
			writeVoid(&b, "circle",
				attr("cx", num(pv.X)), attr("cy", num(pv.Y)),
				attr("r", "0.85"), attr("fill", colorPoint))
			layout.addDot(pv.X, pv.Y, 0.85)
		}
		// 构造点（hidden:true）只参与作图，默认不标注点名：
		// 否则模型自造的轴辅助点（如 x1/y1）会被当成点标签画到图上。
		// 坐标轴端点同理，只有显式给了 label 才标注。
		label := strings.TrimSpace(p.Label)
		if label == "" {
			if p.Hidden || axisOnly[p.Name] {
				continue
			}
			label = p.Name
		}
		var off vec
		if p.LabelDX != nil || p.LabelDY != nil {
			if p.LabelDX != nil {
				off.X = *p.LabelDX
			}
			if p.LabelDY != nil {
				off.Y = *p.LabelDY
			}
		}
		// 点标签从点本身出发搜索落点（不再预先按"远离图形中心"偏移一格）：
		// 内部点（如 CD 与 AG 的交点 G）按中心方向会被推向拥挤的一侧，
		// 最终被挤到坐标轴附近，看上去像"标错了点"。交给避让算法按八个方向就近择优。
		pending = append(pending, textItem{
			Pos: add(pv, off), Text: label, Size: 3.9, Weight: 700, Anchor: "middle", Priority: 0,
			MoveWeight: textCostMove * 5, NearWeight: textCostNear * 0.22,
		})
	}

	// 10) 自由标注（角度类文字一律跳过）。
	for _, l := range s.Labels {
		if isAngleText(l.Text) {
			continue
		}
		size := l.Size
		if size <= 0 || !isFinite(size) {
			size = defaultLabelSize
		}
		pending = append(pending, textItem{
			Pos:      vec{X: l.X, Y: l.Y},
			Text:     l.Text,
			Size:     size,
			Weight:   500,
			Anchor:   l.Anchor,
			Priority: 2,
		})
	}

	// 11) 统一布局并绘制：点标签优先，逐个避开线、圆与彼此。
	sort.SliceStable(pending, func(i, j int) bool { return pending[i].Priority < pending[j].Priority })
	for _, item := range pending {
		pos := layout.place(item)
		writeTextEl(&b, pos, item.Text, item.Size, item.Weight, item.Anchor)
	}

	b.WriteString("</svg>\n")
	return b.String()
}

// figurePointNames 收集被图形要素引用的点（线段/多边形/圆/弧/各类标记/曲线/坐标系原点）。
// 用于区分"纯轴构造点"与"真正的图形点"：真正的图形点即使被写进 axes 当端点，
// 也必须照常画点与点名，不能因为"轴点"就被隐去。
func figurePointNames(s *Spec) map[string]bool {
	used := make(map[string]bool, len(s.Points))
	add := func(names ...string) {
		for _, n := range names {
			if n = strings.TrimSpace(n); n != "" {
				used[n] = true
			}
		}
	}
	for _, seg := range s.Segments {
		add(seg.From, seg.To)
	}
	for _, poly := range s.Polygons {
		add(poly.Points...)
	}
	for _, c := range s.Circles {
		add(c.Center, c.Through)
	}
	for _, a := range s.Arcs {
		add(a.Center)
	}
	for _, ra := range s.RightAngles {
		add(ra.Vertex, ra.A, ra.B)
	}
	for _, am := range s.AngleMarks {
		add(am.Vertex, am.A, am.B)
	}
	for _, t := range s.Ticks {
		add(t.From, t.To)
	}
	for _, m := range s.Parallels {
		add(m.From, m.To)
	}
	for _, c := range s.Curves {
		add(c.Through...)
	}
	if s.Axes != nil {
		add(s.Axes.Origin)
	}
	return used
}

// 坐标系绘制参数（画布单位，与 canvas 的 100x100 量级匹配）。
const (
	axisStrokeWidth = 1.0
	axisLabelSize   = 4.2
	axisLabelGap    = 2.0
)

// drawGrid 绘制小正方形网格背景：cols×rows 个格子，格点由 (X+i*Cell, Y+j*Cell) 给出。
// 线用浅灰细线（原图是印刷网格），不参与文字避让。
func drawGrid(b *strings.Builder, s *Spec) {
	g := s.Grid
	if g == nil || !isFinite(g.Cell) || g.Cell <= 0 || g.Cols <= 0 || g.Rows <= 0 {
		return
	}
	cols, rows := g.Cols, g.Rows
	if cols > maxGridCols {
		cols = maxGridCols
	}
	if rows > maxGridRows {
		rows = maxGridRows
	}
	stroke := colorGrid
	width := "0.35"
	dash := ""
	if g.Dashed {
		dash = gridDashArray
	}
	line := func(x1, y1, x2, y2 float64) {
		attrs := []svgAttr{
			attr("x1", num(x1)), attr("y1", num(y1)),
			attr("x2", num(x2)), attr("y2", num(y2)),
			attr("stroke", stroke), attr("stroke-width", width),
		}
		if dash != "" {
			attrs = append(attrs, attr("stroke-dasharray", dash))
		}
		writeVoid(b, "line", attrs...)
	}
	x0, y0 := g.X, g.Y
	x1 := x0 + float64(cols)*g.Cell
	y1 := y0 + float64(rows)*g.Cell
	for i := 0; i <= cols; i++ {
		x := x0 + float64(i)*g.Cell
		line(x, y0, x, y1)
	}
	for j := 0; j <= rows; j++ {
		y := y0 + float64(j)*g.Cell
		line(x0, y, x1, y)
	}
}

// axisEdgeInset 轴线端点与画布边框的最小间距（坐标轴不能顶到边框上）。
const axisEdgeInset = 2.0

// axisNegativeRatio 负方向射线相对正方向轴长的比例。
// 原图里的坐标轴是穿过原点的一整条直线（只是只在正方向画箭头），
// 若负方向只剩图形点撑出来的一小截，左侧/下侧看起来就像被截断了。
const axisNegativeRatio = 0.35

// axisRoom 从 origin 沿 dir 方向（单位向量）到画布内边界的可用距离。
func axisRoom(origin, dir vec, w, h float64) float64 {
	t := math.Inf(1)
	if dir.X > 1e-9 {
		t = math.Min(t, (w-axisEdgeInset-origin.X)/dir.X)
	} else if dir.X < -1e-9 {
		t = math.Min(t, (axisEdgeInset-origin.X)/dir.X)
	}
	if dir.Y > 1e-9 {
		t = math.Min(t, (h-axisEdgeInset-origin.Y)/dir.Y)
	} else if dir.Y < -1e-9 {
		t = math.Min(t, (axisEdgeInset-origin.Y)/dir.Y)
	}
	if math.IsInf(t, 1) || t <= 0 {
		return 0
	}
	return t
}

// axisNegativeEnd 计算坐标轴在原点负方向的终点。
// 长度取以下三者的最小可用值：
//  1. 正方向轴长的 axisNegativeRatio 倍——保证左右（上下）看起来是同一条整轴；
//  2. 负方向有图形点落在轴上时，伸到最远点之外（A(-4,0) 这类点不能被轴线贴住）；
//  3. 到画布边框的可用空间。
//
// 负方向既没有图形点、可用空间又很小时返回 false：原点在画布角落的图不凭空添线头。
func axisNegativeEnd(s *Spec, origin, u vec, skip string, posLen, w, h float64) (vec, bool) {
	const collinearTol = 1.2
	room := axisRoom(origin, mul(u, -1), w, h)
	if room <= 0 {
		return vec{}, false
	}
	farthest, found, aux := 0.0, false, false
	for _, p := range s.Points {
		if p.Name == skip {
			continue
		}
		d := sub(vec{X: p.X, Y: p.Y}, origin)
		if math.Abs(d.X*u.Y-d.Y*u.X) > collinearTol {
			continue
		}
		if t := d.X*u.X + d.Y*u.Y; t < -1 && (!found || t < farthest) {
			farthest, found, aux = t, true, p.Hidden
		}
	}
	negLen := posLen * axisNegativeRatio
	if !found {
		// 没有图形点撑着的负方向射线要有足够的空间才画，否则只是角落里的一根线头。
		if room < 0.12*math.Min(w, h) {
			return vec{}, false
		}
	} else {
		need := -farthest
		if !aux {
			// 最远点之外再放一段，避免轴线正好贴着图形点收尾。
			need += math.Max(4, math.Min(need*0.2, 0.12*math.Min(w, h)))
		}
		negLen = math.Max(negLen, need)
	}
	if negLen > room {
		negLen = room
	}
	if negLen < 1 {
		return vec{}, false
	}
	end := add(origin, mul(u, -negLen))
	// 收进画布内侧留一点余量：轴线顶到画布边框会显得"画断了"。
	end.X = clampF(end.X, axisEdgeInset, w-axisEdgeInset)
	end.Y = clampF(end.Y, axisEdgeInset, h-axisEdgeInset)
	return end, true
}

// drawAxes 绘制平面直角坐标系：轴线 → 实心箭头 → 轴标签 → 刻度。
// 箭头用填充三角形而非 SVG marker-end：导出链路用 oksvg 光栅化
// （question-service/internal/export/svg.go），对 marker 支持不可靠。
func drawAxes(b *strings.Builder, s *Spec, at func(string) (Point, bool), layout *textLayout, w, h float64) {
	a := s.Axes
	if a == nil {
		return
	}
	o, ok := at(a.Origin)
	if !ok {
		return
	}
	origin := vec{X: o.X, Y: o.Y}

	drawAxis := func(ax Axis, fallbackLabel string) {
		end, okEnd := at(ax.To)
		if !okEnd {
			return
		}
		tip := vec{X: end.X, Y: end.Y}
		length := dist(origin, tip)
		if length < 1.5 {
			return
		}
		u := unit(sub(tip, origin))
		n := vec{X: -u.Y, Y: u.X}
		// 箭头随轴长自适应，小图上不至于过大。
		arrow := clampF(length*0.06, 1.9, 3.4)
		base := add(tip, mul(u, -arrow))

		// 负方向按正方向轴长的比例补一段（原图的坐标轴是穿过原点的整条直线），
		// 若有图形落在负半轴（如 A(-4,0)）则一并延伸过去覆盖它。
		if neg, okNeg := axisNegativeEnd(s, origin, u, ax.To, length, w, h); okNeg {
			writeVoid(b, "line",
				attr("x1", num(origin.X)), attr("y1", num(origin.Y)),
				attr("x2", num(neg.X)), attr("y2", num(neg.Y)),
				attr("stroke", colorLine),
				attr("stroke-width", num(axisStrokeWidth)),
				attr("stroke-linecap", "round"),
			)
			layout.addSegment(origin, neg)
		}

		// 轴线画到箭头底部，避免线头穿出箭尖。
		writeVoid(b, "line",
			attr("x1", num(origin.X)), attr("y1", num(origin.Y)),
			attr("x2", num(base.X)), attr("y2", num(base.Y)),
			attr("stroke", colorLine),
			attr("stroke-width", num(axisStrokeWidth)),
			attr("stroke-linecap", "round"),
		)
		layout.addSegment(origin, tip)

		halfW := arrow * 0.46
		writeVoid(b, "path",
			attr("d", fmt.Sprintf("M %s %s L %s %s L %s %s Z",
				num(tip.X), num(tip.Y),
				num(base.X+n.X*halfW), num(base.Y+n.Y*halfW),
				num(base.X-n.X*halfW), num(base.Y-n.Y*halfW))),
			attr("fill", colorLine),
			attr("stroke", "none"),
		)

		label := strings.TrimSpace(ax.Label)
		if label == "" {
			label = fallbackLabel
		}
		if label == "" {
			return
		}
		pos := add(tip, mul(u, arrow+axisLabelGap))
		// 标签按轴线延长线摆放时，文字中线正好落在轴线上（x 被横轴穿过、y 被竖轴穿过）。
		// 按教材惯例错开：x 轴标签放在轴线上方，y 轴标签放在轴线右侧。
		if math.Abs(u.X) >= math.Abs(u.Y) {
			pos.Y -= axisLabelSize * 0.9
		} else {
			pos.X += axisLabelSize * 0.9
		}
		pos.X = clampF(pos.X, axisLabelSize*0.6, w-axisLabelSize*0.6)
		pos.Y = clampF(pos.Y, axisLabelSize*0.9, h-axisLabelSize*0.4)
		writeTextEl(b, pos, label, axisLabelSize, 700, "middle")
	}
	drawAxis(a.X, "x")
	drawAxis(a.Y, "y")

}

// drawCurves 绘制函数图像/平滑曲线。
func drawCurves(b *strings.Builder, s *Spec, at func(string) (Point, bool), layout *textLayout) {
	w, h := s.Canvas.Width, s.Canvas.Height
	if w <= 0 || !isFinite(w) {
		w = DefaultCanvasSize
	}
	if h <= 0 || !isFinite(h) {
		h = DefaultCanvasSize
	}
	for _, c := range s.Curves {
		nodes := make([]vec, 0, len(c.Through))
		for _, name := range c.Through {
			if p, ok := at(name); ok {
				nodes = append(nodes, vec{X: p.X, Y: p.Y})
			}
		}
		if len(nodes) < 2 {
			continue
		}
		// 模型常用 curves 表达 y=kx+b 这类函数图象：三点共线时它就是直线，
		// 要延长到图框边界，否则画出来只是两个标注点之间的一段线段。
		if !c.Smooth {
			nodes = extendIfCollinear(nodes, w, h)
		}
		writeVoid(b, "path",
			attr("d", curvePath(nodes, c.Smooth)),
			attr("fill", "none"),
			attr("stroke", colorLine),
			attr("stroke-width", "0.95"),
			attr("stroke-linecap", "round"),
			attr("stroke-linejoin", "round"),
			attr("stroke-dasharray", dashAttr(c.Dashed)),
		)
		layout.addPolyline(nodes)
	}
}

// extendIfCollinear 把"折线形式的直线"延长到画布边界，返回两个端点：
//   - 只有两个点：两点唯一确定一条直线，折线没有意义，按直线贯穿图框
//     （模型常把 y=kx+b 写成只有两个点的 curves，不延长就画成了一截线段）；
//   - 三个点及以上：仅当中间点全部共线时延长，否则视为真正的折线/曲线原样返回。
func extendIfCollinear(nodes []vec, w, h float64) []vec {
	if len(nodes) < 2 {
		return nodes
	}
	if len(nodes) == 2 {
		u := unit(sub(nodes[1], nodes[0]))
		if u.X == 0 && u.Y == 0 {
			return nodes
		}
		return []vec{
			rayToCanvasEdge(nodes[0], mul(u, -1), w, h),
			rayToCanvasEdge(nodes[1], u, w, h),
		}
	}
	a, b := nodes[0], nodes[len(nodes)-1]
	d := sub(b, a)
	l := math.Hypot(d.X, d.Y)
	if l < 1e-6 {
		return nodes
	}
	tol := math.Max(0.08, l*0.01)
	for _, p := range nodes[1 : len(nodes)-1] {
		if math.Abs(d.X*(p.Y-a.Y)-d.Y*(p.X-a.X))/l > tol {
			return nodes
		}
	}
	u := unit(d)
	return []vec{
		rayToCanvasEdge(a, mul(u, -1), w, h),
		rayToCanvasEdge(b, u, w, h),
	}
}

// curvePath 生成曲线路径：smooth 时用 Catmull-Rom 转三次贝塞尔做平滑，
// 否则按折线连接（点足够密时可近似任意曲线）。
func curvePath(nodes []vec, smooth bool) string {
	var b strings.Builder
	b.WriteString("M " + num(nodes[0].X) + " " + num(nodes[0].Y))
	if !smooth || len(nodes) < 3 {
		for _, p := range nodes[1:] {
			b.WriteString(" L " + num(p.X) + " " + num(p.Y))
		}
		return b.String()
	}
	for i := 0; i+1 < len(nodes); i++ {
		p0 := nodes[i]
		if i > 0 {
			p0 = nodes[i-1]
		}
		p1, p2 := nodes[i], nodes[i+1]
		p3 := p2
		if i+2 < len(nodes) {
			p3 = nodes[i+2]
		}
		c1 := add(p1, mul(sub(p2, p0), 1.0/6))
		c2 := sub(p2, mul(sub(p3, p1), 1.0/6))
		b.WriteString(" C " + num(c1.X) + " " + num(c1.Y) +
			" " + num(c2.X) + " " + num(c2.Y) +
			" " + num(p2.X) + " " + num(p2.Y))
	}
	return b.String()
}
