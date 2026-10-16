<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Search, ChevronLeft, ChevronRight, ScrollText } from 'lucide-vue-next'
import { listPoems, type Poem } from '../api/poetry'

const route = useRoute()
const router = useRouter()

const DYNASTIES = ['先秦', '汉', '三国', '晋', '南北朝', '唐', '宋', '元', '明', '清']
const VOLUMES = [
  '七年级上册', '七年级下册', '八年级上册', '八年级下册', '九年级上册', '九年级下册',
]
const CATEGORIES = ['课内', '课外']

const keyword = ref('')
const dynasty = ref(typeof route.query.dynasty === 'string' ? route.query.dynasty : '')
const volume = ref(typeof route.query.volume === 'string' ? route.query.volume : '')
const category = ref(typeof route.query.category === 'string' ? route.query.category : '')

const items = ref<Poem[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref('')
const PAGE_SIZE = 20
const page = ref(1)

// 防抖全文检索 + 多维筛选。
let timer: ReturnType<typeof setTimeout> | null = null

async function fetchList() {
  loading.value = true
  error.value = ''
  try {
    const result = await listPoems({
      offset: (page.value - 1) * PAGE_SIZE,
      limit: PAGE_SIZE,
      dynasty: dynasty.value || undefined,
      volume: volume.value || undefined,
      category: category.value || undefined,
      keyword: keyword.value || undefined,
    })
    items.value = result.items
    total.value = result.total
  } catch (err) {
    console.error('加载篇目列表失败', err)
    const data = (err as { response?: { data?: { message?: string } } })?.response?.data
    error.value = data?.message || '列表加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

watch([keyword, dynasty, volume, category], () => {
  page.value = 1
  if (timer) clearTimeout(timer)
  timer = setTimeout(fetchList, 300)
})

watch(page, fetchList)

const totalPages = ref(1)
watch(total, (t) => {
  totalPages.value = Math.max(1, Math.ceil(t / PAGE_SIZE))
})

function goDetail(poem: Poem) {
  router.push(`/poetry/detail/${poem.id}`)
}

function prevPage() {
  if (page.value > 1) page.value--
}
function nextPage() {
  if (page.value < totalPages.value) page.value++
}

onMounted(fetchList)
</script>

<template>
  <div class="min-h-screen bg-poetry-paper">
    <div class="mx-auto max-w-3xl px-4 py-8">
      <h1 class="font-serif text-2xl font-semibold text-poetry-inkstone">篇目检索</h1>
      <p class="mt-1 text-sm text-[#8A8272]">全文关键词检索，支持按朝代、册数、课内外组合筛选</p>

      <!-- 搜索与筛选 -->
      <div class="mt-5 space-y-3">
        <div class="relative">
          <Search class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-faint" />
          <input
            v-model="keyword"
            type="text"
            placeholder="搜索篇名、作者或正文关键词…"
            class="w-full pl-10 pr-4 py-2.5 rounded-xl border border-[#E0D9C8] bg-white text-sm text-ink placeholder:text-ink-faint outline-none focus:border-poetry-cinnabar focus:ring-2 focus:ring-poetry-cinnabar/15 transition-shadow"
            data-test="poetry-search"
          />
        </div>
        <div class="flex flex-wrap gap-2" data-test="poetry-filters">
          <select
            v-model="dynasty"
            class="px-3 py-2 rounded-lg border border-[#E0D9C8] bg-white text-sm text-poetry-inkstone outline-none focus:border-poetry-cinnabar cursor-pointer"
            data-test="filter-dynasty"
          >
            <option value="">全部朝代</option>
            <option v-for="d in DYNASTIES" :key="d" :value="d">{{ d }}</option>
          </select>
          <select
            v-model="volume"
            class="px-3 py-2 rounded-lg border border-[#E0D9C8] bg-white text-sm text-poetry-inkstone outline-none focus:border-poetry-cinnabar cursor-pointer"
            data-test="filter-volume"
          >
            <option value="">全部册数</option>
            <option v-for="v in VOLUMES" :key="v" :value="v">{{ v }}</option>
          </select>
          <select
            v-model="category"
            class="px-3 py-2 rounded-lg border border-[#E0D9C8] bg-white text-sm text-poetry-inkstone outline-none focus:border-poetry-cinnabar cursor-pointer"
            data-test="filter-category"
          >
            <option value="">课内外全部</option>
            <option v-for="c in CATEGORIES" :key="c" :value="c">{{ c }}</option>
          </select>
        </div>
      </div>

      <!-- 错误提示 -->
      <div
        v-if="error"
        class="mt-4 rounded-xl border border-poetry-crimson/20 bg-poetry-crimson/5 px-4 py-3 text-sm text-poetry-crimson"
      >
        {{ error }}
      </div>

      <!-- 结果计数 -->
      <p class="mt-5 text-xs text-[#8A8272]" data-test="result-count">
        共 {{ total }} 篇{{ loading ? ' · 加载中…' : '' }}
      </p>

      <!-- 骨架屏 -->
      <div v-if="loading" class="mt-3 space-y-3" data-test="list-skeleton">
        <div v-for="i in 5" :key="i" class="h-20 rounded-xl bg-[#EFE9DB] animate-pulse" />
      </div>

      <!-- 结果列表 -->
      <ul v-else class="mt-3 space-y-3" data-test="poetry-list">
        <li v-for="poem in items" :key="poem.id">
          <button
            class="w-full text-left rounded-xl border border-[#E5DFD0] bg-white/80 px-4 py-3.5 transition-all hover:border-poetry-cinnabar/40 hover:shadow-md cursor-pointer"
            :data-test="`poem-item-${poem.id}`"
            @click="goDetail(poem)"
          >
            <div class="flex items-center justify-between gap-2">
              <h3 class="font-serif text-base font-semibold text-poetry-inkstone">{{ poem.title }}</h3>
              <ScrollText class="w-4 h-4 text-poetry-gold shrink-0" />
            </div>
            <div class="mt-1.5 flex flex-wrap items-center gap-1.5 text-xs">
              <span class="px-2 py-0.5 rounded-full bg-poetry-cinnabar/10 text-poetry-cinnabar">{{ poem.dynasty }}</span>
              <span class="px-2 py-0.5 rounded-full bg-poetry-inkstone/8 text-poetry-inkstone">{{ poem.author }}</span>
              <span class="px-2 py-0.5 rounded-full bg-poetry-jade/10 text-poetry-jade">{{ poem.volume }}</span>
              <span
                v-if="poem.category"
                class="px-2 py-0.5 rounded-full bg-poetry-gold/15 text-[#8A6D14]"
              >{{ poem.category }}</span>
            </div>
            <p class="mt-2 text-xs text-[#8A8272] leading-relaxed line-clamp-2">{{ poem.content }}</p>
          </button>
        </li>
      </ul>

      <!-- 空态 -->
      <div
        v-if="!loading && items.length === 0"
        class="mt-8 py-12 text-center text-sm text-[#8A8272]"
        data-test="empty-state"
      >
        未找到匹配的篇目，换个关键词试试
      </div>

      <!-- 分页 -->
      <div
        v-if="totalPages > 1"
        class="mt-6 flex items-center justify-center gap-4"
        data-test="pagination"
      >
        <button
          class="inline-flex items-center gap-1 px-3 py-2 rounded-lg text-sm text-poetry-inkstone hover:bg-black/5 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
          :disabled="page <= 1"
          data-test="prev-page"
          @click="prevPage"
        >
          <ChevronLeft class="w-4 h-4" />
          上一页
        </button>
        <span class="text-sm text-[#8A8272]">{{ page }} / {{ totalPages }}</span>
        <button
          class="inline-flex items-center gap-1 px-3 py-2 rounded-lg text-sm text-poetry-inkstone hover:bg-black/5 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
          :disabled="page >= totalPages"
          data-test="next-page"
          @click="nextPage"
        >
          下一页
          <ChevronRight class="w-4 h-4" />
        </button>
      </div>
    </div>
  </div>
</template>
