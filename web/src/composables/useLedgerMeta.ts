import { computed, ref } from 'vue'
import {
  acceptConsent,
  errorMessage,
  getConsent,
  getMeta,
  listMembers,
  type LedgerConsent,
  type LedgerMember,
  type LedgerMeta,
} from '../api/ledger'

// 账本元数据在四个页面间共享：枚举、成员标签与同意状态都只需拉取一次。
// 模块级缓存避免切页重复请求，也让「同意」后的状态立即对其他页面生效。
const meta = ref<LedgerMeta | null>(null)
const consent = ref<LedgerConsent | null>(null)
const members = ref<LedgerMember[]>([])
const loading = ref(false)
const accepting = ref(false)
const error = ref('')

let inflight: Promise<void> | null = null

export function useLedgerMeta() {
  // 同意是访问账本数据的前置条件：服务端未留痕时 summary/items 返回 403。
  const consentRequired = computed(() => consent.value?.required ?? meta.value?.consent_required ?? true)

  async function loadMembers(): Promise<void> {
    try {
      members.value = await listMembers()
    } catch (err) {
      // 成员标签属可选功能，加载失败不阻塞主流程。
      console.error('加载成员标签失败', err)
    }
  }

  async function load(force = false): Promise<void> {
    if (!force && meta.value && consent.value) return
    if (inflight) return inflight
    loading.value = true
    error.value = ''
    inflight = (async () => {
      try {
        const [metaResult, consentResult] = await Promise.all([getMeta(), getConsent()])
        meta.value = metaResult
        consent.value = consentResult
        await loadMembers()
      } catch (err) {
        error.value = errorMessage(err, '加载账本配置失败，请稍后重试')
      } finally {
        loading.value = false
        inflight = null
      }
    })()
    return inflight
  }

  async function accept(): Promise<boolean> {
    const version = meta.value?.consent_policy_version ?? consent.value?.policy_version
    if (!version) {
      error.value = '未获取到告知版本，请刷新后重试'
      return false
    }
    accepting.value = true
    error.value = ''
    try {
      consent.value = await acceptConsent(version)
      return true
    } catch (err) {
      error.value = errorMessage(err, '记录同意失败，请稍后重试')
      return false
    } finally {
      accepting.value = false
    }
  }

  return {
    meta,
    consent,
    members,
    loading,
    accepting,
    error,
    consentRequired,
    load,
    loadMembers,
    accept,
  }
}

/** 测试用：清空模块级缓存（生产代码不调用）。 */
export function resetLedgerMetaCache(): void {
  meta.value = null
  consent.value = null
  members.value = []
  error.value = ''
  inflight = null
}
