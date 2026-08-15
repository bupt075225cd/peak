import { describe, it, expect } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import MistakeList from './MistakeList.vue'

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

describe('MistakeList.vue', () => {
  it('渲染示例错题数量', () => {
    const wrapper = mount(MistakeList, {
      global: { plugins: [buildRouter()] },
    })
    expect(wrapper.text()).toContain('我的错题本')
    expect(wrapper.text()).toContain('共 3 道错题')
  })

  it('渲染每道错题的题干', () => {
    const wrapper = mount(MistakeList, {
      global: { plugins: [buildRouter()] },
    })
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
    // 定位文字为「录入错题」的按钮（组件内唯一的 goEntry 入口）
    const btn = wrapper.findAll('button').find((b) => b.text().includes('录入错题'))
    expect(btn).toBeDefined()
    await btn!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/entry')
  })

  it('难度标签根据 difficulty 渲染不同文案', () => {
    const wrapper = mount(MistakeList, {
      global: { plugins: [buildRouter()] },
    })
    // difficulty=3 对应「中等」
    expect(wrapper.text()).toContain('中等')
  })
})
