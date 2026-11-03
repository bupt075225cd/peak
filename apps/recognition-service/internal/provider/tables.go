package provider

import (
	"regexp"
	"strconv"
	"strings"
)

// tableIDRe 表格占位名格式：table1、table2…
var tableIDRe = regexp.MustCompile(`^table\d+$`)

// tablePlaceholderRe 匹配题干里的表格占位符 {{table1}}（允许花括号内有空白）。
var tablePlaceholderRe = regexp.MustCompile(`\{\{\s*(table\d+)\s*\}\}`)

// TablePlaceholder 返回表格在题干文本中的占位符写法（{{table1}}）。
// 前端渲染、保存序列化都用同一格式，避免各处各写一套拼接逻辑。
func TablePlaceholder(id string) string {
	return "{{" + id + "}}"
}

// normalizeTables 校正表格数据与题干占位符：
//   - 丢掉完全空白的表格；单元格去首尾空白，数据行按表头列数补齐空格子；
//   - id 缺失/重复/非法时按"题干里已出现的占位符 → table1、table2…"补全；
//   - 模型给了表格却漏写占位符时，把占位符补到题干末尾——否则前端和导出都会丢掉这张表。
func normalizeTables(text string, tables []Table) (string, []Table) {
	if len(tables) == 0 {
		return text, nil
	}
	placeholders := tablePlaceholderRe.FindAllStringSubmatch(text, -1)
	out := make([]Table, 0, len(tables))
	seen := make(map[string]bool, len(tables))

	for i, t := range tables {
		t.Headers = cleanCells(t.Headers)
		rows := make([][]string, 0, len(t.Rows))
		for _, r := range t.Rows {
			if cells := cleanCells(r); len(cells) > 0 {
				rows = append(rows, cells)
			}
		}
		t.Rows = rows
		if len(t.Headers) == 0 && len(t.Rows) == 0 {
			continue // 空表（模型常为"没有表格"输出一行空对象）
		}

		cols := len(t.Headers)
		for _, r := range t.Rows {
			if len(r) > cols {
				cols = len(r)
			}
		}
		if len(t.Headers) > 0 {
			t.Headers = padCells(t.Headers, cols)
		}
		for j := range t.Rows {
			t.Rows[j] = padCells(t.Rows[j], cols)
		}

		id := strings.TrimSpace(t.ID)
		if !tableIDRe.MatchString(id) || seen[id] {
			// 优先沿用题干里同序号的占位符，避免"表 id 为空"时与原占位错配。
			if i < len(placeholders) && !seen[placeholders[i][1]] {
				id = placeholders[i][1]
			} else {
				id = nextTableID(seen)
			}
		}
		t.ID = id
		seen[id] = true
		out = append(out, t)
	}
	if len(out) == 0 {
		return text, nil
	}

	// 模型给了表格却漏写占位符：补到题干末尾，保证表格不会在渲染/导出时丢失。
	for _, t := range out {
		if strings.Contains(text, TablePlaceholder(t.ID)) {
			continue
		}
		text = strings.TrimRight(text, "\n")
		if text != "" {
			text += "\n"
		}
		text += TablePlaceholder(t.ID)
	}
	// 占位符必须独占一行：表格要作为独立块渲染与导出，粘在句子里（"如表{{table1}}所示"）
	// 替换成 Markdown 后，表头会带上"如表"、最后一行会带上"所示"，行列直接错位。
	for _, t := range out {
		text = placePlaceholderOwnLine(text, t.ID)
	}
	return text, out
}

// placePlaceholderOwnLine 把占位符规范成独占一行，并压缩因此产生的连续空行。
func placePlaceholderOwnLine(text, id string) string {
	ph := TablePlaceholder(id)
	if !strings.Contains(text, ph) {
		return text
	}
	re := regexp.MustCompile(`[ \t]*` + regexp.QuoteMeta(ph) + `[ \t]*`)
	text = re.ReplaceAllString(text, "\n"+ph+"\n")
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(text)
}

// cleanCells 去掉每个单元格的首尾空白，丢弃纯空白单元格行。
func cleanCells(cells []string) []string {
	out := make([]string, 0, len(cells))
	for _, c := range cells {
		out = append(out, strings.TrimSpace(c))
	}
	if len(out) == 0 {
		return nil
	}
	for _, c := range out {
		if c != "" {
			return out
		}
	}
	return nil
}

// padCells 把一行单元格补齐到 cols 个（多出的保留，交由渲染侧按最宽行列数处理）。
func padCells(cells []string, cols int) []string {
	for len(cells) < cols {
		cells = append(cells, "")
	}
	return cells
}

// nextTableID 返回尚未占用的最小 tableN。
func nextTableID(used map[string]bool) string {
	for n := 1; ; n++ {
		id := "table" + strconv.Itoa(n)
		if !used[id] {
			return id
		}
	}
}
