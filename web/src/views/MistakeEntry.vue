<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import {
  Upload, RefreshCw, Check, X, ImagePlus, Loader2, BookOpen, FileText, Info,
} from 'lucide-vue-next'
import {
  uploadImage, uploadDocument, isDocument, getTask, retryTask, createQuestion, createMistake,
  getRecognitionFileURLs,
  type RecognitionTask, type RecognitionResult, type QuestionItem, type QuestionImageRef,
  type QuestionTable,
} from '../api'
import ImageViewer from '../components/ImageViewer.vue'
import StemText from '../components/StemText.vue'
import { inlineStemTables, serializeStemWithTables } from '../utils/stemTable'

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
// 题干中的表格：识别阶段从正文摘出（题干里只留 {{table1}} 占位），
// 保存时再序列化成 Markdown 表格写回题干，导出侧按 Markdown 还原成表格。
const tables = ref<QuestionTable[]>([])
// 文档拆题场景：子问几何图的存储 key（保存到题目记录，不自动重绘）。
const selectedGeometryKeys = ref<string[]>([])

// 几何重绘结果：识别流水线对数学题含几何图的原图进行重绘，
// 一张原图可含多个子图（图1/图2/图3），每个子图一张独立 SVG，逐张展示并保留图号。
// 图号来自识别阶段，保存后用于导出文档给配图加标注。
const redrawFigures = ref<QuestionImageRef[]>([])
const redrawConsistent = ref(true)

// 配图访问地址：识别产物的短时签名 URL（key -> url 映射，识别完成后加载）。
// <img> 无法携带 Authorization 头，直接拼文件路径会被网关 401。
const fileUrls = ref<Record<string, string>>({})

function figureUrl(key: string): string {
  return fileUrls.value[key] || ''
}

// 为识别产物（重绘 SVG、文档子图）批量换取签名 URL。
async function loadFileUrls(keys: string[]) {
  const unique = [...new Set(keys.filter((k) => k))]
  if (!task.value || unique.length === 0) return
  try {
    fileUrls.value = await getRecognitionFileURLs(task.value.id, unique)
  } catch (err) {
    console.error('获取配图访问链接失败', err)
  }
}

// 年级（学科、题型由识别自动回填，见 applyResult/selectQuestion）。
const grade = ref('')
const gradeOptions = ['七年级上', '七年级下', '八年级上', '八年级下', '九年级上', '九年级下']
const questionType = ref('')
const subject = ref('')

// 来源：这道错题的出处（如"期中考试""练习册 P32"），录入时必填。
const source = ref('')

// 识别进度文案：processing 阶段优先展示后端上报的阶段说明（如"正在重绘图形…"），
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

// 原图内嵌平移查看：按住拖动可上下左右移动查看放大后的原图，便于对照识别结果。
const PAN_SCALE = 1.8
const panEl = ref<HTMLElement | null>(null)
const panX = ref(0)
const panY = ref(0)
const panDragging = ref(false)
const panMoved = ref(false)
const panStart = { x: 0, y: 0, px: 0, py: 0 }

// 换图后重置平移位置，避免新图沿用上一张的偏移。
watch(previewUrl, () => {
  panX.value = 0
  panY.value = 0
})

// 可平移范围：放大后图像超出容器的半宽/半高（transform-origin 居中）。
function panRange() {
  const el = panEl.value
  if (!el) return { x: 0, y: 0 }
  return {
    x: ((PAN_SCALE - 1) * el.clientWidth) / 2,
    y: ((PAN_SCALE - 1) * el.clientHeight) / 2,
  }
}

function clampPan(v: number, min: number, max: number) {
  return Math.min(max, Math.max(min, v))
}

function onPanStart(clientX: number, clientY: number) {
  panDragging.value = true
  panMoved.value = false
  panStart.x = clientX
  panStart.y = clientY
  panStart.px = panX.value
  panStart.py = panY.value
}

function onPanMove(clientX: number, clientY: number) {
  if (!panDragging.value) return
  const dx = clientX - panStart.x
  const dy = clientY - panStart.y
  if (Math.abs(dx) + Math.abs(dy) > 4) panMoved.value = true
  const range = panRange()
  panX.value = clampPan(panStart.px + dx, -range.x, range.x)
  panY.value = clampPan(panStart.py + dy, -range.y, range.y)
}

function onPanEnd() {
  panDragging.value = false
}

function onPanMouseDown(e: MouseEvent) {
  onPanStart(e.clientX, e.clientY)
}
function onPanMouseMove(e: MouseEvent) {
  onPanMove(e.clientX, e.clientY)
}
function onPanMouseUp() {
  onPanEnd()
}
function onPanTouchStart(e: TouchEvent) {
  const t = e.touches[0]
  if (t) onPanStart(t.clientX, t.clientY)
}
function onPanTouchMove(e: TouchEvent) {
  const t = e.touches[0]
  if (t) onPanMove(t.clientX, t.clientY)
}
function onPanTouchEnd() {
  onPanEnd()
}

// 卡片点击仅在未发生拖拽时视为"重新选择"，避免拖动看图误触发。
function onCardClick() {
  if (panMoved.value) {
    panMoved.value = false
    return
  }
  pickImage()
}

function pickImage() {
  fileInput.value?.click()
}

// 上传体积上限（MB）：与 nginx client_max_body_size 保持一致。
// 前端先拦截可立即给出明确原因；否则请求会被 nginx 以 413 拒掉，
// 用户只能看到笼统的"图片上传失败，请重试"。
const MAX_UPLOAD_MB = 5

// 目前只支持上传图片：非图片文件直接提示，不发起识别。
// MIME 可能为空（部分拖拽来源），因此再按扩展名兜底判断。
function ensureImage(file: File): boolean {
  const isImage =
    file.type.startsWith('image/') || /\.(jpe?g|png|webp|gif|bmp|heic)$/i.test(file.name)
  if (!isImage) {
    errorMsg.value = '目前仅支持上传图片（JPG、PNG 等）'
    return false
  }
  if (file.size > MAX_UPLOAD_MB * 1024 * 1024) {
    errorMsg.value = `图片大小不能超过 ${MAX_UPLOAD_MB}MB（当前 ${(file.size / 1024 / 1024).toFixed(1)}MB），请压缩或裁剪后再上传`
    return false
  }
  return true
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
  // 常规阶段 180s；几何重绘阶段单次 VLM 调用可能耗时数分钟（且可能多轮重试），
  // 检测到重绘文案时放宽到 600s（一旦放宽保持粘性，避免文案切换后上限回缩）。
  let limit = 180
  for (let i = 0; i < limit; i++) {
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
    if ((t.progress_text || '').includes('重绘')) {
      limit = Math.max(limit, 600)
    }
  }
  // 轮询仍无结果时提示用户去手动查询/刷新，而不是显示"超时失败"，避免误判。
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
    // 表格结构单独存一份；编辑框里展示可读的表格文本（否则只剩 {{table1}} 占位符，
    // 用户看不出表格内容，会以为表格没识别出来）。
    tables.value = Array.isArray(result.tables)
      ? result.tables.filter((t) => t && typeof t.id === 'string' && t.id.length > 0)
      : []
    stemText.value = inlineStemTables(result.stem_text || '', tables.value)
    subject.value = result.subject || '数学'
    questionType.value = result.question_type || '解答题'
    // 几何重绘结果：数学题含几何图时由识别流水线对原图重绘，每个子图一张独立 SVG，
    // 并带回该子图的图号（图1/图2…）。
    redrawFigures.value = Array.isArray(result.redraw_figures)
      ? result.redraw_figures
        .filter((f) => typeof f?.key === 'string' && f.key.length > 0)
        .map((f) => ({ key: f.key, label: (f.label || '').trim() }))
      : []
    redrawConsistent.value = result.redraw_report ? result.redraw_report.consistent !== false : true
  }
  warningMsg.value = result.warning || ''
  // 服务端识别前可能把整图转正过（无 EXIF 方向时靠模型判倾斜角），这里把本地预览
  // 也转到同一朝向，让用户看到的就是模型实际识别的那张图。
  rectifyPreview(result.image_rotation || 0)
  // 为全部产物图（重绘 SVG + 文档子图）换取签名访问 URL。
  void loadFileUrls([
    ...redrawFigures.value.map((f) => f.key),
    ...questions.value.flatMap((q) => (q.sub_questions || []).flatMap((sq) => sq.geometry_keys || [])),
  ])
}

// 把本地预览图顺时针旋转 deg 度（服务端识别时对整图做过同样的转正）。
// 用 canvas 重新编码而不是 CSS transform：90/270 度需要交换宽高，用 CSS 会溢出或留白。
function rectifyPreview(deg: number) {
  if (!deg || !previewUrl.value) return
  const src = previewUrl.value
  const img = new Image()
  img.onload = () => {
    const swap = deg % 180 !== 0
    const w = swap ? img.naturalHeight : img.naturalWidth
    const h = swap ? img.naturalWidth : img.naturalHeight
    if (!w || !h) return
    const canvas = document.createElement('canvas')
    canvas.width = w
    canvas.height = h
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.translate(w / 2, h / 2)
    ctx.rotate((deg * Math.PI) / 180)
    ctx.drawImage(img, -img.naturalWidth / 2, -img.naturalHeight / 2)
    canvas.toBlob((blob) => {
      // 期间用户重新选了图就不再覆盖（previewUrl 已被换掉）。
      if (!blob || previewUrl.value !== src) return
      previewUrl.value = URL.createObjectURL(blob)
      URL.revokeObjectURL(src)
    }, 'image/jpeg', 0.92)
  }
  img.onerror = () => console.error('预览图转正失败：图片加载失败')
  img.src = src
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

// 保存到题目的配图引用：数学题含几何图时存 AI 重绘的子图（含图号）；
// 其余（如文档拆题的子问图）沿用裁剪/内嵌子图 key，这些图没有图号。
function imageRefsValue(): QuestionImageRef[] {
  if (redrawFigures.value.length > 0) {
    return redrawFigures.value
  }
  return selectedGeometryKeys.value.map((key) => ({ key }))
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
      // 题干里的表格占位在这里还原成 Markdown 表格，落库后导出侧按它渲染表格。
      stem_text: serializeStemWithTables(stemText.value, tables.value),
      image: JSON.stringify(imageRefsValue()),
      question_type: questionType.value || '解答题',
    })
    // 第二步：创建错题记录，关联刚创建的题目。
    // 不传 user_id：归属由网关校验 JWT 后注入的 X-User-Id 在服务端确定。
    await createMistake({
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
  tables.value = []
  selectedGeometryKeys.value = []
  redrawFigures.value = []
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
  redrawFigures.value = []
  redrawConsistent.value = true
}
</script>

<template>
  <div class="mx-auto max-w-5xl px-4 py-8">
    <div class="mb-6 animate-fade-up">
      <h1 class="text-2xl font-semibold text-ink">录入错题</h1>
      <p class="text-sm text-ink-soft mt-1">上传错题图片，自动识别题目</p>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-5 gap-6">
      <!-- 左侧：图片上传 + 识别进度 -->
      <div class="lg:col-span-2 space-y-6">
        <div
          class="relative rounded-2xl border-2 border-dashed border-slate-300 bg-white overflow-hidden transition-all hover:border-primary-light group"
          @click="onCardClick"
          @dragover.prevent
          @drop.prevent="onDrop"
        >
          <input ref="fileInput" type="file" accept="image/*" class="hidden" @change="onFileChange" />
          <div v-if="!previewUrl && !docName" class="flex flex-col items-center justify-center py-16 px-6 text-center cursor-pointer">
            <div class="w-16 h-16 rounded-2xl bg-primary/10 flex items-center justify-center mb-4 group-hover:animate-float">
              <ImagePlus class="w-8 h-8 text-primary" />
            </div>
            <p class="font-medium text-ink">上传错题图片</p>
            <p class="text-sm text-ink-faint mt-1">支持 JPG、PNG 等图片格式，拖拽或点击上传，自动识别</p>
            <button
              type="button"
              class="mt-4 inline-flex items-center gap-2 rounded-xl bg-primary text-white px-4 py-2.5 text-sm font-medium shadow-lg shadow-blue-500/25 hover:bg-primary-light transition-colors"
            >
              <Upload class="w-4 h-4" /> 选择图片
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
          <div v-else class="relative group">
            <!-- 原图预览：放大展示（PAN_SCALE），按住可上下左右拖动查看，便于对照识别结果 -->
            <div
              ref="panEl"
              class="relative h-72 overflow-hidden select-none touch-none"
              :class="panDragging ? 'cursor-grabbing' : 'cursor-grab'"
              @mousedown="onPanMouseDown"
              @mousemove="onPanMouseMove"
              @mouseup="onPanMouseUp"
              @mouseleave="onPanMouseUp"
              @touchstart.passive="onPanTouchStart"
              @touchmove.passive="onPanTouchMove"
              @touchend="onPanTouchEnd"
            >
              <img
                :src="previewUrl"
                class="pointer-events-none absolute left-1/2 top-1/2 h-full w-full object-cover select-none"
                :style="{ transform: `translate(-50%, -50%) translate(${panX}px, ${panY}px) scale(${PAN_SCALE})` }"
                draggable="false"
                alt="试卷预览（按住拖动查看）"
              />
            </div>
            <!-- 悬停才浮现"重新选择"，避免常驻遮罩挡住题目内容 -->
            <div class="absolute inset-0 bg-black/20 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity duration-150 pointer-events-none">
              <button
                type="button"
                class="pointer-events-auto inline-flex items-center gap-2 rounded-xl bg-white/90 text-ink px-4 py-2 text-sm font-medium hover:bg-white transition-colors"
                @click.stop="pickImage"
              >
                <Upload class="w-4 h-4" /> 重新选择
              </button>
            </div>
            <!-- 拖动查看提示（低调角标） -->
            <span class="absolute bottom-2 right-2 rounded-md bg-black/40 text-white/90 text-xs px-2 py-0.5 pointer-events-none">按住拖动查看</span>
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
                      :src="figureUrl(gk)"
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

          <!-- 识别完成引导校对：可对照左侧原图核对，提交后无法修改 -->
          <div
            v-if="task?.status === 'success'"
            class="mb-4 flex items-start gap-2 rounded-xl bg-amber-50 border border-amber-200 px-3 py-2.5"
          >
            <Info class="w-4 h-4 text-amber-500 shrink-0 mt-0.5" />
            <p class="text-sm text-amber-700">识别完成，请对照左侧原图核对题干与图形，确认无误后保存；保存后无法修改。</p>
          </div>

          <div class="space-y-4">
            <div>
              <label class="block text-sm font-medium text-ink-soft mb-1.5">题干</label>
              <!-- 题干预览放在编辑框上方：先让用户看到识别出的表格，再看可编辑的题干原文。
                   与列表页/编辑弹窗共用同一渲染组件，观感一致。 -->
              <div
                v-if="tables.length > 0"
                class="mb-3 rounded-xl border border-slate-200 bg-white/60 p-3"
                data-testid="stem-preview"
              >
                <div class="mb-2 text-xs text-ink-soft">题干预览（表格已还原）</div>
                <StemText :text="stemText" :tables="tables" class="text-sm text-ink" />
              </div>
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
            <div v-if="redrawFigures.length" class="rounded-xl bg-surface-tint p-4">
              <div class="flex items-center justify-between mb-2">
                <label class="block text-sm font-medium text-ink-soft">
                  几何图形（AI 精确重绘）<span v-if="redrawFigures.length > 1"> · {{ redrawFigures.length }} 张</span>
                </label>
                <span
                  v-if="!redrawConsistent"
                  class="text-xs text-amber-600"
                  title="结构校验未全部通过，图形可能存在偏差"
                >结构校验未通过，仅供参考</span>
              </div>
              <!-- 每个几何子图一张独立 SVG，逐张展示并标注图号（重绘会丢失原图的"图1/图2"标注），点击可放大查看 -->
              <div class="flex flex-wrap gap-3">
                <figure
                  v-for="(fig, i) in redrawFigures"
                  :key="i"
                  class="flex flex-col items-center gap-1"
                >
                  <button
                    type="button"
                    class="block focus:outline-none focus:ring-2 focus:ring-primary/30 rounded-lg bg-white"
                    title="点击放大查看"
                    @click="openViewer(figureUrl(fig.key))"
                  >
                    <img
                      :src="figureUrl(fig.key)"
                      class="h-40 w-auto rounded-lg border border-slate-200 bg-white object-contain cursor-zoom-in transition-transform hover:scale-[1.02]"
                      :alt="fig.label || 'AI 重绘几何图'"
                    />
                  </button>
                  <figcaption v-if="fig.label" class="text-xs text-ink-soft">{{ fig.label }}</figcaption>
                </figure>
              </div>
            </div>
          </div>
        </div>

        <!-- 底部操作栏 -->
        <div v-if="stemText.trim()" class="space-y-3">
          <p v-if="saveError" class="text-sm text-red-500 text-right">{{ saveError }}</p>
          <p class="text-xs text-ink-faint text-right">保存后不可再修改，请仔细核对</p>
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
