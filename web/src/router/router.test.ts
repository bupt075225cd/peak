import { describe, it, expect } from 'vitest'
import router from './index'

describe('router/index.ts', () => {
  it('根路径重定向到 /home', () => {
    const root = router.getRoutes().find((r) => r.path === '/')
    expect(root).toBeDefined()
    expect(root?.redirect).toBe('/home')
  })

  it('包含 home/login/entry/list 四条路由', () => {
    const names = router.getRoutes().map((r) => r.name)
    expect(names).toContain('home')
    expect(names).toContain('login')
    expect(names).toContain('entry')
    expect(names).toContain('list')
  })

  it('entry 路由可解析到组件', async () => {
    const entry = router.getRoutes().find((r) => r.name === 'entry')
    expect(entry).toBeDefined()
    const loader = entry!.components!.default as () => Promise<unknown>
    const comp = await loader()
    expect(comp).toBeTruthy()
  })

  it('list 路由可解析到组件（懒加载）', async () => {
    const list = router.getRoutes().find((r) => r.name === 'list')
    expect(list).toBeDefined()
    const loader = list!.components!.default as () => Promise<unknown>
    const comp = await loader()
    expect(comp).toBeTruthy()
  })
})
