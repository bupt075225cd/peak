// Package provider 定义第三方 AI 服务的能力级适配接口，屏蔽厂商差异，支持配置化切换。
package provider

import "context"

// TextResult 文本 OCR 结果。
type TextResult struct {
	Text     string `json:"text"`
	Confidence float64 `json:"confidence"`
}

// FormulaResult 公式识别结果（LaTeX 结构）。
type FormulaResult struct {
	LaTeX    string `json:"latex"`
	RawText  string `json:"raw_text"`
}

// ErasureResult 手写擦除结果。
type ErasureResult struct {
	ImageData []byte `json:"-"`        // 擦除后的图片字节
	StorageKey string `json:"storage_key"` // 若已存储，返回 key
}

// GeometryResult 几何图形识别结果（结构化描述）。
type GeometryResult struct {
	ShapeType string            `json:"shape_type"` // triangle/circle/quadrilateral/...
	Properties map[string]string `json:"properties"` // 结构化属性，如边长、角度
	Description string          `json:"description"`
	// BoundingBox 几何图形在原图中的位置（归一化坐标，0~1，原点左上角）。
	// 用于从原图中裁剪出“只有几何图”的子图，避免把题干文字一起展示。
	// 为 nil 表示模型未能定位图形。
	BoundingBox *BoundingBox `json:"bounding_box,omitempty"`
}

// BoundingBox 几何图形在原图中的外接矩形，采用归一化坐标（0~1）。
// 服务端根据原图实际宽高换算成像素后裁剪。
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// OCRProvider 文本 OCR 能力。
type OCRProvider interface {
	RecognizeText(ctx context.Context, image []byte) (*TextResult, error)
}

// FormulaProvider 公式识别能力。
type FormulaProvider interface {
	RecognizeFormula(ctx context.Context, image []byte) (*FormulaResult, error)
}

// ErasureProvider 手写擦除能力。
type ErasureProvider interface {
	EraseHandwriting(ctx context.Context, image []byte) (*ErasureResult, error)
}

// GeometryProvider 几何图形识别能力。
type GeometryProvider interface {
	RecognizeGeometry(ctx context.Context, image []byte) (*GeometryResult, error)
}

// Provider 聚合接口，表示一个厂商提供的完整识别能力组合。
type Provider interface {
	Name() string
	OCRProvider
	FormulaProvider
	ErasureProvider
	GeometryProvider
	DocumentProvider
	StructuredDocumentProvider
}
