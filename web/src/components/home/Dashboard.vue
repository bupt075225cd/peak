<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Camera, BookOpen, Layers, FolderOpen, Loader2, ChevronRight, PlusCircle } from 'lucide-vue-next'
import { listMistakes, type Mistake } from '../../api'

// 错题总数与学科/来源分面计数、最近 5 条错题，一次请求同时获得。
const RECENT_COUNT = 5

const loading = ref(true)
const total = ref(0)
const subjectCounts = ref<Record<string, number>>({})
const sourceCounts = ref<Record<string, number>>({})
const recent = ref<Mistake[]>([])

// 统计卡片。
const stats = computed(() => [
  { icon: BookOpen, label: '错题总数', value: total.value, tint: 'bg-primary/10 text-primary' },
  { icon: Layers, label: '覆盖学科', value: Object.keys(subjectCounts.value).length, tint: 'bg-emerald-50 text-emerald-600' },
  { icon: FolderOpen, label: '错题来源', value: Object.keys(sourceCounts.value).length, tint: 'bg-amber-50 text-amber-600' },
])

// 学科分布（按数量降序），用于条形图渲染。
const subjectBars = computed(() => {
  const entries = Object.entries(subjectCounts.value).sort((a, b) => b[1] - a[1])
  const max = entries[0]?.[1] ?? 0
  return entries.map(([name, count]) => ({ name, count, pct: max ? (count / max) * 100 : 0 }))
})

// 按时段问候。
const greeting = computed(() => {
  const h = new Date().getHours()
  if (h < 6) return '夜深了'
  if (h < 12) return '早上好'
  if (h < 18) return '下午好'
  return '晚上好'
})

// 当天日期（如「2026年9月28日 星期一」）。
const today = computed(() => {
  const d = new Date()
  const week = ['日', '一', '二', '三', '四', '五', '六']
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日 星期${week[d.getDay()]}`
})

// 录入日期展示（如「09-28」）。
function formatDate(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

// 题干摘要：压缩空白，供两行截断展示。
function stemOf(m: Mistake): string {
  return (m.question?.stem_text ?? '').replace(/\s+/g, ' ').trim() || '（未填写题干）'
}

onMounted(async () => {
  try {
    const res = await listMistakes({ offset: 0, limit: RECENT_COUNT })
    total.value = res.total
    subjectCounts.value = res.subjectCounts
    sourceCounts.value = res.sourceCounts
    recent.value = res.items
  } catch (err) {
    console.error('加载主页统计数据失败', err)
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="mx-auto max-w-5xl px-4 py-8">
    <!-- 欢迎横幅 -->
    <section
      class="relative overflow-hidden rounded-2xl bg-gradient-to-r from-primary to-primary-light p-8 text-white shadow-xl shadow-blue-500/20 animate-fade-up"
    >
      <h1 class="text-2xl font-semibold tracking-tight">{{ greeting }}，继续加油！</h1>
      <p class="mt-1.5 text-sm text-blue-100">{{ today }} · 温故而知新，今天也来复习几道错题吧。</p>
      <RouterLink
        to="/entry"
        class="mt-5 inline-flex items-center gap-2 px-5 py-2.5 rounded-xl bg-white/15 border border-white/25 backdrop-blur text-sm font-semibold hover:bg-white/25 transition-colors cursor-pointer"
      >
        <Camera class="w-4 h-4" />
        录入新错题
      </RouterLink>
    </section>

    <!-- 加载态 -->
    <div v-if="loading" class="flex justify-center py-16">
      <Loader2 class="w-6 h-6 text-primary animate-spin" />
    </div>

    <template v-else>
      <!-- 统计卡片 -->
      <section class="mt-6 grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div
          v-for="(s, i) in stats"
          :key="s.label"
          class="bg-white rounded-2xl border border-slate-200/80 p-5 shadow-sm flex items-center gap-4 animate-fade-up"
          :style="{ animationDelay: `${i * 80}ms` }"
        >
          <div :class="[s.tint, 'w-11 h-11 rounded-xl flex items-center justify-center shrink-0']">
            <component :is="s.icon" class="w-5 h-5" />
          </div>
          <div>
            <div class="text-2xl font-semibold text-ink tabular-nums">{{ s.value }}</div>
            <div class="text-xs text-ink-faint mt-0.5">{{ s.label }}</div>
          </div>
        </div>
      </section>

      <div class="mt-6 grid grid-cols-1 lg:grid-cols-5 gap-6">
        <!-- 学科分布 -->
        <section
          class="lg:col-span-2 bg-white rounded-2xl border border-slate-200/80 p-6 shadow-sm animate-fade-up [animation-delay:160ms]"
        >
          <h2 class="text-base font-semibold text-ink">学科分布</h2>
          <p class="mt-1 text-xs text-ink-faint">薄弱学科一目了然</p>
          <div v-if="subjectBars.length" class="mt-5 space-y-4">
            <div v-for="bar in subjectBars" :key="bar.name">
              <div class="flex items-center justify-between text-sm">
                <span class="font-medium text-ink">{{ bar.name }}</span>
                <span class="text-ink-faint tabular-nums">{{ bar.count }}</span>
              </div>
              <div class="mt-1.5 h-2 rounded-full bg-slate-100 overflow-hidden">
                <div
                  class="h-full rounded-full bg-gradient-to-r from-primary to-primary-light transition-all duration-700"
                  :style="{ width: `${bar.pct}%` }"
                />
              </div>
            </div>
          </div>
          <div v-else class="mt-5 py-6 text-center text-sm text-ink-faint">暂无数据</div>
        </section>

        <!-- 最近录入 -->
        <section
          class="lg:col-span-3 bg-white rounded-2xl border border-slate-200/80 p-6 shadow-sm animate-fade-up [animation-delay:240ms]"
        >
          <div class="flex items-center justify-between">
            <div>
              <h2 class="text-base font-semibold text-ink">最近录入</h2>
              <p class="mt-1 text-xs text-ink-faint">最新收录的错题</p>
            </div>
            <RouterLink
              v-if="total > 0"
              to="/list"
              class="inline-flex items-center gap-0.5 text-sm font-medium text-primary hover:text-primary-light transition-colors cursor-pointer"
            >
              查看全部
              <ChevronRight class="w-4 h-4" />
            </RouterLink>
          </div>

          <!-- 空状态 -->
          <div v-if="recent.length === 0" class="mt-6 py-10 text-center">
            <div class="w-14 h-14 mx-auto rounded-2xl bg-surface-tint flex items-center justify-center">
              <PlusCircle class="w-7 h-7 text-primary" />
            </div>
            <p class="mt-4 text-sm font-medium text-ink">还没有错题</p>
            <p class="mt-1 text-xs text-ink-faint">拍下第一道错题，开始你的进步之旅</p>
            <RouterLink
              to="/entry"
              class="mt-5 inline-flex items-center gap-2 px-5 py-2.5 rounded-xl bg-primary text-white text-sm font-semibold shadow-lg shadow-blue-500/25 hover:bg-primary-light transition-colors cursor-pointer"
            >
              <Camera class="w-4 h-4" />
              去录入
            </RouterLink>
          </div>

          <!-- 最近错题列表 -->
          <ul v-else class="mt-4 divide-y divide-slate-100">
            <li v-for="m in recent" :key="m.id">
              <RouterLink
                to="/list"
                class="flex items-center gap-3 py-3.5 group cursor-pointer"
              >
                <span
                  class="shrink-0 px-2 py-1 rounded-md bg-primary/10 text-primary text-xs font-medium"
                >{{ m.question?.subject || '未分类' }}</span>
                <span class="flex-1 min-w-0 text-sm text-ink-soft leading-snug line-clamp-2 group-hover:text-ink transition-colors">
                  {{ stemOf(m) }}
                </span>
                <span class="shrink-0 text-xs text-ink-faint tabular-nums">{{ formatDate(m.recorded_at) }}</span>
              </RouterLink>
            </li>
          </ul>
        </section>
      </div>
    </template>
  </div>
</template>
