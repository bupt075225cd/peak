<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, h, type FunctionalComponent } from 'vue'
import { BookOpen, Plus, Search, Loader2, Download } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import {
  listMistakes, exportMistakes,
  type Mistake, type ExportFormat, type QuestionImageRef,
} from '../api'
import ImageViewer from '../components/ImageViewer.vue'

const router = useRouter()
const keyword = ref('')
const loading = ref(false)
const items = ref<Mistake[]>([])
const activeSubject = ref('全部')
const activeSource = ref('全部')

// 每页加载条数（后端 limit 上限为 100，超出会回落为 20）。
const PAGE_SIZE = 20
// 符合当前筛选条件的错题总数（后端返回）。
const total = ref(0)
const loadingMore = ref(false)
// 服务端分面计数：学科/来源分布（仅按关键词统计，便于切换筛选）。
const subjectCounts = ref<Record<string, number>>({})
const sourceCounts = ref<Record<string, number>>({})

// 年级固定顺序（与录入页预置选项一致）。
const gradeOrder = ['七年级上', '七年级下', '八年级上', '八年级下', '九年级上', '九年级下']

// 是否有生效的筛选条件：用于区分「还没有错题」与「没有匹配的错题」。
const hasActiveFilter = computed(
  () => !!keyword.value.trim() || activeSubject.value !== '全部' || activeSource.value !== '全部',
)

// 学科列表：全部 + 后端分面中的学科。
const subjects = computed(() => ['全部', ...Object.keys(subjectCounts.value).sort()])

// 某学科下的错题数量：取后端分面计数（不是已加载条数）。
function subjectCount(subject: string): number {
  if (subject === '全部') return total.value
  return subjectCounts.value[subject] ?? 0
}

// 错题来源：优先题目来源，其次错题记录来源（与导出的取值口径一致）。
function mistakeSource(m: Mistake): string {
  return (m.question?.source ?? '').trim() || (m.source ?? '').trim()
}

// 来源列表：全部 + 后端分面中的来源（来源为空的历史数据不展示）。
const sourceOptions = computed(() => Object.keys(sourceCounts.value).sort())

// 某来源下的错题数量：取后端分面计数。
function sourceCount(source: string): number {
  if (source === '全部') return total.value
  return sourceCounts.value[source] ?? 0
}

// 按年级分组：固定顺序在前，无年级/未知年级归入「未分类」。
// items 已是后端按 关键词/学科/来源 过滤后的当前页数据，前端不再重复过滤。
const groupedItems = computed(() => {
  const groups: { grade: string; items: Mistake[] }[] = []
  for (const g of gradeOrder) {
    const list = items.value.filter((m) => (m.question?.grade ?? '') === g)
    if (list.length) groups.push({ grade: g, items: list })
  }
  const ungrouped = items.value.filter((m) => {
    const g = m.question?.grade ?? ''
    return !g || !gradeOrder.includes(g)
  })
  if (ungrouped.length) groups.push({ grade: '未分类', items: ungrouped })
  return groups
})

// 当前筛选条件（列表与分页请求共用）。
function currentFilters() {
  return {
    keyword: keyword.value.trim(),
    subject: activeSubject.value === '全部' ? '' : activeSubject.value,
    source: activeSource.value === '全部' ? '' : activeSource.value,
  }
}

// 是否还有未加载的错题。
const hasMore = computed(() => items.value.length < total.value)

// 加载第一页（筛选变化后重置列表，并刷新分面计数）。
async function loadFirstPage() {
  loading.value = true
  try {
    const res = await listMistakes({ offset: 0, limit: PAGE_SIZE, ...currentFilters() })
    items.value = res.items
    total.value = res.total
    subjectCounts.value = res.subjectCounts
    sourceCounts.value = res.sourceCounts
  } catch (err) {
    console.error('加载错题失败', err)
  } finally {
    loading.value = false
  }
}

// 追加下一页；失败时保留已加载内容，不影响已有列表。
async function loadMore() {
  if (loadingMore.value || !hasMore.value) return
  loadingMore.value = true
  try {
    const res = await listMistakes({
      offset: items.value.length,
      limit: PAGE_SIZE,
      ...currentFilters(),
    })
    items.value = [...items.value, ...res.items]
    total.value = res.total
    subjectCounts.value = res.subjectCounts
    sourceCounts.value = res.sourceCounts
  } catch (err) {
    console.error('加载更多错题失败', err)
  } finally {
    loadingMore.value = false
  }
}

// 关键词防抖：停止输入约 300ms 后再请求，避免每敲一个字都发一次请求。
const KEYWORD_DEBOUNCE_MS = 300
let keywordTimer: ReturnType<typeof setTimeout> | undefined

watch(keyword, () => {
  clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => void loadFirstPage(), KEYWORD_DEBOUNCE_MS)
})

// 学科 / 来源切换立即按第一页重新加载。
watch([activeSubject, activeSource], () => void loadFirstPage())

onUnmounted(() => clearTimeout(keywordTimer))

// 关键词高亮：把命中片段拆成数组交给模板渲染（不使用 v-html，避免 XSS）。
function highlightParts(text: string): { text: string; hit: boolean }[] {
  const terms = keyword.value.trim().toLowerCase().split(/\s+/).filter(Boolean)
  if (!text || terms.length === 0) return [{ text, hit: false }]

  const lower = text.toLowerCase()
  const marks = new Array<boolean>(text.length).fill(false)
  for (const term of terms) {
    for (let from = 0; ; ) {
      const idx = lower.indexOf(term, from)
      if (idx < 0) break
      for (let i = idx; i < idx + term.length; i++) marks[i] = true
      from = idx + term.length
    }
  }

  const parts: { text: string; hit: boolean }[] = []
  let start = 0
  for (let i = 1; i <= text.length; i++) {
    if (i === text.length || (marks[i] ?? false) !== (marks[start] ?? false)) {
      parts.push({ text: text.slice(start, i), hit: marks[start] ?? false })
      start = i
    }
  }
  return parts
}

// 高亮渲染组件：命中片段标黄，其余原样输出。
const HighlightText: FunctionalComponent<{ text: string }> = (props) => {
  if (!props.text) return null
  return h(
    'span',
    highlightParts(props.text).map((part, i) =>
      part.hit
        ? h('mark', { key: i, class: 'bg-amber-200/70 text-ink rounded px-0.5' }, part.text)
        : h('span', { key: i }, part.text),
    ),
  )
}

onMounted(loadFirstPage)

function goEntry() {
  router.push('/entry')
}

// 解析题目的 image（JSON 字符串）为配图引用列表（含图号）。
function imageRefs(q: Mistake['question']): QuestionImageRef[] {
  if (!q?.image) return []
  try {
    const arr = JSON.parse(q.image)
    if (!Array.isArray(arr)) return []
    return (arr as unknown[])
      .map((x) => x as QuestionImageRef)
      .filter((ref) => typeof ref?.key === 'string' && ref.key.length > 0)
      .map((ref) => ({ key: ref.key, label: (ref.label || '').trim() }))
  } catch {
    return []
  }
}

// 配图访问地址：识别服务的文件接口。
function figureUrl(key: string): string {
  return `/api/recognition/files/${key}`
}

// 解析题目的 knowledge_points（JSON 字符串数组）为知识点标签列表。
function knowledgePoints(q: Mistake['question']): string[] {
  if (!q?.knowledge_points) return []
  try {
    const arr = JSON.parse(q.knowledge_points)
    return Array.isArray(arr) ? arr.filter((x) => typeof x === 'string') : []
  } catch {
    return []
  }
}

// ImageViewer 状态：点击几何图时打开放大查看。
const viewerOpen = ref(false)
const viewerSrc = ref('')
function openViewer(url: string) {
  viewerSrc.value = url
  viewerOpen.value = true
}
function closeViewer() {
  viewerOpen.value = false
}

// ---- 导出 ----

// 选中集合：切换学科/关键词时保留勾选，避免误操作丢失已选题目。
const selectedIds = ref<Set<number>>(new Set())
const exporting = ref(false)
const exportError = ref('')
const exportMenuOpen = ref(false)

// 当前筛选结果中的已勾选项。
const selectedVisible = computed(() =>
  items.value.filter((m) => selectedIds.value.has(m.id)),
)

// 已勾选但被当前筛选排除的题目数量（导出时不会被包含）。
const hiddenSelectedCount = computed(() =>
  Math.max(0, selectedIds.value.size - selectedVisible.value.length),
)

// 导出目标：有勾选时只导出勾选项，否则导出已加载的当前筛选结果。
const exportTargets = computed(() =>
  selectedVisible.value.length > 0 ? selectedVisible.value : items.value,
)

const canExport = computed(() => !exporting.value && exportTargets.value.length > 0)

// 全选只作用于已加载的当前筛选结果，不跨筛选累加。
const allVisibleSelected = computed(
  () =>
    items.value.length > 0 &&
    items.value.every((m) => selectedIds.value.has(m.id)),
)

function isSelected(id: number): boolean {
  return selectedIds.value.has(id)
}

function toggleSelect(id: number) {
  const next = new Set(selectedIds.value)
  if (next.has(id)) {
    next.delete(id)
  } else {
    next.add(id)
  }
  selectedIds.value = next
}

function toggleSelectAllVisible() {
  const next = new Set(selectedIds.value)
  if (allVisibleSelected.value) {
    for (const m of items.value) next.delete(m.id)
  } else {
    for (const m of items.value) next.add(m.id)
  }
  selectedIds.value = next
}

async function handleExport(format: ExportFormat) {
  exportMenuOpen.value = false
  exportError.value = ''

  const ids = exportTargets.value.map((m) => m.id)
  if (!ids.length) {
    exportError.value = '没有可导出的错题'
    return
  }

  exporting.value = true
  try {
    const { blob, filename } = await exportMistakes(ids, format)
    downloadBlob(blob, filename)
  } catch (err) {
    exportError.value = await resolveExportError(err)
  } finally {
    exporting.value = false
  }
}

// downloadBlob 通过临时 <a> 触发浏览器下载，并释放 object URL。
function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

// resolveExportError 解析导出失败原因：blob 请求出错时响应体实际是 JSON。
//
// 用鸭子类型判断而非 instanceof Blob：跨 realm 与测试替身下 instanceof 不可靠。
async function resolveExportError(err: unknown): Promise<string> {
  const data = (err as { response?: { data?: unknown } })?.response?.data
  const readText = (data as { text?: () => Promise<string> } | undefined)?.text
  if (typeof readText === 'function') {
    try {
      const parsed = JSON.parse(await readText.call(data)) as { message?: string }
      if (parsed?.message) return parsed.message
    } catch {
      // 解析失败时回落到通用提示。
    }
  }
  return '导出失败，请重试'
}
</script>

<template>
  <div class="mx-auto max-w-6xl px-4 py-8">
    <div class="flex items-center justify-between mb-6 animate-fade-up">
      <div>
        <h1 class="text-2xl font-semibold text-ink">我的错题本</h1>
        <p class="text-sm text-ink-soft mt-1">
          共 {{ total }} 道错题<template v-if="hasMore"> · 已加载 {{ items.length }} 条</template>
        </p>
      </div>
      <div class="flex items-center gap-2">
        <!-- 导出：未勾选时导出当前筛选结果 -->
        <div class="relative">
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-xl border border-slate-200 bg-white px-4 py-2.5 text-sm font-medium text-ink-soft shadow-sm hover:bg-slate-50 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            :disabled="!canExport"
            @click="exportMenuOpen = !exportMenuOpen"
          >
            <Loader2 v-if="exporting" class="w-4 h-4 animate-spin" />
            <Download v-else class="w-4 h-4" />
            {{ exporting ? '导出中…' : '导出' }}
          </button>
          <div
            v-if="exportMenuOpen"
            class="absolute right-0 mt-2 w-40 rounded-xl border border-slate-200 bg-white shadow-lg py-1 z-10"
          >
            <button
              type="button"
              class="w-full text-left px-3 py-2 text-sm text-ink-soft hover:bg-slate-50"
              @click="handleExport('pdf')"
            >
              导出为 PDF
            </button>
            <button
              type="button"
              class="w-full text-left px-3 py-2 text-sm text-ink-soft hover:bg-slate-50"
              @click="handleExport('docx')"
            >
              导出为 Word
            </button>
          </div>
        </div>

        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-xl bg-primary text-white px-4 py-2.5 text-sm font-medium shadow-lg shadow-blue-500/25 hover:bg-primary-light transition-colors"
          @click="goEntry"
        >
          <Plus class="w-4 h-4" /> 录入错题
        </button>
      </div>
    </div>

    <!-- 搜索 -->
    <div class="relative mb-4 animate-fade-up">
      <Search class="w-4 h-4 text-ink-faint absolute left-3 top-1/2 -translate-y-1/2" />
      <input
        v-model="keyword"
        class="w-full rounded-xl border border-slate-200 bg-white pl-10 pr-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
        placeholder="搜索错题、知识点、来源"
      />
    </div>

    <!-- 按来源筛选（来源在录入时填写） -->
    <div
      v-if="sourceOptions.length"
      class="flex flex-wrap items-center gap-2 mb-6 animate-fade-up"
    >
      <span class="text-xs text-ink-faint mr-1">来源</span>
      <button
        v-for="s in ['全部', ...sourceOptions]"
        :key="s"
        type="button"
        class="rounded-full border px-3 py-1 text-xs font-medium transition-colors"
        :class="activeSource === s
          ? 'bg-primary text-white border-primary'
          : 'bg-white text-ink-soft border-slate-200 hover:bg-slate-50'"
        @click="activeSource = s"
      >
        {{ s }}<span class="ml-1 opacity-70">{{ sourceCount(s) }}</span>
      </button>
    </div>

    <!-- 选择与导出状态 -->
    <div
      v-if="items.length"
      class="flex flex-wrap items-center gap-3 mb-4 text-sm text-ink-soft animate-fade-up"
    >
      <label class="inline-flex items-center gap-2 cursor-pointer select-none">
        <input
          type="checkbox"
          class="rounded border-slate-300 text-primary focus:ring-primary/30 cursor-pointer disabled:cursor-not-allowed"
          :checked="allVisibleSelected"
          :disabled="!items.length"
          @change="toggleSelectAllVisible"
        />
        全选当前已加载
      </label>
      <span v-if="selectedIds.size" class="text-xs text-ink-faint">
        已选 {{ selectedIds.size }} 题
        <template v-if="hiddenSelectedCount">
          （其中 {{ hiddenSelectedCount }} 题不在当前筛选结果中，不会被导出）
        </template>
      </span>
      <span v-else class="text-xs text-ink-faint">未勾选时导出当前已加载的筛选结果</span>
      <span v-if="exportError" class="text-xs text-red-500">{{ exportError }}</span>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-[220px_1fr] gap-6">
      <!-- 左侧学科 Tab -->
      <aside class="animate-fade-up">
        <div class="rounded-2xl bg-white shadow-sm border border-slate-200/60 p-2 space-y-1 md:sticky md:top-4">
          <button
            v-for="s in subjects"
            :key="s"
            type="button"
            class="w-full flex items-center justify-between rounded-xl px-3 py-2.5 text-sm font-medium transition-colors"
            :class="activeSubject === s ? 'bg-primary text-white shadow-sm' : 'text-ink-soft hover:bg-slate-50'"
            @click="activeSubject = s"
          >
            <span>{{ s }}</span>
            <span class="text-xs" :class="activeSubject === s ? 'text-white/80' : 'text-ink-faint'">
              {{ subjectCount(s) }}
            </span>
          </button>
        </div>
      </aside>

      <!-- 右侧按年级分组 -->
      <div class="space-y-6">
        <div v-if="loading" class="flex items-center justify-center py-20 text-ink-faint">
          <Loader2 class="w-6 h-6 animate-spin mr-2" />
          <span>加载中…</span>
        </div>

        <div v-else-if="!items.length" class="text-center py-20 text-ink-faint">
          <BookOpen class="w-12 h-12 mx-auto mb-3 opacity-40" />
          <p v-if="hasActiveFilter">没有匹配的错题，试试调整关键词或筛选项</p>
          <p v-else>还没有错题，点击「录入错题」开始</p>
        </div>

        <section v-for="group in groupedItems" :key="group.grade" class="animate-fade-up">
          <div class="flex items-center gap-2 mb-3">
            <h2 class="font-semibold text-ink">{{ group.grade }}</h2>
            <span class="text-xs text-ink-faint">{{ group.items.length }} 题</span>
          </div>

          <div class="space-y-4">
            <div
              v-for="item in group.items"
              :key="item.id"
              class="rounded-2xl bg-white p-5 shadow-sm border border-slate-200/60 hover:shadow-md transition-shadow"
            >
              <div class="flex items-start gap-3">
                <input
                  type="checkbox"
                  class="mt-1 rounded border-slate-300 text-primary focus:ring-primary/30 cursor-pointer shrink-0"
                  :checked="isSelected(item.id)"
                  :aria-label="`选择第 ${item.id} 道错题`"
                  @change="toggleSelect(item.id)"
                />
                <div class="w-10 h-10 rounded-xl bg-primary/10 flex items-center justify-center shrink-0">
                  <BookOpen class="w-5 h-5 text-primary" />
                </div>
                <div class="flex-1">
                  <div class="flex items-center gap-2 mb-1 flex-wrap">
                    <span class="text-xs font-medium text-primary bg-primary/10 px-2 py-0.5 rounded-full">{{ item.question?.subject ?? '未分类' }}</span>
                    <span class="text-xs text-ink-faint">{{ item.question?.question_type }}</span>
                    <span
                      v-if="mistakeSource(item)"
                      class="text-xs text-ink-soft bg-surface-tint px-2 py-0.5 rounded-full"
                    >来源：<HighlightText :text="mistakeSource(item)" /></span>
                    <span class="text-xs text-ink-faint">{{ item.recorded_at?.slice(0, 10) }}</span>
                  </div>
                  <!-- 题干：命中的关键词标黄 -->
                  <p class="text-sm text-ink leading-relaxed">
                    <HighlightText :text="item.question?.stem_text ?? ''" />
                  </p>
                  <!-- 知识点标签 -->
                  <div v-if="knowledgePoints(item.question).length" class="mt-2 flex gap-1.5 flex-wrap">
                    <span
                      v-for="(kp, i) in knowledgePoints(item.question)"
                      :key="i"
                      class="rounded-full bg-primary/10 text-primary px-2 py-0.5 text-xs font-medium"
                    >{{ kp }}</span>
                  </div>
                  <!-- 题目配图：点击放大查看；有图号的在图下方标注 -->
                  <div v-if="imageRefs(item.question).length" class="mt-2 flex gap-3 flex-wrap">
                    <figure
                      v-for="(fig, i) in imageRefs(item.question)"
                      :key="i"
                      class="flex flex-col items-center gap-1"
                    >
                      <button
                        type="button"
                        class="block group focus:outline-none focus:ring-2 focus:ring-primary/30 rounded-lg"
                        title="点击放大查看"
                        @click="openViewer(figureUrl(fig.key))"
                      >
                        <img
                          :src="figureUrl(fig.key)"
                          class="h-20 rounded-lg border border-slate-200 object-contain bg-white cursor-zoom-in transition-transform group-hover:scale-[1.03]"
                          :alt="fig.label || '题目配图'"
                        />
                      </button>
                      <figcaption v-if="fig.label" class="text-xs text-ink-faint">{{ fig.label }}</figcaption>
                    </figure>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </section>

        <!-- 还有未加载的错题时提供「加载更多」（每次追加一页） -->
        <div v-if="hasMore" class="flex justify-center pt-2 animate-fade-up">
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-xl border border-slate-200 bg-white px-4 py-2.5 text-sm font-medium text-ink-soft shadow-sm hover:bg-slate-50 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            :disabled="loadingMore"
            @click="loadMore"
          >
            <Loader2 v-if="loadingMore" class="w-4 h-4 animate-spin" />
            加载更多（剩余 {{ total - items.length }} 条）
          </button>
        </div>
      </div>
    </div>

    <!-- 题目配图放大查看器 -->
    <ImageViewer :src="viewerSrc" :open="viewerOpen" @close="closeViewer" />
  </div>
</template>
