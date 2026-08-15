// Package handler 处理 HTTP 请求，负责参数校验、绑定与响应。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"peak/libs/domain"
	"peak/libs/errors"
	httpx "peak/libs/http"

	"peak/apps/question-service/internal/service"
)

// Handler HTTP 处理器。
type Handler struct {
	svc *service.Service
}

// New 创建处理器实例。
func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
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
		mistake.POST("", h.createMistake)
		mistake.GET("/:id", h.getMistake)
		mistake.GET("", h.listMistakes)
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
	list, total, err := h.svc.ListMistakes(c.Request.Context(), userID, offset, limit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": list, "total": total})
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
