package export

import (
	"bytes"
	"compress/zlib"
	"image/png"
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

func TestSplitItemImageSplitsTallBitmapAtSafeBreaks(t *testing.T) {
	// 高度超过单页容量的位图应被切成多片，且总高度不变。
	tall := pdfStripHeightPx*2 + 100
	data := encodePNG(t, 40, tall)

	// 模拟文字行/配图边界：每 300px 一个安全断点。
	breaks := make([]int, 0, tall/300)
	for y := 300; y < tall; y += 300 {
		breaks = append(breaks, y)
	}

	parts, err := splitItemImage(data, 40, tall, breaks)
	if err != nil {
		t.Fatalf("splitItemImage: %v", err)
	}
	if len(parts) < 3 {
		t.Fatalf("parts = %d, want >= 3", len(parts))
	}

	total := 0
	for _, p := range parts {
		if p.pxW != 40 || p.pxH <= 0 || p.pxH > pdfStripHeightPx {
			t.Fatalf("unexpected part: %dx%d", p.pxW, p.pxH)
		}
		if _, err := png.Decode(bytes.NewReader(p.data)); err != nil {
			t.Fatalf("part is not a valid png: %v", err)
		}
		total += p.pxH
	}
	if total != tall {
		t.Fatalf("total height = %d, want %d", total, tall)
	}
}

func TestSplitCutsOnlyAtSafeBreaks(t *testing.T) {
	// 配图占 [1000, 2000)，区间内没有任何断点：切点不得落在其中，否则会截断图形。
	const pxH = 6000
	breaks := []int{500, 1000, 2000, 2500, 3000, 3500, 4000, 4500, 5000, 5500}

	cuts := splitCuts(pxH, breaks)
	if len(cuts) == 0 || cuts[len(cuts)-1] != pxH {
		t.Fatalf("cuts = %v, want last cut %d", cuts, pxH)
	}

	prev := 0
	for _, cut := range cuts {
		if cut <= prev {
			t.Fatalf("cuts not increasing: %v", cuts)
		}
		if cut < pxH && !containsInt(breaks, cut) {
			t.Fatalf("cut %d is not a safe break (would tear an image)", cut)
		}
		prev = cut
	}
}

func containsInt(values []int, v int) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

func TestSplitItemImageKeepsShortBitmapIntact(t *testing.T) {
	data := encodePNG(t, 40, 20)
	parts, err := splitItemImage(data, 40, 20, nil)
	if err != nil {
		t.Fatalf("splitItemImage: %v", err)
	}
	if len(parts) != 1 || parts[0].pxH != 20 || !bytes.Equal(parts[0].data, data) {
		t.Fatalf("short bitmap should be returned unchanged: %+v", parts)
	}
}

func TestBuildPDFSplitsOverflowingItemAcrossPages(t *testing.T) {
	fonts := testFonts(t)
	long := strings.Repeat("这是一道超长题干，用于验证分页而不是整题缩小。", 160)
	items := []renderItem{{item: ExportItem{Subject: "数学", StemText: long}}}

	data, err := buildPDF("t", items, fonts)
	if err != nil {
		t.Fatalf("buildPDF: %v", err)
	}
	if pages := pdfPageCount(t, data); pages < 2 {
		t.Fatalf("expected the long item to span multiple pages, got %d", pages)
	}
}

// TestSplitCutsAvoidFigureRegion 回归测试：配图恰好跨越页面边界时，切点不得落在
// 配图内部，否则导出的 PDF 会把几何图形从中间截断成两页。
func TestSplitCutsAvoidFigureRegion(t *testing.T) {
	fonts := testFonts(t)
	rc := defaultRenderConfig()

	it := renderItem{
		item:   ExportItem{StemText: strings.Repeat("这是一段用于验证分页不会截断配图的较长题干内容。", 80)},
		images: []ImageAsset{{Data: encodePNG(t, 1200, 900), Format: "png", Width: 1200, Height: 900}},
	}

	layout, err := renderItemLayout(rc, fonts, 1, it)
	if err != nil {
		t.Fatalf("renderItemLayout: %v", err)
	}
	if layout.height <= pdfStripHeightPx {
		t.Fatalf("test setup expects an over-height item, got %d", layout.height)
	}

	placed, err := decodeImages(it.images, float64(rc.width-2*rc.padding))
	if err != nil || len(placed) != 1 {
		t.Fatalf("decodeImages: %v, placed %d", err, len(placed))
	}
	// 配图是最后一个元素：其上下边界可由画布高度反推。
	figureBottom := float64(layout.height - rc.padding)
	figureTop := figureBottom - placed[0].h

	for _, cut := range splitCuts(layout.height, layout.breaks) {
		y := float64(cut)
		if y > figureTop+1 && y < figureBottom-1 {
			t.Fatalf("cut %d falls inside figure [%.1f, %.1f): figure would be torn",
				cut, figureTop, figureBottom)
		}
	}
}
