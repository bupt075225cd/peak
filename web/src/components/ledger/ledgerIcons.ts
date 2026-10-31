import type { Component } from 'vue'
import {
  Banknote,
  Building2,
  Car,
  CircleEllipsis,
  Coins,
  CreditCard,
  FileText,
  Gem,
  HandCoins,
  House,
  Landmark,
  LineChart,
  MapPin,
  Package,
  Palette,
  PiggyBank,
  Receipt,
  Shield,
  Smartphone,
  TrendingUp,
  Wallet,
} from 'lucide-vue-next'

// 子类图标由服务端 /meta 下发名称，前端按名映射为组件（单一真源在 Go 侧）。
const ICONS: Record<string, Component> = {
  Landmark,
  PiggyBank,
  Wallet,
  Smartphone,
  Coins,
  House,
  Building2,
  Car,
  MapPin,
  Gem,
  Palette,
  Package,
  LineChart,
  TrendingUp,
  Receipt,
  Shield,
  HandCoins,
  FileText,
  CreditCard,
  Banknote,
  CircleEllipsis,
}

/** 按名称取图标组件，未知时回退为 Package。 */
export function ledgerIcon(name: string): Component {
  return ICONS[name] ?? Package
}
