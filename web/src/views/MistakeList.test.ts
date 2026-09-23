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
    source: '',
    recorded_at: '2026-08-10T00:00:00Z',
    question: {
      id: 10,
      subject: '数学',
      stem_text: '已知二次函数 y = x² - 2x - 3，求其顶点坐标与对称轴。',
      answer: '',
      analysis: '',
      question_type: '解答题',
    },
  },
  {
    id: 2,
    user_id: 1,
    question_id: 11,
    wrong_reason: '',
    source: '',
    recorded_at: '2026-08-09T00:00:00Z',
    question: {
      id: 11,
      subject: '数学',
      stem_text: '在直角三角形 ABC 中，∠C=90°，AC=3，BC=4，求 AB 的长。',
      answer: '',
      analysis: '',
      question_type: '解答题',
    },
  },
  {
    id: 3,
    user_id: 1,
    question_id: 12,
    wrong_reason: '',
    source: '',
    recorded_at: '2026-08-08T00:00:00Z',
    question: {
      id: 12,
      subject: '物理',
      stem_text: '一个质量为 2kg 的物体在水平面上受到 10N 拉力，求加速度。',
      answer: '',
      analysis: '',
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
    expect(httpMethods.get).toHaveBeenCalledWith('/mistakes', { params: { offset: 0, limit: 20 } })
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

  it('显示后端返回的总条数，并支持「加载更多」追加下一页', async () => {
    // 第一页 2 条（共 4 条），第二页再返回 1 条。
    httpMethods.get
      .mockResolvedValueOnce(ok({ items: mockMistakes.slice(0, 2), total: 4 }))
      .mockResolvedValueOnce(ok({ items: [mockMistakes[2]], total: 4 }))

    const wrapper = mount(MistakeList, { global: { plugins: [buildRouter()] } })
    await flushPromises()

    // 总数取后端 total（而不是已加载条数）。
    expect(wrapper.text()).toContain('共 4 道错题')
    expect(wrapper.text()).toContain('已加载 2 条')

    const more = wrapper.findAll('button').find((b) => b.text().includes('加载更多'))
    expect(more).toBeDefined()
    await more!.trigger('click')
    await flushPromises()

    // 第二页从 offset=已加载条数 开始拉取。
    expect(httpMethods.get).toHaveBeenLastCalledWith('/mistakes', {
      params: { offset: 2, limit: 20 },
    })
    expect(wrapper.text()).toContain('已加载 3 条')
    // 新一页的题目已追加到列表。
    expect(wrapper.text()).toContain('一个质量为 2kg')
  })
})

describe('MistakeList.vue 导出', () => {
  beforeEach(() => {
    httpMethods.post.mockReset()

    // jsdom 未实现 object URL 与真实下载，需要打桩。
    Object.defineProperty(URL, 'createObjectURL', {
      value: vi.fn(() => 'blob:mock'),
      writable: true,
      configurable: true,
    })
    Object.defineProperty(URL, 'revokeObjectURL', {
      value: vi.fn(),
      writable: true,
      configurable: true,
    })
  })

  function mountList() {
    const router = buildRouter()
    router.push('/list')
    return mount(MistakeList, { global: { plugins: [router] } })
  }

  function buttonByText(wrapper: ReturnType<typeof mountList>, text: string) {
    const btn = wrapper.findAll('button').find((b) => b.text().includes(text))
    expect(btn, `button containing ${text}`).toBeDefined()
    return btn!
  }

  it('未勾选时导出当前筛选结果，并触发浏览器下载', async () => {
    httpMethods.post.mockResolvedValueOnce({
      data: new Blob(['pdf-bytes']),
      headers: {
        'content-disposition': `filename*=UTF-8''${encodeURIComponent('我的错题本 2026-09-17.pdf')}`,
      },
    })
    let downloadedName = ''
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      downloadedName = this.download
    })

    const wrapper = mountList()
    await flushPromises()

    await buttonByText(wrapper, '导出').trigger('click')
    await buttonByText(wrapper, '导出为 PDF').trigger('click')
    await flushPromises()

    expect(httpMethods.post).toHaveBeenCalledWith(
      '/mistakes/export',
      { ids: [1, 2, 3], format: 'pdf' },
      { responseType: 'blob', timeout: 120000 },
    )
    expect(URL.createObjectURL).toHaveBeenCalled()
    expect(URL.revokeObjectURL).toHaveBeenCalled()
    expect(downloadedName).toBe('我的错题本 2026-09-17.pdf')
  })

  it('勾选后只导出勾选的题目', async () => {
    httpMethods.post.mockResolvedValueOnce({ data: new Blob(['x']), headers: {} })

    const wrapper = mountList()
    await flushPromises()

    // 第 0 个复选框是「全选当前筛选」，其后依次为各错题卡片。
    await wrapper.findAll('input[type="checkbox"]')[1].setValue(true)
    await flushPromises()
    expect(wrapper.text()).toContain('已选 1 题')

    await buttonByText(wrapper, '导出').trigger('click')
    await buttonByText(wrapper, '导出为 Word').trigger('click')
    await flushPromises()

    expect(httpMethods.post).toHaveBeenCalledWith(
      '/mistakes/export',
      { ids: [1], format: 'docx' },
      { responseType: 'blob', timeout: 120000 },
    )
  })

  it('全选只作用于当前筛选结果', async () => {
    const wrapper = mountList()
    await flushPromises()

    // 切到物理学科：仅有 1 道题。
    await buttonByText(wrapper, '物理').trigger('click')
    await flushPromises()

    await wrapper.findAll('input[type="checkbox"]')[0].setValue(true)
    await flushPromises()

    expect(wrapper.text()).toContain('已选 1 题')
  })

  it('切换筛选后保留勾选，并提示被排除的题目不会被导出', async () => {
    const wrapper = mountList()
    await flushPromises()

    // 勾选一道数学题后切到物理学科。
    await wrapper.findAll('input[type="checkbox"]')[1].setValue(true)
    await flushPromises()
    await buttonByText(wrapper, '物理').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('已选 1 题')
    expect(wrapper.text()).toContain('不会被导出')
  })

  it('导出失败时展示服务端返回的错误信息', async () => {
    // 出错时服务端返回 JSON，但 axios 按 blob 读取，这里模拟可读文本的响应体。
    const payload = {
      text: async () => JSON.stringify({ code: 1004, message: '没有可导出的错题' }),
    }
    httpMethods.post.mockRejectedValueOnce({ response: { data: payload } })

    const wrapper = mountList()
    await flushPromises()

    await buttonByText(wrapper, '导出').trigger('click')
    await buttonByText(wrapper, '导出为 PDF').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('没有可导出的错题')
  })

  it('列表为空时导出按钮禁用', async () => {
    httpMethods.get.mockResolvedValue(ok({ items: [], total: 0 }))

    const wrapper = mountList()
    await flushPromises()

    expect(buttonByText(wrapper, '导出').attributes('disabled')).toBeDefined()
  })
})

describe('MistakeList.vue 来源', () => {
  const withSources: Mistake[] = [
    { ...mockMistakes[0], id: 1, source: '期中考试' },
    { ...mockMistakes[1], id: 2, source: '练习册 P32' },
  ]

  function mountList() {
    const router = buildRouter()
    router.push('/list')
    return mount(MistakeList, { global: { plugins: [router] } })
  }

  it('卡片展示来源，并可点来源胶囊筛选', async () => {
    httpMethods.get.mockResolvedValue(ok({ items: withSources, total: 2 }))
    const wrapper = mountList()
    await flushPromises()

    expect(wrapper.text()).toContain('来源：期中考试')
    expect(wrapper.text()).toContain('来源：练习册 P32')

    // 点「练习册 P32」胶囊后只剩第二条。
    const chip = wrapper.findAll('button').find((b) => b.text().includes('练习册 P32'))!
    await chip.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('∠C=90°')
    expect(wrapper.text()).not.toContain('y = x² - 2x - 3')
  })

  it('关键词可以搜索来源', async () => {
    httpMethods.get.mockResolvedValue(ok({ items: withSources, total: 2 }))
    const wrapper = mountList()
    await flushPromises()

    await wrapper.find('input[placeholder*="搜索"]').setValue('期中考试')
    await flushPromises()

    expect(wrapper.text()).toContain('y = x² - 2x - 3')
    expect(wrapper.text()).not.toContain('∠C=90°')
  })
})
