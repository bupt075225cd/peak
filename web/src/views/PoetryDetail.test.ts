import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import PoetryDetail from './PoetryDetail.vue'
import type { Poem } from '../api/poetry'

const { getPoem, getRelatedPoems } = vi.hoisted(() => ({
  getPoem: vi.fn(),
  getRelatedPoems: vi.fn(),
}))

vi.mock('../api/poetry', () => ({ getPoem, getRelatedPoems }))

const mockPoem: Poem = {
  id: 1,
  title: '爱莲说',
  dynasty: '宋',
  dynasty_order: 6,
  author: '周敦颐',
  volume: '七年级下册',
  category: '课内',
  content: '水陆草木之花，可爱者甚蕃。\n予独爱莲之出淤泥而不染。',
  word_count: 119,
}

const mockRelated: Poem[] = [
  { ...mockPoem, id: 2, title: '陋室铭', author: '刘禹锡' },
]

function buildRouter(id: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', redirect: '/poetry/list' },
      { path: '/poetry/list', name: 'poetry-list', component: { template: '<div />' } },
      { path: '/poetry/detail/:id', name: 'poetry-detail', component: PoetryDetail },
    ],
  })
  router.push(`/poetry/detail/${id}`)
  return router
}

async function mountView(id = '1') {
  const router = buildRouter(id)
  await router.isReady()
  const wrapper = mount(PoetryDetail, { global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  getPoem.mockReset()
  getRelatedPoems.mockReset()
  getPoem.mockResolvedValue(mockPoem)
  getRelatedPoems.mockResolvedValue(mockRelated)
})

describe('PoetryDetail.vue', () => {
  it('渲染篇名、元信息徽标与分段正文', async () => {
    const { wrapper } = await mountView()
    expect(wrapper.find('[data-test="poem-title"]').text()).toBe('爱莲说')
    expect(wrapper.find('[data-test="poem-meta"]').text()).toContain('周敦颐')
    expect(wrapper.find('[data-test="poem-meta"]').text()).toContain('课内')
    const paragraphs = wrapper.findAll('[data-test="poem-content"] p')
    expect(paragraphs).toHaveLength(2)
    expect(paragraphs[0].text()).toContain('水陆草木之花')
  })

  it('渲染相关篇目推荐并可跳转', async () => {
    const { wrapper, router } = await mountView()
    const links = wrapper.findAll('[data-test="related-poems"] a')
    expect(links).toHaveLength(1)
    expect(links[0].text()).toContain('陋室铭')
    expect(router.currentRoute.value.params.id).toBe('1')
  })

  it('篇目不存在时展示错误态', async () => {
    getPoem.mockRejectedValue({ response: { data: { message: '篇目不存在' } } })
    const { wrapper } = await mountView('999')
    expect(wrapper.find('[data-test="detail-error"]').text()).toContain('篇目不存在')
  })

  it('非法路由参数不发起请求', async () => {
    await mountView('abc')
    expect(getPoem).not.toHaveBeenCalled()
  })

  it('点击 AI 入口打开占位弹窗', async () => {
    const { wrapper } = await mountView()
    // 弹窗经 Teleport 挂载到 body。
    expect(document.querySelector('[data-test="ai-chat-modal"]')).toBeNull()
    await wrapper.find('[data-test="detail-open-chat"]').trigger('click')
    await flushPromises()
    expect(document.querySelector('[data-test="ai-chat-modal"]')).not.toBeNull()
  })

  it('返回按钮跳回列表页', async () => {
    const { wrapper, router } = await mountView()
    await wrapper.find('[data-test="back-to-list"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/poetry/list')
  })
})
