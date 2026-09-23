package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"strings"
)

const (
	// emuPerPixel 96 DPI 下 1 像素对应的 EMU（English Metric Unit）。
	emuPerPixel = 9525
	// docxPageWidthTwips A4 页面宽度（twip，1/20 磅）。
	docxPageWidthTwips = 11906
	// docxMarginTwips 页面左右边距（1440 twip = 1 英寸）。
	docxMarginTwips = 1440
	// emuPerTwip 1 twip 对应的 EMU。
	emuPerTwip = 635
	// emuPerMM 1 毫米对应的 EMU（914400/25.4）。
	emuPerMM = 36000
)

// docxContentWidthEMU 正文可用宽度：A4 页宽去掉左右边距。
const docxContentWidthEMU = (docxPageWidthTwips - 2*docxMarginTwips) * emuPerTwip

const (
	nsWord   = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsRel    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsWP     = "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
	nsDraw   = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsPic    = "http://schemas.openxmlformats.org/drawingml/2006/picture"
	nsPkgRel = "http://schemas.openxmlformats.org/package/2006/relationships"
	nsTypes  = "http://schemas.openxmlformats.org/package/2006/content-types"
)

// buildDocx 生成最小可用的 .docx 文档字节。
//
// 文档结构仅需"标题 + 段落 + 内嵌图片"，而 Go 生态缺少成熟且许可宽松的
// docx 生成库（gooxml 已归档、unioffice 为商业许可），因此直接手写 OOXML：
// 零依赖、结构可控，也便于测试。
func buildDocx(title string, items []renderItem) ([]byte, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items to export")
	}

	var body strings.Builder
	body.WriteString(docxParagraph(title, "Title"))

	var rels strings.Builder
	type mediaFile struct {
		path string
		data []byte
	}
	var media []mediaFile

	imageSeq := 0
	docPrSeq := 0

	for i := range items {
		it := &items[i]
		body.WriteString(docxParagraph(itemMetaLine(i+1, it.item), "Heading3"))

		if stem := strings.TrimSpace(it.item.StemText); stem != "" {
			body.WriteString(docxParagraph(stem, ""))
		}

		for _, img := range it.images {
			imageSeq++
			docPrSeq++

			ext := "png"
			if img.Format == "jpeg" {
				ext = "jpeg"
			}
			name := fmt.Sprintf("image%d.%s", imageSeq, ext)
			relID := fmt.Sprintf("rId%d", imageSeq)

			rels.WriteString(fmt.Sprintf(
				`<Relationship Id="%s" Type="%s/image" Target="media/%s"/>`,
				relID, nsRel, name))

			media = append(media, mediaFile{path: "word/media/" + name, data: img.Data})

			cx, cy := docxImageSize(img)
			body.WriteString(docxImageParagraph(docxImageRun(relID, docPrSeq, name, cx, cy)))
			// 图号单独成段排在配图正下方（AI 重绘会丢失原图的"图1/图2"标注）。
			body.WriteString(docxCaptionParagraph(img.Caption))
		}
	}

	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="` + nsWord + `" xmlns:r="` + nsRel +
		`" xmlns:wp="` + nsWP + `" xmlns:a="` + nsDraw + `" xmlns:pic="` + nsPic + `">` +
		`<w:body>` + body.String() + docxSectPr() + `</w:body></w:document>`

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="` + nsTypes + `">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Default Extension="png" ContentType="image/png"/>` +
		`<Default Extension="jpeg" ContentType="image/jpeg"/>` +
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
		`</Types>`

	rootRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="` + nsPkgRel + `">` +
		`<Relationship Id="rId1" Type="` + nsRel + `/officeDocument" Target="word/document.xml"/>` +
		`</Relationships>`

	docRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="` + nsPkgRel + `">` + rels.String() + `</Relationships>`

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	if err := writeZipEntry(zw, "[Content_Types].xml", []byte(contentTypes)); err != nil {
		return nil, err
	}
	if err := writeZipEntry(zw, "_rels/.rels", []byte(rootRels)); err != nil {
		return nil, err
	}
	if err := writeZipEntry(zw, "word/document.xml", []byte(document)); err != nil {
		return nil, err
	}
	if err := writeZipEntry(zw, "word/_rels/document.xml.rels", []byte(docRels)); err != nil {
		return nil, err
	}
	for _, m := range media {
		if err := writeZipEntry(zw, m.path, m.data); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close docx archive: %w", err)
	}
	return buf.Bytes(), nil
}

func writeZipEntry(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("create docx entry %q: %w", name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write docx entry %q: %w", name, err)
	}
	return nil
}

// docxParagraph 生成文本段落，style 为空时不带段落样式。
func docxParagraph(text, style string) string {
	text = sanitizeXMLText(text)
	if text == "" {
		return `<w:p/>`
	}
	pPr := ""
	if style != "" {
		pPr = `<w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`
	}
	return `<w:p>` + pPr + `<w:r><w:t xml:space="preserve">` +
		escapeXMLText(text) + `</w:t></w:r></w:p>`
}

// docxImageParagraph 生成配图段落：整段水平居中并留出适度上下间距。
//
// 每张配图独占一段：这样图号才能紧跟在该图正下方，而不会被 Word 的自动换行打乱。
func docxImageParagraph(run string) string {
	return `<w:p><w:pPr>` +
		`<w:spacing w:before="120" w:after="60"/>` +
		`<w:jc w:val="center"/>` +
		`</w:pPr>` + run + `</w:p>`
}

// docxCaptionFontHalfPoints 图注字号（半磅）：9 磅，比正文小一档。
const docxCaptionFontHalfPoints = 18

// docxCaptionParagraph 生成图注段落（如"图1"）：小字号、居中、紧贴配图下方。
//
// 空图注返回空串，调用方无需判断即可直接拼接。
func docxCaptionParagraph(caption string) string {
	caption = sanitizeXMLText(strings.TrimSpace(caption))
	if caption == "" {
		return ""
	}
	rPr := fmt.Sprintf(`<w:rPr><w:color w:val="404040"/><w:sz w:val="%d"/></w:rPr>`, docxCaptionFontHalfPoints)
	return `<w:p><w:pPr>` +
		`<w:spacing w:before="0" w:after="120"/>` +
		`<w:jc w:val="center"/>` + rPr +
		`</w:pPr><w:r>` + rPr +
		`<w:t xml:space="preserve">` + escapeXMLText(caption) + `</w:t></w:r></w:p>`
}

// docxImageRun 生成单张内嵌图片的 run。
func docxImageRun(relID string, id int, name string, cx, cy int) string {
	return `<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0">` +
		fmt.Sprintf(`<wp:extent cx="%d" cy="%d"/>`, cx, cy) +
		fmt.Sprintf(`<wp:docPr id="%d" name="%s"/>`, id, escapeXMLText(name)) +
		`<a:graphic><a:graphicData uri="` + nsPic + `"><pic:pic>` +
		fmt.Sprintf(`<pic:nvPicPr><pic:cNvPr id="%d" name="%s"/><pic:cNvPicPr/></pic:nvPicPr>`, id, escapeXMLText(name)) +
		`<pic:blipFill><a:blip r:embed="` + relID + `"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>` +
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/>` +
		fmt.Sprintf(`<a:ext cx="%d" cy="%d"/>`, cx, cy) +
		`</a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>` +
		`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
}

// docxSectPr A4 页面设置（纵向、四周 1 英寸边距）。
func docxSectPr() string {
	return fmt.Sprintf(
		`<w:sectPr><w:pgSz w:w="%d" w:h="16838"/>`+
			`<w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="851" w:footer="992" w:gutter="0"/>`+
			`</w:sectPr>`,
		docxPageWidthTwips, docxMarginTwips, docxMarginTwips, docxMarginTwips, docxMarginTwips)
}

// docxImageSize 按共用尺寸策略计算内嵌图片尺寸（EMU），并等比限制在正文可用宽度内。
func docxImageSize(asset ImageAsset) (int, int) {
	wMM, hMM := imageDisplaySizeMM(asset)
	cx := int(math.Round(wMM * emuPerMM))
	cy := int(math.Round(hMM * emuPerMM))
	if cx > docxContentWidthEMU {
		cy = cy * docxContentWidthEMU / cx
		cx = docxContentWidthEMU
	}
	if cx < 1 || cy < 1 {
		return docxContentWidthEMU, docxContentWidthEMU / 2
	}
	return cx, cy
}

// itemMetaLine 生成"题号 + 元信息"标题行，跳过空字段。
func itemMetaLine(index int, item ExportItem) string {
	parts := make([]string, 0, 4)
	for _, v := range []string{item.Grade, item.Subject, item.QuestionType} {
		if s := strings.TrimSpace(v); s != "" {
			parts = append(parts, s)
		}
	}
	if s := strings.TrimSpace(item.Source); s != "" {
		parts = append(parts, "来源："+s)
	}

	line := fmt.Sprintf("%d.", index)
	if len(parts) > 0 {
		line += " " + strings.Join(parts, " · ")
	}
	return line
}

// escapeXMLText 转义 XML 文本节点内容。
func escapeXMLText(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	// xml.EscapeText 会把换行转义为 &#xA;，便于在 Word 中保留换行。
	return buf.String()
}

// sanitizeXMLText 移除 XML 1.0 不允许出现的控制字符。
func sanitizeXMLText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return r
		case r < 0x20:
			return -1
		case r == 0xFFFE || r == 0xFFFF:
			return -1
		default:
			return r
		}
	}, s)
}
