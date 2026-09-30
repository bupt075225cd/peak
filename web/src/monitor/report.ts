// 上报模块：队列 + 批量 + 采样 + 页面关闭兜底。
//
// 设计要点（面向"不阻塞用户请求、页面关闭数据不丢"）：
// - 事件先进内存队列，按「条数满（默认 20）或定时（默认 5s）」批量发送；
// - 发送优先 navigator.sendBeacon（浏览器保证页面卸载后仍送达），
//   不可用时退化为 fetch(..., { keepalive: true })；
// - 页面隐藏/关闭时立即 flush 队列（visibilitychange -> hidden、pagehide）；
// - 采样在事件入队前完成，被丢弃的事件不产生任何网络/内存开销。

import type { MonitorEvent, MonitorOptions } from './types'

const DEFAULT_ENDPOINT = '/api/monitor/report'
const DEFAULT_ERROR_SAMPLE = 1
const DEFAULT_BEHAVIOR_SAMPLE = 0.1
const DEFAULT_MAX_BATCH = 20
const DEFAULT_FLUSH_INTERVAL = 5000

/** 采样判断：rate 为 0~1，返回是否命中。 */
export function sampled(rate: number): boolean {
  if (rate >= 1) return true
  if (rate <= 0) return false
  return Math.random() < rate
}

export interface Reporter {
  /** 事件入队（内部完成采样），队列满时立即触发上报。 */
  enqueue(event: MonitorEvent, opts: Required<Pick<MonitorOptions, 'errorSampleRate' | 'behaviorSampleRate' | 'maxBatchSize'>>): void
  /** 立即上报队列中的全部事件。 */
  flush(): void
  /** 供测试检查当前积压。 */
  pending(): number
}

/** 默认发送：sendBeacon 优先，fetch keepalive 兜底；返回是否成功提交。 */
export function defaultSend(payload: string, keepalive: boolean): boolean {
  const endpoint = DEFAULT_ENDPOINT
  if (typeof navigator !== 'undefined' && typeof navigator.sendBeacon === 'function') {
    // sendBeacon 仅支持有限 MIME，Blob(type text/plain) 兼容性最好；
    // 服务端按 body 解析，不依赖 Content-Type。
    if (navigator.sendBeacon(endpoint, new Blob([payload], { type: 'text/plain;charset=UTF-8' }))) {
      return true
    }
  }
  if (typeof fetch === 'function') {
    fetch(endpoint, {
      method: 'POST',
      body: payload,
      keepalive,
      headers: { 'Content-Type': 'application/json' },
    }).catch(() => {
      /* 上报失败静默：监控绝不影响业务 */
    })
    return true
  }
  return false
}

export function createReporter(options: MonitorOptions = {}, send: (payload: string, keepalive: boolean) => boolean = defaultSend): Reporter {
  const maxBatchSize = options.maxBatchSize ?? DEFAULT_MAX_BATCH
  const flushInterval = options.flushInterval ?? DEFAULT_FLUSH_INTERVAL
  let queue: MonitorEvent[] = []
  let timer: ReturnType<typeof setInterval> | null = null

  function doFlush() {
    if (queue.length === 0) return
    const batch = queue
    queue = []
    // JSON 序列化失败等异常全部吞掉：监控永不抛错到业务。
    try {
      send(JSON.stringify({ events: batch }), false)
    } catch {
      /* ignore */
    }
  }

  function ensureTimer() {
    if (timer !== null) return
    timer = setInterval(doFlush, flushInterval)
    // Node 测试环境无 window 时也安全。
    if (typeof window !== 'undefined') {
      const flushOnHide = () => {
        if (document.visibilityState === 'hidden') doFlush()
      }
      window.addEventListener('pagehide', doFlush)
      document.addEventListener('visibilitychange', flushOnHide)
    }
  }

  return {
    enqueue(event, opts) {
      const isBehavior = event.type === 'pv' || event.type === 'behavior' || event.type === 'webvital'
      if (!sampled(isBehavior ? opts.behaviorSampleRate : opts.errorSampleRate)) return
      queue.push(event)
      ensureTimer()
      if (queue.length >= opts.maxBatchSize) doFlush()
    },
    flush: doFlush,
    pending: () => queue.length,
  }
}
