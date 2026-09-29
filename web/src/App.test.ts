import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import App from './App.vue'
import { login, logout } from './composables/useAuth'

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
      { path: '/home', name: 'home', component: { template: '<div>home-page</div>' } },
      { path: '/login', name: 'login', component: { template: '<div>login-page</div>' } },
      { path: '/entry', name: 'entry', component: { template: '<div>entry-page</div>' } },
      { path: '/list', name: 'list', component: { template: '<div>list-page</div>' } },
    ],
  })
}

function ok<T>(data: T) {
  return { data: { code: 0, message: 'ok', data } }
}

beforeEach(() => {
  logout()
  httpMethods.post.mockReset()
  httpMethods.get.mockReset()
  httpMethods.put.mockReset()
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
    expect(wrapper.find('[data-test="user-menu-trigger"]').exists()).toBe(false)
  })
})

describe('App.vue（已登录）', () => {
  function mountLoggedIn() {
    login('test-token')
    const router = buildRouter()
    router.push('/entry')
    return router.isReady().then(() => {
      const wrapper = mount(App, { global: { plugins: [router] } })
      return { wrapper, router }
    })
  }

  beforeEach(() => {
    httpMethods.get.mockResolvedValue(ok({ id: 1, account: '13800001234', phone: '13800001234', name: '小明同学' }))
  })

  it('渲染 RouterView 内容', async () => {
    const { wrapper } = await mountLoggedIn()
    await flushPromises()
    expect(wrapper.text()).toContain('entry-page')
  })

  it('动态展示服务端返回的真实昵称', async () => {
    const { wrapper } = await mountLoggedIn()
    await flushPromises()
    expect(httpMethods.get).toHaveBeenCalledWith('/users/me')
    expect(wrapper.find('[data-test="user-menu-trigger"]').text()).toContain('小明同学')
  })

  it('昵称拉取失败时回退默认称呼', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    httpMethods.get.mockRejectedValueOnce(new Error('network down'))
    const { wrapper } = await mountLoggedIn()
    await flushPromises()
    expect(wrapper.find('[data-test="user-menu-trigger"]').text()).toContain('同学')
    errorSpy.mockRestore()
  })

  it('当前路由为 entry 时高亮「录入错题」', async () => {
    const { wrapper } = await mountLoggedIn()
    await flushPromises()
    const entryLink = wrapper.findAll('a').find((a) => a.text().includes('录入错题'))
    expect(entryLink).toBeDefined()
    expect(entryLink!.classes()).toContain('bg-primary/10')
  })

  it('切换到 /list 后高亮「错题本」', async () => {
    const { wrapper, router } = await mountLoggedIn()
    await flushPromises()
    await router.push('/list')
    await flushPromises()
    const listLink = wrapper.findAll('a').find((a) => a.attributes('href') === '/list')
    expect(listLink).toBeDefined()
    expect(listLink!.classes()).toContain('bg-primary/10')
    expect(wrapper.text()).toContain('list-page')
  })

  it('打开用户菜单并修改昵称后展示新昵称', async () => {
    const { wrapper } = await mountLoggedIn()
    await flushPromises()

    await wrapper.find('[data-test="user-menu-trigger"]').trigger('click')
    expect(wrapper.find('[data-test="edit-name-trigger"]').exists()).toBe(true)

    httpMethods.put.mockResolvedValueOnce(ok({ id: 1, account: '13800001234', phone: '13800001234', name: '新昵称' }))
    await wrapper.find('[data-test="edit-name-trigger"]').trigger('click')
    await wrapper.find('[data-test="nickname-input"]').setValue('新昵称')
    await wrapper.find('[data-test="nickname-save"]').trigger('click')
    await flushPromises()

    expect(httpMethods.put).toHaveBeenCalledWith('/users/me', { name: '新昵称' })
    expect(wrapper.find('[data-test="user-menu-trigger"]').text()).toContain('新昵称')
    expect(wrapper.find('[data-test="nickname-save"]').exists()).toBe(false)
  })

  it('修改昵称为空时提示且不发送请求', async () => {
    const { wrapper } = await mountLoggedIn()
    await flushPromises()

    await wrapper.find('[data-test="user-menu-trigger"]').trigger('click')
    await wrapper.find('[data-test="edit-name-trigger"]').trigger('click')
    await wrapper.find('[data-test="nickname-input"]').setValue('   ')
    await wrapper.find('[data-test="nickname-save"]').trigger('click')

    expect(httpMethods.put).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('昵称不能为空')
  })

  it('渲染用户信息占位', async () => {
    const { wrapper } = await mountLoggedIn()
    await flushPromises()
    expect(wrapper.find('[data-test="user-menu-trigger"]').exists()).toBe(true)
  })
})
