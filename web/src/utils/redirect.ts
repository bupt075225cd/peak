/**
 * 解析登录/注册后的回跳目标。
 *
 * 仅接受站内绝对路径（以单个 `/` 开头），拒绝 `//evil.com` 这类协议相对地址，
 * 避免开放重定向；非法或缺失时回落到默认页。
 */
export function safeRedirect(value: unknown, fallback = '/home'): string {
  if (typeof value === 'string' && value.startsWith('/') && !value.startsWith('//')) return value
  return fallback
}
