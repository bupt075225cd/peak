// Package provider 定义第三方 AI 服务的能力级适配接口，屏蔽厂商差异，支持配置化切换。
package provider

import (
	"context"
	"strconv"
	"strings"
)

// TextResult 文本 OCR 结果。
type TextResult struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
}

// ErasureResult 手写擦除结果。
type ErasureResult struct {
	ImageData  []byte `json:"-"`           // 擦除后的图片字节
	StorageKey string `json:"storage_key"` // 若已存储，返回 key
}

// GeometryResult 几何图形识别结果（结构化描述）。
type GeometryResult struct {
	ShapeType   string            `json:"shape_type"` // triangle/circle/quadrilateral/...
	Properties  map[string]string `json:"properties"` // 结构化属性，如边长、角度
	Description string            `json:"description"`
	// BoundingBox 几何图形在原图中的位置（归一化坐标，0~1，原点左上角）。
	// 用于从原图中裁剪出“只有几何图”的子图，避免把题干文字一起展示。
	// 为 nil 表示模型未能定位图形。
	BoundingBox *BoundingBox `json:"bounding_box,omitempty"`
	// Rotation 图片内容相对正立状态顺时针倾斜的角度（0/90/180/270）。
	// 手机照片/截图经常侧躺，重绘前据此把裁剪出的几何区域转正：先 Degrees 得到倾斜角，
	// 再用 FixDegrees 换算成"顺时针转多少度才正立"的修正量交给 rotateImageData。
	// 注意方向：VLM 实测按"内容倾斜了多少度"作答（题干文字需要逆时针转 90° 才读正的图，
	// 它会答 90），所以这里按倾斜角定义；按修正量理解会让 90/270 互换、图片转反。
	Rotation Rotation `json:"rotation,omitempty"`
	// LineLabels 原图上**印刷**的直线名称（如 "l1"、"l2"），没有印刷则为空数组。
	// 题干里的 l₁/l₂ 只是代号，原图未必印了：重绘时只能按这个列表决定是否画名称标签。
	// 为 nil 表示模型没返回该字段（旧输出），调用方按"无从判断"保守处理。
	LineLabels []string `json:"line_labels,omitempty"`
}

// Rotation 图片内容相对正立状态的顺时针倾斜角度，仅接受 0/90/180/270。
// UnmarshalJSON 对 "90"、90.0 等非标准写法宽容处理，避免整个几何 JSON 解析失败。
type Rotation int

// UnmarshalJSON 宽容解析转正角度：非法值按 0 处理，不返回错误。
func (r *Rotation) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*r = 0
		return nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		*r = NormalizeRotation(int(f))
		return nil
	}
	*r = 0
	return nil
}

// NormalizeRotation 把角度折算为 0/90/180/270；偏离 90 度倍数超过 15 度按 0 处理。
func NormalizeRotation(deg int) Rotation {
	d := ((deg % 360) + 360) % 360
	switch {
	case d <= 15 || d >= 345:
		return 0
	case d >= 75 && d <= 105:
		return 90
	case d >= 165 && d <= 195:
		return 180
	case d >= 255 && d <= 285:
		return 270
	default:
		return 0
	}
}

// Degrees 返回图片内容顺时针倾斜的角度（0/90/180/270）。
func (r Rotation) Degrees() int {
	switch r {
	case 90, 180, 270:
		return int(r)
	default:
		return 0
	}
}

// FixDegrees 返回把图片顺时针旋转多少度才能转正（0/270/180/90），
// 即倾斜角的相反数；0 表示图片本来就正立，无需旋转。
func (r Rotation) FixDegrees() int {
	d := r.Degrees()
	if d == 0 {
		return 0
	}
	return 360 - d
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

// ErasureProvider 手写擦除能力。
type ErasureProvider interface {
	EraseHandwriting(ctx context.Context, image []byte) (*ErasureResult, error)
}

// GeometryProvider 几何图形识别能力。
type GeometryProvider interface {
	RecognizeGeometry(ctx context.Context, image []byte) (*GeometryResult, error)
}

// OrientationDetector 图片方向探测能力（可选能力，未实现的 provider 由调用方跳过）。
//
// 微信转发图、截图常常连 EXIF 方向都被剥掉了：像素可能侧躺或倒立，几何识别的
// bbox 与图形描述会整体错位（实测倒立图容易被判成"没有几何图形"）。这里让模型
// 先判一次倾斜角，调用方据此把整图转正，后续识别、裁剪、旋转共用正立坐标系。
type OrientationDetector interface {
	DetectOrientation(ctx context.Context, image []byte) (Rotation, error)
}

// UprightChecker 判断图片是否已经正立（可选能力，用于方向探测后的二次确认）。
//
// 让模型直接报"倾斜多少度"在 90/270 上很容易判混（实测同一张图三次里两次答错），
// 但问"这张图的文字能不能直接正常阅读"就直观得多：转错方向时答案是明确的否。
type UprightChecker interface {
	IsUpright(ctx context.Context, image []byte) (bool, error)
}

// Table 题干中的表格（分档计费表、数据对照表、统计表等）。
//
// 表格从题干正文里单独摘出来存：text 中对应位置只留 {{table1}} 占位符，由前端渲染成
// 表格、导出时序列化成 Markdown 表格。若让模型直接把表格拼成 "a | b | c" 的文本行，
// 行列关系全靠自由发挥，实测容易出现漏行、串列、整段丢失。
type Table struct {
	ID      string     `json:"id"`      // 占位名（table1、table2…）
	Headers []string   `json:"headers"` // 表头行；无表头时为空数组
	Rows    [][]string `json:"rows"`    // 数据行，每行单元格数与表头一致
}

// QuestionParseResult 整图一次解析结果：题干文本 + 学科 + 题型 + 题干表格。
type QuestionParseResult struct {
	Text         string  `json:"text"`
	Subject      string  `json:"subject"`       // 学科：数学/语文/英语/物理/化学
	QuestionType string  `json:"question_type"` // 题型：选择题/填空题/解答题
	Tables       []Table `json:"tables"`        // 题干中的表格（无表格时为空）
}

// QuestionParser 整题解析能力：一次 VLM 调用同时返回题干 OCR 文本与学科/题型
// （合并原 RecognizeText + ClassifyQuestion 两次调用，显著降低耗时）。
type QuestionParser interface {
	ParseQuestion(ctx context.Context, image []byte) (*QuestionParseResult, error)
}

// Provider 聚合接口，表示一个厂商提供的完整识别能力组合。
type Provider interface {
	Name() string
	OCRProvider
	ErasureProvider
	GeometryProvider
	QuestionParser
	DocumentProvider
	StructuredDocumentProvider
}
