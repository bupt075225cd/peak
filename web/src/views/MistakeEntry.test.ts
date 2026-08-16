import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import MistakeEntry from './MistakeEntry.vue'
import type { ApiResponse, Category, RecognitionTask } from '../api'

const { httpMethods } = vi.hoisted(() => ({
  httpMethods: { post: vi.fn(), get: vi.fn() },
}))

vi.mock('axios', () => ({
  default: { create: () => ({ ...httpMethods, defaults: { headers: { common: {} } } }) },
}))

function buildRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', redirect: '/entry' },
      { path: '/entry', name: 'entry', component: MistakeEntry },
    ],
  })
}

function ok<T>(data: T): { data: ApiResponse<T> } {
  return { data: { code: 0, message: 'ok', data } }
}

beforeEach(() => {
  httpMethods.post.mockReset()
  httpMethods.get.mockReset()
  const cats: Category[] = [
    { id: 1, parent_id: null, name: '二次函数', type: 'tag', sort_order: 1 },
  ]
  httpMethods.get.mockResolvedValue(ok(cats))
  if (!URL.createObjectURL) URL.createObjectURL = vi.fn(() => 'blob:mock')
  // jsdom 未实现 alert，stub 之，避免保存成功路径里 alert 抛错中断 reset
  vi.stubGlobal('alert', vi.fn())
  vi.useFakeTimers()
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

function getSaveBtn(wrapper: ReturnType<typeof mount>) {
  return wrapper
    .findAll('button')
    .find((b) => b.text().includes('保存错题'))!
}

describe('MistakeEntry.vue', () => {
  it('题干为空时保存按钮被禁用（防止空提交）', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const saveBtn = getSaveBtn(wrapper)
    expect((saveBtn.element as HTMLButtonElement).disabled).toBe(true)
    // 此时不应调用 createMistake
    expect(httpMethods.post).not.toHaveBeenCalledWith('/mistakes', expect.anything())
  })

  it('识别成功后回填到表单', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const task: RecognitionTask = {
      id: 1,
      image_id: 1,
      status: 'success',
      progress: 100,
      provider: 'mock',
      result_json: JSON.stringify({
        stem_text: '已知函数 f(x)=x',
        answer: '1',
        formula: { latex: 'x^2', raw_text: '' },
        geometry: { shape_type: 'circle', properties: {}, description: '圆形' },
        erased_image_key: 'key123',
      }),
    }
    httpMethods.post.mockResolvedValueOnce(ok({ ...task, status: 'pending' }))
    httpMethods.get.mockResolvedValue(ok(task))

    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', {
      value: [file],
      configurable: true,
    })
    await input.trigger('change')
    await vi.runAllTimersAsync()
    await flushPromises()

    expect(wrapper.text()).toContain('识别完成')
    // 题干在 textarea 内，需用 .value 读取
    const ta = wrapper.find('textarea')
    expect((ta.element as HTMLTextAreaElement).value).toBe('已知函数 f(x)=x')
    // 公式由 KaTeX 渲染为 HTML，断言渲染产物的 class 存在即可（无需依赖具体文本节点）。
    expect(wrapper.html()).toContain('katex')
    // 识别完成后保存按钮可用
    expect((getSaveBtn(wrapper).element as HTMLButtonElement).disabled).toBe(false)
  })

  it('识别失败时展示失败状态与重试按钮', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const failed: RecognitionTask = {
      id: 2,
      image_id: 1,
      status: 'failed',
      progress: 0,
      provider: 'mock',
      error_message: '图片不清晰',
    }
    httpMethods.post.mockResolvedValueOnce(ok({ ...failed, status: 'pending' }))
    httpMethods.get.mockResolvedValue(ok(failed))

    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', {
      value: [file],
      configurable: true,
    })
    await input.trigger('change')
    await vi.runAllTimersAsync()
    await flushPromises()

    // 识别失败文案 + 错误信息 + 重试按钮
    expect(wrapper.text()).toContain('识别失败')
    expect(wrapper.text()).toContain('图片不清晰')
    const retryBtn = wrapper.findAll('button').find((b) => b.text().includes('重试'))
    expect(retryBtn).toBeDefined()
  })

  it('填写题干后点击保存会先创建题目再创建错题', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const ta = wrapper.find('textarea')
    ;(ta.element as HTMLTextAreaElement).value = 'y = x^2'
    await ta.trigger('input')
    await flushPromises()

    // 此时保存按钮可用
    const saveBtn = getSaveBtn(wrapper)
    expect((saveBtn.element as HTMLButtonElement).disabled).toBe(false)

    // 第一步 createQuestion 返回 question id=42，第二步 createMistake 关联该题目。
    httpMethods.post.mockResolvedValueOnce(ok({ id: 42 }))
    httpMethods.post.mockResolvedValueOnce({ data: { code: 0, message: 'ok', data: { id: 1 } } })

    await saveBtn.trigger('click')
    await vi.waitFor(() => {
      expect(httpMethods.post).toHaveBeenCalledWith('/questions', expect.objectContaining({
        stem_text: 'y = x^2',
        question_type: '解答',
      }))
    })
    await vi.waitFor(() => {
      expect(httpMethods.post).toHaveBeenCalledWith('/mistakes', expect.objectContaining({
        user_id: 1,
        question_id: 42,
      }))
    })
    // 保存成功后 reset() 清空题干
    await vi.waitFor(() => {
      expect((wrapper.find('textarea').element as HTMLTextAreaElement).value).toBe('')
    })
  })

  it('保存失败时展示错误提示', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const ta = wrapper.find('textarea')
    ;(ta.element as HTMLTextAreaElement).value = 'y = x^2'
    await ta.trigger('input')
    await flushPromises()

    // createQuestion 失败 → saveError 显示在底部操作栏
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    httpMethods.post.mockRejectedValueOnce(new Error('network'))
    const saveBtn = getSaveBtn(wrapper)
    await saveBtn.trigger('click')
    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('保存失败，请重试')
    })
    errSpy.mockRestore()
  })

  it('点击上传区触发文件选择（pickImage）', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const input = wrapper.find('input[type="file"]')
    const clickSpy = vi.spyOn(input.element as HTMLInputElement, 'click')
    // 上传区是绑定 @click="pickImage" 的最外层 div
    const dropzone = wrapper.find('.cursor-pointer')
    await dropzone.trigger('click')
    expect(clickSpy).toHaveBeenCalled()
  })

  it('拖拽文件触发识别（onDrop）', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const task: RecognitionTask = {
      id: 3,
      image_id: 1,
      status: 'pending',
      progress: 0,
      provider: 'mock',
    }
    httpMethods.post.mockResolvedValueOnce(ok(task))
    httpMethods.get.mockResolvedValue(ok(task))

    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const dropzone = wrapper.find('.cursor-pointer')
    await dropzone.trigger('drop', {
      dataTransfer: { files: [file] },
    })
    await flushPromises()
    expect(httpMethods.post).toHaveBeenCalledWith(
      '/recognition/tasks',
      expect.any(FormData),
    )
  })

  it('上传失败时展示错误', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    httpMethods.post.mockRejectedValueOnce(new Error('upload fail'))

    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', {
      value: [file],
      configurable: true,
    })
    await input.trigger('change')
    await flushPromises()
    expect(errSpy).toHaveBeenCalledWith('上传识别失败', expect.anything())
    errSpy.mockRestore()
  })

  it('识别失败后点击重试重新发起识别', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const failed: RecognitionTask = {
      id: 4,
      image_id: 1,
      status: 'failed',
      progress: 0,
      provider: 'mock',
      error_message: '图片不清晰',
    }
    // 上传返回 pending，随后轮询到 failed
    httpMethods.post.mockResolvedValueOnce(ok({ ...failed, status: 'pending' }))
    httpMethods.get.mockResolvedValue(ok(failed))

    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', {
      value: [file],
      configurable: true,
    })
    await input.trigger('change')
    await vi.runAllTimersAsync()
    await flushPromises()

    const retryBtn = wrapper.findAll('button').find((b) => b.text().includes('重试'))!
    expect(retryBtn).toBeDefined()

    // 点击重试：retryTask（POST /retry）后再次轮询
    httpMethods.post.mockResolvedValueOnce({ data: { code: 0, message: 'ok' } })
    await retryBtn.trigger('click')
    await vi.runAllTimersAsync()
    await flushPromises()
    expect(httpMethods.post).toHaveBeenCalledWith('/recognition/tasks/4/retry')
  })
})
