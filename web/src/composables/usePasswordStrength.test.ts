import { describe, it, expect } from 'vitest'
import { ref } from 'vue'
import { passwordStrength, usePasswordStrength } from './usePasswordStrength'

describe('passwordStrength 评分', () => {
  it('不足 8 位不评分（后端会拒绝）', () => {
    expect(passwordStrength('').score).toBe(0)
    expect(passwordStrength('abc').score).toBe(0)
    expect(passwordStrength('abcd1234'.slice(0, 7)).score).toBe(0)
    expect(passwordStrength('a1b2c3d').label).toBe('')
  })

  it('仅单一字符类别 → 弱', () => {
    expect(passwordStrength('abcdefgh')).toEqual({ score: 1, label: '弱' })
    expect(passwordStrength('12345678')).toEqual({ score: 1, label: '弱' })
  })

  it('两类字符 → 中', () => {
    expect(passwordStrength('abcd1234')).toEqual({ score: 2, label: '中' })
    expect(passwordStrength('abcd1234!')).toEqual({ score: 2, label: '中' }) // 3 类但长度不足 10
  })

  it('三类字符且长度 ≥ 10 → 强', () => {
    expect(passwordStrength('Abcd1234!x')).toEqual({ score: 3, label: '强' })
    expect(passwordStrength('P@ssw0rd123')).toEqual({ score: 3, label: '强' })
  })
})

describe('usePasswordStrength 响应式', () => {
  it('跟随输入实时升档', () => {
    const pwd = ref('')
    const { strength, barClass } = usePasswordStrength(pwd)

    expect(strength.value.score).toBe(0)
    expect(barClass.value.active).toBe('')

    pwd.value = 'onlyletters'
    // 11 位但仅小写单类 → 弱。
    expect(strength.value.score).toBe(1)
    expect(strength.value.label).toBe('弱')
    expect(barClass.value.active).toBe('bg-red-500')

    pwd.value = 'Onlyletters1'
    expect(strength.value.score).toBe(3)
    expect(strength.value.label).toBe('强')
    expect(barClass.value.active).toBe('bg-emerald-500')
    expect(barClass.value.text).toBe('text-emerald-600')
  })
})
