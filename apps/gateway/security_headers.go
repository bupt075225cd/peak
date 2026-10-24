package main

import "github.com/gin-gonic/gin"

// securityHeaders 为所有响应补充基础安全头。
//
// 账本页面承载家庭资产等敏感数据，需限制外部资源加载与 Referer 外泄：
//   - Referrer-Policy: 不向第三方泄露来源 URL（含路径）；
//   - X-Content-Type-Options: 禁止浏览器猜测类型（配合下载类接口）；
//   - X-Frame-Options: 禁止被第三方页面嵌套（防点击劫持）；
//   - Content-Security-Policy: 仅允许同源资源（前端为同域 SPA，无需外部 CDN）。
//
// CSP 可通过 security.csp 覆盖；置空表示不下发该头（便于按环境放开）。
func securityHeaders(csp string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		if csp != "" {
			c.Header("Content-Security-Policy", csp)
		}
		c.Next()
	}
}
