<script setup lang="ts">
// 几何图框选组件：在整张原图上拖拽画框，裁剪出几何图区域。
// 用于自动识别 bbox 不准确时，让用户手动重选几何图。
import { ref, watch, onBeforeUnmount } from 'vue'
import { Crop, X } from 'lucide-vue-next'

const props = defineProps<{
  open: boolean
  src: string // 原图 URL
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'confirm', file: File): void // 裁剪完成，返回 blob 封装的 File
}>()

const imgEl = ref<HTMLImageElement | null>(null)
const containerRef = ref<HTMLDivElement | null>(null)

// 框选状态（相对于图片原始尺寸的像素坐标）。
const start = ref<{ x: number; y: number } | null>(null)
const end = ref<{ x: number; y: number } | null>(null)
const dragging = ref(false)

// 缩放比：图片显示尺寸 vs 原始尺寸。
let scaleRatio = 1

function getPos(e: MouseEvent): { x: number; y: number } {
  const el = imgEl.value
  if (!el) return { x: 0, y: 0 }
  const rect = el.getBoundingClientRect()
  return {
    x: e.clientX - rect.left,
    y: e.clientY - rect.top,
  }
}

function onMouseDown(e: MouseEvent) {
  const pos = getPos(e)
  start.value = pos
  end.value = pos
  dragging.value = true
}

function onMouseMove(e: MouseEvent) {
  if (!dragging.value) return
  end.value = getPos(e)
}

function onMouseUp() {
  dragging.value = false
}

// 计算框选矩形（显示坐标）。
const rect = computedRect()

function computedRect() {
  const s = start.value
  const e = end.value
  if (!s || !e) return null
  const x = Math.min(s.x, e.x)
  const y = Math.min(s.y, e.y)
  const w = Math.abs(e.x - s.x)
  const h = Math.abs(e.y - s.y)
  return { x, y, w, h }
}

watch(() => props.open, (v) => {
  if (v) {
    reset()
    // 等图片加载完成后计算缩放比。
    requestAnimationFrame(() => computeRatio())
  }
})

function computeRatio() {
  const el = imgEl.value
  if (!el || !el.naturalWidth) return
  scaleRatio = el.naturalWidth / el.getBoundingClientRect().width
}

function onImgLoad() {
  computeRatio()
}

function reset() {
  start.value = null
  end.value = null
  dragging.value = false
}

async function confirm() {
  const r = computedRect()
  const el = imgEl.value
  if (!r || !el || r.w < 5 || r.h < 5) return

  // 显示坐标换算为原始像素坐标。
  const sx = Math.round(r.x * scaleRatio)
  const sy = Math.round(r.y * scaleRatio)
  const sw = Math.round(r.w * scaleRatio)
  const sh = Math.round(r.h * scaleRatio)

  const canvas = document.createElement('canvas')
  canvas.width = sw
  canvas.height = sh
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  ctx.drawImage(el, sx, sy, sw, sh, 0, 0, sw, sh)

  const blob = await new Promise<Blob | null>((resolve) => {
    canvas.toBlob(resolve, 'image/jpeg', 0.9)
  })
  if (!blob) return
  const file = new File([blob], 'geometry.jpg', { type: 'image/jpeg' })
  emit('confirm', file)
}

function onKey(e: KeyboardEvent) {
  if (!props.open) return
  if (e.key === 'Escape') emit('close')
}

function onBackdropClick(e: MouseEvent) {
  if (e.target === e.currentTarget) emit('close')
}

onBeforeUnmount(() => {
  reset()
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="cropper-backdrop"
      role="dialog"
      aria-modal="true"
      tabindex="0"
      @click="onBackdropClick"
      @keydown="onKey"
    >
      <div class="cropper-panel" @click.stop>
        <div class="cropper-header">
          <div class="flex items-center gap-2">
            <Crop class="w-4 h-4 text-primary" />
            <h3 class="font-medium text-ink">框选几何图形</h3>
          </div>
          <button type="button" class="cropper-close" title="关闭" @click="emit('close')">
            <X class="w-4 h-4" />
          </button>
        </div>
        <p class="cropper-hint">在图片上按住鼠标拖拽，框出几何图形区域（不含题干文字）。</p>

        <div ref="containerRef" class="cropper-stage">
          <div
            class="cropper-img-wrap"
            @mousedown="onMouseDown"
            @mousemove="onMouseMove"
            @mouseup="onMouseUp"
            @mouseleave="onMouseUp"
          >
            <img ref="imgEl" :src="src" class="cropper-img select-none" alt="原图" @load="onImgLoad" draggable="false" />
            <!-- 框选矩形 -->
            <div
              v-if="computedRect() && computedRect()!.w > 2 && computedRect()!.h > 2"
              class="cropper-rect"
              :style="{
                left: computedRect()!.x + 'px',
                top: computedRect()!.y + 'px',
                width: computedRect()!.w + 'px',
                height: computedRect()!.h + 'px',
              }"
            />
          </div>
        </div>

        <div class="cropper-footer">
          <button type="button" class="cropper-btn-ghost" @click="emit('close')">取消</button>
          <button
            type="button"
            class="cropper-btn-primary"
            :disabled="!computedRect() || computedRect()!.w < 5 || computedRect()!.h < 5"
            @click="confirm"
          >
            确认裁剪
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.cropper-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.85);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  outline: none;
}
.cropper-panel {
  background: #fff;
  border-radius: 16px;
  padding: 20px;
  width: min(90vw, 860px);
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.25);
}
.cropper-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}
.cropper-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  border: none;
  background: transparent;
  color: #64748b;
  cursor: pointer;
}
.cropper-close:hover {
  background: #f1f5f9;
}
.cropper-hint {
  font-size: 13px;
  color: #64748b;
  margin-bottom: 12px;
}
.cropper-stage {
  flex: 1;
  overflow: auto;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  background: #f8fafc;
  max-height: 60vh;
}
.cropper-img-wrap {
  position: relative;
  display: inline-block;
  cursor: crosshair;
  line-height: 0;
}
.cropper-img {
  max-width: 100%;
  height: auto;
  display: block;
  user-select: none;
  -webkit-user-drag: none;
}
.cropper-rect {
  position: absolute;
  border: 2px solid #2563eb;
  background: rgba(37, 99, 235, 0.15);
  pointer-events: none;
}
.cropper-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 16px;
}
.cropper-btn-ghost {
  padding: 8px 20px;
  border-radius: 10px;
  border: 1px solid #e2e8f0;
  background: #fff;
  color: #475569;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
}
.cropper-btn-ghost:hover {
  background: #f8fafc;
}
.cropper-btn-primary {
  padding: 8px 20px;
  border-radius: 10px;
  border: none;
  background: #2563eb;
  color: #fff;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
}
.cropper-btn-primary:hover {
  background: #1d4ed8;
}
.cropper-btn-primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>