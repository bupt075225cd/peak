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
type RecognitionResult struct {
	StemText       string                   `json:"stem_text"`
	Answer         string                   `json:"answer"`
	Formula        provider.FormulaResult   `json:"formula"`
	Geometry       provider.GeometryResult  `json:"geometry"`
	ErasedImageKey string                   `json:"erased_image_key"`
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

// process 执行识别流程：OCR -> 公式 -> 几何 -> 手写擦除，更新任务状态。
func (s *Service) process(taskID uint64, storageKey string) {
	ctx := context.Background()
	start := time.Now()

	var task domain.RecognitionTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return
	}

	s.updateStatus(taskID, domain.TaskProcessing, 10, "")

	// 读取原始图片。
	imageData, err := s.storage.Get(ctx, storageKey)
	if err != nil {
		s.log.Error("read image failed", zap.String("error", err.Error()))
		s.updateStatus(taskID, domain.TaskFailed, 0, "read image failed")
		return
	}

	result := &RecognitionResult{}

	// 文本 OCR。
	textRes, err := s.prov.RecognizeText(ctx, imageData)
	if err != nil {
		s.log.Error("ocr failed", zap.String("error", err.Error()))
	} else {
		result.StemText = textRes.Text
	}
	s.updateStatus(taskID, domain.TaskProcessing, 40, "")

	// 公式识别。
	formulaRes, err := s.prov.RecognizeFormula(ctx, imageData)
	if err != nil {
		s.log.Error("formula failed", zap.String("error", err.Error()))
	} else {
		result.Formula = *formulaRes
	}
	s.updateStatus(taskID, domain.TaskProcessing, 60, "")

	// 几何图形识别。
	geoRes, err := s.prov.RecognizeGeometry(ctx, imageData)
	if err != nil {
		s.log.Error("geometry failed", zap.String("error", err.Error()))
	} else {
		result.Geometry = *geoRes
	}
	s.updateStatus(taskID, domain.TaskProcessing, 80, "")

	// 手写擦除。
	eraseRes, err := s.prov.EraseHandwriting(ctx, imageData)
	if err != nil {
		s.log.Error("erase failed", zap.String("error", err.Error()))
	} else if eraseRes != nil && len(eraseRes.ImageData) > 0 {
		key := erasedKey(taskID)
		if err := s.storage.Put(ctx, key, eraseRes.ImageData); err != nil {
			s.log.Error("store erased image failed", zap.String("error", err.Error()))
		} else {
			result.ErasedImageKey = key
		}
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
