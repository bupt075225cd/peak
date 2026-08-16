package provider

import (
	"context"
	"fmt"

	"peak/apps/recognition-service/internal/docparse"
)

// ExtractDocument 阿里云文档识别：本地解析 word/pdf 提取文本与内嵌图片，
// 图片项复用 DashScope（通义千问-VL）做视觉理解，产出结构化文本。
func (a *AliyunProvider) ExtractDocument(ctx context.Context, data []byte, filename string) (*DocumentResult, error) {
	res, err := docparse.Parse(data, filename)
	if err != nil {
		return nil, err
	}

	items := make([]DocumentItem, 0, len(res.Items))
	for _, it := range res.Items {
		switch it.Kind {
		case "text":
			items = append(items, DocumentItem{Kind: "text", Text: it.Text})
		case "image":
			// 对文档内嵌图片调用千问-VL 做 OCR，提取图片中的题目文本。
			text, err := a.ocrImageViaDashVL(ctx, it.Image)
			if err != nil {
				// 图片 OCR 失败时保留图片项，由上层降级处理。
				items = append(items, DocumentItem{Kind: "image", Image: it.Image})
				continue
			}
			items = append(items, DocumentItem{Kind: "text", Text: text})
		}
	}

	return &DocumentResult{Items: items, PageCount: res.PageCount}, nil
}

// ocrImageViaDashVL 调用通义千问-VL 识别图片中的文字/题目内容。
func (a *AliyunProvider) ocrImageViaDashVL(ctx context.Context, image []byte) (string, error) {
	prompt := "识别图片中的全部文字与题目内容，按顺序输出纯文本，保留公式与几何描述。"
	out, err := a.dash.chat(ctx, prompt, image)
	if err != nil {
		return "", fmt.Errorf("dashscope ocr: %w", err)
	}
	return out, nil
}
