package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"peak/apps/recognition-service/internal/docparse"
)

// vlmCapabilities 基于任意 OpenAI 兼容多模态对话客户端（ChatClient）实现的
// 通用 VLM 能力集合：整题解析、OCR、几何识别、文档解析、结构化拆题、几何描述提取。
// 由 aliyun（通义千问-VL）与 zhipu（智谱 GLM）等 provider 内嵌复用，
// 各厂商只需提供 ChatClient 配置与自身特有的能力（如阿里云的手写擦除）。
type vlmCapabilities struct {
	dash *ChatClient
}

// RecognizeText 调用多模态大模型提取图片中的全部文字（文档/子图 OCR 场景使用）。
func (v *vlmCapabilities) RecognizeText(ctx context.Context, image []byte) (*TextResult, error) {
	prompt := "请识别图片中的全部文字内容，按阅读顺序原样输出，不要添加任何解释。"
	out, err := v.dash.chat(ctx, prompt, image)
	if err != nil {
		return nil, err
	}
	return &TextResult{Text: out, Confidence: 0.9}, nil
}

// ParseQuestion 一次 VLM 调用完成整题解析：题干文本（OCR）+ 学科 + 题型。
func (v *vlmCapabilities) ParseQuestion(ctx context.Context, image []byte) (*QuestionParseResult, error) {
	prompt := `识别图片中的这道题目，仅输出如下 JSON（不要 Markdown 代码块、不要解释）：
{"text":"题干原文（按阅读顺序原样输出，不要修改、不要省略）","subject":"数学|语文|英语|物理|化学","question_type":"选择题|填空题|解答题"}

要求：
- text 必须包含题目的完整题干文字（含数字与单位，若图中仅有图形则输出空字符串）。
- subject 只能从 数学/语文/英语/物理/化学 中选择；question_type 只能从 选择题/填空题/解答题 中选择。不确定时用最接近的值。`
	out, err := v.dash.chat(ctx, prompt, image)
	if err != nil {
		return nil, err
	}
	return parseQuestion(out), nil
}

// parseQuestion 解析整题解析 JSON；容错非 JSON 输出时把原文整体当作题干文本。
func parseQuestion(out string) *QuestionParseResult {
	obj, err := extractJSON(out)
	if err != nil {
		// 模型未按 JSON 输出：整段文本作为题干，学科/题型留空交给规则判断兜底。
		return &QuestionParseResult{Text: strings.TrimSpace(out)}
	}
	var r struct {
		Text         string `json:"text"`
		Subject      string `json:"subject"`
		QuestionType string `json:"question_type"`
	}
	if err := json.Unmarshal([]byte(obj), &r); err != nil {
		return &QuestionParseResult{Text: strings.TrimSpace(out)}
	}
	res := &QuestionParseResult{
		Text:         strings.TrimSpace(r.Text),
		Subject:      strings.TrimSpace(r.Subject),
		QuestionType: strings.TrimSpace(r.QuestionType),
	}
	if res.Text == "" {
		res.Text = strings.TrimSpace(out)
	}
	return res
}

// RecognizeGeometry 调用多模态大模型输出结构化几何描述（JSON），并附带几何图形在原图中的位置框。
func (v *vlmCapabilities) RecognizeGeometry(ctx context.Context, image []byte) (*GeometryResult, error) {
	prompt := `识别图片中的几何图形，仅输出如下 JSON（不要输出其他内容、不要用 Markdown 代码块）：
{"shape_type":"triangle|circle|quadrilateral|other","properties":{"边长":"","角度":"","位置关系":""},"description":"对图形的文字描述","bounding_box":{"x":0.1,"y":0.2,"width":0.5,"height":0.4}}

其中 bounding_box 是几何图形（仅图形本身，不含题目文字）在原图中的外接矩形，坐标采用归一化（0~1），x/y 为左上角，width/height 为宽高。
重要：若图中有多块几何图形（例如多个子图、图1/图2/图3），请输出包含「所有几何图形整体」的最大外接矩形，确保完整覆盖每一个图，不要只框出其中一部分。适度宽松即可，不要缩得太紧。若图片中没有几何图形，bounding_box 设为 null。`
	out, err := v.dash.chat(ctx, prompt, image)
	if err != nil {
		return nil, err
	}
	return parseGeometry(out), nil
}

// parseGeometry 解析几何识别返回的几何 JSON（容错处理非 JSON 输出）。
func parseGeometry(out string) *GeometryResult {
	var g GeometryResult
	if err := json.Unmarshal([]byte(out), &g); err != nil {
		// 若模型未按 JSON 输出，将原文放入 description 兜底。
		return &GeometryResult{
			ShapeType:   "unknown",
			Properties:  map[string]string{},
			Description: out,
		}
	}
	g.BoundingBox = normalizeBBox(g.BoundingBox)
	return &g
}

// normalizeBBox 规范化几何图形外接矩形：
//   - 越界/非法坐标（<0 或 >1、宽高非正、x+width>1 等）视为无效，返回 nil。
//   - 有效时 clamp 到 [0,1] 区间，保证服务端裁剪安全。
func normalizeBBox(b *BoundingBox) *BoundingBox {
	if b == nil {
		return nil
	}
	if b.Width <= 0 || b.Height <= 0 {
		return nil
	}
	clamp01 := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	x := clamp01(b.X)
	y := clamp01(b.Y)
	w := clamp01(b.Width)
	h := clamp01(b.Height)
	// 宽高越界修正：不能超出图片范围。
	if x+w > 1 {
		w = 1 - x
	}
	if y+h > 1 {
		h = 1 - y
	}
	if w <= 0 || h <= 0 {
		return nil
	}
	return &BoundingBox{X: x, Y: y, Width: w, Height: h}
}

// ExtractDocument 文档识别：本地解析 word/pdf 提取文本与内嵌图片，
// 图片项复用多模态大模型做视觉理解，产出结构化文本。
func (v *vlmCapabilities) ExtractDocument(ctx context.Context, data []byte, filename string) (*DocumentResult, error) {
	res, err := docparse.Parse(data, filename)
	if err != nil {
		return nil, err
	}

	items := make([]DocumentItem, 0, len(res.Items))
	for _, it := range res.Items {
		switch it.Kind {
		case "text":
			items = append(items, DocumentItem{Kind: "text", Text: it.Text})
		case "image":
			// 对文档内嵌图片调用多模态大模型做 OCR，提取图片中的题目文本。
			text, err := v.ocrImageText(ctx, it.Image)
			if err != nil {
				// 图片 OCR 失败时保留图片项，由上层降级处理。
				items = append(items, DocumentItem{Kind: "image", Image: it.Image})
				continue
			}
			items = append(items, DocumentItem{Kind: "text", Text: text})
		}
	}

	return &DocumentResult{Items: items, PageCount: res.PageCount}, nil
}

// ocrImageText 调用多模态大模型识别图片中的文字/题目内容。
func (v *vlmCapabilities) ocrImageText(ctx context.Context, image []byte) (string, error) {
	prompt := "识别图片中的全部文字与题目内容，按顺序输出纯文本，保留公式与几何描述。"
	out, err := v.dash.chat(ctx, prompt, image)
	if err != nil {
		return "", fmt.Errorf("vlm ocr: %w", err)
	}
	return out, nil
}
