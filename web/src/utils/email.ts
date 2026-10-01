// 邮箱工具函数。

// maskEmail 邮箱脱敏：保留首字符与域名，如 "student@peak.local" → "s***@peak.local"。
// 用于"验证码已发送至 xxx"提示，确认去向而不暴露完整地址。
export function maskEmail(email: string): string {
  const at = email.indexOf('@')
  if (at <= 0) return email
  return email[0] + '***' + email.slice(at)
}
