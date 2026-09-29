import { ref, readonly } from 'vue'

// 登录令牌在 localStorage 中的键名。
//
// 登录页（待开发）完成后，只需调用 login(token) 写入同一键即可接管登录态；
// 若后续切换为服务端会话校验（如 /auth/me），仅需替换本模块实现。
const TOKEN_KEY = 'peak_token'

// 模块级单例：所有组件共享同一登录态，一处变更全局生效。
const isLoggedIn = ref(readToken())

function readToken(): boolean {
  return !!localStorage.getItem(TOKEN_KEY)
}

// 监听跨标签页的登录态变化（其他标签页 login/logout 时同步）。
window.addEventListener('storage', (e) => {
  if (e.key === TOKEN_KEY || e.key === null) {
    isLoggedIn.value = readToken()
  }
})

// 写入登录令牌并更新登录态（供登录页复用）。
export function login(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
  isLoggedIn.value = true
}

// 清除登录令牌并退出登录态。
export function logout(): void {
  localStorage.removeItem(TOKEN_KEY)
  isLoggedIn.value = false
}

// 登录态组合式函数：响应式 isLoggedIn + login/logout。
export function useAuth() {
  return {
    isLoggedIn: readonly(isLoggedIn),
    login,
    logout,
  }
}
