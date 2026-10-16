import { describe, it, expect, vi, beforeEach } from 'vitest'

const { httpMethods } = vi.hoisted(() => ({
  httpMethods: { get: vi.fn(), post: vi.fn() },
}))

// 模拟共享 axios 实例（./index），仅验证 poetry.ts 的参数拼装与解包逻辑。
vi.mock('./index', () => ({ http: httpMethods }))

import {
  listPoems,
  getPoem,
  getRelatedPoems,
  getDynastyStats,
  getVolumeStats,
  getAuthorStats,
  getWordStats,
  sendChat,
} from './poetry'

function ok<T>(data: T) {
  return { data: { code: 0, message: 'ok', data } }
}

beforeEach(() => {
  httpMethods.get.mockReset()
  httpMethods.post.mockReset()
})

describe('api/poetry.ts', () => {
  it('listPoems 默认分页并返回列表', async () => {
    httpMethods.get.mockResolvedValueOnce(ok({ items: [{ id: 1 }], total: 1 }))
    const result = await listPoems()
    expect(httpMethods.get).toHaveBeenCalledWith('/poetry/pieces', {
      params: { offset: 0, limit: 20 },
    })
    expect(result.total).toBe(1)
  })

  it('listPoems 携带筛选条件并修剪关键词空白', async () => {
    httpMethods.get.mockResolvedValueOnce(ok({ items: [], total: 0 }))
    await listPoems({
      dynasty: '唐',
      author: '李白',
      volume: '八年级上册',
      category: '课内',
      keyword: '  明月  ',
    })
    expect(httpMethods.get).toHaveBeenCalledWith('/poetry/pieces', {
      params: {
        offset: 0,
        limit: 20,
        dynasty: '唐',
        author: '李白',
        volume: '八年级上册',
        category: '课内',
        keyword: '明月',
      },
    })
  })

  it('listPoems 空关键词不作为查询参数下发', async () => {
    httpMethods.get.mockResolvedValueOnce(ok({ items: [], total: 0 }))
    await listPoems({ keyword: '   ' })
    const call = httpMethods.get.mock.calls[0] as unknown[]
    expect((call[1] as { params: Record<string, unknown> }).params.keyword).toBeUndefined()
  })

  it('getPoem 返回详情，getRelatedPoems 空值兜底为数组', async () => {
    httpMethods.get.mockResolvedValueOnce(ok({ id: 7, title: '爱莲说' }))
    expect(await getPoem(7)).toMatchObject({ id: 7, title: '爱莲说' })
    expect(httpMethods.get).toHaveBeenCalledWith('/poetry/pieces/7')

    httpMethods.get.mockResolvedValueOnce(ok(null))
    expect(await getRelatedPoems(7)).toEqual([])
  })

  it('统计接口返回数组（空值兜底）', async () => {
    httpMethods.get.mockResolvedValueOnce(ok([{ dynasty: '唐', order: 5, count: 30 }]))
    expect(await getDynastyStats()).toHaveLength(1)
    expect(httpMethods.get).toHaveBeenCalledWith('/poetry/stats/dynasties')

    httpMethods.get.mockResolvedValueOnce(ok([{ volume: '八年级上册', inclass: 10, outline: 2, other: 0 }]))
    expect(await getVolumeStats()).toHaveLength(1)

    httpMethods.get.mockResolvedValueOnce(ok([{ author: '李白', count: 8 }]))
    expect(await getAuthorStats(5)).toHaveLength(1)
    expect(httpMethods.get).toHaveBeenCalledWith('/poetry/stats/authors', { params: { top: 5 } })

    httpMethods.get.mockResolvedValueOnce(ok(null))
    const words = await getWordStats(50)
    expect(words).toEqual({ items: [], total_chars: 0 })
  })

  it('sendChat 返回对话响应，data 缺失时兜底提示', async () => {
    httpMethods.post.mockResolvedValueOnce(ok({ enabled: false, notice: '敬请期待' }))
    expect(await sendChat('问题')).toEqual({ enabled: false, notice: '敬请期待' })
    expect(httpMethods.post).toHaveBeenCalledWith('/poetry/chat', { question: '问题' })

    httpMethods.post.mockResolvedValueOnce(ok(null))
    const fallback = await sendChat('问题')
    expect(fallback.enabled).toBe(false)
    expect(fallback.notice).toContain('暂不可用')
  })
})
