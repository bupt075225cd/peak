import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import Login from './Login.vue'
import { logout, TOKEN_KEY } from '../composables/useAuth'

const { httpMethods } = vi.hoisted(() => ({
  httpMethods: { post: vi.fn(), get: vi.fn() },
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
      { path: '/', redirect: '/login' },
      { path: '/login', name: 'login', component: Login },
      { path: '/home', name: 'home', component: { template: '<div>home-page</div>' } },
    ],
  })
}

function mountLogin() {
  const router = buildRouter()
  router.push('/login')
  const wrapper = mount(Login, { global: { plugins: [router] } })
  return { wrapper, router }
}

function ok<T>(data: T) {
  return { data: { code: 0, message: 'ok', data } }
}

function fail(message: string, status: number) {
  return { response: { status, data: { code: 1001, message } } }
}

const VALID_PHONE = '13800001234'

beforeEach(() => {
  logout()
  httpMethods.post.mockReset()
  httpMethods.get.mockReset()
})

describe('Login.vue', () => {
  it('渲染手机号与验证码输入', () => {
    const { wrapper } = mountLogin()
    expect(wrapper.find('#login-phone').exists()).toBe(true)
    expect(wrapper.find('#login-code').exists()).toBe(true)
    expect(wrapper.text()).toContain('自动创建账号')
  })

  it('手机号不合法时获取验证码按钮禁用', async () => {
    const { wrapper } = mountLogin()
    await wrapper.find('#login-phone').setValue('12345')
    const sendBtn = wrapper.findAll('button').find((b) => b.text().includes('获取验证码'))!
    expect(sendBtn.attributes('disabled')).toBeDefined()
  })

  it('发送验证码后显示倒计时并保存凭证', async () => {
    vi.useFakeTimers()
    const { wrapper } = mountLogin()
    httpMethods.post.mockResolvedValueOnce(ok({ ticket: 't-1', debug_code: '654321' }))

    await wrapper.find('#login-phone').setValue(VALID_PHONE)
    const sendBtn = wrapper.findAll('button').find((b) => b.text().includes('获取验证码'))!
    await sendBtn.trigger('click')
    await flushPromises()

    expect(httpMethods.post).toHaveBeenCalledWith('/users/auth/sms/code', { phone: VALID_PHONE })
    expect(wrapper.text()).toContain('重新发送')
    expect(wrapper.text()).toContain('开发模式验证码：654321')
    vi.useRealTimers()
  })

  it('验证码登录成功写入令牌并跳转主页', async () => {
    const { wrapper, router } = mountLogin()
    httpMethods.post
      .mockResolvedValueOnce(ok({ ticket: 't-1' }))
      .mockResolvedValueOnce(ok({ token: 'jwt-token', user: { id: 1, account: VALID_PHONE, phone: VALID_PHONE, name: '同学1234' } }))

    await wrapper.find('#login-phone').setValue(VALID_PHONE)
    const sendBtn = wrapper.findAll('button').find((b) => b.text().includes('获取验证码'))!
    await sendBtn.trigger('click')
    await flushPromises()

    await wrapper.find('#login-code').setValue('123456')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(httpMethods.post).toHaveBeenLastCalledWith('/users/auth/sms/login', {
      phone: VALID_PHONE, code: '123456', ticket: 't-1',
    })
    expect(localStorage.getItem(TOKEN_KEY)).toBe('jwt-token')
    expect(router.currentRoute.value.path).toBe('/home')
    logout()
  })

  it('登录失败展示服务端错误信息', async () => {
    const { wrapper } = mountLogin()
    httpMethods.post
      .mockResolvedValueOnce(ok({ ticket: 't-1' }))
      .mockRejectedValueOnce(fail('验证码错误', 401))

    await wrapper.find('#login-phone').setValue(VALID_PHONE)
    const sendBtn = wrapper.findAll('button').find((b) => b.text().includes('获取验证码'))!
    await sendBtn.trigger('click')
    await flushPromises()

    await wrapper.find('#login-code').setValue('000000')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('验证码错误')
    expect(localStorage.getItem(TOKEN_KEY)).toBeNull()
  })
})
