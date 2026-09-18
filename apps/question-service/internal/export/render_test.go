package export

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	"golang.org/x/image/font"
)

func testFonts(t *testing.T) *FontProvider {
	t.Helper()
	fonts, err := NewFontProvider("")
	if err != nil {
		t.Fatalf("NewFontProvider: %v", err)
	}
	return fonts
}

func lineWidth(face font.Face, s string) float64 {
	return float64(font.MeasureString(face, s)) / 64.0
}

func TestWrapTextWrapsLongChinese(t *testing.T) {
	fonts := testFonts(t)
	face, err := fonts.Face(20)
	if err != nil {
		t.Fatalf("Face: %v", err)
	}

	text := strings.Repeat("错题本", 40)
	const maxWidth = 200.0

	lines := wrapText(face, text, maxWidth)
	if len(lines) < 2 {
		t.Fatalf("expected multiple lines, got %d", len(lines))
	}
	for _, line := range lines {
		if w := lineWidth(face, line); w > maxWidth {
			t.Fatalf("line width %.1f exceeds limit %.1f: %q", w, maxWidth, line)
		}
	}
	if got := strings.Join(lines, ""); got != text {
		t.Fatal("text content lost while wrapping")
	}
}

func TestWrapTextKeepsExplicitNewlines(t *testing.T) {
	fonts := testFonts(t)
	face, err := fonts.Face(20)
	if err != nil {
		t.Fatalf("Face: %v", err)
	}

	lines := wrapText(face, "第一行\n第二行", 1000)
	if len(lines) != 2 || lines[0] != "第一行" || lines[1] != "第二行" {
		t.Fatalf("unexpected lines: %#v", lines)
	}
}

func TestWrapTextBreaksAtSpaceWithoutSplittingWords(t *testing.T) {
	fonts := testFonts(t)
	face, err := fonts.Face(20)
	if err != nil {
		t.Fatalf("Face: %v", err)
	}

	maxWidth := runeWidth(face, 'a')*4 + 1
	lines := wrapText(face, "aaaa aaaa", maxWidth)

	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %#v", lines)
	}
	if lines[0] != "aaaa" {
		t.Fatalf("expected whole word on first line, got %q", lines[0])
	}
	for _, line := range lines {
		if w := lineWidth(face, line); w > maxWidth {
			t.Fatalf("line width %.1f exceeds limit %.1f", w, maxWidth)
		}
	}
}

func TestRenderItemImageProducesValidPNG(t *testing.T) {
	fonts := testFonts(t)
	rc := defaultRenderConfig()

	it := renderItem{item: ExportItem{
		Grade: "七年级上", Subject: "数学", QuestionType: "解答题",
		Source:   "期中试卷",
		StemText: "已知二次函数 y = x^2 - 2x - 3，求顶点坐标。",
	}}

	data, w, h, err := renderItemImage(rc, fonts, 1, it)
	if err != nil {
		t.Fatalf("renderItemImage: %v", err)
	}
	if w != rc.width {
		t.Fatalf("width = %d, want %d", w, rc.width)
	}
	if h <= 2*rc.padding {
		t.Fatalf("height %d looks too small", h)
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode rendered png: %v", err)
	}
	if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
		t.Fatalf("png size = %dx%d, want %dx%d", img.Bounds().Dx(), img.Bounds().Dy(), w, h)
	}
}

func TestRenderItemImageHeightGrowsWithLongerStem(t *testing.T) {
	fonts := testFonts(t)
	rc := defaultRenderConfig()

	_, _, shortH, err := renderItemImage(rc, fonts, 1, renderItem{item: ExportItem{StemText: "短题干"}})
	if err != nil {
		t.Fatalf("render short: %v", err)
	}

	long := strings.Repeat("这是一道很长的题干内容，用于验证换行与高度计算。", 20)
	_, _, longH, err := renderItemImage(rc, fonts, 2, renderItem{item: ExportItem{StemText: long}})
	if err != nil {
		t.Fatalf("render long: %v", err)
	}

	if longH <= shortH {
		t.Fatalf("expected taller canvas for longer stem: %d <= %d", longH, shortH)
	}
}

func TestRenderItemImageHeightGrowsWithImage(t *testing.T) {
	fonts := testFonts(t)
	rc := defaultRenderConfig()

	_, _, plainH, err := renderItemImage(rc, fonts, 1, renderItem{item: ExportItem{StemText: "题干"}})
	if err != nil {
		t.Fatalf("render without image: %v", err)
	}

	withImage := renderItem{
		item:   ExportItem{StemText: "题干"},
		images: []ImageAsset{{Data: encodePNG(t, 200, 100), Format: "png", Width: 200, Height: 100}},
	}
	_, _, imageH, err := renderItemImage(rc, fonts, 1, withImage)
	if err != nil {
		t.Fatalf("render with image: %v", err)
	}

	if imageH <= plainH {
		t.Fatalf("expected taller canvas with image: %d <= %d", imageH, plainH)
	}
}

func TestRenderItemImageNotesFailedImages(t *testing.T) {
	fonts := testFonts(t)
	rc := defaultRenderConfig()

	_, _, plainH, err := renderItemImage(rc, fonts, 1, renderItem{item: ExportItem{StemText: "题干"}})
	if err != nil {
		t.Fatalf("render plain: %v", err)
	}

	failed := renderItem{
		item:        ExportItem{StemText: "题干", ImageKeys: []string{"a.png"}},
		imageFailed: true,
	}
	_, _, failedH, err := renderItemImage(rc, fonts, 1, failed)
	if err != nil {
		t.Fatalf("render failed image: %v", err)
	}

	if failedH <= plainH {
		t.Fatalf("expected placeholder note to add height: %d <= %d", failedH, plainH)
	}
}

func TestRenderItemImageTooNarrowWidth(t *testing.T) {
	fonts := testFonts(t)
	rc := defaultRenderConfig()
	rc.width = 10

	if _, _, _, err := renderItemImage(rc, fonts, 1, renderItem{item: ExportItem{StemText: "x"}}); err == nil {
		t.Fatal("expected error for too narrow canvas")
	}
}
