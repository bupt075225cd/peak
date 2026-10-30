import { beforeEach, describe, expect, it, vi } from 'vitest'

const HIDDEN_KEY = 'peak_ledger_hidden'

// 隐藏状态是模块级单例，初始值只在首次 import 时读取 sessionStorage，
// 所以每个用例都要重置模块后重新导入，才能验证「进入页面时的默认值」。
async function load() {
  vi.resetModules()
  return import('./useLedgerPrivacy')
}

describe('useLedgerPrivacy', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('首次进入默认隐藏金额', async () => {
    const { useLedgerPrivacy } = await load()
    expect(useLedgerPrivacy().hidden.value).toBe(true)
  })

  it('本次会话里点开过金额后重新加载仍显示', async () => {
    sessionStorage.setItem(HIDDEN_KEY, '0')
    const { useLedgerPrivacy } = await load()
    expect(useLedgerPrivacy().hidden.value).toBe(false)
  })

  it('切换与隐藏都会写回 sessionStorage', async () => {
    const { useLedgerPrivacy } = await load()
    const privacy = useLedgerPrivacy()

    privacy.show()
    expect(privacy.hidden.value).toBe(false)
    expect(sessionStorage.getItem(HIDDEN_KEY)).toBe('0')

    privacy.hide()
    expect(privacy.hidden.value).toBe(true)
    expect(sessionStorage.getItem(HIDDEN_KEY)).toBe('1')

    privacy.toggle()
    expect(privacy.hidden.value).toBe(false)
    expect(sessionStorage.getItem(HIDDEN_KEY)).toBe('0')
  })
})
