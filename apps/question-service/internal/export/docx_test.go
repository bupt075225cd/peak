package export

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

// docxFiles 把 .docx 当作 zip 解开，返回各部件内容。
func docxFiles(t *testing.T, data []byte) map[string][]byte {
	t.Helper()

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open docx as zip: %v", err)
	}
	out := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read entry %s: %v", f.Name, err)
		}
		out[f.Name] = b
	}
	return out
}

func TestBuildDocxProducesRequiredParts(t *testing.T) {
	items := []renderItem{{
		item: ExportItem{
			Grade: "七年级上", Subject: "数学", QuestionType: "解答题",
			Source: "期中试卷", StemText: "已知 x^2=4，求 x。",
		},
		images: []ImageAsset{{Data: encodePNG(t, 40, 20), Format: "png", Width: 40, Height: 20}},
	}}

	data, err := buildDocx("我的错题本 2026-09-17", items)
	if err != nil {
		t.Fatalf("buildDocx: %v", err)
	}

	files := docxFiles(t, data)
	for _, name := range []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"word/document.xml",
		"word/_rels/document.xml.rels",
		"word/media/image1.png",
	} {
		if _, ok := files[name]; !ok {
			t.Fatalf("missing docx part %q", name)
		}
	}

	doc := string(files["word/document.xml"])
	for _, want := range []string{
		"我的错题本 2026-09-17",
		"七年级上",
		"数学",
		"解答题",
		"来源：期中试卷",
		"已知 x^2=4，求 x。",
		"<w:drawing>",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("document.xml missing %q", want)
		}
	}

	if rels := string(files["word/_rels/document.xml.rels"]); !strings.Contains(rels, "media/image1.png") {
		t.Fatalf("image relationship missing: %s", rels)
	}
	if ct := string(files["[Content_Types].xml"]); !strings.Contains(ct, `Extension="png"`) {
		t.Fatalf("png content type missing: %s", ct)
	}
}

func TestBuildDocxEscapesSpecialCharacters(t *testing.T) {
	items := []renderItem{{item: ExportItem{StemText: "a < b & c > d"}}}

	data, err := buildDocx("t", items)
	if err != nil {
		t.Fatalf("buildDocx: %v", err)
	}

	doc := string(docxFiles(t, data)["word/document.xml"])
	if !strings.Contains(doc, "a &lt; b &amp; c &gt; d") {
		t.Fatalf("expected escaped stem text, got %s", doc)
	}
}

func TestBuildDocxWithoutImages(t *testing.T) {
	items := []renderItem{{item: ExportItem{StemText: "no image"}}}

	data, err := buildDocx("t", items)
	if err != nil {
		t.Fatalf("buildDocx: %v", err)
	}

	files := docxFiles(t, data)
	if rels := string(files["word/_rels/document.xml.rels"]); strings.Contains(rels, "image") {
		t.Fatalf("unexpected image relationship: %s", rels)
	}
	if doc := string(files["word/document.xml"]); strings.Contains(doc, "<w:drawing>") {
		t.Fatal("unexpected drawing element")
	}
}

func TestBuildDocxMultipleImagesUseUniqueIds(t *testing.T) {
	items := []renderItem{
		{
			item:   ExportItem{StemText: "a"},
			images: []ImageAsset{{Data: encodePNG(t, 10, 10), Format: "png", Width: 10, Height: 10}},
		},
		{
			item: ExportItem{StemText: "b"},
			images: []ImageAsset{
				{Data: encodePNG(t, 10, 10), Format: "png", Width: 10, Height: 10},
				{Data: encodeJPEG(t, 10, 10), Format: "jpeg", Width: 10, Height: 10},
			},
		},
	}

	data, err := buildDocx("t", items)
	if err != nil {
		t.Fatalf("buildDocx: %v", err)
	}

	files := docxFiles(t, data)
	for _, name := range []string{
		"word/media/image1.png",
		"word/media/image2.png",
		"word/media/image3.jpeg",
	} {
		if _, ok := files[name]; !ok {
			t.Fatalf("missing media part %q", name)
		}
	}

	rels := string(files["word/_rels/document.xml.rels"])
	for _, rid := range []string{`Id="rId1"`, `Id="rId2"`, `Id="rId3"`} {
		if !strings.Contains(rels, rid) {
			t.Fatalf("missing relationship %s in %s", rid, rels)
		}
	}
}

func TestBuildDocxRejectsEmptyItems(t *testing.T) {
	if _, err := buildDocx("t", nil); err == nil {
		t.Fatal("expected error for empty items")
	}
}

func TestDocxImageSizeClampsToContentWidth(t *testing.T) {
	cx, cy := docxImageSize(4000, 2000)
	if cx != docxContentWidthEMU || cy != docxContentWidthEMU/2 {
		t.Fatalf("large image size = %dx%d, want %dx%d", cx, cy, docxContentWidthEMU, docxContentWidthEMU/2)
	}

	cx, cy = docxImageSize(100, 50)
	if cx != 100*emuPerPixel || cy != 50*emuPerPixel {
		t.Fatalf("small image size = %dx%d, want %dx%d", cx, cy, 100*emuPerPixel, 50*emuPerPixel)
	}

	cx, cy = docxImageSize(0, 0)
	if cx != docxContentWidthEMU || cy != docxContentWidthEMU/2 {
		t.Fatalf("invalid image size = %dx%d, want fallback", cx, cy)
	}
}

func TestItemMetaLineSkipsEmptyFields(t *testing.T) {
	if got := itemMetaLine(1, ExportItem{}); got != "1." {
		t.Fatalf("empty meta line = %q, want %q", got, "1.")
	}
	if got := itemMetaLine(2, ExportItem{Subject: "数学", Source: "卷子"}); got != "2. 数学 · 来源：卷子" {
		t.Fatalf("meta line = %q", got)
	}
}
