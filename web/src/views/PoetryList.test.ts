import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import PoetryList from './PoetryList.vue'
import type { Poem } from '../api/poetry'

const { listPoems } = vi.hoisted(() => ({ listPoems: vi.fn() }))

vi.mock('../api/poetry', () => ({
  listPoems,
  getPoem: vi.fn(),
  getRelatedPoems: vi.fn(),
}))

const mockPoems: Poem[] = [
  {
    id: 1,
    title: '关雎',
    dynasty: '先秦',
    dynasty_order: 0,
    author: '佚名',
    volume: '八年级下册',
    category: '课内',
    content: '关关雎鸠，在河之洲。',
    word_count: 60,
  },
  {
    id: 2,
    title: '望岳',
    dynasty: '唐',
    dynasty_order: 5,
    author: '杜甫',
    volume: '七年级下册',
    category: '课内',
    content: '岱宗夫如何？齐鲁青未了。',
    word_count: 40,
  },
]

function buildRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', redirect: '/poetry/list' },
      { path: '/poetry/list', name: 'poetry-list', component: PoetryList },
      { path: '/poetry/detail/:id', name: 'poetry-detail', component: { template: '<div />' } },
    ],
  })
}

async function mountView() {
  const router = buildRouter()
  router.push('/poetry/list')
  await router.isReady()
  const wrapper = mount(PoetryList, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  listPoems.mockReset()
  listPoems.mockResolvedValue({ items: mockPoems, total: 2 })
})

afterEach(() => {
  vi.useRealTimers()
})

describe('PoetryList.vue', () => {
  it('加载并渲染篇目卡片（篇名与徽标）', async () => {
    const wrapper = await mountView()
    expect(wrapper.text()).toContain('关雎')
    expect(wrapper.text()).toContain('望岳')
    expect(wrapper.text()).toContain('杜甫')
    expect(wrapper.find('[data-test="result-count"]').text()).toContain('共 2 篇')
  })

  it('输入关键词后防抖触发检索（新关键词首字母置顶第一页）', async () => {
    vi.useFakeTimers()
    const wrapper = await mountView()
    listPoems.mockClear()

    const input = wrapper.find('[data-test="poetry-search"]')
    await input.setValue('明月')
    expect(listPoems).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(350)
    expect(listPoems).toHaveBeenCalledTimes(1)
    expect(listPoems).toHaveBeenCalledWith(expect.objectContaining({ keyword: '明月', offset: 0 }))
  })

  it('选择朝代筛选触发重新检索', async () => {
    vi.useFakeTimers()
    const wrapper = await mountView()
    listPoems.mockClear()

    await wrapper.find('[data-test="filter-dynasty"]').setValue('唐')
    await vi.advanceTimersByTimeAsync(350)
    expect(listPoems).toHaveBeenCalledWith(expect.objectContaining({ dynasty: '唐' }))
  })

  it('结果为空时展示空态', async () => {
    listPoems.mockResolvedValue({ items: [], total: 0 })
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="empty-state"]').exists()).toBe(true)
  })

  it('加载失败展示错误提示', async () => {
    listPoems.mockRejectedValue({ response: { data: { message: '服务不可用' } } })
    const wrapper = await mountView()
    expect(wrapper.text()).toContain('服务不可用')
  })

  it('多页时渲染分页并可翻页', async () => {
    listPoems.mockResolvedValue({
      items: mockPoems,
      total: 45, // 3 页
    })
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="pagination"]').exists()).toBe(true)

    await wrapper.find('[data-test="next-page"]').trigger('click')
    await flushPromises()
    expect(listPoems).toHaveBeenLastCalledWith(expect.objectContaining({ offset: 20 }))

    await wrapper.find('[data-test="prev-page"]').trigger('click')
    await flushPromises()
    expect(listPoems).toHaveBeenLastCalledWith(expect.objectContaining({ offset: 0 }))
  })

  it('点击卡片跳转详情页', async () => {
    const wrapper = await mountView()
    await wrapper.find('[data-test="poem-item-1"]').trigger('click')
    await flushPromises()
    expect(wrapper.vm.$router.currentRoute.value.path).toBe('/poetry/detail/1')
  })
})
