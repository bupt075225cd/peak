package provider

import (
	"fmt"

	"peak/libs/config"
)

// NewFromConfig 根据配置创建 provider 实例，实现厂商可配置、可切换。
func NewFromConfig(cfg *config.Loader) (Provider, error) {
	name := cfg.String("recognition.provider", "mock")
	switch name {
	case "mock":
		return NewMockProvider(), nil
	case "aliyun":
		return NewAliyunProvider(AliyunConfig{
			DashKey:      cfg.String("recognition.aliyun.dash_key", ""),
			DashModel:    cfg.String("recognition.aliyun.dash_model", ""),
			DashEndpoint: cfg.String("recognition.aliyun.dash_endpoint", ""),
		}), nil
	default:
		return nil, fmt.Errorf("unsupported recognition provider: %s", name)
	}
}
