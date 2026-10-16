import { http } from './index'

// 统一响应结构（复用 api/index 的 axios 实例与拦截器）。
interface ApiResponse<T = unknown> {
  code: number
  message: string
  data?: T
}

// 古诗文篇目。
export interface Poem {
  id: number
  title: string
  dynasty: string
  dynasty_order: number
  author: string
  volume: string
  category: string
  content: string
  word_count: number
}

// 篇目列表筛选参数。
export interface PoemListParams {
  offset?: number
  limit?: number
  dynasty?: string
  author?: string
  volume?: string
  category?: string
  keyword?: string
}

// 篇目列表结果。
export interface PoemListResult {
  items: Poem[]
  total: number
}

// 朝代分布统计项。
export interface DynastyStat {
  dynasty: string
  order: number
  count: number
}

// 册数分布统计项（课内/课外拆分）。
export interface VolumeStat {
  volume: string
  inclass: number
  outline: number
  other: number
}

// 作者排行统计项。
export interface AuthorCount {
  author: string
  count: number
}

// 字词频次统计项。
export interface WordStat {
  word: string
  count: number
}

// 词云统计结果。
export interface WordFreqResult {
  items: WordStat[]
  total_chars: number
}

// AI 智能对话占位响应（RAG 二阶段启用）。
export interface ChatResponse {
  enabled: boolean
  answer?: string
  notice: string
}

// 查询篇目列表（多维筛选 + 分页）。
export async function listPoems(params: PoemListParams = {}): Promise<PoemListResult> {
  const { offset = 0, limit = 20, dynasty, author, volume, category, keyword } = params
  const query: Record<string, string | number> = { offset, limit }
  if (dynasty) query.dynasty = dynasty
  if (author) query.author = author
  if (volume) query.volume = volume
  if (category) query.category = category
  if (keyword?.trim()) query.keyword = keyword.trim()

  const { data } = await http.get<ApiResponse<PoemListResult>>('/poetry/pieces', { params: query })
  return { items: data.data?.items ?? [], total: data.data?.total ?? 0 }
}

// 查询篇目详情。
export async function getPoem(id: number): Promise<Poem> {
  const { data } = await http.get<ApiResponse<Poem>>(`/poetry/pieces/${id}`)
  return data.data as Poem
}

// 查询相关篇目推荐（同作者优先，其次同朝代）。
export async function getRelatedPoems(id: number): Promise<Poem[]> {
  const { data } = await http.get<ApiResponse<Poem[]>>(`/poetry/pieces/${id}/related`)
  return (data.data as Poem[]) ?? []
}

// 朝代分布统计。
export async function getDynastyStats(): Promise<DynastyStat[]> {
  const { data } = await http.get<ApiResponse<DynastyStat[]>>('/poetry/stats/dynasties')
  return (data.data as DynastyStat[]) ?? []
}

// 册数 × 课内外分布统计。
export async function getVolumeStats(): Promise<VolumeStat[]> {
  const { data } = await http.get<ApiResponse<VolumeStat[]>>('/poetry/stats/volumes')
  return (data.data as VolumeStat[]) ?? []
}

// 作者作品数排行。
export async function getAuthorStats(top = 10): Promise<AuthorCount[]> {
  const { data } = await http.get<ApiResponse<AuthorCount[]>>('/poetry/stats/authors', {
    params: { top },
  })
  return (data.data as AuthorCount[]) ?? []
}

// 高频字词统计（词云数据源）。
export async function getWordStats(top = 100): Promise<WordFreqResult> {
  const { data } = await http.get<ApiResponse<WordFreqResult>>('/poetry/stats/words', {
    params: { top },
  })
  return { items: data.data?.items ?? [], total_chars: data.data?.total_chars ?? 0 }
}

// AI 智能对话（当前为占位：后端返回"未启用"提示）。
export async function sendChat(question: string): Promise<ChatResponse> {
  const { data } = await http.post<ApiResponse<ChatResponse>>('/poetry/chat', { question })
  return (data.data as ChatResponse) ?? { enabled: false, notice: '服务暂不可用，请稍后重试' }
}
