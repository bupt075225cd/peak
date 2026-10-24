import { computed } from 'vue'
import { useRoute } from 'vue-router'

export type AppKey = 'mistake' | 'poetry' | 'ledger'

/**
 * 根据当前路径推断所属应用。
 *
 * 登录/注册页本身不属于任何应用，但用户是从某个应用被守卫拦过来的，
 * 此时以 `redirect` 参数为准，避免访问账本时看到错题本品牌。
 */
export function resolveAppKey(path: string, redirect?: unknown): AppKey {
  const target = typeof redirect === 'string' ? redirect : ''
  const isAuthPage = path === '/login' || path === '/register'
  const probe = isAuthPage && target.startsWith('/') && !target.startsWith('//') ? target : path
  if (probe.startsWith('/poetry')) return 'poetry'
  if (probe.startsWith('/ledger')) return 'ledger'
  return 'mistake'
}

/** 应用品牌归属：App 外壳、登录页、注册页共用同一套判断。 */
export function useAppBrand() {
  const route = useRoute()
  const app = computed(() => resolveAppKey(route.path, route.query.redirect))
  return {
    app,
    isLedger: computed(() => app.value === 'ledger'),
    isPoetry: computed(() => app.value === 'poetry'),
    isMistake: computed(() => app.value === 'mistake'),
  }
}
