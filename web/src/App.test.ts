import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import App from './App.vue'

function buildRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', redirect: '/entry' },
      { path: '/entry', name: 'entry', component: { template: '<div>entry-page</div>' } },
      { path: '/list', name: 'list', component: { template: '<div>list-page</div>' } },
    ],
  })
}

describe('App.vue', () => {
  it('渲染顶部导航栏与应用标题', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    expect(wrapper.text()).toContain('Peak 错题本')
    expect(wrapper.text()).toContain('录入错题')
    expect(wrapper.text()).toContain('错题本')
  })

  it('渲染 RouterView 内容', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    expect(wrapper.text()).toContain('entry-page')
  })

  it('当前路由为 entry 时高亮「录入错题」', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    const entryLink = wrapper.findAll('a').find((a) => a.text().includes('录入错题'))
    expect(entryLink).toBeDefined()
    expect(entryLink!.classes()).toContain('bg-primary/10')
  })

  it('切换到 /list 后高亮「错题本」', async () => {
    const router = buildRouter()
    router.push('/list')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    const listLink = wrapper.findAll('a').find((a) => a.text().includes('错题本'))
    expect(listLink).toBeDefined()
    expect(listLink!.classes()).toContain('bg-primary/10')
    expect(wrapper.text()).toContain('list-page')
  })

  it('渲染用户信息占位', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    expect(wrapper.text()).toContain('小明')
  })
})
