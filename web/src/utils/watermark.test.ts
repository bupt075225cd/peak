import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { dataURLToBlob, downloadBlob, ledgerMark, watermarkDataURL } from './watermark'

beforeEach(() => {
  sessionStorage.clear()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('ledgerMark', () => {
  it('同一会话内稳定，格式为短标记 + 日期', () => {
    const first = ledgerMark(new Date(2026, 9, 23))
    const second = ledgerMark(new Date(2026, 9, 23))
    expect(first).toBe(second)
    expect(first).toMatch(/^[A-Z0-9]{1,6} · 2026\.10\.23$/)
  })

  it('标记只写 sessionStorage，不落 localStorage', () => {
    ledgerMark()
    expect(sessionStorage.getItem('peak_ledger_mark')).toBeTruthy()
    expect(localStorage.getItem('peak_ledger_mark')).toBeNull()
  })

  it('会话清空后重新生成新标记', () => {
    const first = ledgerMark()
    sessionStorage.clear()
    const second = ledgerMark()
    expect(second).toMatch(/^[A-Z0-9]{1,6} · \d{4}\.\d{2}\.\d{2}$/)
    expect(first).toMatch(/^[A-Z0-9]{1,6} · \d{4}\.\d{2}\.\d{2}$/)
  })
})

describe('downloadBlob', () => {
  it('创建临时链接并触发点击后清理', () => {
    vi.useFakeTimers()
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const createObjectURL = vi.fn(() => 'blob:mock')
    const revokeObjectURL = vi.fn()
    Object.defineProperty(URL, 'createObjectURL', { value: createObjectURL, configurable: true })
    Object.defineProperty(URL, 'revokeObjectURL', { value: revokeObjectURL, configurable: true })

    downloadBlob(new Blob(['x'], { type: 'image/png' }), '组成.png')

    expect(createObjectURL).toHaveBeenCalled()
    expect(click).toHaveBeenCalled()
    expect(document.querySelectorAll('a[download="组成.png"]')).toHaveLength(0)

    vi.advanceTimersByTime(1000)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock')
    vi.useRealTimers()
  })
})

describe('watermarkDataURL', () => {
  class FakeImage {
    width = 800
    height = 400
    onload: (() => void) | null = null
    onerror: (() => void) | null = null
    set src(_value: string) {
      queueMicrotask(() => this.onload?.())
    }
  }

  function stubCanvas(context: unknown) {
    const original = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation(((tag: string) => {
      if (tag !== 'canvas') return original(tag)
      return {
        width: 0,
        height: 0,
        getContext: () => context,
        toDataURL: () => 'data:image/png;base64,MARKED',
      }
    }) as typeof document.createElement)
  }

  it('在右下角绘制伪名水印并返回新的 dataURL', async () => {
    vi.stubGlobal('Image', FakeImage)
    const ctx = {
      drawImage: vi.fn(),
      measureText: () => ({ width: 120 }),
      fillRect: vi.fn(),
      fillText: vi.fn(),
    }
    stubCanvas(ctx)

    const result = await watermarkDataURL('data:image/png;base64,QQ==', 'AB12CD · 2026.10.23')

    expect(result).toBe('data:image/png;base64,MARKED')
    expect(ctx.drawImage).toHaveBeenCalled()
    expect(ctx.fillText).toHaveBeenCalledWith('家庭资产账本 · AB12CD · 2026.10.23', expect.any(Number), expect.any(Number))
    vi.unstubAllGlobals()
  })

  it('画布不可用时原样返回', async () => {
    vi.stubGlobal('Image', FakeImage)
    stubCanvas(null)
    await expect(watermarkDataURL('data:image/png;base64,QQ==', 'X')).resolves.toBe('data:image/png;base64,QQ==')
    vi.unstubAllGlobals()
  })
})

describe('dataURLToBlob', () => {
  it('解析 base64 载荷与 MIME', () => {
    const blob = dataURLToBlob('data:image/png;base64,QQ==')
    expect(blob.type).toBe('image/png')
    expect(blob.size).toBe(1)
  })

  it('缺少 MIME 时回退 image/png', () => {
    expect(dataURLToBlob('data:,QQ==').type).toBe('image/png')
  })
})
