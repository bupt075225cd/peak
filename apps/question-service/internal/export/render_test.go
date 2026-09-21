package export

import (
	"bytes"
	"image/png"
	"math"
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

func TestDecodeImagesUsesSharedLayoutStrategy(t *testing.T) {
	rc := defaultRenderConfig()
	contentWidth := float64(rc.width - 2*rc.padding)

	assets := []ImageAsset{{Data: encodePNG(t, 400, 400), Format: "png", Width: 400, Height: 400}}
	placed, err := decodeImages(assets, contentWidth)
	if err != nil {
		t.Fatalf("decodeImages: %v", err)
	}
	if len(placed) != 1 {
		t.Fatalf("placed %d images, want 1", len(placed))
	}

	wMM, hMM := imageDisplaySizeMM(assets[0])
	wantW, wantH := wMM*pdfPixelsPerMM, hMM*pdfPixelsPerMM
	if math.Abs(placed[0].w-wantW) > 1e-9 || math.Abs(placed[0].h-wantH) > 1e-9 {
		t.Fatalf("image size = %.4f×%.4f, want %.4f×%.4f", placed[0].w, placed[0].h, wantW, wantH)
	}
	// 不再铺满正文宽度：正方形图应明显小于正文宽，避免"太大"。
	if placed[0].w >= contentWidth {
		t.Fatalf("image width %.1f should be smaller than content width %.1f", placed[0].w, contentWidth)
	}
}

func TestDecodeImagesLimitsBitmapUpscale(t *testing.T) {
	rc := defaultRenderConfig()
	contentWidth := float64(rc.width - 2*rc.padding)

	// 极小位图按共用策略放大，但不超过原始尺寸的放大倍数上限。
	assets := []ImageAsset{{
		Data: encodePNG(t, 60, 60), Format: "png",
		Width: 60, Height: 60, NaturalWidth: 60, NaturalHeight: 60,
	}}
	placed, err := decodeImages(assets, contentWidth)
	if err != nil {
		t.Fatalf("decodeImages: %v", err)
	}
	if len(placed) != 1 {
		t.Fatalf("placed %d images, want 1", len(placed))
	}
	wantMM := float64(60) / imageSourceDPI * mmPerInch * imageMaxUpscale
	if got, want := placed[0].w, wantMM*pdfPixelsPerMM; math.Abs(got-want) > 1e-9 {
		t.Fatalf("image width = %.4f, want %.4f (upscale capped)", got, want)
	}
}

func TestWrapTextAvoidsEarlySpaceBreak(t *testing.T) {
	fonts := testFonts(t)
	face, err := fonts.Face(20)
	if err != nil {
		t.Fatalf("Face: %v", err)
	}

	// 空格出现在行首附近，之后是长串中文：应尽量填满整行，而不是在空格处早断。
	text := "a " + strings.Repeat("中", 40)
	maxWidth := runeWidth(face, '中')*10 + 1

	lines := wrapText(face, text, maxWidth)
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %#v", lines)
	}
	if w := lineWidth(face, lines[0]); w < maxWidth*spaceBreakMinFill {
		t.Fatalf("first line %.1f too short, want >= %.1f", w, maxWidth*spaceBreakMinFill)
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

func TestRenderItemLayoutBreaksAreSortedWithinCanvas(t *testing.T) {
	fonts := testFonts(t)
	rc := defaultRenderConfig()
	it := renderItem{
		item:   ExportItem{StemText: strings.Repeat("这是一段用于验证分页断点的较长题干内容。", 60)},
		images: []ImageAsset{{Data: encodePNG(t, 400, 300), Format: "png", Width: 400, Height: 300}},
	}

	layout, err := renderItemLayout(rc, fonts, 1, it)
	if err != nil {
		t.Fatalf("renderItemLayout: %v", err)
	}
	if len(layout.breaks) == 0 {
		t.Fatal("expected safe break positions")
	}
	if layout.width != rc.width || layout.height <= 0 || len(layout.data) == 0 {
		t.Fatalf("unexpected layout: %dx%d", layout.width, layout.height)
	}
	for i, b := range layout.breaks {
		if b < 0 || b > layout.height {
			t.Fatalf("break %d out of canvas height %d", b, layout.height)
		}
		if i > 0 && b < layout.breaks[i-1] {
			t.Fatalf("breaks not sorted: %v", layout.breaks)
		}
	}
}

func TestGroupImageRowsPacksSideBySide(t *testing.T) {
	placed := []placedImage{{w: 300, h: 200}, {w: 300, h: 250}}

	rows := groupImageRows(placed, 1000, 50)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (images should share a row)", len(rows))
	}
	if rows[0].width != 650 || rows[0].height != 250 {
		t.Fatalf("row = %.0fx%.0f, want 650x250", rows[0].width, rows[0].height)
	}
}

func TestGroupImageRowsWrapsWhenTooWide(t *testing.T) {
	// 600 + 50(间距) + 600 = 1250 > 1000，应换行。
	placed := []placedImage{{w: 600, h: 100}, {w: 600, h: 100}}

	rows := groupImageRows(placed, 1000, 50)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
}

func TestGroupImageRowsKeepsOversizedImageAlone(t *testing.T) {
	placed := []placedImage{{w: 2000, h: 100}}

	rows := groupImageRows(placed, 1000, 50)
	if len(rows) != 1 || len(rows[0].images) != 1 {
		t.Fatalf("oversized image should occupy its own row: %+v", rows)
	}
}

func TestRenderItemImagePutsSmallImagesInOneRow(t *testing.T) {
	fonts := testFonts(t)
	rc := defaultRenderConfig()

	one := renderItem{
		item:   ExportItem{StemText: "题干"},
		images: []ImageAsset{{Data: encodePNG(t, 100, 100), Format: "png", Width: 100, Height: 100}},
	}
	two := renderItem{
		item: ExportItem{StemText: "题干"},
		images: []ImageAsset{
			{Data: encodePNG(t, 100, 100), Format: "png", Width: 100, Height: 100},
			{Data: encodePNG(t, 100, 100), Format: "png", Width: 100, Height: 100},
		},
	}

	_, _, oneH, err := renderItemImage(rc, fonts, 1, one)
	if err != nil {
		t.Fatalf("render one: %v", err)
	}
	_, _, twoH, err := renderItemImage(rc, fonts, 1, two)
	if err != nil {
		t.Fatalf("render two: %v", err)
	}
	// 两张小图并排在同一行，画布高度应与单图一致，而不是叠加成两行。
	if twoH != oneH {
		t.Fatalf("two side-by-side images height = %d, want %d (same row)", twoH, oneH)
	}
}

func TestImageRowGapIsWiderThanParagraphGap(t *testing.T) {
	rc := defaultRenderConfig()
	rowGap := float64(rc.gap) * imageRowGapScale

	// 同行配图间距应明显大于段落间距，避免并排的图形看起来粘连成一整张。
	if rowGap <= float64(rc.gap) {
		t.Fatalf("row gap %.1f should exceed paragraph gap %d", rowGap, rc.gap)
	}
}
