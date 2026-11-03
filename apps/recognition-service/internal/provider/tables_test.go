package provider

import (
	"strings"
	"testing"
)

// TestParseQuestionTables 表格从题干正文摘出、按行列结构化，单元格去首尾空白。
func TestParseQuestionTables(t *testing.T) {
	out := `{"text":"如表{{table1}}所示：\n(1) 求y与x的关系式","subject":"数学","question_type":"解答题",` +
		`"tables":[{"id":"table1","headers":["计费档","用水量x","单价"],` +
		`"rows":[["第一档","0<x≤220","3.5"],["第二档"," 220<x≤300 ","5.0"]]}]}`

	res := parseQuestion(out)
	if len(res.Tables) != 1 {
		t.Fatalf("tables = %d, want 1", len(res.Tables))
	}
	tbl := res.Tables[0]
	if tbl.ID != "table1" {
		t.Fatalf("id = %q", tbl.ID)
	}
	if len(tbl.Headers) != 3 || tbl.Headers[1] != "用水量x" {
		t.Fatalf("headers = %v", tbl.Headers)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(tbl.Rows))
	}
	if got := tbl.Rows[1][1]; got != "220<x≤300" {
		t.Fatalf("cell not trimmed: %q", got)
	}
	if !strings.Contains(res.Text, "{{table1}}") {
		t.Fatalf("placeholder lost: %q", res.Text)
	}
}

// TestNormalizeTablesAddsMissingPlaceholder 模型给了表却漏写占位符时，补到题干末尾，
// 否则前端与导出都会丢掉这张表。
func TestNormalizeTablesAddsMissingPlaceholder(t *testing.T) {
	text, tables := normalizeTables("题干没有占位符", []Table{{
		Headers: []string{"a", "b"},
		Rows:    [][]string{{"1"}}, // 少一列，应补齐到 2 列
	}})
	if len(tables) != 1 || tables[0].ID != "table1" {
		t.Fatalf("tables = %+v", tables)
	}
	if !strings.Contains(text, "{{table1}}") {
		t.Fatalf("placeholder not appended: %q", text)
	}
	if got := len(tables[0].Rows[0]); got != 2 {
		t.Fatalf("row cells = %d, want padded to 2", got)
	}
}

// TestNormalizeTablesKeepsExistingPlaceholder 模型自带 {{table1}} 时不要重复追加。
func TestNormalizeTablesKeepsExistingPlaceholder(t *testing.T) {
	text, _ := normalizeTables("如表{{table1}}所示", []Table{{
		Headers: []string{"a"},
		Rows:    [][]string{{"1"}},
	}})
	if strings.Count(text, "{{table1}}") != 1 {
		t.Fatalf("duplicated placeholder: %q", text)
	}
}

// TestNormalizeTablesPlaceholderOwnLine 占位符粘在句子里（"如表{{table1}}所示"）时要拆到独立行，
// 否则替换成 Markdown 表格后表头会带上"如表"、末行带上"所示"，行列直接错位。
func TestNormalizeTablesPlaceholderOwnLine(t *testing.T) {
	text, _ := normalizeTables("如表{{table1}}所示，求水费", []Table{{
		Headers: []string{"档", "价"},
		Rows:    [][]string{{"一档", "3.5"}},
	}})
	lines := strings.Split(text, "\n")
	if len(lines) != 3 || lines[0] != "如表" || lines[1] != "{{table1}}" || lines[2] != "所示，求水费" {
		t.Fatalf("lines = %q", lines)
	}
}

// TestNormalizeTablesDropsEmpty 空表直接丢弃（模型常用空对象表示"没有表格"）。
func TestNormalizeTablesDropsEmpty(t *testing.T) {
	text, tables := normalizeTables("只有文字题", []Table{{ID: "table1"}})
	if len(tables) != 0 {
		t.Fatalf("tables = %+v, want none", tables)
	}
	if text != "只有文字题" {
		t.Fatalf("text = %q", text)
	}
}
