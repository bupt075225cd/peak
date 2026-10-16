import { ref, type Ref } from 'vue'
import {
  getDynastyStats,
  getVolumeStats,
  getAuthorStats,
  getWordStats,
  type DynastyStat,
  type VolumeStat,
  type AuthorCount,
  type WordStat,
} from '../api/poetry'

// 可视化总览的统计数据集合。
export interface PoetryStats {
  dynasties: DynastyStat[]
  volumes: VolumeStat[]
  authors: AuthorCount[]
  words: WordStat[]
  totalChars: number
}

// usePoetryStats 组合式函数：并行加载四类统计，暴露加载/错误状态。
export function usePoetryStats(authorTop = 10, wordTop = 100) {
  const stats: Ref<PoetryStats> = ref({
    dynasties: [],
    volumes: [],
    authors: [],
    words: [],
    totalChars: 0,
  })
  const loading = ref(false)
  const error = ref('')

  async function load() {
    loading.value = true
    error.value = ''
    try {
      const [dynasties, volumes, authors, wordResult] = await Promise.all([
        getDynastyStats(),
        getVolumeStats(),
        getAuthorStats(authorTop),
        getWordStats(wordTop),
      ])
      stats.value = {
        dynasties,
        volumes,
        authors,
        words: wordResult.items,
        totalChars: wordResult.total_chars,
      }
    } catch (err) {
      console.error('加载古诗文统计数据失败', err)
      const data = (err as { response?: { data?: { message?: string } } })?.response?.data
      error.value = data?.message || '统计数据加载失败，请稍后重试'
    } finally {
      loading.value = false
    }
  }

  return { stats, loading, error, load }
}

// 篇目总量：由朝代分布求和。
export function totalPoems(dynasties: DynastyStat[]): number {
  return dynasties.reduce((sum, d) => sum + d.count, 0)
}
