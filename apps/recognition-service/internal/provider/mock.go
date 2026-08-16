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

func (m *MockProvider) RecognizeFormula(_ context.Context, image []byte) (*FormulaResult, error) {
	return &FormulaResult{
		LaTeX:   "x^2 + y^2 = r^2",
		RawText: fmt.Sprintf("mock formula (%d bytes)", len(image)),
	}, nil
}

func (m *MockProvider) EraseHandwriting(_ context.Context, image []byte) (*ErasureResult, error) {
	// mock 直接返回原图作为"擦除后"结果。
	return &ErasureResult{ImageData: image}, nil
}

func (m *MockProvider) RecognizeGeometry(_ context.Context, image []byte) (*GeometryResult, error) {
	return &GeometryResult{
		ShapeType:  "triangle",
		Properties: map[string]string{"type": "right-angle", "note": fmt.Sprintf("%d bytes", len(image))},
		Description: "直角三角形 ABC，∠C = 90°",
		// 固定返回一个右下区域的外接矩形，便于流程测试裁剪路径。
		BoundingBox: &BoundingBox{X: 0.5, Y: 0.5, Width: 0.5, Height: 0.5},
	}, nil
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
