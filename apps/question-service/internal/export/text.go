package export

import (
	"strings"
	"unicode"
)

// normalizeStemText 归一化题干文本。
//
// 识别阶段会把源文档的断行原样带进题干（识别服务用 "\n" 拼接文本块，模型输出也可能
// 含换行）。这些换行位置与导出页宽无关，按原样断行会导致"一行没排满就换行、整段挤在
// 左侧"。merge 为 true 时把换行与连续空白折叠为单个空格，交由渲染器按正文宽度统一
// 重排；merge 为 false 时保留原有换行，仅去除首尾空白。
//
// 折叠换行时，仅当断行两侧都不是中文/中文标点才补空格：中文之间补空格会显得突兀
// （如"4。 求"），而英文单词之间必须保留分隔（如"hello\nworld"）。
func normalizeStemText(raw string, merge bool) string {
	if !merge {
		return strings.TrimSpace(raw)
	}

	var sb strings.Builder
	sb.Grow(len(raw))

	var (
		prev           rune
		pendingSpace   bool
		pendingNewline bool
	)

	for _, r := range raw {
		if isStemSpace(r) {
			pendingSpace = true
			if r == '\n' || r == '\r' {
				pendingNewline = true
			}
			continue
		}
		if pendingSpace && sb.Len() > 0 {
			if !pendingNewline || (!isCJK(prev) && !isCJK(r)) {
				sb.WriteByte(' ')
			}
		}
		pendingSpace = false
		pendingNewline = false
		sb.WriteRune(r)
		prev = r
	}
	return sb.String()
}

// isStemSpace 判断是否为需要折叠的空白字符（含全角空格与不换行空格）。
func isStemSpace(r rune) bool {
	switch r {
	case '\n', '\r', '\t', '\v', '\f', ' ', '\u00a0', '\u3000':
		return true
	default:
		return false
	}
}

// isCJK 判断是否为中日韩文字或中文常用标点：中文之间断行无需补空格。
func isCJK(r rune) bool {
	switch {
	case unicode.Is(unicode.Han, r):
		return true
	case r >= 0x3000 && r <= 0x303F: // CJK 标点（。、，「」等）
		return true
	case r >= 0xFF00 && r <= 0xFFEF: // 全角符号（，；：？！等）
		return true
	case r == '\u2018' || r == '\u2019' || r == '\u201c' || r == '\u201d' ||
		r == '\u2014' || r == '\u2026' || r == '\u00b7':
		return true
	default:
		return false
	}
}
