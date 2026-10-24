import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import LedgerItems from './LedgerItems.vue'
import ReauthDialog from '../components/ledger/ReauthDialog.vue'
import { resetLedgerMetaCache } from '../composables/useLedgerMeta'
import { useLedgerPrivacy } from '../composables/useLedgerPrivacy'

const {
  getMeta,
  getConsent,
  acceptConsent,
  listMembers,
  listItems,
  createItem,
  updateItem,
  deleteItem,
  exportLedger,
  getDeletion,
  requestDeletion,
  cancelDeletion,
  downloadBlob,
  getMe,
  passwordLogin,
} = vi.hoisted(() => ({
  getMeta: vi.fn(),
  getConsent: vi.fn(),
  acceptConsent: vi.fn(),
  listMembers: vi.fn(),
  listItems: vi.fn(),
  createItem: vi.fn(),
  updateItem: vi.fn(),
  deleteItem: vi.fn(),
  exportLedger: vi.fn(),
  getDeletion: vi.fn(),
  requestDeletion: vi.fn(),
  cancelDeletion: vi.fn(),
  downloadBlob: vi.fn(),
  getMe: vi.fn(),
  passwordLogin: vi.fn(),
}))

vi.mock('../api/ledger', async () => {
  const actual = await vi.importActual<typeof import('../api/ledger')>('../api/ledger')
  return {
    ...actual,
    getMeta,
    getConsent,
    acceptConsent,
    listMembers,
    listItems,
    createItem,
    updateItem,
    deleteItem,
    exportLedger,
    getDeletion,
    requestDeletion,
    cancelDeletion,
  }
})

vi.mock('../utils/watermark', () => ({ downloadBlob }))
vi.mock('../api', () => ({ getMe, passwordLogin }))

const meta = {
  categories: [
    {
      key: 'liquid',
      label: '流动资金',
      kind: 'asset',
      color: '#E9A66B',
      sub_types: [{ key: 'bank_current', label: '银行活期', icon: 'Landmark', is_fixed: false }],
    },
    {
      key: 'fixed',
      label: '固定资产',
      kind: 'asset',
      color: '#9DBCC4',
      sub_types: [{ key: 'house_self', label: '房产（自住）', icon: 'House', is_fixed: true }],
    },
  ],
  risk_types: [{ key: 'none', label: '不标记' }],
  currencies: [{ key: 'CNY', label: '人民币', symbol: '¥' }],
  consent_policy_version: 'v1',
  consent_required: false,
}

const item = {
  id: 1,
  name: '招行活期',
  category: 'liquid',
  sub_type: 'bank_current',
  amount_cents: 1_000_000,
  currency: 'CNY',
  risk_type: 'none',
  member_id: null,
  member_name: '',
  note: '工资卡',
  version: 1,
  updated_at: '2026-10-01T10:00:00Z',
}

function listResult(overrides: Record<string, unknown> = {}) {
  return { items: [item], total: 1, sum_cents: 1_000_000, category_counts: { liquid: 1 }, ...overrides }
}

function buildRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/ledger/items', name: 'ledger-items', component: LedgerItems },
      { path: '/ledger', name: 'ledger-dashboard', component: { template: '<div />' } },
      { path: '/ledger/calendar', name: 'ledger-calendar', component: { template: '<div />' } },
      { path: '/ledger/composition', name: 'ledger-composition', component: { template: '<div />' } },
    ],
  })
}

async function mountView() {
  const router = buildRouter()
  router.push('/ledger/items')
  await router.isReady()
  const wrapper = mount(LedgerItems, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  resetLedgerMetaCache()
  sessionStorage.clear()
  useLedgerPrivacy().show()
  Object.values({ getMeta, getConsent, acceptConsent, listMembers, listItems, createItem, updateItem, deleteItem, exportLedger, getDeletion, requestDeletion, cancelDeletion, downloadBlob, getMe, passwordLogin }).forEach((fn) => fn.mockReset())

  getMeta.mockResolvedValue(meta)
  getConsent.mockResolvedValue({ policy_version: 'v1', required: false, consented_at: '2026-10-01T00:00:00Z' })
  listMembers.mockResolvedValue([{ id: 3, name: '配偶', sort_order: 1 }])
  listItems.mockResolvedValue(listResult())
  getDeletion.mockResolvedValue({ status: 'none', requested_at: null, execute_after: null, days_left: 0 })
  createItem.mockResolvedValue({ ...item, id: 9 })
  updateItem.mockResolvedValue({ ...item, version: 2 })
  deleteItem.mockResolvedValue(undefined)
  exportLedger.mockResolvedValue({ blob: new Blob(['{}']), filename: '账本.json' })
  requestDeletion.mockResolvedValue({
    status: 'pending',
    requested_at: '2026-10-23T00:00:00Z',
    execute_after: '2026-11-22T00:00:00Z',
    days_left: 30,
  })
  cancelDeletion.mockResolvedValue({ status: 'cancelled', requested_at: null, execute_after: null, days_left: 0 })
  getMe.mockResolvedValue({ id: 1, account: '13800001234', phone: '13800001234', email: null, name: '小明', email_verified: false })
})

describe('LedgerItems.vue 明细', () => {
  it('渲染条目、合计与分页信息', async () => {
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="item-row-1"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('招行活期')
    expect(wrapper.find('[data-test="items-sum"]').text()).toBe('10,000.00')
    expect(wrapper.text()).toContain('1 / 1')
  })

  it('分类筛选带 category 参数重新查询', async () => {
    const wrapper = await mountView()
    listItems.mockClear()
    await wrapper.find('[data-test="filter-fixed"]').trigger('click')
    await flushPromises()
    expect(listItems).toHaveBeenCalledWith(expect.objectContaining({ category: 'fixed', offset: 0, limit: 10 }))
  })

  it('选择成员后改为取回结果集在内存中过滤', async () => {
    const wrapper = await mountView()
    listItems.mockClear()
    listItems.mockResolvedValue(listResult())
    await wrapper.find('[data-test="member-filter"]').setValue('self')
    await flushPromises()
    expect(listItems).toHaveBeenCalledWith(expect.objectContaining({ offset: 0, limit: 500 }))
    expect(wrapper.find('[data-test="item-row-1"]').exists()).toBe(true)
  })

  it('删除需二次确认，确认后调用删除接口', async () => {
    const wrapper = await mountView()
    await wrapper.find('[data-test="remove-1"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('删除这条记录？')

    await wrapper.find('[data-test="confirm-action"]').trigger('click')
    await flushPromises()
    expect(deleteItem).toHaveBeenCalledWith(1)
    expect(wrapper.text()).not.toContain('删除这条记录？')
  })

  it('新增资产提交后刷新列表', async () => {
    const wrapper = await mountView()
    await wrapper.find('[data-test="add-item"]').trigger('click')
    await flushPromises()

    await wrapper.find('input[placeholder="如：招商银行活期"]').setValue('招行定期')
    await wrapper.find('input[placeholder="点击输入金额"]').setValue('5000')
    await wrapper.find('[data-test="ledger-submit"]').trigger('click')
    await flushPromises()

    expect(createItem).toHaveBeenCalledWith(expect.objectContaining({ name: '招行定期', amount_cents: 500000 }))
  })

  it('导出遇到 428 时弹出重认证，验证成功后自动重试', async () => {
    exportLedger.mockRejectedValueOnce({ response: { status: 428 } })
    const wrapper = await mountView()

    await wrapper.find('[data-test="export-data"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('重新验证身份')

    await wrapper.findComponent(ReauthDialog).vm.$emit('success')
    await flushPromises()

    expect(exportLedger).toHaveBeenCalledTimes(2)
    expect(downloadBlob).toHaveBeenCalledWith(expect.any(Blob), '账本.json')
  })

  it('申请注销后展示冷静期提示', async () => {
    const wrapper = await mountView()
    await wrapper.find('[data-test="open-deletion"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-test="request-deletion"]').trigger('click')
    await flushPromises()

    expect(requestDeletion).toHaveBeenCalled()
    expect(wrapper.find('[data-test="deletion-hint"]').text()).toContain('30 天')
  })

  it('冷静期提示条可直接撤销注销', async () => {
    getDeletion.mockResolvedValue({
      status: 'pending',
      requested_at: '2026-10-23T00:00:00Z',
      execute_after: '2026-11-22T00:00:00Z',
      days_left: 30,
    })
    const wrapper = await mountView()
    await flushPromises()

    const cancel = wrapper.find('[data-test="cancel-deletion"]')
    expect(cancel.exists()).toBe(true)
    await cancel.trigger('click')
    await flushPromises()

    expect(cancelDeletion).toHaveBeenCalled()
    expect(wrapper.find('[data-test="deletion-hint"]').exists()).toBe(false)
  })

  it('空列表时引导添加第一笔资产', async () => {
    listItems.mockResolvedValue(listResult({ items: [], total: 0, sum_cents: 0, category_counts: {} }))
    const wrapper = await mountView()
    expect(wrapper.text()).toContain('还没有记录，先添加第一笔资产')
  })

  it('列表加载失败展示错误', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    listItems.mockRejectedValue({ response: { data: { message: '解密失败' } } })
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="items-error"]').text()).toContain('解密失败')
    errorSpy.mockRestore()
  })
})
