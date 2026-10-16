// ECharts 按需注册与图表配置构建（纯函数，便于单元测试）。
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart, ScatterChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  TitleComponent,
} from 'echarts/components'
import 'echarts-wordcloud'

let registered = false

// ensureECharts 幂等注册按需组件（各图表组件首次渲染前调用）。
export function ensureECharts() {
  if (registered) return
  use([
    CanvasRenderer,
    BarChart,
    ScatterChart,
    GridComponent,
    TooltipComponent,
    LegendComponent,
    TitleComponent,
  ])
  registered = true
}

// ── 新中式配色系统 ────────────────────────────────────────────
export const POETRY_PALETTE = ['#A63D2F', '#2C3E50', '#3D6B5E', '#C9A227', '#8B3A3A', '#4A7C59']

// 词云色阶：朱砂 → 黛青渐变（按频次取色）。
export const WORDCLOUD_COLORS = ['#A63D2F', '#8B3A3A', '#C05A4A', '#C9A227', '#3D6B5E', '#2C3E50']

// 朝代年代表（约值，用于时间轴横轴定位）。
export const DYNASTY_YEARS: Record<string, [number, number]> = {
  先秦: [-1046, -221],
  汉: [-202, 220],
  三国: [220, 280],
  晋: [265, 420],
  南北朝: [420, 589],
  唐: [618, 907],
  宋: [960, 1279],
  元: [1271, 1368],
  明: [1368, 1644],
  清: [1636, 1912],
}

// 统计项类型（与 api/poetry.ts 对应，避免循环依赖此处重新声明结构）。
export interface StatItem {
  dynasty: string
  order: number
  count: number
}

export interface VolumeStatItem {
  volume: string
  inclass: number
  outline: number
  other: number
}

export interface AuthorStatItem {
  author: string
  count: number
}

export interface WordStatItem {
  word: string
  count: number
}

// 公共文字与图例样式。
const axisLabel = { color: '#2B2B2B', fontSize: 12 }
const tooltipStyle = {
  backgroundColor: 'rgba(44, 62, 80, 0.92)',
  borderWidth: 0,
  textStyle: { color: '#F7F4EC', fontSize: 12 },
}

// 朝代分布柱状图（先秦 → 清有序排列）。
export function dynastyBarOption(stats: StatItem[]) {
  return {
    color: [POETRY_PALETTE[0]],
    tooltip: { ...tooltipStyle, trigger: 'axis' },
    grid: { left: 8, right: 8, top: 24, bottom: 0, containLabel: true },
    xAxis: {
      type: 'category',
      data: stats.map((s) => s.dynasty),
      axisLabel: { ...axisLabel, interval: 0 },
      axisLine: { lineStyle: { color: '#D8D2C4' } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      axisLabel,
      splitLine: { lineStyle: { color: '#EFE9DB' } },
    },
    series: [
      {
        type: 'bar',
        barMaxWidth: 26,
        itemStyle: { borderRadius: [4, 4, 0, 0] },
        data: stats.map((s) => s.count),
      },
    ],
  }
}

// 朝代时间线：横轴年代跨度 + 气泡大小表示篇目数量。
export function dynastyTimelineOption(stats: StatItem[]) {
  const data = stats
    .filter((s) => DYNASTY_YEARS[s.dynasty])
    .map((s) => {
      const [start] = DYNASTY_YEARS[s.dynasty]
      return {
        name: s.dynasty,
        value: [start, s.dynasty, s.count],
        symbolSize: 10 + Math.sqrt(s.count) * 4,
      }
    })
  return {
    tooltip: {
      ...tooltipStyle,
      formatter: (p: { name: string; value: [number, string, number] }) =>
        `${p.name}：${p.value[2]} 篇`,
    },
    grid: { left: 8, right: 24, top: 16, bottom: 0, containLabel: true },
    xAxis: {
      type: 'value',
      min: -1200,
      max: 2000,
      axisLabel: { ...axisLabel, formatter: (v: number) => (v < 0 ? `前${-v}` : `${v}`) },
      splitLine: { lineStyle: { color: '#EFE9DB' } },
    },
    yAxis: {
      type: 'category',
      data: stats.map((s) => s.dynasty),
      inverse: true,
      axisLabel: { ...axisLabel },
      axisLine: { lineStyle: { color: '#D8D2C4' } },
      axisTick: { show: false },
    },
    series: [
      {
        type: 'scatter',
        data,
        itemStyle: { color: POETRY_PALETTE[2], opacity: 0.85 },
        label: { show: true, position: 'right', color: '#6B6B6B', fontSize: 11,
          formatter: (p: { value: [number, string, number] }) => `${p.value[2]}` },
      },
    ],
  }
}

// 册数 × 课内外堆叠柱状图。
export function volumeStackOption(stats: VolumeStatItem[]) {
  return {
    color: [POETRY_PALETTE[0], POETRY_PALETTE[1], POETRY_PALETTE[3]],
    tooltip: { ...tooltipStyle, trigger: 'axis' },
    legend: { top: 0, textStyle: { color: '#6B6B6B', fontSize: 12 }, itemWidth: 12, itemHeight: 12 },
    grid: { left: 8, right: 8, top: 40, bottom: 0, containLabel: true },
    xAxis: {
      type: 'category',
      data: stats.map((s) => s.volume),
      axisLabel: { ...axisLabel, interval: 0, rotate: 24 },
      axisLine: { lineStyle: { color: '#D8D2C4' } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel,
      splitLine: { lineStyle: { color: '#EFE9DB' } },
    },
    series: [
      { name: '课内', type: 'bar', stack: 'total', barMaxWidth: 26, data: stats.map((s) => s.inclass) },
      { name: '课外', type: 'bar', stack: 'total', data: stats.map((s) => s.outline) },
      { name: '未收录标注', type: 'bar', stack: 'total', data: stats.map((s) => s.other) },
    ],
  }
}

// 作者作品数 Top N 横向条形图。
export function authorBarOption(authors: AuthorStatItem[]) {
  // 横向条形图自下而上渲染，反转后第一名显示在最上方。
  const sorted = [...authors].reverse()
  return {
    color: [POETRY_PALETTE[2]],
    tooltip: { ...tooltipStyle, trigger: 'axis' },
    grid: { left: 8, right: 32, top: 8, bottom: 0, containLabel: true },
    xAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel,
      splitLine: { lineStyle: { color: '#EFE9DB' } },
    },
    yAxis: {
      type: 'category',
      data: sorted.map((a) => a.author),
      axisLabel: { ...axisLabel, width: 72, overflow: 'truncate' },
      axisLine: { lineStyle: { color: '#D8D2C4' } },
      axisTick: { show: false },
    },
    series: [
      {
        type: 'bar',
        barMaxWidth: 16,
        itemStyle: { borderRadius: [0, 4, 4, 0] },
        label: { show: true, position: 'right', color: '#6B6B6B', fontSize: 11 },
        data: sorted.map((a) => a.count),
      },
    ],
  }
}

// 高频字词词云。
export function wordCloudOption(words: WordStatItem[]) {
  return {
    tooltip: { ...tooltipStyle, formatter: (p: { name: string; value: number }) => `${p.name}：${p.value} 次` },
    series: [
      {
        type: 'wordCloud',
        shape: 'circle',
        width: '96%',
        height: '96%',
        sizeRange: [14, 52],
        rotationRange: [0, 0],
        gridSize: 6,
        textStyle: {
          fontFamily: '"Noto Serif SC", "Songti SC", SimSun, serif',
          fontWeight: 600,
        },
        data: words.map((w) => ({
          name: w.word,
          value: w.count,
          textStyle: { color: WORDCLOUD_COLORS[w.count % WORDCLOUD_COLORS.length] },
        })),
      },
    ],
  }
}
