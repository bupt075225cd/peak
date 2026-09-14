package provider

import (
	"context"
	"fmt"
	"strings"

	"peak/apps/recognition-service/internal/docparse"
)

// MockProvider 用于单元测试与无密钥本地跑通的 mock 实现。
type MockProvider struct{}

// NewMockProvider 创建 mock provider。
func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

func (m *MockProvider) Name() string { return "mock" }

func (m *MockProvider) RecognizeText(_ context.Context, image []byte) (*TextResult, error) {
	return &TextResult{
		Text:       fmt.Sprintf("mock ocr text (%d bytes)", len(image)),
		Confidence: 0.99,
	}, nil
}

func (m *MockProvider) ParseQuestion(_ context.Context, image []byte) (*QuestionParseResult, error) {
	return &QuestionParseResult{
		Text:         fmt.Sprintf("mock stem text (%d bytes)", len(image)),
		Subject:      "数学",
		QuestionType: "解答题",
	}, nil
}

func (m *MockProvider) EraseHandwriting(_ context.Context, image []byte) (*ErasureResult, error) {
	// mock 直接返回原图作为"擦除后"结果。
	return &ErasureResult{ImageData: image}, nil
}

func (m *MockProvider) RecognizeGeometry(_ context.Context, image []byte) (*GeometryResult, error) {
	return &GeometryResult{
		ShapeType:   "triangle",
		Properties:  map[string]string{"type": "right-angle", "note": fmt.Sprintf("%d bytes", len(image))},
		Description: "直角三角形 ABC，∠C = 90°",
		// 固定返回一个右下区域的外接矩形，便于流程测试裁剪路径。
		BoundingBox: &BoundingBox{X: 0.5, Y: 0.5, Width: 0.5, Height: 0.5},
	}, nil
}

// mockGeometrySpec 无外部依赖的完整坐标直出 spec（直角三角形 + 圆），
// 覆盖 segments/polygons/circles/arcs/right_angles/angle_marks/ticks/parallels/labels
// 全部图元，配合内置 Go 渲染器可端到端跑通"提取 → 校验 → SVG 渲染"链路。
const mockGeometrySpec = `{
  "title": "mock 几何重绘",
  "canvas": {"width": 100, "height": 80},
  "points": [
    {"name": "A", "x": 12, "y": 62},
    {"name": "B", "x": 88, "y": 62},
    {"name": "C", "x": 50, "y": 20},
    {"name": "O", "x": 50, "y": 62},
    {"name": "P", "x": 50, "y": 10}
  ],
  "segments": [
    {"from": "A", "to": "B"},
    {"from": "A", "to": "C"},
    {"from": "B", "to": "C"},
    {"from": "C", "to": "P", "extend": true, "dashed": true}
  ],
  "polygons": [{"points": ["A", "B", "C"], "fill": false}],
  "circles": [{"center": "O", "through": "A"}],
  "arcs": [{"center": "C", "radius": 10, "start_angle": 200, "end_angle": 340}],
  "right_angles": [{"vertex": "C", "a": "A", "b": "B"}],
  "angle_marks": [{"vertex": "A", "a": "B", "b": "C", "count": 1}],
  "ticks": [{"from": "A", "to": "C", "count": 1}],
  "parallels": [{"from": "A", "to": "B", "count": 1}],
  "labels": [{"x": 50, "y": 74, "text": "AB=AC", "anchor": "middle"}]
}`

// ExtractGeometrySpec mock 实现：返回固定的合法几何描述 spec，
// 使无密钥环境也能端到端验证几何重绘链路。
func (m *MockProvider) ExtractGeometrySpec(_ context.Context, _ []byte, _, _ string) (string, error) {
	return mockGeometrySpec, nil
}

// ExtractDocument mock 实现：本地解析文档（无需第三方），返回文本与内嵌图片。
func (m *MockProvider) ExtractDocument(_ context.Context, data []byte, filename string) (*DocumentResult, error) {
	res, err := docparse.Parse(data, filename)
	if err != nil {
		return nil, err
	}
	items := make([]DocumentItem, 0, len(res.Items))
	for _, it := range res.Items {
		items = append(items, DocumentItem{Kind: it.Kind, Text: it.Text, Image: it.Image})
	}
	return &DocumentResult{Items: items, PageCount: res.PageCount}, nil
}

// ExtractStructured mock 实现：本地解析文档，简单按题号正则拆题（不调用第三方）。
func (m *MockProvider) ExtractStructured(_ context.Context, data []byte, filename string) (*StructuredResult, error) {
	res, err := docparse.Parse(data, filename)
	if err != nil {
		return nil, err
	}
	// 拼接所有文本项，按题号正则拆分为多道题。
	var items []StructuredItem
	var cur *StructuredItem
	for _, it := range res.Items {
		if it.Kind != "text" {
			continue
		}
		if questionNoRe.MatchString(it.Text) {
			if cur != nil && strings.TrimSpace(cur.StemText) != "" {
				items = append(items, *cur)
			}
			cur = &StructuredItem{StemText: strings.TrimSpace(it.Text)}
		} else if cur != nil {
			cur.StemText += "\n" + strings.TrimSpace(it.Text)
		} else {
			cur = &StructuredItem{StemText: strings.TrimSpace(it.Text)}
		}
	}
	if cur != nil && strings.TrimSpace(cur.StemText) != "" {
		items = append(items, *cur)
	}
	if len(items) == 0 {
		var sb strings.Builder
		for _, it := range res.Items {
			if it.Kind == "text" {
				sb.WriteString(strings.TrimSpace(it.Text))
				sb.WriteString("\n")
			}
		}
		if s := strings.TrimSpace(sb.String()); s != "" {
			items = append(items, StructuredItem{StemText: s})
		}
	}
	return &StructuredResult{Items: items, PageCount: res.PageCount}, nil
}
