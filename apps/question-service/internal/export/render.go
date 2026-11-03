package export

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/fogleman/gg"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
)

// renderConfig 题目位图渲染参数（单位：像素）。
type renderConfig struct {
	width       int     // 画布宽度
	padding     int     // 内边距
	gap         int     // 段落间距
	metaSize    float64 // 元信息字号
	stemSize    float64 // 题干字号
	lineSpacing float64 // 行高倍数
}

// renderBaseWidth 排版基准宽度：字号与间距按该宽度设计，实际整体等比放大。
const renderBaseWidth = 1000

// defaultRenderConfig 返回默认渲染参数。
//
// 画布宽度直接决定 PDF 中的打印清晰度（见 pdfRenderWidth），这里所有尺寸按
// 同一比例放大，保证字号、间距与配图的相对比例与基准设计一致。
func defaultRenderConfig() renderConfig {
	scale := float64(pdfRenderWidth) / renderBaseWidth
	return renderConfig{
		width:       pdfRenderWidth,
		padding:     int(math.Round(36 * scale)),
		gap:         int(math.Round(20 * scale)),
		metaSize:    20 * scale,
		stemSize:    22 * scale,
		lineSpacing: 1.6,
	}
}

// spaceBreakMinFill 空格断行的最小填充比例：仅当在空格断行后该行宽度仍不低于可用
// 宽度的该比例时才断在空格处，否则按字符断行，避免行尾参差与"换行太早"。
const spaceBreakMinFill = 0.6

// imageRowGapScale 同一行内相邻配图之间的水平间距相对段落间距（renderConfig.gap）的倍数。
//
// 并排的几何图之间需要比段落间距更明显的间隔，否则两张图挨得太近、容易被看成一整张。
const imageRowGapScale = 3.0

// 渲染配色。
var (
	colorInk     = color.RGBA{R: 0x11, G: 0x18, B: 0x27, A: 0xff} // 题干
	colorMeta    = color.RGBA{R: 0x1f, G: 0x29, B: 0x37, A: 0xff} // 元信息
	colorWarning = color.RGBA{R: 0xb4, G: 0x53, B: 0x09, A: 0xff} // 配图失败提示
	colorPaper   = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	// colorTableHead 题干表格的表头底色，和正文行区分开。
	colorTableHead = color.RGBA{R: 0xf1, G: 0xf5, B: 0xf9, A: 0xff}
)

// itemLayout 单题渲染结果：位图字节、像素尺寸与可安全断页的位置。
type itemLayout struct {
	data   []byte
	width  int
	height int
	// breaks 可安全断页的 y 像素位置（段落、文字行与配图的起始处）。
	// 配图内部不含断点，保证分页时不会把图形从中间截断。
	breaks []int
}

// renderItemImage 把一道错题渲染为 PNG 位图，返回字节与像素尺寸。
func renderItemImage(rc renderConfig, fonts *FontProvider, index int, it renderItem) (data []byte, width, height int, err error) {
	layout, err := renderItemLayout(rc, fonts, index, it)
	if err != nil {
		return nil, 0, 0, err
	}
	return layout.data, layout.width, layout.height, nil
}

// renderItemLayout 把一道错题渲染为 PNG 位图，并记录可安全断页的位置。
//
// 高度先按文本行数与配图尺寸算好，再创建画布绘制，避免二次裁剪。
func renderItemLayout(rc renderConfig, fonts *FontProvider, index int, it renderItem) (itemLayout, error) {
	metaFace, err := fonts.Face(rc.metaSize)
	if err != nil {
		return itemLayout{}, err
	}
	stemFace, err := fonts.Face(rc.stemSize)
	if err != nil {
		return itemLayout{}, err
	}

	contentWidth := float64(rc.width - 2*rc.padding)
	if contentWidth <= 0 {
		return itemLayout{}, fmt.Errorf("render width %d is too small", rc.width)
	}

	metaLines := wrapText(metaFace, itemMetaLine(index, it.item), contentWidth)
	stemText := strings.TrimSpace(it.item.StemText)

	metaLineH := lineHeight(metaFace, rc.lineSpacing)
	stemLineH := lineHeight(stemFace, rc.lineSpacing)

	// 题干先切成"段落 + 表格"块：表格由识别结果的 Markdown 表格还原，
	// 段落按可用宽度折行，两者高度一起参与画布尺寸预算。
	parts := make([]stemPart, 0, 4)
	for _, b := range parseStemBlocks(stemText) {
		if b.table != nil {
			if lay := layoutStemTable(stemFace, b.table, contentWidth, stemLineH); lay != nil {
				parts = append(parts, stemPart{table: lay, height: lay.height})
			}
			continue
		}
		lines := wrapText(stemFace, b.text, contentWidth)
		parts = append(parts, stemPart{lines: lines, height: float64(len(lines)) * stemLineH})
	}

	placed, err := decodeImages(it.images, contentWidth)
	if err != nil {
		return itemLayout{}, err
	}

	hasStem := len(parts) > 0
	hasNote := it.imageFailed

	rowGap := float64(rc.gap) * imageRowGapScale
	captionLineH := lineHeight(metaFace, rc.lineSpacing)
	rows := groupImageRows(placed, contentWidth, rowGap, captionLineH)

	totalHeight := float64(rc.padding) + float64(len(metaLines))*metaLineH
	if hasStem {
		totalHeight += float64(rc.gap)
		for _, p := range parts {
			totalHeight += p.height
		}
	}
	if hasNote {
		totalHeight += float64(rc.gap) + metaLineH
	}
	for _, row := range rows {
		totalHeight += float64(rc.gap) + row.height
	}
	totalHeight += float64(rc.padding)

	canvasH := int(math.Ceil(totalHeight))
	if canvasH < 1 {
		canvasH = 1
	}

	dc := gg.NewContext(rc.width, canvasH)
	dc.SetColor(colorPaper)
	dc.Clear()

	breaks := make([]int, 0, len(metaLines)+len(parts)+len(rows)+2)

	x := float64(rc.padding)
	y := float64(rc.padding)

	breaks = append(breaks, lineBreaks(y, metaLineH, len(metaLines))...)
	dc.SetColor(colorMeta)
	y = drawTextLines(dc, metaFace, metaLines, x, y, metaLineH)

	if hasStem {
		y += float64(rc.gap)
		breaks = append(breaks, int(math.Round(y)))
		dc.SetColor(colorInk)
		for _, p := range parts {
			if p.table != nil {
				breaks = append(breaks, tableLineBreaks(y, p.table)...)
				y = drawStemTable(dc, stemFace, p.table, x, y)
				dc.SetColor(colorInk)
				continue
			}
			breaks = append(breaks, lineBreaks(y, stemLineH, len(p.lines))...)
			y = drawTextLines(dc, stemFace, p.lines, x, y, stemLineH)
		}
	}

	if hasNote {
		y += float64(rc.gap)
		breaks = append(breaks, int(math.Round(y)))
		dc.SetColor(colorWarning)
		y = drawTextLines(dc, metaFace, []string{"（配图加载失败）"}, x, y, metaLineH)
	}

	// 图注用元信息色（比题干浅、字更小），与配图一起构成"图 + 图号"块。
	dc.SetColor(colorMeta)
	for _, row := range rows {
		y += float64(rc.gap)
		// 每一行配图的起始是安全断点；图与图注同在一行内，分页时不会截断图形或图注。
		breaks = append(breaks, int(math.Round(y)))

		// 整行水平居中；行内配图底边对齐，图注排在行底统一的图注带里。
		x := float64(rc.padding)
		if row.width < contentWidth {
			x += (contentWidth - row.width) / 2
		}
		captionY := y + row.height - row.captionH
		for _, p := range row.images {
			drawImageAt(dc, p, x, captionY-p.h)
			drawCaption(dc, metaFace, p.caption, x, captionY, p.w)
			x += p.w + rowGap
		}
		y += row.height
	}

	var buf bytes.Buffer
	if err := dc.EncodePNG(&buf); err != nil {
		return itemLayout{}, fmt.Errorf("encode item image: %w", err)
	}
	return itemLayout{data: buf.Bytes(), width: rc.width, height: canvasH, breaks: breaks}, nil
}

// lineBreaks 返回文本块内每一行的起始 y，作为可安全断页的位置。
func lineBreaks(y, lineH float64, count int) []int {
	out := make([]int, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, int(math.Round(y+float64(i)*lineH)))
	}
	return out
}

// imageRow 同一行内水平排列的配图。
type imageRow struct {
	images []placedImage
	// width 行内配图总宽（含行内间距），用于整行居中。
	width float64
	// height 行高：行内最高配图 + 图注带。
	height float64
	// captionH 行底图注带高度（行内无图注时为 0）：配图底边对齐、图注成行。
	captionH float64
}

// groupImageRows 把配图按可用宽度水平排列：一行放得下就并排，放不下才换行。
//
// 同一题的多个几何图（图1、图2…）并排展示比每图独占一行更紧凑，也更接近试卷版式。
// 单张配图超过可用宽度时仍独占一行。带图注的行在底部预留一条与字号等高的图注带。
func groupImageRows(placed []placedImage, contentWidth, gap, captionH float64) []imageRow {
	rows := make([]imageRow, 0, 2)
	var cur imageRow

	for _, p := range placed {
		// 尺寸过小无法绘制的图片直接忽略，避免影响行宽计算。
		if int(math.Round(p.w)) < 1 || int(math.Round(p.h)) < 1 {
			continue
		}
		if len(cur.images) > 0 && cur.width+gap+p.w > contentWidth {
			rows = append(rows, cur)
			cur = imageRow{}
		}
		if len(cur.images) > 0 {
			cur.width += gap
		}
		cur.images = append(cur.images, p)
		cur.width += p.w
		if p.h > cur.height {
			cur.height = p.h
		}
		if p.caption != "" {
			cur.captionH = captionH
		}
	}
	if len(cur.images) > 0 {
		rows = append(rows, cur)
	}
	// 图注带统一计入行高：行内配图底边对齐，图注落在带内居中。
	for i := range rows {
		rows[i].height += rows[i].captionH
	}
	return rows
}

// drawCaption 在配图正下方水平居中绘制图注（如"图1"）；空图注不绘制。
func drawCaption(dc *gg.Context, face font.Face, caption string, x, y, width float64) {
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return
	}
	dc.SetFontFace(face)
	ascent := float64(face.Metrics().Ascent) / 64.0
	textWidth := runesWidth(face, []rune(caption))
	dc.DrawString(caption, x+(width-textWidth)/2, y+ascent)
}

// drawImageAt 把配图按显示尺寸缩放后绘制到画布指定位置。
func drawImageAt(dc *gg.Context, p placedImage, x, y float64) {
	dw, dh := int(math.Round(p.w)), int(math.Round(p.h))
	if dw < 1 || dh < 1 {
		return
	}
	scaled := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), p.src, p.src.Bounds(), draw.Over, nil)
	dc.DrawImage(scaled, int(math.Round(x)), int(math.Round(y)))
}

// placedImage 已解码并计算好显示尺寸的配图。
type placedImage struct {
	src  image.Image
	w, h float64
	// caption 图注（如"图1"）；为空表示该图不加标注。
	caption string
}

// decodeImages 解码配图并按共用尺寸策略计算显示尺寸，解码失败的图片直接跳过。
func decodeImages(assets []ImageAsset, contentWidth float64) ([]placedImage, error) {
	out := make([]placedImage, 0, len(assets))

	for _, asset := range assets {
		if asset.Width <= 0 || asset.Height <= 0 {
			continue
		}
		src, _, err := image.Decode(bytes.NewReader(asset.Data))
		if err != nil {
			continue
		}
		wMM, hMM := imageDisplaySizeMM(asset)
		w := wMM * pdfPixelsPerMM
		h := hMM * pdfPixelsPerMM
		// 兜底：不超过正文可用宽度。
		if w > contentWidth {
			h = h * contentWidth / w
			w = contentWidth
		}
		out = append(out, placedImage{src: src, w: w, h: h, caption: strings.TrimSpace(asset.Caption)})
	}
	return out, nil
}

// stemPart 题干的一个排版块：普通段落（lines）或表格（table），height 为占用高度。
type stemPart struct {
	lines  []string
	table  *stemTableLayout
	height float64
}

// 表格单元格内边距：水平留白大一点（列之间不拥挤），垂直用小值（行高接近常规表格）。
const (
	stemTableCellPadX = 8.0
	stemTableCellPadY = 4.0
)

// stemTableBottomGap 表格与后续段落之间的间距。
const stemTableBottomGap = 12.0

// stemTableMinColWidth 列的最小宽度，避免窄列把汉字挤成竖排。
const stemTableMinColWidth = 28.0

// stemTableLayout 表格排版结果：列宽、每行单元格折行文本与行高。
type stemTableLayout struct {
	cols      []float64
	cells     [][][]string // cells[row][col] 为折行后的文本
	rowH      []float64
	height    float64
	lineH     float64
	hasHeader bool
}

// layoutStemTable 计算表格排版：列宽按内容自然宽度分配，超出可用宽度时等比压缩，
// 单元格文本按列宽折行；表头行与数据行同字号，靠分隔线与居中排版区分。
func layoutStemTable(face font.Face, tbl *stemTable, contentWidth, lineH float64) *stemTableLayout {
	cols := tbl.columnCount()
	if cols == 0 {
		return nil
	}
	widths := make([]float64, cols)
	for c := 0; c < cols; c++ {
		w := 0.0
		for r := -1; r < len(tbl.rows); r++ {
			if s := runesWidth(face, []rune(tbl.cell(r, c))); s > w {
				w = s
			}
		}
		widths[c] = w + 2*stemTableCellPadX
		if widths[c] < stemTableMinColWidth {
			widths[c] = stemTableMinColWidth
		}
	}
	total := 0.0
	for _, w := range widths {
		total += w
	}
	if total > contentWidth && total > 0 {
		ratio := contentWidth / total
		total = 0
		for i := range widths {
			widths[i] *= ratio
			if widths[i] < stemTableMinColWidth {
				widths[i] = stemTableMinColWidth
			}
			total += widths[i]
		}
	}

	// 单元格内行距比正文紧凑（正文行距是为阅读留白，表格行不需要那么多）。
	cellLineH := lineHeight(face, 1.25)
	if cellLineH > lineH {
		cellLineH = lineH
	}
	lay := &stemTableLayout{
		cols:      widths,
		cells:     make([][][]string, 0, tbl.rowCount()),
		rowH:      make([]float64, 0, tbl.rowCount()),
		lineH:     cellLineH,
		hasHeader: len(tbl.header) > 0,
	}
	for r := 0; r < tbl.rowCount(); r++ {
		// rowCount 把表头算作第 0 行；tbl.cell 用 -1 表示表头行。
		ri := r
		if lay.hasHeader {
			ri = r - 1
		}
		rowCells := make([][]string, cols)
		maxLines := 1
		for c := 0; c < cols; c++ {
			lines := wrapText(face, tbl.cell(ri, c), widths[c]-2*stemTableCellPadX)
			rowCells[c] = lines
			if len(lines) > maxLines {
				maxLines = len(lines)
			}
		}
		lay.cells = append(lay.cells, rowCells)
		h := float64(maxLines)*cellLineH + 2*stemTableCellPadY
		lay.rowH = append(lay.rowH, h)
		lay.height += h
	}
	// 表格与后续段落之间留白（高度预算和绘制都按同一值）。
	lay.height += stemTableBottomGap
	return lay
}

// tableLineBreaks 表格内每一行的起始 y，作为可安全断页的位置。
func tableLineBreaks(y float64, lay *stemTableLayout) []int {
	out := make([]int, 0, len(lay.rowH))
	cur := y
	for _, h := range lay.rowH {
		out = append(out, int(math.Round(cur)))
		cur += h
	}
	return out
}

// drawStemTable 绘制题干表格（外框 + 行列线 + 单元格文本），返回表格底部的 y。
func drawStemTable(dc *gg.Context, face font.Face, lay *stemTableLayout, x, y float64) float64 {
	dc.SetFontFace(face)
	ascent := float64(face.Metrics().Ascent) / 64.0
	totalW := 0.0
	for _, w := range lay.cols {
		totalW += w
	}
	bottom := y + lay.height - stemTableBottomGap

	// 表头底色：与数据行区分（PDF 只有一种字重，靠底色而非加粗）。
	if lay.hasHeader && len(lay.rowH) > 0 {
		dc.SetColor(colorTableHead)
		dc.DrawRectangle(x, y, totalW, lay.rowH[0])
		dc.Fill()
	}

	// gg 的 DrawLine 只是把线段累积进当前路径，必须 Stroke 才真正落笔——
	// 少了这一步，表格只剩对齐的文本、看不到任何框线。
	dc.SetColor(colorInk)
	dc.SetLineWidth(1)
	dc.DrawLine(x, y, x+totalW, y)
	cur := y
	for _, h := range lay.rowH {
		cur += h
		dc.DrawLine(x, cur, x+totalW, cur)
	}
	cx := x
	dc.DrawLine(cx, y, cx, bottom)
	for _, w := range lay.cols {
		cx += w
		dc.DrawLine(cx, y, cx, bottom)
	}
	dc.Stroke()

	cur = y
	for r, h := range lay.rowH {
		cx = x
		for c, w := range lay.cols {
			lines := lay.cells[r][c]
			top := cur + (h-float64(len(lines))*lay.lineH)/2
			for i, ln := range lines {
				if ln == "" {
					continue
				}
				baseline := top + float64(i)*lay.lineH + ascent
				if lay.hasHeader && r == 0 {
					dc.DrawStringAnchored(ln, cx+w/2, baseline, 0.5, 0)
					continue
				}
				dc.DrawString(ln, cx+stemTableCellPadX, baseline)
			}
			cx += w
		}
		cur += h
	}
	return y + lay.height
}

// drawTextLines 逐行绘制文本，返回绘制后下一段的 y 起点。
func drawTextLines(dc *gg.Context, face font.Face, lines []string, x, y, lineH float64) float64 {
	dc.SetFontFace(face)
	ascent := float64(face.Metrics().Ascent) / 64.0
	for i, line := range lines {
		if line == "" {
			continue
		}
		dc.DrawString(line, x, y+float64(i)*lineH+ascent)
	}
	return y + float64(len(lines))*lineH
}

// lineHeight 返回按行距倍数放大的行高。
func lineHeight(face font.Face, spacing float64) float64 {
	if spacing < 1 {
		spacing = 1
	}
	return float64(face.Metrics().Height) / 64.0 * spacing
}

// wrapText 按可用宽度对文本换行，支持显式换行符。
//
// 中文之间没有空格，必须逐字符测量宽度；同时优先在空格处断行，
// 避免把英文单词从中间截断。
func wrapText(face font.Face, text string, maxWidth float64) []string {
	if maxWidth <= 0 {
		return []string{text}
	}

	out := make([]string, 0, 8)
	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimRight(para, "\r")
		if para == "" {
			out = append(out, "")
			continue
		}

		line := make([]rune, 0, len(para))
		var width float64
		for _, r := range para {
			rw := runeWidth(face, r)
			if width+rw > maxWidth && len(line) > 0 {
				cut := len(line)
				// 优先在空格处断行（避免切断英文单词），但仅当断点距行尾足够近；
				// 否则宁可按字符断行，避免行尾大面积留白（"换行太早"）。
				for i := len(line) - 1; i >= 0; i-- {
					if line[i] == ' ' {
						if runesWidth(face, line[:i]) >= maxWidth*spaceBreakMinFill {
							cut = i
						}
						break
					}
				}
				out = append(out, string(line[:cut]))

				rest := make([]rune, len(line)-cut)
				copy(rest, line[cut:])
				line = rest
				for len(line) > 0 && line[0] == ' ' {
					line = line[1:]
				}

				width = runesWidth(face, line)
			}
			line = append(line, r)
			width += rw
		}
		if len(line) > 0 {
			out = append(out, string(line))
		}
	}

	if len(out) == 0 {
		out = append(out, "")
	}
	return out
}

// runeWidth 返回单个字符的显示宽度（像素）。
func runeWidth(face font.Face, r rune) float64 {
	return float64(font.MeasureString(face, string(r))) / 64.0
}

// runesWidth 返回一段文本的显示宽度（像素）。
func runesWidth(face font.Face, rs []rune) float64 {
	var w float64
	for _, r := range rs {
		w += runeWidth(face, r)
	}
	return w
}
