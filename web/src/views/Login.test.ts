import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import Login from './Login.vue'
import { logout } from '../composables/useAuth'

function buildRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', redirect: '/login' },
      { path: '/login', name: 'login', component: Login },
      { path: '/home', name: 'home', component: { template: '<div>home-page</div>' } },
    ],
  })
}

function mountLogin() {
  const router = buildRouter()
  router.push('/login')
  return mount(Login, { global: { plugins: [router] } })
}

beforeEach(() => {
  logout()
})

describe('Login.vue（占位页）', () => {
  it('渲染登录表单骨架与开发中提示', () => {
    const wrapper = mountLogin()
    expect(wrapper.find('#login-username').exists()).toBe(true)
    expect(wrapper.find('#login-password').exists()).toBe(true)
    expect(wrapper.text()).toContain('登录功能开发中')
  })

  it('点击演示登录后写入令牌并跳转主页', async () => {
    const router = buildRouter()
    router.push('/login')
    await router.isReady()
    const wrapper = mount(Login, { global: { plugins: [router] } })

    await wrapper.findAll('button').find((b) => b.text().includes('演示登录'))!.trigger('click')
    await flushPromises()

    expect(localStorage.getItem('peak_token')).toBe('demo-token')
    expect(router.currentRoute.value.path).toBe('/home')
    logout()
  })

  it('密码可见性切换', async () => {
    const wrapper = mountLogin()
    const passwordInput = wrapper.find('#login-password')
    expect(passwordInput.attributes('type')).toBe('password')

    await wrapper.find('[aria-label="显示密码"]').trigger('click')
    expect(wrapper.find('#login-password').attributes('type')).toBe('text')

    await wrapper.find('[aria-label="隐藏密码"]').trigger('click')
    expect(wrapper.find('#login-password').attributes('type')).toBe('password')
  })
})
