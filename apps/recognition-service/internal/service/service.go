// Package service 实现识别任务的业务编排与异步处理状态机。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"peak/libs/domain"
	"peak/libs/errors"
	"peak/libs/logger"
	"peak/libs/storage"

	"peak/apps/recognition-service/internal/geom"
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
	Geometry       provider.GeometryResult `json:"geometry"`
	// RedrawFigures 几何重绘输出的子图列表：key 为独立 SVG 的存储 key，
	// label 为该子图在题干中的图号（如"图1"）。一张原图含多个几何子图时逐个填充
	// （配置了 geometry sidecar 才填充）。
	RedrawFigures []RedrawFigure `json:"redraw_figures,omitempty"`
	// RedrawReport 几何重绘求解报告（残差、重试次数、是否几何自洽）。
	RedrawReport *RedrawReport `json:"redraw_report,omitempty"`
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

// New 创建识别服务实例。
func New(db *gorm.DB, store storage.FileStorage, prov provider.Provider, log *logger.Logger, opts ...Option) *Service {
	s := &Service{db: db, storage: store, prov: prov, log: log, geometryMaxAttempts: 3}
	for _, opt := range opts {
		opt(s)
	}
	return s
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
	task.ProgressText = ""
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
		"status":        domain.TaskSuccess,
		"progress":      100,
		"progress_text": "",
		"result_json":   string(resultJSON),
	})
	s.log.Info("recognition task done",
		zap.Uint64("task_id", taskID),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	)
}

// processImage 处理图片：整题解析（题干+学科+题型一次 VLM）∥ 几何识别（并发）→ 几何重绘。
// 优化点：
//   - 合并原 OCR + 学科/题型分类两次调用为一次 ParseQuestion；
//   - 几何识别与整题解析并行发起，重叠耗时；
//   - 每个阶段实时更新 progress_text 并记录耗时日志（便于定位慢点）。
// 几何重绘直接用原始图片（不再裁剪子图）：数学题且图中含几何图形时，
// 把题干文本交给 VLM 坐标直出各子图的几何描述，内置渲染器每个子图渲染一张独立 SVG。
func (s *Service) processImage(ctx context.Context, taskID uint64, storageKey string) (*RecognitionResult, error) {
	imageData, err := s.storage.Get(ctx, storageKey)
	if err != nil {
		return nil, err
	}

	result := &RecognitionResult{}
	s.updateProgress(taskID, 10, "正在识别题干…")

	// 几何识别与整题解析相互独立 → 并行发起，二者耗时重叠。
	type geomOut struct {
		res *provider.GeometryResult
		err error
	}
	geoCh := make(chan geomOut, 1)
	go func() {
		gstart := time.Now()
		g, gerr := s.prov.RecognizeGeometry(ctx, imageData)
		s.logStage("geometry-recognize", gstart)
		geoCh <- geomOut{res: g, err: gerr}
	}()

	// 整题解析：题干文本 + 学科 + 题型，一次 VLM 调用（关键步骤，失败则任务失败）。
	parseStart := time.Now()
	parse, perr := s.prov.ParseQuestion(ctx, imageData)
	if perr != nil {
		s.log.Error("parse question failed", zap.String("error", perr.Error()))
		return nil, perr
	}
	s.logStage("parse-question", parseStart)

	result.StemText = parse.Text
	result.Subject = parse.Subject
	result.QuestionType = parse.QuestionType
	// 模型未给出学科/题型时降级为规则判断（不再额外调 VLM）。
	if result.Subject == "" {
		result.Subject = detectSubject(result.StemText)
	}
	if result.QuestionType == "" {
		result.QuestionType = detectQuestionType(result.StemText)
	}
	s.updateProgress(taskID, 40, "正在识别几何图形…")

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
		geomBBox = gout.res.BoundingBox
		hasGeometry = geomBBox != nil
	}
	s.updateProgress(taskID, 60, "识别完成")

	// 几何重绘（增强步骤，失败仅记录 warning）：
	// 数学题 + 图中含几何图形时，对原始图片按 bbox 裁出几何区域并下采样成小图，
	// 作为 VLM 几何描述提取的输入（不落库），以大幅减少 vision token、缩短耗时。
	if s.geometryRender && result.Subject == "数学" && hasGeometry {
		redrawStart := time.Now()
		if rerr := s.redrawGeometry(ctx, taskID, imageData, geomBBox, result); rerr != nil {
			s.log.Error("geometry redraw failed", zap.String("error", rerr.Error()))
			if result.Warning != "" {
				result.Warning += "；"
			}
			result.Warning += "几何重绘失败：" + rerr.Error()
		} else {
			s.logStage("geometry-redraw", redrawStart)
		}
	}

	return result, nil
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
	extractImage := imageData
	if bbox != nil {
		if small, cerr := cropAndScaleRegion(imageData, bbox, geometryRegionMaxDim); cerr == nil {
			extractImage = small
			s.log.Info("geometry extract input shrunk",
				zap.Uint64("task_id", taskID),
				zap.Int("src_bytes", len(imageData)), zap.Int("region_bytes", len(small)))
		} else {
			s.log.Warn("geometry region prep failed, fallback to full image",
				zap.Uint64("task_id", taskID), zap.String("error", cerr.Error()))
		}
	}

	var panels []geom.PanelResult
	var lastErr error
	correction := ""
	attempts := 0
	for attempt := 1; attempt <= s.geometryMaxAttempts; attempt++ {
		attempts = attempt
		// 重绘轮次可能较久（VLM 提取几何描述），实时更新进度文案让用户感知。
		p := 80 + (attempt-1)*8
		if p > 94 {
			p = 94
		}
		s.updateProgress(taskID, p, fmt.Sprintf("正在重绘几何图形（第 %d 次尝试）…", attempt))
		spec, eerr := extractor.ExtractGeometrySpec(ctx, extractImage, result.StemText, correction)
		if eerr != nil {
			lastErr = eerr
			s.log.Warn("geometry spec extract failed", zap.Int("attempt", attempt), zap.String("error", eerr.Error()))
			continue
		}
		panels, lastErr = geom.RenderPanels(spec)
		if lastErr != nil {
			s.log.Warn("geometry render failed", zap.Int("attempt", attempt),
				zap.String("error", lastErr.Error()), zap.String("spec", truncate(spec, 512)))
			correction = fmt.Sprintf("上一轮输出的 JSON 无法解析或渲染（错误：%v）。请严格按字段说明重新输出完整 JSON，只输出一个 JSON 对象。", lastErr)
			continue
		}
		// 结构校验（点引用完整性、坐标有限性、图元字段完备性等）替代原残差判据。
		issues := geom.CollectIssues(panels)
		if len(issues) == 0 {
			break // 结构完整，提前结束。
		}
		s.log.Warn("geometry spec validation failed", zap.Int("attempt", attempt), zap.Strings("issues", issues))
		correction = formatValidateIssues(issues)
	}

	if panels == nil {
		if lastErr != nil {
			return lastErr
		}
		return errors.New(errors.CodeUpstream, "geometry redraw produced no result")
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

	consistent := geom.AllConsistent(panels)
	result.RedrawReport = &RedrawReport{Attempts: attempts, Consistent: consistent}
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

// geometrySVGKey 生成几何重绘 SVG 的存储 key。
// 单张原图只含一个子图时用 task_<id>.svg；含多个子图时用 task_<id>_<i>.svg（i 从 1 开始）。
func geometrySVGKey(taskID uint64, index, total int) string {
	if total <= 1 {
		return "geometry/task_" + itoa(taskID) + ".svg"
	}
	return "geometry/task_" + itoa(taskID) + "_" + itoa(uint64(index+1)) + ".svg"
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

