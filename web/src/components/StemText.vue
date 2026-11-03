<script setup lang="ts">
import { computed } from 'vue'
import type { QuestionTable } from '../api'
import { splitStemBlocks } from '../utils/stemTable'
import HighlightText from './HighlightText.vue'

// 题干渲染：段落 + 表格统一观感。
//
// 两种输入都要支持：
//   - 识别结果：题干里是 {{table1}} 占位符，表格结构在 tables 里；
//   - 已保存的题目：题干里是 Markdown 表格（保存时由占位符序列化而来）。
const props = withDefaults(
  defineProps<{
    text?: string
    tables?: QuestionTable[]
    keyword?: string
    /** >0 时限制展示高度（约 clamp 行），用于编辑弹窗等窄容器 */
    clamp?: number
  }>(),
  { text: '', tables: () => [], keyword: '', clamp: 0 },
)

const segments = computed(() => splitStemBlocks(props.text, props.tables))

const clampStyle = computed(() =>
  props.clamp > 0 ? { maxHeight: `${props.clamp * 1.5}rem` } : undefined,
)
</script>

<template>
  <div v-if="segments.length" class="stem-text" :class="{ 'stem-clamp': clamp > 0 }" :style="clampStyle">
    <template v-for="(seg, i) in segments" :key="i">
      <table v-if="seg.table" class="stem-table">
        <thead v-if="seg.table.headers.length > 0">
          <tr>
            <th v-for="(head, hi) in seg.table.headers" :key="hi">
              <HighlightText :text="head" :keyword="keyword" />
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(row, ri) in seg.table.rows" :key="ri">
            <td v-for="(cell, ci) in row" :key="ci">
              <HighlightText :text="cell" :keyword="keyword" />
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="stem-para">
        <HighlightText :text="seg.text || ''" :keyword="keyword" />
      </p>
    </template>
  </div>
</template>

<style scoped>
.stem-text {
  line-height: 1.6;
  word-break: break-word;
}

.stem-para {
  margin: 0;
  white-space: pre-line;
}

.stem-para + .stem-para {
  margin-top: 0.25rem;
}

.stem-table {
  width: 100%;
  border-collapse: collapse;
  margin: 0.5rem 0;
  font-size: 0.875em;
}

.stem-table th,
.stem-table td {
  border: 1px solid #cbd5e1;
  padding: 0.3rem 0.5rem;
  text-align: left;
  vertical-align: top;
}

.stem-table th {
  background: #f8fafc;
  font-weight: 500;
}

.stem-clamp {
  overflow: hidden;
}
</style>
