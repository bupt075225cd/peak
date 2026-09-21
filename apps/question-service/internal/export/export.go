package export

import (
	"context"
	"fmt"
	"time"
)

// Config 导出配置。
type Config struct {
	// RecognitionBaseURL 图片服务基地址（recognition-service）。
	RecognitionBaseURL string
	// FontPath 外部字体文件路径；为空时使用内置字体。
	FontPath string
	// MaxImageWidth 配图最大像素宽度，<=0 表示不限制。
	MaxImageWidth int
	// MaxItems 单次导出题数上限，<=0 表示不限制。
	MaxItems int
	// FetchTimeout 单张图片拉取超时。
	FetchTimeout time.Duration
	// MergeStemLineBreaks 是否把题干中的换行折叠为空格后重新排版。
	//
	// 识别阶段带入的换行位置与导出页宽无关，默认合并，避免"一行没排满就换行"。
	MergeStemLineBreaks bool
}

// DefaultConfig 返回带默认值的导出配置。
func DefaultConfig() Config {
	return Config{
		RecognitionBaseURL: "http://localhost:8082",
		MaxImageWidth:      1200,
		MaxItems:           200,
		FetchTimeout:       10 * time.Second,
		MergeStemLineBreaks: true,
	}
}

// Service 导出服务：接收导出视图模型，产出文件字节与文件名。
type Service interface {
	Export(ctx context.Context, items []ExportItem, format Format) (Result, error)
}

type service struct {
	cfg     Config
	fetcher Fetcher
	fonts   *FontProvider
}

// New 创建导出服务，依赖由调用方注入，便于测试替换。
func New(cfg Config, fetcher Fetcher, fonts *FontProvider) Service {
	return &service{cfg: cfg, fetcher: fetcher, fonts: fonts}
}

// NewDefault 按配置创建导出服务：内部创建图片拉取器并加载字体。
//
// 字体在创建时即加载，配置错误会在服务启动阶段暴露，而不是等到用户点导出。
func NewDefault(cfg Config) (Service, error) {
	fonts, err := NewFontProvider(cfg.FontPath)
	if err != nil {
		return nil, err
	}
	return New(cfg, NewHTTPFetcher(cfg.RecognitionBaseURL, cfg.FetchTimeout), fonts), nil
}

// Export 按指定格式导出错题。
func (s *service) Export(ctx context.Context, items []ExportItem, format Format) (Result, error) {
	if len(items) == 0 {
		return Result{}, fmt.Errorf("no items to export")
	}
	if s.cfg.MaxItems > 0 && len(items) > s.cfg.MaxItems {
		return Result{}, fmt.Errorf("too many items: %d exceeds limit %d", len(items), s.cfg.MaxItems)
	}

	rendered, warnings := s.loadImages(ctx, items)
	title := exportTitle(time.Now())

	var (
		data []byte
		err  error
	)
	switch format {
	case FormatPDF:
		data, err = buildPDF(title, rendered, s.fonts)
	case FormatDocx:
		data, err = buildDocx(title, rendered)
	default:
		return Result{}, fmt.Errorf("unsupported export format %q", format)
	}
	if err != nil {
		return Result{}, err
	}

	return Result{
		Data:     data,
		Filename: title + format.Extension(),
		Warnings: warnings,
	}, nil
}

// loadImages 批量加载全部配图并按题目归位。
//
// 一次性收集所有 key 再加载，跨题目的同一张图只会拉取一次，
// 并发度也不受题目数量限制。
func (s *service) loadImages(ctx context.Context, items []ExportItem) ([]renderItem, []string) {
	loader := NewImageLoader(s.fetcher, s.cfg.MaxImageWidth, s.fonts)

	allKeys := make([]string, 0, len(items))
	for _, it := range items {
		allKeys = append(allKeys, it.ImageKeys...)
	}
	byKey, warnings := loader.Load(ctx, allKeys)

	rendered := make([]renderItem, 0, len(items))
	for _, it := range items {
		it.StemText = normalizeStemText(it.StemText, s.cfg.MergeStemLineBreaks)
		ri := renderItem{item: it}
		for _, key := range it.ImageKeys {
			if asset, ok := byKey[key]; ok {
				ri.images = append(ri.images, *asset)
			}
		}
		ri.imageFailed = len(it.ImageKeys) > 0 && len(ri.images) == 0
		rendered = append(rendered, ri)
	}
	return rendered, warnings
}

// exportTitle 生成导出标题（含日期），同时用作文档标题与文件名。
func exportTitle(now time.Time) string {
	return "我的错题本 " + now.Format("2006-01-02")
}
