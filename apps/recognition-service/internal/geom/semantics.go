package geom

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
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

// panelEdgeSegments 汇总子图中"真正画出来的边"（普通线段 + 多边形边）。
// 与 panelSegments 的区别是排除 extend 线段：函数图象/直线穿过两个顶点，并不等于
// 原图画了这条三角形的边（如 l₁ 穿过 C、D，不能认为 CD 边已经画出来）。
func panelEdgeSegments(spec *Spec) []namedSeg {
	var out []namedSeg
	add := func(from, to string) {
		a, okA := spec.PointByName(from)
		b, okB := spec.PointByName(to)
		if okA && okB && from != to {
			out = append(out, namedSeg{from: from, to: to, a: vec{a.X, a.Y}, b: vec{b.X, b.Y}})
		}
	}
	for _, s := range spec.Segments {
		if s.Extend {
			continue
		}
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
//  1. 比例核对：同一子图内画出的各线段长度比例应与题干数值比例一致（缩放无关），
//     例如题干 AB=2.3、BC=2.6 时，图里 AB 画得比 BC 长即矛盾；
//  2. 标注核对：长度标注（如 "2.6m"）应放在题干对应线段附近，而不是别的线段旁。
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

// stemPassPointRe 匹配 "XX恰好经过点P" / "XX经过点P"（XX 为两条线段端点名拼接，如 C1D1）。
// 题干中的下标数字（C₁）先统一归一化为 ASCII（C1）再匹配。
var stemPassPointRe = regexp.MustCompile(`([A-Z][0-9]?)([A-Z][0-9]?)\s*(?:恰好)?经\s*过\s*点\s*([A-Z][0-9]?)`)

// stemSubscripts 题干/点名中的 Unicode 下标数字 → ASCII 数字。
var stemSubscripts = strings.NewReplacer(
	"₀", "0", "₁", "1", "₂", "2", "₃", "3", "₄", "4",
	"₅", "5", "₆", "6", "₇", "7", "₈", "8", "₉", "9")

// collinearityConstraint 题干中解析出的"线段经过点"共线约束（如 边C₁D₁恰好经过点A）。
type collinearityConstraint struct {
	from, to, point string
}

// parseStemPassPoints 从题干解析"线段经过点"约束（同一线段+点去重）。
func parseStemPassPoints(stemText string) []collinearityConstraint {
	normalized := stemSubscripts.Replace(stemText)
	var out []collinearityConstraint
	seen := make(map[string]bool)
	for _, m := range stemPassPointRe.FindAllStringSubmatch(normalized, -1) {
		a, b, p := m[1], m[2], m[3]
		if a == b {
			continue
		}
		key := a + b + p
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, collinearityConstraint{from: a, to: b, point: p})
	}
	return out
}

// stemCollinearTolerance "恰好经过点"共线核对容忍度：点到线段距离超过线段长度的
// 该比例视为明显不共线（坐标直出不保证精确，但"恰好经过"应接近共线）。
const stemCollinearTolerance = 0.15

// normalizedPointIndex 点名的归一化索引（下标数字统一为 ASCII，便于与题干匹配）。
func normalizedPointIndex(spec *Spec) map[string]Point {
	idx := make(map[string]Point, len(spec.Points))
	for _, p := range spec.Points {
		idx[stemSubscripts.Replace(p.Name)] = p
	}
	return idx
}

// CheckStemPassPoints 题干-图形语义核对：题干中"XX恰好经过点P"（如"边C₁D₁恰好经过点A"）
// 要求点 P 落在线段 XX 上（共线）。折叠/交点类题目常依赖这类条件，模型坐标直出时
// 容易忽略，画出 P 远离 XX 的图。点到线段距离超过线段长度的容忍比例即判为矛盾，
// 回喂修正并计入 consistent 判定。题干无"经过点"表述时保守跳过。
func CheckStemPassPoints(panels []Panel, stemText string) {
	constraints := parseStemPassPoints(stemText)
	if len(constraints) == 0 {
		return
	}
	for i := range panels {
		idx := normalizedPointIndex(&panels[i].Spec)
		for _, c := range constraints {
			pa, okA := idx[c.from]
			pb, okB := idx[c.to]
			pp, okP := idx[c.point]
			if !okA || !okB || !okP {
				continue
			}
			a, b, p := vec{pa.X, pa.Y}, vec{pb.X, pb.Y}, vec{pp.X, pp.Y}
			segLen := dist(a, b)
			if segLen <= 0 {
				continue
			}
			if ratio := pointSegDistance(p, a, b) / segLen; ratio > stemCollinearTolerance {
				panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
					"题干说 %s%s 恰好经过点 %s，但图中点 %s 明显不在线段 %s%s 上（偏离约为线段长度的 %.0f%%）。"+
						"请先按题意推算各点相对位置（如折叠关系、共线条件），调整 points 坐标使 %s 落在 %s%s 所在直线上，"+
						"并保持其余图形要素协调。",
					c.from, c.to, c.point, c.point, c.from, c.to, ratio*100,
					c.point, c.from, c.to))
			}
		}
	}
}

// stemOnEdgeRe 匹配"点P在OA边上" / "点P在边OA上" / "点P在线段OA上" / "点M在BC上"。
// 只认大写字母点名，避免把"点P在x轴上"之类误当约束。
var stemOnEdgeRe = regexp.MustCompile(
	`点\s*([A-Z][0-9]?)\s*在\s*(?:线段|边)?\s*([A-Z][0-9]?)\s*([A-Z][0-9]?)\s*(?:边|线段)?\s*上`)

// onEdgeConstraint 题干中解析出的"点在边上"约束。
type onEdgeConstraint struct {
	point, from, to string
}

// stemIntersectOnRe 匹配"交CD于点G"：交点 G 必须落在线段 CD 上
// （"交 CD 的延长线于点 G" 这类带"的延长线"的表述不匹配，避免误判）。
var stemIntersectOnRe = regexp.MustCompile(
	`交\s*(?:线段|直线)?\s*([A-Z][0-9]?)\s*([A-Z][0-9]?)\s*于点\s*([A-Z][0-9]?)`)

// parseStemOnEdge 从题干解析"点在边上"约束（同一点+同一边去重）。
// 同时覆盖"点P在OA边上"与"交CD于点G"两种常见表述。
func parseStemOnEdge(stemText string) []onEdgeConstraint {
	normalized := stemSubscripts.Replace(stemText)
	var out []onEdgeConstraint
	seen := make(map[string]bool)
	add := func(p, a, b string) {
		if a == b || p == a || p == b {
			return
		}
		key := p + a + b
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, onEdgeConstraint{point: p, from: a, to: b})
	}
	for _, m := range stemOnEdgeRe.FindAllStringSubmatch(normalized, -1) {
		add(m[1], m[2], m[3])
	}
	for _, m := range stemIntersectOnRe.FindAllStringSubmatch(normalized, -1) {
		add(m[3], m[1], m[2])
	}
	return out
}

// stemOnEdgeRatio 点偏离线段所在直线的容忍度（相对线段长度）。
const stemOnEdgeRatio = 0.12

// CheckPointOnEdge 题干-图形语义核对：题干说"点P在OA边上"时，P 必须落在
// 线段 OA 上（两端点之间），不得画到边的延长线上。
// 折叠/动点类题目模型常把点推到边的外侧（例如把 P 画在 OA 的 A 端之外），
// 结构完全合法但与原图/题意矛盾，只有这类几何位置核对才能发现。
// 违反时给对应子图追加修正问题（进入 Issues，随回喂修正与 consistent 判定生效）。
func CheckPointOnEdge(panels []Panel, stemText string) {
	constraints := parseStemOnEdge(stemText)
	if len(constraints) == 0 {
		return
	}
	for i := range panels {
		idx := normalizedPointIndex(&panels[i].Spec)
		for _, c := range constraints {
			pa, okA := idx[c.from]
			pb, okB := idx[c.to]
			pp, okP := idx[c.point]
			if !okA || !okB || !okP {
				continue
			}
			a, b, p := vec{pa.X, pa.Y}, vec{pb.X, pb.Y}, vec{pp.X, pp.Y}
			segLen := dist(a, b)
			if segLen <= 0 {
				continue
			}
			d := sub(b, a)
			t := ((p.X-a.X)*d.X + (p.Y-a.Y)*d.Y) / (segLen * segLen)
			if t < -0.02 || t > 1.02 {
				side := c.from
				if t > 1 {
					side = c.to
				}
				panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
					"题干说点 %s 在 %s%s 边上，但图中点 %s 落在 %s%s 的延长线上（在 %s 的外侧）。"+
						"请把 %s 放到 %s 与 %s 之间（线段内部），再按折叠/动点关系调整其余点位。",
					c.point, c.from, c.to, c.point, c.from, c.to, side,
					c.point, c.from, c.to))
				continue
			}
			if ratio := pointSegDistance(p, a, b) / segLen; ratio > stemOnEdgeRatio {
				panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
					"题干说点 %s 在 %s%s 边上，但图中点 %s 明显偏离线段 %s%s（偏离约为线段长度的 %.0f%%）。"+
						"请调整 points 坐标，使 %s 落在线段 %s%s 上。",
					c.point, c.from, c.to, c.point, c.from, c.to, ratio*100,
					c.point, c.from, c.to))
			}
		}
	}
}

var (
	// figureIndexRe 匹配"图1".."图9"。
	figureIndexRe = regexp.MustCompile(`图\s*([1-9])`)
	// figureCountRe 匹配"三幅（子）图""三张图"这类中文数量表述。
	figureCountRe = regexp.MustCompile(`([一二三四五六七八九])\s*[幅张个]\s*(?:子图|图)`)
)

// chineseDigit 汉字数字 → 整数（1~9，其它返回 0）。
func chineseDigit(s string) int {
	switch s {
	case "一":
		return 1
	case "二":
		return 2
	case "三":
		return 3
	case "四":
		return 4
	case "五":
		return 5
	case "六":
		return 6
	case "七":
		return 7
	case "八":
		return 8
	case "九":
		return 9
	}
	return 0
}

// expectedFigureCount 从题干与几何描述推断应有的子图数量。
// 依据：图1/图2/图3 的最大编号、中文数量表述（三幅图）、以及"备用图"。
func expectedFigureCount(text string) int {
	idxMax := 0
	for _, m := range figureIndexRe.FindAllStringSubmatch(text, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n > idxMax {
			idxMax = n
		}
	}
	expect := idxMax
	if m := figureCountRe.FindStringSubmatch(text); m != nil {
		if n := chineseDigit(m[1]); n > expect {
			expect = n
		}
	}
	// "备用图"通常排在图1/图2之后，是额外一张；已被中文数量覆盖时不重复计。
	if strings.Contains(text, "备用图") && expect == idxMax {
		expect++
	}
	return expect
}

// CheckFigureCount 子图数量核对：原图/题干有几张子图（含"备用图"）就必须输出几张 panel。
// 模型偶尔只还原前两张、把"备用图"整张漏掉，缺失的图没有任何结构错误，
// 只能靠数量核对发现。数量不足时给首个子图追加修正问题，触发回喂补齐。
// 保守触发：期望数量 2~4 张且确实多于当前输出。
func CheckFigureCount(panels []Panel, hintText string) {
	if len(panels) == 0 || len(panels) >= 4 {
		return
	}
	expect := expectedFigureCount(hintText)
	if expect <= len(panels) || expect > 4 {
		return
	}
	panels[0].Issues = append(panels[0].Issues, fmt.Sprintf(
		"原图与题干共有 %d 张子图（如图1、图2、备用图），但当前只输出 %d 张。"+
			"请为每一张子图各输出一个 panel（各自独立的 canvas 与 points），一张都不能少。",
		expect, len(panels)))
}

var (
	// stemTriangleDeltaRe 匹配 "△DEM"。
	stemTriangleDeltaRe = regexp.MustCompile(`△\s*([A-Z][0-9]?)\s*([A-Z][0-9]?)\s*([A-Z][0-9]?)`)
	// stemTriangleRe 匹配 "三角形DEM"。
	stemTriangleRe = regexp.MustCompile(`三角形\s*([A-Z][0-9]?)([A-Z][0-9]?)([A-Z][0-9]?)`)
	// stemTriangleVertRe 匹配 "以点D、E、M为顶点的三角形"。
	stemTriangleVertRe = regexp.MustCompile(
		`以点\s*([A-Z][0-9]?)\s*[、,，]\s*([A-Z][0-9]?)\s*[、,，]\s*([A-Z][0-9]?)\s*(?:为顶点|三点)`)
)

// triangleConstraint 题干中解析出的三角形顶点约束。
type triangleConstraint struct{ a, b, c string }

// parseStemTriangles 从题干解析三角形顶点（同一组合去重，顶点按名字排序作为键）。
func parseStemTriangles(stemText string) []triangleConstraint {
	normalized := stemSubscripts.Replace(stemText)
	var out []triangleConstraint
	seen := make(map[string]bool)
	add := func(a, b, c string) {
		if a == b || b == c || a == c {
			return
		}
		key := a + b + c
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, triangleConstraint{a: a, b: b, c: c})
	}
	for _, m := range stemTriangleDeltaRe.FindAllStringSubmatch(normalized, -1) {
		add(m[1], m[2], m[3])
	}
	for _, m := range stemTriangleRe.FindAllStringSubmatch(normalized, -1) {
		add(m[1], m[2], m[3])
	}
	for _, m := range stemTriangleVertRe.FindAllStringSubmatch(normalized, -1) {
		add(m[1], m[2], m[3])
	}
	return out
}

// segCoverTolerance 判断"某条已画线段是否覆盖给定两点连线"的垂直偏离容忍度（相对该线段长度）。
const segCoverTolerance = 0.06

// segmentCovers 判断线段 pq 是否完整覆盖线段 ab（a、b 的投影都落在 pq 上且离线不远）。
// 用于认定"贴在坐标轴上的边"已被轴本身覆盖（如 M、D 都在 x 轴上时的 MD 边）。
func segmentCovers(a, b, p, q vec) bool {
	l := dist(p, q)
	if l < 1e-6 {
		return false
	}
	u := unit(sub(q, p))
	proj := func(v vec) (float64, float64) {
		d := sub(v, p)
		return (d.X*u.X + d.Y*u.Y) / l, math.Abs(d.X*u.Y - d.Y*u.X)
	}
	ta, na := proj(a)
	tb, nb := proj(b)
	tol := segCoverTolerance * l
	return ta >= -0.03 && ta <= 1.03 && tb >= -0.03 && tb <= 1.03 && na <= tol && nb <= tol
}

// CheckTriangleEdges 题干-图形语义核对：题干出现"△DEM"或"以点D、E、M为顶点的三角形"时，
// 三角形的三条边都必须在图上画出来（贴在某条轴上的边可由轴本身覆盖）。
// 模型常漏画动点相关的一条边（例如只画了折痕 PD、DE，漏掉 EM），结构校验发现不了。
func CheckTriangleEdges(panels []Panel, stemText string) {
	tris := parseStemTriangles(stemText)
	if len(tris) == 0 {
		return
	}
	for i := range panels {
		// "备用图"是给学生在考场上自己作图的空白图，原图里没有题干后续小问才引入的
		// 连线（如 △COD 的边），不能按题干把它也逼着画出来。
		if strings.Contains(panels[i].Title, "备用") {
			continue
		}
		spec := &panels[i].Spec
		segs := panelEdgeSegments(spec)
		// 坐标轴也算可覆盖的线（例如两点都在 x 轴上时，轴本身覆盖这条边）。
		if a := spec.Axes; a != nil {
			if o, okO := spec.PointByName(a.Origin); okO {
				ov := vec{o.X, o.Y}
				for _, end := range []string{a.X.To, a.Y.To} {
					if e, okE := spec.PointByName(end); okE {
						segs = append(segs, namedSeg{from: a.Origin, to: end, a: ov, b: vec{e.X, e.Y}})
					}
				}
			}
		}
		for _, t := range tris {
			pa, okA := spec.PointByName(t.a)
			pb, okB := spec.PointByName(t.b)
			pc, okC := spec.PointByName(t.c)
			if !okA || !okB || !okC {
				continue // 该子图没有这个三角形（如未画动点 M 的图），保守跳过
			}
			idx := map[string]vec{t.a: {pa.X, pa.Y}, t.b: {pb.X, pb.Y}, t.c: {pc.X, pc.Y}}
			edges := [][2]string{{t.a, t.b}, {t.b, t.c}, {t.c, t.a}}
			var missing []string
			coveredCount := 0
			for _, e := range edges {
				x, y := idx[e[0]], idx[e[1]]
				covered := false
				for _, s := range segs {
					if (s.from == e[0] && s.to == e[1]) || (s.from == e[1] && s.to == e[0]) ||
						segmentCovers(x, y, s.a, s.b) {
						covered = true
						break
					}
				}
				if covered {
					coveredCount++
				} else {
					missing = append(missing, e[0]+e[1])
				}
			}
			// 只画出一条边（甚至一条都没有）时不要补画：题干里的"△COD 的面积为 6"
			// 往往只是计算条件，而那条"边"常常只是函数图象恰好穿过两个点。
			// 只有已经画了两条边、明显缺第三条时才算漏画。
			if coveredCount < 2 {
				continue
			}
			if len(missing) > 0 {
				panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
					"题干以点 %s、%s、%s 为顶点的三角形，但图中没有画出 %s 边。"+
						"请把这些边补进该子图的 segments（即使与坐标轴重合也要单独输出），保证三角形完整。",
					t.a, t.b, t.c, strings.Join(missing, "、")))
			}
		}
	}
}

// ---------- 与坐标轴重合的线段 / 直线名称核对 ----------

// axisCoincidentSegment 报告线段是否整条压在坐标轴上（画出来会被轴线完全盖住）。
func (s *Spec) axisCoincidentSegment(seg Segment) bool {
	a := s.Axes
	if a == nil {
		return false
	}
	o, okO := s.PointByName(a.Origin)
	p, okP := s.PointByName(seg.From)
	q, okQ := s.PointByName(seg.To)
	if !okO || !okP || !okQ {
		return false
	}
	const tol = 0.6
	if math.Abs(p.Y-q.Y) <= tol && math.Abs(p.Y-o.Y) <= tol {
		return true // 落在水平轴（通常 x 轴）上
	}
	if math.Abs(p.X-q.X) <= tol && math.Abs(p.X-o.X) <= tol {
		return true // 落在竖直轴（通常 y 轴）上
	}
	return false
}

// segmentRedundant 报告第 idx 条线段的两端点能否只经由其它图元连通。
// 三角形/多边形与轴重合的边属于这种情况，是图形的真实边，不算问题。
func (s *Spec) segmentRedundant(idx int) bool {
	if idx < 0 || idx >= len(s.Segments) {
		return false
	}
	from := strings.TrimSpace(s.Segments[idx].From)
	to := strings.TrimSpace(s.Segments[idx].To)
	if from == "" || to == "" || from == to {
		return false
	}
	adj := make(map[string][]string, len(s.Points))
	link := func(a, b string) {
		a, b = strings.TrimSpace(a), strings.TrimSpace(b)
		if a == "" || b == "" || a == b {
			return
		}
		adj[a] = append(adj[a], b)
		adj[b] = append(adj[b], a)
	}
	for i, seg := range s.Segments {
		// 只有普通线段才构成闭合图形的边；extend 线段是函数图象（直线），
		// 让它参与连通性判断会把"O→B→A"这类真实图象误当成闭合边。
		if i == idx || seg.Extend {
			continue
		}
		link(seg.From, seg.To)
	}
	for _, poly := range s.Polygons {
		for j := range poly.Points {
			link(poly.Points[j], poly.Points[(j+1)%len(poly.Points)])
		}
	}
	if a := s.Axes; a != nil {
		link(a.Origin, a.X.To)
		link(a.Origin, a.Y.To)
	}
	seen := map[string]bool{from: true}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range adj[cur] {
			if next == to {
				return true
			}
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}

// floatingTolRatio 标注点"漂浮"的判定阈值：到所有已画图元的最小距离超过该比例即视为漂浮。
const floatingTolRatio = 0.08

// drawnLines 收集子图中所有"画出来的线"（延长线段按画布边界延长）。
func (s *Spec) drawnLines() []line2 {
	var out []line2
	w, h := s.Canvas.Width, s.Canvas.Height
	add := func(pi, pj int) {
		if pi >= 0 && pj >= 0 && pi != pj {
			out = append(out, line2{a: s.pt(pi), b: s.pt(pj)})
		}
	}
	for _, seg := range s.Segments {
		pi, pj := s.pointIndex(seg.From), s.pointIndex(seg.To)
		if pi < 0 || pj < 0 || pi == pj {
			continue
		}
		a, b := s.pt(pi), s.pt(pj)
		if seg.Extend {
			u := unit(sub(b, a))
			if u.X != 0 || u.Y != 0 {
				a = rayToCanvasEdge(a, mul(u, -1), w, h)
				b = rayToCanvasEdge(b, u, w, h)
			}
		}
		out = append(out, line2{a: a, b: b})
	}
	if a := s.Axes; a != nil {
		oi := s.pointIndex(a.Origin)
		add(oi, s.pointIndex(a.X.To))
		add(oi, s.pointIndex(a.Y.To))
	}
	for _, poly := range s.Polygons {
		for i := range poly.Points {
			add(s.pointIndex(poly.Points[i]), s.pointIndex(poly.Points[(i+1)%len(poly.Points)]))
		}
	}
	for _, c := range s.Curves {
		for i := 0; i+1 < len(c.Through); i++ {
			add(s.pointIndex(c.Through[i]), s.pointIndex(c.Through[i+1]))
		}
	}
	return out
}

// CheckFloatingPoints 坐标系题里的标注点必须落在某条线或坐标轴上（顶点、交点、轴上的点
// 都是如此）。模型偶尔把某个点放偏：实测 B 该是 l₁ 与 y 轴的交点，却漂浮在两条线之间，
// 图上就出现一个孤立的点，与原图不符。圆心不在圆周上，单独排除。
func CheckFloatingPoints(panels []Panel) {
	for i := range panels {
		spec := &panels[i].Spec
		if spec.Axes == nil {
			continue
		}
		lines := spec.drawnLines()
		if len(lines) == 0 {
			continue
		}
		tol := floatingTolRatio * math.Min(spec.Canvas.Width, spec.Canvas.Height)
		centers := make(map[string]bool, len(spec.Circles)+len(spec.Arcs))
		for _, c := range spec.Circles {
			centers[strings.TrimSpace(c.Center)] = true
		}
		for _, a := range spec.Arcs {
			centers[strings.TrimSpace(a.Center)] = true
		}
		for _, p := range spec.Points {
			if p.Hidden || !isFigureLabel(p.Name) || centers[strings.TrimSpace(p.Name)] {
				continue
			}
			pv := vec{p.X, p.Y}
			best := math.Inf(1)
			for _, l := range lines {
				if d := pointSegDistance(pv, l.a, l.b); d < best {
					best = d
				}
			}
			if best <= tol {
				continue
			}
			panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
				"点 %s 没有落在任何一条线或坐标轴上（离最近的线约 %s 个单位），图上成了一个孤立的点。"+
					"原图里的标注点都是顶点或交点：若它是某条直线与坐标轴（或另一条直线）的交点，请把坐标改成交点位置；"+
					"否则请把它放到它所在的那条线上。", p.Name, num(best)))
		}
	}
}

// CheckAxisCoincidentSegments 报告"整条线段压在坐标轴上"的情况：
// 这种线段画出来会被轴线完全盖住，等于没画——模型常把没有字母点的直线
// （如 l₃:y=kx+k）退化成沿轴的线段（O→A、O→B）。闭合图形与轴重合的边不算问题。
func CheckAxisCoincidentSegments(panels []Panel) {
	for i := range panels {
		spec := &panels[i].Spec
		if spec.Axes == nil {
			continue
		}
		for j, seg := range spec.Segments {
			if !spec.axisCoincidentSegment(seg) || spec.segmentRedundant(j) {
				continue
			}
			panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
				"线段 %s→%s 与坐标轴完全重合，画出来会被轴线盖住（等于没画）。"+
					"若它代表某条直线（如 l₃），请改成过两个不在轴上的点并加 extend:true；"+
					"若它确实是图形的边，可忽略本条提示。", seg.From, seg.To))
		}
	}
}

// namedLineRe 匹配题干里的直线名称：l₁／l1／L 3 等。
var namedLineRe = regexp.MustCompile(`(?i)l\s*([0-9]|[₀₁₂₃₄₅₆₇₈₉])`)

// subscriptDigits 下标数字 → 普通数字，便于统一比较。
var subscriptDigits = map[rune]string{'₀': "0", '₁': "1", '₂': "2", '₃': "3",
	'₄': "4", '₅': "5", '₆': "6", '₇': "7", '₈': "8", '₉': "9"}

// normalizeLineName 把 "l₁" / "L 3" 统一成 "l3"。
func normalizeLineName(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if d, ok := subscriptDigits[r]; ok {
			b.WriteString(d)
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, b.String())
}

// namedLine 题干点名的直线：norm 用于比对（"l3"），display 保留原样（"l₃"）。
type namedLine struct {
	norm    string
	display string
}

// parseStemLineNames 从题干里取出被点名的直线（l₁、l₂、l₃…），保序去重。
func parseStemLineNames(stemText string) []namedLine {
	matches := namedLineRe.FindAllString(stemText, -1)
	out := make([]namedLine, 0, len(matches))
	seen := make(map[string]bool, len(matches))
	for _, m := range matches {
		n := normalizeLineName(m)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, namedLine{norm: n, display: strings.TrimSpace(m)})
	}
	return out
}

// panelLabelText 汇总子图里所有可能出现直线名称的文字（labels + 点名/点标签），并归一化。
func panelLabelText(spec *Spec) string {
	var b strings.Builder
	for _, l := range spec.Labels {
		b.WriteString(normalizeLineName(l.Text))
		b.WriteByte('\n')
	}
	for _, p := range spec.Points {
		if strings.TrimSpace(p.Label) != "" {
			b.WriteString(normalizeLineName(p.Label))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// countExtendedLines 统计"画成直线"的图元数量（extend 线段 + 曲线图象）。
func countExtendedLines(spec *Spec) (extended, curves int) {
	for _, seg := range spec.Segments {
		if seg.Extend {
			extended++
		}
	}
	return extended, len(spec.Curves)
}

// stemLineCountRe 匹配几何描述里的"两条直线""三条相交直线"。
var stemLineCountRe = regexp.MustCompile(`([一二两三四五六七八九十0-9]+)\s*条\s*(?:相交)?\s*直?线`)

var cnDigits = map[string]int{"一": 1, "二": 2, "两": 2, "三": 3, "四": 4,
	"五": 5, "六": 6, "七": 7, "八": 8, "九": 9, "十": 10}

// requiredNamedLines 从题干点名的直线里筛出"原图确实画了"的那些。
// 关键依据是几何识别阶段对图的描述（figDescription）：题干常在后面小问里引入图上
// 并没有的直线（如"直线 l₃：y=mx+2n-2 与 x 轴交于点 E"），只按题干判定会把模型
// 逼着凭空画出 l₃。描述里点名了该直线、或写明"N 条直线"且覆盖到它的，才要求。
func requiredNamedLines(names []namedLine, figDescription string) []namedLine {
	desc := normalizeLineName(figDescription)
	if desc == "" {
		return nil
	}
	limit := 0
	if m := stemLineCountRe.FindStringSubmatch(figDescription); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			limit = n
		} else if n, ok := cnDigits[m[1]]; ok {
			limit = n
		}
	}
	out := make([]namedLine, 0, len(names))
	for _, n := range names {
		if strings.Contains(desc, n.norm) {
			out = append(out, n)
			continue
		}
		if limit <= 0 || len(n.norm) < 2 {
			continue
		}
		idx, err := strconv.Atoi(n.norm[1:])
		if err == nil && idx <= limit {
			out = append(out, n)
		}
	}
	return out
}

// CheckNamedLines 题干与图形描述都点名了直线 l₁/l₂/l₃ 时，核对"原图画有的直线"是否都画出来了：
//  1. 条数：每条直线都要画，且必须用 extend:true（或 curves 图象）表达成贯穿的直线，
//     只连两个标注点的一截线段不算——这是模型最常见的错误（直线被画成线段）；
//  2. 名称：只有原图上确实印刷了直线名称时才要求在 labels 中写出。题干里的 l₁/l₂
//     往往只是直线的代号，原图并没有印，多画标签会与原图不一致。
//
// 返回值是本轮新增的问题文本。这类约束有"题干点名 + 几何描述确认"双重把关、误报率低，
// 与函数图象过点问题一样值得回喂模型重试（漏画的直线含未知斜率，程序无法确定性补画）。
func CheckNamedLines(panels []Panel, stemText, figDescription string, lineLabels []string) []string {
	names := requiredNamedLines(parseStemLineNames(stemText), figDescription)
	if len(names) == 0 {
		return nil
	}
	printed := printedLineNames(lineLabels)
	all := make([]string, 0, len(names))
	for _, n := range names {
		all = append(all, n.display)
	}
	var added []string
	for i := range panels {
		spec := &panels[i].Spec
		extended, curves := countExtendedLines(spec)
		if drawn := extended + curves; drawn < len(names) {
			msg := fmt.Sprintf(
				"原图共有 %d 条直线（%s），该子图只画出 %d 条贯穿的直线（extend 线段或曲线图象）。"+
					"请对照原图逐条核对：原图画有的每条直线都要画出来——只有两个标注点时也要写好 extend:true，"+
					"没有字母标注点的直线请新增 hidden:true 构造点，取两个不共线于轴的坐标后用 extend:true 连成直线；"+
					"原图确实没有的直线不要添加。",
				len(names), strings.Join(all, "、"), drawn)
			panels[i].Issues = append(panels[i].Issues, msg)
			added = append(added, msg)
		}
		drawn := lineNamesInText(panelLabelText(spec))
		var missing []string
		for _, n := range names {
			if printed[n.norm] && !drawn[n.norm] {
				missing = append(missing, n.display)
			}
		}
		if len(missing) > 0 {
			msg := fmt.Sprintf(
				"原图上印刷了直线名称 %s，但图中没有写出来。请把名称写进 labels（如 {\"text\":\"l₁\"}），"+
					"位置紧贴对应的那条直线；原图上没有印刷的名称不要添加。",
				strings.Join(missing, "、"))
			panels[i].Issues = append(panels[i].Issues, msg)
			added = append(added, msg)
		}
	}
	return added
}

// ---------- 函数图象过点核对 ----------

// functionPointTolRatio 判定"直线经过某标注点/定点"的阈值（占画布短边比例）。
// 4% 对 100x70 画布约 2.8 个单位：能抓住实测中"点偏离直线 10~19 个单位"的明显错误，
// 又不会因坐标取整（1~2 位小数）或文字避让误报。
const functionPointTolRatio = 0.04

// functionOriginTolRatio 判定 y=kx+k "与 x 轴交点贴在原点/落在原点右侧" 的阈值，
// 取 functionPointTolRatio 的一半：交点必须明显在 O 左侧一点才算对，
// 又不能把"正确过 (-1,0) 但斜率很缓"的图形误判成过原点。
const functionOriginTolRatio = 0.5

// functionLineConstraint 题干解析出的"某条函数图象必须满足"的几何约束。
type functionLineConstraint struct {
	line           string   // 归一化直线名，如 l1
	display        string   // 原样名称，如 l₁
	points         []string // 该直线必须经过的标注点（题干"交于 A、B"/"交于点 C"）
	proportional   bool     // 正比例函数（y=mx，常数项为 0），图象必过原点
	fixedNegativeX bool     // y=kx+k 型：恒过 (-1,0)，与 x 轴交点须在 O 负方向且不过 O
}

var (
	// proportionalLineRe 匹配"正比例函数 y=mx 的图象 l₂"。
	proportionalLineRe = regexp.MustCompile(
		`正比例函数\s*y\s*=\s*[a-zA-Z]?\s*x\s*(?:的)?\s*(?:图象|图像|图)?\s*(l\s*[0-9₀-₉])`)
	// selfParamLineRe 匹配"y=kx+k 的图象 l₃"（x 系数与常数项为同一字母）。
	// Go 的 regexp 不支持反向引用，故捕获两个字母后在代码里比较。
	selfParamLineRe = regexp.MustCompile(
		`y\s*=\s*([a-zA-Z])\s*x\s*[+＋]\s*([a-zA-Z])\s*(?:的)?\s*(?:图象|图像|图)?\s*(l\s*[0-9₀-₉])`)
	// lineNameTokenRe2 匹配题干里出现的直线名（l₁ / l1）。
	lineNameTokenRe2 = regexp.MustCompile(`l\s*[0-9₀-₉]`)
	// axisInterceptsRe 匹配"分别与 x，y 轴交于 A，B"/"与 x 轴，y 轴分别交于 A(-4,0)，B"。
	axisInterceptsRe = regexp.MustCompile(
		`(?:分别与\s*x\s*[，,、]?\s*y\s*轴|与\s*x\s*轴\s*[，,、]?\s*y\s*轴\s*分别)\s*交于\s*` +
			`([A-Z])(?:\s*[（(][^）)]*[）)])?\s*[，,、]\s*([A-Z])`)
	// crossPointRe 匹配"交于点 C"。
	crossPointRe = regexp.MustCompile(`交于点\s*([A-Z])`)
)

// splitStemSentences 按句号/分号/换行切句（不按逗号切，避免拆散"x，y 轴""A，B"）。
func splitStemSentences(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return r == '。' || r == '；' || r == ';' || r == '\n'
	})
}

func appendUniqueString(list []string, items ...string) []string {
	for _, it := range items {
		dup := false
		for _, e := range list {
			if e == it {
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

// parseFunctionLineConstraints 从题干解析函数图象的"必须过点"约束。
// 采用保守解析：句式不认识时直接跳过（宁漏勿错），只处理教材/试卷里最常见的表述。
func parseFunctionLineConstraints(stemText string) []functionLineConstraint {
	if strings.TrimSpace(stemText) == "" {
		return nil
	}
	byName := make(map[string]*functionLineConstraint)
	get := func(raw string) *functionLineConstraint {
		n := normalizeLineName(raw)
		if n == "" {
			return nil
		}
		c := byName[n]
		if c == nil {
			c = &functionLineConstraint{line: n, display: strings.TrimSpace(raw)}
			byName[n] = c
		}
		return c
	}
	for _, m := range proportionalLineRe.FindAllStringSubmatch(stemText, -1) {
		if c := get(m[1]); c != nil {
			c.proportional = true
		}
	}
	for _, m := range selfParamLineRe.FindAllStringSubmatch(stemText, -1) {
		if !strings.EqualFold(m[1], m[2]) {
			continue // 不是 y=kx+k 型（系数与常数项不同）
		}
		if c := get(m[3]); c != nil {
			c.fixedNegativeX = true
		}
	}
	for _, sentence := range splitStemSentences(stemText) {
		names := lineNameTokenRe2.FindAllString(sentence, -1)
		if len(names) == 0 {
			continue
		}
		// 轴交点归属句内第一条直线（"直线 l₁ … 分别与 x，y 轴交于 A，B"）。
		if m := axisInterceptsRe.FindStringSubmatch(sentence); m != nil {
			if c := get(names[0]); c != nil {
				c.points = appendUniqueString(c.points, m[1], m[2])
			}
		}
		// "A 与 B 交于点 C"：交点由"交于点"之前最近的两条直线确定。
		// 不能关联句内所有直线——同一句后半段可能又引入别的线（如"…l₂ 交于点 C，
		// 若一次函数 y=kx+k 的图象 l₃ 与…"），把 C 也挂到 l₃ 上会误报。
		for _, loc := range crossPointRe.FindAllStringSubmatchIndex(sentence, -1) {
			prior := lineNameTokenRe2.FindAllString(sentence[:loc[0]], -1)
			if len(prior) > 2 {
				prior = prior[len(prior)-2:]
			}
			pt := sentence[loc[2]:loc[3]]
			for _, n := range prior {
				if c := get(n); c != nil {
					c.points = appendUniqueString(c.points, pt)
				}
			}
		}
	}
	out := make([]functionLineConstraint, 0, len(byName))
	for _, c := range byName {
		if c.proportional || c.fixedNegativeX || len(c.points) > 0 {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].line < out[j].line })
	return out
}

// extendedLineByLabelName 用图上印刷的直线名称标签（l₁/l₂…）找到对应的延长线。
// 标签离所有延长线都太远（或该名称未印刷）时返回 false，调用方跳过校验。
func (s *Spec) extendedLineByLabelName(name string) (line2, bool) {
	want := normalizeLineName(name)
	if want == "" || len(s.Labels) == 0 {
		return line2{}, false
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
		return line2{}, false
	}
	short := math.Min(s.Canvas.Width, s.Canvas.Height)
	if short <= 0 {
		return line2{}, false
	}
	best, bestD := line2{}, math.Inf(1)
	for _, seg := range s.Segments {
		if !seg.Extend {
			continue
		}
		pi, pj := s.pointIndex(seg.From), s.pointIndex(seg.To)
		if pi < 0 || pj < 0 || pi == pj {
			continue
		}
		ln := line2{a: s.pt(pi), b: s.pt(pj)}
		if d := distToLine(lp, ln); d < bestD {
			best, bestD = ln, d
		}
	}
	if bestD > lineLabelTol*short {
		return line2{}, false
	}
	return best, true
}

// xAxisLine 取 x 轴所在直线（原点 → x 轴正方向端点）。
func xAxisLine(s *Spec) (line2, bool) {
	a := s.Axes
	if a == nil {
		return line2{}, false
	}
	oi, xi := s.pointIndex(a.Origin), s.pointIndex(a.X.To)
	if oi < 0 || xi < 0 || oi == xi {
		return line2{}, false
	}
	return line2{a: s.pt(oi), b: s.pt(xi)}, true
}

// CheckFunctionGraphPoints 题干-图形语义核对：函数图象必须经过题干明确要求的点。
// 结构校验只检查"标注点是否落在某条线/轴上"，无法发现"直线该过某个点却没画过"：
// 实测同一道题里 l₁ 应过 A、B 却漏了 B（B 浮在线外 18 个单位），
// l₃:y=kx+k 恒过 (-1,0) 却被画成过原点。这里补上三类低误报的硬约束：
//  1. 正比例函数（y=mx，常数项为 0）的图象必须过原点 O；
//  2. y=kx+k 型渐过 (-1,0)：与 x 轴交点须在 O 负方向，且整条线不得过 O；
//  3. 题干"交于 A、B"/"交于点 C"里的标注点必须真的落在对应的直线上。
//
// 返回值是本轮新增的问题文本，供调用方决定是否回喂模型修正
// （这类约束来自题干解析式，误报率低，值得单独回喂；不计入结构问题）。
func CheckFunctionGraphPoints(panels []Panel, stemText string) []string {
	cons := parseFunctionLineConstraints(stemText)
	if len(cons) == 0 {
		return nil
	}
	added := make([]string, 0, 4)
	add := func(pi int, msg string) {
		panels[pi].Issues = append(panels[pi].Issues, msg)
		added = append(added, msg)
	}
	for i := range panels {
		spec := &panels[i].Spec
		if spec.Axes == nil {
			continue
		}
		short := math.Min(spec.Canvas.Width, spec.Canvas.Height)
		if short <= 0 {
			continue
		}
		tol := functionPointTolRatio * short
		originName := strings.TrimSpace(spec.Axes.Origin)
		for _, c := range cons {
			ln, ok := spec.extendedLineByLabelName(c.line)
			if !ok {
				continue
			}
			// 1) 必须经过的标注点
			var miss []string
			for _, pn := range c.points {
				p, ok := spec.PointByName(pn)
				if !ok || p.Hidden || !isFigureLabel(strings.TrimSpace(p.Name)) {
					continue
				}
				if d := distToLine(vec{X: p.X, Y: p.Y}, ln); d > tol {
					miss = append(miss, fmt.Sprintf("%s（偏离约 %s 个单位）", pn, num(d)))
				}
			}
			if len(miss) > 0 {
				add(i, fmt.Sprintf(
					"题干中直线 %s 经过 %s，但重绘的 %s 没有穿过：%s。"+
						"请按题干给出的解析式/已知点重新推算 %s 的斜率与位置，使它同时经过这些点。",
					c.display, strings.Join(c.points, "、"), c.display,
					strings.Join(miss, "、"), c.display))
			}
			// 2) 正比例函数必过原点
			if c.proportional {
				if o, ok := spec.PointByName(spec.Axes.Origin); ok {
					if d := distToLine(vec{X: o.X, Y: o.Y}, ln); d > tol {
						add(i, fmt.Sprintf(
							"题干中 %s 是正比例函数（y=mx，常数项为 0），图象必须过原点 %s，"+
								"但重绘的 %s 偏离原点约 %s 个单位。请把它改成过原点的直线。",
							c.display, originName, c.display, num(d)))
					}
				}
			}
			// 3) y=kx+k 恒过 (-1,0)，不可能过原点。
			// 这里按"与 x 轴的交点位置"判断，而不是"到原点的垂距"：斜率较缓时
			// 一条正确过 (-1,0) 的直线离原点本来就很近，用垂距会把正确图形判成"过原点"。
			if c.fixedNegativeX {
				t, ok := interceptOffset(spec, ln)
				if !ok {
					continue
				}
				nearTol := functionOriginTolRatio * tol
				if t > nearTol {
					add(i, fmt.Sprintf(
						"题干中 %s 的解析式是 y=kx+k，恒过 (-1,0)，与 x 轴的交点应在原点 %s 的左侧，"+
							"但重绘的 %s 与 x 轴交于 O 的右侧。请把交点改到 O 左侧。",
						c.display, originName, c.display))
				} else if t > -nearTol {
					add(i, fmt.Sprintf(
						"题干中 %s 的解析式是 y=kx+k（x 的系数与常数项相同），它恒过 (-1,0)、不可能过原点，"+
							"但重绘的 %s 却穿过了原点 %s。请按原图把它改成与 x 轴交于 O 左侧一点的直线。",
						c.display, c.display, originName))
				}
			}
		}
	}
	return added
}

// lineNameTokenRe 匹配文本里出现的直线名称（l₁ / l2 / L 3 / 单独的 l）。
var lineNameTokenRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])l\s*([0-9₀₁₂₃₄₅₆₇₈₉]*)`)

// lineNamesInText 提取文本中出现的直线名称集合（归一化为 l1/l2/l 形式）。
// 用集合而不是 strings.Contains：否则 "l" 会被 "l1" 误判为命中。
func lineNamesInText(text string) map[string]bool {
	out := make(map[string]bool, 4)
	for _, m := range lineNameTokenRe.FindAllStringSubmatch(text, -1) {
		if name := normalizeLineName("l" + m[1]); name != "" {
			out[name] = true
		}
	}
	return out
}

// printedLineNames 汇总"原图上确实印刷了"的直线名称集合。
// 只认几何识别阶段专门核对原图后给出的印刷名称（lineLabels）。模型没返回该字段时
// 按"没印刷"处理：图形描述里常混着题干代号（"直线l1：y=kx+b"），拿它反推会让
// 我们在图上补画原图根本没有的名称，而用户明确要求"原图没有的名称不要画"。
func printedLineNames(lineLabels []string) map[string]bool {
	out := make(map[string]bool, len(lineLabels))
	for _, raw := range lineLabels {
		if name := normalizeLineName(raw); name != "" {
			out[name] = true
		}
	}
	return out
}

// PrintedLineNamesConsensus 取两个独立看图阶段对"原图印刷直线名称"的共识：
// 识别阶段（RecognizeGeometry 的 lineLabels）与描述阶段（spec labels 里写出的直线名称）。
// 只有两侧都确认的名称才视为原图印刷，任一侧未确认即按"未印刷"处理：
// 识别阶段偶发误报（把题干代号当成印刷标注）时，描述阶段没画就不该反过来强迫补画；
// 描述阶段照抄题干代号时，识别阶段没确认就不该画上。宁可漏画一个名称，
// 也不能把原图没有的 l₁ 标注画上去（用户明确要求重绘与原图一致）。
func PrintedLineNamesConsensus(lineLabels []string, panels []Panel) []string {
	printed := printedLineNames(lineLabels)
	if len(printed) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(printed))
	out := make([]string, 0, len(printed))
	for i := range panels {
		for _, l := range panels[i].Spec.Labels {
			if !isNamedLineLabel(l.Text) {
				continue
			}
			name := normalizeLineName(l.Text)
			if printed[name] && !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	return out
}

// StripUnprintedLineNames 删除 labels 中"原图并未印刷"的直线名称（l₁/l₂…）。
// 题干常把直线叫作 l₁、l₂，但原图上未必印了这个标注；重绘时多画上去会与原图不一致。
// lineLabels 为识别阶段核对原图后给出的印刷名称，为空表示原图没有印刷名称。
func StripUnprintedLineNames(panels []Panel, lineLabels []string) {
	printed := printedLineNames(lineLabels)
	for i := range panels {
		labels := panels[i].Spec.Labels[:0]
		for _, l := range panels[i].Spec.Labels {
			if isNamedLineLabel(l.Text) && !printed[normalizeLineName(l.Text)] {
				continue
			}
			labels = append(labels, l)
		}
		panels[i].Spec.Labels = labels
	}
}

// ---------- 动点方向约束 ----------

// stemMotionRe 匹配"点D从A点出发……水平向右运动" / "点E从点B出发……竖直向上运动"。
var stemMotionRe = regexp.MustCompile(
	`点\s*([A-Z][0-9]?)\s*从\s*(?:点)?\s*([A-Z][0-9]?)\s*点?\s*出发[^。；\n]{0,40}?(水平|竖直|垂直)\s*(向右|向左|向上|向下)`)

// motionConstraint 题干中的动点运动方向约束。
type motionConstraint struct {
	point, from, dir string
	horizontal       bool
}

// parseStemMotions 解析动点运动方向（同一点去重）。
func parseStemMotions(stemText string) []motionConstraint {
	normalized := stemSubscripts.Replace(stemText)
	var out []motionConstraint
	seen := make(map[string]bool)
	for _, m := range stemMotionRe.FindAllStringSubmatch(normalized, -1) {
		p, from, dir := m[1], m[2], m[4]
		if p == from || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, motionConstraint{
			point: p, from: from, dir: dir,
			horizontal: strings.Contains(m[3], "水平"),
		})
	}
	return out
}

const motionToleranceRatio = 0.06

// CheckMotionConstraints 动点方向核对：题干说"从A点出发水平向右运动"时，
// 动点必须落在过 A 的水平线上，且位于 A 的右侧（竖直向上/向下同理）。
// 模型常把动点随手放到别的边上（例如 D 从 A 水平向右运动，却被画到竖直边 AC 上）。
func CheckMotionConstraints(panels []Panel, stemText string) {
	constraints := parseStemMotions(stemText)
	if len(constraints) == 0 {
		return
	}
	for i := range panels {
		spec := &panels[i].Spec
		for _, c := range constraints {
			p, okP := spec.PointByName(c.point)
			f, okF := spec.PointByName(c.from)
			if !okP || !okF {
				continue
			}
			if c.horizontal {
				tol := math.Max(2, motionToleranceRatio*spec.Canvas.Height)
				if math.Abs(p.Y-f.Y) > tol {
					panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
						"题干说动点 %s 从 %s 出发%s %s运动，但图中 %s 不在过 %s 的水平线上。"+
							"请把 %s 的 y 坐标改成与 %s 相同，再按运动距离确定 x 坐标。",
						c.point, c.from, c.dir, "水平", c.point, c.from, c.point, c.from))
					continue
				}
				if c.dir == "向右" && p.X < f.X-1 || c.dir == "向左" && p.X > f.X+1 {
					panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
						"题干说动点 %s 从 %s 出发水平%s运动，但图中 %s 落在了 %s 的另一侧，方向不对。",
						c.point, c.from, c.dir, c.point, c.from))
				}
				continue
			}
			tol := math.Max(2, motionToleranceRatio*spec.Canvas.Width)
			if math.Abs(p.X-f.X) > tol {
				panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
					"题干说动点 %s 从 %s 出发竖直%s运动，但图中 %s 不在过 %s 的竖直线上。"+
						"请把 %s 的 x 坐标改成与 %s 相同，再按运动距离确定 y 坐标。",
					c.point, c.from, c.dir, c.point, c.from, c.point, c.from))
				continue
			}
			if c.dir == "向上" && p.Y > f.Y+1 || c.dir == "向下" && p.Y < f.Y-1 {
				panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
					"题干说动点 %s 从 %s 出发竖直%s运动，但图中 %s 落在了 %s 的另一侧，方向不对。",
					c.point, c.from, c.dir, c.point, c.from))
			}
		}
	}
}

// ---------- 平行关系 ----------

// stemParallelRe 匹配 "AG∥CE" / "AG//CE" / "AG平行于CE"。
var stemParallelRe = regexp.MustCompile(
	`([A-Z][0-9]?)([A-Z][0-9]?)\s*(?:∥|//|平行于)\s*([A-Z][0-9]?)([A-Z][0-9]?)`)

// stemParallelTolerance 平行核对的角度容忍度（度）。
const stemParallelTolerance = 12.0

// CheckParallelRelations 题干-图形语义核对：题干写明"A∥B"时两条线段必须近似平行。
// 平行关系往往决定交点（如"作AG∥CE交CD于点G"），不平行则 G 的位置必然是错的。
func CheckParallelRelations(panels []Panel, stemText string) {
	normalized := stemSubscripts.Replace(stemText)
	pairs := stemParallelRe.FindAllStringSubmatch(normalized, -1)
	if len(pairs) == 0 {
		return
	}
	for i := range panels {
		spec := &panels[i].Spec
		for _, m := range pairs {
			ok := true
			pts := make([]vec, 4)
			for j, name := range m[1:] {
				p, found := spec.PointByName(name)
				if !found {
					ok = false
					break
				}
				pts[j] = vec{p.X, p.Y}
			}
			if !ok {
				continue
			}
			if d1, d2 := sub(pts[1], pts[0]), sub(pts[3], pts[2]); dist(vec{}, d1) > 1 && dist(vec{}, d2) > 1 {
				cos := (d1.X*d2.X + d1.Y*d2.Y) / (dist(vec{}, d1) * dist(vec{}, d2))
				angle := deg(math.Acos(clampF(cos, -1, 1)))
				if angle > stemParallelTolerance && math.Abs(180-angle) > stemParallelTolerance {
					panels[i].Issues = append(panels[i].Issues, fmt.Sprintf(
						"题干说 %s%s ∥ %s%s，但图中这两条线段夹角约 %.0f°，并不平行。"+
							"请按平行关系重新确定相关点的坐标（尤其是由平行线求出的交点）。",
						m[1], m[2], m[3], m[4], math.Min(angle, 180-angle)))
				}
			}
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
		after := string(runes[i+2 : min(len(runes), i+2+window/3)])
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
