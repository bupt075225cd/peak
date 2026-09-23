package service

import (
	"context"
	"encoding/json"
	"strings"

	"peak/libs/domain"
	"peak/libs/errors"

	"peak/apps/question-service/internal/export"
)

// ExportMistakes 按 id 批量导出指定用户的错题。
//
// 传入 ids 的顺序决定文档中的题目顺序；不属于该用户、已删除或缺少题目关联的
// id 会被跳过，因此调用方无需保证 ids 全部有效。
func (s *Service) ExportMistakes(
	ctx context.Context,
	userID uint64,
	ids []uint64,
	format export.Format,
) (export.Result, error) {
	if s.exporter == nil {
		return export.Result{}, errors.New(errors.CodeInternal, "export is not configured")
	}
	if userID == 0 {
		return export.Result{}, errors.New(errors.CodeInvalidArgument, "user_id is required")
	}
	if len(ids) == 0 {
		return export.Result{}, errors.New(errors.CodeInvalidArgument, "ids is required")
	}

	list, err := s.repos.Mistake.ListByIDs(ctx, userID, ids)
	if err != nil {
		return export.Result{}, errors.Wrap(errors.CodeInternal, "query mistakes for export", err)
	}
	if len(list) == 0 {
		return export.Result{}, errors.New(errors.CodeNotFound, "no mistakes found for export")
	}

	byID := make(map[uint64]*domain.Mistake, len(list))
	for i := range list {
		byID[list[i].ID] = &list[i]
	}

	items := make([]export.ExportItem, 0, len(ids))
	for _, id := range ids {
		m, ok := byID[id]
		if !ok || m.Question == nil {
			continue
		}
		items = append(items, export.ExportItem{
			Grade:        m.Question.Grade,
			Subject:      m.Question.Subject,
			QuestionType: m.Question.QuestionType,
			Source:       mistakeSource(m),
			StemText:     m.Question.StemText,
			Images:       parseImages(m.Question.Image),
		})
	}
	if len(items) == 0 {
		return export.Result{}, errors.New(errors.CodeNotFound, "no exportable mistakes found")
	}

	result, err := s.exporter.Export(ctx, items, format)
	if err != nil {
		return export.Result{}, errors.Wrap(errors.CodeInternal, "export mistakes", err)
	}
	return result, nil
}

// mistakeSource 取题目来源：优先题目自身来源，其次错题记录来源。
func mistakeSource(m *domain.Mistake) string {
	if m.Question != nil {
		if s := strings.TrimSpace(m.Question.Source); s != "" {
			return s
		}
	}
	return strings.TrimSpace(m.Source)
}

// parseImages 解析 questions.image（JSON：配图引用数组，元素含 key 与图号 label）。
//
// 与前端 imageRefs() 行为保持一致：解析失败、缺少 key 或空值都按无图处理。
func parseImages(raw string) []export.ImageRef {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var refs []export.ImageRef
	if err := json.Unmarshal([]byte(raw), &refs); err != nil {
		return nil
	}

	out := make([]export.ImageRef, 0, len(refs))
	for _, ref := range refs {
		key := strings.TrimSpace(ref.Key)
		if key == "" {
			continue
		}
		out = append(out, export.ImageRef{Key: key, Label: strings.TrimSpace(ref.Label)})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
