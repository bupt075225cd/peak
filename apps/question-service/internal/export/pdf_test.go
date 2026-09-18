package export

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func TestBuildPDFProducesValidDocument(t *testing.T) {
	fonts := testFonts(t)
	items := []renderItem{{
		item: ExportItem{
			Grade: "七年级上", Subject: "数学", QuestionType: "解答题",
			StemText: "已知 x^2 = 4，求 x。",
		},
	}}

	data, err := buildPDF("我的错题本 2026-09-17", items, fonts)
	if err != nil {
		t.Fatalf("buildPDF: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("missing pdf header, got %q", data[:min(8, len(data))])
	}
	if !bytes.Contains(data, []byte("%%EOF")) {
		t.Fatal("missing pdf eof marker")
	}
}

func TestBuildPDFPaginatesManyItems(t *testing.T) {
	fonts := testFonts(t)

	long := strings.Repeat("这是一道用于分页验证的较长题干内容。", 12)
	items := make([]renderItem, 0, 12)
	for i := 0; i < 12; i++ {
		items = append(items, renderItem{item: ExportItem{Subject: "数学", StemText: long}})
	}

	data, err := buildPDF("t", items, fonts)
	if err != nil {
		t.Fatalf("buildPDF: %v", err)
	}
	if count := pdfPageCount(t, data); count < 2 {
		t.Fatalf("expected multiple pages, got %d", count)
	}
}

func TestBuildPDFRejectsEmptyItems(t *testing.T) {
	if _, err := buildPDF("t", nil, testFonts(t)); err == nil {
		t.Fatal("expected error for empty items")
	}
}

func TestBuildPDFRequiresFontProvider(t *testing.T) {
	items := []renderItem{{item: ExportItem{StemText: "x"}}}
	if _, err := buildPDF("t", items, nil); err == nil {
		t.Fatal("expected error when font provider is nil")
	}
}

// pdfPageCount 从 PDF 的 Pages 对象字典中读取页数（对象字典不会被压缩）。
func pdfPageCount(t *testing.T, data []byte) int {
	t.Helper()

	idx := bytes.Index(data, []byte("/Count "))
	if idx < 0 {
		t.Fatal("no /Count entry found in pdf")
	}
	rest := data[idx+len("/Count "):]

	end := bytes.IndexAny(rest, " \n\r/>")
	if end < 0 {
		end = len(rest)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(rest[:end])))
	if err != nil {
		t.Fatalf("parse page count: %v", err)
	}
	return n
}
