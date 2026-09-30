import { computed, type Ref } from 'vue'

// 密码强度档位：0 未评分（不展示）、1 弱、2 中、3 强。
export interface PasswordStrength {
  score: 0 | 1 | 2 | 3
  label: string
}

const LABELS: Record<1 | 2 | 3, string> = { 1: '弱', 2: '中', 3: '强' }

// 评分规则：不足 8 位不计分（后端会拒绝）；
// 按字符类别（小写/大写/数字/符号）数量与长度综合升档：
// 仅 1 类字符 → 弱；2 类 → 中；3 类及以上且长度 ≥ 10 → 强。
export function passwordStrength(pwd: string): PasswordStrength {
  if (pwd.length < 8) return { score: 0, label: '' }
  const classes = [/[a-z]/, /[A-Z]/, /\d/, /[^A-Za-z0-9]/].filter((re) => re.test(pwd)).length
  const score: 1 | 2 | 3 = classes >= 3 && pwd.length >= 10 ? 3 : classes >= 2 ? 2 : 1
  return { score, label: LABELS[score] }
}

// 密码强度组合式函数：响应式跟随输入变化。
export function usePasswordStrength(password: Ref<string>) {
  const strength = computed(() => passwordStrength(password.value))

  // 三档指示条的激活色与文案色（Tailwind 类名，随档位切换）。
  const barClass = computed(() => {
    switch (strength.value.score) {
      case 3:
        return { active: 'bg-emerald-500', text: 'text-emerald-600' }
      case 2:
        return { active: 'bg-amber-500', text: 'text-amber-600' }
      case 1:
        return { active: 'bg-red-500', text: 'text-red-600' }
      default:
        return { active: '', text: '' }
    }
  })

  return { strength, barClass }
}
