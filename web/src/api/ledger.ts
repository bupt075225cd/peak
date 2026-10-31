import { http } from './index'

// 统一响应结构（复用 api/index 的 axios 实例与拦截器）。
interface ApiResponse<T = unknown> {
  code: number
  message: string
  data?: T
}

// 账本分类：敏感字段一律由服务端加密，前端只见枚举 key。
export type LedgerCategory = 'liquid' | 'fixed' | 'investment' | 'receivable' | 'liability'
export type RiskType = 'cash' | 'stable' | 'long' | 'none'

export interface LedgerSubTypeMeta {
  key: string
  label: string
  icon: string
  is_fixed: boolean
}

export interface LedgerCategoryMeta {
  key: LedgerCategory
  label: string
  kind: 'asset' | 'liability'
  color: string
  sub_types: LedgerSubTypeMeta[]
  /** 分类口径说明（更新流程的「什么是{分类}」与子类弹层顶部文案）。 */
  description: string
}

export interface LedgerRiskMeta {
  key: RiskType
  label: string
}

export interface LedgerCurrencyMeta {
  key: string
  label: string
  symbol: string
}

export interface LedgerMeta {
  categories: LedgerCategoryMeta[]
  risk_types: LedgerRiskMeta[]
  currencies: LedgerCurrencyMeta[]
  consent_policy_version: string
  consent_required: boolean
}

export interface LedgerItem {
  id: number
  name: string
  category: LedgerCategory
  sub_type: string
  amount_cents: number
  currency: string
  risk_type: RiskType
  member_id: number | null
  member_name: string
  note: string
  version: number
  updated_at: string
}

export interface LedgerSummaryChild {
  key: string
  label: string
  amount_cents: number
  count: number
  is_fixed: boolean
}

export interface LedgerSummaryGroup {
  category: LedgerCategory
  label: string
  kind: 'asset' | 'liability'
  color: string
  amount_cents: number
  count: number
  children: LedgerSummaryChild[]
}

export interface LedgerSummary {
  groups: LedgerSummaryGroup[]
  total_asset_cents: number
  total_liability_cents: number
  net_asset_cents: number
  item_count: number
  updated_at: string
  has_foreign: boolean
  consent_required: boolean
}

export interface LedgerItemList {
  items: LedgerItem[]
  total: number
  sum_cents: number
  category_counts: Partial<Record<LedgerCategory, number>>
}

export interface LedgerMember {
  id: number
  name: string
  sort_order: number
  /** 是否在家庭资产记账的成员入口中显示（关闭后仍保留成员与名下条目）。 */
  visible: boolean
}

/**
 * 汇总范围：'' 为全部；'self' 为未归属任何成员的条目（总览里的「我」）；
 * 其余为成员 ID 字符串。
 */
export type MemberScope = '' | 'self' | `${number}`

export interface SnapshotBreakdown {
  category: LedgerCategory
  label: string
  amount_cents: number
  count: number
}

export interface SnapshotPayload {
  total_asset_cents: number
  total_liability_cents: number
  net_asset_cents: number
  breakdown: SnapshotBreakdown[]
}

export interface LedgerSnapshot {
  period: string
  payload: SnapshotPayload
  updated_at: string
}

export interface LedgerConsent {
  policy_version: string
  required: boolean
  consented_at: string | null
}

export interface LedgerDeletion {
  status: 'none' | 'pending' | 'cancelled' | 'completed'
  requested_at: string | null
  execute_after: string | null
  days_left: number
}

export interface LedgerItemInput {
  name: string
  category: LedgerCategory
  sub_type: string
  amount_cents: number
  currency: string
  risk_type: RiskType
  member_id: number | null
  note: string
}

/** 总览视图：排除固定资产后在客户端重算，0 网络请求。 */
export interface LedgerSummaryView {
  groups: LedgerSummaryGroup[]
  totalAssetCents: number
  totalLiabilityCents: number
  netAssetCents: number
  itemCount: number
  excludeFixed: boolean
}

export interface LedgerItemQuery {
  category?: string
  keyword?: string
  offset?: number
  limit?: number
}

// ── 元数据与聚合 ──────────────────────────────────────────

export async function getMeta(): Promise<LedgerMeta> {
  const { data } = await http.get<ApiResponse<LedgerMeta>>('/ledger/meta')
  return data.data as LedgerMeta
}

/** 按成员范围取汇总；筛选在服务端解密后完成，前端无需聚合密文。 */
export async function getSummary(scope: MemberScope = ''): Promise<LedgerSummary> {
  const { data } =
    scope === ''
      ? await http.get<ApiResponse<LedgerSummary>>('/ledger/summary')
      : await http.get<ApiResponse<LedgerSummary>>('/ledger/summary', { params: { member_id: scope } })
  return data.data as LedgerSummary
}

/**
 * 按「是否排除固定资产」构建总览视图。
 *
 * 固定资产整类剔除（含房产、汽车等自用实物），负债不受影响；
 * 排除后负债率可能超过 100%，按实际值展示、不做截断。
 */
export function buildSummaryView(summary: LedgerSummary, excludeFixed: boolean): LedgerSummaryView {
  const groups = summary.groups.filter((g) => !(excludeFixed && g.category === 'fixed'))
  let totalAssetCents = 0
  let totalLiabilityCents = 0
  let itemCount = 0
  for (const group of groups) {
    if (group.kind === 'liability') totalLiabilityCents += group.amount_cents
    else totalAssetCents += group.amount_cents
    itemCount += group.count
  }
  return {
    groups,
    totalAssetCents,
    totalLiabilityCents,
    netAssetCents: totalAssetCents - totalLiabilityCents,
    itemCount,
    excludeFixed,
  }
}

// ── 条目 ──────────────────────────────────────────────────

export async function listItems(query: LedgerItemQuery = {}): Promise<LedgerItemList> {
  const params: Record<string, string | number> = {
    offset: query.offset ?? 0,
    limit: query.limit ?? 20,
  }
  if (query.category) params.category = query.category
  if (query.keyword?.trim()) params.keyword = query.keyword.trim()
  const { data } = await http.get<ApiResponse<LedgerItemList>>('/ledger/items', { params })
  const payload = data.data
  return {
    items: payload?.items ?? [],
    total: payload?.total ?? 0,
    sum_cents: payload?.sum_cents ?? 0,
    category_counts: payload?.category_counts ?? {},
  }
}

export async function createItem(input: LedgerItemInput): Promise<LedgerItem> {
  const { data } = await http.post<ApiResponse<LedgerItem>>('/ledger/items', input)
  return data.data as LedgerItem
}

export async function updateItem(id: number, input: LedgerItemInput, version: number): Promise<LedgerItem> {
  const { data } = await http.put<ApiResponse<LedgerItem>>(`/ledger/items/${id}`, { ...input, version })
  return data.data as LedgerItem
}

export async function deleteItem(id: number): Promise<void> {
  await http.delete(`/ledger/items/${id}`)
}

// ── 成员 ──────────────────────────────────────────────────

export async function listMembers(): Promise<LedgerMember[]> {
  const { data } = await http.get<ApiResponse<{ items: LedgerMember[] }>>('/ledger/members')
  return data.data?.items ?? []
}

export async function createMember(name: string, visible = true): Promise<LedgerMember> {
  const { data } = await http.post<ApiResponse<LedgerMember>>('/ledger/members', { name, visible })
  return data.data as LedgerMember
}

/** 修改成员昵称与「是否在记账中显示」；仅传需要变更的字段。 */
export async function updateMember(
  id: number,
  patch: { name?: string; visible?: boolean },
): Promise<LedgerMember> {
  const { data } = await http.put<ApiResponse<LedgerMember>>(`/ledger/members/${id}`, patch)
  return data.data as LedgerMember
}

export async function renameMember(id: number, name: string): Promise<LedgerMember> {
  return updateMember(id, { name })
}

export async function deleteMember(id: number): Promise<void> {
  await http.delete(`/ledger/members/${id}`)
}

// ── 月度快照 ──────────────────────────────────────────────

export async function listSnapshots(year: string): Promise<LedgerSnapshot[]> {
  const { data } = await http.get<ApiResponse<{ items: LedgerSnapshot[] }>>('/ledger/snapshots', {
    params: { year },
  })
  return data.data?.items ?? []
}

// ── 隐私：同意 / 导出 / 注销 ──────────────────────────────

export async function getConsent(): Promise<LedgerConsent> {
  const { data } = await http.get<ApiResponse<LedgerConsent>>('/ledger/privacy/consent')
  return data.data as LedgerConsent
}

export async function acceptConsent(policyVersion: string): Promise<LedgerConsent> {
  const { data } = await http.post<ApiResponse<LedgerConsent>>('/ledger/privacy/consent', {
    policy_version: policyVersion,
  })
  return data.data as LedgerConsent
}

/** 全量导出：需新鲜令牌，服务端以文件流返回。 */
export async function exportLedger(): Promise<{ blob: Blob; filename: string }> {
  const response = await http.get('/ledger/privacy/export', { responseType: 'blob', timeout: 60000 })
  return {
    blob: response.data as Blob,
    filename: parseFilename(response.headers['content-disposition']) ?? '家庭资产账本导出.json',
  }
}

export function parseFilename(disposition?: string): string | null {
  if (!disposition) return null
  const utf8 = /filename\*=UTF-8''([^;]+)/i.exec(disposition)
  if (utf8?.[1]) {
    try {
      return decodeURIComponent(utf8[1])
    } catch {
      return null
    }
  }
  const ascii = /filename="?([^";]+)"?/i.exec(disposition)
  return ascii?.[1] ?? null
}

export async function getDeletion(): Promise<LedgerDeletion> {
  const { data } = await http.get<ApiResponse<LedgerDeletion>>('/ledger/privacy/deletion')
  return data.data as LedgerDeletion
}

export async function requestDeletion(): Promise<LedgerDeletion> {
  const { data } = await http.post<ApiResponse<LedgerDeletion>>('/ledger/privacy/deletion')
  return data.data as LedgerDeletion
}

export async function cancelDeletion(): Promise<LedgerDeletion> {
  const { data } = await http.delete<ApiResponse<LedgerDeletion>>('/ledger/privacy/deletion')
  return data.data as LedgerDeletion
}

export async function purgeNow(): Promise<void> {
  await http.post('/ledger/privacy/deletion/purge')
}

/** 判断错误是否为「需要重认证」（428 / 业务码 1007）。 */
export function isReauthRequired(error: unknown): boolean {
  const status = (error as { response?: { status?: number } })?.response?.status
  return status === 428
}

/** 取业务错误消息，回退到通用文案。 */
export function errorMessage(error: unknown, fallback = '操作失败，请稍后重试'): string {
  const data = (error as { response?: { data?: { message?: string } } })?.response?.data
  return data?.message || fallback
}
