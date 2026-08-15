package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AliyunProvider 阿里云识别能力组合实现。
// 文本/公式使用阿里云 OCR（通用文字识别、公式识别），
// 手写擦除与几何图形识别使用通义千问-VL（DashScope 视觉大模型）。
type AliyunProvider struct {
	httpClient   *http.Client
	ocrEndpoint  string // OCR OpenAPI endpoint
	accessKeyID  string
	accessSecret string
	dashKey      string // DashScope API Key
	dashModel    string // 千问-VL 模型名，默认 qwen-vl-max
	dashEndpoint string // DashScope API endpoint，默认官方地址
}

// AliyunConfig 阿里云 provider 配置。
type AliyunConfig struct {
	AccessKeyID  string
	AccessSecret string
	OCRAppCode   string // OCR 应用 code（阿里云 OCR 市场版）
	OCREndpoint  string
	DashKey      string // DashScope API Key
	DashModel    string
	DashEndpoint string // DashScope API endpoint（可注入，便于测试）
}

// NewAliyunProvider 创建阿里云 provider。
func NewAliyunProvider(cfg AliyunConfig) *AliyunProvider {
	if cfg.DashModel == "" {
		cfg.DashModel = "qwen-vl-max"
	}
	if cfg.OCREndpoint == "" {
		cfg.OCREndpoint = "https://ocrapi-advanced.taobao.com/ocrservice/advanced"
	}
	if cfg.DashEndpoint == "" {
		cfg.DashEndpoint = "https://dashscope.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation"
	}
	return &AliyunProvider{
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		ocrEndpoint:  cfg.OCREndpoint,
		accessKeyID:  cfg.AccessKeyID,
		accessSecret: cfg.AccessSecret,
		dashKey:      cfg.DashKey,
		dashModel:    cfg.DashModel,
		dashEndpoint: cfg.DashEndpoint,
	}
}

func (a *AliyunProvider) Name() string { return "aliyun" }

// RecognizeText 调用阿里云通用文字识别 OCR。
func (a *AliyunProvider) RecognizeText(ctx context.Context, image []byte) (*TextResult, error) {
	resp, err := a.callOCR(ctx, "general", image)
	if err != nil {
		return nil, err
	}
	text := extractText(resp)
	return &TextResult{Text: text, Confidence: 0.9}, nil
}

// RecognizeFormula 调用阿里云公式识别 OCR，返回 LaTeX。
func (a *AliyunProvider) RecognizeFormula(ctx context.Context, image []byte) (*FormulaResult, error) {
	resp, err := a.callOCR(ctx, "formula", image)
	if err != nil {
		return nil, err
	}
	latex := extractLaTeX(resp)
	return &FormulaResult{LaTeX: latex, RawText: extractText(resp)}, nil
}

// EraseHandwriting 调用通义千问-VL 完成手写擦除（视觉理解 + 生成）。
func (a *AliyunProvider) EraseHandwriting(ctx context.Context, image []byte) (*ErasureResult, error) {
	prompt := "识别并移除图片中的手写笔迹，保留印刷体内容。"
	out, err := a.callDashVL(ctx, prompt, image)
	if err != nil {
		return nil, err
	}
	// 千问-VL 返回擦除后的图片（base64 或描述），此处解析出图片数据。
	imgData, err := decodeImage(out)
	if err != nil {
		// 若无法直接得到图片，回退返回原图，由上层降级处理。
		return &ErasureResult{ImageData: image}, nil
	}
	return &ErasureResult{ImageData: imgData}, nil
}

// RecognizeGeometry 调用通义千问-VL 输出结构化几何描述。
func (a *AliyunProvider) RecognizeGeometry(ctx context.Context, image []byte) (*GeometryResult, error) {
	prompt := "识别图片中的几何图形，输出 JSON：{shape_type, properties, description}，描述边长、角度、位置关系。"
	out, err := a.callDashVL(ctx, prompt, image)
	if err != nil {
		return nil, err
	}
	return parseGeometry(out), nil
}

// callOCR 调用阿里云 OCR OpenAPI（骨架，需配置 appcode/签名）。
func (a *AliyunProvider) callOCR(ctx context.Context, ocrType string, image []byte) (map[string]any, error) {
	body := map[string]any{
		"type":  ocrType,
		"image": base64.StdEncoding.EncodeToString(image),
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.ocrEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// 预留：签名鉴权（阿里云 OCR 市场版使用 AppCode）。
	if a.accessKeyID != "" {
		req.Header.Set("Authorization", "APPCODE "+a.accessSecret)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aliyun ocr request: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("aliyun ocr parse: %w", err)
	}
	return out, nil
}

// callDashVL 调用 DashScope 通义千问-VL。
func (a *AliyunProvider) callDashVL(ctx context.Context, prompt string, image []byte) (string, error) {
	reqBody := map[string]any{
		"model": a.dashModel,
		"input": map[string]any{
			"messages": []map[string]any{
				{
					"role": "user",
					"content": []map[string]any{
						{"image": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image)},
						{"text": prompt},
					},
				},
			},
		},
	}
	payload, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.dashEndpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.dashKey)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("dashscope request: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	// 提取输出文本（简化解析，实际结构按 DashScope 返回解析）。
	return string(data), nil
}

// extractText / extractLaTeX / parseGeometry / decodeImage 为结果解析占位实现。
func extractText(resp map[string]any) string {
	if v, ok := resp["text"].(string); ok {
		return v
	}
	return ""
}

func extractLaTeX(resp map[string]any) string {
	if v, ok := resp["latex"].(string); ok {
		return v
	}
	return ""
}

func parseGeometry(out string) *GeometryResult {
	return &GeometryResult{
		ShapeType:  "unknown",
		Properties: map[string]string{},
		Description: out,
	}
}

func decodeImage(out string) ([]byte, error) {
	return nil, fmt.Errorf("image not embedded in output")
}
