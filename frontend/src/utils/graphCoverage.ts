// M6-1 WS1.5 · 图谱覆盖进度的纯逻辑（设置页进度条与图谱页「未覆盖」标注共用）。
//
// 分母口径（D3）：粘贴类文档无契约语义、从不进图谱，后端已在 eligible 里剔除——
// 这里只负责把「状态计数 + 分母」折算成可展示的数字，并守住几种失真输入。

export type GraphCoverageCounts = {
  ready?: number
  pending?: number
  building?: number
  failed?: number
}

export type GraphCoveragePayload = {
  available?: boolean
  reason?: string
  total_docs?: number
  exempt_manual?: number
  eligible?: number
  coverage?: GraphCoverageCounts
}

export type GraphCoverageSummary = {
  /** 进度条百分比（0–100）；分母为 0 时为 null（无可用文档，不渲染进度条）。 */
  percent: number | null
  ready: number
  pending: number
  building: number
  failed: number
  eligible: number
  /** eligible - ready，下限 0：状态计数滞后时不能报负数。 */
  uncovered: number
}

const toCount = (value: unknown): number =>
  typeof value === 'number' && Number.isFinite(value) && value > 0 ? Math.floor(value) : 0

/**
 * 折算覆盖率。分母缺省时用状态计数之和兜底（老数据面没有 eligible 字段），
 * 但分母小于 ready 时以 ready 为准——分母失真不该让进度条超过 100%。
 */
export function coverageSummary(payload: GraphCoveragePayload | null | undefined): GraphCoverageSummary {
  const cov = payload?.coverage || {}
  const ready = toCount(cov.ready)
  const pending = toCount(cov.pending)
  const building = toCount(cov.building)
  const failed = toCount(cov.failed)
  const tracked = ready + pending + building + failed
  const rawEligible = toCount(payload?.eligible) || tracked
  const eligible = Math.max(rawEligible, ready)
  return {
    percent: eligible > 0 ? Math.round((ready / eligible) * 100) : null,
    ready,
    pending,
    building,
    failed,
    eligible,
    uncovered: Math.max(0, eligible - ready),
  }
}
