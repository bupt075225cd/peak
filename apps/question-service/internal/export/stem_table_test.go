package export

import (
	"bytes"
	"image"
	"os"
	"strings"
	"testing"
)

const stemTableMarkdown = "| 计费档 | 用水量x | 单价 |\n| --- | --- | --- |\n| 第一档 | 0<x≤220 | 3.5 |\n| 第二档 | 220<x≤300 | 5.0 |"

// TestNormalizeStemTextKeepsTableRows 表格块内部的换行必须保留，否则行列关系被折叠成一行，
// 导出时无法再还原成表格。
func TestNormalizeStemTextKeepsTableRows(t *testing.T) {
	raw := "如表所示：\n" + stemTableMarkdown + "\n(1) 求 y 与 x 的关系式"
	want := "如表所示：\n" + stemTableMarkdown + "\n(1) 求 y 与 x 的关系式"
	if got := normalizeStemText(raw, true); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

// TestParseStemBlocks 题干切成"段落 + 表格"块，行列内容完整。
func TestParseStemBlocks(t *testing.T) {
	blocks := parseStemBlocks("前文\n" + stemTableMarkdown + "\n后文")
	if len(blocks) != 3 {
		t.Fatalf("blocks = %d, want 3 (%+v)", len(blocks), blocks)
	}
	if blocks[0].table != nil || blocks[0].text != "前文" {
		t.Fatalf("block0 = %+v", blocks[0])
	}
	if blocks[2].table != nil || blocks[2].text != "后文" {
		t.Fatalf("block2 = %+v", blocks[2])
	}
	tbl := blocks[1].table
	if tbl == nil {
		t.Fatal("middle block should be a table")
	}
	if len(tbl.header) != 3 || tbl.header[0] != "计费档" || tbl.header[2] != "单价" {
		t.Fatalf("header = %v", tbl.header)
	}
	if len(tbl.rows) != 2 || tbl.rows[1][1] != "220<x≤300" {
		t.Fatalf("rows = %v", tbl.rows)
	}
}

// TestParseStemBlocksIgnoresPlainPipes 没有分隔行的 | 文本不是表格，按普通段落处理（向后兼容旧结果）。
func TestParseStemBlocksIgnoresPlainPipes(t *testing.T) {
	blocks := parseStemBlocks("第一档 | 0<x≤220 | 3.5")
	if len(blocks) != 1 || blocks[0].table != nil {
		t.Fatalf("blocks = %+v", blocks)
	}
}

// TestBuildDocxRendersStemTable 题干表格在 DOCX 里生成带边框的 w:tbl，表头加粗。
func TestBuildDocxRendersStemTable(t *testing.T) {
	items := []renderItem{{item: ExportItem{StemText: "如表\n" + stemTableMarkdown}}}
	data, err := buildDocx("测试", items)
	if err != nil {
		t.Fatalf("buildDocx: %v", err)
	}
	doc := string(docxFiles(t, data)["word/document.xml"])
	for _, want := range []string{`<w:tbl>`, "计费档", "第一档", "220&lt;x≤300", `<w:b/>`} {
		if !strings.Contains(doc, want) {
			t.Fatalf("document.xml missing %q", want)
		}
	}
}

// TestRenderItemLayoutDrawsStemTable 表格参与高度预算并成功光栅化（比纯文字高）。
func TestRenderItemLayoutDrawsStemTable(t *testing.T) {
	fonts := testFonts(t)
	rc := defaultRenderConfig()

	withTable, err := renderItemLayout(rc, fonts, 1, renderItem{
		item: ExportItem{StemText: "如表\n" + stemTableMarkdown},
	})
	if err != nil {
		t.Fatalf("render with table: %v", err)
	}
	if len(withTable.data) == 0 || withTable.height <= 0 {
		t.Fatalf("empty table layout: %+v", withTable)
	}

	plain, err := renderItemLayout(rc, fonts, 1, renderItem{
		item: ExportItem{StemText: "只有一段文字"},
	})
	if err != nil {
		t.Fatalf("render plain: %v", err)
	}
	if withTable.height <= plain.height {
		t.Fatalf("table height %d should exceed plain %d", withTable.height, plain.height)
	}
}

// TestRenderStemTableDrawsBorders 表格必须真的画出框线：gg 的 DrawLine 不 Stroke 时只有文本、
// 没有任何线，导出的 PDF 看起来就是"表格内容散成几行文字"。
func TestRenderStemTableDrawsBorders(t *testing.T) {
	fonts := testFonts(t)
	rc := defaultRenderConfig()
	lay, err := renderItemLayout(rc, fonts, 1, renderItem{
		item: ExportItem{StemText: "如表\n" + stemTableMarkdown},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(lay.data))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	b := img.Bounds()
	longest := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		run := 0
		for x := b.Min.X; x < b.Max.X; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			if r>>8 < 0x80 { // 深色像素
				run++
				if run > longest {
					longest = run
				}
				continue
			}
			run = 0
		}
	}
	if longest < 120 {
		t.Fatalf("no table border found (longest dark run = %d)", longest)
	}
}

// TestManualRenderStemTable 手工导出渲染结果，便于人眼核对表格版式（GEOM_MANUAL=1）。
func TestManualRenderStemTable(t *testing.T) {
	if os.Getenv("GEOM_MANUAL") == "" {
		t.Skip("manual only")
	}
	fonts := testFonts(t)
	rc := defaultRenderConfig()
	lay, err := renderItemLayout(rc, fonts, 1, renderItem{
		item: ExportItem{
			StemText:     "17.为了鼓励市民节约用水，三明市采用分档计费的方式计算水费。如表是家庭人口不超过4人时户年用水量及分档计费标准：\n" + stemTableMarkdown + "\n(1) 当220<x≤300时，写出水费 y（单位：元）与 x 之间的关系式；",
			Subject:      "数学",
			QuestionType: "解答题",
			Source:       "第八周周中练习",
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := os.WriteFile("/tmp/stem_table_render.png", lay.data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("written /tmp/stem_table_render.png (%d bytes, height=%d)", len(lay.data), lay.height)
}
