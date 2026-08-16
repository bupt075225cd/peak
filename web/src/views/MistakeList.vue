<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { BookOpen, Plus, Search, Filter, Loader2 } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import { listMistakes, type Mistake } from '../api'
import ImageViewer from '../components/ImageViewer.vue'

const router = useRouter()
const keyword = ref('')
const loading = ref(false)
const items = ref<Mistake[]>([])

// 按关键词过滤（题干/学科）。
const filteredItems = computed(() => {
  const kw = keyword.value.trim()
  if (!kw) return items.value
  return items.value.filter((m) => {
    const subject = m.question?.subject ?? ''
    const stem = m.question?.stem_text ?? ''
    return subject.includes(kw) || stem.includes(kw)
  })
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
  <div class="mx-auto max-w-5xl px-4 py-8">
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

    <!-- 搜索与筛选 -->
    <div class="flex items-center gap-3 mb-6 animate-fade-up">
      <div class="relative flex-1">
        <Search class="w-4 h-4 text-ink-faint absolute left-3 top-1/2 -translate-y-1/2" />
        <input
          v-model="keyword"
          class="w-full rounded-xl border border-slate-200 bg-white pl-10 pr-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
          placeholder="搜索错题、知识点"
        />
      </div>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-xl border border-slate-200 bg-white px-4 py-2.5 text-sm font-medium text-ink-soft hover:bg-slate-50 transition-colors"
      >
        <Filter class="w-4 h-4" /> 筛选
      </button>
    </div>

    <!-- 错题卡片列表 -->
    <div class="space-y-4">
      <div v-if="loading" class="flex items-center justify-center py-20 text-ink-faint">
        <Loader2 class="w-6 h-6 animate-spin mr-2" />
        <span>加载中…</span>
      </div>

      <div
        v-for="item in filteredItems"
        :key="item.id"
        class="rounded-2xl bg-white p-5 shadow-sm border border-slate-200/60 hover:shadow-md transition-shadow animate-fade-up"
      >
        <div class="flex items-start justify-between gap-4">
          <div class="flex items-start gap-3 flex-1">
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

      <div v-if="!loading && !filteredItems.length" class="text-center py-20 text-ink-faint">
        <BookOpen class="w-12 h-12 mx-auto mb-3 opacity-40" />
        <p>还没有错题，点击「录入错题」开始</p>
      </div>
    </div>

    <!-- 几何图放大查看器 -->
    <ImageViewer :src="viewerSrc" :open="viewerOpen" @close="closeViewer" />
  </div>
</template>
