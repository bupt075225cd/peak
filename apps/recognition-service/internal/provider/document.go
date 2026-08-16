package provider

import "context"

// DocumentItem 文档中解析出的单个内容项（文本段落或图片）。
type DocumentItem struct {
	// Kind 内容类型：text / image。
	Kind string `json:"kind"`
	// Text 文本内容（Kind=text 时有效）。
	Text string `json:"text,omitempty"`
	// Image 图片字节（Kind=image 时有效）。
	Image []byte `json:"-"`
}

// DocumentResult 文档解析结果（按文档顺序混排文本与图片）。
type DocumentResult struct {
	// Items 按出现顺序排列的内容项。
	Items []DocumentItem `json:"items"`
	// PageCount 页数（pdf）。
	PageCount int `json:"page_count"`
}

// DocumentProvider 文档识别能力：解析 word/pdf 文档，提取文本与内嵌图片。
// 文档中的图片由上层继续调用 OCR/公式/几何等能力处理。
type DocumentProvider interface {
	// ExtractDocument 解析文档字节，返回混排内容项。
	// filename 用于判断文档格式（.docx / .pdf）。
	ExtractDocument(ctx context.Context, data []byte, filename string) (*DocumentResult, error)
}
