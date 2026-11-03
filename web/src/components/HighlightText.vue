<script setup lang="ts">
import { computed } from 'vue'
import { highlightParts } from '../utils/highlight'

// 关键词高亮渲染：命中片段标黄，其余原样输出（纯文本插值，不用 v-html）。
const props = withDefaults(defineProps<{ text?: string; keyword?: string }>(), {
  text: '',
  keyword: '',
})

const parts = computed(() => highlightParts(props.text, props.keyword))
</script>

<template>
  <span>
    <template v-for="(part, i) in parts" :key="i">
      <mark v-if="part.hit" class="bg-amber-200/70 text-ink rounded px-0.5">{{ part.text }}</mark>
      <template v-else>{{ part.text }}</template>
    </template>
  </span>
</template>
