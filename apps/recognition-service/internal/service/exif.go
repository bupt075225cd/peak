package service

// 图片 EXIF 方向校正。
//
// 手机/微信导出的题图片经常把拍摄方向写在 EXIF 的 Orientation 里：原始像素是侧躺的，
// 查看器按标记转一下再显示。Go 的 image/jpeg 解码不会应用该标记，于是会出现"人眼看到的是
// 正的、程序拿到的是横的"。几何识别的 bounding_box 是相对"视觉正立图"给的归一化坐标，
// 若直接拿它去裁原始纵向像素，就会框到别处（实测图形被切掉一半，重绘随之全错）。
//
// 因此在任务入口把像素按 EXIF 方向一次性转正，后续识别 / 裁剪 / 旋转共用同一坐标系。

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
)

// exifOrientation 读取 JPEG 的 EXIF Orientation（1~8）；无标记或非 JPEG 返回 1（正常）。
func exifOrientation(src []byte) int {
	if len(src) < 4 || src[0] != 0xFF || src[1] != 0xD8 {
		return 1
	}
	for i := 2; i+4 <= len(src); {
		if src[i] != 0xFF {
			i++
			continue
		}
		marker := src[i+1]
		i += 2
		// 无长度字段的标记（填充、RST、SOI/EOI）。
		if marker == 0xD8 || marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0xFF {
			continue
		}
		if i+2 > len(src) {
			return 1
		}
		segLen := int(binary.BigEndian.Uint16(src[i : i+2]))
		if segLen < 2 || i+segLen > len(src) {
			return 1
		}
		if marker == 0xE1 {
			seg := src[i+2 : i+segLen]
			if o := parseExifOrientation(seg); o != 0 {
				return o
			}
		}
		if marker == 0xDA { // 图像数据开始，后面不会再有 EXIF
			return 1
		}
		i += segLen
	}
	return 1
}

// parseExifOrientation 从 APP1 段（Exif\0\0 + TIFF）里取 Orientation 标签。
func parseExifOrientation(seg []byte) int {
	if len(seg) < 14 || string(seg[:6]) != "Exif\x00\x00" {
		return 0
	}
	tiff := seg[6:]
	var bo binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	ifd := int(bo.Uint32(tiff[4:8]))
	if ifd+2 > len(tiff) {
		return 0
	}
	n := int(bo.Uint16(tiff[ifd : ifd+2]))
	for k := 0; k < n; k++ {
		e := ifd + 2 + k*12
		if e+12 > len(tiff) {
			return 0
		}
		if bo.Uint16(tiff[e:e+2]) != 0x0112 { // Orientation
			continue
		}
		v := int(bo.Uint16(tiff[e+8 : e+10]))
		if v >= 1 && v <= 8 {
			return v
		}
		return 0
	}
	return 0
}

// normalizeImageOrientation 按 EXIF Orientation 把像素转正，返回新字节与顺时针转正角度；
// 无需校正（解码/编码失败也算）时原样返回、角度为 0——方向问题不该让整条识别链路失败。
func normalizeImageOrientation(src []byte) ([]byte, int) {
	o := exifOrientation(src)
	if o <= 1 {
		return src, 0
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return src, 0
	}
	out := applyOrientation(img, o)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 92}); err != nil {
		return src, 0
	}
	return buf.Bytes(), exifFixDegrees(o)
}

// exifFixDegrees 把 EXIF Orientation（1~8）换算成"顺时针转多少度才正立"。
// 2/4/5/7 是镜像类方向，无法用纯旋转表达，按 0 返回（像素已由 applyOrientation 校正，
// 只是没法用角度回传给前端做预览旋转）。
func exifFixDegrees(o int) int {
	switch o {
	case 3: // 180°
		return 180
	case 6: // 顺时针 90°
		return 90
	case 8: // 逆时针 90°
		return 270
	default:
		return 0
	}
}

// applyOrientation 按 EXIF 约定旋转/镜像位图（5~8 会交换宽高）。
func applyOrientation(src image.Image, o int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sx, sy int
			switch o {
			case 2: // 水平镜像
				sx, sy = w-1-x, y
			case 3: // 180°
				sx, sy = w-1-x, h-1-y
			case 4: // 垂直镜像
				sx, sy = x, h-1-y
			case 5: // 主对角线转置
				sx, sy = y, x
			case 6: // 顺时针 90°
				sx, sy = y, h-1-x
			case 7: // 副对角线转置
				sx, sy = w-1-y, h-1-x
			case 8: // 逆时针 90°
				sx, sy = w-1-y, x
			default:
				sx, sy = x, y
			}
			dst.Set(x, y, src.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	return dst
}
