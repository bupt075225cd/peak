package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"peak/apps/recognition-service/internal/geom"
	"peak/apps/recognition-service/internal/provider"
)

// fakeAngleMarkProvider 在 mock 之上实现可选的 AngleMarkVerifier 能力。
type fakeAngleMarkProvider struct {
	provider.Provider
	verdicts map[string]bool
	err      error
	called   bool
	titles   []string
}

func (f *fakeAngleMarkProvider) VerifyAngleMarks(_ context.Context, _ []byte, titles []string) (map[string]bool, error) {
	f.called = true
	f.titles = append([]string(nil), titles...)
	if f.err != nil {
		return nil, f.err
	}
	return f.verdicts, nil
}

func newAngleMarkTestService(t *testing.T, p provider.Provider) *Service {
	t.Helper()
	svc, _ := setupServiceWithProvider(t, p)
	return svc
}

func angleMarkPanels(marks ...bool) []geom.Panel {
	panels := make([]geom.Panel, 0, len(marks))
	for i, has := range marks {
		p := geom.Panel{Title: "图" + itoa(uint64(i+1))}
		if has {
			p.Spec.AngleMarks = []geom.AngleMark{{Vertex: "A", A: "B", B: "C"}}
		}
		panels = append(panels, p)
	}
	return panels
}

func TestVerifyAngleMarksWithoutCapability(t *testing.T) {
	svc, _ := setupService(t) // mock provider 未实现 AngleMarkVerifier
	panels := angleMarkPanels(true)
	svc.verifyAngleMarks(context.Background(), 1, []byte("img"), panels)
	if panels[0].Spec.AngleMarks == nil {
		t.Fatal("provider 不具备核对能力时应保留提取结果")
	}
}

func TestVerifyAngleMarksNoClaimedMarks(t *testing.T) {
	fake := &fakeAngleMarkProvider{Provider: provider.NewMockProvider()}
	svc := newAngleMarkTestService(t, fake)
	panels := angleMarkPanels(false, false)
	svc.verifyAngleMarks(context.Background(), 1, []byte("img"), panels)
	if fake.called {
		t.Fatal("没有子图声称有角标记时不应发起核对")
	}
}

func TestVerifyAngleMarksDropsUnconfirmed(t *testing.T) {
	fake := &fakeAngleMarkProvider{Provider: provider.NewMockProvider(), verdicts: map[string]bool{"图1": false, "图2": true}}
	svc := newAngleMarkTestService(t, fake)
	panels := angleMarkPanels(true, true)
	svc.verifyAngleMarks(context.Background(), 1, []byte("img"), panels)
	if !fake.called || len(fake.titles) != 2 {
		t.Fatalf("应对全部子图发起核对，titles=%v", fake.titles)
	}
	if panels[0].Spec.AngleMarks != nil {
		t.Fatal("原图核对为 false 的角标记应被删除")
	}
	if panels[1].Spec.AngleMarks == nil {
		t.Fatal("原图核对为 true 的角标记应保留")
	}
}

func TestVerifyAngleMarksErrorKeepsExtracted(t *testing.T) {
	fake := &fakeAngleMarkProvider{Provider: provider.NewMockProvider(), err: context.DeadlineExceeded}
	svc := newAngleMarkTestService(t, fake)
	panels := angleMarkPanels(true)
	svc.verifyAngleMarks(context.Background(), 1, []byte("img"), panels)
	if panels[0].Spec.AngleMarks == nil {
		t.Fatal("核对失败时应保留提取结果")
	}
}

// ---------- 纯函数辅助 ----------

func TestClamp01(t *testing.T) {
	cases := map[float64]float64{-0.5: 0, 0: 0, 0.25: 0.25, 1: 1, 1.5: 1}
	for in, want := range cases {
		if got := clamp01(in); got != want {
			t.Fatalf("clamp01(%v)=%v, want %v", in, got, want)
		}
	}
}

func TestRegionIntHelpers(t *testing.T) {
	if clampInt(5, 1, 3) != 3 || clampInt(0, 1, 3) != 1 || clampInt(2, 1, 3) != 2 {
		t.Fatal("clampInt 边界不正确")
	}
	if maxInt(3, 7) != 7 || maxInt(9, 2) != 9 {
		t.Fatal("maxInt 不正确")
	}
	if areaOf(image.Rect(0, 0, 4, 5)) != 20 || areaOf(image.Rect(3, 3, 3, 3)) != 0 {
		t.Fatal("areaOf 不正确")
	}
	if !isGridFigure("题干", "在方格纸上作图") || !isGridFigure("网格") || !isGridFigure("格点") {
		t.Fatal("网格关键词应命中")
	}
	if isGridFigure("普通几何图形", "  ") || isGridFigure() {
		t.Fatal("非网格文本不应命中")
	}
}

// ---------- grayReader：各图像类型的灰度快路径 ----------

func TestGrayReaderImageTypes(t *testing.T) {
	gray := image.NewGray(image.Rect(0, 0, 2, 2))
	gray.SetGray(1, 0, color.Gray{Y: 100})
	if got := grayReader(gray)(1, 0); got != 100 {
		t.Fatalf("Gray 快路径=%d", got)
	}
	ycbcr := image.NewYCbCr(image.Rect(0, 0, 2, 2), image.YCbCrSubsampleRatio444)
	ycbcr.Y[ycbcr.YOffset(0, 1)] = 200
	if got := grayReader(ycbcr)(0, 1); got != 200 {
		t.Fatalf("YCbCr 快路径=%d", got)
	}
	nrgba := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	nrgba.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255}) // 白
	if got := grayReader(nrgba)(0, 0); got != 255 {
		t.Fatalf("NRGBA=%d", got)
	}
	rgba := image.NewRGBA(image.Rect(0, 0, 2, 2))
	rgba.SetRGBA(0, 0, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	rgba.SetRGBA(1, 0, color.RGBA{R: 0, G: 0, B: 0, A: 10}) // 透明像素按白处理
	gr := grayReader(rgba)
	if gr(0, 0) != 0 || gr(1, 0) != 255 {
		t.Fatalf("RGBA 透明处理不正确: %d %d", gr(0, 0), gr(1, 0))
	}
	// 其他类型回退 At()。
	cmyk := image.NewCMYK(image.Rect(0, 0, 2, 2))
	cmyk.SetCMYK(0, 0, color.CMYK{C: 0, M: 0, Y: 0, K: 255})
	if got := grayReader(cmyk)(0, 0); got != 0 {
		t.Fatalf("回退路径=%d", got)
	}
}

// ---------- EXIF 方向 ----------

// buildJPEGWithExif 构造带 APP1 EXIF Orientation 的最小 JPEG。
func buildJPEGWithExif(t *testing.T, orientation int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(30 * x), G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	src := buf.Bytes()
	if orientation <= 1 {
		return src
	}
	// TIFF 小端：IFD 含 1 个条目（Orientation=0x0112, type SHORT, value=orientation）。
	ifd := make([]byte, 0, 2+12+4)
	ifd = append(ifd, 0, 1) // 条目数
	ifd = append(ifd, 0x12, 0x01, 0x03, 0x00, 0, 0, 0, 1)
	var v [2]byte
	binary.LittleEndian.PutUint16(v[:], uint16(orientation))
	ifd = append(ifd, v[0], v[1])
	ifd = append(ifd, 0, 0, 0, 0) // 下一个 IFD 偏移
	tiff := append([]byte("II*\x00\x08\x00\x00\x00"), ifd...)
	app1 := append([]byte("Exif\x00\x00"), tiff...)
	out := []byte{src[0], src[1]} // SOI
	out = append(out, 0xFF, 0xE1) // APP1
	var l [2]byte
	binary.BigEndian.PutUint16(l[:], uint16(len(app1)+2))
	out = append(out, l[:]...)
	out = append(out, app1...)
	out = append(out, src[2:]...)
	return out
}

func TestExifOrientationReadsApp1(t *testing.T) {
	if got := exifOrientation([]byte{0x00, 0x01, 0x02}); got != 1 {
		t.Fatalf("非 JPEG 应返回 1，得到 %d", got)
	}
	if got := exifOrientation(buildJPEGWithExif(t, 6)); got != 6 {
		t.Fatalf("EXIF Orientation=6 应读出 6，得到 %d", got)
	}
	if got := exifOrientation(buildJPEGWithExif(t, 1)); got != 1 {
		t.Fatalf("无标记应返回 1，得到 %d", got)
	}
}

func TestParseExifOrientationMalformed(t *testing.T) {
	if parseExifOrientation([]byte("short")) != 0 {
		t.Fatal("过短段应返回 0")
	}
	if parseExifOrientation([]byte("NotExifxxxxx")) != 0 {
		t.Fatal("错误魔数应返回 0")
	}
	bad := append([]byte("Exif\x00\x00XX"), make([]byte, 20)...)
	if parseExifOrientation(bad) != 0 {
		t.Fatal("错误字节序应返回 0")
	}
}

func TestNormalizeImageOrientationRotates(t *testing.T) {
	// EXIF 6（顺时针 90°）→ 像素转正后宽高互换，角度回传 90。
	out, deg := normalizeImageOrientation(buildJPEGWithExif(t, 6))
	if deg != 90 {
		t.Fatalf("orientation 6 应换算 90°，得到 %d", deg)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("转正图解码失败: %v", err)
	}
	if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 8 {
		t.Fatalf("转正后应为 4x8，得到 %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
	// 无需校正时原样返回。
	src := buildJPEGWithExif(t, 1)
	got, deg := normalizeImageOrientation(src)
	if deg != 0 || !bytes.Equal(got, src) {
		t.Fatal("orientation=1 应原样返回")
	}
}

func TestApplyOrientationAllModes(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	src.Set(0, 0, color.RGBA{R: 255, A: 255}) // 左上角红点
	for o := 2; o <= 8; o++ {
		out := applyOrientation(src, o)
		dw, dh := 3, 2
		if o >= 5 {
			dw, dh = 2, 3
		}
		if out.Bounds().Dx() != dw || out.Bounds().Dy() != dh {
			t.Fatalf("orientation %d 尺寸错误", o)
		}
		// 逐像素比对：每个红点像斑位置应有且仅有一个。
		reds := 0
		for y := 0; y < dh; y++ {
			for x := 0; x < dw; x++ {
				r, _, _, _ := out.At(x, y).RGBA()
				if r == 0xffff {
					reds++
				}
			}
		}
		if reds != 1 {
			t.Fatalf("orientation %d 红点数量=%d，应为 1", o, reds)
		}
	}
}

func TestExifFixDegreesMirrorModes(t *testing.T) {
	cases := map[int]int{2: 0, 4: 0, 5: 0, 7: 0, 9: 0}
	for o, want := range cases {
		if got := exifFixDegrees(o); got != want {
			t.Fatalf("exifFixDegrees(%d)=%d, want %d", o, got, want)
		}
	}
}
