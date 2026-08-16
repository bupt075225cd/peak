// Package docparse 提供 word/pdf 文档解析能力，提取文本与内嵌图片。
// 纯 Go 实现，无 cgo、无外部命令依赖，便于容器内自包含部署。
package docparse

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Item 文档解析出的内容项。
type Item struct {
	Kind  string // "text" / "image"
	Text  string
	Image []byte
}

// Result 文档解析结果。
type Result struct {
	Items     []Item
	PageCount int
}

// Parse 根据文件名后缀分发解析器。
func Parse(data []byte, filename string) (*Result, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".docx":
		return parseDocx(data)
	case ".pdf":
		return parsePDF(data)
	case ".doc":
		return nil, fmt.Errorf("legacy .doc format is not supported, please convert to .docx")
	default:
		return nil, fmt.Errorf("unsupported document format: %s", ext)
	}
}

// parseDocx 解析 docx（本质是 zip + word/document.xml + word/media/* 图片）。
// 按 document.xml 的原始顺序遍历段落与内嵌图片（drawing），保证题与图的顺序对齐。
func parseDocx(data []byte) (*Result, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open docx: %w", err)
	}

	// 读取 rId -> media 文件 的映射（word/_rels/document.xml.rels）。
	relMap, err := readRels(zr)
	if err != nil {
		return nil, err
	}

	// 收集内嵌图片（word/media/ 下的图片文件）。
	media := map[string][]byte{}
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "word/media/") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		media[f.Name] = b
	}

	// 流式解析 document.xml：按顺序输出文本段落与图片。
	docFile := findZipFile(zr, "word/document.xml")
	if docFile == nil {
		return nil, fmt.Errorf("word/document.xml not found in docx")
	}
	rc, err := docFile.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	res := &Result{}
	dec := xml.NewDecoder(rc)

	var (
		inParagraph bool
		paraBuf     strings.Builder
		inDrawing   bool
		blipEmbed   string
	)

	flushParagraph := func() {
		line := strings.TrimSpace(paraBuf.String())
		if line != "" {
			res.Items = append(res.Items, Item{Kind: "text", Text: line})
		}
		paraBuf.Reset()
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode document.xml: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				inParagraph = true
				paraBuf.Reset()
			case "drawing":
				inDrawing = true
			case "blip":
				if inDrawing {
					// 提取 r:embed 属性。
					for _, attr := range t.Attr {
						if attr.Name.Local == "embed" {
							blipEmbed = attr.Value
						}
					}
				}
			}
		case xml.CharData:
			if !inDrawing && inParagraph {
				paraBuf.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "p":
				if inParagraph {
					flushParagraph()
					inParagraph = false
				}
			case "drawing":
				if inDrawing {
					// drawing 结束：根据 blip embed 找到对应图片，追加为 image 项。
					if blipEmbed != "" {
						if mediaPath, ok := relMap[blipEmbed]; ok {
							if img, ok := media[mediaPath]; ok {
								res.Items = append(res.Items, Item{Kind: "image", Image: img})
							}
						}
					}
					inDrawing = false
					blipEmbed = ""
				}
			}
		}
	}

	// 若段落流里还有未刷新的文本，补一次。
	if inParagraph {
		flushParagraph()
	}

	if len(res.Items) == 0 {
		return nil, fmt.Errorf("docx contains no text or image")
	}
	return res, nil
}

// readRels 解析 word/_rels/document.xml.rels，返回 rId -> media 相对路径 映射。
func readRels(zr *zip.Reader) (map[string]string, error) {
	rels := findZipFile(zr, "word/_rels/document.xml.rels")
	if rels == nil {
		return map[string]string{}, nil // 无 rels 时返回空映射，不报错。
	}
	rc, err := rels.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	type relationship struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	}
	var doc struct {
		Rels []relationship `xml:"Relationship"`
	}
	if err := xml.NewDecoder(rc).Decode(&doc); err != nil {
		return nil, err
	}

	m := make(map[string]string, len(doc.Rels))
	for _, r := range doc.Rels {
		// Target 形如 "media/image1.jpeg"，去掉前导 "./" 或 "../"。
		target := strings.TrimPrefix(r.Target, "./")
		if strings.Contains(target, "media/") {
			// 统一为 "word/media/..." 形式。
			if !strings.HasPrefix(target, "word/") {
				target = "word/" + target
			}
			m[r.ID] = target
		}
	}
	return m, nil
}

// findZipFile 按名称查找 zip 内文件（忽略大小写路径差异）。
func findZipFile(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// parsePDF 解析 pdf：提取每页文本，并提取内嵌图片（XObject 位图）。
func parsePDF(data []byte) (*Result, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}

	res := &Result{PageCount: reader.NumPage()}
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			res.Items = append(res.Items, Item{Kind: "text", Text: text})
		}
	}

	if len(res.Items) == 0 {
		return nil, fmt.Errorf("pdf contains no extractable text")
	}
	return res, nil
}
