package export

import (
	"bytes"
	"compress/zlib"
	"io"
	"regexp"
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

// TestBuildPDFFirstPageHasContent 回归测试。
//
// 曾因 fpdf 的 GetY/SetY 使用绝对坐标、而分页判断使用相对可用高度，
// 导致首题被误判为放不下而换页，生成一整页空白。
func TestBuildPDFFirstPageHasContent(t *testing.T) {
	fonts := testFonts(t)

	// 长题干使题目位图接近整页高度，正是原先触发误判的场景。
	long := strings.Repeat("这是一道用于验证首页不空白的较长题干内容。", 80)
	items := []renderItem{{item: ExportItem{Subject: "数学", StemText: long}}}

	data, err := buildPDF("t", items, fonts)
	if err != nil {
		t.Fatalf("buildPDF: %v", err)
	}
	if !pdfPageHasDrawing(t, data, 0) {
		t.Fatal("first page has no drawing operations (blank first page)")
	}
}

func TestBuildPDFAllPagesHaveContent(t *testing.T) {
	fonts := testFonts(t)

	items := make([]renderItem, 0, 6)
	for i := 0; i < 6; i++ {
		items = append(items, renderItem{item: ExportItem{
			Subject:  "数学",
			StemText: strings.Repeat("较长的题干内容，用于占满页面。", 30),
		}})
	}

	data, err := buildPDF("t", items, fonts)
	if err != nil {
		t.Fatalf("buildPDF: %v", err)
	}

	pages := pdfPageCount(t, data)
	if pages < 2 {
		t.Fatalf("expected multiple pages, got %d", pages)
	}
	for i := 0; i < pages; i++ {
		if !pdfPageHasDrawing(t, data, i) {
			t.Fatalf("page %d has no drawing operations", i+1)
		}
	}
}

// pdfPageHasDrawing 判断第 idx 页（从 0 开始）的内容流中是否有 XObject 绘制操作。
func pdfPageHasDrawing(t *testing.T, data []byte, idx int) bool {
	t.Helper()

	objects := pdfObjects(data)

	var pagesBody []byte
	for _, body := range objects {
		if bytes.Contains(body, []byte("/Type /Pages")) {
			pagesBody = body
			break
		}
	}
	if pagesBody == nil {
		t.Fatal("no /Pages object found in pdf")
	}

	kids := regexp.MustCompile(`(?s)/Kids\s*\[([^\]]*)\]`).FindSubmatch(pagesBody)
	if kids == nil {
		t.Fatal("no /Kids entry found in pdf")
	}
	refs := regexp.MustCompile(`(\d+)\s+0\s+R`).FindAllSubmatch(kids[1], -1)
	if idx >= len(refs) {
		t.Fatalf("page index %d out of range (have %d pages)", idx, len(refs))
	}

	pageBody := objects[string(refs[idx][1])]
	if pageBody == nil {
		t.Fatalf("page object %s not found", refs[idx][1])
	}

	contents := regexp.MustCompile(`/Contents\s+(\d+)\s+0\s+R`).FindSubmatch(pageBody)
	if contents == nil {
		return false
	}
	return streamHasDrawingOp(objects[string(contents[1])])
}

// pdfObjects 解析 PDF 间接对象（fpdf 输出的对象字典未压缩）。
func pdfObjects(data []byte) map[string][]byte {
	out := make(map[string][]byte)
	re := regexp.MustCompile(`(?s)(\d+)\s+0\s+obj(.*?)endobj`)
	for _, m := range re.FindAllSubmatch(data, -1) {
		out[string(m[1])] = m[2]
	}
	return out
}

// streamHasDrawingOp 解压对象流并判断是否包含 XObject 绘制操作（Do）。
func streamHasDrawingOp(body []byte) bool {
	m := regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`).FindSubmatch(body)
	if m == nil {
		return false
	}

	content := m[1]
	if zr, err := zlib.NewReader(bytes.NewReader(m[1])); err == nil {
		if decoded, err := io.ReadAll(zr); err == nil {
			content = decoded
		}
		_ = zr.Close()
	}
	return bytes.Contains(content, []byte(" Do"))
}
