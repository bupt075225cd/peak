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

// 识别结果。
export interface RecognitionResult {
  stem_text: string
  answer: string
  subject?: string
  question_type?: string
  geometry: { shape_type: string; properties: Record<string, string>; description: string }
  // 几何重绘 SVG 的存储 key 列表：一个 key 对应一张独立 SVG
  //（一张原图含多个几何子图时，每个子图一张）。
  redraw_svg_keys?: string[]
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
  image?: string // JSON 字符串：图片 image key 列表（几何图或其它科目插图）
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

// 查询错题列表。
export async function listMistakes(): Promise<Mistake[]> {
  const { data } = await http.get<ApiResponse<{ items: Mistake[]; total: number }>>('/mistakes')
  return data.data?.items ?? []
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
