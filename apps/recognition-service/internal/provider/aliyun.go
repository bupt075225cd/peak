package provider

import (
	"context"
	"time"
)

// AliyunProvider 阿里云识别能力组合实现（方案 B：统一使用通义千问-VL）。
// 文字/公式/几何/文档等 VLM 能力复用 vlmCapabilities（OpenAI 兼容多模态对话），
// 手写擦除走通义万相图像编辑（wanx2.1-imageedit）异步任务。
type AliyunProvider struct {
	vlmCapabilities
	wanx *WanxClient
}

// AliyunConfig 阿里云 provider 配置。
type AliyunConfig struct {
	// 保留字段以兼容历史配置（方案 B 不再使用传统 OCR）。
	AccessKeyID  string
	AccessSecret string
	OCREndpoint  string
	// 千问-VL 配置。
	DashKey      string // DashScope API Key
	DashModel    string // 视觉多模态模型名（token-plan 套餐默认 qwen3.8-max）
	DashEndpoint string // DashScope API endpoint（可注入，便于测试）
	// 万相图像编辑配置（手写擦除）。
	WanxModel    string // 万相图像编辑模型名，默认 wanx2.1-imageedit
	WanxEndpoint string // 万相接口基地址（可注入，便于测试）
}

// NewAliyunProvider 创建阿里云 provider。
func NewAliyunProvider(cfg AliyunConfig) *AliyunProvider {
	return &AliyunProvider{
		vlmCapabilities: vlmCapabilities{dash: NewChatClient(ChatConfig{
			Provider: "dashscope",
			APIKey:   cfg.DashKey,
			Model:    cfg.DashModel,
			Endpoint: cfg.DashEndpoint,
		})},
		wanx: NewWanxClient(cfg.DashKey, cfg.WanxModel, cfg.WanxEndpoint),
	}
}

func (a *AliyunProvider) Name() string { return "aliyun" }

// EraseHandwriting 调用通义万相图像编辑擦除手写笔迹，返回擦除后的图片字节。
// 万相为异步任务（提交→轮询→下载），此处设置整体 deadline 约 150s，避免无限等待。
func (a *AliyunProvider) EraseHandwriting(ctx context.Context, image []byte) (*ErasureResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()

	data, err := a.wanx.EraseHandwriting(ctx, image)
	if err != nil {
		return nil, err
	}
	return &ErasureResult{ImageData: data}, nil
}
