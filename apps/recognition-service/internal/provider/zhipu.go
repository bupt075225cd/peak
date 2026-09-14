package provider

import "context"

// ZhipuProvider 智谱开放平台（bigmodel.cn）识别能力组合实现。
// 文字/公式/几何/文档等 VLM 能力复用 vlmCapabilities（OpenAI 兼容多模态对话，GLM 系列）；
// 智谱当前不提供手写擦除类图像编辑接口，该能力原样返回输入图片（与 mock 行为一致）。
type ZhipuProvider struct {
	vlmCapabilities
}

// ZhipuConfig 智谱 provider 配置。
type ZhipuConfig struct {
	APIKey   string
	Model    string // 视觉多模态模型名，默认 glm-5.3-flash
	Endpoint string // OpenAI 兼容地址，默认 https://open.bigmodel.cn/api/paas/v4/chat/completions
	// MaxTokens 单次输出最大 token 数，默认 8192（GLM 默认值偏小会截断长题干转录）。
	MaxTokens int
	// ReasoningEffort 思考强度：low/high/max（glm-5.3-flash 始终思考且无法关闭，
	// 默认强度下单次识别耗时远超超时，因此默认 low）；设为 off 表示不下发该参数
	// （用于不支持该参数的模型，如 glm-4v 系列）。
	ReasoningEffort string
}

// NewZhipuProvider 创建智谱 provider。
func NewZhipuProvider(cfg ZhipuConfig) *ZhipuProvider {
	if cfg.Model == "" {
		cfg.Model = "glm-5.3-flash"
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = zhipuEndpoint
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 8192
	}
	if cfg.ReasoningEffort == "" {
		cfg.ReasoningEffort = "low"
	} else if cfg.ReasoningEffort == "off" || cfg.ReasoningEffort == "none" {
		cfg.ReasoningEffort = ""
	}
	return &ZhipuProvider{vlmCapabilities: vlmCapabilities{dash: NewChatClient(ChatConfig{
		Provider:        "zhipu",
		APIKey:          cfg.APIKey,
		Model:           cfg.Model,
		Endpoint:        cfg.Endpoint,
		MaxTokens:       cfg.MaxTokens,
		ReasoningEffort: cfg.ReasoningEffort,
	})}}
}

func (z *ZhipuProvider) Name() string { return "zhipu" }

// EraseHandwriting 智谱不提供图像编辑（手写擦除）接口，原样返回输入图片。
func (z *ZhipuProvider) EraseHandwriting(_ context.Context, image []byte) (*ErasureResult, error) {
	return &ErasureResult{ImageData: image}, nil
}
