package service

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"go.uber.org/zap"

	"peak/apps/recognition-service/internal/provider"
)

// storeDocumentImages 将文档内嵌图片存入 storage，返回按序号（从 1 开始）排列的 key 列表。
func (s *Service) storeDocumentImages(ctx context.Context, taskID uint64, images [][]byte) []string {
	keys := make([]string, 0, len(images))
	for i, img := range images {
		key := fmt.Sprintf("document/task_%d/img_%d.jpg", taskID, i+1)
		if err := s.storage.Put(ctx, key, img); err != nil {
			s.log.Error("store document image failed", zap.String("key", key), zap.String("error", err.Error()))
			keys = append(keys, "") // 占位保持序号对齐。
			continue
		}
		keys = append(keys, key)
	}
	return keys
}

// fromStructured 将 provider 的结构化识别结果转换为 service 层 QuestionItem 列表。
// imgKeys 为文档图片的存储 key 列表（序号从 1 开始），用于把 geometry_refs 映射为 geometry_keys。
func fromStructured(res *provider.StructuredResult, imgKeys []string) []QuestionItem {
	questions := make([]QuestionItem, 0, len(res.Items))
	for _, it := range res.Items {
		subs := make([]QuestionSubQuestion, 0, len(it.SubQuestions))
		for _, sq := range it.SubQuestions {
			// 把 geometry_refs 映射为 geometry_keys。
			keys := make([]string, 0, len(sq.GeometryRefs))
			for _, ref := range sq.GeometryRefs {
				if ref >= 1 && ref <= len(imgKeys) && imgKeys[ref-1] != "" {
					keys = append(keys, imgKeys[ref-1])
				}
			}
			subs = append(subs, QuestionSubQuestion{
				Label:        sq.Label,
				Text:         sq.Text,
				GeometryRefs: sq.GeometryRefs,
				GeometryDesc: sq.GeometryDesc,
				GeometryKeys: keys,
			})
		}

		q := QuestionItem{
			StemText:     it.StemText,
			Answer:       it.Answer,
			Geometry:     it.Geometry,
			SubQuestions: subs,
		}
		// 若模型未给出子问，但题干里含子问文本，保持原样（题干已含全部文字）。
		if strings.TrimSpace(q.StemText) == "" && len(it.SubQuestions) > 0 {
			// 兜底：用子问拼出题干。
			var sb strings.Builder
			for _, sq := range it.SubQuestions {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(sq.Label)
				sb.WriteString(" ")
				sb.WriteString(sq.Text)
			}
			q.StemText = sb.String()
		}
		questions = append(questions, q)
	}
	return questions
}

// filenameFromKey 从存储 key（original/<nano>_<filename>）提取原始文件名。
func filenameFromKey(key string) string {
	base := filepath.Base(key)
	// 去掉 "前缀_时间戳_" 部分，保留真实文件名。
	if idx := strings.Index(base, "_"); idx >= 0 {
		return base[idx+1:]
	}
	return base
}

// questionNoPattern 匹配题号前缀，如 "1."、"1、"、"1)"、"第1题"、"（1）"。
var questionNoPattern = regexp.MustCompile(`^[\s]*(\d{1,3})[.、)）]|^第[\s]*(\d{1,3})[\s]*题|^[（(][\s]*(\d{1,3})[\s]*[)）]`)

// splitQuestions 将文档内容项拆分为多道题。
// 启发式：遇到题号前缀开启新题；图片项归入当前题（或独立成题）。
func splitQuestions(ctx context.Context, prov provider.Provider, items []provider.DocumentItem) []QuestionItem {
	var questions []QuestionItem
	var cur *QuestionItem

	flush := func() {
		if cur != nil && strings.TrimSpace(cur.StemText) != "" {
			questions = append(questions, *cur)
		}
		cur = nil
	}

	for _, it := range items {
		switch it.Kind {
		case "text":
			if questionNoPattern.MatchString(it.Text) {
				flush()
				cur = &QuestionItem{StemText: strings.TrimSpace(it.Text)}
			} else if cur != nil {
				cur.StemText += "\n" + strings.TrimSpace(it.Text)
			} else {
				cur = &QuestionItem{StemText: strings.TrimSpace(it.Text)}
			}
		case "image":
			// 文档内嵌图片：走 OCR 提取文字，归入当前题或独立成题。
			text := ""
			if tr, err := prov.RecognizeText(ctx, it.Image); err == nil {
				text = tr.Text
			}
			if cur == nil {
				cur = &QuestionItem{}
			}
			if strings.TrimSpace(text) != "" {
				cur.StemText += "\n" + strings.TrimSpace(text)
			}
		}
	}
	flush()

	// 若未拆分出任何题（如纯文本无题号），整体作为一道题。
	if len(questions) == 0 {
		var sb strings.Builder
		for _, it := range items {
			if it.Kind == "text" {
				sb.WriteString(strings.TrimSpace(it.Text))
				sb.WriteString("\n")
			}
		}
		if s := strings.TrimSpace(sb.String()); s != "" {
			questions = append(questions, QuestionItem{StemText: s})
		}
	}
	return questions
}
