import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  acceptConsent,
  buildSummaryView,
  cancelDeletion,
  createItem,
  createMember,
  deleteItem,
  deleteMember,
  errorMessage,
  exportLedger,
  getConsent,
  getDeletion,
  getMeta,
  getSummary,
  isReauthRequired,
  listItems,
  listMembers,
  listSnapshots,
  parseFilename,
  purgeNow,
  renameMember,
  requestDeletion,
  updateItem,
  updateMember,
  type LedgerSummary,
} from './ledger'

const { httpGet, httpPost, httpPut, httpDelete } = vi.hoisted(() => ({
  httpGet: vi.fn(),
  httpPost: vi.fn(),
  httpPut: vi.fn(),
  httpDelete: vi.fn(),
}))

vi.mock('./index', () => ({
  http: { get: httpGet, post: httpPost, put: httpPut, delete: httpDelete },
}))

function ok<T>(data: T) {
  return { data: { code: 0, message: 'ok', data } }
}

const summaryFixture: LedgerSummary = {
  groups: [
    {
      category: 'liquid',
      label: '流动资金',
      kind: 'asset',
      color: '#E9A66B',
      amount_cents: 1_000_00,
      count: 2,
      children: [{ key: 'bank_current', label: '银行活期', amount_cents: 1_000_00, count: 2, is_fixed: false }],
    },
    {
      category: 'fixed',
      label: '固定资产',
      kind: 'asset',
      color: '#9DBCC4',
      amount_cents: 5_000_00,
      count: 1,
      children: [{ key: 'house_self', label: '房产（自住）', amount_cents: 5_000_00, count: 1, is_fixed: true }],
    },
    {
      category: 'liability',
      label: '负债',
      kind: 'liability',
      color: '#B9B4C7',
      amount_cents: 2_000_00,
      count: 1,
      children: [{ key: 'mortgage', label: '房屋贷款', amount_cents: 2_000_00, count: 1, is_fixed: false }],
    },
  ],
  total_asset_cents: 6_000_00,
  total_liability_cents: 2_000_00,
  net_asset_cents: 4_000_00,
  item_count: 4,
  updated_at: '2026-10-01T10:00:00Z',
  has_foreign: false,
  consent_required: false,
}

beforeEach(() => {
  httpGet.mockReset()
  httpPost.mockReset()
  httpPut.mockReset()
  httpDelete.mockReset()
})

describe('元数据与聚合', () => {
  it('getMeta / getSummary 解包统一响应', async () => {
    httpGet.mockResolvedValueOnce(ok({ categories: [], risk_types: [], currencies: [], consent_policy_version: 'v1', consent_required: true }))
    await expect(getMeta()).resolves.toMatchObject({ consent_policy_version: 'v1', consent_required: true })

    httpGet.mockResolvedValueOnce(ok(summaryFixture))
    await expect(getSummary()).resolves.toMatchObject({ net_asset_cents: 400_000 })
    expect(httpGet).toHaveBeenNthCalledWith(2, '/ledger/summary')
  })

  it('getSummary 按成员范围透传 member_id（self 表示「我」）', async () => {
    httpGet.mockResolvedValue(ok(summaryFixture))
    await getSummary('self')
    expect(httpGet).toHaveBeenLastCalledWith('/ledger/summary', { params: { member_id: 'self' } })
    await getSummary('12')
    expect(httpGet).toHaveBeenLastCalledWith('/ledger/summary', { params: { member_id: '12' } })
  })

  it('buildSummaryView 不排除固定资产时保留全部分类', () => {
    const view = buildSummaryView(summaryFixture, false)
    expect(view.groups).toHaveLength(3)
    expect(view.totalAssetCents).toBe(600_000)
    expect(view.totalLiabilityCents).toBe(200_000)
    expect(view.netAssetCents).toBe(400_000)
    expect(view.itemCount).toBe(4)
  })

  it('buildSummaryView 排除固定资产后重算总量与条目数', () => {
    const view = buildSummaryView(summaryFixture, true)
    expect(view.groups.map((g) => g.category)).toEqual(['liquid', 'liability'])
    expect(view.totalAssetCents).toBe(100_000)
    expect(view.totalLiabilityCents).toBe(200_000)
    // 排除固定资产后负债率可超过 100%，不做截断。
    expect(view.netAssetCents).toBe(-100_000)
    expect(view.itemCount).toBe(3)
    expect(view.excludeFixed).toBe(true)
  })
})

describe('条目接口', () => {
  it('listItems 默认分页并剔除空关键词', async () => {
    httpGet.mockResolvedValueOnce(ok({ items: [], total: 0, sum_cents: 0, category_counts: {} }))
    await listItems({ keyword: '   ', category: 'fixed' })
    expect(httpGet).toHaveBeenCalledWith('/ledger/items', {
      params: { offset: 0, limit: 20, category: 'fixed' },
    })
  })

  it('listItems 关键词去空格后透传，payload 缺失时回落默认值', async () => {
    httpGet.mockResolvedValueOnce({ data: { code: 0, message: 'ok' } })
    const result = await listItems({ keyword: ' 招行 ', offset: 10, limit: 5 })
    expect(httpGet).toHaveBeenCalledWith('/ledger/items', {
      params: { offset: 10, limit: 5, keyword: '招行' },
    })
    expect(result).toEqual({ items: [], total: 0, sum_cents: 0, category_counts: {} })
  })

  it('创建/更新/删除条目', async () => {
    httpPost.mockResolvedValueOnce(ok({ id: 7, version: 1 }))
    await createItem({
      name: '招行活期',
      category: 'liquid',
      sub_type: 'bank_current',
      amount_cents: 100,
      currency: 'CNY',
      risk_type: 'cash',
      member_id: null,
      note: '',
    })
    expect(httpPost).toHaveBeenCalledWith('/ledger/items', expect.objectContaining({ name: '招行活期' }))

    httpPut.mockResolvedValueOnce(ok({ id: 7, version: 2 }))
    await updateItem(7, {
      name: '招行活期',
      category: 'liquid',
      sub_type: 'bank_current',
      amount_cents: 200,
      currency: 'CNY',
      risk_type: 'cash',
      member_id: null,
      note: '',
    }, 1)
    expect(httpPut).toHaveBeenCalledWith('/ledger/items/7', expect.objectContaining({ version: 1, amount_cents: 200 }))

    httpDelete.mockResolvedValueOnce({})
    await deleteItem(7)
    expect(httpDelete).toHaveBeenCalledWith('/ledger/items/7')
  })
})

describe('成员与快照', () => {
  it('成员增删改查', async () => {
    httpGet.mockResolvedValueOnce(ok({ items: [{ id: 1, name: '配偶', sort_order: 1 }] }))
    await expect(listMembers()).resolves.toHaveLength(1)

    httpPost.mockResolvedValueOnce(ok({ id: 2, name: '孩子', sort_order: 2, visible: true }))
    await expect(createMember('孩子')).resolves.toMatchObject({ id: 2 })
    expect(httpPost).toHaveBeenCalledWith('/ledger/members', { name: '孩子', visible: true })

    httpPut.mockResolvedValueOnce(ok({ id: 2, name: '女儿', sort_order: 2, visible: true }))
    await expect(renameMember(2, '女儿')).resolves.toMatchObject({ name: '女儿' })

    httpDelete.mockResolvedValueOnce({})
    await deleteMember(2)
    expect(httpDelete).toHaveBeenCalledWith('/ledger/members/2')
  })

  it('更新成员显示状态', async () => {
    httpPut.mockResolvedValueOnce(ok({ id: 3, name: '配偶', sort_order: 1, visible: false }))
    await expect(updateMember(3, { visible: false })).resolves.toMatchObject({ visible: false })
    expect(httpPut).toHaveBeenCalledWith('/ledger/members/3', { visible: false })
  })

  it('成员载荷缺失时返回空数组', async () => {
    httpGet.mockResolvedValueOnce({ data: { code: 0, message: 'ok' } })
    await expect(listMembers()).resolves.toEqual([])
  })

  it('按月拉取快照', async () => {
    httpGet.mockResolvedValueOnce(ok({ items: [{ period: '2026-01', payload: {}, updated_at: '' }] }))
    const result = await listSnapshots('2026')
    expect(httpGet).toHaveBeenCalledWith('/ledger/snapshots', { params: { year: '2026' } })
    expect(result).toHaveLength(1)
  })
})

describe('隐私接口', () => {
  it('读取与记录同意', async () => {
    httpGet.mockResolvedValueOnce(ok({ policy_version: 'v1', required: true, consented_at: null }))
    await expect(getConsent()).resolves.toMatchObject({ required: true })

    httpPost.mockResolvedValueOnce(ok({ policy_version: 'v1', required: false, consented_at: '2026-10-01T00:00:00Z' }))
    await expect(acceptConsent('v1')).resolves.toMatchObject({ required: false })
    expect(httpPost).toHaveBeenCalledWith('/ledger/privacy/consent', { policy_version: 'v1' })
  })

  it('导出返回 blob 与文件名', async () => {
    httpGet.mockResolvedValueOnce({
      data: new Blob(['{}']),
      headers: { 'content-disposition': "attachment; filename*=UTF-8''%E5%AE%B6%E5%BA%AD%E8%B4%A6%E6%9C%AC.json" },
    })
    const result = await exportLedger()
    expect(result.filename).toBe('家庭账本.json')
    expect(result.blob.size).toBeGreaterThan(0)
  })

  it('注销状态查询、申请、撤销与立即清除', async () => {
    httpGet.mockResolvedValueOnce(ok({ status: 'none', requested_at: null, execute_after: null, days_left: 0 }))
    await expect(getDeletion()).resolves.toMatchObject({ status: 'none' })

    httpPost.mockResolvedValueOnce(ok({ status: 'pending', days_left: 30 }))
    await expect(requestDeletion()).resolves.toMatchObject({ status: 'pending' })

    httpDelete.mockResolvedValueOnce(ok({ status: 'cancelled', days_left: 0 }))
    await expect(cancelDeletion()).resolves.toMatchObject({ status: 'cancelled' })

    httpPost.mockResolvedValueOnce({})
    await purgeNow()
    expect(httpPost).toHaveBeenCalledWith('/ledger/privacy/deletion/purge')
  })
})

describe('错误处理辅助', () => {
  it('isReauthRequired 只认 428', () => {
    expect(isReauthRequired({ response: { status: 428 } })).toBe(true)
    expect(isReauthRequired({ response: { status: 403 } })).toBe(false)
    expect(isReauthRequired(new Error('x'))).toBe(false)
  })

  it('errorMessage 优先使用服务端消息', () => {
    expect(errorMessage({ response: { data: { message: '请先同意告知' } } })).toBe('请先同意告知')
    expect(errorMessage(new Error('boom'), '兜底')).toBe('兜底')
  })

  it('parseFilename 兼容 RFC 5987 与普通写法', () => {
    expect(parseFilename("attachment; filename*=UTF-8''%E5%AF%BC%E5%87%BA.json")).toBe('导出.json')
    expect(parseFilename('attachment; filename="ledger.json"')).toBe('ledger.json')
    expect(parseFilename("attachment; filename*=UTF-8''%E0%A4%A")).toBeNull()
    expect(parseFilename(undefined)).toBeNull()
  })
})
