<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  Camera, Upload, RefreshCw, Check, X, ImagePlus, Loader2, Sparkles, BookOpen,
} from 'lucide-vue-next'
import {
  uploadImage, getTask, retryTask, listCategories, createMistake,
  type RecognitionTask, type RecognitionResult, type Category,
} from '../api'

// 上传与识别状态。
const fileInput = ref<HTMLInputElement | null>(null)
const previewUrl = ref('')
const task = ref<RecognitionTask | null>(null)
const recognizing = ref(false)
const loading = ref(false)
const errorMsg = ref('')
const saveError = ref('')

// 识别结果（可手动修正）。
const stemText = ref('')
const answer = ref('')
const analysis = ref('')
const formula = ref('')
const geometry = ref('')
const erasedImageUrl = ref('')

// 分类与难度。
const categories = ref<Category[]>([])
const selectedCategories = ref<number[]>([])
const difficulty = ref(2)
const questionType = ref('解答')
const subject = ref('数学')

const difficultyLabels = ['', '很简单', '简单', '中等', '较难', '很难']

const progressText = computed(() => {
  if (!task.value) return ''
  const map: Record<string, string> = {
    pending: '排队中…',
    processing: `识别中 ${task.value.progress}%`,
    success: '识别完成',
    failed: '识别失败',
  }
  return map[task.value.status] ?? ''
})

onMounted(async () => {
  categories.value = await listCategories()
})

function pickImage() {
  fileInput.value?.click()
}

function onFileChange(e: Event) {
  const target = e.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return
  previewUrl.value = URL.createObjectURL(file)
  startRecognition(file)
}

function onDrop(e: DragEvent) {
  const file = e.dataTransfer?.files?.[0]
  if (!file) return
  previewUrl.value = URL.createObjectURL(file)
  startRecognition(file)
}

async function startRecognition(file: File) {
  recognizing.value = true
  errorMsg.value = ''
  try {
    task.value = await uploadImage(file)
    await pollTask(task.value.id)
  } catch (err) {
    console.error('上传识别失败', err)
    errorMsg.value = '图片上传失败，请重试'
  } finally {
    recognizing.value = false
  }
}

async function pollTask(id: number) {
  // 轮询识别任务，直到完成或失败。
  for (let i = 0; i < 30; i++) {
    await new Promise((r) => setTimeout(r, 1000))
    const t = await getTask(id)
    task.value = t
    if (t.status === 'success') {
      applyResult(t)
      return
    }
    if (t.status === 'failed') {
      errorMsg.value = t.error_message || '识别失败，请重试'
      return
    }
  }
  errorMsg.value = '识别超时，请重试'
}

function applyResult(t: RecognitionTask) {
  if (!t.result_json) return
  const result = JSON.parse(t.result_json) as RecognitionResult
  stemText.value = result.stem_text || ''
  answer.value = result.answer || ''
  formula.value = result.formula?.latex || ''
  geometry.value = result.geometry?.description || ''
  if (result.erased_image_key) {
    erasedImageUrl.value = `/api/recognition/files/${result.erased_image_key}`
  }
}

async function handleRetry() {
  if (!task.value) return
  recognizing.value = true
  errorMsg.value = ''
  try {
    await retryTask(task.value.id)
    await pollTask(task.value.id)
  } catch (err) {
    console.error('重试失败', err)
  } finally {
    recognizing.value = false
  }
}

async function handleSave() {
  saveError.value = ''
  if (!stemText.value.trim()) {
    saveError.value = '题干不能为空'
    return
  }
  loading.value = true
  try {
    await createMistake({
      user_id: 1,
      subject: subject.value,
      stem_text: stemText.value,
      answer: answer.value,
      analysis: analysis.value,
      stem_formula: JSON.stringify({ latex: formula.value }),
      geometry_refs: JSON.stringify({ description: geometry.value }),
      difficulty: difficulty.value,
      question_type: questionType.value,
      category_ids: selectedCategories.value,
    })
    alert('错题已保存')
    reset()
  } catch (err) {
    console.error('保存失败', err)
    saveError.value = '保存失败，请重试'
  } finally {
    loading.value = false
  }
}

function reset() {
  task.value = null
  previewUrl.value = ''
  stemText.value = ''
  answer.value = ''
  analysis.value = ''
  formula.value = ''
  geometry.value = ''
  erasedImageUrl.value = ''
  selectedCategories.value = []
  difficulty.value = 2
  errorMsg.value = ''
  saveError.value = ''
}

function toggleCategory(id: number) {
  const idx = selectedCategories.value.indexOf(id)
  if (idx >= 0) selectedCategories.value.splice(idx, 1)
  else selectedCategories.value.push(id)
}
</script>

<template>
  <div class="mx-auto max-w-5xl px-4 py-8">
    <div class="mb-6 animate-fade-up">
      <h1 class="text-2xl font-semibold text-ink">录入错题</h1>
      <p class="text-sm text-ink-soft mt-1">拍照上传纸质试卷，自动识别题干、公式与几何图形</p>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-5 gap-6">
      <!-- 左侧：拍照上传 + 识别进度 -->
      <div class="lg:col-span-2 space-y-6">
        <div
          class="relative rounded-2xl border-2 border-dashed border-slate-300 bg-white overflow-hidden transition-all hover:border-primary-light cursor-pointer group"
          @click="pickImage"
          @dragover.prevent
          @drop.prevent="onDrop"
        >
          <input ref="fileInput" type="file" accept="image/*" capture="environment" class="hidden" @change="onFileChange" />
          <div v-if="!previewUrl" class="flex flex-col items-center justify-center py-16 px-6 text-center">
            <div class="w-16 h-16 rounded-2xl bg-primary/10 flex items-center justify-center mb-4 group-hover:animate-float">
              <ImagePlus class="w-8 h-8 text-primary" />
            </div>
            <p class="font-medium text-ink">拍照或上传试卷图片</p>
            <p class="text-sm text-ink-faint mt-1">支持拖拽、点击上传，自动识别</p>
            <button
              type="button"
              class="mt-4 inline-flex items-center gap-2 rounded-xl bg-primary text-white px-4 py-2.5 text-sm font-medium shadow-lg shadow-blue-500/25 hover:bg-primary-light transition-colors"
            >
              <Camera class="w-4 h-4" /> 拍照 / 选择图片
            </button>
          </div>
          <div v-else class="relative">
            <img :src="previewUrl" class="w-full h-72 object-cover" alt="试卷预览" />
            <div class="absolute inset-0 bg-black/20 flex items-center justify-center">
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-xl bg-white/90 text-ink px-4 py-2 text-sm font-medium hover:bg-white transition-colors"
                @click.stop="pickImage"
              >
                <Upload class="w-4 h-4" /> 重新选择
              </button>
            </div>
          </div>
        </div>

        <!-- 识别进度卡片 -->
        <div v-if="recognizing || task" class="rounded-2xl bg-white p-5 shadow-sm border border-slate-200/60 animate-fade-up">
          <div class="flex items-center gap-3">
            <div v-if="recognizing" class="w-10 h-10 rounded-full bg-blue-50 flex items-center justify-center">
              <Loader2 class="w-5 h-5 text-primary animate-spin" />
            </div>
            <div v-else-if="task?.status === 'success'" class="w-10 h-10 rounded-full bg-emerald-50 flex items-center justify-center">
              <Check class="w-5 h-5 text-emerald-500" />
            </div>
            <div v-else class="w-10 h-10 rounded-full bg-red-50 flex items-center justify-center">
              <X class="w-5 h-5 text-red-500" />
            </div>
            <div class="flex-1">
              <p class="font-medium text-ink">{{ progressText }}</p>
              <p class="text-xs text-ink-faint">识别服务：{{ task?.provider || '-' }}</p>
            </div>
            <button
              v-if="task?.status === 'failed'"
              type="button"
              class="inline-flex items-center gap-1.5 rounded-lg text-primary text-sm font-medium hover:bg-blue-50 px-3 py-1.5 transition-colors"
              @click="handleRetry"
            >
              <RefreshCw class="w-4 h-4" /> 重试
            </button>
          </div>
          <div v-if="recognizing" class="mt-4 h-1.5 rounded-full bg-slate-100 overflow-hidden">
            <div
              class="h-full rounded-full bg-gradient-to-r from-primary to-primary-light transition-all duration-500"
              :style="{ width: (task?.progress ?? 10) + '%' }"
            />
          </div>
          <p v-if="errorMsg" class="mt-3 text-sm text-red-500">{{ errorMsg }}</p>
        </div>

        <!-- 擦除后图片预览 -->
        <div v-if="erasedImageUrl" class="rounded-2xl bg-white p-5 shadow-sm border border-slate-200/60 animate-fade-up">
          <div class="flex items-center gap-2 mb-3">
            <Sparkles class="w-4 h-4 text-primary" />
            <h3 class="font-medium text-ink">手写擦除效果</h3>
          </div>
          <img :src="erasedImageUrl" class="w-full rounded-xl" alt="擦除后题目" />
        </div>
      </div>

      <!-- 右侧：识别结果 + 手动修正表单 -->
      <div class="lg:col-span-3 space-y-6">
        <div class="rounded-2xl bg-white shadow-sm border border-slate-200/60 p-6 animate-fade-up">
          <div class="flex items-center gap-2 mb-5">
            <BookOpen class="w-4 h-4 text-primary" />
            <h2 class="font-semibold text-ink">题目信息</h2>
          </div>

          <div class="space-y-4">
            <div>
              <label class="block text-sm font-medium text-ink-soft mb-1.5">题干</label>
              <textarea
                v-model="stemText"
                rows="4"
                class="w-full rounded-xl border border-slate-200 bg-surface-muted/50 px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 focus:border-primary transition-shadow resize-none"
                placeholder="识别出的题干将显示在这里，可手动修正"
              />
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <label class="block text-sm font-medium text-ink-soft mb-1.5">学科</label>
                <select v-model="subject" class="w-full rounded-xl border border-slate-200 bg-surface-muted/50 px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30">
                  <option>数学</option>
                  <option>物理</option>
                  <option>化学</option>
                </select>
              </div>
              <div>
                <label class="block text-sm font-medium text-ink-soft mb-1.5">题型</label>
                <select v-model="questionType" class="w-full rounded-xl border border-slate-200 bg-surface-muted/50 px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30">
                  <option>选择题</option>
                  <option>填空题</option>
                  <option>解答题</option>
                </select>
              </div>
            </div>

            <div>
              <label class="block text-sm font-medium text-ink-soft mb-1.5">难度：{{ difficultyLabels[difficulty] }}</label>
              <div class="flex gap-1.5">
                <button
                  v-for="n in 5"
                  :key="n"
                  type="button"
                  class="flex-1 h-9 rounded-lg text-sm font-medium transition-colors"
                  :class="n <= difficulty ? 'bg-primary text-white' : 'bg-slate-100 text-ink-faint hover:bg-slate-200'"
                  @click="difficulty = n"
                >
                  {{ n }}
                </button>
              </div>
            </div>

            <div v-if="formula" class="rounded-xl bg-surface-tint p-4">
              <label class="block text-sm font-medium text-ink-soft mb-1.5">识别公式（LaTeX）</label>
              <code class="text-sm text-primary">{{ formula }}</code>
            </div>

            <div v-if="geometry" class="rounded-xl bg-surface-tint p-4">
              <label class="block text-sm font-medium text-ink-soft mb-1.5">几何图形</label>
              <p class="text-sm text-ink">{{ geometry }}</p>
            </div>

            <div>
              <label class="block text-sm font-medium text-ink-soft mb-1.5">答案</label>
              <input
                v-model="answer"
                class="w-full rounded-xl border border-slate-200 bg-surface-muted/50 px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
                placeholder="答案"
              />
            </div>

            <div>
              <label class="block text-sm font-medium text-ink-soft mb-1.5">解析</label>
              <textarea
                v-model="analysis"
                rows="3"
                class="w-full rounded-xl border border-slate-200 bg-surface-muted/50 px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 transition-shadow resize-none"
                placeholder="解题思路与解析"
              />
            </div>

            <div>
              <label class="block text-sm font-medium text-ink-soft mb-1.5">分类标签</label>
              <div class="flex flex-wrap gap-2">
                <button
                  v-for="cat in categories"
                  :key="cat.id"
                  type="button"
                  class="rounded-full px-3 py-1.5 text-sm font-medium border transition-colors"
                  :class="selectedCategories.includes(cat.id) ? 'bg-primary text-white border-primary' : 'bg-white text-ink-soft border-slate-200 hover:border-primary-light'"
                  @click="toggleCategory(cat.id)"
                >
                  {{ cat.name }}
                </button>
                <span v-if="!categories.length" class="text-sm text-ink-faint">暂无分类，可先在错题本中添加</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 底部操作栏 -->
        <div class="space-y-3">
          <p v-if="saveError" class="text-sm text-red-500 text-right">{{ saveError }}</p>
          <div class="flex items-center justify-end gap-3">
            <button
              type="button"
              class="rounded-xl px-5 py-2.5 text-sm font-medium text-ink-soft border border-slate-200 hover:bg-slate-50 transition-colors"
              @click="reset"
            >
              取消
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl bg-primary text-white px-6 py-2.5 text-sm font-medium shadow-lg shadow-blue-500/25 hover:bg-primary-light transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              :disabled="loading || !stemText.trim()"
              @click="handleSave"
            >
              <Loader2 v-if="loading" class="w-4 h-4 animate-spin" />
              <Check v-else class="w-4 h-4" />
              保存错题
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
