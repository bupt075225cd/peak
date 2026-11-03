import { mount } from '@vue/test-utils'
import { describe, it, expect } from 'vitest'
import StemText from './StemText.vue'

describe('StemText.vue', () => {
  it('把 Markdown 表格渲染成 <table>（已保存题目的形式）', () => {
    const wrapper = mount(StemText, {
      props: { text: '如表\n| a | b |\n| --- | --- |\n| 1 | 2 |' },
    })
    expect(wrapper.findAll('table')).toHaveLength(1)
    expect(wrapper.findAll('th')).toHaveLength(2)
    expect(wrapper.findAll('td')).toHaveLength(2)
    expect(wrapper.text()).toContain('如表')
  })

  it('按占位符 + tables 渲染表格（识别结果的形式）', () => {
    const wrapper = mount(StemText, {
      props: {
        text: '如表{{table1}}所示',
        tables: [{ id: 'table1', headers: ['档位'], rows: [['第一档']] }],
      },
    })
    expect(wrapper.find('table').exists()).toBe(true)
    expect(wrapper.text()).toContain('第一档')
    expect(wrapper.text()).not.toContain('{{table1}}')
  })

  it('关键词高亮命中表格单元格', () => {
    const wrapper = mount(StemText, {
      props: { text: '| 单价 |\n| --- |\n| 3.5 |', keyword: '单价' },
    })
    expect(wrapper.find('mark').exists()).toBe(true)
  })

  it('clamp 时限制展示高度', () => {
    const wrapper = mount(StemText, { props: { text: '一段题干', clamp: 3 } })
    expect(wrapper.attributes('style') || '').toContain('max-height')
  })
})
