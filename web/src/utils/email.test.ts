import { describe, it, expect } from 'vitest'
import { maskEmail } from './email'

describe('maskEmail 邮箱脱敏', () => {
  it('保留首字符与域名，其余打码', () => {
    expect(maskEmail('student@peak.local')).toBe('s***@peak.local')
    expect(maskEmail('a@b.com')).toBe('a***@b.com')
  })

  it('无 @ 或 @ 在首位时原样返回', () => {
    expect(maskEmail('not-an-email')).toBe('not-an-email')
    expect(maskEmail('@b.com')).toBe('@b.com')
  })
})
