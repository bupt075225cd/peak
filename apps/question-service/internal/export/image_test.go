package export

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"sync"
	"testing"
)

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// fakeFetcher 记录每个 key 的调用次数，用于验证缓存与并发去重行为。
type fakeFetcher struct {
	mu    sync.Mutex
	data  map[string][]byte
	errs  map[string]error
	calls map[string]int
}

func newFakeFetcher(data map[string][]byte) *fakeFetcher {
	return &fakeFetcher{data: data, errs: map[string]error{}, calls: map[string]int{}}
}

func (f *fakeFetcher) Fetch(_ context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls[key]++
	if err, ok := f.errs[key]; ok {
		return nil, err
	}
	if d, ok := f.data[key]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("not found: %s", key)
}

func (f *fakeFetcher) callCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[key]
}

func TestNormalizeImageKeepsSmallJPEGBytes(t *testing.T) {
	raw := encodeJPEG(t, 100, 50)
	asset, err := normalizeImage(raw, "a.jpg", 1200)
	if err != nil {
		t.Fatalf("normalizeImage: %v", err)
	}
	if asset.Format != "jpeg" {
		t.Fatalf("format = %q, want jpeg", asset.Format)
	}
	if asset.Width != 100 || asset.Height != 50 {
		t.Fatalf("size = %dx%d, want 100x50", asset.Width, asset.Height)
	}
	if !bytes.Equal(asset.Data, raw) {
		t.Fatal("expected original jpeg bytes to be reused")
	}
}

func TestNormalizeImageScalesOversizedBitmap(t *testing.T) {
	raw := encodePNG(t, 2000, 1000)
	asset, err := normalizeImage(raw, "a.png", 1000)
	if err != nil {
		t.Fatalf("normalizeImage: %v", err)
	}
	if asset.Format != "png" {
		t.Fatalf("format = %q, want png", asset.Format)
	}
	if asset.Width != 1000 || asset.Height != 500 {
		t.Fatalf("size = %dx%d, want 1000x500", asset.Width, asset.Height)
	}
	if _, err := png.Decode(bytes.NewReader(asset.Data)); err != nil {
		t.Fatalf("scaled bytes are not a valid png: %v", err)
	}
}

func TestNormalizeImageRasterizesSVG(t *testing.T) {
	asset, err := normalizeImage([]byte(simpleSVG), "geometry/task_1.svg", 0)
	if err != nil {
		t.Fatalf("normalizeImage: %v", err)
	}
	if asset.Format != "png" || asset.Width != 100 || asset.Height != 50 {
		t.Fatalf("unexpected asset: %+v", asset)
	}
}

func TestNormalizeImageRejectsInvalidData(t *testing.T) {
	if _, err := normalizeImage([]byte("not an image"), "a.png", 0); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestScaleSize(t *testing.T) {
	cases := []struct {
		name         string
		w, h, max    int
		wantW, wantH int
	}{
		{"no limit", 100, 50, 0, 100, 50},
		{"within limit", 100, 50, 200, 100, 50},
		{"scaled", 200, 100, 100, 100, 50},
		{"min height one", 2000, 1, 100, 100, 1},
		{"zero width", 0, 0, 100, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotW, gotH := scaleSize(tc.w, tc.h, tc.max)
			if gotW != tc.wantW || gotH != tc.wantH {
				t.Fatalf("scaleSize(%d,%d,%d) = %dx%d, want %dx%d",
					tc.w, tc.h, tc.max, gotW, gotH, tc.wantW, tc.wantH)
			}
		})
	}
}

func TestIsSVG(t *testing.T) {
	cases := map[string]bool{
		"geometry/task_1.svg": true,
		"a.SVG":               true,
		"a.png":               false,
		"a.jpg":               false,
		"noext":               false,
	}
	for key, want := range cases {
		if got := isSVG(key); got != want {
			t.Fatalf("isSVG(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestImageLoaderLoadsAndDeduplicates(t *testing.T) {
	ff := newFakeFetcher(map[string][]byte{
		"a.png": encodePNG(t, 10, 10),
		"b.png": encodePNG(t, 20, 20),
	})

	loader := NewImageLoader(ff, 0)
	assets, warnings := loader.Load(context.Background(), []string{"a.png", "b.png", "a.png"})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(assets))
	}
	if assets["a.png"].Width != 10 || assets["b.png"].Width != 20 {
		t.Fatalf("unexpected assets: %+v", assets)
	}
	if n := ff.callCount("a.png"); n != 1 {
		t.Fatalf("a.png fetched %d times, want 1", n)
	}
	if n := ff.callCount("b.png"); n != 1 {
		t.Fatalf("b.png fetched %d times, want 1", n)
	}
}

func TestImageLoaderDegradesOnFailure(t *testing.T) {
	ff := newFakeFetcher(map[string][]byte{"ok.png": encodePNG(t, 10, 10)})
	ff.errs["bad.png"] = errors.New("boom")

	loader := NewImageLoader(ff, 0)
	assets, warnings := loader.Load(context.Background(), []string{"ok.png", "bad.png"})
	if len(assets) != 1 || assets["ok.png"] == nil || assets["ok.png"].Width != 10 {
		t.Fatalf("unexpected assets: %+v", assets)
	}
	if _, ok := assets["bad.png"]; ok {
		t.Fatal("failed image should not appear in results")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "bad.png") {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func TestImageLoaderWithoutKeys(t *testing.T) {
	loader := NewImageLoader(newFakeFetcher(nil), 0)
	assets, warnings := loader.Load(context.Background(), nil)
	if assets != nil || warnings != nil {
		t.Fatalf("expected nil results, got assets=%v warnings=%v", assets, warnings)
	}
}
