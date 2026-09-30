// 行为与运营埋点：PV/UV、路由切换耗时、页面停留时长。
//
// - PV：每次路由切换计一次（name=pv，data 含来源页）
// - UV：会话级去重（sessionStorage 标记，首次进入计一次）
// - 路由耗时：router.beforeEach 记起点、afterEach 结算（懒加载组件解析时间）
// - 停留时长：路由切换时上报上一页面停留时长；整页关闭时由 pagehide 兜底

import type { Router } from 'vue-router'
import type { MonitorEvent } from './types'

const UV_KEY = 'peak_monitor_uv'

/** 会话内是否已计 UV（sessionStorage 级去重，关标签页后重新计数）。 */
export function isNewSession(storage: Storage | undefined): boolean {
  if (!storage) return true
  try {
    if (storage.getItem(UV_KEY)) return false
    storage.setItem(UV_KEY, '1')
    return true
  } catch {
    return true
  }
}

/** 构造 PV 事件（UV 复用同结构，name 区分）。 */
export function buildPageEvent(name: 'pv' | 'uv', to: string, from: string, traceId?: string): MonitorEvent {
  return {
    type: 'pv',
    name,
    data: { from },
    ts: Date.now(),
    path: to,
    trace_id: traceId,
  }
}

/** 计算停留时长事件（秒，保留 1 位小数）。 */
export function buildDwellEvent(path: string, seconds: number, traceId?: string): MonitorEvent {
  return {
    type: 'behavior',
    name: 'page_dwell',
    data: { seconds: Math.max(0, Math.round(seconds * 10) / 10) },
    ts: Date.now(),
    path,
    trace_id: traceId,
  }
}

/** 构造路由切换耗时事件（毫秒）。 */
export function buildRouteTimingEvent(to: string, ms: number, traceId?: string): MonitorEvent {
  return {
    type: 'behavior',
    name: 'route_timing',
    data: { ms },
    ts: Date.now(),
    path: to,
    trace_id: traceId,
  }
}

/** 挂载路由埋点：PV/UV、路由耗时、页面停留时长，返回解绑函数。 */
export function attachRouterTracking(
  router: Router,
  report: (event: MonitorEvent) => void,
  getTraceId: () => string | undefined,
  storage: Storage | undefined = typeof sessionStorage !== 'undefined' ? sessionStorage : undefined,
): () => void {
  let enteredAt = Date.now()
  let firstEnter = true

  const removeAfterEach = router.afterEach((to, from) => {
    const now = Date.now()
    // 停留时长：跳过首次进入（无上一页或来自新开标签）。
    if (!firstEnter && from.path) {
      report(buildDwellEvent(from.path, (now - enteredAt) / 1000, getTraceId()))
    }
    firstEnter = false
    enteredAt = now

    report(buildPageEvent('pv', to.path, from.path || '(direct)', getTraceId()))
    if (isNewSession(storage)) {
      report(buildPageEvent('uv', to.path, from.path || '(direct)', getTraceId()))
    }
  })

  // 路由切换耗时：beforeEach 记录起点，afterEach 结算。
  let navStart = 0
  const removeBeforeEach = router.beforeEach((_to, _from, next) => {
    navStart = Date.now()
    next()
  })
  const removeTiming = router.afterEach((to) => {
    if (navStart > 0) {
      report(buildRouteTimingEvent(to.path, Date.now() - navStart, getTraceId()))
      navStart = 0
    }
  })

  return () => {
    removeAfterEach()
    removeBeforeEach()
    removeTiming()
  }
}
