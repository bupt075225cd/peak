// Package handler 家庭资产账本 HTTP 处理器。
//
// 网关已完成 JWT 校验并把真实 user_id 注入 X-User-Id；本服务只信任该头，
// 再经 HMAC 映射为业务伪名 owner_ref，业务层与数据库都不接触原始用户 ID。
package handler

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"peak/apps/asset-service/internal/service"
	"peak/libs/errors"
	httpx "peak/libs/http"
)

// Handler 账本 HTTP 处理器。
type Handler struct {
	svc *service.Service
}

// New 创建处理器。
func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes 注册路由（网关不剥离前缀，路径与后端一致）。
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/ledger")
	{
		api.GET("/meta", h.meta)
		api.GET("/summary", h.summary)
		api.GET("/items", h.listItems)
		api.POST("/items", h.createItem)
		api.PUT("/items/:id", h.updateItem)
		api.DELETE("/items/:id", h.deleteItem)
		api.GET("/members", h.listMembers)
		api.POST("/members", h.createMember)
		api.PUT("/members/:id", h.renameMember)
		api.DELETE("/members/:id", h.deleteMember)
		api.GET("/snapshots", h.snapshots)
		api.GET("/privacy/consent", h.consentStatus)
		api.POST("/privacy/consent", h.acceptConsent)
		api.GET("/privacy/export", h.export)
		api.GET("/privacy/deletion", h.deletionStatus)
		api.POST("/privacy/deletion", h.requestDeletion)
		api.DELETE("/privacy/deletion", h.cancelDeletion)
		api.POST("/privacy/deletion/purge", h.purge)
	}
}

// ── 元数据与总览 ──────────────────────────────────────────

func (h *Handler) meta(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	meta, err := h.svc.Meta(c.Request.Context(), owner)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, meta)
}

func (h *Handler) summary(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	summary, err := h.svc.Summary(c.Request.Context(), owner)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, summary)
}

// ── 条目 ──────────────────────────────────────────────────

func (h *Handler) listItems(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	result, err := h.svc.ListItems(c.Request.Context(), owner, service.ItemFilter{
		Category: c.Query("category"),
		Keyword:  c.Query("keyword"),
		Offset:   intParam(c, "offset", 0),
		Limit:    intParam(c, "limit", 20),
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *Handler) createItem(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	var in service.ItemInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "请求参数格式错误"))
		return
	}
	item, err := h.svc.CreateItem(c.Request.Context(), owner, traceID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, item)
}

func (h *Handler) updateItem(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in service.UpdateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "请求参数格式错误"))
		return
	}
	item, err := h.svc.UpdateItem(c.Request.Context(), owner, traceID(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, item)
}

func (h *Handler) deleteItem(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteItem(c.Request.Context(), owner, traceID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}

// ── 成员 ──────────────────────────────────────────────────

func (h *Handler) listMembers(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	members, err := h.svc.ListMembers(c.Request.Context(), owner)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": members})
}

func (h *Handler) createMember(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "请求参数格式错误"))
		return
	}
	member, err := h.svc.CreateMember(c.Request.Context(), owner, traceID(c), body.Name)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, member)
}

func (h *Handler) renameMember(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "请求参数格式错误"))
		return
	}
	member, err := h.svc.RenameMember(c.Request.Context(), owner, traceID(c), id, body.Name)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, member)
}

func (h *Handler) deleteMember(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteMember(c.Request.Context(), owner, traceID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}

// ── 快照 ──────────────────────────────────────────────────

func (h *Handler) snapshots(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	snaps, err := h.svc.Snapshots(c.Request.Context(), owner, c.Query("year"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": snaps})
}

// ── 隐私：同意 / 导出 / 注销 ──────────────────────────────

func (h *Handler) consentStatus(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	view, err := h.svc.ConsentStatus(c.Request.Context(), owner)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *Handler) acceptConsent(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	var body struct {
		PolicyVersion string `json:"policy_version"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, errors.New(errors.CodeInvalidArgument, "请求参数格式错误"))
		return
	}
	view, err := h.svc.AcceptConsent(c.Request.Context(), owner, body.PolicyVersion, c.ClientIP(), traceID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// export 全量导出：含明文资产数据，要求新鲜令牌（重认证）。
func (h *Handler) export(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	if err := h.requireFresh(c); err != nil {
		httpx.Fail(c, err)
		return
	}
	payload, filename, err := h.svc.Export(c.Request.Context(), owner, traceID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename))
	c.Data(200, "application/json; charset=utf-8", payload)
}

func (h *Handler) deletionStatus(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	view, err := h.svc.DeletionStatus(c.Request.Context(), owner)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *Handler) requestDeletion(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	if err := h.requireFresh(c); err != nil {
		httpx.Fail(c, err)
		return
	}
	view, err := h.svc.RequestDeletion(c.Request.Context(), owner, traceID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *Handler) cancelDeletion(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	view, err := h.svc.CancelDeletion(c.Request.Context(), owner, traceID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// purge 立即执行密码学删除：不可恢复，要求新鲜令牌。
func (h *Handler) purge(c *gin.Context) {
	owner, ok := h.owner(c)
	if !ok {
		return
	}
	if err := h.requireFresh(c); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.PurgeNow(c.Request.Context(), owner, traceID(c)); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"purged": true})
}

// ── 辅助 ──────────────────────────────────────────────────

// owner 解析网关注入的用户身份并映射为伪名。
func (h *Handler) owner(c *gin.Context) (string, bool) {
	uid, err := strconv.ParseUint(c.GetHeader("X-User-Id"), 10, 64)
	if err != nil || uid == 0 {
		httpx.Fail(c, errors.New(errors.CodeUnauthorized, "未登录或身份信息缺失"))
		return "", false
	}
	return h.svc.OwnerRef(uid), true
}

// requireFresh 敏感操作要求令牌在重认证窗口内签发（网关写入 X-Token-Iat）。
// 客户端自传的该头会被网关覆盖，因此此处可信。
func (h *Handler) requireFresh(c *gin.Context) error {
	raw := c.GetHeader("X-Token-Iat")
	if raw == "" {
		return errors.New(errors.CodeReauthRequired, "该操作涉及敏感数据，请重新验证身份后再试")
	}
	iat, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return errors.New(errors.CodeReauthRequired, "该操作涉及敏感数据，请重新验证身份后再试")
	}
	if time.Since(time.Unix(iat, 0)) > h.svc.ReauthWindow() {
		return errors.New(errors.CodeReauthRequired, "登录状态已超时，请重新验证身份后再试")
	}
	return nil
}

func traceID(c *gin.Context) string {
	return c.GetString("trace_id")
}

func parseID(c *gin.Context) (uint64, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, errors.New(errors.CodeInvalidArgument, "记录 ID 不合法")
	}
	return id, nil
}

func intParam(c *gin.Context, key string, def int) int {
	raw := c.Query(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return def
	}
	return n
}
