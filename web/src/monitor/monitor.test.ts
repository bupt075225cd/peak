import { describe, expect, it, vi, afterEach } from 'vitest'
import { sampled } from './report'
import { newTraceparent, randomHex, traceIdFromTraceparent } from './trace'
import { buildErrorEvent, attachErrorListeners, describeResourceError, isResourceErrorTarget } from './errors'
import { buildPageEvent, buildDwellEvent, buildRouteTimingEvent, isNewSession } from './behavior'
import { buildVitalEvent } from './vitals'
import { createReporter, defaultSend } from './report'
import { initMonitor, getPageTraceId } from './index'
import type { MonitorEvent } from './types'

describe('trace', () => {
  it('生成合法 W3C traceparent', () => {
    const tp = newTraceparent()
    const parts = tp.split('-')
    expect(parts).toHaveLength(4)
    expect(parts[0]).toBe('00')
    expect(parts[1]).toMatch(/^[0-9a-f]{32}$/)
    expect(parts[2]).toMatch(/^[0-9a-f]{16}$/)
    expect(parts[3]).toBe('01')
  })

  it('从 traceparent 提取 trace id', () => {
    expect(traceIdFromTraceparent('00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01'))
      .toBe('4bf92f3577b34da6a3ce929d0e0e4736')
    expect(traceIdFromTraceparent('garbage')).toBe('')
  })

  it('randomHex 长度与字符集', () => {
    expect(randomHex(8)).toMatch(/^[0-9a-f]{8}$/)
  })
})

describe('sampling', () => {
  it('采样率边界', () => {
    expect(sampled(1)).toBe(true)
    expect(sampled(0)).toBe(false)
    for (let i = 0; i < 20; i++) {
      const v = sampled(0.5)
      expect(typeof v).toBe('boolean')
    }
  })
})

describe('event builders', () => {
  it('错误事件', () => {
    const e = buildErrorEvent('js_error', '/list', '4bf92f3577b34da6a3ce929d0e0e4736', { message: 'boom' })
    expect(e.type).toBe('error')
    expect(e.name).toBe('js_error')
    expect(e.path).toBe('/list')
    expect(e.trace_id).toBe('4bf92f3577b34da6a3ce929d0e0e4736')
    expect(typeof e.ts).toBe('number')
  })

  it('PV/UV 事件携带来源页', () => {
    const e = buildPageEvent('pv', '/list', '/home')
    expect(e.type).toBe('pv')
    expect(e.data?.from).toBe('/home')
  })

  it('停留时长事件（负值钳为 0，保留 1 位小数）', () => {
    expect(buildDwellEvent('/home', -5).data?.seconds).toBe(0)
    expect(buildDwellEvent('/home', 3.456).data?.seconds).toBe(3.5)
  })

  it('路由耗时事件', () => {
    expect(buildRouteTimingEvent('/list', 128).data?.ms).toBe(128)
  })

  it('Web Vitals 事件', () => {
    const e = buildVitalEvent(
      { name: 'LCP', value: 1234.5678, rating: 'needs-improvement', delta: 0, id: 'v1', entries: [], navigationType: 'navigate' } as never,
      '/home',
    )
    expect(e.type).toBe('webvital')
    expect(e.name).toBe('LCP')
    expect(e.data?.value).toBe(1234.57)
    expect(e.data?.rating).toBe('needs-improvement')
  })
})

describe('reporter', () => {
  const opts = { errorSampleRate: 1, behaviorSampleRate: 1, maxBatchSize: 2 }

  function ev(name: string, type: MonitorEvent['type'] = 'error'): MonitorEvent {
    return { type, name, ts: 1, path: '/x' }
  }

  it('队列满触发批量上报', () => {
    const sent: string[] = []
    const r = createReporter({}, (payload) => {
      sent.push(payload)
      return true
    })
    r.enqueue(ev('a'), opts)
    r.enqueue(ev('b'), opts)
    expect(sent).toHaveLength(1)
    const parsed = JSON.parse(sent[0]) as { events: MonitorEvent[] }
    expect(parsed.events.map((e) => e.name)).toEqual(['a', 'b'])
    expect(r.pending()).toBe(0)
  })

  it('flush 发送剩余队列', () => {
    const sent: string[] = []
    const r = createReporter({}, (payload) => {
      sent.push(payload)
      return true
    })
    r.enqueue(ev('only'), opts)
    expect(sent).toHaveLength(0)
    r.flush()
    expect(sent).toHaveLength(1)
  })

  it('采样不命中的事件不入队', () => {
    const sent: string[] = []
    const r = createReporter({}, (payload) => {
      sent.push(payload)
      return true
    })
    r.enqueue(ev('drop'), { ...opts, errorSampleRate: 0 })
    r.flush()
    expect(sent).toHaveLength(0)
  })

  it('发送抛错不影响调用方', () => {
    const r = createReporter({}, () => {
      throw new Error('network down')
    })
    expect(() => {
      r.enqueue(ev('a'), opts)
    }).not.toThrow()
    expect(() => r.flush()).not.toThrow()
  })
})

describe('defaultSend', () => {
  it('jsdom 下无 sendBeacon 时退化为 fetch（不真正发送，只验证不抛错）', () => {
    // jsdom 无 sendBeacon；fetch 为 undici mock，返回 promise。静默失败即可。
    expect(() => defaultSend('{"events":[]}', false)).not.toThrow()
  })
})

describe('error listeners', () => {
  it('捕获全局 JS 错误、Promise 拒绝与资源加载失败', () => {
    const events: MonitorEvent[] = []
    const detach = attachErrorListeners(
      (e) => events.push(e),
      () => '/list',
      () => '4bf92f3577b34da6a3ce929d0e0e4736',
    )

    // 1. 全局 JS 错误。
    const errEvent = new ErrorEvent('error', {
      message: 'boom',
      filename: 'app.js',
      lineno: 12,
      colno: 3,
      error: new Error('boom'),
    })
    window.dispatchEvent(errEvent)

    // 2. Promise 未处理拒绝（jsdom 无 PromiseRejectionEvent，用自定义类构造）。
    class FakeRejectionEvent extends Event {
      reason: unknown
      promise: Promise<unknown>
      constructor(reason: unknown) {
        super('unhandledrejection')
        this.reason = reason
        this.promise = Promise.resolve()
      }
    }
    window.dispatchEvent(new FakeRejectionEvent(new Error('async fail')))

    // 3. 资源加载失败（img error，不冒泡，捕获阶段触发）。
    const img = document.createElement('img')
    const resEvent = new Event('error')
    Object.defineProperty(resEvent, 'target', { value: img, enumerable: true })
    window.dispatchEvent(resEvent)

    // window 自身的 error（非资源）不应记为 resource_error。
    const windowErr = new Event('error')
    Object.defineProperty(windowErr, 'target', { value: window })
    window.dispatchEvent(windowErr)

    const names = events.map((e) => e.name)
    expect(names).toContain('js_error')
    expect(names).toContain('unhandled_rejection')
    expect(names).toContain('resource_error')
    expect(names.filter((n) => n === 'resource_error')).toHaveLength(1)

    const jsErr = events.find((e) => e.name === 'js_error')!
    expect(jsErr.path).toBe('/list')
    expect(jsErr.data?.message).toBe('boom')
    expect(jsErr.trace_id).toBe('4bf92f3577b34da6a3ce929d0e0e4736')

    const res = events.find((e) => e.name === 'resource_error')!
    expect(res.data?.tag).toBe('IMG')

    detach()
    // 解绑后不再捕获。
    events.length = 0
    window.dispatchEvent(new ErrorEvent('error', { message: 'after' }))
    expect(events).toHaveLength(0)
  })

  it('isResourceErrorTarget / describeResourceError', () => {
    expect(isResourceErrorTarget({ target: { tagName: 'IMG' } } as unknown as Event)).toBe(true)
    expect(isResourceErrorTarget({ target: { tagName: 'DIV' } } as unknown as Event)).toBe(false)
    expect(isResourceErrorTarget(null)).toBe(false)
    const imgLike = document.createElement('img')
    imgLike.src = 'https://x/404.png'
    const d = describeResourceError({ target: imgLike } as unknown as Event)
    expect(d.url).toContain('404.png')
  })
})

describe('initMonitor', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('初始化后提供页面 trace id，且重复调用安全', () => {
    const router = { currentRoute: { value: { path: '/home' } } }
    initMonitor({ router } as never)
    expect(getPageTraceId()).toMatch(/^[0-9a-f]{32}$/)
    // 第二次调用不抛错也不改变 trace id。
    const before = getPageTraceId()
    initMonitor({ router } as never)
    expect(getPageTraceId()).toBe(before)
  })
})

describe('isNewSession', () => {
  it('首次 true，标记后 false', () => {
    const store = new Map<string, string>()
    const fake = {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
    } as unknown as Storage
    expect(isNewSession(fake)).toBe(true)
    expect(isNewSession(fake)).toBe(false)
  })

  it('storage 不可用时始终视为新会话', () => {
    const broken = {
      getItem: () => {
        throw new Error('blocked')
      },
      setItem: () => {
        throw new Error('blocked')
      },
    } as unknown as Storage
    expect(isNewSession(broken)).toBe(true)
  })
})
