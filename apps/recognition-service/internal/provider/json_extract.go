package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

// extractJSON 从模型输出中提取 JSON 文本。
//
// 模型的返回常常不是"干净的 JSON 对象"，这里按以下顺序容忍：
//  1. ```json 代码块包裹、代码块前后夹带解释文字；
//  2. 正文前后有多段花括号内容（示例、说明）——只取第一个花括号配对完整的对象；
//  3. 尾随逗号、非法反斜杠转义（如数学文本里直接写 \frac、\angle）；
//  4. 被截断的半截 JSON（括号/引号未闭合）——补全后仍能校验通过时才采用。
func extractJSON(text string) (string, error) {
	t := strings.TrimSpace(text)
	if t == "" {
		return "", fmt.Errorf("model output is empty")
	}
	for _, cand := range jsonCandidates(t) {
		obj := scanJSONObject(cand)
		if obj == "" {
			continue
		}
		if json.Valid([]byte(obj)) {
			return obj, nil
		}
		if fixed, ok := repairJSON(obj); ok {
			return fixed, nil
		}
	}
	if strings.Contains(t, "{") {
		return "", fmt.Errorf("model output is not valid JSON")
	}
	return "", fmt.Errorf("model output contains no JSON object")
}

// jsonCandidates 给出可能承载 JSON 的文本片段：先各代码块，再整段原文。
func jsonCandidates(t string) []string {
	out := make([]string, 0, 3)
	if strings.Contains(t, "```") {
		for _, part := range strings.Split(t, "```") {
			p := strings.TrimSpace(part)
			if len(p) >= 4 && (strings.HasPrefix(p, "json") || strings.HasPrefix(p, "JSON")) {
				p = strings.TrimSpace(p[4:])
			}
			if strings.Contains(p, "{") {
				out = append(out, p)
			}
		}
	}
	if !strings.Contains(t, "{") {
		return out
	}
	return append(out, t)
}

// scanJSONObject 从文本里取出第一个花括号配对完整的对象。
// 字符串内部的括号与转义不参与配对；未闭合（多半被截断）时返回剩余全文，交给 repairJSON 补齐。
func scanJSONObject(t string) string {
	start := strings.IndexByte(t, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inStr, esc := false, false
	for i := start; i < len(t); i++ {
		c := t[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			switch c {
			case '\\':
				esc = true
			case '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return t[start : i+1]
			}
		}
	}
	return t[start:]
}

// repairJSON 尝试修复常见的 JSON 书写问题，修复后仍不合法则返回 false。
func repairJSON(obj string) (string, bool) {
	fixed := closeTruncatedJSON(obj)
	fixed = dropTrailingCommas(fixed)
	fixed = escapeInvalidBackslashes(fixed)
	if !json.Valid([]byte(fixed)) {
		return "", false
	}
	return fixed, true
}

// closeTruncatedJSON 补齐被截断时未闭合的字符串与括号。
func closeTruncatedJSON(s string) string {
	var stack []byte
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			switch c {
			case '\\':
				esc = true
			case '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{', '[':
			stack = append(stack, c)
		case '}', ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	// 截断停在半个转义符上：补一个反斜杠让它成为合法转义。
	if esc {
		s += `\`
	}
	if inStr {
		s += `"`
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == '{' {
			s += "}"
		} else {
			s += "]"
		}
	}
	return s
}

// dropTrailingCommas 去掉对象/数组末尾多余逗号（跳过字符串内容）。
func dropTrailingCommas(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			b.WriteByte(c)
			if esc {
				esc = false
				continue
			}
			switch c {
			case '\\':
				esc = true
			case '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
			b.WriteByte(c)
		case ',':
			j := i + 1
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\r' || s[j] == '\n') {
				j++
			}
			if j < len(s) && (s[j] == '}' || s[j] == ']') {
				continue // 丢弃尾随逗号
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// escapeInvalidBackslashes 把字符串里非法的反斜杠转义（如 \f 之外的 \a、\c、\l）转义化，
// 模型直接把 LaTeX（\angle、\frac、\left）写进 JSON 时很常见。
func escapeInvalidBackslashes(s string) string {
	const validEscapes = `"\/bfnrtu`
	var b strings.Builder
	b.Grow(len(s))
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !inStr {
			if c == '"' {
				inStr = true
			}
			b.WriteByte(c)
			continue
		}
		if esc {
			esc = false
			b.WriteByte(c)
			continue
		}
		if c == '\\' {
			var next byte
			if i+1 < len(s) {
				next = s[i+1]
			}
			if strings.IndexByte(validEscapes, next) >= 0 {
				b.WriteByte(c)
				esc = true
				continue
			}
			b.WriteString(`\\`)
			continue
		}
		if c == '"' {
			inStr = false
		}
		b.WriteByte(c)
	}
	return b.String()
}
