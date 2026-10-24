import { ref } from 'vue'
import {
  cancelDeletion,
  errorMessage,
  exportLedger,
  getDeletion,
  isReauthRequired,
  requestDeletion,
  type LedgerDeletion,
} from '../api/ledger'
import { downloadBlob } from '../utils/watermark'

/**
 * 账本高风险动作编排：导出（可携带权）、注销申请/撤销，以及重认证后的自动重试。
 *
 * 导出与注销都要求 5 分钟内的新鲜令牌；服务端返回 428 时抛出，这里统一弹出
 * 重认证弹层，验证成功后重放原动作，避免每个页面各写一套。
 */
export function useLedgerPrivacyActions() {
  const exporting = ref(false)
  const deletion = ref<LedgerDeletion | null>(null)
  const deletionOpen = ref(false)
  const deletionBusy = ref(false)
  const deletionError = ref('')
  const error = ref('')

  const reauthOpen = ref(false)
  const reauthAction = ref('')
  const pendingAction = ref<(() => Promise<void>) | null>(null)

  async function loadDeletion(): Promise<void> {
    try {
      deletion.value = await getDeletion()
    } catch (err) {
      // 注销状态属辅助信息，失败不阻塞明细页。
      console.error('加载注销状态失败', err)
    }
  }

  function requireReauth(action: string, retry: () => Promise<void>): void {
    reauthAction.value = action
    pendingAction.value = retry
    reauthOpen.value = true
  }

  async function onReauthSuccess(): Promise<void> {
    reauthOpen.value = false
    const retry = pendingAction.value
    pendingAction.value = null
    if (retry) await retry()
  }

  async function handleExport(): Promise<void> {
    if (exporting.value) return
    exporting.value = true
    error.value = ''
    try {
      const { blob, filename } = await exportLedger()
      downloadBlob(blob, filename)
    } catch (err) {
      if (isReauthRequired(err)) {
        requireReauth('导出全部数据', handleExport)
        return
      }
      console.error('导出账本失败', err)
      error.value = errorMessage(err, '导出失败，请稍后重试')
    } finally {
      exporting.value = false
    }
  }

  async function handleDeletion(): Promise<void> {
    deletionBusy.value = true
    deletionError.value = ''
    try {
      deletion.value = await requestDeletion()
      deletionOpen.value = false
    } catch (err) {
      if (isReauthRequired(err)) {
        deletionOpen.value = false
        requireReauth('注销账本', handleDeletion)
        return
      }
      deletionError.value = errorMessage(err, '提交注销申请失败')
    } finally {
      deletionBusy.value = false
    }
  }

  async function handleCancelDeletion(): Promise<void> {
    deletionBusy.value = true
    deletionError.value = ''
    try {
      deletion.value = await cancelDeletion()
      deletionOpen.value = false
    } catch (err) {
      deletionError.value = errorMessage(err, '撤销失败，请稍后重试')
    } finally {
      deletionBusy.value = false
    }
  }

  return {
    exporting,
    deletion,
    deletionOpen,
    deletionBusy,
    deletionError,
    error,
    reauthOpen,
    reauthAction,
    loadDeletion,
    handleExport,
    handleDeletion,
    handleCancelDeletion,
    onReauthSuccess,
  }
}
