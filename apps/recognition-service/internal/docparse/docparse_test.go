package docparse

import (
	"archive/zip"
	"bytes"
	"testing"
)

// buildDocx 构造一个最小可解析的 docx 文件（含两段文本 + 一张内嵌图片）。
func buildDocx(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// document.xml：两个段落，第二段含一个内嵌图片（drawing + blip）。
	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>1. 已知集合 A={1,2,3}，求子集个数。</w:t></w:r></w:p>
    <w:p><w:r><w:t>2. 计算 lim(x→0) sin(x)/x。</w:t></w:r></w:p>
    <w:p>
      <w:r>
        <w:drawing>
          <a:blip r:embed="rId1"/>
        </w:drawing>
      </w:r>
    </w:p>
  </w:body>
</w:document>`

	w1, _ := zw.Create("word/document.xml")
	w1.Write([]byte(docXML))

	// 内嵌图片。
	w2, _ := zw.Create("word/media/image1.png")
	w2.Write([]byte("fake-png-data"))

	// rels：rId1 -> media/image1.png。
	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/>
</Relationships>`
	w3, _ := zw.Create("word/_rels/document.xml.rels")
	w3.Write([]byte(rels))

	zw.Close()
	return buf.Bytes()
}

func TestParseDocx(t *testing.T) {
	data := buildDocx(t)
	res, err := Parse(data, "paper.docx")
	if err != nil {
		t.Fatalf("parse docx: %v", err)
	}

	var textCount, imageCount int
	for _, it := range res.Items {
		switch it.Kind {
		case "text":
			textCount++
		case "image":
			imageCount++
		}
	}
	if textCount != 2 {
		t.Fatalf("expected 2 text items, got %d", textCount)
	}
	if imageCount != 1 {
		t.Fatalf("expected 1 image item, got %d", imageCount)
	}
}

func TestParseUnsupported(t *testing.T) {
	if _, err := Parse([]byte("x"), "a.doc"); err == nil {
		t.Fatal("expected error for .doc")
	}
	if _, err := Parse([]byte("x"), "a.txt"); err == nil {
		t.Fatal("expected error for .txt")
	}
}

func TestParseInvalidDocx(t *testing.T) {
	if _, err := Parse([]byte("not-a-zip"), "a.docx"); err == nil {
		t.Fatal("expected error for invalid docx")
	}
}
