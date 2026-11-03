package service

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

// makeExifJPEG 构造只含 SOI + APP1(Exif) + EOI 的最小 JPEG 头，用于解析方向标记。
func makeExifJPEG(orientation int16) []byte {
	tiff := make([]byte, 26)
	copy(tiff, "MM")
	binary.BigEndian.PutUint16(tiff[2:], 0x002A)
	binary.BigEndian.PutUint32(tiff[4:], 8) // IFD0 偏移
	binary.BigEndian.PutUint16(tiff[8:], 1) // 1 个条目
	binary.BigEndian.PutUint16(tiff[10:], 0x0112)
	binary.BigEndian.PutUint16(tiff[12:], 3) // SHORT
	binary.BigEndian.PutUint32(tiff[14:], 1)
	binary.BigEndian.PutUint16(tiff[18:], uint16(orientation))
	binary.BigEndian.PutUint32(tiff[22:], 0) // 无下一个 IFD

	seg := append([]byte("Exif\x00\x00"), tiff...)
	out := []byte{0xFF, 0xD8, 0xFF, 0xE1}
	lenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBytes, uint16(len(seg)+2))
	out = append(out, lenBytes...)
	out = append(out, seg...)
	return append(out, 0xFF, 0xD9)
}

func TestExifOrientation(t *testing.T) {
	for _, want := range []int{1, 3, 6, 8} {
		if got := exifOrientation(makeExifJPEG(int16(want))); got != want {
			t.Fatalf("exifOrientation = %d, want %d", got, want)
		}
	}
	// 非 JPEG / 无 EXIF 均按正常处理。
	if got := exifOrientation([]byte("not a jpeg")); got != 1 {
		t.Fatalf("non-jpeg orientation = %d, want 1", got)
	}
	if got := exifOrientation([]byte{0xFF, 0xD8, 0xFF, 0xD9}); got != 1 {
		t.Fatalf("bare jpeg orientation = %d, want 1", got)
	}
}

// TestExifFixDegrees EXIF Orientation → 顺时针转正角；镜像类（2/4/5/7）无法用旋转表达，按 0。
func TestExifFixDegrees(t *testing.T) {
	cases := map[int]int{
		1: 0, 2: 0, 3: 180, 4: 0, 5: 0, 6: 90, 7: 0, 8: 270,
		0: 0, 9: 0,
	}
	for in, want := range cases {
		if got := exifFixDegrees(in); got != want {
			t.Fatalf("exifFixDegrees(%d) = %d, want %d", in, got, want)
		}
	}
}

// TestApplyOrientation 验证 8（逆时针 90°）与 6（顺时针 90°）的宽高交换与像素落位。
func TestApplyOrientation(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	src.Set(0, 0, red)  // 左
	src.Set(1, 0, blue) // 右

	// orientation=6：顺时针 90° → 1 宽 2 高，左上的红点转到右上。
	cw := applyOrientation(src, 6)
	if b := cw.Bounds(); b.Dx() != 1 || b.Dy() != 2 {
		t.Fatalf("orientation 6 size = %dx%d, want 1x2", b.Dx(), b.Dy())
	}
	if got := cw.RGBAAt(0, 0); got != red {
		t.Fatalf("orientation 6 top pixel = %v, want red", got)
	}
	if got := cw.RGBAAt(0, 1); got != blue {
		t.Fatalf("orientation 6 bottom pixel = %v, want blue", got)
	}

	// orientation=8：逆时针 90° → 右侧的蓝点转到顶部。
	ccw := applyOrientation(src, 8)
	if b := ccw.Bounds(); b.Dx() != 1 || b.Dy() != 2 {
		t.Fatalf("orientation 8 size = %dx%d, want 1x2", b.Dx(), b.Dy())
	}
	if got := ccw.RGBAAt(0, 0); got != blue {
		t.Fatalf("orientation 8 top pixel = %v, want blue", got)
	}
	if got := ccw.RGBAAt(0, 1); got != red {
		t.Fatalf("orientation 8 bottom pixel = %v, want red", got)
	}
}
