<script setup lang="ts">
import { ref, computed } from 'vue'
import {
  Camera, Upload, RefreshCw, Check, X, ImagePlus, Loader2, BookOpen, FileText,
} from 'lucide-vue-next'
import {
  uploadImage, uploadDocument, isDocument, getTask, retryTask, createQuestion, createMistake,
  type RecognitionTask, type RecognitionResult, type QuestionItem,
} from '../api'
import ImageViewer from '../components/ImageViewer.vue'

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
// 文档拆题场景：子问几何图的存储 key（保存到题目记录，不自动重绘）。
const selectedGeometryKeys = ref<string[]>([])

// 几何重绘结果：识别流水线对数学题含几何图的原图进行重绘，
// 一张原图可含多个子图（图1/图2/图3），每个子图一张独立 SVG，逐张展示。
const redrawSvgKeys = ref<string[]>([])
const redrawConsistent = ref(true)
const redrawSvgUrls = computed(() =>
  redrawSvgKeys.value.map((k) => `/api/recognition/files/${k}`),
)

// 年级（学科、题型由识别自动回填，见 applyResult/selectQuestion）。
const grade = ref('')
const gradeOptions = ['七年级上', '七年级下', '八年级上', '八年级下', '九年级上', '九年级下']
const questionType = ref('')
const subject = ref('')

// 来源：这道错题的出处（如"期中考试""练习册 P32"），录入时必填。
const source = ref('')

// 识别进度文案：processing 阶段优先展示后端上报的阶段说明（如"正在几何重绘…"），
// 让用户能感知当前正在执行哪一步，而不是长时间只看到百分比。
const progressText = computed(() => {
  if (!task.value) return ''
  if (task.value.status === 'processing') {
    if (task.value.progress_text) {
      return `${task.value.progress_text}（${task.value.progress}%）`
    }
    return `识别中 ${task.value.progress}%`
  }
  const map: Record<string, string> = {
    pending: '排队中…',
    success: '识别完成',
    failed: '识别失败',
  }
  return map[task.value.status] ?? ''
})

// ImageViewer 状态：点击重绘图时打开放大查看。
const viewerOpen = ref(false)
const viewerSrc = ref('')
function openViewer(url: string) {
  viewerSrc.value = url
  viewerOpen.value = true
}
function closeViewer() {
  viewerOpen.value = false
}

function pickImage() {
  fileInput.value?.click()
}

// 目前只支持上传图片：非图片文件直接提示，不发起识别。
// MIME 可能为空（部分拖拽来源），因此再按扩展名兜底判断。
function ensureImage(file: File): boolean {
  const isImage =
    file.type.startsWith('image/') || /\.(jpe?g|png|webp|gif|bmp|heic)$/i.test(file.name)
  if (isImage) return true
  errorMsg.value = '目前仅支持上传图片（JPG、PNG 等）'
  return false
}

function onFileChange(e: Event) {
  const target = e.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file || !ensureImage(file)) return
  prepareFile(file)
  startRecognition(file)
}

function onDrop(e: DragEvent) {
  const file = e.dataTransfer?.files?.[0]
  if (!file || !ensureImage(file)) return
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
  // 180s 仍无结果时提示用户去手动查询/刷新，而不是显示"超时失败"，避免误判。
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
    subject.value = result.subject || '数学'
    questionType.value = result.question_type || '解答题'
    // 几何重绘结果：数学题含几何图时由识别流水线对原图重绘，每个子图一张独立 SVG。
    redrawSvgKeys.value = Array.isArray(result.redraw_svg_keys)
      ? result.redraw_svg_keys.filter((k) => typeof k === 'string' && k.length > 0)
      : []
    redrawConsistent.value = result.redraw_report ? result.redraw_report.consistent !== false : true
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

// 保存到题目的几何引用：数学题含几何图时存 AI 重绘的多张 SVG key；
// 其余（如文档拆题的子问图）沿用裁剪/内嵌子图 key。
function imageKeysValue(): string[] {
  if (redrawSvgKeys.value.length > 0) {
    return redrawSvgKeys.value
  }
  return selectedGeometryKeys.value
}

async function handleSave() {
  saveError.value = ''
  if (!stemText.value.trim()) {
    saveError.value = '题干不能为空'
    return
  }
  if (!grade.value) {
    saveError.value = '请选择年级'
    return
  }
  if (!source.value.trim()) {
    saveError.value = '请填写来源'
    return
  }
  loading.value = true
  try {
    // 第一步：创建题目本体。
    const question = await createQuestion({
      subject: subject.value || '数学',
      grade: grade.value,
      stem_text: stemText.value,
      image: JSON.stringify(imageKeysValue()),
      question_type: questionType.value || '解答题',
    })
    // 第二步：创建错题记录，关联刚创建的题目。
    await createMistake({
      user_id: 1,
      question_id: question.id,
      source: source.value.trim(),
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
  selectedGeometryKeys.value = []
  redrawSvgKeys.value = []
  redrawConsistent.value = true
  grade.value = ''
  source.value = ''
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
  subject.value = q.subject || '数学'
  questionType.value = q.question_type || '解答题'
  // 文档拆题的几何图沿用裁剪子图引用（不自动重绘）。
  selectedGeometryKeys.value = (q.sub_questions || [])
    .flatMap((sq) => sq.geometry_keys || [])
  redrawSvgKeys.value = []
  redrawConsistent.value = true
}
</script>

<template>
  <div class="mx-auto max-w-5xl px-4 py-8">
    <div class="mb-6 animate-fade-up">
      <h1 class="text-2xl font-semibold text-ink">录入错题</h1>
      <p class="text-sm text-ink-soft mt-1">拍照上传错题图片，自动识别题目</p>
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
          <input ref="fileInput" type="file" accept="image/*" class="hidden" @change="onFileChange" />
          <div v-if="!previewUrl && !docName" class="flex flex-col items-center justify-center py-16 px-6 text-center">
            <div class="w-16 h-16 rounded-2xl bg-primary/10 flex items-center justify-center mb-4 group-hover:animate-float">
              <ImagePlus class="w-8 h-8 text-primary" />
            </div>
            <p class="font-medium text-ink">拍照 / 上传错题图片</p>
            <p class="text-sm text-ink-faint mt-1">支持 JPG、PNG 等图片格式，拖拽或点击上传，自动识别</p>
            <button
              type="button"
              class="mt-4 inline-flex items-center gap-2 rounded-xl bg-primary text-white px-4 py-2.5 text-sm font-medium shadow-lg shadow-blue-500/25 hover:bg-primary-light transition-colors"
            >
              <Camera class="w-4 h-4" /> 拍照 / 选择图片
            </button>
          </div>
          <!-- 文档上传能力完成前保留：当前入口只接受图片，故该分支暂不可达 -->
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

        <!-- 上传前的文件校验提示（识别失败的提示仍展示在下方进度卡片里） -->
        <p v-if="errorMsg && !recognizing && !task" class="text-sm text-red-500 animate-fade-up">{{ errorMsg }}</p>

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

        <div v-if="stemText.trim()" class="rounded-2xl bg-white shadow-sm border border-slate-200/60 p-6 animate-fade-up">
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

            <!-- 年级与来源并排：两项都是必填，放一行更紧凑 -->
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <label class="block text-sm font-medium text-ink-soft mb-1.5">年级 <span class="text-red-500">*</span></label>
                <select v-model="grade" class="w-full rounded-xl border border-slate-200 bg-surface-muted/50 px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30">
                  <option disabled value="">请选择年级</option>
                  <option v-for="g in gradeOptions" :key="g" :value="g">{{ g }}</option>
                </select>
              </div>
              <div>
                <label class="block text-sm font-medium text-ink-soft mb-1.5">来源 <span class="text-red-500">*</span></label>
                <input
                  v-model="source"
                  type="text"
                  maxlength="128"
                  data-testid="source-input"
                  class="w-full rounded-xl border border-slate-200 bg-surface-muted/50 px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 focus:border-primary transition-shadow"
                  placeholder="如：期中考试 / 练习册 P32"
                />
              </div>
            </div>

            <!-- 学科、题型由识别自动回填，识别后才展示 -->
            <div v-if="subject || questionType" class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div v-if="subject" class="flex items-center gap-2 text-sm">
                <span class="text-ink-soft shrink-0">学科</span>
                <span class="inline-flex items-center rounded-md bg-surface-tint px-2 py-0.5 text-xs font-medium text-primary">{{ subject }}</span>
              </div>
              <div v-if="questionType" class="flex items-center gap-2 text-sm">
                <span class="text-ink-soft shrink-0">题型</span>
                <span class="inline-flex items-center rounded-md bg-surface-tint px-2 py-0.5 text-xs font-medium text-primary">{{ questionType }}</span>
              </div>
            </div>

            <!-- 数学题含几何图 → AI 重绘图（VLM 坐标直出 → Go 渲染 SVG） -->
            <div v-if="redrawSvgUrls.length" class="rounded-xl bg-surface-tint p-4">
              <div class="flex items-center justify-between mb-2">
                <label class="block text-sm font-medium text-ink-soft">
                  几何图形（AI 精确重绘）<span v-if="redrawSvgUrls.length > 1"> · {{ redrawSvgUrls.length }} 张</span>
                </label>
                <span
                  v-if="!redrawConsistent"
                  class="text-xs text-amber-600"
                  title="结构校验未全部通过，图形可能存在偏差"
                >结构校验未通过，仅供参考</span>
              </div>
              <!-- 每个几何子图一张独立 SVG，逐张展示，点击可放大查看 -->
              <div class="flex flex-wrap gap-3">
                <button
                  v-for="(url, i) in redrawSvgUrls"
                  :key="i"
                  type="button"
                  class="block focus:outline-none focus:ring-2 focus:ring-primary/30 rounded-lg bg-white"
                  title="点击放大查看"
                  @click="openViewer(url)"
                >
                  <img
                    :src="url"
                    class="h-40 w-auto rounded-lg border border-slate-200 bg-white object-contain cursor-zoom-in transition-transform hover:scale-[1.02]"
                    alt="AI 重绘几何图"
                  />
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- 底部操作栏 -->
        <div v-if="stemText.trim()" class="space-y-3">
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
              :disabled="loading || !stemText.trim() || !grade || !source.trim()"
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
  </div>
</template>
