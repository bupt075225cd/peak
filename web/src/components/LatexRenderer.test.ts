import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import LatexRenderer from './LatexRenderer.vue'

describe('LatexRenderer', () => {
  it('renders valid LaTeX to HTML', () => {
    const wrapper = mount(LatexRenderer, { props: { expr: 'x^2 + y^2' } })
    // 渲染成功时使用 .latex 类并注入 KaTeX HTML。
    const el = wrapper.find('.latex')
    expect(el.exists()).toBe(true)
    expect(el.html()).toContain('katex')
  })

  it('renders empty string as fallback', () => {
    const wrapper = mount(LatexRenderer, { props: { expr: '' } })
    // 空表达式不渲染 .latex，走 fallback 分支。
    expect(wrapper.find('.latex').exists()).toBe(false)
    expect(wrapper.find('.latex-fallback').exists()).toBe(true)
    expect(wrapper.find('.latex-fallback').text()).toBe('')
  })

  it('renders whitespace-only expr as fallback', () => {
    const wrapper = mount(LatexRenderer, { props: { expr: '   ' } })
    expect(wrapper.find('.latex').exists()).toBe(false)
    expect(wrapper.find('.latex-fallback').exists()).toBe(true)
  })

  it('supports displayMode prop', () => {
    const wrapper = mount(LatexRenderer, {
      props: { expr: '\\frac{1}{2}', displayMode: true },
    })
    expect(wrapper.find('.latex').exists()).toBe(true)
  })
})
