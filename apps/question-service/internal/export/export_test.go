package export

import (
	"bytes"
	"context"
	"image/png"
	"strings"
	"testing"
)

func newTestService(t *testing.T, fetcher Fetcher, maxItems int) Service {
	t.Helper()
	cfg := DefaultConfig()
	cfg.MaxItems = maxItems
	return New(cfg, fetcher, testFonts(t))
}

func TestExportDocxEmbedsImages(t *testing.T) {
	ff := newFakeFetcher(map[string][]byte{"a.png": encodePNG(t, 40, 20)})
	svc := newTestService(t, ff, 0)

	res, err := svc.Export(context.Background(), []ExportItem{{
		Grade: "七年级上", Subject: "数学", StemText: "题干", Images: []ImageRef{{Key: "a.png"}},
	}}, FormatDocx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(res.Data) == 0 {
		t.Fatal("expected non-empty docx payload")
	}
	if !strings.HasSuffix(res.Filename, ".docx") {
		t.Fatalf("filename = %q, want .docx suffix", res.Filename)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}

	files := docxFiles(t, res.Data)
	if _, ok := files["word/media/image1.png"]; !ok {
		t.Fatal("expected image to be embedded in docx")
	}
}

func TestExportPDFProducesPDFBytes(t *testing.T) {
	svc := newTestService(t, newFakeFetcher(nil), 0)

	res, err := svc.Export(context.Background(), []ExportItem{{StemText: "题干"}}, FormatPDF)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !bytes.HasPrefix(res.Data, []byte("%PDF-")) {
		t.Fatalf("expected pdf header, got %q", res.Data[:min(8, len(res.Data))])
	}
	if !strings.HasSuffix(res.Filename, ".pdf") {
		t.Fatalf("filename = %q, want .pdf suffix", res.Filename)
	}
}

func TestExportRejectsEmptyItems(t *testing.T) {
	svc := newTestService(t, newFakeFetcher(nil), 0)
	if _, err := svc.Export(context.Background(), nil, FormatPDF); err == nil {
		t.Fatal("expected error for empty items")
	}
}

func TestExportRejectsTooManyItems(t *testing.T) {
	svc := newTestService(t, newFakeFetcher(nil), 1)
	items := []ExportItem{{StemText: "a"}, {StemText: "b"}}
	if _, err := svc.Export(context.Background(), items, FormatPDF); err == nil {
		t.Fatal("expected error when exceeding item limit")
	}
}

func TestExportRejectsUnsupportedFormat(t *testing.T) {
	svc := newTestService(t, newFakeFetcher(nil), 0)
	items := []ExportItem{{StemText: "a"}}
	if _, err := svc.Export(context.Background(), items, Format("txt")); err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestExportReportsImageFailuresWithoutAborting(t *testing.T) {
	svc := newTestService(t, newFakeFetcher(nil), 0)

	res, err := svc.Export(context.Background(), []ExportItem{{
		StemText: "题干", Images: []ImageRef{{Key: "missing.png"}},
	}}, FormatDocx)
	if err != nil {
		t.Fatalf("Export should tolerate missing images: %v", err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "missing.png") {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}
}

func TestExportPreservesImageOrder(t *testing.T) {
	ff := newFakeFetcher(map[string][]byte{
		"a.png": encodePNG(t, 10, 10),
		"b.png": encodePNG(t, 20, 20),
	})
	svc := newTestService(t, ff, 0)

	res, err := svc.Export(context.Background(), []ExportItem{{
		StemText: "题干", Images: []ImageRef{{Key: "b.png"}, {Key: "a.png"}},
	}}, FormatDocx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	files := docxFiles(t, res.Data)
	first, ok := files["word/media/image1.png"]
	if !ok {
		t.Fatal("missing first embedded image")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(first))
	if err != nil {
		t.Fatalf("decode first image: %v", err)
	}
	if cfg.Width != 20 {
		t.Fatalf("first embedded image width = %d, want 20 (b.png)", cfg.Width)
	}
}

// TestExportDocxLabelsFigures 验证导出的 Word 会把图号标注在对应配图正下方：
// 图注必须落在该图与下一张图之间，否则图号会错配到别的图上。
func TestExportDocxLabelsFigures(t *testing.T) {
	ff := newFakeFetcher(map[string][]byte{
		"a.png": encodePNG(t, 40, 40),
		"b.png": encodePNG(t, 40, 40),
	})
	svc := newTestService(t, ff, 0)

	res, err := svc.Export(context.Background(), []ExportItem{{
		StemText: "题干",
		Images:   []ImageRef{{Key: "a.png", Label: "图1"}, {Key: "b.png", Label: "图2"}},
	}}, FormatDocx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	doc := string(docxFiles(t, res.Data)["word/document.xml"])
	for _, want := range []string{"图1", "图2"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("docx missing caption %q: %s", want, doc)
		}
	}

	firstImg := strings.Index(doc, `name="image1.png"`)
	firstCap := strings.Index(doc, "图1")
	secondImg := strings.Index(doc, `name="image2.png"`)
	secondCap := strings.Index(doc, "图2")
	if !(firstImg < firstCap && firstCap < secondImg && secondImg < secondCap) {
		t.Fatalf("captions not placed under their own figure: %s", doc)
	}
}

// TestExportSkipsEmptyCaption 验证未带图号的配图不产生图注段落。
func TestExportSkipsEmptyCaption(t *testing.T) {
	ff := newFakeFetcher(map[string][]byte{"a.png": encodePNG(t, 40, 40)})
	svc := newTestService(t, ff, 0)

	res, err := svc.Export(context.Background(), []ExportItem{{
		StemText: "题干", Images: []ImageRef{{Key: "a.png"}},
	}}, FormatDocx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	doc := string(docxFiles(t, res.Data)["word/document.xml"])
	if strings.Contains(doc, "图") {
		t.Fatalf("unexpected caption in docx: %s", doc)
	}
}

func TestParseFormat(t *testing.T) {
	for _, raw := range []string{"pdf", "PDF", " docx "} {
		if _, err := ParseFormat(raw); err != nil {
			t.Fatalf("ParseFormat(%q): %v", raw, err)
		}
	}
	if _, err := ParseFormat("xlsx"); err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestFormatMetadata(t *testing.T) {
	if got := FormatPDF.ContentType(); got != "application/pdf" {
		t.Fatalf("pdf content type = %q", got)
	}
	if got := FormatDocx.Extension(); got != ".docx" {
		t.Fatalf("docx extension = %q", got)
	}
	if got := Format("x").ContentType(); got != "application/octet-stream" {
		t.Fatalf("unknown content type = %q", got)
	}
	if got := Format("x").Extension(); got != ".bin" {
		t.Fatalf("unknown extension = %q", got)
	}
}
