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
  result_json?: string
  error_message?: string
  provider: string
}

// 识别结果。
export interface RecognitionResult {
  stem_text: string
  answer: string
  formula: { latex: string; raw_text: string }
  geometry: { shape_type: string; properties: Record<string, string>; description: string }
  erased_image_key: string
  // 单图上传场景下，与该题关联的几何图存储 key 列表（从原图裁剪出的几何图子图）。
  geometry_keys?: string[]
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
  formula: { latex: string; raw_text: string }
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

// 上传任意图片（手动裁剪的几何图），返回 storage key。
export async function uploadGeometryImage(file: File): Promise<string> {
  const form = new FormData()
  form.append('file', file)
  const { data } = await http.post<ApiResponse<{ key: string }>>('/recognition/files', form)
  return data.data?.key ?? ''
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
  stem_text: string
  answer: string
  analysis: string
  difficulty: number
  question_type: string
  geometry_refs?: string // JSON 字符串：几何图形 image key 列表
}

// 错题（含关联题目）。
export interface Mistake {
  id: number
  user_id: number
  question_id: number
  wrong_reason: string
  mastery_level: number
  source_paper: string
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
