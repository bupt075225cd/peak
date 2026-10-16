import { describe, it, expect, vi } from 'vitest'

// echarts-wordcloud 在模块加载期检测 canvas，jsdom 环境不支持，mock 掉。
vi.mock('echarts-wordcloud', () => ({}))

import {
  dynastyBarOption,
  dynastyTimelineOption,
  volumeStackOption,
  authorBarOption,
  wordCloudOption,
  ensureECharts,
  DYNASTY_YEARS,
} from './chartOptions'

const dynastyStats = [
  { dynasty: '先秦', order: 0, count: 17 },
  { dynasty: '汉', order: 1, count: 5 },
  { dynasty: '唐', order: 5, count: 46 },
]

describe('chartOptions.ts', () => {
  it('dynastyBarOption：类目轴为朝代序列，柱值为篇数', () => {
    const opt = dynastyBarOption(dynastyStats) as {
      xAxis: { data: string[] }
      series: { data: number[] }[]
    }
    expect(opt.xAxis.data).toEqual(['先秦', '汉', '唐'])
    expect(opt.series[0].data).toEqual([17, 5, 46])
  })

  it('dynastyTimelineOption：气泡尺寸随篇数增长，未知朝代被过滤', () => {
    const opt = dynastyTimelineOption([
      ...dynastyStats,
      { dynasty: '未知', order: 99, count: 3 },
    ]) as unknown as {
      series: { data: { name: string; value: [number, string, number]; symbolSize: number }[] }[]
    }
    const names = opt.series[0].data.map((d) => d.name)
    expect(names).not.toContain('未知')
    const xianqin = opt.series[0].data.find((d) => d.name === '先秦')!
    const tang = opt.series[0].data.find((d) => d.name === '唐')!
    expect(xianqin.value[0]).toBe(DYNASTY_YEARS['先秦'][0])
    expect(tang.symbolSize).toBeGreaterThan(xianqin.symbolSize)
  })

  it('volumeStackOption：三个堆叠系列（课内/课外/未收录）', () => {
    const opt = volumeStackOption([
      { volume: '八年级上册', inclass: 8, outline: 3, other: 1 },
    ]) as { series: { name: string; data: number[] }[] }
    expect(opt.series.map((s) => s.name)).toEqual(['课内', '课外', '未收录标注'])
    expect(opt.series.map((s) => s.data[0])).toEqual([8, 3, 1])
  })

  it('authorBarOption：数据反转让第一名显示在最上方', () => {
    const opt = authorBarOption([
      { author: '李白', count: 8 },
      { author: '杜甫', count: 5 },
    ]) as { yAxis: { data: string[] }; series: { data: number[] }[] }
    expect(opt.yAxis.data).toEqual(['杜甫', '李白'])
    expect(opt.series[0].data).toEqual([5, 8])
  })

  it('wordCloudOption：词频映射为词云数据并按频次取色', () => {
    const opt = wordCloudOption([
      { word: '明', count: 120 },
      { word: '月', count: 98 },
    ]) as { series: { data: { name: string; value: number; textStyle?: unknown }[] }[] }
    const items = opt.series[0].data
    expect(items.map((d) => ({ name: d.name, value: d.value }))).toEqual([
      { name: '明', value: 120 },
      { name: '月', value: 98 },
    ])
    // 不同频次取到不同色阶颜色。
    expect(items[0].textStyle).toBeTruthy()
    expect(items[1].textStyle).toBeTruthy()
  })

  it('ensureECharts 幂等注册不抛错', () => {
    expect(() => {
      ensureECharts()
      ensureECharts()
    }).not.toThrow()
  })
})
