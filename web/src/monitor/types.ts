// 前端监控事件与上报结构的统一类型定义。

/** 上报事件类型。 */
export type MonitorEventType = 'error' | 'webvital' | 'pv' | 'behavior'

/** 单条监控事件（与网关 /api/monitor/report 的解析约定一致）。 */
export interface MonitorEvent {
  /** 事件大类：error / webvital / pv / behavior */
  type: MonitorEventType
  /** 事件名：error_kind / LCP / route / page_dwell 等 */
  name: string
  /** 扩展数据（浅对象，值需可 JSON 序列化） */
  data?: Record<string, string | number | boolean | null>
  /** 事件产生时间戳（毫秒） */
  ts: number
  /** 页面路径 */
  path: string
  /** 关联的页面 trace id（与后端链路对齐） */
  trace_id?: string
}

/** SDK 配置。 */
export interface MonitorOptions {
  /** 上报端点（默认 /api/monitor/report） */
  endpoint?: string
  /** 错误类事件采样率 0~1（默认 1：全采） */
  errorSampleRate?: number
  /** 行为/性能类事件采样率 0~1（默认 0.1） */
  behaviorSampleRate?: number
  /** 批量上报最大条数（默认 20） */
  maxBatchSize?: number
  /** 批量上报定时间隔毫秒（默认 5000） */
  flushInterval?: number
  /** 事件产生到上报的延迟回调（测试用） */
  send?: (payload: string, keepalive: boolean) => boolean
}
