package geom

// 文字宽度启发式估算：零依赖、不需要字体文件、不做文字转路径。
//
// SVG 里的文字由客户端渲染字形，服务端只需要一个足够准的近似宽度来做避让计算，
// 因此按字符类估算即可（CJK/全角约 1em、ASCII 字母数字约 0.55em、标点/空格另计）。
const (
	widthCJK   = 1.0
	widthASCII = 0.55
	widthPunct = 0.45
	widthSpace = 0.30
	widthOther = 0.60
)

// MeasureTextWidth 估算 text 在字号 size 下的宽度（画布单位）。
func MeasureTextWidth(text string, size float64) float64 {
	if text == "" || !isFinite(size) || size <= 0 {
		return 0
	}
	var total float64
	for _, r := range text {
		total += widthFactor(r)
	}
	return total * size
}

// widthFactor 返回单个字符相对字号的宽度系数。
func widthFactor(r rune) float64 {
	switch {
	case r == '\t' || r == ' ' || r == '\u00A0' || r == '\u3000':
		return widthSpace
	case r < 0x20: // 控制字符不占宽
		return 0
	case r >= 0x2E80: // CJK 汉字、全角标点、多数几何/数学符号、表情
		return widthCJK
	case r >= '0' && r <= '9', r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		return widthASCII
	case r < 0x80: // ASCII 标点与符号
		return widthPunct
	default: // 拉丁补充、希腊字母等
		return widthOther
	}
}
