import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  uploadImage,
  uploadDocument,
  isDocument,
  getTask,
  retryTask,
  listCategories,
  createQuestion,
  createMistake,
  listMistakes,
  type ApiResponse,
  type RecognitionTask,
  type Category,
} from './index'

// 模块加载时 api/index.ts 会调用 axios.create() 创建 http 实例，
// 因此必须在 vi.mock 工厂里同步提供 create 的返回，确保加载即可用。
// 用 vi.hoisted 保证 httpMethods 在 hoisted 的 mock 工厂之前初始化。
const { httpMethods } = vi.hoisted(() => ({
  httpMethods: {
    post: vi.fn(),
    get: vi.fn(),
  },
}))

vi.mock('axios', () => ({
  default: {
    create: () => ({ ...httpMethods, defaults: { headers: { common: {} } } }),
  },
}))

beforeEach(() => {
  httpMethods.post.mockReset()
  httpMethods.get.mockReset()
})

function ok<T>(data: T): { data: ApiResponse<T> } {
  return { data: { code: 0, message: 'ok', data } }
}

describe('api/index.ts', () => {
  it('uploadImage 发送 FormData 并返回 task', async () => {
    const task: RecognitionTask = {
      id: 7,
      image_id: 1,
      status: 'pending',
      progress: 0,
      provider: 'mock',
    }
    httpMethods.post.mockResolvedValueOnce(ok(task))

    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const res = await uploadImage(file)

    expect(httpMethods.post).toHaveBeenCalledWith(
      '/recognition/tasks',
      expect.any(FormData),
    )
    expect(res.id).toBe(7)
    expect(res.provider).toBe('mock')
  })

  it('uploadDocument 发送 document 字段并返回 task', async () => {
    const task: RecognitionTask = {
      id: 8,
      image_id: 1,
      status: 'pending',
      progress: 0,
      provider: 'mock',
    }
    httpMethods.post.mockResolvedValueOnce(ok(task))

    const file = new File(['x'], 'a.docx', { type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document' })
    const res = await uploadDocument(file)

    expect(httpMethods.post).toHaveBeenCalledWith(
      '/recognition/tasks',
      expect.any(FormData),
    )
    expect(res.id).toBe(8)
  })

  it('isDocument 正确判断文档类型', () => {
    expect(isDocument(new File(['x'], 'a.docx'))).toBe(true)
    expect(isDocument(new File(['x'], 'a.pdf'))).toBe(true)
    expect(isDocument(new File(['x'], 'a.PDF'))).toBe(true)
    expect(isDocument(new File(['x'], 'a.png'))).toBe(false)
    expect(isDocument(new File(['x'], 'a.jpg'))).toBe(false)
  })

  it('getTask 返回对应任务', async () => {
    const task: RecognitionTask = {
      id: 9,
      image_id: 2,
      status: 'success',
      progress: 100,
      provider: 'aliyun',
    }
    httpMethods.get.mockResolvedValueOnce(ok(task))

    const res = await getTask(9)
    expect(httpMethods.get).toHaveBeenCalledWith('/recognition/tasks/9')
    expect(res.id).toBe(9)
    expect(res.status).toBe('success')
  })

  it('retryTask 调用重试接口且不返回数据', async () => {
    httpMethods.post.mockResolvedValueOnce({ data: { code: 0, message: 'ok' } })
    await expect(retryTask(9)).resolves.toBeUndefined()
    expect(httpMethods.post).toHaveBeenCalledWith('/recognition/tasks/9/retry')
  })

  it('listCategories 透传 type 参数并在无数据时兜底空数组', async () => {
    const cats: Category[] = [
      { id: 1, parent_id: null, name: '数学', type: 'subject', sort_order: 1 },
    ]
    httpMethods.get.mockResolvedValueOnce(ok(cats))
    const res = await listCategories('subject')
    expect(httpMethods.get).toHaveBeenCalledWith('/categories', { params: { type: 'subject' } })
    expect(res).toHaveLength(1)

    // 兜底：data 为 null 时返回 []
    httpMethods.get.mockResolvedValueOnce({ data: { code: 0, message: 'ok', data: null } })
    const empty = await listCategories()
    expect(empty).toEqual([])
  })

  it('createMistake 返回 data 字段', async () => {
    httpMethods.post.mockResolvedValueOnce({ data: { code: 0, message: 'ok', data: { id: 42 } } })
    const res = await createMistake({ question_id: 7, user_id: 1 })
    expect(httpMethods.post).toHaveBeenCalledWith('/mistakes', { question_id: 7, user_id: 1 })
    expect(res).toEqual({ id: 42 })
  })

  it('createQuestion 提交到 /questions 并返回题目', async () => {
    httpMethods.post.mockResolvedValueOnce({ data: { code: 0, message: 'ok', data: { id: 7, subject: '数学' } } })
    const res = await createQuestion({ subject: '数学', stem_text: '1+1=?' })
    expect(httpMethods.post).toHaveBeenCalledWith('/questions', { subject: '数学', stem_text: '1+1=?' })
    expect(res.id).toBe(7)
  })

  it('listMistakes 返回 items 并在无数据时兜底空数组', async () => {
    httpMethods.get.mockResolvedValueOnce(ok({ items: [{ id: 1, question_id: 2, user_id: 1 }], total: 1 }))
    const res = await listMistakes()
    expect(httpMethods.get).toHaveBeenCalledWith('/mistakes')
    expect(res).toHaveLength(1)

    // 兜底：data 为 null 时返回 []
    httpMethods.get.mockResolvedValueOnce({ data: { code: 0, message: 'ok', data: null } })
    const empty = await listMistakes()
    expect(empty).toEqual([])
  })
})
