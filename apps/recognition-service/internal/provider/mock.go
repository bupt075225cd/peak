package provider

import (
	"context"
	"fmt"
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
	}, nil
}
