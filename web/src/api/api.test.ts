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
  exportMistakes,
  parseContentDisposition,
  sendSmsCode,
  smsLogin,
  sendEmailCode,
  emailRegister,
  passwordLogin,
  resetPassword,
  authRequestInterceptor,
  authResponseErrorInterceptor,
  type ApiResponse,
  type RecognitionTask,
  type Category,
} from './index'
import { logout, TOKEN_KEY } from '../composables/useAuth'

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
    create: () => ({
      ...httpMethods,
      interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
      defaults: { headers: { common: {} } },
    }),
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

  it('listMistakes 透传分页与筛选参数，并返回分面计数', async () => {
    httpMethods.get.mockResolvedValueOnce(
      ok({
        items: [{ id: 1, question_id: 2, user_id: 1 }],
        total: 7,
        subject_counts: { 数学: 5 },
        source_counts: { 期中考试: 7 },
      }),
    )
    const res = await listMistakes({
      offset: 20,
      limit: 20,
      keyword: ' 函数 ',
      subject: '数学',
      source: '期中考试',
    })
    expect(httpMethods.get).toHaveBeenCalledWith('/mistakes', {
      params: { offset: 20, limit: 20, keyword: '函数', subject: '数学', source: '期中考试' },
    })
    expect(res.items).toHaveLength(1)
    expect(res.total).toBe(7)
    expect(res.subjectCounts).toEqual({ 数学: 5 })
    expect(res.sourceCounts).toEqual({ 期中考试: 7 })

    // 缺省参数：offset=0、limit=20；空白/空的筛选条件不发送。
    httpMethods.get.mockResolvedValueOnce(ok({ items: [], total: 0 }))
    await listMistakes({ keyword: '   ', subject: '', source: '' })
    expect(httpMethods.get).toHaveBeenLastCalledWith('/mistakes', { params: { offset: 0, limit: 20 } })

    // 兜底：data 为 null 时返回空列表、0 与空分面。
    httpMethods.get.mockResolvedValueOnce({ data: { code: 0, message: 'ok', data: null } })
    const empty = await listMistakes()
    expect(empty).toEqual({ items: [], total: 0, subjectCounts: {}, sourceCounts: {} })
  })

  it('exportMistakes 以 blob 方式请求并解析服务端文件名', async () => {
    const blob = new Blob(['pdf-bytes'], { type: 'application/pdf' })
    httpMethods.post.mockResolvedValueOnce({
      data: blob,
      headers: {
        'content-disposition': `attachment; filename="mistakes.pdf"; filename*=UTF-8''${encodeURIComponent(
          '我的错题本 2026-09-17.pdf',
        )}`,
      },
    })

    const res = await exportMistakes([1, 2], 'pdf')

    expect(httpMethods.post).toHaveBeenCalledWith(
      '/mistakes/export',
      { ids: [1, 2], format: 'pdf' },
      { responseType: 'blob', timeout: 120000 },
    )
    expect(res.blob).toBe(blob)
    expect(res.filename).toBe('我的错题本 2026-09-17.pdf')
  })

  it('exportMistakes 缺少响应头时回退默认文件名', async () => {
    httpMethods.post.mockResolvedValueOnce({ data: new Blob(['x']), headers: {} })

    const res = await exportMistakes([1], 'docx')
    expect(res.filename).toBe('我的错题本.docx')
  })

  it('parseContentDisposition 优先 UTF-8 文件名，兼容 ASCII 与异常输入', () => {
    const utf8 = `attachment; filename="m.pdf"; filename*=UTF-8''${encodeURIComponent('错题本.pdf')}`
    expect(parseContentDisposition(utf8)).toBe('错题本.pdf')
    expect(parseContentDisposition('attachment; filename="m.pdf"')).toBe('m.pdf')
    expect(parseContentDisposition(undefined)).toBeNull()
    // 非法百分号编码不应抛出异常。
    expect(parseContentDisposition("attachment; filename*=UTF-8''%E4%B8")).toBeNull()
  })

  it('sendEmailCode 携带邮箱与用途，返回开发模式验证码', async () => {
    httpMethods.post.mockResolvedValueOnce(ok({ debug_code: '246810' }))

    const res = await sendEmailCode('stu@peak.local', 'register')

    expect(httpMethods.post).toHaveBeenCalledWith('/users/auth/email/code', {
      email: 'stu@peak.local', purpose: 'register',
    })
    expect(res.debugCode).toBe('246810')
  })

  it('emailRegister 提交邮箱、密码与验证码并返回令牌', async () => {
    httpMethods.post.mockResolvedValueOnce(ok({ token: 'jwt-reg', user: { id: 3 } }))

    const res = await emailRegister('stu@peak.local', 'password123', '135790')

    expect(httpMethods.post).toHaveBeenCalledWith('/users/auth/email/register', {
      email: 'stu@peak.local', password: 'password123', code: '135790',
    })
    expect(res.token).toBe('jwt-reg')
  })

  it('passwordLogin 提交账号与密码并返回令牌', async () => {
    httpMethods.post.mockResolvedValueOnce(ok({ token: 'jwt-login', user: { id: 4 } }))

    const res = await passwordLogin('13800001234', 'password123')

    expect(httpMethods.post).toHaveBeenCalledWith('/users/auth/password/login', {
      account: '13800001234', password: 'password123',
    })
    expect(res.token).toBe('jwt-login')
  })

  it('resetPassword 提交邮箱、验证码与新密码', async () => {
    httpMethods.post.mockResolvedValueOnce(ok({ reset: true }))

    await expect(resetPassword('stu@peak.local', '246810', 'new-password')).resolves.toBeUndefined()

    expect(httpMethods.post).toHaveBeenCalledWith('/users/auth/password/reset', {
      email: 'stu@peak.local', code: '246810', password: 'new-password',
    })
  })
})

describe('api/index.ts 鉴权拦截器', () => {
  beforeEach(() => {
    logout()
  })

  it('authRequestInterceptor 为请求注入 Bearer 令牌', () => {
    localStorage.setItem(TOKEN_KEY, 'jwt-abc')
    const config = { headers: {} as Record<string, string> }
    const res = authRequestInterceptor(config as never)
    expect((res.headers as Record<string, string>).Authorization).toBe('Bearer jwt-abc')
  })

  it('authRequestInterceptor 未登录时不注入 Authorization', () => {
    const config = { headers: {} as Record<string, string> }
    const res = authRequestInterceptor(config as never)
    expect((res.headers as Record<string, string>).Authorization).toBeUndefined()
  })

  it('authResponseErrorInterceptor 对 401 清除登录态并跳转登录页', async () => {
    localStorage.setItem(TOKEN_KEY, 'jwt-abc')
    const err = { response: { status: 401 } }
    await expect(authResponseErrorInterceptor(err)).rejects.toBe(err)
    expect(localStorage.getItem(TOKEN_KEY)).toBeNull()
  })

  it('authResponseErrorInterceptor 对其他错误原样透传', async () => {
    localStorage.setItem(TOKEN_KEY, 'jwt-abc')
    const err = { response: { status: 500 } }
    await expect(authResponseErrorInterceptor(err)).rejects.toBe(err)
    expect(localStorage.getItem(TOKEN_KEY)).toBe('jwt-abc')
  })
})
