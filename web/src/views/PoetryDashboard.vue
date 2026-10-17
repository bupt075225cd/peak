<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Search, Sparkles, BookMarked, ChevronRight } from 'lucide-vue-next'
import { useAuth } from '../composables/useAuth'
import { usePoetryStats, totalPoems } from '../composables/usePoetryStats'
import { listPoems, type Poem } from '../api/poetry'
import {
  dynastyBarOption,
  dynastyTimelineOption,
  volumeStackOption,
  authorBarOption,
  wordCloudOption,
} from '../components/poetry/chartOptions'
import ChartCard from '../components/poetry/ChartCard.vue'
import AiChatPlaceholder from '../components/poetry/AiChatPlaceholder.vue'

const router = useRouter()
const { isLoggedIn } = useAuth()
const { stats, loading, error, load } = usePoetryStats()

// 筛选状态：总览图表保持全量统计口径；选择条件后即时展示匹配篇数，
// 并可一键跳转列表页查看筛选结果（列表页支持继续调整筛选与关键词）。
const filterDynasty = ref('')
const filterVolume = ref('')
const filterCategory = ref('')

// 匹配篇数（null = 未选择任何筛选，不展示）；reqToken 防止快速切换时的竞态。
const matchCount = ref<number | null>(null)
const matching = ref(false)
let matchToken = 0
let matchTimer: ReturnType<typeof setTimeout> | undefined

async function fetchMatchCount() {
  const token = ++matchToken
  if (!filterDynasty.value && !filterVolume.value && !filterCategory.value) {
    matchCount.value = null
    return
  }
  matching.value = true
  try {
    const res = await listPoems({
      limit: 1,
      dynasty: filterDynasty.value || undefined,
      volume: filterVolume.value || undefined,
      category: filterCategory.value || undefined,
    })
    if (token === matchToken) matchCount.value = res.total
  } catch (err) {
    console.error('查询筛选匹配篇数失败', err)
    if (token === matchToken) matchCount.value = null
  } finally {
    if (token === matchToken) matching.value = false
  }
}

// 选择变化后 300ms 防抖查询（连续切换只发一次请求）。
watch([filterDynasty, filterVolume, filterCategory], () => {
  if (matchTimer) clearTimeout(matchTimer)
  matchTimer = setTimeout(fetchMatchCount, 300)
})

const total = computed(() => totalPoems(stats.value.dynasties))

// 各图表配置（纯函数构建，便于测试）。
const dynastyBar = computed(() => dynastyBarOption(stats.value.dynasties))
const dynastyTimeline = computed(() => dynastyTimelineOption(stats.value.dynasties))
const volumeStack = computed(() => volumeStackOption(stats.value.volumes))
const authorBar = computed(() => authorBarOption(stats.value.authors))
const wordCloud = computed(() => wordCloudOption(stats.value.words))

// 筛选在生效时跳转到列表页（总览图表保持全量统计口径）。
function goList() {
  const query: Record<string, string> = {}
  if (filterDynasty.value) query.dynasty = filterDynasty.value
  if (filterVolume.value) query.volume = filterVolume.value
  if (filterCategory.value) query.category = filterCategory.value
  router.push({ path: '/poetry/list', query })
}

const chatRef = ref<InstanceType<typeof AiChatPlaceholder> | null>(null)
// 智能对话需要登录（与错题本共用用户体系）：未登录引导去登录页并回跳本页。
function openChat() {
  if (!isLoggedIn.value) {
    router.push({ path: '/login', query: { redirect: '/poetry' } })
    return
  }
  chatRef.value?.show()
}

onMounted(load)
</script>

<template>
  <div class="min-h-screen bg-poetry-paper">
    <div class="mx-auto max-w-5xl px-4 py-8">
      <!-- 标题栏 -->
      <header class="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 class="font-serif text-2xl sm:text-3xl font-semibold text-poetry-inkstone flex items-center gap-2.5">
            <BookMarked class="w-7 h-7 text-poetry-cinnabar" />
            古诗文一览
          </h1>
          <p class="mt-1.5 text-sm text-[#8A8272]">
            统编版初中教材
            <span class="text-poetry-cinnabar font-semibold" data-test="poetry-total">{{ loading ? '…' : total }}</span>
            篇 · 先秦至清 · 课内外全收录
          </p>
        </div>
        <div class="flex items-center gap-2">
          <button
            class="inline-flex items-center gap-1.5 px-4 py-2.5 rounded-xl bg-poetry-cinnabar text-white text-sm font-semibold shadow-md shadow-poetry-cinnabar/20 hover:bg-poetry-cinnabarLight hover:shadow-lg transition-all cursor-pointer"
            data-test="open-ai-chat"
            @click="openChat"
          >
            <Sparkles class="w-4 h-4" />
            AI 问答
          </button>
          <button
            class="inline-flex items-center gap-1.5 px-4 py-2.5 rounded-xl border border-poetry-jade/30 text-poetry-jade bg-white/60 text-sm font-medium hover:bg-poetry-jade/5 transition-colors cursor-pointer"
            data-test="go-list"
            @click="goList"
          >
            <Search class="w-4 h-4" />
            检索篇目
          </button>
        </div>
      </header>

      <!-- 快速筛选条 -->
      <div class="mt-6 flex flex-wrap items-center gap-2" data-test="filter-bar">
        <select
          v-model="filterDynasty"
          class="px-3 py-2 rounded-lg border border-[#E0D9C8] bg-white text-sm text-poetry-inkstone outline-none focus:border-poetry-cinnabar cursor-pointer"
          data-test="filter-dynasty"
        >
          <option value="">全部朝代</option>
          <option v-for="d in stats.dynasties" :key="d.dynasty" :value="d.dynasty">
            {{ d.dynasty }}（{{ d.count }}）
          </option>
        </select>
        <select
          v-model="filterVolume"
          class="px-3 py-2 rounded-lg border border-[#E0D9C8] bg-white text-sm text-poetry-inkstone outline-none focus:border-poetry-cinnabar cursor-pointer"
          data-test="filter-volume"
        >
          <option value="">全部册数</option>
          <option v-for="v in stats.volumes" :key="v.volume" :value="v.volume">{{ v.volume }}</option>
        </select>
        <select
          v-model="filterCategory"
          class="px-3 py-2 rounded-lg border border-[#E0D9C8] bg-white text-sm text-poetry-inkstone outline-none focus:border-poetry-cinnabar cursor-pointer"
          data-test="filter-category"
        >
          <option value="">课内外全部</option>
          <option value="课内">课内</option>
          <option value="课外">课外</option>
        </select>
      </div>

      <!-- 筛选即时反馈：匹配篇数 + 一键查看 -->
      <div
        v-if="matchCount !== null"
        class="mt-3 inline-flex items-center gap-2 px-3.5 py-2 rounded-xl bg-poetry-cinnabar/8 border border-poetry-cinnabar/20"
        data-test="filter-match"
      >
        <span class="text-sm text-poetry-inkstone">
          当前筛选匹配
          <b class="text-poetry-cinnabar font-semibold" data-test="match-count">{{ matchCount }}</b>
          篇
        </span>
        <button
          class="inline-flex items-center gap-1 text-sm font-semibold text-poetry-cinnabar hover:text-poetry-cinnabarLight transition-colors cursor-pointer"
          data-test="go-filtered-list"
          @click="goList"
        >
          查看筛选结果
          <ChevronRight class="w-4 h-4" />
        </button>
      </div>

      <!-- 错误提示 -->
      <div
        v-if="error"
        class="mt-6 rounded-xl border border-poetry-crimson/20 bg-poetry-crimson/5 px-4 py-3 text-sm text-poetry-crimson"
        data-test="stats-error"
      >
        {{ error }}
      </div>

      <!-- 图表网格 -->
      <div class="mt-6 grid grid-cols-1 md:grid-cols-2 gap-4" data-test="chart-grid">
        <ChartCard
          title="朝代分布"
          subtitle="先秦 → 清"
          :option="dynastyBar"
          data-test="chart-dynasty-bar"
        />
        <ChartCard
          title="朝代时间线"
          subtitle="气泡大小 = 篇目数量"
          :option="dynastyTimeline"
        />
        <ChartCard
          title="册数 × 课内外分布"
          subtitle="七~九年级"
          :option="volumeStack"
        />
        <ChartCard
          title="作者作品数 Top 10"
          :option="authorBar"
        />
        <div class="md:col-span-2">
          <ChartCard
            title="高频字词词云"
            :subtitle="stats.totalChars ? `全文共 ${stats.totalChars} 字` : ''"
            :option="wordCloud"
            height="h-72"
          />
        </div>
      </div>

      <!-- 列表预览骨架 -->
      <div v-if="loading" class="mt-6 grid grid-cols-1 md:grid-cols-2 gap-4" data-test="loading-skeleton">
        <div v-for="i in 4" :key="i" class="h-64 rounded-2xl bg-[#EFE9DB] animate-pulse" />
      </div>
    </div>

    <AiChatPlaceholder ref="chatRef" />
  </div>
</template>
