<script setup lang="ts">
import { computed, ref } from 'vue'
import VChart from 'vue-echarts'
import { buildSankeyOption, ensureLedgerECharts, type CompositionMode } from './ledgerChartOptions'
import type { LedgerSummaryView } from '../../api/ledger'

ensureLedgerECharts()

const props = defineProps<{
  view: LedgerSummaryView
  mode: CompositionMode
  loading?: boolean
}>()

// vue-echarts 暴露底层 ECharts 实例方法，导出图片时直接取 canvas 数据。
const chartRef = ref<{ getDataURL: (options: Record<string, unknown>) => string } | null>(null)
const option = computed(() => buildSankeyOption(props.view, props.mode))

function toDataURL(): string {
  if (!chartRef.value) return ''
  try {
    return chartRef.value.getDataURL({ type: 'png', pixelRatio: 2, backgroundColor: '#FFFFFF' })
  } catch (err) {
    console.error('导出图表失败', err)
    return ''
  }
}

defineExpose({ toDataURL })
</script>

<template>
  <div class="relative h-[560px] w-full">
    <div v-if="loading" class="absolute inset-0 flex items-center justify-center rounded-xl bg-white/70">
      <div class="h-40 w-2/3 animate-pulse rounded-2xl bg-slate-100" />
    </div>
    <VChart v-else ref="chartRef" class="h-full w-full" :option="option" autoresize />
  </div>
</template>
