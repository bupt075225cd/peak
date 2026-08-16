<script setup lang="ts">
// 使用 KaTeX 渲染 LaTeX 公式：用户能看到排版后的数学符号，而不是源码。
// 渲染失败时回退到原始文本，避免破坏录入流程。
import { computed } from 'vue'
import katex from 'katex'
import 'katex/dist/katex.min.css'

const props = withDefaults(defineProps<{
  expr: string
  displayMode?: boolean
}>(), {
  displayMode: false,
})

const rendered = computed(() => {
  const text = (props.expr ?? '').trim()
  if (!text) return ''
  try {
    return katex.renderToString(text, {
      throwOnError: false,
      displayMode: props.displayMode,
      output: 'html',
    })
  } catch (e) {
    // 渲染失败时回退到纯文本，方便用户手动修正。
    return ''
  }
})
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -->
  <span v-if="rendered" class="latex" v-html="rendered"></span>
  <span v-else class="latex-fallback">{{ expr }}</span>
</template>

<style scoped>
.latex {
  /* 长公式在窄列里自然换行，避免单行铺开溢出右侧。 */
  display: block;
  font-size: 1rem;
  line-height: 1.75;
  color: rgb(37 99 235); /* text-primary */
  word-break: break-word;
  overflow-wrap: anywhere;
}
.latex :deep(.katex-display) {
  margin: 0;
}
.latex-fallback {
  font-size: 0.875rem;
  color: rgb(71 85 105); /* text-ink-soft */
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  word-break: break-word;
}
</style>