package geom

import "math"

// 文字避让代价权重（与参考实现 createTextLayout 保持一致）。
const (
	textCostHit     = 300.0 // 与其它文字/点发生任意重叠
	textCostOverlap = 900.0 // 重叠面积占比的放大系数
	textCostNear    = 700.0 // 离线段、圆周太近
	textCostMove    = 34.0  // 偏离模型给定位置（每移动一个步长）
)

// textDirs 候选落点方向：优先正上/正下，其次左右，最后四个斜向。
var textDirs = [...]vec{
	{0, -1}, {0, 1}, {-1, 0}, {1, 0},
	{-0.7071, -0.7071}, {0.7071, -0.7071},
	{-0.7071, 0.7071}, {0.7071, 0.7071},
}

// rect 轴对齐矩形（已放置的文字框、实心点占位）。
type rect struct{ X0, Y0, X1, Y1, W, H float64 }

type lineSeg struct{ A, B vec }

type circleRing struct {
	C vec
	R float64
}

// textItem 待摆放的一段文字。
type textItem struct {
	Pos      vec
	Text     string
	Size     float64
	Weight   int
	Anchor   string
	Priority int
}

// textLayout 文字摆放器：先登记线/圆/点等障碍，再逐个为文字选代价最低的落点。
type textLayout struct {
	w, h  float64
	boxes []rect
	segs  []lineSeg
	rings []circleRing
}

// newTextLayout 创建一个文字摆放器（w/h 为画布尺寸）。
func newTextLayout(w, h float64) *textLayout {
	return &textLayout{w: w, h: h}
}

// addSegment 登记一条线段（或折线的一段）。
func (l *textLayout) addSegment(a, b vec) {
	if dist(a, b) > 1e-6 {
		l.segs = append(l.segs, lineSeg{a, b})
	}
}

// addPolyline 登记一条折线。
func (l *textLayout) addPolyline(nodes []vec) {
	for i := 0; i+1 < len(nodes); i++ {
		l.addSegment(nodes[i], nodes[i+1])
	}
}

// addPolygon 登记多边形（按边避让，内部不阻挡）。
func (l *textLayout) addPolygon(nodes []vec) {
	if len(nodes) < 3 {
		return
	}
	closed := make([]vec, 0, len(nodes)+1)
	closed = append(closed, nodes...)
	closed = append(closed, nodes[0])
	l.addPolyline(closed)
}

// addCircle 登记圆/圆弧的轮廓。
func (l *textLayout) addCircle(c vec, r float64) {
	if r > 0 {
		l.rings = append(l.rings, circleRing{C: c, R: r})
	}
}

// addDot 登记实心点，避免文字压在点上。
func (l *textLayout) addDot(x, y, r float64) {
	l.boxes = append(l.boxes, rect{
		X0: x - r, Y0: y - r, X1: x + r, Y1: y + r, W: 2 * r, H: 2 * r,
	})
}

// place 为一段文字选一个代价最低的落点。
func (l *textLayout) place(item textItem) vec {
	size := item.Size
	if size <= 0 || !isFinite(size) {
		size = 3.6
	}
	bw := MeasureTextWidth(item.Text, size) + 1.2
	bh := size * 1.2
	pad := math.Max(bh*0.5+0.9, 2.2) // 与线、圆保持的最小间距
	step := math.Max(bh*1.05, 3)     // 候选点间距（步长）

	found := false
	var bestCost float64
	var best vec
	for ring := 0; ring <= 4; ring++ {
		for i := range textDirs {
			if ring == 0 && i > 0 {
				continue // 原点只需试一次
			}
			pos := clampTextPos(
				item.Pos.X+textDirs[i].X*step*float64(ring),
				item.Pos.Y+textDirs[i].Y*step*float64(ring),
				bw, bh, item.Anchor, l.w, l.h)
			r := rectOf(pos.X, pos.Y, bw, bh, item.Anchor)
			if r.X0 < -0.6 || r.Y0 < -0.6 || r.X1 > l.w+0.6 || r.Y1 > l.h+0.6 {
				continue // 越界的位置直接丢弃
			}
			cost := l.costOf(r, pad) + textCostMove*dist(pos, item.Pos)/step
			if !found || cost < bestCost-1e-9 {
				found = true
				bestCost = cost
				best = pos
			}
		}
		if found && bestCost < 0.5 {
			break // 已经放到干净位置，不再外扩
		}
	}
	if !found { // 兜底：贴画布边缘
		best = clampTextPos(item.Pos.X, item.Pos.Y, bw, bh, item.Anchor, l.w, l.h)
	}
	// 登记为障碍，留一点字间距。
	r := rectOf(best.X, best.Y, bw, bh, item.Anchor)
	l.boxes = append(l.boxes, rect{
		X0: r.X0 - 0.5, Y0: r.Y0 - 0.3, X1: r.X1 + 0.5, Y1: r.Y1 + 0.3,
		W: bw + 1, H: bh + 0.6,
	})
	return best
}

// costOf 计算把文字放在 r 处的避让代价。
func (l *textLayout) costOf(r rect, pad float64) float64 {
	var cost float64
	for _, b := range l.boxes {
		if ov := overlapArea(r, b); ov > 0 {
			cost += textCostHit + textCostOverlap*ov/(r.W*r.H)
		}
	}
	for _, s := range l.segs {
		if d := segRectDistance(s.A, s.B, r); d < pad {
			t := 1 - d/pad
			cost += textCostNear * t * t
		}
	}
	for _, c := range l.rings {
		if d := ringRectDistance(c, r); d < pad {
			t := 1 - d/pad
			cost += textCostNear * t * t
		}
	}
	return cost
}

// rectOf 按锚点（start|middle|end）计算文字框。
func rectOf(x, y, w, h float64, anchor string) rect {
	var x0 float64
	switch anchor {
	case "start":
		x0 = x
	case "end":
		x0 = x - w
	default:
		x0 = x - w/2
	}
	return rect{X0: x0, Y0: y - h/2, X1: x0 + w, Y1: y + h/2, W: w, H: h}
}

// overlapArea 两个矩形的重叠面积。
func overlapArea(a, b rect) float64 {
	w := math.Min(a.X1, b.X1) - math.Max(a.X0, b.X0)
	h := math.Min(a.Y1, b.Y1) - math.Max(a.Y0, b.Y0)
	if w > 0 && h > 0 {
		return w * h
	}
	return 0
}

// clampTextPos 把文字框整体收进画布（锚点不变，只平移）。
func clampTextPos(x, y, w, h float64, anchor string, W, H float64) vec {
	r := rectOf(x, y, w, h, anchor)
	nx, ny := x, y
	if r.X1 > W-0.6 {
		nx -= r.X1 - (W - 0.6)
	}
	if r.X0 < 0.6 {
		nx += 0.6 - r.X0
	}
	if r.Y1 > H-0.6 {
		ny -= r.Y1 - (H - 0.6)
	}
	if r.Y0 < 0.6 {
		ny += 0.6 - r.Y0
	}
	return vec{nx, ny}
}

// pointRectDistance 点到矩形的最短距离（点在矩形内为 0）。
func pointRectDistance(p vec, r rect) float64 {
	dx := math.Max(math.Max(r.X0-p.X, 0), p.X-r.X1)
	dy := math.Max(math.Max(r.Y0-p.Y, 0), p.Y-r.Y1)
	return math.Hypot(dx, dy)
}

// pointSegDistance 点到线段的最短距离。
func pointSegDistance(p, a, b vec) float64 {
	vx, vy := b.X-a.X, b.Y-a.Y
	len2 := vx*vx + vy*vy
	if len2 < 1e-9 {
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	}
	t := ((p.X-a.X)*vx + (p.Y-a.Y)*vy) / len2
	t = math.Min(1, math.Max(0, t))
	return math.Hypot(p.X-(a.X+t*vx), p.Y-(a.Y+t*vy))
}

// segIntersectsRect 线段是否与矩形相交（Liang-Barsky 裁剪）。
func segIntersectsRect(a, b vec, r rect) bool {
	t0, t1 := 0.0, 1.0
	dx, dy := b.X-a.X, b.Y-a.Y
	p := [4]float64{-dx, dx, -dy, dy}
	q := [4]float64{a.X - r.X0, r.X1 - a.X, a.Y - r.Y0, r.Y1 - a.Y}
	for i := 0; i < 4; i++ {
		if p[i] == 0 {
			if q[i] < 0 {
				return false
			}
			continue
		}
		t := q[i] / p[i]
		if p[i] < 0 {
			if t > t0 {
				t0 = t
				if t0 > t1 {
					return false
				}
			}
		} else if t < t1 {
			t1 = t
			if t0 > t1 {
				return false
			}
		}
	}
	return true
}

// segRectDistance 线段到矩形的最短距离（相交为 0）。
func segRectDistance(a, b vec, r rect) float64 {
	if segIntersectsRect(a, b, r) {
		return 0
	}
	d := math.Min(math.Min(
		pointSegDistance(vec{r.X0, r.Y0}, a, b),
		pointSegDistance(vec{r.X1, r.Y0}, a, b)),
		math.Min(
			pointSegDistance(vec{r.X1, r.Y1}, a, b),
			pointSegDistance(vec{r.X0, r.Y1}, a, b)))
	return math.Min(d, math.Min(pointRectDistance(a, r), pointRectDistance(b, r)))
}

// ringRectDistance 矩形到圆周（圆环）的最短距离。
func ringRectDistance(c circleRing, r rect) float64 {
	minD := pointRectDistance(c.C, r)
	maxD := math.Max(math.Max(
		math.Hypot(c.C.X-r.X0, c.C.Y-r.Y0),
		math.Hypot(c.C.X-r.X1, c.C.Y-r.Y0)),
		math.Max(
			math.Hypot(c.C.X-r.X1, c.C.Y-r.Y1),
			math.Hypot(c.C.X-r.X0, c.C.Y-r.Y1)))
	if c.R < minD {
		return minD - c.R
	}
	if c.R > maxD {
		return c.R - maxD
	}
	return 0
}
