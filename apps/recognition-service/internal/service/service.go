// Package service 实现识别任务的业务编排与异步处理状态机。
package service

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"peak/libs/domain"
	"peak/libs/errors"
	"peak/libs/logger"
	"peak/libs/storage"

	"peak/apps/recognition-service/internal/provider"
)

// RecognitionResult 识别任务最终结果（回填给题目服务）。
// 图片上传：单题结构（StemText 等字段）。
// 文档上传：多题结构（Questions 数组）。
type RecognitionResult struct {
	StemText       string                  `json:"stem_text"`
	Answer         string                  `json:"answer"`
	Formula        provider.FormulaResult  `json:"formula"`
	Geometry       provider.GeometryResult `json:"geometry"`
	ErasedImageKey string                  `json:"erased_image_key"`
	// GeometryKeys 单图上传场景下，与该题关联的几何图存储 key 列表。
	// （文档上传场景下，几何图分散在各子问的 GeometryKeys 中。）
	GeometryKeys []string `json:"geometry_keys,omitempty"`
	// Questions 文档识别出的多道题（仅文档上传时填充）。
	Questions []QuestionItem `json:"questions,omitempty"`
	// Warning 非致命错误提示（如公式/几何识别失败），供前端展示。
	Warning string `json:"warning,omitempty"`
}

// QuestionItem 文档中拆分出的单道题。
type QuestionItem struct {
	StemText     string                  `json:"stem_text"`
	Answer       string                  `json:"answer"`
	Formula      provider.FormulaResult  `json:"formula"`
	Geometry     provider.GeometryResult `json:"geometry"`
	// SubQuestions 子问列表，如 (1)(2)(3)（方案 B 结构化拆题时填充）。
	SubQuestions []QuestionSubQuestion `json:"sub_questions,omitempty"`
}

// QuestionSubQuestion 服务层的子问结构，在 provider.SubQuestion 基础上附加几何图片 key。
type QuestionSubQuestion struct {
	Label        string   `json:"label"`
	Text         string   `json:"text"`
	GeometryRefs []int    `json:"geometry_refs,omitempty"`
	GeometryDesc string   `json:"geometry_desc"`
	GeometryKeys []string `json:"geometry_keys,omitempty"`
}

// Service 识别服务业务逻辑。
type Service struct {
	db      *gorm.DB
	storage storage.FileStorage
	prov    provider.Provider
	log     *logger.Logger
}

// New 创建识别服务实例。
func New(db *gorm.DB, store storage.FileStorage, prov provider.Provider, log *logger.Logger) *Service {
	return &Service{db: db, storage: store, prov: prov, log: log}
}

// CreateTask 创建识别任务并异步执行。
func (s *Service) CreateTask(ctx context.Context, imageID uint64, storageKey string) (*domain.RecognitionTask, error) {
	task := &domain.RecognitionTask{
		ImageID:  imageID,
		Status:   domain.TaskPending,
		Progress: 0,
		Provider: s.prov.Name(),
	}
	if err := s.db.WithContext(ctx).Create(task).Error; err != nil {
		return nil, err
	}
	// 异步执行（简化：go routine；生产可接入消息队列）。
	go s.process(task.ID, storageKey)
	return task, nil
}

// GetTask 查询任务状态。
func (s *Service) GetTask(ctx context.Context, id uint64) (*domain.RecognitionTask, error) {
	var task domain.RecognitionTask
	if err := s.db.WithContext(ctx).First(&task, id).Error; err != nil {
		return nil, errors.Wrap(errors.CodeNotFound, "task not found", err)
	}
	return &task, nil
}

// RetryTask 重试失败任务。
func (s *Service) RetryTask(ctx context.Context, id uint64) error {
	var task domain.RecognitionTask
	if err := s.db.WithContext(ctx).First(&task, id).Error; err != nil {
		return errors.Wrap(errors.CodeNotFound, "task not found", err)
	}
	task.Status = domain.TaskPending
	task.Progress = 0
	task.ErrorMessage = ""
	task.RetryCount++
	if err := s.db.WithContext(ctx).Save(&task).Error; err != nil {
		return err
	}
	go s.process(task.ID, "")
	return nil
}

// process 执行识别流程，根据文件类型分流：图片走 OCR 流程，文档走解析流程。
func (s *Service) process(taskID uint64, storageKey string) {
	ctx := context.Background()
	start := time.Now()

	var task domain.RecognitionTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return
	}

	// 查询关联文件，判断是图片还是文档。
	var img domain.Image
	if err := s.db.First(&img, task.ImageID).Error; err != nil {
		s.updateStatus(taskID, domain.TaskFailed, 0, "image not found")
		return
	}

	// storageKey 为空时（如重试场景），从图片记录恢复。
	if storageKey == "" {
		storageKey = img.StorageKey
	}

	s.updateStatus(taskID, domain.TaskProcessing, 10, "")

	var result *RecognitionResult
	var err error
	if img.ImageType == domain.ImageTypeDocument {
		result, err = s.processDocument(ctx, taskID, storageKey)
	} else {
		result, err = s.processImage(ctx, taskID, storageKey)
	}
	if err != nil {
		s.log.Error("recognition failed", zap.String("error", err.Error()))
		s.updateStatus(taskID, domain.TaskFailed, 0, err.Error())
		return
	}

	// 序列化结果并标记成功。
	resultJSON, _ := json.Marshal(result)
	s.db.Model(&domain.RecognitionTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"status":      domain.TaskSuccess,
		"progress":    100,
		"result_json": string(resultJSON),
	})
	s.log.Info("recognition task done",
		zap.Uint64("task_id", taskID),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	)
}

// processImage 处理图片：OCR -> 公式 -> 几何 -> 手写擦除。
// 几何图部分：根据模型返回的外接矩形（bounding_box）从原图中裁剪出
// “只有几何图”的子图，存为 geometry/task_<id>.jpg 并写入 GeometryKeys，
// 供前端录入错题时关联展示，避免把题干文字整图附在题目后面。
func (s *Service) processImage(ctx context.Context, taskID uint64, storageKey string) (*RecognitionResult, error) {
	imageData, err := s.storage.Get(ctx, storageKey)
	if err != nil {
		return nil, err
	}

	result := &RecognitionResult{}

	// 文本 OCR（关键步骤，失败则任务失败）。
	textRes, err := s.prov.RecognizeText(ctx, imageData)
	if err != nil {
		s.log.Error("ocr failed", zap.String("error", err.Error()))
		return nil, err
	}
	result.StemText = textRes.Text
	s.updateStatus(taskID, domain.TaskProcessing, 40, "")

	// 公式识别（增强步骤，失败仅记录 warning）。
	if formulaRes, ferr := s.prov.RecognizeFormula(ctx, imageData); ferr != nil {
		s.log.Error("formula failed", zap.String("error", ferr.Error()))
		result.Warning = "公式识别失败：" + ferr.Error()
	} else {
		result.Formula = *formulaRes
	}
	s.updateStatus(taskID, domain.TaskProcessing, 60, "")

	// 几何图形识别（增强步骤，失败仅记录 warning）。
	var geoBBox *provider.BoundingBox
	if geoRes, gerr := s.prov.RecognizeGeometry(ctx, imageData); gerr != nil {
		s.log.Error("geometry failed", zap.String("error", gerr.Error()))
		if result.Warning != "" {
			result.Warning += "；"
		}
		result.Warning += "几何识别失败：" + gerr.Error()
	} else {
		result.Geometry = *geoRes
		geoBBox = geoRes.BoundingBox
	}
	s.updateStatus(taskID, domain.TaskProcessing, 80, "")

	// 手写擦除。
	if eraseRes, err := s.prov.EraseHandwriting(ctx, imageData); err != nil {
		s.log.Error("erase failed", zap.String("error", err.Error()))
	} else if eraseRes != nil && len(eraseRes.ImageData) > 0 {
		key := erasedKey(taskID)
		if err := s.storage.Put(ctx, key, eraseRes.ImageData); err != nil {
			s.log.Error("store erased image failed", zap.String("error", err.Error()))
		} else {
			result.ErasedImageKey = key
		}
	}

	// 单图场景：根据几何图形外接矩形裁剪出“只有几何图”的子图存储。
	// 无有效 bbox 时不附加任何图（不再把整张原图当作几何图）。
	if geoBBox != nil {
		if cropped, err := cropGeometryImage(imageData, geoBBox); err != nil {
			s.log.Error("crop geometry image failed", zap.String("error", err.Error()))
			if result.Warning != "" {
				result.Warning += "；"
			}
			result.Warning += "几何图裁剪失败：" + err.Error()
		} else {
			key := geometryKey(taskID)
			if err := s.storage.Put(ctx, key, cropped); err != nil {
				s.log.Error("store geometry image failed", zap.String("error", err.Error()))
			} else {
				result.GeometryKeys = []string{key}
			}
		}
	}

	return result, nil
}

// processDocument 处理 word/pdf 文档：解析文本+图片，拆分多道题，图片走 OCR。
func (s *Service) processDocument(ctx context.Context, taskID uint64, storageKey string) (*RecognitionResult, error) {
	data, err := s.storage.Get(ctx, storageKey)
	if err != nil {
		return nil, err
	}

	filename := filenameFromKey(storageKey)

	// 方案 B：优先走结构化拆题（含子问/几何），失败时回退到旧启发式拆分。
	if structured, err := s.prov.ExtractStructured(ctx, data, filename); err == nil {
		s.updateStatus(taskID, domain.TaskProcessing, 60, "")
		// 存图并映射 geometry_refs -> geometry_keys。
		imgKeys := s.storeDocumentImages(ctx, taskID, structured.Images)
		questions := fromStructured(structured, imgKeys)
		s.updateStatus(taskID, domain.TaskProcessing, 90, "")
		return &RecognitionResult{Questions: questions}, nil
	}

	// 回退：解析文档内容项，用正则启发式拆分。
	doc, err := s.prov.ExtractDocument(ctx, data, filename)
	if err != nil {
		return nil, err
	}
	s.updateStatus(taskID, domain.TaskProcessing, 40, "")

	questions := splitQuestions(ctx, s.prov, doc.Items)
	s.updateStatus(taskID, domain.TaskProcessing, 90, "")

	return &RecognitionResult{Questions: questions}, nil
}

// updateStatus 更新任务进度与状态。
func (s *Service) updateStatus(taskID uint64, status string, progress int, errMsg string) {
	s.db.Model(&domain.RecognitionTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"status":        status,
		"progress":      progress,
		"error_message": errMsg,
	})
}

// erasedKey 生成擦除后图片的存储 key。
func erasedKey(taskID uint64) string {
	return "erased/task_" + itoa(taskID) + ".jpg"
}

// geometryKey 生成单图场景下几何图（原图或擦除后的图）的存储 key。
func geometryKey(taskID uint64) string {
	return "geometry/task_" + itoa(taskID) + ".jpg"
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
