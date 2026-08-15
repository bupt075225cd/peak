import axios from 'axios'

const http = axios.create({
  baseURL: '/api',
  timeout: 30000,
})

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

// 保存错题。
export async function createMistake(payload: Record<string, unknown>): Promise<unknown> {
  const { data } = await http.post<ApiResponse>('/mistakes', payload)
  return data.data
}
