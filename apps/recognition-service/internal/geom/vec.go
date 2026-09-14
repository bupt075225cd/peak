package geom

import (
	"math"
	"strconv"
)

// vec 是画布坐标系下的点/向量。坐标系与 SVG 一致：原点在左上角，x 向右、y 向下。
type vec struct{ X, Y float64 }

func sub(a, b vec) vec { return vec{a.X - b.X, a.Y - b.Y} }

func add(a, b vec) vec { return vec{a.X + b.X, a.Y + b.Y} }

func mul(v vec, s float64) vec { return vec{v.X * s, v.Y * s} }

func mid(a, b vec) vec { return vec{(a.X + b.X) / 2, (a.Y + b.Y) / 2} }

func dist(a, b vec) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }

// unit 返回单位向量；零向量返回零向量。
func unit(v vec) vec {
	l := math.Hypot(v.X, v.Y)
	if l < 1e-6 {
		return vec{}
	}
	return vec{v.X / l, v.Y / l}
}

// deg 弧度转角度。
func deg(rad float64) float64 { return rad * 180 / math.Pi }

// polar 以 c 为圆心、r 为半径、degrees 为角度（度；0° 指向 x 轴正方向，
// 角度增大方向与 y 轴正向一致）取点。
func polar(c vec, r, degrees float64) vec {
	t := degrees * math.Pi / 180
	return vec{c.X + r*math.Cos(t), c.Y + r*math.Sin(t)}
}

// normalizeAngle 把角度归一化到 [0,360)。
func normalizeAngle(a float64) float64 {
	if !isFinite(a) {
		return 0
	}
	a = math.Mod(a, 360)
	if a < 0 {
		a += 360
	}
	return a
}

// isFinite 判断浮点数是否为有限值（既非 NaN 也非 ±Inf）。
func isFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// clampF 把 v 夹到 [lo,hi]；非有限值返回 lo（与参考实现的 clamp 语义一致）。
func clampF(v, lo, hi float64) float64 {
	if !isFinite(v) {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// clampCount 把标记数量（等长/平行/角弧条数）限制在 1~3。
func clampCount(count int) int {
	if count < 1 {
		return 1
	}
	if count > 3 {
		return 3
	}
	return count
}

// clampIntRange 四舍五入后夹到 [lo,hi]；非有限值返回 lo。
func clampIntRange(v float64, lo, hi int) int {
	n := math.Round(v)
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return lo
	}
	if n < float64(lo) {
		return lo
	}
	if n > float64(hi) {
		return hi
	}
	return int(n)
}

// num 把画布数值格式化为 SVG 属性文本：保留至多 3 位小数，非有限值输出 "0"。
// 用于避免浮点尾数膨胀 SVG 体积（与参考实现的 toFixed(3) 等价）。
func num(v float64) string {
	if !isFinite(v) {
		return "0"
	}
	return strconv.FormatFloat(math.Round(v*1000)/1000, 'f', -1, 64)
}
