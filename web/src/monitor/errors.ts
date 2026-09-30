// 错误与异常捕获：全局 JS 错误、Promise 未处理拒绝、静态资源加载失败。
//
// 覆盖三类线上高频问题：
// 1. window.onerror：运行时 JS 异常（含堆栈位置）
// 2. unhandledrejection：Promise 未处理的拒绝（fetch 失败、异步逻辑 bug）
// 3. 资源加载 error 捕获（捕获阶段）：图片/JS/CSS 404 或加载失败——
//    这类错误不冒泡、不会进入 window.onerror，必须用捕获阶段监听

import type { MonitorEvent } from './types'

/** 生成一条错误事件。 */
export function buildErrorEvent(
  name: string,
  path: string,
  traceId: string | undefined,
  data: Record<string, string | number | boolean | null>,
): MonitorEvent {
  return { type: 'error', name, data, ts: Date.now(), path, trace_id: traceId }
}

/** 解析 ErrorEvent 为事件数据（截断超长 message/stack）。 */
export function describeErrorEvent(e: ErrorEvent): Record<string, string | number | boolean | null> {
  return {
    message: truncate(e.message, 500),
    source: e.filename || '',
    line: e.lineno || 0,
    col: e.colno || 0,
    stack: truncate(e.error?.stack ?? '', 2000),
  }
}

/** 解析 PromiseRejectionEvent。 */
export function describeRejection(e: PromiseRejectionEvent): Record<string, string | number | boolean | null> {
  const reason = e.reason
  let message = String(reason ?? 'unknown')
  let stack = ''
  if (reason instanceof Error) {
    message = reason.message
    stack = reason.stack ?? ''
  }
  return { message: truncate(message, 500), stack: truncate(stack, 2000) }
}

/** 判断资源加载失败事件是否值得上报（排除脚本执行错误以外的噪音）。 */
export function describeResourceError(e: Event): Record<string, string | number | boolean | null> {
  const target = e.target as (HTMLElement & { src?: string; href?: string; tagName?: string }) | null
  return {
    tag: target?.tagName ?? 'unknown',
    url: truncate(target?.src || target?.href || '', 500),
  }
}

/** 判断是否资源加载失败（而非普通运行时错误冒泡）。 */
export function isResourceErrorTarget(e: Event | null | undefined): boolean {
  if (!e) return false
  const target = e.target as { tagName?: string } | null
  if (!target?.tagName) return false
  return ['IMG', 'SCRIPT', 'LINK', 'AUDIO', 'VIDEO', 'SOURCE'].includes(target.tagName)
}

function truncate(s: string, n: number): string {
  if (s.length <= n) return s
  return s.slice(0, n) + '...'
}

/** 挂载全局错误监听，返回解绑函数（测试与 HMR 用）。 */
export function attachErrorListeners(report: (event: MonitorEvent) => void, getPath: () => string, getTraceId: () => string | undefined): () => void {
  const onError = (e: ErrorEvent) => {
    report(buildErrorEvent('js_error', getPath(), getTraceId(), describeErrorEvent(e)))
  }
  const onRejection = (e: PromiseRejectionEvent) => {
    report(buildErrorEvent('unhandled_rejection', getPath(), getTraceId(), describeRejection(e)))
  }
  const onCaptureError = (e: Event) => {
    // target 为 window 时是普通运行时错误（已由 onError 处理），跳过。
    if (e.target === window || !isResourceErrorTarget(e)) return
    report(buildErrorEvent('resource_error', getPath(), getTraceId(), describeResourceError(e)))
  }
  window.addEventListener('error', onError)
  window.addEventListener('unhandledrejection', onRejection)
  // 资源错误不冒泡，必须捕获阶段。
  window.addEventListener('error', onCaptureError, true)
  return () => {
    window.removeEventListener('error', onError)
    window.removeEventListener('unhandledrejection', onRejection)
    window.removeEventListener('error', onCaptureError, true)
  }
}
