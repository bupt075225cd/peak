import { describe, it, expect } from 'vitest'
import { resolveAppKey } from './useAppBrand'

describe('resolveAppKey', () => {
  it('按路径前缀识别应用', () => {
    expect(resolveAppKey('/ledger')).toBe('ledger')
    expect(resolveAppKey('/ledger/items')).toBe('ledger')
    expect(resolveAppKey('/poetry/list')).toBe('poetry')
    expect(resolveAppKey('/home')).toBe('mistake')
    expect(resolveAppKey('/list')).toBe('mistake')
  })

  it('登录/注册页按 redirect 参数归属', () => {
    expect(resolveAppKey('/login', '/ledger')).toBe('ledger')
    expect(resolveAppKey('/login', '/ledger/items')).toBe('ledger')
    expect(resolveAppKey('/register', '/ledger/composition')).toBe('ledger')
    expect(resolveAppKey('/login', '/poetry/list')).toBe('poetry')
    expect(resolveAppKey('/login', '/list')).toBe('mistake')
  })

  it('缺少或非法 redirect 时回落到错题本品牌', () => {
    expect(resolveAppKey('/login')).toBe('mistake')
    expect(resolveAppKey('/login', undefined)).toBe('mistake')
    expect(resolveAppKey('/login', ['/ledger'])).toBe('mistake')
    expect(resolveAppKey('/login', '//evil.com/ledger')).toBe('mistake')
  })

  it('非登录页忽略 redirect 参数，只认当前路径', () => {
    expect(resolveAppKey('/ledger', '/poetry')).toBe('ledger')
    expect(resolveAppKey('/home', '/ledger')).toBe('mistake')
  })
})
