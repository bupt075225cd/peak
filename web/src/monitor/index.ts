// 前端监控 SDK 入口：一行接入全部采集能力。
//
//   import { initMonitor } from './monitor'
//   initMonitor({ router })
//
// 采集能力：
//   - 性能：Web Vitals（LCP/FID/CLS/TTFB）
//   - 错误：全局 JS 错误、Promise 未处理拒绝、静态资源加载失败
//   - 行为：PV/UV、路由切换耗时、页面停留时长
//   - 上报：批量 + 采样 + sendBeacon/keepalive，页面关闭时兜底 flush
//
// 约束：任何采集/上报异常都不得影响业务（内部全部静默吞错）。

import type { Router } from 'vue-router'
import { attachErrorListeners } from './errors'
import { attachRouterTracking } from './behavior'
import { createReporter } from './report'
import { randomHex, traceIdFromTraceparent, newTraceparent } from './trace'
import { collectVitals } from './vitals'
import type { MonitorEvent, MonitorOptions } from './types'

export { newTraceparent, traceIdFromTraceparent, randomHex } from './trace'
export type { MonitorEvent, MonitorOptions } from './types'

/** 页面级 Trace ID：进入应用时生成一次，SPA 期间保持不变，
 *  用于把前端事件与该页面的 API 请求链路关联（Grafana 中按 trace_id 互查）。 */
let pageTraceparent = ''

export function getPageTraceId(): string | undefined {
  return traceIdFromTraceparent(pageTraceparent) || undefined
}

/** 为一次 API 请求生成 traceparent（axios 请求拦截器调用）。 */
export function requestTraceparent(): string {
  return newTraceparent()
}

/** 初始化监控。重复调用安全（以第一次为准）。 */
export function initMonitor(options: MonitorOptions & { router: Router }): void {
  if (initialized) return
  initialized = true

  pageTraceparent = newTraceparent()

  const reporter = createReporter(options)
  const opts = {
    errorSampleRate: options.errorSampleRate ?? 1,
    behaviorSampleRate: options.behaviorSampleRate ?? 0.1,
    maxBatchSize: options.maxBatchSize ?? 20,
  }

  const report = (event: MonitorEvent) => {
    try {
      reporter.enqueue(event, opts)
    } catch {
      /* ignore */
    }
  }

  const getPath = () => options.router.currentRoute.value.path || location.pathname

  try {
    attachErrorListeners(report, getPath, getPageTraceId)
  } catch { /* ignore */ }
  try {
    attachRouterTracking(options.router, report, getPageTraceId)
  } catch { /* ignore */ }
  try {
    collectVitals(report, getPath, getPageTraceId)
  } catch { /* ignore */ }

  // SPA 页面卸载兜底：submit 由 reporter 内部的 pagehide 处理。
}

let initialized = false
