// Package service 实现识别任务的业务编排与异步处理状态机。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"peak/libs/domain"
	"peak/libs/errors"
	"peak/libs/logger"
	"peak/libs/observability"
	"peak/libs/storage"

	"peak/apps/recognition-service/internal/geom"
	"peak/apps/recognition-service/internal/provider"
)

// RecognitionResult 识别任务最终结果（回填给题目服务）。
// 图片上传：单题结构（StemText 等字段）。
// 文档上传：多题结构（Questions 数组）。
type RecognitionResult struct {
	StemText     string `json:"stem_text"`
	Answer       string `json:"answer"`
	Subject      string `json:"subject,omitempty"`       // 学科：数学/语文/英语/物理/化学
	QuestionType string `json:"question_type,omitempty"` // 题型：选择题/填空题/解答题
	// Tables 题干中的表格（分档计费表、数据对照表等）：StemText 里以 {{table1}} 占位，
	// 前端渲染成表格、导出时序列化成 Markdown 表格。
	Tables   []provider.Table        `json:"tables,omitempty"`
	Geometry provider.GeometryResult `json:"geometry"`
	// RedrawFigures 几何重绘输出的子图列表：key 为独立 SVG 的存储 key，
	// label 为该子图在题干中的图号（如"图1"）。一张原图含多个几何子图时逐个填充
	// （启用了内置几何渲染时填充）。
	RedrawFigures []RedrawFigure `json:"redraw_figures,omitempty"`
	// RedrawReport 几何重绘报告（重试轮数、结构校验是否通过）。
	RedrawReport *RedrawReport `json:"redraw_report,omitempty"`
	// ImageRotation 入口把整图转正所用的顺时针角度（0/90/180/270，不含 EXIF 那部分）。
	// 前端据此把本地预览图转到与服务端识别时相同的朝向，避免"用户看的图"与"模型看的图"不一致。
	ImageRotation int `json:"image_rotation,omitempty"`
	// Questions 文档识别出的多道题（仅文档上传时填充）。
	Questions []QuestionItem `json:"questions,omitempty"`
	// Warning 非致命错误提示（如公式/几何识别失败），供前端展示。
	Warning string `json:"warning,omitempty"`
}

// RedrawFigure 几何重绘产出的单个子图。
//
// 重绘只保留图形本身，原图上的"图1/图2"文字标注不会出现在新 SVG 里；label 来自
// 几何描述提取阶段的 panel.title（模型未给出时按下标兜底为"图1/图2…"），
// 保存后即可在导出文档中把图号标注回配图。
type RedrawFigure struct {
	Key   string `json:"key"`
	Label string `json:"label,omitempty"`
}

// QuestionItem 文档中拆分出的单道题。
type QuestionItem struct {
	StemText     string                  `json:"stem_text"`
	Answer       string                  `json:"answer"`
	Subject      string                  `json:"subject,omitempty"`       // 学科：数学/语文/英语/物理/化学
	QuestionType string                  `json:"question_type,omitempty"` // 题型：选择题/填空题/解答题
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

// RedrawReport 几何重绘报告。
//
// 坐标直出方案不再有约束残差，max_hard/max_soft 仅为兼容前端契约保留（恒为 0）；
// consistent 表示全部子图通过结构校验。
type RedrawReport struct {
	MaxHard    float64 `json:"max_hard"`   // Deprecated: 残差语义已废弃，恒为 0
	MaxSoft    float64 `json:"max_soft"`   // Deprecated: 残差语义已废弃，恒为 0
	Attempts   int     `json:"attempts"`   // 实际执行的"提取→渲染"轮数
	Consistent bool    `json:"consistent"` // 是否全部子图通过结构校验
	// StructIssues 最终输出版本的结构问题数（0 表示结构完整，可直接渲染）。
	StructIssues int `json:"struct_issues,omitempty"`
	// SemanticIssues 最终输出版本的语义偏差数（多源于题干转录不准，只记日志不重试）。
	SemanticIssues int `json:"semantic_issues,omitempty"`
}

// Service 识别服务业务逻辑。
type Service struct {
	db      *gorm.DB
	storage storage.FileStorage
	prov    provider.Provider
	log     *logger.Logger
	// geometryRender 是否启用几何重绘（内置 Go 渲染器）。
	geometryRender bool
	// geometryMaxAttempts 结构校验失败回喂修正的最大轮数（提取→渲染→修正）。
	geometryMaxAttempts int
	// geometryRedrawTimeout 重绘阶段整体超时：超时后跳过重绘、保留原图识别结果。
	geometryRedrawTimeout time.Duration
	// wg 跟踪在途的异步识别流程，Wait 供测试收尾等待，避免后台协程
	// 与临时目录清理（t.TempDir）竞态。
	wg sync.WaitGroup
}

// Option 服务可选依赖。
type Option func(*Service)

// WithGeometryRender 启用几何重绘（内置 Go 渲染器）：enabled 控制开关，
// maxAttempts 为结构校验失败回喂修正的最大轮数。
func WithGeometryRender(enabled bool, maxAttempts int) Option {
	return func(s *Service) {
		s.geometryRender = enabled
		if maxAttempts < 1 {
			maxAttempts = 1
		}
		s.geometryMaxAttempts = maxAttempts
	}
}

// WithGeometryRenderTimeout 设置重绘阶段整体超时；d<=0 表示不限制。
func WithGeometryRenderTimeout(d time.Duration) Option {
	return func(s *Service) { s.geometryRedrawTimeout = d }
}

// New 创建识别服务实例。
func New(db *gorm.DB, store storage.FileStorage, prov provider.Provider, log *logger.Logger, opts ...Option) *Service {
	s := &Service{db: db, storage: store, prov: prov, log: log, geometryMaxAttempts: 3}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// CreateTask 创建识别任务并异步执行。userID 为归属用户（0 视为非法，由调用方校验）。
func (s *Service) CreateTask(ctx context.Context, userID uint64, imageID uint64, storageKey string) (*domain.RecognitionTask, error) {
	task := &domain.RecognitionTask{
		UserID:   userID,
		ImageID:  imageID,
		Status:   domain.TaskPending,
		Progress: 0,
		Provider: s.prov.Name(),
	}
	if err := s.db.WithContext(ctx).Create(task).Error; err != nil {
		return nil, err
	}
	// 异步执行（简化：go routine；生产可接入消息队列）。
	// WithoutCancel：脱离请求生命周期，但保留 trace 上下文，任务链路可串联到触发请求。
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.process(context.WithoutCancel(ctx), task.ID, storageKey)
	}()
	return task, nil
}

// GetTask 查询任务状态。仅返回属于 userID 的任务，否则按不存在处理。
func (s *Service) GetTask(ctx context.Context, userID, id uint64) (*domain.RecognitionTask, error) {
	var task domain.RecognitionTask
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&task).Error; err != nil {
		return nil, errors.Wrap(errors.CodeNotFound, "task not found", err)
	}
	return &task, nil
}

// RetryTask 重试失败任务。仅允许任务属主操作。
func (s *Service) RetryTask(ctx context.Context, userID, id uint64) error {
	var task domain.RecognitionTask
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&task).Error; err != nil {
		return errors.Wrap(errors.CodeNotFound, "task not found", err)
	}
	task.Status = domain.TaskPending
	task.Progress = 0
	task.ProgressText = ""
	task.ErrorMessage = ""
	task.RetryCount++
	if err := s.db.WithContext(ctx).Save(&task).Error; err != nil {
		return err
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.process(context.WithoutCancel(ctx), task.ID, "")
	}()
	return nil
}

// Wait 等待全部在途的异步识别流程结束。测试收尾时调用，
// 避免后台协程在 t.TempDir 清理后仍写文件/数据库导致的竞态。
func (s *Service) Wait() { s.wg.Wait() }

// process 执行识别流程，根据文件类型分流：图片走 OCR 流程，文档走解析流程。
// 全程记录业务指标（任务耗时/成败）与链路 span（可定位卡在哪个 VLM 阶段）。
func (s *Service) process(ctx context.Context, taskID uint64, storageKey string) {
	start := time.Now()

	var task domain.RecognitionTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return
	}

	// 查询关联文件，判断是图片还是文档。
	var img domain.Image
	if err := s.db.First(&img, task.ImageID).Error; err != nil {
		s.observeTask(ctx, domain.TaskFailed, start)
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
		s.observeTask(ctx, domain.TaskFailed, start)
		s.updateStatus(taskID, domain.TaskFailed, 0, err.Error())
		// 失败即时清理：任务失败后其重绘产物（geometry/task_<id>.）不会再被
		// 任何错题引用，直接从存储删除，避免未提交流程在对象存储中留下孤儿文件。
		// 原图（original/…）必须保留：失败后用户可点重试，重试需从存储重新读取原图。
		s.cleanupTaskArtifacts(taskID)
		return
	}

	// 序列化结果并标记成功。
	resultJSON, _ := json.Marshal(result)
	resStr := string(resultJSON)
	s.db.Model(&domain.RecognitionTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"status":        domain.TaskSuccess,
		"progress":      100,
		"progress_text": "",
		"result_json":   &resStr,
	})
	s.observeTask(ctx, domain.TaskSuccess, start)
	s.log.Info("recognition task done",
		zap.Uint64("task_id", taskID),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	)
}

// observeTask 记录识别任务的业务指标（端到端耗时 + 成败计数）。
func (s *Service) observeTask(ctx context.Context, status string, start time.Time) {
	_, span := observability.Tracer().Start(ctx, "recognition.task",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String("recognition.provider", s.prov.Name()),
			attribute.String("recognition.status", status)))
	observability.FinishSpan(span, nil)
	observability.RecognitionTaskDuration.WithLabelValues(s.prov.Name(), status).Observe(time.Since(start).Seconds())
	observability.RecognitionTaskTotal.WithLabelValues(s.prov.Name(), status).Inc()
}

// callProvider 包裹一次第三方 AI provider 调用：创建 client span + 指标埋点，
// 便于区分"慢在自家代码还是第三方 AI"、统计各能力调用的失败率与耗时分布。
func (s *Service) callProvider(ctx context.Context, operation string, call func(context.Context) error) {
	ctx, span := observability.StartAISpan(ctx, s.prov.Name(), operation)
	start := time.Now()
	err := call(ctx)
	observability.ObserveAICall(s.prov.Name(), operation, err, start)
	observability.FinishSpan(span, err)
}

// processImage 处理图片：整题解析（题干+学科+题型一次 VLM）∥ 几何识别（并发）→ 几何重绘。
// 优化点：
//   - 合并原 OCR + 学科/题型分类两次调用为一次 ParseQuestion；
//   - 几何识别与整题解析并行发起，重叠耗时；
//   - 每个阶段实时更新 progress_text 并记录耗时日志（便于定位慢点）。
//
// 几何重绘直接用原始图片（不再裁剪子图）：数学题且图中含几何图形时，
// 把题干文本交给 VLM 坐标直出各子图的几何描述，内置渲染器每个子图渲染一张独立 SVG。
func (s *Service) processImage(ctx context.Context, taskID uint64, storageKey string) (*RecognitionResult, error) {
	imageData, err := s.storage.Get(ctx, storageKey)
	if err != nil {
		return nil, err
	}
	// 图片方向统一在入口处理：几何识别的 bbox 是相对"视觉正立图"给的归一化坐标，
	// 侧躺/倒立的图不先生转正，会出现"框到别处、图形被切掉一半"，倒立图甚至被判成
	// 没有几何图形。EXIF 有方向标记就按标记转（零网络成本），没有标记（微信转发图、
	// 截图常见）时让模型判一次倾斜角。
	imageData, rotation := s.uprightImage(ctx, taskID, imageData)

	result := &RecognitionResult{ImageRotation: rotation}
	s.updateProgress(taskID, 10, "正在识别题干…")

	// 几何识别与整题解析相互独立 → 并行发起，二者耗时重叠。
	type geomOut struct {
		res *provider.GeometryResult
		err error
	}
	geoCh := make(chan geomOut, 1)
	go func() {
		gstart := time.Now()
		var g *provider.GeometryResult
		var gerr error
		s.callProvider(ctx, "geometry_recognize", func(c context.Context) (err error) {
			g, gerr = s.prov.RecognizeGeometry(c, imageData)
			return gerr
		})
		s.logStage("geometry-recognize", gstart)
		geoCh <- geomOut{res: g, err: gerr}
	}()

	// 整题解析：题干文本 + 学科 + 题型，一次 VLM 调用（关键步骤，失败则任务失败）。
	parseStart := time.Now()
	var parse *provider.QuestionParseResult
	var perr error
	s.callProvider(ctx, "parse_question", func(c context.Context) (err error) {
		parse, perr = s.prov.ParseQuestion(c, imageData)
		return perr
	})
	if perr != nil {
		s.log.Error("parse question failed", zap.String("error", perr.Error()))
		return nil, perr
	}
	s.logStage("parse-question", parseStart)

	result.StemText = parse.Text
	result.Subject = parse.Subject
	result.QuestionType = parse.QuestionType
	result.Tables = parse.Tables
	// 模型未给出学科/题型时降级为规则判断（不再额外调 VLM）。
	if result.Subject == "" {
		result.Subject = detectSubject(result.StemText)
	}
	if result.QuestionType == "" {
		result.QuestionType = detectQuestionType(result.StemText)
	}
	s.updateProgress(taskID, 40, "正在识别图形…")

	// 接收并发几何识别的结果（增强步骤，失败仅记录 warning）。
	// 模型返回 bounding_box 即认为图中含几何图形，用于决定是否触发重绘。
	hasGeometry := false
	var geomBBox *provider.BoundingBox
	if gout := <-geoCh; gout.err != nil {
		s.log.Error("geometry failed", zap.String("error", gout.err.Error()))
		if result.Warning != "" {
			result.Warning += "；"
		}
		result.Warning += "几何识别失败：" + gout.err.Error()
	} else {
		result.Geometry = *gout.res
		// 入口已按模型判定的倾斜角把整图转正；几何识别若仍报倾斜角，一般是同一张图被重复判读，
		// 以入口结论为准。但报 180° 是特例：说明入口把顺时针 90/270 判反了，整图被转成了倒立图——
		// 这种图上"上下左右"全反（实测把 B(0,4) 画到原点下方、l1 斜率取成负），必须翻回来再提取。
		if rotation != 0 && result.Geometry.Rotation == 180 {
			if fixed, ferr := rotateImageData(imageData, 180); ferr == nil {
				imageData = fixed
				rotation = (rotation + 180) % 360
				result.ImageRotation = rotation
				geomBBox = flipBBox180(gout.res.BoundingBox)
				s.log.Warn("entry rotation looked upside down, image flipped back 180",
					zap.Uint64("task_id", taskID), zap.Int("tilt", 180),
					zap.Int("rotation", rotation))
			}
		} else if rotation != 0 && result.Geometry.Rotation != 0 {
			s.log.Info("geometry tilt ignored, image already upright at entry",
				zap.Uint64("task_id", taskID), zap.Int("tilt", result.Geometry.Rotation.Degrees()),
				zap.Int("entry_rotation", rotation))
		}
		// 入口（含上面的翻转）已经决定了朝向，几何阶段不再二次旋转。
		result.Geometry.Rotation = 0
		if geomBBox == nil {
			geomBBox = gout.res.BoundingBox
		}
		hasGeometry = geomBBox != nil
	}
	s.updateProgress(taskID, 60, "识别完成")

	// 几何重绘（增强步骤，失败仅记录 warning）：
	// 数学题 + 图中含几何图形时，对原始图片按 bbox 裁出几何区域并下采样成小图，
	// 作为 VLM 几何描述提取的输入（不落库），以大幅减少 vision token、缩短耗时。
	if s.geometryRender && result.Subject == "数学" && hasGeometry {
		redrawStart := time.Now()
		// 整体 deadline：单次 VLM 几何提取可能耗时数分钟，失败重试回路
		// 最坏可耗 30 分钟以上且任务级 context 无 deadline；这里对重绘
		// 阶段单独限时，超时降级为跳过重绘（保留原图识别结果），不判任务失败。
		redrawCtx := ctx
		if s.geometryRedrawTimeout > 0 {
			var cancel context.CancelFunc
			redrawCtx, cancel = context.WithTimeout(ctx, s.geometryRedrawTimeout)
			defer cancel()
		}
		if rerr := s.redrawGeometry(redrawCtx, taskID, imageData, geomBBox, result); rerr != nil {
			if ctxErr := redrawCtx.Err(); ctxErr == context.DeadlineExceeded {
				observability.ObserveGeometryRedraw(s.prov.Name(), "timeout", 0, 0, 0, redrawStart)
				s.log.Warn("geometry redraw timeout, skip redraw",
					zap.Duration("timeout", s.geometryRedrawTimeout))
				if result.Warning != "" {
					result.Warning += "；"
				}
				result.Warning += "几何重绘超时，已跳过（仅保留原图）"
			} else {
				observability.ObserveGeometryRedraw(s.prov.Name(), "failed", 0, 0, 0, redrawStart)
				s.log.Error("geometry redraw failed", zap.String("error", rerr.Error()))
				if result.Warning != "" {
					result.Warning += "；"
				}
				result.Warning += "几何重绘失败：" + rerr.Error()
			}
		} else {
			// 记录重绘结果指标：ok=已产出子图，empty=未检出几何图形（干净跳过）。
			report := result.RedrawReport
			attempts, structIssues, semanticIssues := 0, 0, 0
			outcome := "empty"
			if report != nil {
				attempts = report.Attempts
				structIssues = report.StructIssues
				semanticIssues = report.SemanticIssues
				if len(result.RedrawFigures) > 0 {
					outcome = "ok"
				}
			}
			observability.ObserveGeometryRedraw(s.prov.Name(), outcome, attempts, structIssues, semanticIssues, redrawStart)
			s.logStage("geometry-redraw", redrawStart)
		}
	}

	return result, nil
}

// flipBBox180 把归一化 bbox 换算到图片旋转 180° 后的坐标系：
// 原图左上角对应新图右下角，宽高不变。
func flipBBox180(b *provider.BoundingBox) *provider.BoundingBox {
	if b == nil {
		return nil
	}
	return &provider.BoundingBox{
		X:      clamp01(1 - b.X - b.Width),
		Y:      clamp01(1 - b.Y - b.Height),
		Width:  b.Width,
		Height: b.Height,
	}
}

func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}

// updateProgress 更新任务进度与当前阶段文案（前端展示"正在做什么"）。
func (s *Service) updateProgress(taskID uint64, progress int, text string) {
	s.db.Model(&domain.RecognitionTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"status":        domain.TaskProcessing,
		"progress":      progress,
		"progress_text": text,
		"error_message": "",
	})
}

// logStage 记录单个 VLM 阶段的耗时（用于定位慢点）。
func (s *Service) logStage(stage string, start time.Time) {
	s.log.Info("recognition stage",
		zap.String("stage", stage), zap.Int64("ms", time.Since(start).Milliseconds()))
}

// redrawGeometry 执行几何重绘：几何描述提取（含结构校验失败回喂修正回路）
// → 内置 Go 渲染器逐子图渲染 SVG → 存储。
// imageData 为原始图片字节；bbox 为几何识别返回的图形区域，用于把 VLM 提取的
// 输入从整张原图裁成几何区域小图（不落库、不改产物），显著降低 vision token。
// result.StemText 为题干文本；每个子图（panel）各自存储独立 key。
func (s *Service) redrawGeometry(ctx context.Context, taskID uint64, imageData []byte, bbox *provider.BoundingBox, result *RecognitionResult) error {
	// provider 需支持几何描述提取能力（mock/aliyun 均已实现）。
	extractor, ok := s.prov.(provider.GeometrySpecExtractor)
	if !ok {
		return nil
	}

	// 提取的 VLM 输入：优先使用几何区域小图（输入瘦身）；失败则回退整张原图。
	// 裁剪前会按墨迹连通域做"区域完整性"规划：模型框偏紧时被切断的轴线/边会被补全，
	// 框内没有墨迹或并集过大时回退整图，避免把 E 这类端点裁到画面外。
	extractImage := imageData
	regionFallback := false // 是否还可回退整张原图（空 panels 拒绝时用）
	// 网格图的格线是很淡的虚线，缩到 960 宽会被糊掉、模型数不准格子，放宽分辨率。
	regionMaxDim := geometryRegionMaxDim
	if isGridFigure(result.StemText, result.Geometry.Description) {
		regionMaxDim = geometryGridRegionMaxDim
	}
	if bbox != nil {
		if small, plan, cerr := cropAndScaleRegionPlanned(imageData, bbox, regionMaxDim); cerr == nil {
			extractImage = small
			regionFallback = true
			s.log.Info("geometry extract input shrunk",
				zap.Uint64("task_id", taskID), zap.String("plan", plan),
				zap.Int("src_bytes", len(imageData)), zap.Int("region_bytes", len(small)))
		} else {
			s.log.Warn("geometry region prep failed, fallback to full image",
				zap.Uint64("task_id", taskID), zap.String("error", cerr.Error()))
		}
	}
	// 图片侧躺时把几何区域转正：bbox 仍按原图坐标裁剪，故在裁剪之后旋转。
	// rotation 存的是"内容顺时针倾斜角"，转正要用 FixDegrees 取反向修正量；
	// regionImage 保留未旋转版本，供旋转方向判断错误时换个方向重试。
	regionImage := extractImage
	rotationFix := 0
	if deg := result.Geometry.Rotation.FixDegrees(); deg != 0 {
		if upright, rerr := rotateImageData(regionImage, deg); rerr != nil {
			s.log.Warn("geometry region rotate failed",
				zap.Uint64("task_id", taskID), zap.Int("rotation", deg), zap.String("error", rerr.Error()))
		} else {
			extractImage = upright
			rotationFix = deg
			s.log.Info("geometry region rotated upright",
				zap.Uint64("task_id", taskID), zap.Int("tilt", result.Geometry.Rotation.Degrees()),
				zap.Int("rotation", deg))
		}
	}

	var panels []geom.PanelResult
	var lastErr error
	correction := ""
	attempts := 0
	// 无效轮（输出被截断、JSON 不合法、空 panels 等未产出可校验结果的轮次）
	// 不消耗修正预算，只受独立的重试上限约束，防止传输抖动/朝向误判死循环；
	// 实测同一张图 4 轮里可能有 2 轮被截断与空结果浪费，修正根本没机会生效。
	const geometryMaxInfraRetries = 3
	infraRetries := 0
	// 修正回路里问题最少的一版（结构问题数、语义偏差数），循环结束后据此输出。
	var best []geom.PanelResult
	bestHard, bestSoft := 0, 0
	var bestIssues []string
	// 入口转正方向判反时的撤销重试只做一次，避免来回旋转耗光重试轮数。
	entryUprightOppositeTried := false
	// valid 统计"有效轮"（产出非空 panels 且完成校验）——才消耗 geometryMaxAttempts 修正预算。
	for valid := 0; valid < s.geometryMaxAttempts && infraRetries <= geometryMaxInfraRetries; {
		// 超时/取消后立即退出：不更新进度、不再发起注定失败的调用。
		if ctx.Err() != nil {
			break
		}
		attempts++
		// 重绘轮次可能较久（VLM 提取几何描述），实时更新进度文案让用户感知。
		p := 80 + valid*8
		if p > 94 {
			p = 94
		}
		s.updateProgress(taskID, p, fmt.Sprintf("正在重绘图形（第 %d 次尝试）…", valid+1))
		var spec string
		var eerr error
		s.callProvider(ctx, "geometry_spec_extract", func(c context.Context) (err error) {
			spec, eerr = extractor.ExtractGeometrySpec(c, extractImage, result.StemText, correction)
			return eerr
		})
		if eerr != nil {
			lastErr = eerr
			infraRetries++
			fields := []zap.Field{zap.Int("attempt", attempts), zap.String("error", eerr.Error())}
			// 模型输出不是合法 JSON 时把原文带进日志：否则只剩一句
			// "model output is not valid JSON"，完全看不出是截断、多段还是转义问题。
			if moe, ok := eerr.(*provider.ModelOutputError); ok {
				fields = append(fields, zap.String("raw", truncate(moe.Raw, 2048)))
			}
			s.log.Warn("geometry spec extract failed", fields...)
			// 下一轮必须给出反馈，否则重试请求与上一轮完全相同，只会重复失败。
			correction = "上一轮输出不是合法的 JSON（可能是被截断、夹带了说明文字或含非法转义）。" +
				"请只输出一个完整的 JSON 对象：不要 Markdown 代码块、不要任何解释文字；" +
				"字符串里不要写 LaTeX（如 \\frac、\\angle），需要时用普通文字描述。"
			continue
		}
		parsed, perr := geom.ParsePanels(spec)
		if perr != nil {
			lastErr = perr
			infraRetries++
			s.log.Warn("geometry parse failed", zap.Int("attempt", attempts),
				zap.String("error", perr.Error()), zap.String("spec", truncate(spec, 512)))
			correction = fmt.Sprintf("上一轮输出的 JSON 无法解析（错误：%v）。请严格按字段说明重新输出完整 JSON，只输出一个 JSON 对象。", perr)
			continue
		}
		// 模型判定输入区域不是几何示意图（显式空 panels）：
		// 若当前用的是几何区域小图，可能是 bbox 偏移把图形切掉导致误判，
		// 回退整张原图重试（提取有题干在手，能从整图找到对应图形）；
		// 整张原图仍被拒绝才干净跳过重绘（本题确无几何图形）。
		if len(parsed) == 0 {
			infraRetries++ // 空 panels 属无效轮，不消耗修正预算
			// 入口按模型判读把整图转正过，这里却判"没有几何图形"：可能是入口方向判反了，
			// 撤销入口旋转（转回原始方向）再试一次。
			if rotationFix == 0 && result.ImageRotation != 0 && !entryUprightOppositeTried {
				entryUprightOppositeTried = true
				opp := (360 - result.ImageRotation) % 360
				if upright, rerr := rotateImageData(regionImage, opp); rerr == nil {
					extractImage = upright
					s.log.Warn("geometry spec empty, retry with entry rotation undone",
						zap.Uint64("task_id", taskID), zap.Int("attempt", attempts),
						zap.Int("entry_rotation", result.ImageRotation), zap.Int("rotation", opp))
					correction = "上一轮你输出了 {\"panels\":[]}（判定图片区域不是几何示意图），" +
						"但几何识别阶段已在该题目图片中检测到几何图形，可能是图片朝向被转错导致误判。" +
						"请基于这张图片（题干文字可能侧躺，请按文字实际可读方向理解图形），完整输出其几何描述 JSON。"
					continue
				}
			}
			// 再怀疑旋转方向判断错（VLM 的倾斜方向不稳）：按相反方向转一次再试，
			// 若仍为空再回退整张原图，避免把"图被转反了"误判成"区域里没有图形"。
			if rotationFix != 0 {
				opp := (360 - rotationFix) % 360
				if upright, rerr := rotateImageData(regionImage, opp); rerr == nil {
					extractImage = upright
					rotationFix = opp
					s.log.Warn("geometry spec empty after rotate, retry with opposite rotation",
						zap.Uint64("task_id", taskID), zap.Int("attempt", attempts), zap.Int("rotation", opp))
					correction = "上一轮你输出了 {\"panels\":[]}（判定图片区域不是几何示意图），" +
						"但几何识别阶段已在该题目图片中检测到几何图形，可能是图片朝向被转错导致误判。" +
						"请基于这张图片（题干文字可能侧躺，请按文字实际可读方向理解图形），完整输出其几何描述 JSON。"
					continue
				}
			}
			if regionFallback {
				regionFallback = false
				extractImage = imageData
				s.log.Warn("geometry spec empty on cropped region, retry with full image",
					zap.Uint64("task_id", taskID), zap.Int("attempt", attempts))
				correction = "上一轮你输出了 {\"panels\":[]}（判定图片区域不是几何示意图），" +
					"但几何识别阶段已在该题目图片中检测到几何图形，可能是区域裁剪不完整导致误判。" +
					"请基于这张完整图片，找到与题干对应的几何示意图，完整输出其几何描述 JSON；" +
					"仅当整张图片确实没有任何几何图形时才返回 {\"panels\":[]}。"
				continue
			}
			s.log.Info("geometry spec empty, skip redraw",
				zap.Uint64("task_id", taskID), zap.Int("attempt", attempts))
			return nil
		}
		// 直线名称标签只跟着原图走：题干里的 l₁/l₂ 若原图没印，就不要画到图上。
		// 印刷名称取识别与描述两个独立看图阶段的共识，单侧误报不能当依据。
		printedLineNames := geom.PrintedLineNamesConsensus(result.Geometry.LineLabels, parsed)
		geom.StripUnprintedLineNames(parsed, printedLineNames)
		// 语义核对之前先留一份"结构类"问题快照（点引用缺失、坐标非法、图元字段不全等）。
		// 结构问题会直接渲染出坏图，必须报错；而语义偏差（题干与图形不一致）常常源于
		// 题干转录不准（如漏掉"点 E 从 B 出发"半句、把"水平向右"录成"竖直向上"），
		// 只用于回喂模型修正，不应向用户报"结构校验未通过"。
		structCount := 0
		for i := range parsed {
			structCount += len(parsed[i].Issues)
		}
		// 题干-图形语义核对：半圆被画成整圆、半圆朝向不对、题干给定线段
		// 长度与图形矛盾等结构合法但语义错误的情况。
		geom.CheckSemicircle(parsed, result.StemText)
		geom.CheckSemicircleDirection(parsed, result.StemText)
		geom.CheckStemLengths(parsed, result.StemText)
		geom.CheckStemPassPoints(parsed, result.StemText)
		// 子图数量核对：几何识别的文字描述里常写明"三幅图（图1、图2、备用图）"，
		// 与该描述合并后核对，避免模型整张漏掉"备用图"。
		geom.CheckFigureCount(parsed, result.StemText+"\n"+result.Geometry.Description)
		// 点在边上核对：题干"点P在OA边上"要求 P 落在两端点之间，不得画到延长线上。
		geom.CheckPointOnEdge(parsed, result.StemText)
		// 三角形完整性核对：题干"以点D、E、M为顶点的三角形"要求三边都画出（轴上的边可由轴覆盖）。
		geom.CheckTriangleEdges(parsed, result.StemText)
		// 动点方向核对：题干"从A点出发水平向右运动"要求动点落在过 A 的水平线上。
		geom.CheckMotionConstraints(parsed, result.StemText)
		// 平行关系核对：题干"AG∥CE"要求两条线段确实平行（决定交点位置）。
		geom.CheckParallelRelations(parsed, result.StemText)
		// 网格覆盖核对：顶点跑出网格说明行列数/格边长估错（网格图常见）。
		geom.CheckGridCoverage(parsed)
		// 与坐标轴重合的线段核对：这类线段画出来会被轴线盖住（等于没画），
		// 模型常把没有字母点的直线（如 l₃）退化成一截沿轴的线段。
		geom.CheckAxisCoincidentSegments(parsed)
		// 标注点漂浮核对：点必须落在某条线/坐标轴上（顶点、交点都不会漂浮）。
		geom.CheckFloatingPoints(parsed)
		// 直线名称核对：题干点名 l₁/l₂/l₃ 时，图上每条都要画出来并标注名称。
		// 必须同时用几何描述（VLM 对图的判读）把关：题干后面小问才会引入的直线
		// （如"直线 l₃ 与 x 轴交于点 E"）原图并没有画，不能要求模型凭空补画。
		geom.CheckNamedLines(parsed, result.StemText, result.Geometry.Description, printedLineNames)

		panels = panels[:0]
		renderFailed := false
		for i := range parsed {
			svg, rerr := geom.Render(&parsed[i].Spec)
			if rerr != nil {
				lastErr = rerr
				infraRetries++ // 无法渲染同属无效轮
				s.log.Warn("geometry render failed", zap.Int("attempt", attempts),
					zap.String("error", rerr.Error()), zap.String("spec", truncate(spec, 512)))
				correction = fmt.Sprintf("上一轮输出的 JSON 无法渲染（错误：%v）。请严格按字段说明重新输出完整 JSON，只输出一个 JSON 对象。", rerr)
				renderFailed = true
				break
			}
			panels = append(panels, geom.PanelResult{
				Title: parsed[i].Title, SVG: svg, Issues: parsed[i].Issues,
			})
		}
		if renderFailed {
			continue
		}
		// 结构校验（点引用完整性、坐标有限性、图元字段完备性等）替代原残差判据。
		issues := geom.CollectIssues(panels)
		// 语义核对新增的问题条数（= 总问题数 - 结构类问题数）：题干可能有错，只用于修正。
		semanticCount := len(issues) - structCount
		// 记录当前最优版本：题干本身可能转录有误，按它反复"修正"只会让图越改越差，
		// 因此最终输出问题最少的一版；同分保留最早的一版（更接近模型对原图的原始判读）。
		if best == nil || structCount < bestHard ||
			(structCount == bestHard && semanticCount < bestSoft) {
			best, bestHard, bestSoft = snapshotPanels(panels), structCount, semanticCount
			bestIssues = issues
		}
		// 结构问题已清零即停止回喂（含"结构+语义均无"与"仅剩语义偏差"两种情况）。
		// 语义偏差（题干与图形不一致）多源于题干转录不准，反复让模型"修正"只会越改越差，
		// 且每轮都要等满一次 VLM 提取；结构完整即可输出，剩余语义偏差只记日志、不再重试。
		if structCount == 0 {
			break
		}
		s.log.Warn("geometry spec validation failed", zap.Int("attempt", attempts),
			zap.Int("structural", structCount), zap.Int("semantic", semanticCount),
			zap.Strings("issues", issues))
		correction = formatValidateIssues(issues)
	}

	if panels == nil {
		if lastErr != nil {
			return lastErr
		}
		return errors.New(errors.CodeUpstream, "geometry redraw produced no result")
	}
	if best != nil {
		panels = best
	}
	if bestSoft > 0 && bestHard == 0 {
		// 语义偏差只记日志：多半是题干转录与图形不一致，图本身可正常渲染。
		s.log.Info("geometry redraw kept best version with semantic deviations",
			zap.Uint64("task_id", taskID), zap.Int("deviations", bestSoft), zap.Strings("issues", bestIssues))
	}

	// 逐子图存储独立 SVG，并带上图号（如"图1"）。
	// 即使仍有少量结构问题也输出已渲染结果，同时提示告警。
	figures := make([]RedrawFigure, 0, len(panels))
	for i, panel := range panels {
		key := geometrySVGKey(taskID, i, len(panels))
		if err := s.storage.Put(ctx, key, panel.SVG); err != nil {
			return errors.Wrap(errors.CodeStorageFail, "store redraw svg failed", err)
		}
		figures = append(figures, RedrawFigure{Key: key, Label: strings.TrimSpace(panel.Title)})
	}
	result.RedrawFigures = figures

	// 一致性只看结构问题：语义偏差多由题干转录不准引起，不该对用户报"结构校验未通过"。
	consistent := bestHard == 0
	result.RedrawReport = &RedrawReport{
		Attempts: attempts, Consistent: consistent,
		StructIssues: bestHard, SemanticIssues: bestSoft,
	}
	if !consistent {
		if result.Warning != "" {
			result.Warning += "；"
		}
		result.Warning += "几何重绘描述存在结构问题（已输出可渲染部分），图形仅供参考"
	}
	s.log.Info("geometry redraw done", zap.Uint64("task_id", taskID),
		zap.Int("svg_count", len(figures)), zap.Int("attempts", attempts), zap.Bool("consistent", consistent))
	return nil
}

// snapshotPanels 深拷贝渲染结果，避免后续轮次往 Issues 里追加问题、
// 覆盖已保存的最优版本。
func snapshotPanels(src []geom.PanelResult) []geom.PanelResult {
	out := make([]geom.PanelResult, len(src))
	for i, p := range src {
		out[i] = p
		out[i].Issues = append([]string(nil), p.Issues...)
	}
	return out
}

// cleanupTaskArtifacts 清理失败任务的重绘产物（transient/geometry/task_<id>. 前缀）。
// 清理失败仅记录日志——产物残留不影响正确性，只占用存储。
func (s *Service) cleanupTaskArtifacts(taskID uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	prefix := fmt.Sprintf("transient/geometry/task_%d.", taskID)
	n, err := s.storage.DeleteByPrefix(ctx, prefix)
	if err != nil {
		s.log.Warn("cleanup failed-task artifacts", zap.Uint64("task_id", taskID),
			zap.String("prefix", prefix), zap.String("error", err.Error()))
		return
	}
	if n > 0 {
		s.log.Info("cleaned up failed-task artifacts",
			zap.Uint64("task_id", taskID), zap.Int("deleted", n))
	}
}

// truncate 截断字符串用于日志。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// formatValidateIssues 把几何描述的结构校验问题清单格式化为回喂 VLM 的修正提示。
func formatValidateIssues(issues []string) string {
	var sb strings.Builder
	sb.WriteString("上一轮输出的几何描述 JSON 存在以下结构问题：\n")
	for i, issue := range issues {
		if i >= 10 {
			sb.WriteString("-（其余问题省略）\n")
			break
		}
		sb.WriteString("- ")
		sb.WriteString(issue)
		sb.WriteString("\n")
	}
	sb.WriteString("常见原因：引用了 points 中不存在的点、线段起止点相同、多边形有效顶点不足 3 个、")
	sb.WriteString("圆/弧缺少有效半径、canvas 尺寸非法。请修正后重新输出完整 JSON。")
	return sb.String()
}

// processDocument 处理 word/pdf 文档：解析文本+图片，拆分多道题，图片走 OCR。
func (s *Service) processDocument(ctx context.Context, taskID uint64, storageKey string) (*RecognitionResult, error) {
	data, err := s.storage.Get(ctx, storageKey)
	if err != nil {
		return nil, err
	}

	filename := filenameFromKey(storageKey)

	// 方案 B：优先走结构化拆题（含子问/几何），失败时回退到旧启发式拆分。
	var structured *provider.StructuredResult
	var serr error
	s.callProvider(ctx, "document_structured", func(c context.Context) (err error) {
		structured, serr = s.prov.ExtractStructured(c, data, filename)
		return serr
	})
	if serr == nil {
		s.updateStatus(taskID, domain.TaskProcessing, 60, "")
		// 存图并映射 geometry_refs -> geometry_keys。
		imgKeys := s.storeDocumentImages(ctx, taskID, structured.Images)
		questions := fromStructured(structured, imgKeys)
		s.updateStatus(taskID, domain.TaskProcessing, 90, "")
		return &RecognitionResult{Questions: questions}, nil
	}

	// 回退：解析文档内容项，用正则启发式拆分。
	var doc *provider.DocumentResult
	var derr error
	s.callProvider(ctx, "document_parse", func(c context.Context) (err error) {
		doc, derr = s.prov.ExtractDocument(c, data, filename)
		return derr
	})
	if derr != nil {
		return nil, derr
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

// geometrySVGKey 生成几何重绘 SVG 的存储 key。
// 单张原图只含一个子图时用 task_<id>.svg；含多个子图时用 task_<id>_<i>.svg（i 从 1 开始）。
func geometrySVGKey(taskID uint64, index, total int) string {
	if total <= 1 {
		return "transient/geometry/task_" + itoa(taskID) + ".svg"
	}
	return "transient/geometry/task_" + itoa(taskID) + "_" + itoa(uint64(index+1)) + ".svg"
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
// fillClassify 学科/题型降级规则：模型整题解析未给出时按题干文本启发式判断。
// （合并进单次 ParseQuestion 后不再单独调 VLM 做分类。）
