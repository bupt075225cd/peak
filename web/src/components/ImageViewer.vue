<script setup lang="ts">
// 图片放大查看器：点击小图时弹出，背景遮罩，点击背景或按 Esc 关闭，
// 鼠标滚轮可缩放，拖拽可平移（缩放后）。
import { ref, watch } from 'vue'
import { X, ZoomIn, ZoomOut, RotateCcw } from 'lucide-vue-next'

const props = defineProps<{
  src: string
  alt?: string
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const scale = ref(1)
const translateX = ref(0)
const translateY = ref(0)
const dragging = ref(false)
const lastX = ref(0)
const lastY = ref(0)

// 打开时重置状态。
watch(() => props.open, (v) => {
  if (v) {
    scale.value = 1
    translateX.value = 0
    translateY.value = 0
  }
})

function zoom(delta: number) {
  const next = Math.min(8, Math.max(0.5, scale.value + delta))
  scale.value = next
}

function reset() {
  scale.value = 1
  translateX.value = 0
  translateY.value = 0
}

function onWheel(e: WheelEvent) {
  e.preventDefault()
  zoom(e.deltaY < 0 ? 0.2 : -0.2)
}

function onMouseDown(e: MouseEvent) {
  if (scale.value <= 1) return
  dragging.value = true
  lastX.value = e.clientX
  lastY.value = e.clientY
}

function onMouseMove(e: MouseEvent) {
  if (!dragging.value) return
  translateX.value += e.clientX - lastX.value
  translateY.value += e.clientY - lastY.value
  lastX.value = e.clientX
  lastY.value = e.clientY
}

function onMouseUp() {
  dragging.value = false
}

function onKey(e: KeyboardEvent) {
  if (!props.open) return
  if (e.key === 'Escape') emit('close')
  if (e.key === '+' || e.key === '=') zoom(0.2)
  if (e.key === '-') zoom(-0.2)
  if (e.key === '0') reset()
}

function onBackdropClick(e: MouseEvent) {
  if (e.target === e.currentTarget) emit('close')
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="image-viewer-backdrop"
      role="dialog"
      aria-modal="true"
      @click="onBackdropClick"
      @wheel="onWheel"
      @mousedown="onMouseDown"
      @mousemove="onMouseMove"
      @mouseup="onMouseUp"
      @mouseleave="onMouseUp"
      @keydown="onKey"
      tabindex="0"
    >
      <!-- 操作栏 -->
      <div class="image-viewer-toolbar" @click.stop>
        <button type="button" class="iv-btn" title="放大" @click="zoom(0.2)">
          <ZoomIn class="w-4 h-4" />
        </button>
        <button type="button" class="iv-btn" title="缩小" @click="zoom(-0.2)">
          <ZoomOut class="w-4 h-4" />
        </button>
        <button type="button" class="iv-btn" title="还原" @click="reset">
          <RotateCcw class="w-4 h-4" />
        </button>
        <span class="iv-scale">{{ Math.round(scale * 100) }}%</span>
        <button type="button" class="iv-btn iv-btn-close" title="关闭 (Esc)" @click="emit('close')">
          <X class="w-4 h-4" />
        </button>
      </div>

      <!-- 图片：缩放 + 平移 -->
      <img
        :src="src"
        :alt="alt ?? '图片预览'"
        class="image-viewer-img"
        :class="{ 'cursor-grab': scale > 1, 'cursor-default': scale <= 1 }"
        :style="{
          transform: `translate(${translateX}px, ${translateY}px) scale(${scale})`,
        }"
        draggable="false"
        @click.stop
      />
    </div>
  </Teleport>
</template>

<style scoped>
.image-viewer-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.85);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  outline: none;
  user-select: none;
}
.image-viewer-toolbar {
  position: absolute;
  top: 16px;
  right: 16px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: rgba(255, 255, 255, 0.95);
  padding: 6px 10px;
  border-radius: 12px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}
.iv-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  color: rgb(51 65 85);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: background 0.15s;
}
.iv-btn:hover {
  background: rgb(241 245 249);
}
.iv-btn-close {
  margin-left: 4px;
}
.iv-scale {
  font-size: 12px;
  color: rgb(100 116 139);
  min-width: 40px;
  text-align: center;
}
.image-viewer-img {
  max-width: 90vw;
  max-height: 90vh;
  object-fit: contain;
  transition: transform 0.05s linear;
  transform-origin: center center;
}
</style>