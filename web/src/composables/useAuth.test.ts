import { describe, it, expect, beforeEach } from 'vitest'
import { useAuth, TOKEN_KEY } from './useAuth'

// 跨标签页登录态同步：storage 事件（其他标签页 login/logout 时触发）。
describe('useAuth', () => {
  beforeEach(() => {
    localStorage.removeItem(TOKEN_KEY)
  })

  it('storage 事件同步其他标签页的登录态变化', () => {
    const { isLoggedIn, login, logout } = useAuth()
    expect(isLoggedIn.value).toBe(false)

    // 本标签页登录后，其他标签页写入令牌 -> storage 事件 -> 同步为已登录。
    login('token-a')
    window.dispatchEvent(new StorageEvent('storage', { key: TOKEN_KEY, newValue: 'token-b' }))
    expect(isLoggedIn.value).toBe(true)

    // 其他标签页登出（先清存储再派发事件；key=null 表示 clear）-> 同步为未登录。
    localStorage.removeItem(TOKEN_KEY)
    window.dispatchEvent(new StorageEvent('storage', { key: null }))
    expect(isLoggedIn.value).toBe(false)

    logout()
  })
})
