import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import GeometryCropper from './GeometryCropper.vue'

// jsdom 未实现 canvas，mock 掉 getContext / toBlob。
function stubCanvas() {
  const ctx = {
    drawImage: vi.fn(),
  }
  const toBlob = vi.fn((cb: (b: Blob | null) => void) => {
    cb(new Blob(['fake'], { type: 'image/jpeg' }))
  })
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(ctx as never)
  vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation(toBlob)
}

function mountCropper(open = true) {
  return mount(GeometryCropper, {
    props: { open, src: 'https://example.com/paper.jpg' },
    attachTo: document.body,
    global: { stubs: { teleport: true } },
  })
}

describe('GeometryCropper', () => {
  beforeEach(() => {
    stubCanvas()
    // mock requestAnimationFrame 以便 watch open 时的回调可控。
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      cb(0)
      return 0
    })
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
    const imgWrap = wrapper.find('.cropper-img-wrap')
    // 模拟图片元素的 getBoundingClientRect。
    const img = wrapper.find('.cropper-img')
    vi.spyOn(img.element as HTMLElement, 'getBoundingClientRect').mockReturnValue({
      left: 0, top: 0, right: 400, bottom: 300, width: 400, height: 300,
      x: 0, y: 0, toJSON: () => ({}),
    } as DOMRect)

    await imgWrap.trigger('mousedown', { clientX: 10, clientY: 10 })
    await imgWrap.trigger('mousemove', { clientX: 60, clientY: 50 })
    await imgWrap.trigger('mouseup')

    // 框选矩形应出现，且位置/尺寸正确。
    const rect = wrapper.find('.cropper-rect')
    expect(rect.exists()).toBe(true)
    const style = rect.attributes('style') || ''
    expect(style).toContain('left: 10px')
    expect(style).toContain('top: 10px')
    expect(style).toContain('width: 50px')
    expect(style).toContain('height: 40px')
  })

  it('confirms crop and emits File after dragging', async () => {
    const wrapper = mountCropper(true)
    const imgWrap = wrapper.find('.cropper-img-wrap')
    const img = wrapper.find('.cropper-img')
    vi.spyOn(img.element as HTMLElement, 'getBoundingClientRect').mockReturnValue({
      left: 0, top: 0, right: 400, bottom: 300, width: 400, height: 300,
      x: 0, y: 0, toJSON: () => ({}),
    } as DOMRect)
    // naturalWidth 用于计算缩放比。
    Object.defineProperty(img.element as HTMLImageElement, 'naturalWidth', { value: 800, configurable: true })

    await imgWrap.trigger('mousedown', { clientX: 10, clientY: 10 })
    await imgWrap.trigger('mousemove', { clientX: 60, clientY: 50 })
    await imgWrap.trigger('mouseup')

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
})
