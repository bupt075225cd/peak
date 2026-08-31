// Package service 实现识别任务的业务编排与异步处理状态机。
package service

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
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
	Subject        string                  `json:"subject,omitempty"`       // 学科：数学/语文/英语/物理/化学
	QuestionType   string                  `json:"question_type,omitempty"` // 题型：选择题/填空题/解答题
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
	Subject      string                  `json:"subject,omitempty"`       // 学科：数学/语文/英语/物理/化学
	QuestionType string                  `json:"question_type,omitempty"` // 题型：选择题/填空题/解答题
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

// processImage 处理图片：OCR -> 公式 -> 几何。
// 手写擦除不再在识别时自动执行，改为用户在前端勾选后调用 /api/recognition/erase 按需触发。
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
	// 学科/题型：优先模型识别，失败降级规则。
	result.Subject, result.QuestionType = s.classifyQuestion(ctx, imageData, result.StemText)
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

// EraseHandwriting 读取指定 storage key 的几何图子图，调用 provider 擦除手写，
// 将擦除结果写入 erased/ 前缀新 key 并返回。
func (s *Service) EraseHandwriting(ctx context.Context, key string) (string, error) {
	data, err := s.storage.Get(ctx, key)
	if err != nil {
		return "", errors.Wrap(errors.CodeNotFound, "image not found", err)
	}

	// wanx 要求输入图宽高落在 [512, 4096]，几何图子图可能超界，先等比缩放兜底。
	data, err = ensureWanSize(data)
	if err != nil {
		return "", errors.Wrap(errors.CodeInvalidArgument, "invalid image", err)
	}

	eraseRes, err := s.prov.EraseHandwriting(ctx, data)
	if err != nil {
		// 直接透传 wanx 错误 message（如 "wanx submit status 400: ..."），便于前端排查。
		return "", errors.Wrap(errors.CodeUpstream, err.Error(), err)
	}
	if eraseRes == nil || len(eraseRes.ImageData) == 0 {
		return "", errors.New(errors.CodeUpstream, "erase returned empty image")
	}

	newKey := "erased/" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".jpg"
	if err := s.storage.Put(ctx, newKey, eraseRes.ImageData); err != nil {
		return "", errors.Wrap(errors.CodeStorageFail, "store erased image failed", err)
	}
	return newKey, nil
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

// 题型判断正则。
var (
	// choiceMarkRe 选择题选项标记：A-D 后跟句点/全角句点/冒号及非空内容。
	// 排除顿号（A、B、C），避免把几何题里的“点 A、B、C”误判为选项。
	choiceMarkRe = regexp.MustCompile(`[A-Da-d][.．:：]\s*\S`)
	// blankRe 填空题空位：连续下划线，或全角/半角空括号。
	blankRe = regexp.MustCompile(`_{2,}|（\s*）|\(\s*\)`)
)

// detectQuestionType 根据题干文本启发式判断题型，返回"选择题"/"填空题"/"解答题"。
func detectQuestionType(stem string) string {
	s := strings.TrimSpace(stem)

	// 选择题：至少出现两个选项标记（如 A. xxx B. xxx）。
	if len(choiceMarkRe.FindAllString(s, -1)) >= 2 {
		return "选择题"
	}
	// 填空题：出现下划线填空或空括号。
	if blankRe.MatchString(s) {
		return "填空题"
	}
	// 其余默认解答题。
	return "解答题"
}

// 学科判断正则（启发式，按特征强度排序，命中即返回）。
var (
	// englishWordRe 英语常见词。用常见词而非“连续字母”判断，避免几何题的字母（AB、CD、∠ABC）被误判为英语。
	englishWordRe = regexp.MustCompile(`(?i)\b(the|what|which|is|are|was|were|of|to|in|on|for|and|or|not|choose|answer|following|correct|best|true|false|read|write|text|word|sentence|letter|from|with|about|this|that|there|here|you|your|my|he|she|it|we|they)\b`)
	// chemistryRe 化学特征关键词。
	chemistryRe = regexp.MustCompile(`化学|元素|化合物|化合价|离子|酸|碱|盐|溶液|氧化|还原|摩尔|催化|电解|置换|复分解|中和|溶质|溶剂|沉淀|化学式|化学方程式|质量守恒|分子|原子|盐酸|硫酸|氢气|氧气|二氧化碳`)
	// physicsRe 物理特征关键词。
	physicsRe = regexp.MustCompile(`速度|加速度|电压|电流|电阻|功率|压强|浮力|密度|电路|磁场|摩擦|重力|杠杆|透镜|折射|反射|串联|并联|牛顿|焦耳|欧姆|瓦特|安培|伏特|电荷|电场|动量|动能|势能|做功|机械能|弹簧|滑轮|频率|波长|振幅`)
	// chineseRe 语文特征关键词。
	chineseRe = regexp.MustCompile(`古诗|文言|阅读|字词|拼音|修辞|病句|标点|默写|翻译|作者|诗句|成语|歇后语|散文|小说|诗歌|注音|释义|选词|组词|造句|朗读|背诵`)
)

// detectSubject 根据题干文本启发式判断学科，返回"数学"/"语文"/"英语"/"物理"/"化学"。
// 优先命中强特征学科；无强特征时兜底为数学（数学错题为主，且数学应用题常无强关键词）。
func detectSubject(stem string) string {
	s := strings.TrimSpace(stem)

	if englishWordRe.MatchString(s) {
		return "英语"
	}
	if chemistryRe.MatchString(s) {
		return "化学"
	}
	if physicsRe.MatchString(s) {
		return "物理"
	}
	if chineseRe.MatchString(s) {
		return "语文"
	}
	return "数学"
}

// classifyQuestion 判断题目学科与题型：优先调用识别模型，失败时降级为规则判断。
func (s *Service) classifyQuestion(ctx context.Context, image []byte, stem string) (string, string) {
	if cls, err := s.prov.ClassifyQuestion(ctx, image); err == nil && cls != nil {
		if cls.Subject != "" && cls.QuestionType != "" {
			return cls.Subject, cls.QuestionType
		}
	}
	return detectSubject(stem), detectQuestionType(stem)
}
