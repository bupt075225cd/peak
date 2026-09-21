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
    // 页面不再展示"识别公式"区块。
    expect(wrapper.text()).not.toContain('识别公式')
    // 学科、题型由识别结果自动回填并只读展示
    expect(wrapper.text()).toContain('物理')
    expect(wrapper.text()).toContain('选择题')
    // 未选择年级时保存按钮禁用（年级必填）
    expect((getSaveBtn(wrapper).element as HTMLButtonElement).disabled).toBe(true)
    // 选择年级后保存按钮可用
    const gradeSelect = wrapper.findAll('select')[0]
    await gradeSelect.setValue('七年级上')
    // 来源必填：一起填上，否则保存按钮保持禁用。
    await wrapper.find('[data-testid="source-input"]').setValue('期中考试')
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
    // 来源必填：一起填上，否则保存按钮保持禁用。
    await wrapper.find('[data-testid="source-input"]').setValue('期中考试')
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
        source: '期中考试',
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
    // 来源必填：一起填上，否则保存按钮保持禁用。
    await wrapper.find('[data-testid="source-input"]').setValue('期中考试')
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

  // 识别上传的公共准备：先返回 pending，随后轮询到给定 success 结果。
  async function recognizeWithResult(wrapper: ReturnType<typeof mount>, task: RecognitionTask) {
    httpMethods.post.mockResolvedValueOnce(ok({ ...task, status: 'pending' }))
    httpMethods.get.mockResolvedValue(ok(task))
    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await vi.runAllTimersAsync()
    await flushPromises()
  }

  function mathGeoResult(id: number, extra: Record<string, unknown> = {}): RecognitionTask {
    return {
      id,
      image_id: 1,
      status: 'success',
      progress: 100,
      provider: 'mock',
      result_json: JSON.stringify({
        stem_text: '几何题',
        subject: '数学',
        question_type: '解答题',
        formula: { latex: '', raw_text: '' },
        geometry: { shape_type: 'triangle', properties: {}, description: '三角形' },
        geometry_keys: [`geometry/task_${id}.jpg`],
        ...extra,
      }),
    }
  }

  it('数学题含多个几何子图时逐张展示重绘 SVG（每子图一张，无手动操作入口）', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const task = mathGeoResult(5, {
      redraw_svg_keys: ['geometry/task_5.svg', 'geometry/task_5_2.svg'],
      redraw_report: { max_hard: 0.0001, max_soft: 0.01, attempts: 1, consistent: true },
    })
    await recognizeWithResult(wrapper, task)

    // 每张子图都单独展示。
    await vi.waitFor(() => {
      expect(wrapper.html()).toContain('/api/recognition/files/geometry/task_5.svg')
      expect(wrapper.html()).toContain('/api/recognition/files/geometry/task_5_2.svg')
    })
    expect(wrapper.text()).toContain('2 张')
    // 页面不再有手动框选/擦除入口，也不展示原始裁剪图预览。
    expect(wrapper.text()).not.toContain('重新框选')
    expect(wrapper.text()).not.toContain('擦除手写')
  })

  it('识别结果无重绘 key 时不展示重绘区域，保存 image 为空数组', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    // 未产出重绘（无侧车/非几何）：不含 redraw_svg_keys。
    const task = mathGeoResult(6)
    await recognizeWithResult(wrapper, task)
    await flushPromises()

    expect(wrapper.text()).not.toContain('几何图形')

    const gradeSelect = wrapper.findAll('select')[0]
    await gradeSelect.setValue('七年级上')
    // 来源必填：一起填上，否则保存按钮保持禁用。
    await wrapper.find('[data-testid="source-input"]').setValue('期中考试')
    await flushPromises()
    httpMethods.post.mockResolvedValueOnce(ok({ id: 42 }))
    httpMethods.post.mockResolvedValueOnce({ data: { code: 0, message: 'ok', data: { id: 1 } } })

    await getSaveBtn(wrapper).trigger('click')
    await vi.waitFor(() => {
      expect(httpMethods.post).toHaveBeenCalledWith('/questions', expect.objectContaining({
        image: JSON.stringify([]),
      }))
    })
  })

  it('数学题含几何图保存时 image 存全部重绘 SVG key', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const task = mathGeoResult(7, {
      redraw_svg_keys: ['geometry/task_7.svg', 'geometry/task_7_2.svg', 'geometry/task_7_3.svg'],
      redraw_report: { max_hard: 0.0001, max_soft: 0.01, attempts: 1, consistent: true },
    })
    await recognizeWithResult(wrapper, task)
    await vi.waitFor(() => {
      expect(wrapper.html()).toContain('/api/recognition/files/geometry/task_7_3.svg')
    })

    // 选年级 → 保存。
    const gradeSelect = wrapper.findAll('select')[0]
    await gradeSelect.setValue('七年级上')
    // 来源必填：一起填上，否则保存按钮保持禁用。
    await wrapper.find('[data-testid="source-input"]').setValue('期中考试')
    await flushPromises()
    httpMethods.post.mockResolvedValueOnce(ok({ id: 42 }))
    httpMethods.post.mockResolvedValueOnce({ data: { code: 0, message: 'ok', data: { id: 1 } } })

    await getSaveBtn(wrapper).trigger('click')
    await vi.waitFor(() => {
      expect(httpMethods.post).toHaveBeenCalledWith('/questions', expect.objectContaining({
        image: JSON.stringify(['geometry/task_7.svg', 'geometry/task_7_2.svg', 'geometry/task_7_3.svg']),
      }))
    })
  })

  it('非数学题不会产出重绘 key，页面不展示重绘区域', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    const task: RecognitionTask = {
      id: 9,
      image_id: 1,
      status: 'success',
      progress: 100,
      provider: 'mock',
      result_json: JSON.stringify({
        stem_text: '物理几何题',
        subject: '物理',
        question_type: '解答题',
        formula: { latex: '', raw_text: '' },
        geometry: { shape_type: 'triangle', properties: {}, description: '三角形' },
      }),
    }
    await recognizeWithResult(wrapper, task)
    await flushPromises()

    expect(wrapper.text()).not.toContain('几何图形')
    expect(wrapper.text()).not.toContain('重绘')
  })

  it('来源必填：未填写（或只填空白）时保存按钮禁用', async () => {
    const router = buildRouter()
    router.push('/entry')
    await router.isReady()
    const wrapper = mount(MistakeEntry, { global: { plugins: [router] } })
    await flushPromises()

    await recognizeWithResult(wrapper, mathGeoResult(8))

    const sourceInput = wrapper.find('[data-testid="source-input"]')
    expect(sourceInput.exists()).toBe(true)
    expect(wrapper.text()).toContain('来源')

    // 题干与年级都就绪，但来源为空 → 仍禁用。
    await wrapper.findAll('select')[0].setValue('七年级上')
    await flushPromises()
    expect((getSaveBtn(wrapper).element as HTMLButtonElement).disabled).toBe(true)

    // 只填空白同样视为未填写。
    await sourceInput.setValue('   ')
    await flushPromises()
    expect((getSaveBtn(wrapper).element as HTMLButtonElement).disabled).toBe(true)

    await sourceInput.setValue('期中考试')
    await flushPromises()
    expect((getSaveBtn(wrapper).element as HTMLButtonElement).disabled).toBe(false)
  })
})
