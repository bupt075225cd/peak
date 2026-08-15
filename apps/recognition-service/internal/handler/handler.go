// Package handler 处理识别服务的 HTTP 请求。
package handler

import (
	"io"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"peak/libs/domain"
	"peak/libs/errors"
	httpx "peak/libs/http"
	"peak/libs/storage"

	"peak/apps/recognition-service/internal/service"
)

// Handler 识别服务 HTTP 处理器。
type Handler struct {
	svc     *service.Service
	db      *gorm.DB
	storage storage.FileStorage
}

// New 创建处理器。
func New(svc *service.Service, db *gorm.DB, store storage.FileStorage) *Handler {
	return &Handler{svc: svc, db: db, storage: store}
}

// RegisterRoutes 注册路由。
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/recognition")
	{
		api.POST("/tasks", h.createTask)
		api.GET("/tasks/:id", h.getTask)
		api.POST("/tasks/:id/retry", h.retryTask)
	}
}

// createTask 接收图片上传，保存文件并创建识别任务。
func (h *Handler) createTask(c *gin.Context) {
	file, err := c.FormFile("image")
	if err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "image is required"))
		return
	}
	src, err := file.Open()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer src.Close()
	data, err := io.ReadAll(src)
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	// 保存原始图片到存储。
	key := "original/" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + file.Filename
	if err := h.storage.Put(c.Request.Context(), key, data); err != nil {
		httpx.Fail(c, errors.Wrap(errors.CodeStorageFail, "store image failed", err))
		return
	}

	// 记录图片元信息。
	img := &domain.Image{
		StorageKey: key,
		ImageType:  domain.ImageTypeOriginal,
	}
	if err := h.db.Create(img).Error; err != nil {
		httpx.Fail(c, err)
		return
	}

	// 创建识别任务。
	task, err := h.svc.CreateTask(c.Request.Context(), img.ID, key)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, task)
}

// getTask 查询任务状态。
func (h *Handler) getTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "invalid id"))
		return
	}
	task, err := h.svc.GetTask(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, task)
}

// retryTask 重试失败任务。
func (h *Handler) retryTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "invalid id"))
		return
	}
	if err := h.svc.RetryTask(c.Request.Context(), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}
