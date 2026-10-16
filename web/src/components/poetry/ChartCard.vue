<script setup lang="ts">
import { computed } from 'vue'
import VChart from 'vue-echarts'
import { ensureECharts } from './chartOptions'

ensureECharts()

// ChartCard 统一图表卡片：标题 + 副标题 + 响应式 ECharts 容器。
const props = defineProps<{
  title: string
  subtitle?: string
  option: Record<string, unknown>
  height?: string
}>()

const heightClass = computed(() => props.height ?? 'h-64')
</script>

<template>
  <div
    class="rounded-2xl border border-[#E5DFD0] bg-poetry-paper shadow-sm transition-shadow hover:shadow-md p-4 sm:p-5 animate-fade-up"
  >
    <div class="mb-2 flex items-baseline justify-between gap-2">
      <h3 class="font-serif text-base font-semibold text-poetry-inkstone">{{ title }}</h3>
      <span v-if="subtitle" class="text-xs text-[#8A8272]">{{ subtitle }}</span>
    </div>
    <VChart class="w-full" :class="heightClass" :option="option" autoresize />
  </div>
</template>
