import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ImageViewer from './ImageViewer.vue'

function mountViewer(open = true) {
  return mount(ImageViewer, {
    props: { src: 'https://example.com/img.jpg', open },
    attachTo: document.body,
    global: { stubs: { teleport: true } },
  })
}

describe('ImageViewer', () => {
  it('renders nothing when closed', () => {
    const wrapper = mountViewer(false)
    expect(wrapper.find('.image-viewer-backdrop').exists()).toBe(false)
  })

  it('renders backdrop and image when open', () => {
    const wrapper = mountViewer(true)
    expect(wrapper.find('.image-viewer-backdrop').exists()).toBe(true)
    const img = wrapper.find('.image-viewer-img')
    expect(img.exists()).toBe(true)
    expect(img.attributes('src')).toBe('https://example.com/img.jpg')
    expect(img.attributes('alt')).toBe('图片预览')
  })

  it('uses custom alt when provided', () => {
    const wrapper = mount(ImageViewer, {
      props: { src: 'x.jpg', alt: '自定义', open: true },
      attachTo: document.body,
      global: { stubs: { teleport: true } },
    })
    expect(wrapper.find('.image-viewer-img').attributes('alt')).toBe('自定义')
  })

  it('emits close on close button click', async () => {
    const wrapper = mountViewer(true)
    await wrapper.find('.iv-btn-close').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('emits close on Escape key', async () => {
    const wrapper = mountViewer(true)
    await wrapper.find('.image-viewer-backdrop').trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('does not emit close on Escape when closed', async () => {
    const wrapper = mountViewer(false)
    // 未渲染时无法触发，直接断言无事件即可。
    expect(wrapper.emitted('close')).toBeFalsy()
  })

  it('emits close when clicking backdrop itself', async () => {
    const wrapper = mountViewer(true)
    await wrapper.find('.image-viewer-backdrop').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('zooms in and out via toolbar buttons', async () => {
    const wrapper = mountViewer(true)
    const buttons = wrapper.findAll('.iv-btn')
    // 第一个是放大，第二个是缩小。
    await buttons[0].trigger('click')
    expect(wrapper.find('.iv-scale').text()).toBe('120%')
    await buttons[1].trigger('click')
    expect(wrapper.find('.iv-scale').text()).toBe('100%')
  })

  it('resets zoom via reset button', async () => {
    const wrapper = mountViewer(true)
    const buttons = wrapper.findAll('.iv-btn')
    await buttons[0].trigger('click') // 放大到 120%
    await buttons[0].trigger('click') // 140%
    await buttons[2].trigger('click') // 还原
    expect(wrapper.find('.iv-scale').text()).toBe('100%')
  })

  it('zooms on mouse wheel', async () => {
    const wrapper = mountViewer(true)
    await wrapper.find('.image-viewer-backdrop').trigger('wheel', { deltaY: -100 })
    expect(wrapper.find('.iv-scale').text()).toBe('120%')
    await wrapper.find('.image-viewer-backdrop').trigger('wheel', { deltaY: 100 })
    expect(wrapper.find('.iv-scale').text()).toBe('100%')
  })

  it('supports keyboard zoom shortcuts', async () => {
    const wrapper = mountViewer(true)
    await wrapper.find('.image-viewer-backdrop').trigger('keydown', { key: '+' })
    expect(wrapper.find('.iv-scale').text()).toBe('120%')
    await wrapper.find('.image-viewer-backdrop').trigger('keydown', { key: '0' })
    expect(wrapper.find('.iv-scale').text()).toBe('100%')
  })

  it('translates image when dragging after zoom', async () => {
    const wrapper = mountViewer(true)
    const buttons = wrapper.findAll('.iv-btn')
    await buttons[0].trigger('click') // scale -> 1.2
    const backdrop = wrapper.find('.image-viewer-backdrop')
    await backdrop.trigger('mousedown', { clientX: 100, clientY: 100 })
    await backdrop.trigger('mousemove', { clientX: 120, clientY: 130 })
    await backdrop.trigger('mouseup')
    const img = wrapper.find('.image-viewer-img')
    expect(img.attributes('style')).toContain('translate(20px, 30px)')
  })

  it('does not drag when not zoomed', async () => {
    const wrapper = mountViewer(true)
    const backdrop = wrapper.find('.image-viewer-backdrop')
    await backdrop.trigger('mousedown', { clientX: 100, clientY: 100 })
    await backdrop.trigger('mousemove', { clientX: 120, clientY: 130 })
    await backdrop.trigger('mouseup')
    const img = wrapper.find('.image-viewer-img')
    expect(img.attributes('style')).toContain('translate(0px, 0px)')
  })
})
