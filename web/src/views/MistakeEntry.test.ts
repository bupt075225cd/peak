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
  it('未上传照片时不展示题目信息与保存按钮', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    // 未识别：不展示题目信息卡片、学科/题型、保存按钮
    expect(wrapper.text()).not.toContain('题目信息')
    expect(wrapper.text()).not.toContain('学科')
    expect(wrapper.text()).not.toContain('题型')
    expect(wrapper.findAll('button').find((b) => b.text().includes('保存错题'))).toBeUndefined()
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
        subject: '物理',
        question_type: '选择题',
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
    // 学科、题型由识别结果自动回填并只读展示
    expect(wrapper.text()).toContain('物理')
    expect(wrapper.text()).toContain('选择题')
    // 未选择年级时保存按钮禁用（年级必填）
    expect((getSaveBtn(wrapper).element as HTMLButtonElement).disabled).toBe(true)
    // 选择年级后保存按钮可用
    const gradeSelect = wrapper.findAll('select')[0]
    await gradeSelect.setValue('七年级上')
    await flushPromises()
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

  it('识别后选择年级并保存会先创建题目再创建错题', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    // 先识别成功，回填题干，题目信息随之展示。
    const task: RecognitionTask = {
      id: 7,
      image_id: 1,
      status: 'success',
      progress: 100,
      provider: 'mock',
      result_json: JSON.stringify({
        stem_text: 'y = x^2',
        subject: '数学',
        question_type: '解答题',
      }),
    }
    httpMethods.post.mockResolvedValueOnce(ok({ ...task, status: 'pending' }))
    httpMethods.get.mockResolvedValue(ok(task))

    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await vi.runAllTimersAsync()
    await flushPromises()

    // 识别后题目信息展示，未选年级时保存按钮禁用（年级必填）
    let saveBtn = getSaveBtn(wrapper)
    expect((saveBtn.element as HTMLButtonElement).disabled).toBe(true)

    // 选择年级后保存按钮可用
    const gradeSelect = wrapper.findAll('select')[0]
    await gradeSelect.setValue('七年级上')
    await flushPromises()
    saveBtn = getSaveBtn(wrapper)
    expect((saveBtn.element as HTMLButtonElement).disabled).toBe(false)

    // 第一步 createQuestion 返回 question id=42，第二步 createMistake 关联该题目。
    httpMethods.post.mockResolvedValueOnce(ok({ id: 42 }))
    httpMethods.post.mockResolvedValueOnce({ data: { code: 0, message: 'ok', data: { id: 1 } } })

    await saveBtn.trigger('click')
    await vi.waitFor(() => {
      expect(httpMethods.post).toHaveBeenCalledWith('/questions', expect.objectContaining({
        stem_text: 'y = x^2',
        grade: '七年级上',
        question_type: '解答题',
      }))
    })
    await vi.waitFor(() => {
      expect(httpMethods.post).toHaveBeenCalledWith('/mistakes', expect.objectContaining({
        user_id: 1,
        question_id: 42,
      }))
    })
    // 保存成功后 reset() 清空题干，题目信息随之隐藏
    await vi.waitFor(() => {
      expect(wrapper.text()).not.toContain('题目信息')
    })
  })

  it('保存失败时展示错误提示', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    // 先识别成功，回填题干，题目信息随之展示。
    const task: RecognitionTask = {
      id: 8,
      image_id: 1,
      status: 'success',
      progress: 100,
      provider: 'mock',
      result_json: JSON.stringify({ stem_text: 'y = x^2', subject: '数学', question_type: '解答题' }),
    }
    httpMethods.post.mockResolvedValueOnce(ok({ ...task, status: 'pending' }))
    httpMethods.get.mockResolvedValue(ok(task))

    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await vi.runAllTimersAsync()
    await flushPromises()

    // 选择年级
    const gradeSelect = wrapper.findAll('select')[0]
    await gradeSelect.setValue('七年级上')
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

  it('裁剪上传失败时弹窗保持打开并显示错误，不静默关闭', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    // 先完成一次识别，让 previewUrl 有值（"重新框选"按钮才会显示）。
    const task: RecognitionTask = {
      id: 5,
      image_id: 1,
      status: 'success',
      progress: 100,
      provider: 'mock',
      result_json: JSON.stringify({
        stem_text: '几何题',
        formula: { latex: '', raw_text: '' },
        geometry: { shape_type: 'triangle', properties: {}, description: '三角形' },
        geometry_keys: ['geometry/task_5.jpg'],
      }),
    }
    httpMethods.post.mockResolvedValueOnce(ok({ ...task, status: 'pending' }))
    httpMethods.get.mockResolvedValue(ok(task))

    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await vi.runAllTimersAsync()
    await flushPromises()
    // 初始几何图是识别自动带出的旧值。
    expect(wrapper.text()).toContain('几何图形')

    // 点击「重新框选」打开裁剪弹窗。
    const recropBtn = wrapper.findAll('button').find((b) => b.text().includes('重新框选'))!
    await recropBtn.trigger('click')
    await flushPromises()

    // 裁剪上传失败：POST /recognition/files 拒绝。
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    httpMethods.post.mockRejectedValueOnce(new Error('upload crop fail'))

    // 触发 GeometryCropper 的 confirm 事件（模拟用户点击确认裁剪）。
    const cropper = wrapper.findComponent({ name: 'GeometryCropper' })
    expect(cropper.exists()).toBe(true)
    cropper.vm.$emit('confirm', new File(['crop'], 'geometry.jpg', { type: 'image/jpeg' }))
    await flushPromises()
    await vi.runAllTimersAsync()
    await flushPromises()

    // 弹窗保持打开（未 close）。
    const cropperProps = wrapper.findComponent({ name: 'GeometryCropper' }).props()
    expect(cropperProps.open).toBe(true)
    // 错误信息显示在弹窗内。
    expect(cropperProps.error).toContain('几何图上传失败')
    // 几何图 key 未被替换（仍是识别自动带出的旧值）。
    expect(httpMethods.post).not.toHaveBeenCalledWith('/questions', expect.objectContaining({
      geometry_refs: JSON.stringify(['geometry/task_5.jpg']),
    }))
    errSpy.mockRestore()
  })

  it('裁剪上传成功后替换几何图 key 并关闭弹窗', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const task: RecognitionTask = {
      id: 6,
      image_id: 1,
      status: 'success',
      progress: 100,
      provider: 'mock',
      result_json: JSON.stringify({
        stem_text: '几何题',
        formula: { latex: '', raw_text: '' },
        geometry: { shape_type: 'triangle', properties: {}, description: '三角形' },
        geometry_keys: ['geometry/task_6.jpg'],
      }),
    }
    httpMethods.post.mockResolvedValueOnce(ok({ ...task, status: 'pending' }))
    httpMethods.get.mockResolvedValue(ok(task))

    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await vi.runAllTimersAsync()
    await flushPromises()

    const recropBtn = wrapper.findAll('button').find((b) => b.text().includes('重新框选'))!
    await recropBtn.trigger('click')
    await flushPromises()

    // 上传成功，返回新 key。
    httpMethods.post.mockResolvedValueOnce(ok({ key: 'geometry/cropped_123.jpg' }))
    const cropper = wrapper.findComponent({ name: 'GeometryCropper' })
    cropper.vm.$emit('confirm', new File(['crop'], 'geometry.jpg', { type: 'image/jpeg' }))
    await flushPromises()
    await vi.runAllTimersAsync()
    await flushPromises()

    // 弹窗关闭 + 错误清空。
    const cropperProps = wrapper.findComponent({ name: 'GeometryCropper' }).props()
    expect(cropperProps.open).toBe(false)
    expect(cropperProps.error).toBe('')
  })

  it('点击「擦除手写」调用擦除接口并替换几何图 key', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const task: RecognitionTask = {
      id: 10,
      image_id: 1,
      status: 'success',
      progress: 100,
      provider: 'mock',
      result_json: JSON.stringify({
        stem_text: '几何题',
        formula: { latex: '', raw_text: '' },
        geometry: { shape_type: 'triangle', properties: {}, description: '三角形' },
        geometry_keys: ['geometry/task_10.jpg'],
      }),
    }
    httpMethods.post.mockResolvedValueOnce(ok({ ...task, status: 'pending' }))
    httpMethods.get.mockResolvedValue(ok(task))

    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await vi.runAllTimersAsync()
    await flushPromises()

    // 擦除按钮存在。
    const eraseBtn = wrapper.findAll('button').find((b) => b.text().includes('擦除手写'))!
    expect(eraseBtn).toBeDefined()

    // 点击擦除：POST /recognition/erase 返回新 key。
    httpMethods.post.mockResolvedValueOnce(ok({ key: 'erased/123.jpg' }))
    await eraseBtn.trigger('click')
    await vi.runAllTimersAsync()
    await flushPromises()

    expect(httpMethods.post).toHaveBeenCalledWith(
      '/recognition/erase',
      { key: 'geometry/task_10.jpg' },
      { timeout: 120000 },
    )
    // 几何图预览更新为擦除后的 key。
    expect(wrapper.html()).toContain('/api/recognition/files/erased/123.jpg')
  })

  it('擦除手写失败时展示错误提示', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const task: RecognitionTask = {
      id: 11,
      image_id: 1,
      status: 'success',
      progress: 100,
      provider: 'mock',
      result_json: JSON.stringify({
        stem_text: '几何题',
        formula: { latex: '', raw_text: '' },
        geometry: { shape_type: 'triangle', properties: {}, description: '三角形' },
        geometry_keys: ['geometry/task_11.jpg'],
      }),
    }
    httpMethods.post.mockResolvedValueOnce(ok({ ...task, status: 'pending' }))
    httpMethods.get.mockResolvedValue(ok(task))

    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await vi.runAllTimersAsync()
    await flushPromises()

    const eraseBtn = wrapper.findAll('button').find((b) => b.text().includes('擦除手写'))!
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    httpMethods.post.mockRejectedValueOnce(new Error('erase fail'))
    await eraseBtn.trigger('click')
    await vi.runAllTimersAsync()
    await flushPromises()

    expect(wrapper.text()).toContain('擦除失败，请重试')
    errSpy.mockRestore()
  })
})
