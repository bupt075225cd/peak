<script setup lang="ts">
// 几何图框选组件：在整张原图上拖拽画框，裁剪出几何图区域。
// 支持：8 手柄改尺寸、整体平移、键盘微调、图片缩放、90° 旋转。
// 重要：选区始终基于原图自然像素坐标系，旋转仅作为 CSS 视觉变换（不影响裁剪坐标），
// 这样裁剪出来的图与原图方向一致（与用户期望一致）。
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { Crop, X, ZoomIn, ZoomOut, RotateCw, RotateCcw, AlertCircle, Loader2 } from 'lucide-vue-next'

const props = defineProps<{
  open: boolean
  src: string // 原图 URL
  error?: string
  confirming?: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'confirm', file: File): void
}>()

const imgEl = ref<HTMLImageElement | null>(null)
const stageRef = ref<HTMLDivElement | null>(null)

// 原图自然尺寸。
const natW = ref(0)
const natH = ref(0)
const imgScale = ref(1) // 显示缩放因子 0.25 ~ 4
// 旋转角度（CSS 顺时针，0/90/180/270）。仅用于 CSS 视觉变换，不影响裁剪坐标。
const rotation = ref(0)

// 选区：相对原图自然像素的坐标（不受旋转影响）。
const sel = ref<{ x: number; y: number; w: number; h: number } | null>(null)

type Mode =
  | { kind: 'draw' }
  | { kind: 'move' }
  | { kind: 'resize'; handle: Handle }
const mode = ref<Mode | null>(null)
const startPoint = ref({ x: 0, y: 0 })
const originSel = ref<{ x: number; y: number; w: number; h: number } | null>(null)

const MIN_SIZE = 4 // 最小选区边长（原图像素）

type Handle = 'nw' | 'n' | 'ne' | 'e' | 'se' | 's' | 'sw' | 'w'

watch(
  () => props.src,
  (v) => {
    if (v) loadImage(v)
  },
  { immediate: true },
)

watch(
  () => props.open,
  (v) => {
    if (v) reset()
  },
)

function loadImage(src: string) {
  const img = new Image()
  img.onload = () => {
    natW.value = img.naturalWidth
    natH.value = img.naturalHeight
    fitScale()
  }
  img.src = src
}

function fitScale() {
  const stage = stageRef.value
  if (!stage || !natW.value) return
  // 视觉外接矩形（旋转 90/270 时宽高对调），按比例取 min 缩放，确保旋转后也能完整放入 stage。
  const visualW = rotation.value % 180 === 0 ? natW.value : natH.value
  const visualH = rotation.value % 180 === 0 ? natH.value : natW.value
  // jsdom 下 clientWidth/Height 为 0，按宽高都 >0 才缩放，否则保留 scale=1（测试/未挂载时不影响）。
  const maxW = stage.clientWidth
  const maxH = stage.clientHeight
  if (maxW <= 0 && maxH <= 0) return
  const scaleByW = maxW > 0 && visualW > maxW ? maxW / visualW : 1
  const scaleByH = maxH > 0 && visualH > maxH ? maxH / visualH : 1
  imgScale.value = Math.min(scaleByW, scaleByH)
}

function reset() {
  sel.value = null
  mode.value = null
  imgScale.value = 1
  rotation.value = 0
  if (props.src) loadImage(props.src)
}

// 未旋转盒子的 CSS 像素尺寸（无论旋转多少度，图片元素的布局盒子不变）。
function baseBoxW() {
  return natW.value * imgScale.value
}
function baseBoxH() {
  return natH.value * imgScale.value
}

// 鼠标屏幕坐标 → 原图自然像素坐标（处理旋转逆变换）。
// 浏览器实测：CSS `rotate(90deg)` 在屏幕坐标系（y 向下）中表现为逆时针，
// 正变换为 (x,y) → (-y, x)。因此逆变换使用 -rad 的 cos/sin，即 (px,py) → (py, -px)。
// 鼠标相对视觉盒中心的偏移逆旋转回未旋转局部坐标，再加回未旋转盒子的一半（左上角为 0,0），
// 最后按比例换算为原图自然像素。
function inverseRotate(px: number, py: number): { x: number; y: number } {
  const rad = (rotation.value * Math.PI) / 180
  const cosA = Math.cos(-rad)
  const sinA = Math.sin(-rad)
  return {
    x: px * cosA - py * sinA,
    y: px * sinA + py * cosA,
  }
}

function getImagePos(e: MouseEvent): { x: number; y: number } {
  const el = imgEl.value
  if (!el || !natW.value) return { x: 0, y: 0 }
  const rect = el.getBoundingClientRect()
  // rect 是旋转后的视觉外接矩形，中心与未旋转盒子中心一致。
  const cx = rect.left + rect.width / 2
  const cy = rect.top + rect.height / 2
  const px = e.clientX - cx
  const py = e.clientY - cy
  const r = inverseRotate(px, py)
  // 未旋转盒子（CSS 像素）
  const bw = baseBoxW()
  const bh = baseBoxH()
  const localX = r.x + bw / 2
  const localY = r.y + bh / 2
  // 换算为原图自然像素
  return { x: (localX / bw) * natW.value, y: (localY / bh) * natH.value }
}

function clampSel(s: { x: number; y: number; w: number; h: number }) {
  const W = natW.value
  const H = natH.value
  return {
    x: Math.max(0, Math.min(s.x, W - s.w)),
    y: Math.max(0, Math.min(s.y, H - s.h)),
    w: Math.min(s.w, W),
    h: Math.min(s.h, H),
  }
}

function pointInSel(p: { x: number; y: number }) {
  const s = sel.value
  if (!s) return false
  return p.x >= s.x && p.x <= s.x + s.w && p.y >= s.y && p.y <= s.y + s.h
}

function onMouseDown(e: MouseEvent) {
  if (!sel.value) {
    const p = getImagePos(e)
    mode.value = { kind: 'draw' }
    startPoint.value = p
    originSel.value = null
    return
  }
  const h = hitHandle(e)
  if (h) {
    mode.value = { kind: 'resize', handle: h }
    originSel.value = { ...sel.value }
    startPoint.value = getImagePos(e)
    return
  }
  const p = getImagePos(e)
  if (pointInSel(p)) {
    mode.value = { kind: 'move' }
    originSel.value = { ...sel.value }
    startPoint.value = p
  }
}

function onMouseMove(e: MouseEvent) {
  if (!mode.value) return
  const p = getImagePos(e)
  if (mode.value.kind === 'draw') {
    const s = originSel.value ?? startPoint.value
    sel.value = clampSel(normalizeRect(s.x, s.y, p.x, p.y))
    return
  }
  if (mode.value.kind === 'move') {
    const o = originSel.value!
    const dx = p.x - startPoint.value.x
    const dy = p.y - startPoint.value.y
    sel.value = clampSel({ x: o.x + dx, y: o.y + dy, w: o.w, h: o.h })
    return
  }
  if (mode.value.kind === 'resize') {
    sel.value = resizeFromHandle(mode.value.handle, originSel.value!, p)
  }
}

function onMouseUp() {
  if (mode.value?.kind === 'draw' && sel.value && (sel.value.w < 2 || sel.value.h < 2)) {
    sel.value = null
  }
  mode.value = null
}

function normalizeRect(x1: number, y1: number, x2: number, y2: number) {
  return {
    x: Math.min(x1, x2),
    y: Math.min(y1, y2),
    w: Math.abs(x2 - x1),
    h: Math.abs(y2 - y1),
  }
}

// 手柄命中：把手柄（CSS 像素位置）换算回屏幕坐标，再比较鼠标。
// 但 hitHandle 接收 MouseEvent，简化：手柄在显示坐标系（CSS），
// 我们把鼠标 clientX/Y 反变换为「未旋转 CSS 像素」（即相对于未旋转 img 元素局部坐标），
// 然后判断手柄位置。手柄位置用 sel 原图坐标 × scale。
function hitHandle(e: MouseEvent): Handle | null {
  const s = sel.value
  if (!s) return null
  // 鼠标在未旋转 CSS 局部坐标（与 getImagePos 同一套逆变换）。
  const el = imgEl.value
  if (!el) return null
  const rect = el.getBoundingClientRect()
  const cx = rect.left + rect.width / 2
  const cy = rect.top + rect.height / 2
  const px = e.clientX - cx
  const py = e.clientY - cy
  const r = inverseRotate(px, py)
  const bw = baseBoxW()
  const bh = baseBoxH()
  const mx = r.x + bw / 2
  const my = r.y + bh / 2

  // 手柄在未旋转显示坐标系（CSS 像素）位置
  const sx = s.x * imgScale.value
  const sy = s.y * imgScale.value
  const sw = s.w * imgScale.value
  const sh = s.h * imgScale.value
  const HANDLE_HIT = 14
  const hits = (cx2: number, cy2: number) =>
    Math.abs(mx - cx2) <= HANDLE_HIT && Math.abs(my - cy2) <= HANDLE_HIT
  const corners: Partial<Record<Handle, [number, number]>> = {
    nw: [sx, sy],
    ne: [sx + sw, sy],
    sw: [sx, sy + sh],
    se: [sx + sw, sy + sh],
  }
  for (const [k, pos] of Object.entries(corners) as [Handle, [number, number]][]) {
    if (pos && hits(pos[0], pos[1])) return k
  }
  const edges: Array<[Handle, number, number]> = [
    ['n', sx + sw / 2, sy],
    ['s', sx + sw / 2, sy + sh],
    ['w', sx, sy + sh / 2],
    ['e', sx + sw, sy + sh / 2],
  ]
  for (const [k, cx2, cy2] of edges) {
    if (hits(cx2, cy2)) return k
  }
  return null
}

function resizeFromHandle(
  handle: Handle,
  o: { x: number; y: number; w: number; h: number },
  p: { x: number; y: number },
) {
  const W = natW.value
  const H = natH.value
  let { x, y, w, h } = { ...o }
  const mx = Math.max(p.x, 0)
  const my = Math.max(p.y, 0)
  const nx = Math.min(mx, W)
  const ny = Math.min(my, H)
  if (handle.includes('e')) {
    w = Math.max(MIN_SIZE, nx - x)
  } else if (handle.includes('w')) {
    const right = x + w
    x = Math.max(0, Math.min(nx, right - MIN_SIZE))
    w = right - x
  }
  if (handle.includes('s')) {
    h = Math.max(MIN_SIZE, ny - y)
  } else if (handle.includes('n')) {
    const bottom = y + h
    y = Math.max(0, Math.min(ny, bottom - MIN_SIZE))
    h = bottom - y
  }
  if (x + w > W) w = W - x
  if (y + h > H) h = H - y
  if (w < MIN_SIZE) w = MIN_SIZE
  if (h < MIN_SIZE) h = MIN_SIZE
  // 取整避免浮点误差（如 50.00000000000003）污染显示与裁剪。
  return { x: Math.round(x), y: Math.round(y), w: Math.round(w), h: Math.round(h) }
}

function onKey(e: KeyboardEvent) {
  if (!props.open) return
  if (e.key === 'Escape') {
    emit('close')
    return
  }
  const s = sel.value
  if (!s) return
  const step = e.shiftKey ? 10 : e.altKey ? 1 : 4
  let dx = 0
  let dy = 0
  if (e.key === 'ArrowLeft') dx = -step
  else if (e.key === 'ArrowRight') dx = step
  else if (e.key === 'ArrowUp') dy = -step
  else if (e.key === 'ArrowDown') dy = step
  else return
  e.preventDefault()
  sel.value = clampSel({ x: s.x + dx, y: s.y + dy, w: s.w, h: s.h })
}

function onBackdropClick(e: MouseEvent) {
  if (e.target === e.currentTarget) emit('close')
}

function zoom(delta: number) {
  // 缩放范围 10% ~ 400%。降低下界以支持查看超大原图局部细节。
  const next = Math.min(4, Math.max(0.1, +(imgScale.value + delta).toFixed(3)))
  scaleTo(next)
}

function scaleTo(next: number) {
  const ratio = next / imgScale.value
  imgScale.value = next
  // 选区基于原图自然像素，缩放不影响选区（自然像素坐标不变）
  void ratio
}

// 旋转：仅修改 rotation 状态，CSS 应用 transform。选区保持在原图坐标系不变。
// 旋转后视觉外接矩形宽高对调，下一帧重新 fitScale（等 stage 布局稳定）自适应 stage，避免滚动条。
function rotate(angle: 90 | 270) {
  rotation.value = (rotation.value + angle) % 360
  requestAnimationFrame(() => fitScale())
}

function onRotate(angle: 90 | 270) {
  rotate(angle)
}

async function confirm() {
  const s = sel.value
  if (!s || s.w < 5 || s.h < 5) return
  // 加载原图，裁剪 sel 区域到临时 canvas（保持原图方向）。
  const img = new Image()
  await new Promise<void>((r) => {
    img.onload = () => r()
    img.onerror = () => r()
    img.src = props.src
  })
  const sw = Math.round(s.w)
  const sh = Math.round(s.h)
  const tmp = document.createElement('canvas')
  tmp.width = sw
  tmp.height = sh
  const tctx = tmp.getContext('2d')
  if (!tctx) return
  tctx.drawImage(img, s.x, s.y, s.w, s.h, 0, 0, sw, sh)
  // 输出 canvas 按当前 rotation 旋转后尺寸：90/270 时宽高对调，使输出方向与显示一致。
  const swapped = rotation.value % 180 !== 0
  const out = document.createElement('canvas')
  out.width = swapped ? sh : sw
  out.height = swapped ? sw : sh
  const octx = out.getContext('2d')
  if (!octx) return
  octx.translate(out.width / 2, out.height / 2)
  octx.rotate((rotation.value * Math.PI) / 180) // CSS 顺时针正角度 = canvas 顺时针
  octx.drawImage(tmp, -sw / 2, -sh / 2)
  const blob = await new Promise<Blob | null>((resolve) => {
    out.toBlob(resolve, 'image/jpeg', 0.9)
  })
  if (!blob) return
  const file = new File([blob], 'geometry.jpg', { type: 'image/jpeg' })
  emit('confirm', file)
}

const rect = computed(() => sel.value)
const HANDLES: Handle[] = ['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w']

// CSS rotate 不改变 layout box，但视觉外接会超出 layout。
// 绕 transform-origin center 旋转 90/270 时，视觉外接左上角相对 layout 左上角
// 偏移 = ((W-H)/2, (H-W)/2)（90 与 270 相同）。给 wrap 加反号 margin 使
// 视觉外接左上角对齐 layout 左上角，从而 scrollTop=0/scrollLeft=0 即可看到视觉左上角。
const wrapMargin = computed(() => {
  if (rotation.value % 180 === 0) return { marginTop: 0, marginLeft: 0 }
  const bw = baseBoxW()
  const bh = baseBoxH()
  return {
    marginTop: (bw - bh) / 2,
    marginLeft: (bh - bw) / 2,
  }
})

// 手柄位置用 sel 原图坐标 × imgScale（CSS 像素），随 wrap 旋转一起视觉旋转
const handlePos: Record<Handle, (s: { x: number; y: number; w: number; h: number }) => { left: string; top: string; cursor: string }> = {
  nw: (s) => ({ left: (s.x * imgScale.value) + 'px', top: (s.y * imgScale.value) + 'px', cursor: 'nwse-resize' }),
  n: (s) => ({ left: ((s.x + s.w / 2) * imgScale.value) + 'px', top: (s.y * imgScale.value) + 'px', cursor: 'ns-resize' }),
  ne: (s) => ({ left: ((s.x + s.w) * imgScale.value) + 'px', top: (s.y * imgScale.value) + 'px', cursor: 'nesw-resize' }),
  e: (s) => ({ left: ((s.x + s.w) * imgScale.value) + 'px', top: ((s.y + s.h / 2) * imgScale.value) + 'px', cursor: 'ew-resize' }),
  se: (s) => ({ left: ((s.x + s.w) * imgScale.value) + 'px', top: ((s.y + s.h) * imgScale.value) + 'px', cursor: 'nwse-resize' }),
  s: (s) => ({ left: ((s.x + s.w / 2) * imgScale.value) + 'px', top: ((s.y + s.h) * imgScale.value) + 'px', cursor: 'ns-resize' }),
  sw: (s) => ({ left: (s.x * imgScale.value) + 'px', top: ((s.y + s.h) * imgScale.value) + 'px', cursor: 'nesw-resize' }),
  w: (s) => ({ left: (s.x * imgScale.value) + 'px', top: ((s.y + s.h / 2) * imgScale.value) + 'px', cursor: 'ew-resize' }),
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
        <p class="cropper-hint">
          拖拽画框，松手后可用 8 个手柄调整、框内拖动平移，方向键微调；支持缩放与旋转。裁剪结果与显示方向一致。
        </p>

        <div class="cropper-toolbar">
          <button type="button" class="ct-btn" title="缩小" @click="zoom(-0.25)">
            <ZoomOut class="w-4 h-4" />
          </button>
          <span class="ct-scale">{{ Math.round(imgScale * 100) }}%</span>
          <button type="button" class="ct-btn" title="放大" @click="zoom(0.25)">
            <ZoomIn class="w-4 h-4" />
          </button>
          <span class="ct-divider" />
          <button type="button" class="ct-btn" title="逆时针旋转 90°" @click="onRotate(270)">
            <RotateCcw class="w-4 h-4" />
          </button>
          <button type="button" class="ct-btn" title="顺时针旋转 90°" @click="onRotate(90)">
            <RotateCw class="w-4 h-4" />
          </button>
        </div>

        <div ref="stageRef" class="cropper-stage">
          <div
            class="cropper-img-wrap"
            :style="{
              transform: `rotate(${rotation}deg)`,
              width: baseBoxW() + 'px',
              height: baseBoxH() + 'px',
              marginTop: wrapMargin.marginTop + 'px',
              marginLeft: wrapMargin.marginLeft + 'px',
            }"
            @mousedown="onMouseDown"
            @mousemove="onMouseMove"
            @mouseup="onMouseUp"
            @mouseleave="onMouseUp"
          >
            <img
              ref="imgEl"
              :src="props.src"
              class="cropper-img select-none"
              alt="原图"
              draggable="false"
              :style="{ width: baseBoxW() + 'px', height: baseBoxH() + 'px' }"
            />
            <!-- 框选矩形与 8 个手柄（与 wrap 一起旋转视觉） -->
            <template v-if="rect">
              <div
                class="cropper-rect"
                :style="{
                  left: (rect.x * imgScale) + 'px',
                  top: (rect.y * imgScale) + 'px',
                  width: (rect.w * imgScale) + 'px',
                  height: (rect.h * imgScale) + 'px',
                }"
              />
              <div
                v-for="h in HANDLES"
                :key="h"
                class="cropper-handle"
                :style="handlePos[h](rect)"
              />
            </template>
          </div>
        </div>

        <div v-if="props.error" class="cropper-error" role="alert">
          <AlertCircle class="w-4 h-4 shrink-0" />
          <span>{{ props.error }}</span>
        </div>
        <div class="cropper-footer">
          <button
            type="button"
            class="cropper-btn-ghost"
            :disabled="props.confirming"
            @click="emit('close')"
          >
            取消
          </button>
          <button
            type="button"
            class="cropper-btn-primary"
            :disabled="props.confirming || !rect || rect.w < 5 || rect.h < 5"
            @click="confirm"
          >
            <Loader2 v-if="props.confirming" class="w-4 h-4 animate-spin" />
            <span>{{ props.confirming ? '上传中…' : '确认裁剪' }}</span>
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
  max-height: 92vh;
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
.cropper-toolbar {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: #f1f5f9;
  border-radius: 10px;
  padding: 4px 6px;
  margin-bottom: 12px;
}
.ct-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  border: none;
  background: transparent;
  color: #475569;
  cursor: pointer;
}
.ct-btn:hover {
  background: #e2e8f0;
}
.ct-scale {
  font-size: 12px;
  color: #64748b;
  min-width: 44px;
  text-align: center;
}
.ct-divider {
  width: 1px;
  height: 18px;
  background: #cbd5e1;
  margin: 0 4px;
}
.cropper-stage {
  flex: 1;
  overflow: auto;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  background: #f8fafc;
  max-height: 62vh;
  min-height: 320px;
  display: flex;
  /* 左上对齐：避免 wrap 大于 stage 时 flex 居中导致 wrap 顶部在 stage 顶部之上
     而 scrollTop=0 又无法滚到负值（不可见）。让 wrap 顶部对齐 stage 顶部，
     scrollTop=0 即可看到 wrap 顶部；图小时 wrap 视觉仍位于左上角。 */
  align-items: flex-start;
  justify-content: flex-start;
}
.cropper-img-wrap {
  position: relative;
  cursor: crosshair;
  line-height: 0;
  transform-origin: center center;
}
.cropper-img {
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
.cropper-handle {
  position: absolute;
  width: 12px;
  height: 12px;
  border: 2px solid #fff;
  background: #2563eb;
  border-radius: 3px;
  transform: translate(-50%, -50%);
  box-shadow: 0 0 0 1px rgba(37, 99, 235, 0.6);
}
.cropper-error {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  padding: 10px 14px;
  border-radius: 10px;
  background: #fef2f2;
  border: 1px solid #fecaca;
  color: #b91c1c;
  font-size: 13px;
  font-weight: 500;
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
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.cropper-btn-primary:hover {
  background: #1d4ed8;
}
.cropper-btn-primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
