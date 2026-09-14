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

// dashScopeEndpoint 阿里云百炼 token-plan（套餐）的 OpenAI 兼容地址。
const dashScopeEndpoint = "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1/chat/completions"

// zhipuEndpoint 智谱开放平台（bigmodel.cn）的 OpenAI 兼容地址。
const zhipuEndpoint = "https://open.bigmodel.cn/api/paas/v4/chat/completions"

// ChatClient 通用的 OpenAI 兼容多模态对话客户端。
//
// 智谱 BigModel、阿里云 DashScope(token-plan) 等均提供 /chat/completions 兼容接口，
// 请求/响应结构一致，仅默认 endpoint、模型名与可选参数（max_tokens / reasoning_effort）
// 不同，因此统一封装，由 aliyun / zhipu 等 provider 复用。
type ChatClient struct {
	provider string // 服务商标识（仅用于错误信息）
	apiKey   string
	model    string
	endpoint string
	// maxTokens 单次输出的最大 token 数；0 表示不下发（使用服务端默认）。
	maxTokens int
	// reasoningEffort 思考强度（智谱 GLM 系列专用，low/high/max）；空表示不下发。
	reasoningEffort string
	client          *http.Client
}

// ChatConfig 对话客户端配置。
type ChatConfig struct {
	// Provider 服务商标识（仅用于错误信息），默认 "openai-compatible"。
	Provider string
	APIKey   string
	Model    string
	Endpoint string
	// MaxTokens 单次输出最大 token 数，0 表示不下发。
	MaxTokens int
	// ReasoningEffort 思考强度，空表示不下发。
	ReasoningEffort string
}

// NewChatClient 创建对话客户端。
func NewChatClient(cfg ChatConfig) *ChatClient {
	if cfg.Provider == "" {
		cfg.Provider = "openai-compatible"
	}
	if cfg.Model == "" {
		cfg.Model = "qwen3.8-flash"
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = dashScopeEndpoint
	}
	return &ChatClient{
		provider:        cfg.Provider,
		apiKey:          cfg.APIKey,
		model:           cfg.Model,
		endpoint:        cfg.Endpoint,
		maxTokens:       cfg.MaxTokens,
		reasoningEffort: cfg.ReasoningEffort,
		client:          &http.Client{Timeout: 300 * time.Second},
	}
}

// chatRequest OpenAI 兼容 chat.completions 请求体。
type chatRequest struct {
	Model           string        `json:"model"`
	Messages        []chatMessage `json:"messages"`
	MaxTokens       int           `json:"max_tokens,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
}

type chatMessage struct {
	Role    string     `json:"role"`
	Content []chatPart `json:"content"`
}

type chatPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *chatImageURL `json:"image_url,omitempty"`
}

type chatImageURL struct {
	URL string `json:"url"`
}

// chatResponse OpenAI 兼容 chat.completions 响应体。
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

// chat 发送一次多模态对话请求，返回模型输出的文本。
// image 为 nil 时仅发送文本（纯文本结构化拆题场景）。
func (d *ChatClient) chat(ctx context.Context, prompt string, image []byte) (string, error) {
	return d.chatSystem(ctx, "", prompt, image)
}

// chatSystem 与 chat 相同，但支持额外的 system 提示词（几何描述提取等场景）。
// image 为 nil 时仅发送文本。
func (d *ChatClient) chatSystem(ctx context.Context, system, user string, image []byte) (string, error) {
	parts := []chatPart{{Type: "text", Text: user}}
	if image != nil {
		parts = append(parts, chatPart{Type: "image_url", ImageURL: &chatImageURL{
			URL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image),
		}})
	}

	messages := make([]chatMessage, 0, 2)
	if system != "" {
		messages = append(messages, chatMessage{Role: "system", Content: []chatPart{{Type: "text", Text: system}}})
	}
	messages = append(messages, chatMessage{Role: "user", Content: parts})

	reqBody := chatRequest{
		Model:           d.model,
		Messages:        messages,
		MaxTokens:       d.maxTokens,
		ReasoningEffort: d.reasoningEffort,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("%s marshal: %w", d.provider, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("%s new request: %w", d.provider, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s request: %w", d.provider, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%s read: %w", d.provider, err)
	}

	var out chatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("%s parse: %w", d.provider, err)
	}

	if out.Error != nil {
		return "", fmt.Errorf("%s error: %s (%s)", d.provider, out.Error.Message, out.Error.Code)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("%s empty response", d.provider)
	}
	return out.Choices[0].Message.Content, nil
}

// chatTextOnly 纯文本调用（无图片）。
func (d *ChatClient) chatTextOnly(ctx context.Context, prompt string) (string, error) {
	return d.chat(ctx, prompt, nil)
}
