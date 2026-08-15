<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { BookOpen, Plus, Search, Filter } from 'lucide-vue-next'
import { useRouter } from 'vue-router'

const router = useRouter()
const keyword = ref('')

// 示例错题数据（后续接入真实接口）。
interface MistakeItem {
  id: number
  subject: string
  stem: string
  difficulty: number
  category: string
  date: string
}

const items = ref<MistakeItem[]>([
  { id: 1, subject: '数学', stem: '已知二次函数 y = x² - 2x - 3，求其顶点坐标与对称轴。', difficulty: 3, category: '二次函数', date: '2026-08-10' },
  { id: 2, subject: '数学', stem: '在直角三角形 ABC 中，∠C=90°，AC=3，BC=4，求 AB 的长。', difficulty: 2, category: '勾股定理', date: '2026-08-09' },
  { id: 3, subject: '物理', stem: '一个质量为 2kg 的物体在水平面上受到 10N 拉力，求加速度。', difficulty: 2, category: '牛顿定律', date: '2026-08-08' },
])

const difficultyLabels = ['', '很简单', '简单', '中等', '较难', '很难']

onMounted(() => {
  // 预留：从接口加载错题列表。
})

function goEntry() {
  router.push('/entry')
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
      <div
        v-for="item in items"
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
                <span class="text-xs font-medium text-primary bg-primary/10 px-2 py-0.5 rounded-full">{{ item.subject }}</span>
                <span class="text-xs text-ink-faint">{{ item.category }}</span>
                <span class="text-xs text-ink-faint">{{ item.date }}</span>
              </div>
              <p class="text-sm text-ink leading-relaxed">{{ item.stem }}</p>
            </div>
          </div>
          <span
            class="text-xs font-medium shrink-0 px-2.5 py-1 rounded-full"
            :class="item.difficulty >= 4 ? 'bg-red-50 text-red-500' : item.difficulty === 3 ? 'bg-amber-50 text-amber-600' : 'bg-emerald-50 text-emerald-600'"
          >
            {{ difficultyLabels[item.difficulty] }}
          </span>
        </div>
      </div>

      <div v-if="!items.length" class="text-center py-20 text-ink-faint">
        <BookOpen class="w-12 h-12 mx-auto mb-3 opacity-40" />
        <p>还没有错题，点击「录入错题」开始</p>
      </div>
    </div>
  </div>
</template>
