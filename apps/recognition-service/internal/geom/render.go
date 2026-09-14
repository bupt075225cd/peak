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
	dashArray        = "3 2.4"
	defaultLabelSize = 3.6
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
	b.WriteString(escapeText(content))
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
		len(s.Circles) > 0 || len(s.Arcs) > 0
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

	// 图形重心，用于把点标签自动向外偏移。
	var cx, cy float64
	for _, p := range s.Points {
		cx += p.X
		cy += p.Y
	}
	center := vec{X: w / 2, Y: h / 2}
	if n := len(s.Points); n > 0 {
		center = vec{X: cx / float64(n), Y: cy / float64(n)}
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
		if poly.Fill {
			fill = "rgba(37,99,235,0.10)"
		}
		writeVoid(&b, "polygon",
			attr("points", strings.Join(coords, " ")),
			attr("fill", fill),
			attr("stroke", colorLine),
			attr("stroke-width", "0.9"),
			attr("stroke-linejoin", "round"),
			attr("stroke-dasharray", dashAttr(poly.Dashed)),
		)
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
	for _, seg := range s.Segments {
		pa, okA := at(seg.From)
		pb, okB := at(seg.To)
		if !okA || !okB {
			continue
		}
		p1 := vec{X: pa.X, Y: pa.Y}
		p2 := vec{X: pb.X, Y: pb.Y}
		if seg.Extend {
			u := unit(sub(p2, p1))
			ext := math.Max(6, dist(p1, p2)*0.22)
			p1 = add(p1, mul(u, -ext))
			p2 = add(p2, mul(u, ext))
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
		if !p.Hidden {
			writeVoid(&b, "circle",
				attr("cx", num(pv.X)), attr("cy", num(pv.Y)),
				attr("r", "0.85"), attr("fill", colorPoint))
			layout.addDot(pv.X, pv.Y, 0.85)
		}
		var off vec
		if p.LabelDX != nil || p.LabelDY != nil {
			if p.LabelDX != nil {
				off.X = *p.LabelDX
			}
			if p.LabelDY != nil {
				off.Y = *p.LabelDY
			}
		} else {
			off = mul(unit(sub(pv, center)), 3.8)
		}
		if off.X == 0 && off.Y == 0 {
			off = vec{X: 0, Y: -3.8}
		}
		label := p.Label
		if label == "" {
			label = p.Name
		}
		pending = append(pending, textItem{
			Pos: add(pv, off), Text: label, Size: 3.9, Weight: 700, Anchor: "middle", Priority: 0,
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
