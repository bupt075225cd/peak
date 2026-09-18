<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { BookOpen, Plus, Search, Loader2, Download } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import { listMistakes, exportMistakes, type Mistake, type ExportFormat } from '../api'
import ImageViewer from '../components/ImageViewer.vue'

const router = useRouter()
const keyword = ref('')
const loading = ref(false)
const items = ref<Mistake[]>([])
const activeSubject = ref('全部')

// 年级固定顺序（与录入页预置选项一致）。
const gradeOrder = ['七年级上', '七年级下', '八年级上', '八年级下', '九年级上', '九年级下']

// 学科列表：全部 + 数据中实际出现的学科。
const subjects = computed(() => {
  const set = new Set<string>()
  for (const m of items.value) {
    const s = m.question?.subject
    if (s) set.add(s)
  }
  return ['全部', ...set]
})

// 某学科下的错题数量。
function subjectCount(subject: string): number {
  if (subject === '全部') return items.value.length
  return items.value.filter((m) => (m.question?.subject ?? '') === subject).length
}

// 按学科 + 关键词过滤。
const filteredItems = computed(() => {
  const kw = keyword.value.trim()
  return items.value.filter((m) => {
    const subject = m.question?.subject ?? ''
    if (activeSubject.value !== '全部' && subject !== activeSubject.value) return false
    if (!kw) return true
    const stem = m.question?.stem_text ?? ''
    const kps = knowledgePoints(m.question).join(' ')
    return subject.includes(kw) || stem.includes(kw) || kps.includes(kw)
  })
})

// 按年级分组：固定顺序在前，无年级/未知年级归入「未分类」。
const groupedItems = computed(() => {
  const groups: { grade: string; items: Mistake[] }[] = []
  for (const g of gradeOrder) {
    const list = filteredItems.value.filter((m) => (m.question?.grade ?? '') === g)
    if (list.length) groups.push({ grade: g, items: list })
  }
  const ungrouped = filteredItems.value.filter((m) => {
    const g = m.question?.grade ?? ''
    return !g || !gradeOrder.includes(g)
  })
  if (ungrouped.length) groups.push({ grade: '未分类', items: ungrouped })
  return groups
})

onMounted(async () => {
  loading.value = true
  try {
    items.value = await listMistakes()
  } catch (err) {
    console.error('加载错题失败', err)
  } finally {
    loading.value = false
  }
})

function goEntry() {
  router.push('/entry')
}

// 解析题目的 image（JSON 字符串数组）为 image key 列表。
function imageKeys(q: Mistake['question']): string[] {
  if (!q?.image) return []
  try {
    const arr = JSON.parse(q.image)
    return Array.isArray(arr) ? arr.filter((x) => typeof x === 'string') : []
  } catch {
    return []
  }
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
  filteredItems.value.filter((m) => selectedIds.value.has(m.id)),
)

// 已勾选但被当前筛选排除的题目数量（导出时不会被包含）。
const hiddenSelectedCount = computed(() =>
  Math.max(0, selectedIds.value.size - selectedVisible.value.length),
)

// 导出目标：有勾选时只导出勾选项，否则导出当前筛选结果。
const exportTargets = computed(() =>
  selectedVisible.value.length > 0 ? selectedVisible.value : filteredItems.value,
)

const canExport = computed(() => !exporting.value && exportTargets.value.length > 0)

// 全选只作用于当前筛选结果，不跨筛选累加。
const allVisibleSelected = computed(
  () =>
    filteredItems.value.length > 0 &&
    filteredItems.value.every((m) => selectedIds.value.has(m.id)),
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
    for (const m of filteredItems.value) next.delete(m.id)
  } else {
    for (const m of filteredItems.value) next.add(m.id)
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
        <p class="text-sm text-ink-soft mt-1">共 {{ items.length }} 道错题</p>
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
    <div class="relative mb-6 animate-fade-up">
      <Search class="w-4 h-4 text-ink-faint absolute left-3 top-1/2 -translate-y-1/2" />
      <input
        v-model="keyword"
        class="w-full rounded-xl border border-slate-200 bg-white pl-10 pr-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
        placeholder="搜索错题、知识点"
      />
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
          :disabled="!filteredItems.length"
          @change="toggleSelectAllVisible"
        />
        全选当前筛选
      </label>
      <span v-if="selectedIds.size" class="text-xs text-ink-faint">
        已选 {{ selectedIds.size }} 题
        <template v-if="hiddenSelectedCount">
          （其中 {{ hiddenSelectedCount }} 题不在当前筛选结果中，不会被导出）
        </template>
      </span>
      <span v-else class="text-xs text-ink-faint">未勾选时导出当前筛选结果</span>
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
          <p>还没有错题，点击「录入错题」开始</p>
        </div>

        <div v-else-if="!groupedItems.length" class="text-center py-20 text-ink-faint">
          <BookOpen class="w-12 h-12 mx-auto mb-3 opacity-40" />
          <p>暂无匹配的错题</p>
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
                  <div class="flex items-center gap-2 mb-1">
                    <span class="text-xs font-medium text-primary bg-primary/10 px-2 py-0.5 rounded-full">{{ item.question?.subject ?? '未分类' }}</span>
                    <span class="text-xs text-ink-faint">{{ item.question?.question_type }}</span>
                    <span class="text-xs text-ink-faint">{{ item.recorded_at?.slice(0, 10) }}</span>
                  </div>
                  <p class="text-sm text-ink leading-relaxed">{{ item.question?.stem_text }}</p>
                  <!-- 知识点标签 -->
                  <div v-if="knowledgePoints(item.question).length" class="mt-2 flex gap-1.5 flex-wrap">
                    <span
                      v-for="(kp, i) in knowledgePoints(item.question)"
                      :key="i"
                      class="rounded-full bg-primary/10 text-primary px-2 py-0.5 text-xs font-medium"
                    >{{ kp }}</span>
                  </div>
                  <!-- 题目配图：点击放大查看 -->
                  <div v-if="imageKeys(item.question)" class="mt-2 flex gap-2 flex-wrap">
                    <button
                      v-for="(gk, i) in imageKeys(item.question)"
                      :key="i"
                      type="button"
                      class="block group focus:outline-none focus:ring-2 focus:ring-primary/30 rounded-lg"
                      title="点击放大查看"
                      @click="openViewer(`/api/recognition/files/${gk}`)"
                    >
                      <img
                        :src="`/api/recognition/files/${gk}`"
                        class="h-20 rounded-lg border border-slate-200 object-contain bg-white cursor-zoom-in transition-transform group-hover:scale-[1.03]"
                        alt="题目配图"
                      />
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </div>

    <!-- 题目配图放大查看器 -->
    <ImageViewer :src="viewerSrc" :open="viewerOpen" @close="closeViewer" />
  </div>
</template>
