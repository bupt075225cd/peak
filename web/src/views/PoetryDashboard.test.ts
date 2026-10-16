import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import PoetryDashboard from './PoetryDashboard.vue'

const { getDynastyStats, getVolumeStats, getAuthorStats, getWordStats, sendChat } = vi.hoisted(
  () => ({
    getDynastyStats: vi.fn(),
    getVolumeStats: vi.fn(),
    getAuthorStats: vi.fn(),
    getWordStats: vi.fn(),
    sendChat: vi.fn(),
  }),
)

// 图表渲染依赖 canvas，测试环境以 stub 替代 vue-echarts；
// echarts-wordcloud 在模块加载期检测 canvas，jsdom 不支持，一并 mock。
vi.mock('vue-echarts', () => ({ default: { name: 'VChart', template: '<div />' } }))
vi.mock('echarts-wordcloud', () => ({}))
vi.mock('../api/poetry', () => ({
  getDynastyStats,
  getVolumeStats,
  getAuthorStats,
  getWordStats,
  sendChat,
}))

function buildRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', redirect: '/poetry' },
      { path: '/poetry', name: 'poetry-dashboard', component: PoetryDashboard },
      { path: '/poetry/list', name: 'poetry-list', component: { template: '<div />' } },
    ],
  })
}

async function mountView() {
  const router = buildRouter()
  router.push('/poetry')
  await router.isReady()
  const wrapper = mount(PoetryDashboard, { global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  getDynastyStats.mockReset()
  getVolumeStats.mockReset()
  getAuthorStats.mockReset()
  getWordStats.mockReset()
  sendChat.mockReset()

  getDynastyStats.mockResolvedValue([
    { dynasty: '先秦', order: 0, count: 17 },
    { dynasty: '唐', order: 5, count: 46 },
  ])
  getVolumeStats.mockResolvedValue([{ volume: '七年级上册', inclass: 8, outline: 2, other: 0 }])
  getAuthorStats.mockResolvedValue([{ author: '李白', count: 8 }])
  getWordStats.mockResolvedValue({ items: [{ word: '明', count: 120 }], total_chars: 9999 })
})

describe('PoetryDashboard.vue', () => {
  it('加载统计并展示篇目总量（63 = 17 + 46）', async () => {
    const { wrapper } = await mountView()
    expect(wrapper.find('[data-test="poetry-total"]').text()).toBe('63')
    expect(wrapper.find('[data-test="filter-bar"]').exists()).toBe(true)
    expect(wrapper.findAll('.rounded-2xl').length).toBeGreaterThanOrEqual(5) // 5 张图表卡
  })

  it('统计加载失败展示错误提示', async () => {
    getDynastyStats.mockRejectedValue({ response: { data: { message: '服务不可用' } } })
    const { wrapper } = await mountView()
    expect(wrapper.find('[data-test="stats-error"]').text()).toContain('服务不可用')
  })

  it('带筛选跳转列表页并透传查询参数', async () => {
    const { wrapper, router } = await mountView()
    await wrapper.find('[data-test="filter-dynasty"]').setValue('唐')
    await wrapper.find('[data-test="go-list"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/poetry/list')
    expect(router.currentRoute.value.query.dynasty).toBe('唐')
  })

// AI 弹窗通过 Teleport 挂载到 body，需从 document 查询。
const modalEl = () => document.querySelector<HTMLElement>('[data-test="ai-chat-modal"]')
const inputEl = () => document.querySelector<HTMLInputElement>('[data-test="ai-chat-input"]')
const sendEl = () => document.querySelector<HTMLButtonElement>('[data-test="ai-chat-send"]')
const responseEl = () => document.querySelector<HTMLElement>('[data-test="ai-chat-response"]')
const closeEl = () => document.querySelector<HTMLButtonElement>('[data-test="ai-chat-close"]')

function setInput(el: HTMLInputElement, value: string) {
  el.value = value
  el.dispatchEvent(new Event('input', { bubbles: true }))
}

async function click(el: HTMLElement) {
  el.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  await flushPromises()
}

it('打开 AI 问答占位弹窗并发送问题', async () => {
  sendChat.mockResolvedValue({ enabled: false, notice: '敬请期待' })
  const { wrapper } = await mountView()
  expect(modalEl()).toBeNull()

  await click(wrapper.find('[data-test="open-ai-chat"]').element as HTMLElement)
  expect(modalEl()).not.toBeNull()

  setInput(inputEl()!, '《爱莲说》的作者是谁？')
  await click(sendEl()!)
  expect(sendChat).toHaveBeenCalledWith('《爱莲说》的作者是谁？')
  expect(responseEl()!.textContent).toContain('敬请期待')

  await click(closeEl()!)
  expect(modalEl()).toBeNull()
})

it('AI 请求失败时展示兜底提示', async () => {
  sendChat.mockRejectedValue(new Error('network error'))
  const { wrapper } = await mountView()
  await click(wrapper.find('[data-test="open-ai-chat"]').element as HTMLElement)
  setInput(inputEl()!, '问题')
  await click(sendEl()!)
  expect(responseEl()!.textContent).toContain('请求失败')
})
})
