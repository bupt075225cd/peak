package geom

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// semicircleHint 题干提到半圆而描述只有整圆时，回喂给模型的修正指引。
const semicircleHint = "题干提到\"半圆\"，但几何描述中只有整圆 circles、没有任何弧 arcs。" +
	"半圆必须用 arcs 表达，不要用 circles 画整圆：先在 points 中补充直径的中点（hidden:true）作为圆心，" +
	"radius 取直径长度的一半，start_angle/end_angle 只覆盖半圆弧" +
	"（水平直径、弧朝上时为 180→360，弧朝下时为 0→180，竖直直径、弧朝右时为 270→90）。"

// CheckSemicircle 题干-图形语义核对（启发式），弥补结构校验无法发现的语义偏差：
// VLM 提取几何描述时常把半圆误画成整圆（circles），而整圆在结构上完全合法，
// Validate 检测不到。当题干含"半圆"、各子图存在 circles 且没有任何 arcs 时，
// 给含圆的子图追加修正问题（进入 Issues，随回喂修正与 consistent 判定生效）。
//
// 采用保守触发：仅在"有 circles 且无 arcs"时报告，避免题干文字与配图无关时误报。
func CheckSemicircle(panels []Panel, stemText string) {
	if !strings.Contains(stemText, "半圆") {
		return
	}
	hasCircle, hasArc := false, false
	for i := range panels {
		if len(panels[i].Spec.Circles) > 0 {
			hasCircle = true
		}
		if len(panels[i].Spec.Arcs) > 0 {
			hasArc = true
		}
	}
	if !hasCircle || hasArc {
		return
	}
	for i := range panels {
		if len(panels[i].Spec.Circles) > 0 {
			panels[i].Issues = append(panels[i].Issues, semicircleHint)
		}
	}
}

// stemLengthConstraint 题干中解析出的数值型线段长度已知量（如 AB=2.3m）。
type stemLengthConstraint struct {
	from  string
	to    string
	value float64
}

var (
	// stemLengthRe 匹配 "AB=2.3" / "AB＝2.3m" 这类线段长度已知量。
	// 只认两个大写字母的线段名，避免把 ∠ABC=60° 的角名当成线段。
	stemLengthRe = regexp.MustCompile(`([A-Z]{2})\s*[=＝]\s*(\d+(?:\.\d+)?)`)
	// stemNumberRe 提取文本中的第一个数值（用于长度标注匹配）。
	stemNumberRe = regexp.MustCompile(`\d+(?:\.\d+)?`)
)

// parseStemLengths 从题干文本解析数值型线段长度已知量（同名线段只保留首个）。
// 线段名前紧跟字母或数字的命中被丢弃，排除 ∠ABC=60°（ABC 里的 BC）、2AB=6 等非长度表达。
func parseStemLengths(stemText string) []stemLengthConstraint {
	var out []stemLengthConstraint
	seen := make(map[string]bool)
	for _, loc := range stemLengthRe.FindAllStringSubmatchIndex(stemText, -1) {
		if loc[0] > 0 {
			if r, _ := utf8.DecodeLastRuneInString(stemText[:loc[0]]); r >= 'A' && r <= 'Z' ||
				r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
				continue
			}
		}
		name := stemText[loc[2]:loc[3]]
		value, err := strconv.ParseFloat(stemText[loc[4]:loc[5]], 64)
		if err != nil || !isFinite(value) || value <= 0 || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, stemLengthConstraint{from: name[:1], to: name[1:2], value: value})
	}
	return out
}

// stemLengthRatioTolerance 题干给定线段长度的比例核对容忍度：
// 坐标直出不保证数学精确，但同一子图内各线段的"画出长度/题干数值"
// 应能用同一比例因子解释，偏离超过该倍数视为与题意矛盾。
const stemLengthRatioTolerance = 1.3

// stemLabelNearRatio 长度标注归属判定：标注到"最近线段"的距离明显小于到
// 题干对应线段的距离时，才认定标注放错了线段（避免模糊位置误报）。
const stemLabelNearRatio = 0.6

// namedSeg 带端点名的线段（用于标注归属核对）。
type namedSeg struct {
	from, to string
	a, b     vec
}

// panelSegments 汇总子图中全部可量线段（segments + 多边形边）。
func panelSegments(spec *Spec) []namedSeg {
	var out []namedSeg
	add := func(from, to string) {
		a, okA := spec.PointByName(from)
		b, okB := spec.PointByName(to)
		if okA && okB && from != to {
			out = append(out, namedSeg{from: from, to: to, a: vec{a.X, a.Y}, b: vec{b.X, b.Y}})
		}
	}
	for _, s := range spec.Segments {
		add(s.From, s.To)
	}
	for _, poly := range spec.Polygons {
		for i := 0; i < len(poly.Points); i++ {
			add(poly.Points[i], poly.Points[(i+1)%len(poly.Points)])
		}
	}
	return out
}

// labelFirstNumber 提取标注文本中的第一个数值（如 "2.6m"→2.6），无数值返回 false。
func labelFirstNumber(text string) (float64, bool) {
	m := stemNumberRe.FindString(text)
	if m == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(m, 64)
	if err != nil || !isFinite(v) {
		return 0, false
	}
	return v, true
}

// stemValueEq 判断两个数值是否相等（容忍浮点尾数）。
func stemValueEq(a, b float64) bool {
	return math.Abs(a-b) <= 1e-6*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

// CheckStemLengths 题干-图形语义核对：题干给定的数值线段长度（如 AB=2.3m、BC=2.6m）
// 必须与图形一致。这类"结构合法但语义矛盾"的偏差结构校验发现不了：
//   1. 比例核对：同一子图内画出的各线段长度比例应与题干数值比例一致（缩放无关），
//      例如题干 AB=2.3、BC=2.6 时，图里 AB 画得比 BC 长即矛盾；
//   2. 标注核对：长度标注（如 "2.6m"）应放在题干对应线段附近，而不是别的线段旁。
//
// 违反时给对应子图追加修正问题（进入 Issues，随回喂修正与 consistent 判定生效）。
// 采用保守触发：仅核对题干中明确给出的数值线段，且标注只在归属明确时才报告。
func CheckStemLengths(panels []Panel, stemText string) {
	constraints := parseStemLengths(stemText)
	if len(constraints) == 0 {
		return
	}
	for i := range panels {
		spec := &panels[i].Spec

		// 1) 比例核对：画出长度 / 题干数值 应能用同一比例因子解释。
		type segRatio struct {
			c stemLengthConstraint
			k float64
		}
		var rs []segRatio
		for _, c := range constraints {
			if d := spec.distance(c.from, c.to); d > 0 {
				rs = append(rs, segRatio{c, d / c.value})
			}
		}
		if len(rs) >= 2 {
			lo, hi := rs[0], rs[0]
			for _, r := range rs[1:] {
				if r.k < lo.k {
					lo = r
				}
				if r.k > hi.k {
					hi = r
				}
			}
			if hi.k > lo.k*stemLengthRatioTolerance {
				panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
					"题干给定 %s%s=%g、%s%s=%g，但图中 %s%s 画得明显比 %s%s 长，长度比例与题干数值矛盾。"+
						"请调整 points 坐标，使各线段长度的比例与题干给定值一致（数值大的线段画得更长），并把长度标注写在对应线段旁。",
					lo.c.from, lo.c.to, lo.c.value, hi.c.from, hi.c.to, hi.c.value,
					hi.c.from, hi.c.to, lo.c.from, lo.c.to))
			}
		}

		// 2) 标注核对：数值匹配某条题干约束的标注，应离对应线段最近。
		segs := panelSegments(spec)
		if len(segs) == 0 {
			continue
		}
		for _, l := range spec.Labels {
			v, ok := labelFirstNumber(l.Text)
			if !ok {
				continue
			}
			var matched *stemLengthConstraint
			for j := range constraints {
				if stemValueEq(constraints[j].value, v) {
					matched = &constraints[j]
					break
				}
			}
			if matched == nil || strings.Contains(l.Text, matched.from+matched.to) {
				continue
			}
			pa, okA := spec.PointByName(matched.from)
			pb, okB := spec.PointByName(matched.to)
			if !okA || !okB {
				continue
			}
			lp := vec{l.X, l.Y}
			correct := namedSeg{from: matched.from, to: matched.to, a: vec{pa.X, pa.Y}, b: vec{pb.X, pb.Y}}
			dCorrect := pointSegDistance(lp, correct.a, correct.b)
			best, bestD := -1, math.MaxFloat64
			for j, s := range segs {
				if d := pointSegDistance(lp, s.a, s.b); d < bestD {
					best, bestD = j, d
				}
			}
			if best < 0 {
				continue
			}
			near := segs[best]
			sameAsCorrect := (near.from == correct.from && near.to == correct.to) ||
				(near.from == correct.to && near.to == correct.from)
			if sameAsCorrect {
				continue
			}
			// 最近线段恰好也是同数值的另一条题干约束（如 AB=CD=2.3）→ 视为正确。
			sameValueOther := false
			for _, other := range constraints {
				if stemValueEq(other.value, v) &&
					((other.from == near.from && other.to == near.to) ||
						(other.from == near.to && other.to == near.from)) {
					sameValueOther = true
					break
				}
			}
			if sameValueOther || bestD >= stemLabelNearRatio*dCorrect {
				continue
			}
			panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
				"长度标注“%s”画在了线段 %s%s 附近，但题干中 %g 是线段 %s%s 的长度；请把该标注移到 %s%s 旁（或改用正确的数值）。",
				l.Text, near.from, near.to, matched.value, matched.from, matched.to, matched.from, matched.to))
		}
	}
}

// semicircleDirection 从题干判断半圆应在上方还是下方：
// 在"半圆"附近（前后各数个字）找"上面/上方/顶"→ 朝上；"下面/下方/底" → 朝下。
// 都找不到时返回 0（朝向不明，保守跳过核对）。
// 返回值：1 朝上，-1 朝下，0 未知。
func semicircleDirection(stemText string) int {
	const window = 12 // 半圆前后各看 12 个字节（约 4 个汉字）
	runes := []rune(stemText)
	for i := 0; i+2 <= len(runes); i++ {
		if string(runes[i:i+2]) != "半圆" {
			continue
		}
		before := string(runes[max(0, i-window/3):i])
		after := string(runes[i+2:min(len(runes), i+2+window/3)])
		for _, kw := range []string{"上面", "上方", "顶部", "顶上"} {
			if strings.Contains(before, kw) || strings.Contains(after, kw) {
				return 1
			}
		}
		for _, kw := range []string{"下面", "下方", "底部"} {
			if strings.Contains(before, kw) || strings.Contains(after, kw) {
				return -1
			}
		}
	}
	return 0
}

// CheckSemicircleDirection 题干-图形语义核对：题干说"上面（下面）是半圆形"时，
// 半圆弧必须开口朝上（下）——直径为水平边、弧在直径上方（下方）。
// VLM 偶尔把半圆画到侧面（竖直直径、弧朝左右），结构完全合法但与题意矛盾。
// 违反时给含弧的子图追加修正问题（进入 Issues，随回喂修正与 consistent 判定生效）。
// 朝向不明（题干未说明上/下）时保守跳过。
func CheckSemicircleDirection(panels []Panel, stemText string) {
	dir := semicircleDirection(stemText)
	if dir == 0 {
		return
	}
	for i := range panels {
		for _, a := range panels[i].Spec.Arcs {
			c, ok := panels[i].Spec.PointByName(a.Center)
			if !ok || a.Radius <= 0 || !isFinite(a.Radius) {
				continue
			}
			start := normalizeAngle(a.StartAngle)
			delta := math.Mod(normalizeAngle(a.EndAngle)-start, 360)
			if delta < 0 {
				delta += 360
			}
			if delta == 0 {
				continue // 起止相同在结构校验中已报
			}
			// 与渲染器一致：弧从 start 沿角度增大方向扫 delta。
			mid := polar(vec{c.X, c.Y}, a.Radius, start+delta/2)
			p1 := polar(vec{c.X, c.Y}, a.Radius, start)
			p2 := polar(vec{c.X, c.Y}, a.Radius, start+delta)
			horizontal := math.Abs(p1.Y-p2.Y) <= 0.25*a.Radius
			opensUp := mid.Y < c.Y
			if horizontal && ((dir > 0) == opensUp) {
				continue
			}
			if dir > 0 {
				panels[i].Issues = append(panels[i].Issues,
					"题干说“上面是半圆形”，但图中半圆弧没有朝上（直径不是水平边或弧朝向侧面）。"+
						"请把半圆画在长方形上方：直径取顶部的水平边（在 points 中补充直径中点 hidden:true 作圆心），"+
						"弧朝上，start_angle=180、end_angle=360。")
			} else {
				panels[i].Issues = append(panels[i].Issues,
					"题干说“下面是半圆形”，但图中半圆弧没有朝下（直径不是水平边或弧朝向侧面）。"+
						"请把半圆画在长方形下方：直径取底部的水平边（在 points 中补充直径中点 hidden:true 作圆心），"+
						"弧朝下，start_angle=0、end_angle=180。")
			}
			break // 每个子图报一次即可
		}
	}
}
