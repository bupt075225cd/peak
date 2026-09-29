// Package handler 认证 HTTP 接口：发码、验证码登录与用户信息。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"peak/apps/user-service/internal/service"
	bizerr "peak/libs/errors"
	httpx "peak/libs/http"
)

// Handler 认证接口。
type Handler struct {
	svc *service.Service
}

// New 创建认证接口。
func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes 注册路由（均位于 /api/users 前缀下，经网关转发）。
// 发码与登录位于 /auth/* 白名单路径，网关不要求 JWT。
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	authGroup := rg.Group("/api/users/auth")
	{
		authGroup.POST("/sms/code", h.sendCode)
		authGroup.POST("/sms/login", h.smsLogin)
	}
	rg.GET("/api/users/me", h.me)
	rg.PUT("/api/users/me", h.updateMe)
}

// sendCodeRequest 发码请求。
type sendCodeRequest struct {
	Phone string `json:"phone" binding:"required"`
}

// bindJSON 绑定请求体，失败时返回参数错误（避免校验器错误被归为 500）。
func bindJSON(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		httpx.Fail(c, bizerr.New(bizerr.CodeInvalidArgument, "请求参数不完整或格式错误"))
		return false
	}
	return true
}

// sendCode POST /api/users/auth/sms/code
func (h *Handler) sendCode(c *gin.Context) {
	var req sendCodeRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.SendCode(c.Request.Context(), req.Phone, clientIP(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"ticket":     res.Ticket,
		"debug_code": res.DebugCode, // 仅开发模式非空，生产为空
	})
}

// smsLoginRequest 登录请求。
type smsLoginRequest struct {
	Phone  string `json:"phone" binding:"required"`
	Code   string `json:"code" binding:"required"`
	Ticket string `json:"ticket" binding:"required"`
}

// smsLogin POST /api/users/auth/sms/login
func (h *Handler) smsLogin(c *gin.Context) {
	var req smsLoginRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.SmsLogin(c.Request.Context(), req.Phone, req.Code, req.Ticket, clientIP(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"token": res.Token, "user": res.User})
}

// me GET /api/users/me（身份由网关校验 JWT 后注入 X-User-Id）。
func (h *Handler) me(c *gin.Context) {
	uid, err := strconv.ParseUint(c.GetHeader("X-User-Id"), 10, 64)
	if err != nil || uid == 0 {
		httpx.Fail(c, bizerr.New(bizerr.CodeUnauthorized, "missing or invalid user identity"))
		return
	}
	u, err := h.svc.Me(c.Request.Context(), uid)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, u)
}

// updateMeRequest 修改昵称请求。
type updateMeRequest struct {
	Name string `json:"name" binding:"required"`
}

// updateMe PUT /api/users/me（当前仅支持修改昵称；身份由网关注入 X-User-Id）。
func (h *Handler) updateMe(c *gin.Context) {
	uid, err := strconv.ParseUint(c.GetHeader("X-User-Id"), 10, 64)
	if err != nil || uid == 0 {
		httpx.Fail(c, bizerr.New(bizerr.CodeUnauthorized, "missing or invalid user identity"))
		return
	}
	var req updateMeRequest
	if !bindJSON(c, &req) {
		return
	}
	u, err := h.svc.UpdateName(c.Request.Context(), uid, req.Name)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, u)
}

// clientIP 取客户端 IP：优先网关注入的 X-Real-IP（唯一可信入口），
// 缺省回退 gin ClientIP。
func clientIP(c *gin.Context) string {
	if ip := c.GetHeader("X-Real-IP"); ip != "" {
		return ip
	}
	return c.ClientIP()
}
