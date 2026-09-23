import { computed, onUnmounted, ref, type Ref } from 'vue'
import {
  getKnowledgeBaseGraphDocStatus,
  retryKnowledgeBaseGraphDocs,
} from '@/api/knowledge-base'

/**
 * ADR-008 决策 4：文档列表页的图谱状态徽标。
 *
 * 数据不在 WeKnora 的列表接口里（ADR 明确不许把跨服务调用塞进列表热路径），
 * 而是列表渲染出来后由这里批量补一次，前端本地合并。翻页不重复打：缓存里
 * 已有的 id 默认跳过。
 */

/** 单次请求携带的 doc id 上限，与 Go 代理的 graphDocStatusMaxIDs 对齐。 */
const CHUNK = 200
/** 有文档处于进行中状态时的轻轮询间隔。 */
const POLL_MS = 10000

/** 这些状态会自己变（worker 在推进），需要轻轮询。 */
const ACTIVE_STATUSES = new Set(['pending', 'building', 'deleting'])

export interface GraphDocDetail {
  error?: string
  attempts?: number
}

export function useGraphDocStatus(kbId: Ref<string> | string) {
  const kb = computed(() => (typeof kbId === 'string' ? kbId : kbId.value))
  const states = ref<Record<string, string>>({})
  const details = ref<Record<string, GraphDocDetail>>({})
  /**
   * false = 代理降级（STARKB_API_URL 未配置 / starkb-api 不可达）。
   * 此时整列徽标隐藏 —— 显示成「未建图」是误报，比不显示更糟。
   */
  const available = ref(true)
  const loading = ref(false)

  let tracked: string[] = []
  let timer: ReturnType<typeof setInterval> | null = null

  const hasActive = () =>
    tracked.some((id) => ACTIVE_STATUSES.has(states.value[id] || ''))

  const stopPoll = () => {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  const ensurePoll = () => {
    if (!available.value) {
      stopPoll()
      return
    }
    if (hasActive() && !timer) {
      timer = setInterval(() => {
        void refresh(tracked, true)
      }, POLL_MS)
    } else if (!hasActive()) {
      stopPoll()
    }
  }

  /**
   * 拉取给定文档的图谱状态。
   *
   * @param ids   当前列表里可见的 doc id
   * @param force 忽略缓存全量刷新（轮询与重试后用）
   */
  const refresh = async (ids: string[], force = false) => {
    const target = kb.value
    if (!target) return
    tracked = Array.from(new Set(ids.filter(Boolean)))
    const todo = force ? tracked : tracked.filter((id) => !(id in states.value))
    if (!todo.length) {
      ensurePoll()
      return
    }

    loading.value = true
    try {
      for (let i = 0; i < todo.length; i += CHUNK) {
        const res: any = await getKnowledgeBaseGraphDocStatus(target, todo.slice(i, i + CHUNK))
        const payload = res?.data || res
        if (payload?.available === false) {
          available.value = false
          stopPoll()
          return
        }
        available.value = true
        Object.assign(states.value, payload?.states || {})
        Object.assign(details.value, payload?.details || {})
      }
    } catch {
      // 代理不可达不该让文档列表报错，只是不显示徽标。
      available.value = false
      stopPoll()
    } finally {
      loading.value = false
      ensurePoll()
    }
  }

  /** 失败徽标的重试：先乐观置为排队中，worker 接手后由轮询回读真实状态。 */
  const retry = async (id: string) => {
    const target = kb.value
    if (!target) return false
    try {
      await retryKnowledgeBaseGraphDocs(target, [id])
      states.value = { ...states.value, [id]: 'pending' }
      details.value = { ...details.value, [id]: {} }
      ensurePoll()
      return true
    } catch {
      return false
    }
  }

  const statusOf = (id: string) => states.value[id] || 'none'
  const detailOf = (id: string): GraphDocDetail => details.value[id] || {}

  onUnmounted(stopPoll)

  return { states, details, available, loading, refresh, retry, statusOf, detailOf }
}
