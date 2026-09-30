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

// 使用真实功能开关（SMS_LOGIN_ENABLED=false）：验证当前产品形态为仅密码登录；
// 开关开启时的验证码登录行为见 Login.sms.test.ts。
function buildRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', redirect: '/login' },
      { path: '/login', name: 'login', component: Login },
      { path: '/home', name: 'home', component: { template: '<div>home-page</div>' } },
      { path: '/register', name: 'register', component: { template: '<div>register-page</div>' } },
      { path: '/forgot-password', name: 'forgot-password', component: { template: '<div>forgot-page</div>' } },
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

beforeEach(() => {
  logout()
  httpMethods.post.mockReset()
  httpMethods.get.mockReset()
})

describe('Login.vue 默认状态', () => {
  it('渲染密码登录表单', () => {
    const { wrapper } = mountLogin()
    expect(wrapper.find('#login-account').exists()).toBe(true)
    expect(wrapper.find('#login-password').exists()).toBe(true)
    expect(wrapper.text()).toContain('密码登录')
  })

  it('功能开关关闭时隐藏验证码登录 Tab 与手机号输入', () => {
    const { wrapper } = mountLogin()
    expect(wrapper.find('#login-phone').exists()).toBe(false)
    expect(wrapper.findAll('button').some((b) => b.text().includes('验证码登录'))).toBe(false)
    // 密码登录入口仍提供忘记密码与注册跳转。
    expect(wrapper.text()).toContain('忘记密码？')
    expect(wrapper.text()).toContain('邮箱注册')
  })
})

describe('Login.vue 密码登录', () => {
  it('密码登录成功写入令牌并跳转主页', async () => {
    const { wrapper, router } = mountLogin()
    httpMethods.post.mockResolvedValueOnce(
      ok({ token: 'jwt-pwd', user: { id: 2, account: 'stu@peak.local', email: 'stu@peak.local', phone: null, name: '同学stu', email_verified: true } }),
    )
    await wrapper.find('#login-account').setValue('stu@peak.local')
    await wrapper.find('#login-password').setValue('password123')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(httpMethods.post).toHaveBeenCalledWith('/users/auth/password/login', {
      account: 'stu@peak.local', password: 'password123',
    })
    expect(localStorage.getItem(TOKEN_KEY)).toBe('jwt-pwd')
    expect(router.currentRoute.value.path).toBe('/home')
    logout()
  })

  it('密码登录失败展示统一错误信息', async () => {
    const { wrapper } = mountLogin()
    httpMethods.post.mockRejectedValueOnce(fail('账号或密码不正确', 401))
    await wrapper.find('#login-account').setValue('stu@peak.local')
    await wrapper.find('#login-password').setValue('wrong-pass')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('账号或密码不正确')
    expect(localStorage.getItem(TOKEN_KEY)).toBeNull()
  })

  it('触发防爆破限流时透传 429 提示且不清除登录流程', async () => {
    const { wrapper } = mountLogin()
    httpMethods.post.mockRejectedValueOnce(fail('尝试次数过多，账号已锁定，请约 15 分钟后再试', 429))
    await wrapper.find('#login-account').setValue('stu@peak.local')
    await wrapper.find('#login-password').setValue('correct-pass-1')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('尝试次数过多，账号已锁定')
    expect(localStorage.getItem(TOKEN_KEY)).toBeNull()
  })
})
