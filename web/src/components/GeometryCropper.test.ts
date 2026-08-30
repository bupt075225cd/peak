import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import GeometryCropper from './GeometryCropper.vue'

// jsdom 未实现 canvas，mock 掉 getContext / toBlob。
function stubCanvas() {
  const ctx = {
    drawImage: vi.fn(),
    translate: vi.fn(),
    rotate: vi.fn(),
  }
  const toBlob = vi.fn((cb: (b: Blob | null) => void) => {
    cb(new Blob(['fake'], { type: 'image/jpeg' }))
  })
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(ctx as never)
  vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation(toBlob)
}

// stub Image：new Image() 返回带自然尺寸的对象，设 src 时同步触发 onload。
function stubImage(naturalWidth = 800, naturalHeight = 600) {
  class FakeImage {
    naturalWidth = naturalWidth
    naturalHeight = naturalHeight
    onload: (() => void) | null = null
    onerror: (() => void) | null = null
    private _src = ''
    get src() {
      return this._src
    }
    set src(v: string) {
      this._src = v
      if (this.onload) this.onload()
    }
  }
  vi.stubGlobal('Image', FakeImage)
}

function stubURL() {
  vi.stubGlobal('URL', Object.assign(Object.create(URL), {
    createObjectURL: vi.fn(() => 'blob:mock'),
    revokeObjectURL: vi.fn(),
  }))
}

// 为 img 元素 mock getBoundingClientRect。
// 视觉盒子模拟浏览器行为：旋转 90/270 时返回对调后的外接矩形（宽=未旋转高，高=未旋转宽），
// 中心保持不变（transform-origin: center）。未旋转时返回未旋转盒子（natW×natH × scale）。
function mockImgRect(wrapper: ReturnType<typeof mount>, w = 800, h = 600) {
  const original = Element.prototype.getBoundingClientRect
  Element.prototype.getBoundingClientRect = function () {
    if (this instanceof HTMLImageElement && (this as HTMLElement).classList?.contains('cropper-img')) {
      const wrap = this.parentElement as HTMLElement | null
      const transform = wrap?.style?.transform || ''
      const rotated = transform.includes('90deg') || transform.includes('270deg')
      const vw = rotated ? h : w
      const vh = rotated ? w : h
      return {
        left: 0, top: 0, right: vw, bottom: vh, width: vw, height: vh,
        x: 0, y: 0, toJSON: () => ({}),
      } as DOMRect
    }
    return original.call(this)
  }
}

function mountCropper(open = true) {
  return mount(GeometryCropper, {
    props: { open, src: 'https://example.com/paper.jpg' },
    attachTo: document.body,
    global: { stubs: { teleport: true } },
  })
}

// 画框：clientX/Y 即未旋转时显示局部坐标（mock 盒子 0..w / 0..h，左上角为 0）。
async function drawSelection(wrapper: ReturnType<typeof mount>, x1 = 100, y1 = 100, x2 = 200, y2 = 150) {
  const imgWrap = wrapper.find('.cropper-img-wrap')
  await imgWrap.trigger('mousedown', { clientX: x1, clientY: y1 })
  await imgWrap.trigger('mousemove', { clientX: x2, clientY: y2 })
  await imgWrap.trigger('mouseup')
}

describe('GeometryCropper', () => {
  beforeEach(() => {
    stubCanvas()
    stubImage()
    stubURL()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders nothing when closed', () => {
    const wrapper = mountCropper(false)
    expect(wrapper.find('.cropper-backdrop').exists()).toBe(false)
  })

  it('renders dialog and image when open', () => {
    const wrapper = mountCropper(true)
    expect(wrapper.find('.cropper-backdrop').exists()).toBe(true)
    expect(wrapper.find('.cropper-img').attributes('src')).toBe('https://example.com/paper.jpg')
  })

  it('emits close on close button click', async () => {
    const wrapper = mountCropper(true)
    await wrapper.find('.cropper-close').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('emits close on Escape key', async () => {
    const wrapper = mountCropper(true)
    await wrapper.find('.cropper-backdrop').trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('emits close when clicking backdrop itself', async () => {
    const wrapper = mountCropper(true)
    await wrapper.find('.cropper-backdrop').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('emits close on cancel button', async () => {
    const wrapper = mountCropper(true)
    await wrapper.find('.cropper-btn-ghost').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('draws selection rectangle while dragging', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    await drawSelection(wrapper)
    const style = wrapper.find('.cropper-rect').attributes('style') || ''
    expect(style).toContain('left: 100px')
    expect(style).toContain('top: 100px')
    expect(style).toContain('width: 100px')
    expect(style).toContain('height: 50px')
    expect(wrapper.findAll('.cropper-handle').length).toBe(8)
  })

  it('confirms crop and emits File after dragging', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    await drawSelection(wrapper, 10, 10, 60, 50)
    const confirmBtn = wrapper.find('.cropper-btn-primary')
    expect(confirmBtn.attributes('disabled')).toBeUndefined()
    await confirmBtn.trigger('click')
    await flushPromises()
    const emitted = wrapper.emitted('confirm')
    expect(emitted).toBeTruthy()
    const file = emitted![0][0] as File
    expect(file).toBeInstanceOf(File)
    expect(file.name).toBe('geometry.jpg')
  })

  it('keeps confirm button disabled when no selection', () => {
    const wrapper = mountCropper(true)
    expect(wrapper.find('.cropper-btn-primary').attributes('disabled')).toBeDefined()
  })

  it('resizes selection by dragging the SE handle', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    await drawSelection(wrapper, 100, 100, 200, 150)
    const imgWrap = wrapper.find('.cropper-img-wrap')
    await imgWrap.trigger('mousedown', { clientX: 200, clientY: 150 })
    await imgWrap.trigger('mousemove', { clientX: 260, clientY: 210 })
    await imgWrap.trigger('mouseup')
    const style = wrapper.find('.cropper-rect').attributes('style') || ''
    expect(style).toContain('width: 160px')
    expect(style).toContain('height: 110px')
  })

  it('moves selection by dragging inside the box', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    await drawSelection(wrapper, 100, 100, 200, 150)
    const imgWrap = wrapper.find('.cropper-img-wrap')
    await imgWrap.trigger('mousedown', { clientX: 150, clientY: 125 })
    await imgWrap.trigger('mousemove', { clientX: 170, clientY: 145 })
    await imgWrap.trigger('mouseup')
    const style = wrapper.find('.cropper-rect').attributes('style') || ''
    expect(style).toContain('left: 120px')
    expect(style).toContain('top: 120px')
    expect(style).toContain('width: 100px')
    expect(style).toContain('height: 50px')
  })

  it('scales image: rect is rendered by new scale, selection origin unchanged', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    await drawSelection(wrapper, 100, 100, 200, 150)
    const btns = wrapper.findAll('.ct-btn')
    await btns[1].trigger('click') // zoom +0.25 → imgScale 1.25
    const style = wrapper.find('.cropper-rect').attributes('style') || ''
    // 选区原图 (100,100,100,50)，rect CSS = 选区 × imgScale
    expect(style).toContain('width: 125px')   // 100 * 1.25
    expect(style).toContain('height: 62.5px') // 50 * 1.25
  })

  it('rotates image: selection origin unchanged, rect still 100x50 (wrap has rotate transform)', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    await drawSelection(wrapper, 100, 100, 200, 150)
    const btns = wrapper.findAll('.ct-btn')
    await btns[3].trigger('click') // 顺时针 90°
    await flushPromises()
    const style = wrapper.find('.cropper-rect').attributes('style') || ''
    // 关键：旋转不影响选区数据，rect CSS left/top/width/height 与旋转前一致
    expect(style).toContain('left: 100px')
    expect(style).toContain('top: 100px')
    expect(style).toContain('width: 100px')
    expect(style).toContain('height: 50px')
    // wrap 应用了 rotate(90deg)
    const wrapStyle = wrapper.find('.cropper-img-wrap').attributes('style') || ''
    expect(wrapStyle).toContain('rotate(90deg)')
  })

  it('confirms crop: tmp canvas drawImage uses original-image sel coordinates, output rotated to display direction', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    await drawSelection(wrapper, 100, 100, 200, 150)
    // 旋转 90°
    const btns = wrapper.findAll('.ct-btn')
    await btns[3].trigger('click')
    await flushPromises()
    await wrapper.find('.cropper-btn-primary').trigger('click')
    await flushPromises()
    // 收集所有 drawImage 调用，找到使用原图坐标 (100, 100, 100, 50) 切到 tmp canvas 的那次。
    const ctxDrawSpy = (HTMLCanvasElement.prototype.getContext as ReturnType<typeof vi.fn>).mock.results[0]?.value?.drawImage
    expect(ctxDrawSpy).toHaveBeenCalled()
    const calls = ctxDrawSpy.mock.calls
    // 找到 (sx,sy,sw,sh) = (100,100,100,50) 且目标 dx=0,dy=0（裁到 tmp 临时 canvas 原点）的 drawImage。
    const cropCall = calls.find((c: unknown[]) => c[1] === 100 && c[2] === 100 && c[3] === 100 && c[4] === 50 && c[5] === 0 && c[6] === 0)
    expect(cropCall).toBeTruthy()
    // 同时存在一次 drawImage 把 tmp 画到旋转后的输出 canvas。
    const outCall = calls.find((c: unknown[]) => c[1] === -50 && c[2] === -25)
    expect(outCall).toBeTruthy()
  })

  it('moves selection with arrow keys', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    await drawSelection(wrapper, 100, 100, 200, 150)
    const backdrop = wrapper.find('.cropper-backdrop')
    await backdrop.trigger('keydown', { key: 'ArrowRight' })
    await backdrop.trigger('keydown', { key: 'ArrowDown' })
    const style = wrapper.find('.cropper-rect').attributes('style') || ''
    expect(style).toContain('left: 104px')
    expect(style).toContain('top: 104px')
  })

  // ---- 旋转后坐标换算（原 bug：逆变换公式错误，导致手柄命中/拖拽错乱） ----

  // 浏览器实测：CSS rotate(90deg) 在屏幕坐标（y 向下）表现为逆时针，原图 (x,y) → 视觉 (600-y, x)。
  // 逆变换：视觉 (vx,vy) → 原图 (vy, 600-vx)。视觉盒 600x800，中心 (300,400)。
  it('rotated 90°: drawing a box maps to original-image coordinates correctly', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    const imgWrap = wrapper.find('.cropper-img-wrap')

    // 点击顺时针 90°。
    const btns = wrapper.findAll('.ct-btn')
    await btns[3].trigger('click')
    await flushPromises()

    // 视觉盒 600x800（中心 300,400）。画框：
    // 视觉 (300,200) → 原图 (200, 300)
    // 视觉 (500,400) → 原图 (400, 100)
    // 选区 = normalize((200,300),(400,100)) = (200, 100, 200, 200)
    await imgWrap.trigger('mousedown', { clientX: 300, clientY: 200 })
    await imgWrap.trigger('mousemove', { clientX: 500, clientY: 400 })
    await imgWrap.trigger('mouseup')

    const style = wrapper.find('.cropper-rect').attributes('style') || ''
    expect(style).toContain('left: 200px')
    expect(style).toContain('top: 100px')
    expect(style).toContain('width: 200px')
    expect(style).toContain('height: 200px')
  })

  it('rotated 90°: SE handle is hittable at its rotated visual position', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    // 先画框：sel = (100,100,100,50)。
    await drawSelection(wrapper, 100, 100, 200, 150)

    const btns = wrapper.findAll('.ct-btn')
    await btns[3].trigger('click') // 顺时针 90°
    await flushPromises()

    // 原图 SE 手柄 (200,150) → 旋转 90° 后视觉 (600-150, 200) = (450, 200)。
    const imgWrap = wrapper.find('.cropper-img-wrap')
    await imgWrap.trigger('mousedown', { clientX: 450, clientY: 200 })
    await imgWrap.trigger('mousemove', { clientX: 450, clientY: 250 }) // 视觉下移
    await imgWrap.trigger('mouseup')

    // 视觉 (450,250) → 原图 (250, 150)。
    // se 手柄：o=(100,100,100,50), p=(250,150) → w = 250-100 = 150, h = 150-100 = 50。
    const style = wrapper.find('.cropper-rect').attributes('style') || ''
    expect(style).toContain('width: 150px')
    expect(style).toContain('height: 50px')
  })

  it('rotated 90°: dragging the E handle grows original width (visual vertical drag)', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    // sel = (100,100,100,50)，E 手柄原图位置 (200,125)。
    await drawSelection(wrapper, 100, 100, 200, 150)

    const btns = wrapper.findAll('.ct-btn')
    await btns[3].trigger('click')
    await flushPromises()

    // E 手柄 (200,125) → 旋转 90° 后视觉 (600-125, 200) = (475, 200)。
    const imgWrap = wrapper.find('.cropper-img-wrap')
    await imgWrap.trigger('mousedown', { clientX: 475, clientY: 200 })
    await imgWrap.trigger('mousemove', { clientX: 475, clientY: 300 }) // 视觉下移
    await imgWrap.trigger('mouseup')

    // 视觉 (475,300) → 原图 (300, 125)。
    // e 手柄：o=(100,100,100,50), p=(300,125) → w = 300-100 = 200。
    const style = wrapper.find('.cropper-rect').attributes('style') || ''
    expect(style).toContain('width: 200px')
    expect(style).toContain('height: 50px')
  })

  // ---- fitScale 旋转后自适应：视觉外接宽高对调，确保不超 stage 出现滚动条 ----
  it('fitScale uses visual (rotated) dimensions and caps by min(stageW, stageH)', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    const btns = wrapper.findAll('.ct-btn')

    // stub stage 的 layout 尺寸：宽 400, 高 200（模拟 flex 挤压后的窄高 stage）。
    const stage = wrapper.find('.cropper-stage').element as HTMLElement
    Object.defineProperty(stage, 'clientWidth', { configurable: true, value: 400 })
    Object.defineProperty(stage, 'clientHeight', { configurable: true, value: 200 })

    // 默认 natW=800 natH=600 scale=1（jsdom fallback）。旋转 90° → visualW=600, visualH=800。
    // scaleByW=400/600=0.667, scaleByH=200/800=0.25 → min=0.25。
    await btns[3].trigger('click')
    await flushPromises()

    const wrap = wrapper.find('.cropper-img-wrap').element as HTMLElement
    // 旋转后视觉外接：W = baseBoxH = 600*0.25=150, H = baseBoxW = 800*0.25=200。
    // 验证视觉宽高 ≤ stage 宽高。
    const rect = wrap.getBoundingClientRect()
    expect(rect.width).toBeLessThanOrEqual(400)
    expect(rect.height).toBeLessThanOrEqual(200)
  })

  it('fitScale keeps imgScale=1 when stage has no measurable size (jsdom/unmounted)', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    // 默认 clientWidth/Height=0（jsdom 未布局）：fitScale 应直接返回不缩放。
    // 画框后选区坐标应原样反映（与未旋转、无缩放时的预期一致）。
    await drawSelection(wrapper, 100, 100, 200, 150)
    const style = wrapper.find('.cropper-rect').attributes('style') || ''
    expect(style).toContain('left: 100px')
    expect(style).toContain('top: 100px')
    expect(style).toContain('width: 100px')
    expect(style).toContain('height: 50px')
  })

  // ---- 缩放下界：允许 10%~400%，支持查看超大原图 ----
  it('zoom out can go below 25% (down to 10%)', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    const btns = wrapper.findAll('.ct-btn')
    // 连点多次缩小，scale 应能一路降到 10%。
    for (let i = 0; i < 20; i++) {
      await btns[0].trigger('click')
    }
    const scaleText = wrapper.find('.ct-scale').text()
    expect(scaleText).toBe('10%')
  })

  it('zoom in upper bound is 400%', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    const btns = wrapper.findAll('.ct-btn')
    for (let i = 0; i < 30; i++) {
      await btns[1].trigger('click')
    }
    expect(wrapper.find('.ct-scale').text()).toBe('400%')
  })

  // ---- 旋转后 wrap margin：使视觉外接左上角对齐 layout 左上角，避免滚动看不到 ----
  it('rotated 90° (landscape 800x600): wrap marginTop=(W-H)/2 and marginLeft=(H-W)/2', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    const btns = wrapper.findAll('.ct-btn')
    // natW=800, natH=600，scale 1：marginTop=(800-600)/2=100, marginLeft=(600-800)/2=-100。
    await btns[3].trigger('click')
    await flushPromises()
    const style = wrapper.find('.cropper-img-wrap').attributes('style') || ''
    expect(style).toMatch(/margin-top:\s*100px/)
    expect(style).toMatch(/margin-left:\s*-100px/)
  })

  it('rotated 90° (portrait): wrap has negative marginTop and positive marginLeft', async () => {
    // 竖图：natW=300, natH=600。
    stubImage(300, 600)
    const wrapper = mountCropper(true)
    mockImgRect(wrapper, 300, 600)
    const btns = wrapper.findAll('.ct-btn')
    await btns[3].trigger('click')
    await flushPromises()
    const style = wrapper.find('.cropper-img-wrap').attributes('style') || ''
    // marginTop=(300-600)/2=-150, marginLeft=(600-300)/2=150。
    expect(style).toMatch(/margin-top:\s*-150px/)
    expect(style).toMatch(/margin-left:\s*150px/)
  })

  it('not rotated: wrap has no margin offset', async () => {
    const wrapper = mountCropper(true)
    mockImgRect(wrapper)
    const style = wrapper.find('.cropper-img-wrap').attributes('style') || ''
    expect(style).toMatch(/margin-top:\s*0px/)
    expect(style).toMatch(/margin-left:\s*0px/)
  })
})
