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

// 模拟后端：按 keyword/subject/source 过滤、按 offset/limit 分页，并返回分面计数。
// 分面计数只跟关键词走（不含学科/来源过滤），与后端实现保持一致。
function mockListResponse(data: Mistake[] = mockMistakes) {
  httpMethods.get.mockImplementation(
    (_url: string, config?: { params?: Record<string, unknown> }) => {
      const params = config?.params ?? {}
      const keyword = params.keyword ? String(params.keyword).toLowerCase() : ''
      const terms = keyword.split(/\s+/).filter(Boolean)

      const sourceOf = (m: Mistake) =>
        (m.question?.source ?? '').trim() || (m.source ?? '').trim()

      const keywordMatched = data.filter((m) => {
        if (!terms.length) return true
        const hay = `${m.question?.subject ?? ''} ${m.question?.stem_text ?? ''} ${sourceOf(m)}`.toLowerCase()
        return terms.every((t) => hay.includes(t))
      })

      const items = keywordMatched.filter((m) => {
        if (params.subject && (m.question?.subject ?? '') !== params.subject) return false
        if (params.source && sourceOf(m) !== params.source) return false
        return true
      })

      const subjectCounts: Record<string, number> = {}
      const sourceCounts: Record<string, number> = {}
      for (const m of keywordMatched) {
        const subject = m.question?.subject ?? ''
        if (subject) subjectCounts[subject] = (subjectCounts[subject] ?? 0) + 1
        const source = sourceOf(m)
        if (source) sourceCounts[source] = (sourceCounts[source] ?? 0) + 1
      }

      const offset = Number(params.offset ?? 0)
      const limit = Number(params.limit ?? 20)
      return Promise.resolve(
        ok({
          items: items.slice(offset, offset + limit),
          total: items.length,
          subject_counts: subjectCounts,
          source_counts: sourceCounts,
        }),
      )
    },
  )
}

beforeEach(() => {
  httpMethods.get.mockReset()
  mockListResponse()
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
    mockListResponse(withSources)
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

  it('关键词搜索来源：防抖后按关键词请求并过滤', async () => {
    vi.useFakeTimers()
    mockListResponse(withSources)
    const wrapper = mountList()
    await flushPromises()

    await wrapper.find('input[placeholder*="搜索"]').setValue('期中考试')
    // 防抖未到点：只应有首次加载的 1 次请求。
    expect(httpMethods.get).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    expect(httpMethods.get).toHaveBeenLastCalledWith('/mistakes', {
      params: { offset: 0, limit: 20, keyword: '期中考试' },
    })
    expect(wrapper.text()).toContain('y = x² - 2x - 3')
    expect(wrapper.text()).not.toContain('∠C=90°')

    vi.useRealTimers()
  })
})

describe('MistakeList.vue 搜索', () => {
  function mountList() {
    const router = buildRouter()
    router.push('/list')
    return mount(MistakeList, { global: { plugins: [router] } })
  }

  it('关键词防抖：连续输入只发一次请求，并从第一页重新加载', async () => {
    vi.useFakeTimers()
    mockListResponse()
    const wrapper = mountList()
    await flushPromises()
    httpMethods.get.mockClear()

    const input = wrapper.find('input[placeholder*="搜索"]')
    await input.setValue('函')
    await input.setValue('函数')
    await input.setValue('函数题')

    // 防抖窗口内不发请求。
    expect(httpMethods.get).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    expect(httpMethods.get).toHaveBeenCalledTimes(1)
    expect(httpMethods.get).toHaveBeenCalledWith('/mistakes', {
      params: { offset: 0, limit: 20, keyword: '函数题' },
    })

    vi.useRealTimers()
  })

  it('命中的关键词在题干中标黄，且不区分大小写', async () => {
    vi.useFakeTimers()
    mockListResponse([
      {
        ...mockMistakes[0],
        question: { ...mockMistakes[0].question!, stem_text: '已知二次函数，Math 表示数学' },
      },
    ])
    const wrapper = mountList()
    await flushPromises()

    await wrapper.find('input[placeholder*="搜索"]').setValue('math')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    // 命中片段用 <mark> 包裹，原文大小写保持不变。
    expect(wrapper.html()).toContain('<mark')
    expect(wrapper.text()).toContain('Math')

    vi.useRealTimers()
  })

  it('切换学科立即按第一页重新请求，其它学科仍可选（分面计数不受学科筛选影响）', async () => {
    mockListResponse()
    const wrapper = mountList()
    await flushPromises()
    httpMethods.get.mockClear()

    const mathTab = wrapper.findAll('button').find((b) => b.text().includes('数学'))!
    await mathTab.trigger('click')
    await flushPromises()

    expect(httpMethods.get).toHaveBeenCalledWith('/mistakes', {
      params: { offset: 0, limit: 20, subject: '数学' },
    })

    const physicsTab = wrapper.findAll('button').find((b) => b.text().includes('物理'))!
    expect(physicsTab.text()).toContain('1')
  })

  it('筛选后没有匹配结果时展示筛选空态文案', async () => {
    vi.useFakeTimers()
    mockListResponse()
    const wrapper = mountList()
    await flushPromises()

    await wrapper.find('input[placeholder*="搜索"]').setValue('不存在的关键词')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    expect(wrapper.text()).toContain('没有匹配的错题')

    vi.useRealTimers()
  })
})

describe('MistakeList.vue 配图', () => {
  function mountList() {
    const router = buildRouter()
    router.push('/list')
    return mount(MistakeList, { global: { plugins: [router] } })
  }

  it('带图号的配图在图片下方标注，无图号的不标注', async () => {
    mockListResponse([
      {
        ...mockMistakes[0],
        question: {
          ...mockMistakes[0].question!,
          image: JSON.stringify([
            { key: 'geometry/task_1.svg', label: '图1' },
            { key: 'geometry/task_1_2.svg', label: '图2' },
          ]),
        },
      },
      {
        ...mockMistakes[1],
        question: {
          ...mockMistakes[1].question!,
          image: JSON.stringify([{ key: 'geometry/task_2.svg' }]),
        },
      },
    ])

    const wrapper = mountList()
    await flushPromises()

    // 只有带图号的两张图有标注。
    expect(wrapper.findAll('figcaption').map((c) => c.text())).toEqual(['图1', '图2'])

    const html = wrapper.html()
    expect(html).toContain('/api/recognition/files/geometry/task_1.svg')
    expect(html).toContain('/api/recognition/files/geometry/task_1_2.svg')
    expect(html).toContain('/api/recognition/files/geometry/task_2.svg')
  })
})
