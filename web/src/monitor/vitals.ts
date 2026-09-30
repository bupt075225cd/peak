// 性能监控：Web Vitals（LCP / FID / CLS / TTFB）。
//
// 使用 Google 官方 web-vitals 库（~2KB，内含各指标的浏览器兼容 polyfill
// 与阈值判定），只在其 on* 回调里把结果转为统一上报事件。
// 指标对 SEO 与留存的影响：LCP 反映首屏主内容速度，CLS 反映布局稳定性，
// FID 反映首交互响应。

import { onCLS, onFID, onLCP, onTTFB, type Metric } from 'web-vitals'
import type { MonitorEvent } from './types'

/** 将 web-vitals 的 Metric 转为统一上报事件。 */
export function buildVitalEvent(metric: Metric, path: string, traceId?: string): MonitorEvent {
  return {
    type: 'webvital',
    name: metric.name,
    data: {
      value: Math.round(metric.value * 100) / 100,
      // good/needs-improvement/poor：Grafana 面板直接按评级聚合。
      rating: metric.rating,
      // 指标所属页面标识（SPA 下导航不刷新，仍归因到进入时的路径）。
      navigation: metric.navigationType ?? '',
    },
    ts: Date.now(),
    path,
    trace_id: traceId,
  }
}

/** 采集全部 Web Vitals 指标，每个指标首次/变化时回调。 */
export function collectVitals(report: (event: MonitorEvent) => void, getPath: () => string, getTraceId: () => string | undefined): void {
  const handler = (metric: Metric) => report(buildVitalEvent(metric, getPath(), getTraceId()))
  onLCP(handler)
  onFID(handler)
  onCLS(handler)
  onTTFB(handler)
}
