import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import Register from './Register.vue'
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
      { path: '/', redirect: '/home' },
      { path: '/home', name: 'home', component: { template: '<div>home-page</div>' } },
      { path: '/login', name: 'login', component: { template: '<div>login-page</div>' } },
      { path: '/register', name: 'register', component: { template: '<div>register-page</div>' } },
    ],
  })
}

async function mountRegister() {
  const router = buildRouter()
  router.push('/register')
  await router.isReady()
  const wrapper = mount(Register, { global: { plugins: [router] } })
  return { wrapper, router }
}

function ok<T>(data: T) {
  return { data: { code: 0, message: 'ok', data } }
}

function findSendBtn(wrapper: VueWrapper) {
  // 倒计时中按钮文案是"重新发送(Ns)"，初始是"获取验证码"。
  return wrapper.findAll('button').find((b) => b.text().includes('验证码') || b.text().includes('重新发送'))
}

beforeEach(() => {
  logout()
  httpMethods.post.mockReset()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('Register.vue', () => {
  it('渲染注册表单', async () => {
    const { wrapper } = await mountRegister()
    expect(wrapper.find('#reg-email').exists()).toBe(true)
    expect(wrapper.find('#reg-password').exists()).toBe(true)
    expect(wrapper.find('#reg-confirm').exists()).toBe(true)
    expect(wrapper.find('#reg-code').exists()).toBe(true)
    expect(wrapper.text()).toContain('注册 Peak 错题本')
  })

  it('邮箱、密码与确认密码的校验提示', async () => {
    const { wrapper } = await mountRegister()
    await wrapper.find('#reg-email').setValue('not-an-email')
    await wrapper.find('#reg-password').setValue('short')
    await wrapper.find('#reg-confirm').setValue('other')
    expect(wrapper.text()).toContain('邮箱格式不正确')
    expect(wrapper.text()).toContain('密码至少 8 位')
    expect(wrapper.text()).toContain('两次输入的密码不一致')
  })

  it('发送验证码成功：提示脱敏邮箱、展示调试码并进入倒计时，倒计时结束恢复可点', async () => {
    vi.useFakeTimers()
    const { wrapper } = await mountRegister()
    httpMethods.post.mockResolvedValueOnce(ok({ debug_code: '123456' }))

    await wrapper.find('#reg-email').setValue('stu@peak.local')
    await findSendBtn(wrapper)!.trigger('click')
    await flushPromises()

    expect(httpMethods.post).toHaveBeenCalledWith('/users/auth/email/code', {
      email: 'stu@peak.local',
      purpose: 'register',
    })
    expect(wrapper.text()).toContain('验证码已发送至')
    expect(wrapper.text()).toContain('开发模式验证码：123456')
    // 倒计时进行中，按钮不可点。
    expect(wrapper.text()).toContain('重新发送(')
    expect(findSendBtn(wrapper)!.attributes('disabled')).toBeDefined()

    // 倒计时走完后恢复可点。
    vi.advanceTimersByTime(61_000)
    await flushPromises()
    expect(wrapper.text()).toContain('获取验证码')
    expect(findSendBtn(wrapper)!.attributes('disabled')).toBeUndefined()
  })

  it('发送验证码失败展示服务端错误', async () => {
    const { wrapper } = await mountRegister()
    httpMethods.post.mockRejectedValueOnce({
      response: { data: { message: '发送过于频繁，请稍后再试' } },
    })

    await wrapper.find('#reg-email').setValue('stu@peak.local')
    await findSendBtn(wrapper)!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('发送过于频繁，请稍后再试')
  })

  it('注册成功写入令牌并跳转主页', async () => {
    const { wrapper, router } = await mountRegister()
    httpMethods.post.mockResolvedValueOnce(ok({ token: 'jwt-reg', user: { id: 9, email: 'stu@peak.local' } }))

    await wrapper.find('#reg-email').setValue('stu@peak.local')
    await wrapper.find('#reg-password').setValue('password123')
    await wrapper.find('#reg-confirm').setValue('password123')
    await wrapper.find('#reg-code').setValue('654321')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(httpMethods.post).toHaveBeenCalledWith('/users/auth/email/register', {
      email: 'stu@peak.local',
      password: 'password123',
      code: '654321',
    })
    expect(localStorage.getItem(TOKEN_KEY)).toBe('jwt-reg')
    expect(router.currentRoute.value.path).toBe('/home')
    logout()
  })

  it('注册失败展示服务端错误', async () => {
    const { wrapper, router } = await mountRegister()
    httpMethods.post.mockRejectedValueOnce({ response: { data: { message: '验证码错误或已过期' } } })

    await wrapper.find('#reg-email').setValue('stu@peak.local')
    await wrapper.find('#reg-password').setValue('password123')
    await wrapper.find('#reg-confirm').setValue('password123')
    await wrapper.find('#reg-code').setValue('000000')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('验证码错误或已过期')
    expect(localStorage.getItem(TOKEN_KEY)).toBeNull()
    expect(router.currentRoute.value.path).toBe('/register')
  })

  it('密码可见性切换', async () => {
    const { wrapper } = await mountRegister()
    const pwd = wrapper.find('#reg-password')
    await pwd.setValue('password123')
    expect(pwd.attributes('type')).toBe('password')

    // 点击眼睛按钮切换明文/密文。
    const eyeBtn = wrapper
      .findAll('button')
      .find((b) => ['隐藏密码', '显示密码'].includes(b.attributes('aria-label') ?? ''))
    await eyeBtn!.trigger('click')
    expect(wrapper.find('#reg-password').attributes('type')).toBe('text')
    await eyeBtn!.trigger('click')
    expect(wrapper.find('#reg-password').attributes('type')).toBe('password')
  })
})
