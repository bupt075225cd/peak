import { readonly, ref } from 'vue'
import { MONEY_MASK, formatCents, formatWan } from '../utils/money'

// 金额隐藏状态：模块级单例，总览/明细/桑基图共享同一开关。
//
// 只把"是否隐藏"这一个布尔值写入 sessionStorage；任何金额、名称都不落
// 浏览器存储，避免本地磁盘或分享链接泄露家庭资产。
const HIDDEN_KEY = 'peak_ledger_hidden'
// 默认隐藏：进入账本先看不到金额，只有本次会话里显式点开过眼睛按钮（存 '0'）才显示。
const hidden = ref(sessionStorage.getItem(HIDDEN_KEY) !== '0')

function persist(next: boolean): void {
  hidden.value = next
  // 两种状态都落盘：只存 '1' 的话，点开金额后刷新又会退回默认的隐藏态。
  sessionStorage.setItem(HIDDEN_KEY, next ? '1' : '0')
}

let bound = false

// 页面切到后台（切换标签页/最小化）时自动隐藏，防止被旁人看到。
//
// 只在内存里置为隐藏、不写 sessionStorage：页面跳转/刷新同样会触发
// visibilitychange(hidden)，若落盘会导致每次切页后金额都被重新隐藏。
function bindAutoHide(): void {
  if (bound || typeof document === 'undefined') return
  bound = true
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) hidden.value = true
  })
}

export function useLedgerPrivacy() {
  bindAutoHide()

  return {
    hidden: readonly(hidden),
    toggle: () => persist(!hidden.value),
    hide: () => persist(true),
    show: () => persist(false),
    /** 通用文本脱敏。 */
    mask: (text: string) => (hidden.value ? MONEY_MASK : text),
    /** 以「元」展示并脱敏。 */
    money: (cents: number) => (hidden.value ? MONEY_MASK : formatCents(cents)),
    /** 以「万」展示并脱敏。 */
    wan: (cents: number) => (hidden.value ? MONEY_MASK : formatWan(cents)),
  }
}

/** 非组件环境（如图表配置）读取当前隐藏状态。 */
export function isLedgerHidden(): boolean {
  return hidden.value
}
