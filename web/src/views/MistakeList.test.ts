import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import MistakeList from './MistakeList.vue'
import type { ApiResponse, Mistake } from '../api'

const { httpMethods } = vi.hoisted(() => ({
  httpMethods: { post: vi.fn(), get: vi.fn() },
}))

vi.mock('axios', () => ({
  default: { create: () => ({ ...httpMethods, defaults: { headers: { common: {} } } }) },
}))

function buildRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', redirect: '/list' },
      { path: '/list', name: 'list', component: MistakeList },
      { path: '/entry', name: 'entry', component: { template: '<div>entry</div>' } },
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
    mastery_level: 0,
    source_paper: '',
    recorded_at: '2026-08-10T00:00:00Z',
    question: {
      id: 10,
      subject: '数学',
      stem_text: '已知二次函数 y = x² - 2x - 3，求其顶点坐标与对称轴。',
      answer: '',
      analysis: '',
      difficulty: 3,
      question_type: '解答题',
    },
  },
  {
    id: 2,
    user_id: 1,
    question_id: 11,
    wrong_reason: '',
    mastery_level: 0,
    source_paper: '',
    recorded_at: '2026-08-09T00:00:00Z',
    question: {
      id: 11,
      subject: '数学',
      stem_text: '在直角三角形 ABC 中，∠C=90°，AC=3，BC=4，求 AB 的长。',
      answer: '',
      analysis: '',
      difficulty: 2,
      question_type: '解答题',
    },
  },
  {
    id: 3,
    user_id: 1,
    question_id: 12,
    wrong_reason: '',
    mastery_level: 0,
    source_paper: '',
    recorded_at: '2026-08-08T00:00:00Z',
    question: {
      id: 12,
      subject: '物理',
      stem_text: '一个质量为 2kg 的物体在水平面上受到 10N 拉力，求加速度。',
      answer: '',
      analysis: '',
      difficulty: 2,
      question_type: '解答题',
    },
  },
]

beforeEach(() => {
  httpMethods.get.mockReset()
  httpMethods.get.mockResolvedValue(ok({ items: mockMistakes, total: 3 }))
})

describe('MistakeList.vue', () => {
  it('挂载时加载错题并渲染数量', async () => {
    const wrapper = mount(MistakeList, {
      global: { plugins: [buildRouter()] },
    })
    await flushPromises()
    expect(httpMethods.get).toHaveBeenCalledWith('/mistakes')
    expect(wrapper.text()).toContain('我的错题本')
    expect(wrapper.text()).toContain('共 3 道错题')
  })

  it('渲染每道错题的题干', async () => {
    const wrapper = mount(MistakeList, {
      global: { plugins: [buildRouter()] },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('y = x² - 2x - 3')
    expect(wrapper.text()).toContain('∠C=90°')
  })

  it('点击「录入错题」跳转到 /entry', async () => {
    const router = buildRouter()
    router.push('/list')
    await router.isReady()
    const wrapper = mount(MistakeList, {
      global: { plugins: [router] },
    })
    await flushPromises()
    const btn = wrapper.findAll('button').find((b) => b.text().includes('录入错题'))
    expect(btn).toBeDefined()
    await btn!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/entry')
  })
})
