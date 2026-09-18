package export

import (
	_ "embed"
	"fmt"
	"os"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// embeddedFont 内置中文字体（Noto Sans SC 子集，覆盖 GB2312 汉字与常用符号）。
//
// 运行容器基于 alpine，镜像内无任何中文字体；把字体随二进制打包可保证
// 本地调测与容器部署行为一致，也避免构建期下载字体。
//
//go:embed assets/NotoSansSC-Regular.otf
var embeddedFont []byte

// FontProvider 提供按字号缓存的字体 face。
//
// sfnt.Font 解析一次即可复用；opentype.Face 自身并发安全，可跨 goroutine 共享。
type FontProvider struct {
	mu    sync.Mutex
	sfnt  *sfnt.Font
	cache map[float64]font.Face
}

// NewFontProvider 创建字体提供者。
//
// path 为空时使用内置字体；指定 path 时改用外部字体文件，便于替换字体
// 或改用更小的子集字体。
func NewFontProvider(path string) (*FontProvider, error) {
	data := embeddedFont
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read export font %q: %w", path, err)
		}
		data = b
	}
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse export font: %w", err)
	}
	return &FontProvider{sfnt: f, cache: make(map[float64]font.Face)}, nil
}

// Face 返回指定字号的字体 face，同一字号复用同一实例。
func (p *FontProvider) Face(size float64) (font.Face, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if f, ok := p.cache[size]; ok {
		return f, nil
	}
	f, err := opentype.NewFace(p.sfnt, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, fmt.Errorf("create font face at size %.1f: %w", size, err)
	}
	p.cache[size] = f
	return f, nil
}
