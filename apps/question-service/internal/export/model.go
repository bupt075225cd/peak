// Package export 提供错题导出能力：把错题视图模型渲染为 PDF 或 Word 文档。
//
// 本包不感知数据库：调用方组装 ExportItem 后交给 Service，由本包负责取图、
// 规整、渲染与文档组装。
package export

import (
	"fmt"
	"strings"
)

// Format 导出格式。
type Format string

const (
	// FormatPDF PDF 格式（每题渲染为位图后按 A4 排布）。
	FormatPDF Format = "pdf"
	// FormatDocx Word 文档格式（.docx）。
	FormatDocx Format = "docx"
)

// ParseFormat 解析导出格式字符串，非法格式返回错误。
func ParseFormat(raw string) (Format, error) {
	switch f := Format(strings.ToLower(strings.TrimSpace(raw))); f {
	case FormatPDF, FormatDocx:
		return f, nil
	default:
		return "", fmt.Errorf("unsupported export format %q", raw)
	}
}

// ContentType 返回该格式对应的 HTTP Content-Type。
func (f Format) ContentType() string {
	switch f {
	case FormatPDF:
		return "application/pdf"
	case FormatDocx:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}

// Extension 返回该格式的文件扩展名（含点）。
func (f Format) Extension() string {
	switch f {
	case FormatPDF:
		return ".pdf"
	case FormatDocx:
		return ".docx"
	default:
		return ".bin"
	}
}

// ExportItem 导出的单道错题视图模型。
type ExportItem struct {
	Grade        string   // 年级
	Subject      string   // 学科
	QuestionType string   // 题型
	Source       string   // 题目来源
	StemText     string   // 题干
	ImageKeys    []string // 配图存储 key 列表（几何图或其它科目插图）
}

// ImageAsset 已规整为可直接嵌入文档的位图。
type ImageAsset struct {
	Data   []byte // 编码后的图片字节
	Format string // 图片格式："png" / "jpeg"
	Width  int    // 像素宽
	Height int    // 像素高
	// NaturalWidth/NaturalHeight 为图片的原始尺寸：位图为解码后的原始像素，
	// SVG 为其 viewBox 尺寸。用于限制低分辨率图片的放大倍数（见 layout.go）。
	NaturalWidth  int
	NaturalHeight int
	// Vector 表示该图片由矢量图（SVG）光栅化而来，缩放不受分辨率限制。
	Vector bool
}

// Result 导出结果。
type Result struct {
	Data     []byte   // 文件字节
	Filename string   // 建议文件名
	Warnings []string // 非致命问题（如个别配图加载失败）
}

// renderItem 已加载好配图的导出条目，供各格式 writer 使用。
type renderItem struct {
	item        ExportItem
	images      []ImageAsset
	imageFailed bool // 该题配置了配图但全部加载失败
}
