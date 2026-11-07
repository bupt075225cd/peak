package export

import (
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// synthGlyph 主字体缺字时的合成回退描述：用主字体绘制 base 字符，
// 按比例缩放、按 dyEm 偏移基线（单位：字号比例，正数向下）。
type synthGlyph struct {
	base  string  // 用于绘制的替代字符（可为多字符，如分数 "1/3"）
	scale float64 // 相对当前字号的比例
	dyEm  float64 // 基线偏移（字号比例）
}

// synthGlyphs 内置字体缺失、可合成替代的字形映射。
//
// 内置 Noto Sans SC 子集不含下标/上标数字与部分分数（数学题干常用 "C₁D₁"），
// 而 Noto CJK 全量字体同样不含这些码位；直接绘制会得到空白。这里用对应数字
// 缩小/偏移基线合成，视觉上等同真下标/上标。
var synthGlyphs = map[rune]synthGlyph{
	// 下标 ₀-₉（U+2080..U+2089）与下标加/减号。
	0x2080: {base: "0", scale: 0.6, dyEm: 0.20},
	0x2081: {base: "1", scale: 0.6, dyEm: 0.20},
	0x2082: {base: "2", scale: 0.6, dyEm: 0.20},
	0x2083: {base: "3", scale: 0.6, dyEm: 0.20},
	0x2084: {base: "4", scale: 0.6, dyEm: 0.20},
	0x2085: {base: "5", scale: 0.6, dyEm: 0.20},
	0x2086: {base: "6", scale: 0.6, dyEm: 0.20},
	0x2087: {base: "7", scale: 0.6, dyEm: 0.20},
	0x2088: {base: "8", scale: 0.6, dyEm: 0.20},
	0x2089: {base: "9", scale: 0.6, dyEm: 0.20},
	0x208A: {base: "+", scale: 0.6, dyEm: 0.20},
	0x208B: {base: "-", scale: 0.6, dyEm: 0.20},
	// 上标 ⁰、⁴-⁹、⁺、⁻、ⁿ（¹²³ 为 Latin-1，内置字体已有）。
	0x2070: {base: "0", scale: 0.6, dyEm: -0.32},
	0x2074: {base: "4", scale: 0.6, dyEm: -0.32},
	0x2075: {base: "5", scale: 0.6, dyEm: -0.32},
	0x2076: {base: "6", scale: 0.6, dyEm: -0.32},
	0x2077: {base: "7", scale: 0.6, dyEm: -0.32},
	0x2078: {base: "8", scale: 0.6, dyEm: -0.32},
	0x2079: {base: "9", scale: 0.6, dyEm: -0.32},
	0x207A: {base: "+", scale: 0.6, dyEm: -0.32},
	0x207B: {base: "-", scale: 0.6, dyEm: -0.32},
	0x207F: {base: "n", scale: 0.6, dyEm: -0.32},
	// 分数（½¼¾ 为 Latin-1，内置字体已有；这几个缺）。
	0x2153: {base: "1/3", scale: 0.6, dyEm: 0},
	0x2154: {base: "2/3", scale: 0.6, dyEm: 0},
	0x215B: {base: "1/8", scale: 0.6, dyEm: 0},
	0x215C: {base: "3/8", scale: 0.6, dyEm: 0},
	0x215D: {base: "5/8", scale: 0.6, dyEm: 0},
	0x215E: {base: "7/8", scale: 0.6, dyEm: 0},
}

// synthFor 返回字符的合成回退描述；不可合成的字符返回 false。
func synthFor(r rune) (synthGlyph, bool) {
	g, ok := synthGlyphs[r]
	return g, ok
}

// Missing 判断字符在内置字体中是否缺字形。结果按字符缓存。
func (p *FontProvider) Missing(r rune) bool {
	p.checkMu.Lock()
	v, ok := p.missing[r]
	p.checkMu.Unlock()
	if ok {
		return v
	}

	missing := p.computeMissing(r)
	p.checkMu.Lock()
	p.missing[r] = missing
	p.checkMu.Unlock()
	return missing
}

// computeMissing 用固定校验字号探测字形是否存在。
func (p *FontProvider) computeMissing(r rune) bool {
	p.checkMu.Lock()
	if p.checkFace == nil {
		f, err := opentype.NewFace(p.sfnt, &opentype.FaceOptions{
			Size:    20,
			DPI:     72,
			Hinting: font.HintingNone,
		})
		if err == nil {
			p.checkFace = f
		}
	}
	f := p.checkFace
	p.checkMu.Unlock()

	if f == nil {
		return false // 无法探测时按"有字形"处理，维持原渲染行为
	}
	_, ok := f.GlyphAdvance(r)
	return !ok
}

// textSegment 切分后的文本段：主字体段（scale=1）或合成段（缩放 + 基线偏移）。
type textSegment struct {
	text  string
	face  font.Face // 合成段使用的缩放 face；主段为 nil（调用方持有主 face）
	scale float64
	dyEm  float64
}

// splitTextSegments 把文本切成"主字体段 + 合成段"序列。
//
// 主字体已有字形（或不可合成）的字符进主段；缺字且可合成的字符按 synthGlyph
// 生成缩放段。同属性相邻字符合并为一段，减少绘制调用次数。
func splitTextSegments(fonts *FontProvider, size float64, s string) []textSegment {
	out := make([]textSegment, 0, 4)
	var run strings.Builder
	flush := func() {
		if run.Len() > 0 {
			out = append(out, textSegment{text: run.String(), scale: 1})
			run.Reset()
		}
	}
	for _, r := range s {
		g, ok := synthFor(r)
		if !ok || !fonts.Missing(r) {
			run.WriteRune(r)
			continue
		}
		flush()
		seg := textSegment{text: g.base, scale: g.scale, dyEm: g.dyEm}
		if f, err := fonts.Face(size * g.scale); err == nil {
			seg.face = f
		} else {
			// 缩放 face 创建失败时退回主字体原样绘制，避免整题渲染失败。
			seg.scale, seg.dyEm = 1, 0
		}
		out = append(out, seg)
	}
	flush()
	return out
}
