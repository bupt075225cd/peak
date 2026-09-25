import axios from 'axios'

const http = axios.create({
  baseURL: '/api',
  timeout: 30000,
})

// 当前鉴权为预留实现，网关注入 mock 用户；前端也统一带 mock 用户 ID，
// 保证 listMistakes 等按用户维度查询的接口能取到数据。
http.defaults.headers.common['X-User-Id'] = '1'

// 统一响应结构。
export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data?: T
}

// 识别任务。
export interface RecognitionTask {
  id: number
  image_id: number
  status: 'pending' | 'processing' | 'success' | 'failed'
  progress: number
  progress_text?: string
  result_json?: string
  error_message?: string
  provider: string
}

// 题目配图引用：存储 key + 图号标识（如"图1"）。
//
// 图号来自识别阶段：AI 重绘只保留图形本身，原图的"图1/图2"文字标注需要显式保留，
// 才能在导出文档里标注回配图。label 缺省表示该图无图号。
export interface QuestionImageRef {
  key: string
  label?: string
}

// 识别结果。
export interface RecognitionResult {
  stem_text: string
  answer: string
  subject?: string
  question_type?: string
  geometry: { shape_type: string; properties: Record<string, string>; description: string }
  // 几何重绘产出的子图：key 为独立 SVG 的存储 key，label 为图号（如"图1"）；
  // 一张原图含多个几何子图时逐个列出。
  redraw_figures?: QuestionImageRef[]
  // 几何重绘求解报告。
  redraw_report?: {
    max_hard: number
    max_soft: number
    attempts: number
    consistent: boolean
  }
  // 文档识别出的多道题。
  questions?: QuestionItem[]
  // 非致命错误提示（如公式/几何识别失败）。
  warning?: string
}

// 文档识别拆分出的单道题。
export interface SubQuestion {
  label: string
  text: string
  geometry_refs: number[]
  geometry_desc: string
  geometry_keys?: string[]
}

export interface QuestionItem {
  stem_text: string
  answer: string
  subject?: string
  question_type?: string
  geometry: { shape_type: string; properties: Record<string, string>; description: string }
  sub_questions?: SubQuestion[]
}

// 分类。
export interface Category {
  id: number
  parent_id: number | null
  name: string
  type: string
  sort_order: number
}

// 上传图片并创建识别任务。
export async function uploadImage(file: File): Promise<RecognitionTask> {
  const form = new FormData()
  form.append('image', file)
  const { data } = await http.post<ApiResponse<RecognitionTask>>('/recognition/tasks', form)
  return data.data as RecognitionTask
}

// 上传 word/pdf 文档并创建识别任务。
export async function uploadDocument(file: File): Promise<RecognitionTask> {
  const form = new FormData()
  form.append('document', file)
  const { data } = await http.post<ApiResponse<RecognitionTask>>('/recognition/tasks', form)
  return data.data as RecognitionTask
}

// 判断文件是否为文档类型（word/pdf）。
export function isDocument(file: File): boolean {
  const name = file.name.toLowerCase()
  return name.endsWith('.docx') || name.endsWith('.pdf') || name.endsWith('.doc')
}

// 查询识别任务状态。
export async function getTask(id: number): Promise<RecognitionTask> {
  const { data } = await http.get<ApiResponse<RecognitionTask>>(`/recognition/tasks/${id}`)
  return data.data as RecognitionTask
}

// 重试识别任务。
export async function retryTask(id: number): Promise<void> {
  await http.post(`/recognition/tasks/${id}/retry`)
}

// 查询分类。
export async function listCategories(type?: string): Promise<Category[]> {
  const { data } = await http.get<ApiResponse<Category[]>>('/categories', {
    params: { type },
  })
  return (data.data as Category[]) ?? []
}

// 题目。
export interface Question {
  id: number
  subject: string
  grade?: string
  stem_text: string
  answer: string
  analysis: string
  question_type: string
  image?: string // JSON 字符串：配图引用数组（QuestionImageRef[]，含图号）
  knowledge_points?: string // JSON 字符串：知识点标签数组
  source?: string // 题目来源（试卷/练习册等）
}

// 单条复习记录。
export interface ReviewRecord {
  reviewed_at: string // 复习时间
  result: string // 复习结果
}

// 错题（含关联题目）。
export interface Mistake {
  id: number
  user_id: number
  question_id: number
  wrong_reason: string
  source: string // 该次错题的来源
  review_records?: ReviewRecord[] // 复习记录
  recorded_at: string
  question?: Question
}

// 创建题目（题目本体，供错题关联）。
export async function createQuestion(payload: Record<string, unknown>): Promise<Question> {
  const { data } = await http.post<ApiResponse<Question>>('/questions', payload)
  return data.data as Question
}

// 保存错题（需先创建题目得到 question_id）。
export async function createMistake(payload: Record<string, unknown>): Promise<unknown> {
  const { data } = await http.post<ApiResponse>('/mistakes', payload)
  return data.data
}

// 更新错题（修正错误原因、来源、重做记录等）。
//
// 服务端按整条记录覆盖保存，payload 需带上 user_id/question_id/recorded_at 等
// 不可丢字段，否则会被写成零值；前端在编辑弹窗里基于原记录构造。
export async function updateMistake(id: number, payload: Record<string, unknown>): Promise<Mistake> {
  const { data } = await http.put<ApiResponse<Mistake>>(`/mistakes/${id}`, payload)
  return data.data as Mistake
}

// 错题列表查询参数：分页 + 服务端筛选。
export interface MistakeListParams {
  offset?: number
  limit?: number
  /** 关键词：空格分隔多个词，全部命中（匹配 题干/知识点/题型/来源）。 */
  keyword?: string
  /** 学科精确匹配。 */
  subject?: string
  /** 来源精确匹配（题目来源优先，其次错题记录来源）。 */
  source?: string
}

// 错题列表查询结果：当前页条目、总数与分面计数。
export interface MistakeListResult {
  items: Mistake[]
  total: number
  /** 学科分布计数（仅按关键词统计）。 */
  subjectCounts: Record<string, number>
  /** 来源分布计数（仅按关键词统计，不含空来源）。 */
  sourceCounts: Record<string, number>
}

// 查询错题列表：筛选在服务端完成，返回当前页、总数与分面计数。
export async function listMistakes(params: MistakeListParams = {}): Promise<MistakeListResult> {
  const { offset = 0, limit = 20, keyword, subject, source } = params
  const query: Record<string, string | number> = { offset, limit }
  if (keyword?.trim()) query.keyword = keyword.trim()
  if (subject) query.subject = subject
  if (source) query.source = source

  const { data } = await http.get<ApiResponse<{
    items: Mistake[]
    total: number
    subject_counts?: Record<string, number>
    source_counts?: Record<string, number>
  }>>('/mistakes', { params: query })

  return {
    items: data.data?.items ?? [],
    total: data.data?.total ?? 0,
    subjectCounts: data.data?.subject_counts ?? {},
    sourceCounts: data.data?.source_counts ?? {},
  }
}

// 导出格式。
export type ExportFormat = 'pdf' | 'docx'

// 导出错题：服务端直接返回文件流，文件名由 Content-Disposition 给出。
//
// 导出耗时明显高于普通接口（需取图与排版），这里单独放宽超时，不影响列表等请求。
export async function exportMistakes(
  ids: number[],
  format: ExportFormat,
): Promise<{ blob: Blob; filename: string }> {
  const response = await http.post(
    '/mistakes/export',
    { ids, format },
    { responseType: 'blob', timeout: 120000 },
  )
  const filename =
    parseContentDisposition(response.headers['content-disposition']) ?? `我的错题本.${format}`
  return { blob: response.data as Blob, filename }
}

// parseContentDisposition 解析响应头中的文件名，优先 RFC 5987 的 filename*（UTF-8）。
export function parseContentDisposition(disposition?: string): string | null {
  if (!disposition) return null

  const utf8Match = /filename\*=UTF-8''([^;]+)/i.exec(disposition)
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1])
    } catch {
      return null
    }
  }

  const asciiMatch = /filename="?([^";]+)"?/i.exec(disposition)
  return asciiMatch?.[1] ?? null
}
