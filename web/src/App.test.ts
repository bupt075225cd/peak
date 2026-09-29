import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import App from './App.vue'
import { login, logout } from './composables/useAuth'

function buildRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', redirect: '/home' },
      { path: '/home', name: 'home', component: { template: '<div>home-page</div>' } },
      { path: '/login', name: 'login', component: { template: '<div>login-page</div>' } },
      { path: '/entry', name: 'entry', component: { template: '<div>entry-page</div>' } },
      { path: '/list', name: 'list', component: { template: '<div>list-page</div>' } },
    ],
  })
}

beforeEach(() => {
  logout()
})

describe('App.vue（未登录）', () => {
  it('渲染顶部导航栏与应用标题', async () => {
    const router = buildRouter()
    router.push('/login')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    expect(wrapper.text()).toContain('Peak 错题本')
    expect(wrapper.text()).toContain('登录')
  })

  it('未登录时隐藏业务入口与用户占位', async () => {
    const router = buildRouter()
    router.push('/login')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    expect(wrapper.text()).not.toContain('录入错题')
    expect(wrapper.text()).not.toContain('小明')
  })
})

describe('App.vue（已登录）', () => {
  beforeEach(() => {
    login('test-token')
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
    const listLink = wrapper.findAll('a').find((a) => a.attributes('href') === '/list')
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
