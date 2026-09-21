package export

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"path"
	"strings"
	"sync"

	_ "image/gif"
	"image/jpeg"

	"golang.org/x/image/draw"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

// imageFetchConcurrency 单次导出内的图片并发拉取上限。
const imageFetchConcurrency = 4

// jpegQuality 缩放后重新编码 JPEG 的质量，兼顾清晰度与体积。
const jpegQuality = 88

// errUnexpectedAsset 图片加载内部类型断言失败，属于不应出现的编程错误。
var errUnexpectedAsset = errors.New("unexpected image asset type")

// ImageLoader 按存储 key 拉取并规整图片，在一次导出内缓存结果。
//
// 每个导出请求应创建独立实例：缓存生命周期与单次导出一致，避免跨请求堆积内存。
type ImageLoader struct {
	fetcher  Fetcher
	maxWidth int
	fonts    *FontProvider

	mu    sync.Mutex
	cache map[string]*ImageAsset
	fail  map[string]error
	group singleflight.Group
}

// NewImageLoader 创建图片加载器。maxWidth <= 0 表示不做宽度限制。
//
// fonts 用于 SVG 光栅化时补绘 oksvg 不支持的文字标注，可为 nil。
func NewImageLoader(fetcher Fetcher, maxWidth int, fonts *FontProvider) *ImageLoader {
	return &ImageLoader{
		fetcher:  fetcher,
		maxWidth: maxWidth,
		fonts:    fonts,
		cache:    make(map[string]*ImageAsset),
		fail:     make(map[string]error),
	}
}

// Load 并发加载多张图片，返回 key 到图片的映射。
//
// 单张图片失败不会中断整体：跳过该图并在 warnings 中记录，便于文档里给出占位说明。
// 同一 key 只拉取一次，调用方按 key 取用即可保持题目内的配图顺序。
func (l *ImageLoader) Load(ctx context.Context, keys []string) (map[string]*ImageAsset, []string) {
	if len(keys) == 0 {
		return nil, nil
	}

	unique := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}

	assets := make([]*ImageAsset, len(unique))
	errs := make([]error, len(unique))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(imageFetchConcurrency)
	for i, key := range unique {
		i, key := i, key
		g.Go(func() error {
			asset, err := l.loadOne(gctx, key)
			if err != nil {
				errs[i] = err
				return nil // 单图失败不取消其它图片。
			}
			assets[i] = asset
			return nil
		})
	}
	_ = g.Wait()

	out := make(map[string]*ImageAsset, len(unique))
	var warnings []string
	for i, key := range unique {
		switch {
		case errs[i] != nil:
			warnings = append(warnings, fmt.Sprintf("配图 %s 加载失败：%v", key, errs[i]))
		case assets[i] != nil:
			out[key] = assets[i]
		}
	}
	return out, warnings
}

// loadOne 加载单张图片：命中缓存直接返回，同一 key 的并发请求合并为一次拉取。
func (l *ImageLoader) loadOne(ctx context.Context, key string) (*ImageAsset, error) {
	l.mu.Lock()
	if asset, ok := l.cache[key]; ok {
		l.mu.Unlock()
		return asset, nil
	}
	if err, ok := l.fail[key]; ok {
		l.mu.Unlock()
		return nil, err
	}
	l.mu.Unlock()

	value, err, _ := l.group.Do(key, func() (any, error) {
		raw, fetchErr := l.fetcher.Fetch(ctx, key)
		if fetchErr != nil {
			return nil, fetchErr
		}
		return normalizeImage(raw, key, l.maxWidth, l.fonts)
	})

	l.mu.Lock()
	defer l.mu.Unlock()

	if err != nil {
		l.fail[key] = err
		return nil, err
	}
	asset, ok := value.(*ImageAsset)
	if !ok || asset == nil {
		l.fail[key] = errUnexpectedAsset
		return nil, errUnexpectedAsset
	}
	l.cache[key] = asset
	return asset, nil
}

// normalizeImage 把原始图片字节规整为可嵌入文档的位图。
//
// SVG 走光栅化；位图超过宽度上限时等比缩放并统一编码为 PNG，
// 未超限的 JPEG/PNG 直接复用原始字节，避免重新编码导致体积膨胀。
func normalizeImage(raw []byte, key string, maxWidth int, fonts *FontProvider) (*ImageAsset, error) {
	if isSVG(key) {
		return rasterizeSVG(raw, maxWidth, fonts)
	}

	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("invalid image size %dx%d", w, h)
	}

	if maxWidth <= 0 || w <= maxWidth {
		switch f := strings.ToLower(format); f {
		case "jpeg", "png":
			return &ImageAsset{
				Data: raw, Format: f, Width: w, Height: h,
				NaturalWidth: w, NaturalHeight: h,
			}, nil
		}
	}

	nw, nh := scaleSize(w, h, maxWidth)
	var pic image.Image = img
	if nw != w || nh != h {
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		pic = dst
	}

	// 照片类图片缩放后仍编码为 JPEG：转 PNG 会让体积膨胀数倍。
	var buf bytes.Buffer
	if strings.EqualFold(format, "jpeg") {
		if err := jpeg.Encode(&buf, pic, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return nil, fmt.Errorf("encode jpeg: %w", err)
		}
		return &ImageAsset{
			Data: buf.Bytes(), Format: "jpeg", Width: nw, Height: nh,
			NaturalWidth: w, NaturalHeight: h,
		}, nil
	}

	if err := png.Encode(&buf, pic); err != nil {
		return nil, fmt.Errorf("encode image: %w", err)
	}
	return &ImageAsset{
		Data: buf.Bytes(), Format: "png", Width: nw, Height: nh,
		NaturalWidth: w, NaturalHeight: h,
	}, nil
}

// scaleSize 按最大宽度等比缩放尺寸；maxWidth <= 0 或宽度未超限时返回原尺寸。
func scaleSize(w, h, maxWidth int) (int, int) {
	if maxWidth <= 0 || w <= maxWidth || w <= 0 {
		return w, h
	}
	nh := int(float64(h) * float64(maxWidth) / float64(w))
	if nh < 1 {
		nh = 1
	}
	return maxWidth, nh
}

// isSVG 按扩展名判断是否为 SVG。存储 key 的命名约定与前端一致。
func isSVG(key string) bool {
	return strings.EqualFold(path.Ext(strings.TrimSpace(key)), ".svg")
}
