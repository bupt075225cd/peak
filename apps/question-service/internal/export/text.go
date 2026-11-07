package export

import (
	"regexp"
	"strings"
	"unicode"
)

// stemQuestionMarkerRe 以小问编号开头的行：(1)、（1）、①⑧㈠ 等。
// 这类行之前的换行是结构边界（题干分小问），折叠时必须保留。
// 注意不含 "1." 裸编号：避免把正文里的 "1.5 倍" 等数字误判为小问标记
// （生产存量数据中的小问编号均为括号/圈号形式）。
var stemQuestionMarkerRe = regexp.MustCompile(`^\s*(\([0-9]{1,2}\)|（[0-9一二三四五六七八九十]{1,3}）|[①-⑳㈠-㈩])`)

// normalizeStemText 归一化题干文本。
//
// 识别阶段会把源试卷的断行原样带进题干（识别服务用 "\n" 拼接文本块，模型输出
// 也可能含换行）。这些换行分两类，折叠时区别对待：
//   - 结构性换行（保留）：小问编号行之前、Markdown 表格内部、空行分段——
//     它们承载题干的语义结构；
//   - 版式性换行（折叠）：原卷按纸面宽度折行的痕迹，切在句子中间，与导出
//     页宽无关——保留会导致"一行没排满就换行"，折叠后交由渲染器按正文宽度
//     统一重排。
//
// 折叠段内换行时，仅当断行两侧都不是中文/中文标点才补空格：中文之间补空格会
// 显得突兀（如"4。 求"），而英文单词之间必须保留分隔（如"hello\nworld"）。
func normalizeStemText(raw string, merge bool) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !merge {
		return strings.TrimSpace(raw)
	}

	lines := strings.Split(raw, "\n")
	inTable := tableLineMask(lines)

	var (
		out []string
		buf []string
	)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		if s := collapseStemWhitespace(strings.Join(buf, "\n")); s != "" {
			out = append(out, s)
		}
		buf = buf[:0]
	}
	for i, ln := range lines {
		switch {
		case inTable[i]:
			// 表格行逐行保留：折叠成一行后行列关系就没了，导出时无法还原成表格。
			flush()
			if s := strings.TrimSpace(ln); s != "" {
				out = append(out, s)
			}
		case strings.TrimSpace(ln) == "":
			// 空行是段落分隔，保留（连续空行压缩为一个）。
			flush()
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
		case i+1 < len(lines) && stemQuestionMarkerRe.MatchString(lines[i+1]):
			// 下一行是小问编号：本行与其后的换行是结构边界，保留。
			buf = append(buf, ln)
			flush()
		default:
			buf = append(buf, ln)
		}
	}
	flush()
	return strings.Join(out, "\n")
}

// collapseStemWhitespace 把换行与连续空白折叠为单个空格；中文之间的断行不补空格。
func collapseStemWhitespace(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
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

// stemTable 题干里的一张表格（Markdown 表格：表头行 + |---| 分隔行 + 数据行）。
type stemTable struct {
	header []string
	rows   [][]string
}

// columnCount 返回表格列数（表头与数据行取最大值）。
func (t *stemTable) columnCount() int {
	cols := len(t.header)
	for _, r := range t.rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	return cols
}

// cell 返回第 row 行第 col 列的单元格文本；越界返回空串。
// row = -1 表示表头行。
func (t *stemTable) cell(row, col int) string {
	cells := t.header
	if row >= 0 {
		if row >= len(t.rows) {
			return ""
		}
		cells = t.rows[row]
	}
	if col < 0 || col >= len(cells) {
		return ""
	}
	return cells[col]
}

// rowCount 表格总行数（含表头）。
func (t *stemTable) rowCount() int {
	n := len(t.rows)
	if len(t.header) > 0 {
		n++
	}
	return n
}

// stemBlock 题干解析后的块：要么是普通段落（text），要么是一张表格（table）。
type stemBlock struct {
	text  string
	table *stemTable
}

// parseStemBlocks 把题干切成"段落 + 表格"块序列，供 PDF 位图渲染与 DOCX 生成共用。
// 段落已由 normalizeStemText 折叠过换行，表格按 Markdown 语法还原成行列。
func parseStemBlocks(text string) []stemBlock {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	inTable := tableLineMask(lines)

	blocks := make([]stemBlock, 0, 4)
	var para []string
	flushPara := func() {
		if len(para) == 0 {
			return
		}
		if s := strings.TrimSpace(strings.Join(para, "\n")); s != "" {
			blocks = append(blocks, stemBlock{text: s})
		}
		para = nil
	}
	for i := 0; i < len(lines); {
		if !inTable[i] {
			para = append(para, lines[i])
			i++
			continue
		}
		flushPara()
		tbl := &stemTable{header: splitTableRow(lines[i])}
		j := i + 2 // 跳过 |---| 分隔行
		for j < len(lines) && inTable[j] {
			tbl.rows = append(tbl.rows, splitTableRow(lines[j]))
			j++
		}
		blocks = append(blocks, stemBlock{table: tbl})
		i = j
	}
	flushPara()
	return blocks
}

// tableLineMask 标记哪些行属于 Markdown 表格块。
// 表格以"表头行 + 分隔行"开头（分隔行是 Markdown 表格的硬性语法），后续连续的 | 行都算表内。
func tableLineMask(lines []string) []bool {
	mask := make([]bool, len(lines))
	for i := 0; i+1 < len(lines); i++ {
		if mask[i] || !isTableRow(lines[i]) || !isTableSeparator(lines[i+1]) {
			continue
		}
		mask[i] = true
		mask[i+1] = true
		for j := i + 2; j < len(lines) && isTableRow(lines[j]); j++ {
			mask[j] = true
		}
	}
	return mask
}

// isTableRow 判断是否为表格行：以 | 开头并结尾（首尾竖线可省略一侧，但必须含分隔符）。
func isTableRow(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" || !strings.Contains(s, "|") {
		return false
	}
	return strings.HasPrefix(s, "|") || strings.HasSuffix(s, "|")
}

// isTableSeparator 判断是否为 Markdown 表格的分隔行，如 |---|:--:|
func isTableSeparator(line string) bool {
	s := strings.TrimSpace(line)
	if !strings.Contains(s, "|") || !strings.Contains(s, "-") {
		return false
	}
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	for _, part := range strings.Split(s, "|") {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, ":")
		part = strings.TrimSuffix(part, ":")
		if part == "" {
			return false
		}
		for _, r := range part {
			if r != '-' {
				return false
			}
		}
	}
	return true
}

// splitTableRow 拆分表格行为单元格文本（去掉首尾竖线、还原 \| 转义）。
func splitTableRow(line string) []string {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	parts := strings.Split(s, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ReplaceAll(strings.TrimSpace(p), `\|`, "|")
		out = append(out, p)
	}
	return out
}
