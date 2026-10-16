<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Sparkles, Feather } from 'lucide-vue-next'
import { getPoem, getRelatedPoems, type Poem } from '../api/poetry'
import AiChatPlaceholder from '../components/poetry/AiChatPlaceholder.vue'

const route = useRoute()
const router = useRouter()

const poem = ref<Poem | null>(null)
const related = ref<Poem[]>([])
const loading = ref(true)
const error = ref('')

async function load(id: number) {
  loading.value = true
  error.value = ''
  poem.value = null
  related.value = []
  try {
    poem.value = await getPoem(id)
    related.value = await getRelatedPoems(id)
  } catch (err) {
    console.error('加载篇目详情失败', err)
    const data = (err as { response?: { data?: { message?: string } } })?.response?.data
    error.value = data?.message || '篇目加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

// 正文按段落分行渲染。
const paragraphs = () =>
  poem.value?.content.split('\n').filter((p) => p.trim()) ?? []

function backToList() {
  router.push('/poetry/list')
}

const chatRef = ref<InstanceType<typeof AiChatPlaceholder> | null>(null)
function openChat() {
  chatRef.value?.show()
}

onMounted(() => {
  const id = Number(route.params.id)
  if (Number.isFinite(id) && id > 0) load(id)
  else loading.value = false
})

watch(
  () => route.params.id,
  (id) => {
    const n = Number(id)
    if (Number.isFinite(n) && n > 0) load(n)
  },
)
</script>

<template>
  <div class="min-h-screen bg-poetry-paper">
    <div class="mx-auto max-w-3xl px-4 py-8">
      <!-- 返回 -->
      <button
        class="inline-flex items-center gap-1.5 text-sm text-[#8A8272] hover:text-poetry-cinnabar transition-colors cursor-pointer"
        data-test="back-to-list"
        @click="backToList"
      >
        <ArrowLeft class="w-4 h-4" />
        返回列表
      </button>

      <!-- 加载态 -->
      <div v-if="loading" class="mt-6 space-y-4" data-test="detail-skeleton">
        <div class="h-10 w-48 rounded-lg bg-[#EFE9DB] animate-pulse" />
        <div class="h-64 rounded-2xl bg-[#EFE9DB] animate-pulse" />
      </div>

      <!-- 错误态 -->
      <div
        v-else-if="error || !poem"
        class="mt-6 rounded-xl border border-poetry-crimson/20 bg-poetry-crimson/5 px-4 py-3 text-sm text-poetry-crimson"
        data-test="detail-error"
      >
        {{ error || '篇目不存在' }}
      </div>

      <!-- 详情 -->
      <template v-else>
        <header class="mt-5">
          <h1 class="font-serif text-3xl font-semibold text-poetry-inkstone" data-test="poem-title">
            {{ poem.title }}
          </h1>
          <div class="mt-3 flex flex-wrap items-center gap-1.5 text-xs" data-test="poem-meta">
            <span class="px-2.5 py-1 rounded-full bg-poetry-cinnabar/10 text-poetry-cinnabar">{{ poem.dynasty }}</span>
            <span class="px-2.5 py-1 rounded-full bg-poetry-inkstone/8 text-poetry-inkstone">{{ poem.author }}</span>
            <span class="px-2.5 py-1 rounded-full bg-poetry-jade/10 text-poetry-jade">{{ poem.volume }}</span>
            <span
              v-if="poem.category"
              class="px-2.5 py-1 rounded-full bg-poetry-gold/15 text-[#8A6D14]"
            >{{ poem.category }}</span>
            <span class="ml-1 text-[#8A8272]">{{ poem.word_count }} 字</span>
          </div>
        </header>

        <!-- 正文 -->
        <article
          class="mt-6 rounded-2xl border border-[#E5DFD0] bg-white/70 px-6 py-8 sm:px-10 sm:py-10 shadow-sm"
          data-test="poem-content"
        >
          <Feather class="w-5 h-5 text-poetry-gold mb-4" />
          <div class="font-serif text-poetry-inkstone text-[17px] leading-loose sm:text-lg sm:leading-[2.2] space-y-4">
            <p v-for="(p, i) in paragraphs()" :key="i">{{ p }}</p>
          </div>
        </article>

        <!-- AI 对话占位入口 -->
        <button
          class="mt-6 w-full inline-flex items-center justify-center gap-2 px-4 py-3.5 rounded-xl border border-poetry-cinnabar/25 bg-poetry-cinnabar/5 text-poetry-cinnabar text-sm font-medium hover:bg-poetry-cinnabar/10 transition-colors cursor-pointer"
          data-test="detail-open-chat"
          @click="openChat"
        >
          <Sparkles class="w-4 h-4" />
          有疑问？问问 AI（即将上线）
        </button>

        <!-- 相关篇目 -->
        <section v-if="related.length" class="mt-8" data-test="related-poems">
          <h2 class="font-serif text-lg font-semibold text-poetry-inkstone">相关篇目</h2>
          <div class="mt-3 grid grid-cols-1 sm:grid-cols-2 gap-3">
            <RouterLink
              v-for="r in related"
              :key="r.id"
              :to="`/poetry/detail/${r.id}`"
              class="rounded-xl border border-[#E5DFD0] bg-white/80 px-4 py-3 transition-all hover:border-poetry-cinnabar/40 hover:shadow-md"
            >
              <h3 class="font-serif text-sm font-semibold text-poetry-inkstone">{{ r.title }}</h3>
              <p class="mt-1 text-xs text-[#8A8272]">{{ r.dynasty }} · {{ r.author }}</p>
            </RouterLink>
          </div>
        </section>
      </template>
    </div>

    <AiChatPlaceholder ref="chatRef" />
  </div>
</template>
