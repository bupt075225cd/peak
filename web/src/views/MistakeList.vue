<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { BookOpen, Plus, Search, Loader2 } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import { listMistakes, type Mistake } from '../api'
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
    const remark = m.remark ?? ''
    return subject.includes(kw) || stem.includes(kw) || remark.includes(kw)
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

// 解析题目的 geometry_refs（JSON 字符串数组）为 image key 列表。
function geometryKeys(q: Mistake['question']): string[] {
  if (!q?.geometry_refs) return []
  try {
    const arr = JSON.parse(q.geometry_refs)
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
</script>

<template>
  <div class="mx-auto max-w-6xl px-4 py-8">
    <div class="flex items-center justify-between mb-6 animate-fade-up">
      <div>
        <h1 class="text-2xl font-semibold text-ink">我的错题本</h1>
        <p class="text-sm text-ink-soft mt-1">共 {{ items.length }} 道错题</p>
      </div>
      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-xl bg-primary text-white px-4 py-2.5 text-sm font-medium shadow-lg shadow-blue-500/25 hover:bg-primary-light transition-colors"
        @click="goEntry"
      >
        <Plus class="w-4 h-4" /> 录入错题
      </button>
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
                  <!-- 备注 -->
                  <p v-if="item.remark" class="mt-2 text-xs text-ink-soft">备注：{{ item.remark }}</p>
                  <!-- 几何图形：点击放大查看 -->
                  <div v-if="geometryKeys(item.question)" class="mt-2 flex gap-2 flex-wrap">
                    <button
                      v-for="(gk, i) in geometryKeys(item.question)"
                      :key="i"
                      type="button"
                      class="block group focus:outline-none focus:ring-2 focus:ring-primary/30 rounded-lg"
                      title="点击放大查看"
                      @click="openViewer(`/api/recognition/files/${gk}`)"
                    >
                      <img
                        :src="`/api/recognition/files/${gk}`"
                        class="h-20 rounded-lg border border-slate-200 object-contain bg-white cursor-zoom-in transition-transform group-hover:scale-[1.03]"
                        alt="几何图"
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

    <!-- 几何图放大查看器 -->
    <ImageViewer :src="viewerSrc" :open="viewerOpen" @close="closeViewer" />
  </div>
</template>
