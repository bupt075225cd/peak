// W3C Trace Context（traceparent）生成工具：
// 前端为每个 API 请求生成 traceparent，经网关透传后与后端 OTel 链路
// 共用同一 Trace ID，实现"前端慢 → 后端哪段慢"全链路下钻。

const HEX = '0123456789abcdef'

/** 生成指定长度的随机 hex 串（crypto 安全熵，失败时退化为 Math.random）。 */
export function randomHex(len: number): string {
  const bytes = new Uint8Array(Math.ceil(len / 2))
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    crypto.getRandomValues(bytes)
  } else {
    for (let i = 0; i < bytes.length; i++) bytes[i] = Math.floor(Math.random() * 256)
  }
  let out = ''
  for (let i = 0; i < bytes.length; i++) {
    out += HEX[bytes[i] >> 4] + HEX[bytes[i] & 0x0f]
  }
  return out.slice(0, len)
}

/** 生成合法的 W3C traceparent（version 00、flags 01 采样）。 */
export function newTraceparent(): string {
  return `00-${randomHex(32)}-${randomHex(16)}-01`
}

/** 从 traceparent 提取 Trace ID（用于事件与后端日志/链路对齐）。 */
export function traceIdFromTraceparent(tp: string): string {
  const parts = tp.split('-')
  return parts.length === 4 && parts[0] === '00' ? parts[1] : ''
}
