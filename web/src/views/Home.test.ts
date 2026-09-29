import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import Home from './Home.vue'
import { login, logout } from '../composables/useAuth'
import type { ApiResponse, Mistake } from '../api'

const { httpMethods } = vi.hoisted(() => ({
  httpMethods: { post: vi.fn(), get: vi.fn(), put: vi.fn() },
}))

vi.mock('axios', () => ({
  default: {
    create: () => ({
      ...httpMethods,
      interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
      defaults: { headers: { common: {} } },
    }),
  },
}))

function buildRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', redirect: '/home' },
      { path: '/home', name: 'home', component: Home },
      { path: '/login', name: 'login', component: { template: '<div>login-page</div>' } },
      { path: '/entry', name: 'entry', component: { template: '<div>entry-page</div>' } },
      { path: '/list', name: 'list', component: { template: '<div>list-page</div>' } },
    ],
  })
}

function ok<T>(data: T): { data: ApiResponse<T> } {
  return { data: { code: 0, message: 'ok', data } }
}

const mockMistakes: Mistake[] = [
  {
    id: 1,
    user_id: 1,
    question_id: 10,
    wrong_reason: '',
    source: '',
    recorded_at: '2026-09-20T00:00:00Z',
    question: {
      id: 10,
      subject: '数学',
      stem_text: '已知二次函数 y = x² - 2x - 3，求其顶点坐标与对称轴。',
      answer: '',
      analysis: '',
      question_type: '解答题',
    },
  },
]

function mountHome() {
  const router = buildRouter()
  router.push('/home')
  return mount(Home, { global: { plugins: [router] } })
}

beforeEach(() => {
  vi.clearAllMocks()
  logout()
})

describe('Home.vue（未登录）', () => {
  it('渲染产品落地页：Hero 标语与功能亮点', () => {
    const wrapper = mountHome()
    expect(wrapper.text()).toContain('进步的阶梯')
    expect(wrapper.text()).toContain('让每一道错题')
    expect(wrapper.text()).toContain('拍照录入')
    expect(wrapper.text()).toContain('组卷练习')
  })

  it('未登录不发起错题统计请求', () => {
    mountHome()
    expect(httpMethods.get).not.toHaveBeenCalled()
  })

  it('CTA 链接指向登录页', () => {
    const wrapper = mountHome()
    const loginLinks = wrapper.findAll('a').filter((a) => a.attributes('href') === '/login')
    expect(loginLinks.length).toBeGreaterThan(0)
  })
})

describe('Home.vue（已登录）', () => {
  beforeEach(() => {
    login('test-token')
  })

  it('渲染仪表盘：统计卡片、学科分布与最近录入', async () => {
    httpMethods.get.mockResolvedValueOnce(
      ok({
        items: mockMistakes,
        total: 12,
        subject_counts: { 数学: 8, 物理: 4 },
        source_counts: { 期中考试: 7 },
      }),
    )
    const wrapper = mountHome()
    await flushPromises()

    expect(httpMethods.get).toHaveBeenCalledWith('/mistakes', {
      params: { offset: 0, limit: 5 },
    })
    expect(wrapper.text()).toContain('错题总数')
    expect(wrapper.text()).toContain('覆盖学科')
    expect(wrapper.text()).toContain('学科分布')
    expect(wrapper.text()).toContain('数学')
    expect(wrapper.text()).toContain('最近录入')
    expect(wrapper.text()).toContain('二次函数')
  })

  it('无错题时展示空状态引导', async () => {
    httpMethods.get.mockResolvedValueOnce(
      ok({ items: [], total: 0, subject_counts: {}, source_counts: {} }),
    )
    const wrapper = mountHome()
    await flushPromises()

    expect(wrapper.text()).toContain('还没有错题')
    expect(wrapper.text()).toContain('去录入')
  })

  it('接口失败时静默降级为空状态', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    httpMethods.get.mockRejectedValueOnce(new Error('network down'))
    const wrapper = mountHome()
    await flushPromises()

    expect(wrapper.text()).toContain('还没有错题')
    errorSpy.mockRestore()
  })
})
