// Package handler 处理 HTTP 请求，负责参数校验、绑定与响应。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"peak/libs/domain"
	"peak/libs/errors"
	httpx "peak/libs/http"
	"peak/libs/storage"

	"peak/apps/question-service/internal/service"
)

// transientPrefix 识别产物临时区前缀；提交时拷贝到 committed/ 正式区。
const transientPrefix = "transient/"

// committedPrefix 正式区前缀：错题引用的图片全部位于该前缀下。
const committedPrefix = "committed/"

// Handler HTTP 处理器。
type Handler struct {
	svc    *service.Service
	store  storage.FileStorage
	copier *storage.Copier
}

// New 创建处理器实例。copier 负责提交时的 transient/ -> committed/ 跨桶拷贝。
func New(svc *service.Service, store storage.FileStorage, copier *storage.Copier) *Handler {
	return &Handler{svc: svc, store: store, copier: copier}
}

// RegisterRoutes 注册路由。
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/questions")
	{
		api.POST("", h.createQuestion)
		api.GET("/:id", h.getQuestion)
		api.GET("", h.listQuestions)
		api.PUT("/:id", h.updateQuestion)
		api.DELETE("/:id", h.deleteQuestion)
	}

	mistake := r.Group("/api/mistakes")
	{
		mistake.GET("/files/*key", h.getMistakeFile)
		mistake.POST("", h.createMistake)
		mistake.GET("/:id", h.getMistake)
		mistake.GET("", h.listMistakes)
		mistake.POST("/export", h.exportMistakes)
		mistake.PUT("/:id", h.updateMistake)
		mistake.DELETE("/:id", h.deleteMistake)
	}

	category := r.Group("/api/categories")
	{
		category.GET("", h.listCategories)
		category.POST("", h.createCategory)
	}
}

func parseID(c *gin.Context) (uint64, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return 0, errors.New(errors.CodeInvalidArgument, "invalid id")
	}
	return id, nil
}

// ---- Question handlers ----

func (h *Handler) createQuestion(c *gin.Context) {
	var q domain.Question
	if err := c.ShouldBindJSON(&q); err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, err.Error()))
		return
	}
	if q.Image == "" {
		// MySQL 的 JSON 列不接受空字符串；无图题目存空数组。
		q.Image = "[]"
	}
	// 提交错题 = 把识别产物从临时区晋升为正式区：拷贝 transient/<key> ->
	// committed/<key>，并把 questions.image 中的引用改写为正式 key。
	// 拷贝由 Copier 完成：S3 后端走服务端 CopyObject（跨桶），本地走目录间复制。
	if q.Image != "" {
		committed, err := h.promoteImageRefs(c.Request.Context(), q.Image)
		if err != nil {
			httpx.Fail(c, errors.Wrap(errors.CodeStorageFail, "promote images failed", err))
			return
		}
		q.Image = committed
	}
	if err := h.svc.CreateQuestion(c.Request.Context(), &q); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, q)
}

func (h *Handler) getQuestion(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	q, err := h.svc.GetQuestion(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, q)
}

func (h *Handler) listQuestions(c *gin.Context) {
	offset, limit := pageParams(c)
	list, total, err := h.svc.ListQuestions(c.Request.Context(), offset, limit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": list, "total": total})
}

func (h *Handler) updateQuestion(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var q domain.Question
	if err := c.ShouldBindJSON(&q); err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, err.Error()))
		return
	}
	q.ID = id
	if err := h.svc.UpdateQuestion(c.Request.Context(), &q); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, q)
}

func (h *Handler) deleteQuestion(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteQuestion(c.Request.Context(), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

// ---- Mistake handlers ----

func (h *Handler) createMistake(c *gin.Context) {
	var m domain.Mistake
	if err := c.ShouldBindJSON(&m); err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, err.Error()))
		return
	}
	if err := h.svc.CreateMistake(c.Request.Context(), &m); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, m)
}

func (h *Handler) getMistake(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	m, err := h.svc.GetMistake(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, m)
}

func (h *Handler) listMistakes(c *gin.Context) {
	userID, _ := strconv.ParseUint(c.GetHeader("X-User-Id"), 10, 64)
	offset, limit := pageParams(c)

	result, err := h.svc.ListMistakes(c.Request.Context(), domain.MistakeQuery{
		UserID:  userID,
		Keyword: keywordParam(c),
		Subject: strings.TrimSpace(c.Query("subject")),
		Source:  strings.TrimSpace(c.Query("source")),
		Offset:  offset,
		Limit:   limit,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	httpx.OK(c, gin.H{
		"items":          result.Items,
		"total":          result.Total,
		"subject_counts": result.SubjectCounts,
		"source_counts":  result.SourceCounts,
	})
}

func (h *Handler) updateMistake(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var m domain.Mistake
	if err := c.ShouldBindJSON(&m); err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, err.Error()))
		return
	}
	m.ID = id
	if err := h.svc.UpdateMistake(c.Request.Context(), &m); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, m)
}

func (h *Handler) deleteMistake(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteMistake(c.Request.Context(), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

// ---- Category handlers ----

func (h *Handler) listCategories(c *gin.Context) {
	typ := c.Query("type")
	list, err := h.svc.ListCategories(c.Request.Context(), typ)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, list)
}

func (h *Handler) createCategory(c *gin.Context) {
	var cat domain.Category
	if err := c.ShouldBindJSON(&cat); err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, err.Error()))
		return
	}
	if err := h.svc.CreateCategory(c.Request.Context(), &cat); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, cat)
}

func pageParams(c *gin.Context) (int, int) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return offset, limit
}

// maxKeywordRunes 关键词最大长度，避免异常长串进入 LIKE 查询。
const maxKeywordRunes = 100

// keywordParam 读取搜索关键词：去首尾空白并限制长度。
func keywordParam(c *gin.Context) string {
	kw := strings.TrimSpace(c.Query("keyword"))
	if runes := []rune(kw); len(runes) > maxKeywordRunes {
		kw = string(runes[:maxKeywordRunes])
	}
	return kw
}

// getMistakeFile 提供正式区（committed/）错题配图访问。
// key 不带 committed/ 前缀；只允许访问正式区，防止任意对象读取。
func (h *Handler) getMistakeFile(c *gin.Context) {
	key := strings.TrimPrefix(c.Param("key"), "/")
	if key == "" || strings.Contains(key, "..") {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "invalid file key"))
		return
	}
	data, err := h.store.Get(c.Request.Context(), committedPrefix+key)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Data(200, mimeByExt(strings.ToLower(filepath.Ext(key))), data)
}

// promoteImageRefs 把 image JSON 中的 transient/ 引用拷贝到 committed/ 并改写；
// 非 transient 引用（存量数据原样 key）保持不变。
func (h *Handler) promoteImageRefs(ctx context.Context, imageJSON string) (string, error) {
	var refs []struct {
		Key   string `json:"key"`
		Label string `json:"label,omitempty"`
	}
	if err := json.Unmarshal([]byte(imageJSON), &refs); err != nil {
		return "", fmt.Errorf("parse image refs: %w", err)
	}
	changed := false
	for i, r := range refs {
		if !strings.HasPrefix(r.Key, transientPrefix) {
			continue
		}
		dst := committedPrefix + strings.TrimPrefix(r.Key, transientPrefix)
		if err := h.copier.Copy(ctx, r.Key, dst); err != nil {
			return "", fmt.Errorf("copy %q -> %q: %w", r.Key, dst, err)
		}
		refs[i].Key = dst
		changed = true
	}
	if !changed {
		return imageJSON, nil
	}
	out, err := json.Marshal(refs)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// mimeByExt 按扩展名返回内容类型（识别产物只有 SVG/PNG/JPEG）。
func mimeByExt(ext string) string {
	switch ext {
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".pdf":
		return "application/pdf"
	default:
		return "image/jpeg"
	}
}

// time 未直接使用的占位保持导入整洁（当前无额外用途可移除）。
var _ = time.Second
