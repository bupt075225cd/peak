import { describe, it, expect, vi } from 'vitest'
import router from './index'

// poetry-dashboard 懒加载链路引入 echarts 图表：jsdom 无 canvas，
// 与 PoetryDashboard.test.ts 一致以 stub 替代。
vi.mock('vue-echarts', () => ({ default: { name: 'VChart', template: '<div />' } }))
vi.mock('echarts-wordcloud', () => ({}))

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

  it('诗歌模块拥有独立主页面及子页面路由', () => {
    const names = router.getRoutes().map((r) => r.name)
    expect(names).toContain('poetry-dashboard')
    expect(names).toContain('poetry-list')
    expect(names).toContain('poetry-detail')
  })

  it('poetry-dashboard 路由可解析到组件（懒加载）', async () => {
    const poetry = router.getRoutes().find((r) => r.name === 'poetry-dashboard')
    expect(poetry).toBeDefined()
    const loader = poetry!.components!.default as () => Promise<unknown>
    const comp = await loader()
    expect(comp).toBeTruthy()
  })
})
