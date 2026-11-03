package service

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"testing"

	"peak/libs/logger"

	"peak/apps/recognition-service/internal/provider"
)

// markedImage 生成 3×2 的图，每个像素用 R=x*10+1、G=y*10+1 标记，便于校验旋转映射。
func markedImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x*10 + 1), G: uint8(y*10 + 1), B: 0, A: 255})
		}
	}
	return img
}

func assertPixel(t *testing.T, img image.Image, x, y, wantR, wantG int) {
	t.Helper()
	r, g, _, _ := img.At(x, y).RGBA()
	if int(r>>8) != wantR || int(g>>8) != wantG {
		t.Fatalf("pixel (%d,%d) = (%d,%d), want (%d,%d)", x, y, r>>8, g>>8, wantR, wantG)
	}
}

func TestRotateImageClockwise(t *testing.T) {
	src := markedImage()

	// 顺时针 90°：src(x,y) → dst(h-1-y, x)，尺寸 2×3。
	rot90 := rotateImage(src, 90)
	if rot90.Bounds().Dx() != 2 || rot90.Bounds().Dy() != 3 {
		t.Fatalf("90° size = %v", rot90.Bounds())
	}
	assertPixel(t, rot90, 1, 0, 1, 1)   // src(0,0)
	assertPixel(t, rot90, 0, 2, 21, 11) // src(2,1)
	assertPixel(t, rot90, 1, 2, 21, 1)  // src(2,0)

	// 顺时针 180°：src(x,y) → dst(w-1-x, h-1-y)。
	rot180 := rotateImage(src, 180)
	if rot180.Bounds().Dx() != 3 || rot180.Bounds().Dy() != 2 {
		t.Fatalf("180° size = %v", rot180.Bounds())
	}
	assertPixel(t, rot180, 2, 1, 1, 1)   // src(0,0)
	assertPixel(t, rot180, 0, 0, 21, 11) // src(2,1)

	// 顺时针 270°：src(x,y) → dst(y, w-1-x)，尺寸 2×3。
	rot270 := rotateImage(src, 270)
	if rot270.Bounds().Dx() != 2 || rot270.Bounds().Dy() != 3 {
		t.Fatalf("270° size = %v", rot270.Bounds())
	}
	assertPixel(t, rot270, 0, 2, 1, 1)   // src(0,0)
	assertPixel(t, rot270, 1, 0, 21, 11) // src(2,1)
}

func TestRotateImageDataUprightsJPEG(t *testing.T) {
	src := markedImage()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, nil); err != nil {
		t.Fatal(err)
	}
	out, err := rotateImageData(buf.Bytes(), 90)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 3 {
		t.Fatalf("unexpected size: %v", img.Bounds())
	}

	// 0/非法角度原样返回。
	raw := []byte("raw")
	if got, err := rotateImageData(raw, 0); err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("deg 0 should pass through, got %q err=%v", got, err)
	}
	if got, err := rotateImageData(raw, 45); err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("invalid deg should pass through, got %q err=%v", got, err)
	}
}

// fakeOrientationProvider 在 mock 基础上实现方向探测能力，返回固定倾斜角。
type fakeOrientationProvider struct {
	provider.Provider
	tilt   provider.Rotation
	probed bool
}

func (f *fakeOrientationProvider) DetectOrientation(_ context.Context, _ []byte) (provider.Rotation, error) {
	f.probed = true
	return f.tilt, nil
}

// TestUprightImageByModel 无 EXIF 方向时按模型判出的倾斜角转正整图，
// 回传角度是"额外顺时针旋转量"（前端据此转本地预览图）。
func TestUprightImageByModel(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, markedImage(), nil); err != nil {
		t.Fatal(err)
	}
	fake := &fakeOrientationProvider{Provider: provider.NewMockProvider(), tilt: 90}
	svc := &Service{prov: fake, log: logger.NewNop()}

	out, deg := svc.uprightImage(context.Background(), 1, buf.Bytes())
	if !fake.probed {
		t.Fatal("expected orientation probe")
	}
	// 模型答"需逆时针转 90° 读正"，转正就是顺时针 270°。
	if deg != 270 {
		t.Fatalf("rotation = %d, want 270", deg)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 3 {
		t.Fatalf("rotated size = %dx%d, want 2x3", b.Dx(), b.Dy())
	}
}

// TestUprightImagePrefersExif 有 EXIF 方向标记时不再调用模型（省一次网络往返），
// 且不向前端回传角度——EXIF 那部分浏览器显示图片时已自动应用，重复旋转会转两遍。
func TestUprightImagePrefersExif(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, markedImage(), nil); err != nil {
		t.Fatal(err)
	}
	src := withExifOrientation(buf.Bytes(), 6)
	fake := &fakeOrientationProvider{Provider: provider.NewMockProvider(), tilt: 90}
	svc := &Service{prov: fake, log: logger.NewNop()}

	out, deg := svc.uprightImage(context.Background(), 1, src)
	if fake.probed {
		t.Fatal("EXIF 已给出方向，不应再调用模型")
	}
	if deg != 0 {
		t.Fatalf("rotation = %d, want 0 (EXIF 部分由浏览器处理)", deg)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// EXIF orientation=6 即顺时针 90°：3×2 → 2×3。
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 3 {
		t.Fatalf("rotated size = %dx%d, want 2x3", b.Dx(), b.Dy())
	}
}

// TestUprightImageKeepsOriginalWithoutDetector provider 未实现方向探测时原样返回。
func TestUprightImageKeepsOriginalWithoutDetector(t *testing.T) {
	svc := &Service{prov: provider.NewMockProvider(), log: logger.NewNop()}
	src := []byte("not-a-real-image")
	out, deg := svc.uprightImage(context.Background(), 1, src)
	if deg != 0 || !bytes.Equal(out, src) {
		t.Fatalf("expected passthrough, got deg=%d out=%q", deg, out)
	}
}

// fakeUprightProvider 在方向探测基础上实现"是否正立"确认，按 calls 顺序返回结果。
type fakeUprightProvider struct {
	*fakeOrientationProvider
	calls []bool
	idx   int
}

func (f *fakeUprightProvider) IsUpright(_ context.Context, _ []byte) (bool, error) {
	if f.idx >= len(f.calls) {
		return false, nil
	}
	v := f.calls[f.idx]
	f.idx++
	return v, nil
}

// TestUprightImageCorrectsReversedProbe 探测把 90/270 判反时，靠"是否正立"确认纠正（差 180°）。
func TestUprightImageCorrectsReversedProbe(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, markedImage(), nil); err != nil {
		t.Fatal(err)
	}
	fake := &fakeUprightProvider{
		fakeOrientationProvider: &fakeOrientationProvider{Provider: provider.NewMockProvider(), tilt: 270},
		calls:                   []bool{false, true},
	}
	svc := &Service{prov: fake, log: logger.NewNop()}

	_, deg := svc.uprightImage(context.Background(), 1, buf.Bytes())
	if deg != 270 {
		t.Fatalf("rotation = %d, want 270 (reversed probe corrected)", deg)
	}
	if fake.idx != 2 {
		t.Fatalf("upright checks = %d, want 2", fake.idx)
	}
}

// TestUprightImageKeepsProbeWhenUpright 确认通过时沿用探测结果，不再多问一次。
func TestUprightImageKeepsProbeWhenUpright(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, markedImage(), nil); err != nil {
		t.Fatal(err)
	}
	fake := &fakeUprightProvider{
		fakeOrientationProvider: &fakeOrientationProvider{Provider: provider.NewMockProvider(), tilt: 90},
		calls:                   []bool{true},
	}
	svc := &Service{prov: fake, log: logger.NewNop()}

	_, deg := svc.uprightImage(context.Background(), 1, buf.Bytes())
	if deg != 270 {
		t.Fatalf("rotation = %d, want 270", deg)
	}
	if fake.idx != 1 {
		t.Fatalf("upright checks = %d, want 1", fake.idx)
	}
}

// TestFlipBBox180 图片翻 180° 后 bbox 要换到新坐标系（左上角↔右下角）。
func TestFlipBBox180(t *testing.T) {
	b := &provider.BoundingBox{X: 0.1, Y: 0.2, Width: 0.3, Height: 0.4}
	got := flipBBox180(b)
	if got == nil || math.Abs(got.X-0.6) > 1e-9 || math.Abs(got.Y-0.4) > 1e-9 ||
		got.Width != 0.3 || got.Height != 0.4 {
		t.Fatalf("flipBBox180 = %+v", got)
	}
	if flipBBox180(nil) != nil {
		t.Fatal("nil bbox should stay nil")
	}
	// 越界坐标夹到 [0,1]
	out := flipBBox180(&provider.BoundingBox{X: -0.2, Y: 0.9, Width: 0.6, Height: 0.4})
	if out.X != 0.6 || out.Y != 0 {
		t.Fatalf("out-of-range bbox not clamped: %+v", out)
	}
}

// withExifOrientation 给已有 JPEG 插入 EXIF APP1 段（像素不变），构造带方向的图片。
func withExifOrientation(jpg []byte, orientation int16) []byte {
	seg := makeExifJPEG(orientation)
	app1 := seg[2 : len(seg)-2] // 去掉 SOI/EOI，只留 APP1
	out := append([]byte{0xFF, 0xD8}, app1...)
	return append(out, jpg[2:]...)
}
