package service

// 几何区域转正：手机照片/截图经常侧躺（题干文字竖排），
// 把裁剪出来的几何小图按识别出的 rotation 顺时针转到文字正立，
// 降低提取模型对点、线相对位置的空间误判。

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"time"

	"go.uber.org/zap"

	"peak/apps/recognition-service/internal/provider"
)

// orientationProbeMaxDim 方向探测的缩图上限：只判断文字朝向，不需要高分辨率，
// 缩小后上传更快、vision token 更省。
const orientationProbeMaxDim = 512

// 方向探测/正立确认的独立短超时：二者都是"转正增强"步骤（失败可降级用原图），
// 模型变慢时不应拖住整条识别链路（实测单次方向探测曾达 77s）。
// 正立确认超时不能压得太紧：它答"是/否"只需很少输出，但排队+首字延迟可达 5~6s，
// 实测 6s 会周期性超时；超时后只能沿用探测结果，而探测对 90/270 本就不稳，
// 一旦取反就是整图倒立（题干、几何图全部反向），代价远大于多等几秒。
const (
	orientationDetectTimeout  = 15 * time.Second
	orientationUprightTimeout = 12 * time.Second
)

// uprightImage 在识别前把整图转到文字正立，返回转正后的字节与"额外顺时针旋转角"。
//
// 两级判定：
//  1. EXIF Orientation（零网络成本、最可靠）：手机原图、相机导出的图片通常带标记；
//  2. EXIF 没有方向标记时（微信转发图、截图常见）让模型判一次倾斜角。
//
// 返回的角度只包含第 2 级的旋转量：EXIF 那部分浏览器显示图片时已自动应用，
// 前端若拿同一个角度再转一次会转两遍。
func (s *Service) uprightImage(ctx context.Context, taskID uint64, imageData []byte) ([]byte, int) {
	if exifOrientation(imageData) > 1 {
		oriented, deg := normalizeImageOrientation(imageData)
		s.log.Info("image orientation normalized by exif",
			zap.Uint64("task_id", taskID), zap.Int("rotation", deg),
			zap.Int("src_bytes", len(imageData)), zap.Int("out_bytes", len(oriented)))
		return oriented, 0
	}
	det, ok := s.prov.(provider.OrientationDetector)
	if !ok {
		return imageData, 0
	}
	probe := orientationProbe(imageData)
	if probe == nil {
		return imageData, 0
	}
	start := time.Now()
	var tilt provider.Rotation
	var derr error
	s.callProvider(ctx, "detect_orientation", func(c context.Context) error {
		dctx, cancel := context.WithTimeout(c, orientationDetectTimeout)
		defer cancel()
		tilt, derr = det.DetectOrientation(dctx, probe)
		return derr
	})
	if derr != nil {
		s.log.Warn("orientation detect failed, keep original image",
			zap.Uint64("task_id", taskID), zap.String("error", derr.Error()))
		return imageData, 0
	}
	deg := tilt.FixDegrees()
	if deg == 0 {
		s.log.Info("image already upright", zap.Uint64("task_id", taskID),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()))
		return imageData, 0
	}
	// 二次确认：90/270 最容易判混（实测同一张图三次里两次答错），转错方向会让整图倒立，
	// 几何图形随之上下左右全反。把探测图转到候选方向后直接问"正不正"，比报角度可靠。
	deg = s.confirmUpright(ctx, taskID, probe, deg)
	upright, rerr := rotateImageData(imageData, deg)
	if rerr != nil {
		s.log.Warn("image rotate upright failed, keep original image",
			zap.Uint64("task_id", taskID), zap.Int("rotation", deg), zap.String("error", rerr.Error()))
		return imageData, 0
	}
	s.log.Info("image rotated upright by model",
		zap.Uint64("task_id", taskID), zap.Int("tilt", tilt.Degrees()), zap.Int("rotation", deg),
		zap.Int("src_bytes", len(imageData)), zap.Int("out_bytes", len(upright)),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()))
	return upright, deg
}

// confirmUpright 用"是否正立"的二次询问校正方向探测结果，主要防 90/270 判混：
// 转错方向时结果正好差 180°，所以第一个候选不正立就试反方向。
// provider 未实现该能力或检查失败时，沿用探测结论（不阻塞识别）。
func (s *Service) confirmUpright(ctx context.Context, taskID uint64, probe []byte, deg int) int {
	chk, ok := s.prov.(provider.UprightChecker)
	if !ok || deg == 0 {
		return deg
	}
	start := time.Now()
	isUpright := func(candidate []byte) (bool, error) {
		var up bool
		var err error
		s.callProvider(ctx, "upright_check", func(c context.Context) error {
			uctx, cancel := context.WithTimeout(c, orientationUprightTimeout)
			defer cancel()
			up, err = chk.IsUpright(uctx, candidate)
			return err
		})
		return up, err
	}

	first, err := rotateImageData(probe, deg)
	if err != nil {
		return deg
	}
	if up, cerr := isUpright(first); cerr != nil {
		s.log.Warn("upright check failed, keep probe result",
			zap.Uint64("task_id", taskID), zap.String("error", cerr.Error()))
		return deg
	} else if up {
		return deg
	}

	alt := (deg + 180) % 360
	second, err := rotateImageData(probe, alt)
	if err != nil {
		return deg
	}
	if up, cerr := isUpright(second); cerr == nil && up {
		s.log.Warn("orientation probe corrected by upright check",
			zap.Uint64("task_id", taskID), zap.Int("probe_rotation", deg), zap.Int("rotation", alt),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()))
		return alt
	}
	s.log.Warn("upright check failed on both directions, keep probe result",
		zap.Uint64("task_id", taskID), zap.Int("rotation", deg))
	return deg
}

// orientationProbe 生成方向探测用小图（最长边 orientationProbeMaxDim）；解码失败返回 nil。
func orientationProbe(src []byte) []byte {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil
	}
	out, err := encodeRegion(img, img.Bounds(), orientationProbeMaxDim)
	if err != nil {
		return nil
	}
	return out
}

// rotateImageData 按顺时针 deg（0/90/180/270）旋转图片并重新编码为 JPEG。
// deg 非法或为 0 时原样返回。
func rotateImageData(src []byte, deg int) ([]byte, error) {
	deg = ((deg % 360) + 360) % 360
	if deg != 90 && deg != 180 && deg != 270 {
		return src, nil
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	out := rotateImage(img, deg)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// rotateImage 顺时针旋转 90/180/270 度；其他角度原样返回。
func rotateImage(src image.Image, deg int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	switch deg {
	case 90:
		dst := image.NewRGBA(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(h-1-y, x, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 180:
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(w-1-x, h-1-y, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 270:
		dst := image.NewRGBA(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(y, w-1-x, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	default:
		return src
	}
}
