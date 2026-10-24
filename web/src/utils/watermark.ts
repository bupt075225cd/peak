// 图片下载与水印：导出的资产组成图带伪名短标记与生成日期，
// 便于外泄后溯源，同时提示"该图含敏感财务信息"。

/** 下载 Blob。 */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.rel = 'noopener'
  document.body.appendChild(link)
  link.click()
  link.remove()
  // 延迟释放，避免部分浏览器下载中断。
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/**
 * 在图片右下角叠加水印后返回新的 dataURL。
 *
 * @param dataUrl 源图（PNG dataURL）
 * @param mark 水印文本（应为伪名短标记 + 日期，不含金额）
 */
export async function watermarkDataURL(dataUrl: string, mark: string): Promise<string> {
  const image = await loadImage(dataUrl)
  const canvas = document.createElement('canvas')
  canvas.width = image.width
  canvas.height = image.height
  const ctx = canvas.getContext('2d')
  if (!ctx) return dataUrl

  ctx.drawImage(image, 0, 0)
  const fontSize = Math.max(12, Math.round(image.width * 0.014))
  ctx.font = `${fontSize}px "PingFang SC", system-ui, sans-serif`
  ctx.textAlign = 'right'
  ctx.textBaseline = 'bottom'
  const padding = Math.round(fontSize * 1.2)
  const text = `家庭资产账本 · ${mark}`
  const width = ctx.measureText(text).width

  ctx.globalAlpha = 0.72
  ctx.fillStyle = 'rgba(255,255,255,0.82)'
  ctx.fillRect(image.width - width - padding * 2, image.height - fontSize - padding * 1.6, width + padding, fontSize + padding * 0.9)
  ctx.globalAlpha = 0.85
  ctx.fillStyle = '#475569'
  ctx.fillText(text, image.width - padding, image.height - padding * 0.8)
  return canvas.toDataURL('image/png')
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error('图片加载失败'))
    image.src = src
  })
}

/** dataURL → Blob，供下载使用。 */
export function dataURLToBlob(dataUrl: string): Blob {
  const [head, body] = dataUrl.split(',')
  const mime = /:(.*?);/.exec(head)?.[1] ?? 'image/png'
  const binary = atob(body ?? '')
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i)
  return new Blob([bytes], { type: mime })
}

const MARK_KEY = 'peak_ledger_mark'

/**
 * 下载水印文本：会话内稳定的随机短标记 + 日期。
 *
 * 标记仅用于「同一会话导出的图能互相关联」，不含用户 ID、手机号或金额，
 * 也不写入 localStorage，关闭标签页即失效。
 */
export function ledgerMark(date: Date = new Date()): string {
  let token = sessionStorage.getItem(MARK_KEY)
  if (!token) {
    token = Math.random().toString(36).slice(2, 8).toUpperCase()
    sessionStorage.setItem(MARK_KEY, token)
  }
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${token} · ${date.getFullYear()}.${month}.${day}`
}
