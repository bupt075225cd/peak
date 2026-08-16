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

// DashScopeClient 封装通义千问-VL 的 OpenAI 兼容接口调用。
type DashScopeClient struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// DashScopeConfig 千问-VL 客户端配置。
type DashScopeConfig struct {
	APIKey   string
	Model    string
	Endpoint string
}

// NewDashScopeClient 创建千问-VL 客户端。
func NewDashScopeClient(cfg DashScopeConfig) *DashScopeClient {
	if cfg.Model == "" {
		cfg.Model = "qwen-vl-max"
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"
	}
	return &DashScopeClient{
		apiKey:   cfg.APIKey,
		model:    cfg.Model,
		endpoint: cfg.Endpoint,
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

// dashRequest OpenAI 兼容 chat.completions 请求体。
type dashRequest struct {
	Model    string         `json:"model"`
	Messages []dashMessage  `json:"messages"`
}

type dashMessage struct {
	Role    string       `json:"role"`
	Content []dashPart   `json:"content"`
}

type dashPart struct {
	Type     string           `json:"type"`
	Text     string           `json:"text,omitempty"`
	ImageURL *dashImageURL    `json:"image_url,omitempty"`
}

type dashImageURL struct {
	URL string `json:"url"`
}

// dashResponse OpenAI 兼容 chat.completions 响应体。
type dashResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

// chat 发送一次多模态对话请求，返回模型输出的文本。
// image 为 nil 时仅发送文本（纯文本结构化拆题场景）。
func (d *DashScopeClient) chat(ctx context.Context, prompt string, image []byte) (string, error) {
	parts := []dashPart{{Type: "text", Text: prompt}}
	if image != nil {
		parts = append(parts, dashPart{Type: "image_url", ImageURL: &dashImageURL{
			URL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image),
		}})
	}

	reqBody := dashRequest{
		Model: d.model,
		Messages: []dashMessage{
			{
				Role:    "user",
				Content: parts,
			},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("dashscope marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("dashscope new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("dashscope request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("dashscope read: %w", err)
	}

	var out dashResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("dashscope parse: %w", err)
	}

	if out.Error != nil {
		return "", fmt.Errorf("dashscope error: %s (%s)", out.Error.Message, out.Error.Code)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("dashscope empty response")
	}
	return out.Choices[0].Message.Content, nil
}
