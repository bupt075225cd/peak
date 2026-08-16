<script setup lang="ts">
import { ref, computed } from 'vue'
import {
  Camera, Upload, RefreshCw, Check, X, ImagePlus, Loader2, BookOpen, FileText,
} from 'lucide-vue-next'
import {
  uploadImage, uploadDocument, isDocument, getTask, retryTask, createQuestion, createMistake,
  uploadGeometryImage, type RecognitionTask, type RecognitionResult, type QuestionItem,
} from '../api'
import LatexRenderer from '../components/LatexRenderer.vue'
import ImageViewer from '../components/ImageViewer.vue'
import GeometryCropper from '../components/GeometryCropper.vue'

// 上传与识别状态。
const fileInput = ref<HTMLInputElement | null>(null)
const previewUrl = ref('')
const docName = ref('')
const isDoc = ref(false)
const task = ref<RecognitionTask | null>(null)
const recognizing = ref(false)
const loading = ref(false)
const errorMsg = ref('')
const saveError = ref('')
const warningMsg = ref('')
const questions = ref<QuestionItem[]>([])

// 识别结果（可手动修正）。
const stemText = ref('')
const formula = ref('')
const geometry = ref('')
const selectedGeometryKeys = ref<string[]>([])

// 学科与题型。
const questionType = ref('解答')
const subject = ref('数学')

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

// 已选几何图（裁剪后的子图）的完整 URL 列表，供保存前预览。
const geometryImageUrls = computed(() =>
  selectedGeometryKeys.value.map((k) => `/api/recognition/files/${k}`),
)

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

// GeometryCropper 状态：手动重选几何图区域。
const cropperOpen = ref(false)
function openCropper() {
  cropperOpen.value = true
}
function closeCropper() {
  cropperOpen.value = false
}
// 手动裁剪完成后：上传裁剪图得到新 key，替换原几何图 key。
async function onCropperConfirm(file: File) {
  try {
    const key = await uploadGeometryImage(file)
    if (key) {
      selectedGeometryKeys.value = [key]
    }
  } catch (err) {
    console.error('上传裁剪几何图失败', err)
    saveError.value = '几何图上传失败，请重试'
  } finally {
    closeCropper()
  }
}

function pickImage() {
  fileInput.value?.click()
}

function onFileChange(e: Event) {
  const target = e.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return
  prepareFile(file)
  startRecognition(file)
}

function onDrop(e: DragEvent) {
  const file = e.dataTransfer?.files?.[0]
  if (!file) return
  prepareFile(file)
  startRecognition(file)
}

// 根据文件类型准备预览与状态。
function prepareFile(file: File) {
  isDoc.value = isDocument(file)
  if (isDoc.value) {
    docName.value = file.name
    previewUrl.value = ''
  } else {
    docName.value = ''
    previewUrl.value = URL.createObjectURL(file)
  }
}

async function startRecognition(file: File) {
  recognizing.value = true
  errorMsg.value = ''
  questions.value = []
  try {
    task.value = isDocument(file) ? await uploadDocument(file) : await uploadImage(file)
    await pollTask(task.value.id)
  } catch (err) {
    console.error('上传识别失败', err)
    errorMsg.value = isDocument(file) ? '文档上传失败，请重试' : '图片上传失败，请重试'
  } finally {
    recognizing.value = false
  }
}

async function pollTask(id: number) {
  // 轮询识别任务，直到完成或失败。
  // 文档拆题（多图 OCR + 结构化拆题）耗时较长，给到 180s。
  for (let i = 0; i < 180; i++) {
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
  // 180s 仍无结果时提示用户去手动查询/刷新，而不是显示“超时失败”，避免误判。
  errorMsg.value = '识别耗时较长，刷新页面查看结果，或点击重试'
}

function applyResult(t: RecognitionTask) {
  if (!t.result_json) return
  const result = JSON.parse(t.result_json) as RecognitionResult
  if (result.questions && result.questions.length > 0) {
    // 文档多题结果。
    questions.value = result.questions
  } else {
    // 图片单题结果。
    stemText.value = result.stem_text || ''
    formula.value = result.formula?.latex || ''
    geometry.value = result.geometry?.description || ''
    // 单图场景默认携带裁剪出的几何图 key。
    selectedGeometryKeys.value = Array.isArray(result.geometry_keys)
      ? result.geometry_keys.filter((k) => typeof k === 'string' && k.length > 0)
      : []
  }
  warningMsg.value = result.warning || ''
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
    // 第一步：创建题目本体。
    const question = await createQuestion({
      subject: subject.value,
      stem_text: stemText.value,
      stem_formula: JSON.stringify({ latex: formula.value }),
      geometry_refs: JSON.stringify(selectedGeometryKeys.value),
      question_type: questionType.value,
    })
    // 第二步：创建错题记录，关联刚创建的题目。
    await createMistake({
      user_id: 1,
      question_id: question.id,
      source_paper: '',
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
  docName.value = ''
  isDoc.value = false
  questions.value = []
  stemText.value = ''
  formula.value = ''
  geometry.value = ''
  selectedGeometryKeys.value = []
  errorMsg.value = ''
  saveError.value = ''
  warningMsg.value = ''
}

// 选中某道识别出的题，填入下方表单供修正/保存。
function selectQuestion(idx: number) {
  const q = questions.value[idx]
  if (!q) return
  // 题干：优先用 stem_text；若模型拆出子问，拼出完整题干（含子问 + 几何描述）。
  let stem = q.stem_text || ''
  if (q.sub_questions && q.sub_questions.length > 0) {
    const parts = q.sub_questions.map((sq) => {
      let s = `${sq.label} ${sq.text}`.trim()
      if (sq.geometry_desc) s += `（图：${sq.geometry_desc}）`
      return s
    })
    // 若 stem_text 已含子问文字则直接使用，否则拼接。
    const hasSub = q.sub_questions.some((sq) => stem.includes(sq.text))
    if (!hasSub && !stem) {
      stem = parts.join('\n')
    } else if (!hasSub) {
      stem = stem + '\n' + parts.join('\n')
    }
  }
  stemText.value = stem
  formula.value = q.formula?.latex || ''
  geometry.value = q.geometry?.description || ''
  // 收集该题所有子问的几何图片 key，用于保存到题目记录。
  selectedGeometryKeys.value = (q.sub_questions || [])
    .flatMap((sq) => sq.geometry_keys || [])
}
</script>

<template>
  <div class="mx-auto max-w-5xl px-4 py-8">
    <div class="mb-6 animate-fade-up">
      <h1 class="text-2xl font-semibold text-ink">录入错题</h1>
      <p class="text-sm text-ink-soft mt-1">拍照上传试卷图片，或上传 word/pdf 文档，自动识别题目</p>
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
          <input ref="fileInput" type="file" accept="image/*,.doc,.docx,.pdf" class="hidden" @change="onFileChange" />
          <div v-if="!previewUrl && !docName" class="flex flex-col items-center justify-center py-16 px-6 text-center">
            <div class="w-16 h-16 rounded-2xl bg-primary/10 flex items-center justify-center mb-4 group-hover:animate-float">
              <ImagePlus class="w-8 h-8 text-primary" />
            </div>
            <p class="font-medium text-ink">拍照 / 上传试卷图片或文档</p>
            <p class="text-sm text-ink-faint mt-1">支持图片、Word、PDF，拖拽或点击上传，自动识别</p>
            <button
              type="button"
              class="mt-4 inline-flex items-center gap-2 rounded-xl bg-primary text-white px-4 py-2.5 text-sm font-medium shadow-lg shadow-blue-500/25 hover:bg-primary-light transition-colors"
            >
              <Camera class="w-4 h-4" /> 拍照 / 选择文件
            </button>
          </div>
          <div v-else-if="docName" class="relative">
            <div class="flex items-center gap-4 p-6 bg-surface-muted/40">
              <div class="w-14 h-14 rounded-xl bg-primary/10 flex items-center justify-center shrink-0">
                <FileText class="w-7 h-7 text-primary" />
              </div>
              <div class="flex-1 min-w-0">
                <p class="font-medium text-ink truncate">{{ docName }}</p>
                <p class="text-sm text-ink-faint mt-0.5">Word/PDF 文档，识别中或已完成</p>
              </div>
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-xl bg-white text-ink px-4 py-2 text-sm font-medium border border-slate-200 hover:bg-slate-50 transition-colors"
                @click.stop="pickImage"
              >
                <Upload class="w-4 h-4" /> 重新选择
              </button>
            </div>
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
              v-if="task?.status === 'failed' || (task && errorMsg && !recognizing)"
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
          <p v-if="warningMsg" class="mt-3 text-sm text-amber-600">{{ warningMsg }}</p>
        </div>

        <!-- 擦除后图片预览 -->
      </div>

      <!-- 右侧：识别结果 + 手动修正表单 -->
      <div class="lg:col-span-3 space-y-6">
        <!-- 文档识别出的多道题列表 -->
        <div v-if="questions.length" class="rounded-2xl bg-white shadow-sm border border-slate-200/60 p-6 animate-fade-up">
          <div class="flex items-center gap-2 mb-4">
            <FileText class="w-4 h-4 text-primary" />
            <h2 class="font-semibold text-ink">识别出 {{ questions.length }} 道题</h2>
          </div>
          <p class="text-xs text-ink-faint mb-4">点击某道题填入下方表单，可手动修正后保存；重复操作可逐题录入。</p>
          <div class="space-y-3 max-h-80 overflow-y-auto pr-1">
            <button
              v-for="(q, i) in questions"
              :key="i"
              type="button"
              class="w-full text-left rounded-xl border border-slate-200 p-4 hover:border-primary-light hover:bg-surface-tint transition-colors"
              @click="selectQuestion(i)"
            >
              <div class="flex items-start gap-2">
                <span class="shrink-0 w-6 h-6 rounded-md bg-primary/10 text-primary text-xs font-semibold flex items-center justify-center mt-0.5">{{ i + 1 }}</span>
                <p class="text-sm text-ink whitespace-pre-wrap line-clamp-3">{{ q.stem_text || '（无题干文本）' }}</p>
              </div>
              <div v-if="q.sub_questions && q.sub_questions.length" class="mt-2 pl-8 space-y-2">
                <div v-for="(sq, j) in q.sub_questions" :key="j" class="text-xs text-ink-soft">
                  <p>
                    <span class="font-medium text-ink">{{ sq.label }}</span> {{ sq.text }}
                    <span v-if="sq.geometry_desc" class="text-primary">【图：{{ sq.geometry_desc }}】</span>
                  </p>
                  <div v-if="sq.geometry_keys && sq.geometry_keys.length" class="mt-1 flex gap-2">
                    <img
                      v-for="(gk, k) in sq.geometry_keys"
                      :key="k"
                      :src="`/api/recognition/files/${gk}`"
                      class="h-20 rounded-lg border border-slate-200 object-contain bg-white"
                      alt="几何图"
                    />
                  </div>
                </div>
              </div>
            </button>
          </div>
        </div>

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

            <div v-if="formula.trim()" class="rounded-xl bg-surface-tint p-4">
              <label class="block text-sm font-medium text-ink-soft mb-1.5">识别公式</label>
              <LatexRenderer :expr="formula" />
            </div>

            <!-- 裁剪出的几何图预览（题图一起展示效果） -->
            <div v-if="geometryImageUrls.length" class="rounded-xl bg-surface-tint p-4">
              <div class="flex items-center justify-between mb-2">
                <label class="block text-sm font-medium text-ink-soft">几何图形</label>
                <button
                  v-if="previewUrl"
                  type="button"
                  class="inline-flex items-center gap-1 text-xs font-medium text-primary hover:text-primary-light"
                  @click="openCropper"
                >
                  重新框选
                </button>
              </div>
              <div class="flex flex-wrap gap-3">
                <button
                  v-for="(url, i) in geometryImageUrls"
                  :key="i"
                  type="button"
                  class="block group focus:outline-none focus:ring-2 focus:ring-primary/30 rounded-lg"
                  title="点击放大查看"
                  @click="openViewer(url)"
                >
                  <img
                    :src="url"
                    class="h-32 rounded-lg border border-slate-200 object-contain bg-white cursor-zoom-in transition-transform group-hover:scale-[1.02]"
                    alt="几何图"
                  />
                </button>
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

    <!-- 几何图放大查看器 -->
    <ImageViewer :src="viewerSrc" :open="viewerOpen" @close="closeViewer" />

    <!-- 几何图重新框选弹窗 -->
    <GeometryCropper :open="cropperOpen" :src="previewUrl" @close="closeCropper" @confirm="onCropperConfirm" />
  </div>
</template>
