// M6-1 WS1.2 · 图谱浏览器页的纯逻辑层（与渲染/请求解耦，可单测）。
//
// 两处容易出错的归一化：后端字段可能缺省（老数据面 / 降级响应），以及证据条目要
// 翻译成 ProvenancePanel 认得的输入形状——字段名对不上时面板会静默显示空态，
// 而不是报错，所以值得用测试盯住。

import type { GraphEdgeDatum, GraphNodeDatum } from '@/components/knowledge/graphForceChart'
import { UNTYPED } from '@/components/knowledge/graphForceChart'
import type { ProvenanceInput } from '@/utils/provenance'

export type GraphViewPayload = {
  available?: boolean
  reason?: string
  workspace?: string
  nodes?: GraphNodeDatum[]
  edges?: GraphEdgeDatum[]
  total_nodes?: number
  total_edges?: number
  truncated?: boolean
  /** P0-2（图谱浏览器规划 2026-09-25）：ego 子图回执。 */
  mode?: 'overview' | 'ego'
  center?: string | null
  depth?: number | null
  types?: string[]
}

export type GraphEntitySearchPayload = {
  available?: boolean
  reason?: string
  query?: string
  results?: { id: string; degree?: number; entity_type?: string; description?: string }[]
}

export type GraphEntityRef = {
  id: string
  entity_type?: string
  description?: string
  /** LightRAG 会把多值 description 用 <SEP> 合并存储；结构化列表优先。 */
  descriptions?: string[]
  degree?: number
}

export type GraphNeighbor = {
  id: string
  entity_type?: string
  relation_type?: string
  relation_types?: string[]
  description?: string
  descriptions?: string[]
  /** out = 该实体指向邻居；in = 邻居指向该实体 */
  direction?: 'out' | 'in' | string
}

export type GraphRelation = {
  source: string
  target: string
  direction?: 'out' | 'in' | string
  relation_type?: string
  relation_types?: string[]
  description?: string
  descriptions?: string[]
}

export type GraphEvidence = {
  chunk_id: string
  knowledge_id?: string
  title?: string
  snippet?: string
  chunk_metadata?: unknown
}

export type GraphEntityDetail = {
  available?: boolean
  reason?: string
  workspace?: string
  entity?: GraphEntityRef
  neighbors?: GraphNeighbor[]
  neighbor_total?: number
  evidence?: GraphEvidence[]
  evidence_total?: number
  chunk_total?: number
  dropped_chunks?: number
  truncated?: boolean
}

export type GraphEdgeDetail = {
  available?: boolean
  reason?: string
  workspace?: string
  source?: string
  target?: string
  relations?: GraphRelation[]
  evidence?: GraphEvidence[]
  evidence_total?: number
  chunk_total?: number
  dropped_chunks?: number
  truncated?: boolean
}

/** 图谱规模口径：shown ≤ total，供「显示 300 / 共 5000」提示用。 */
export type GraphScale = {
  shownNodes: number
  shownEdges: number
  totalNodes: number
  totalEdges: number
  truncated: boolean
}

/**
 * 归一化图谱规模。
 *
 * 缺 total_* 时退化为「已展示即全部」：旧数据面（M5-1 的 /graph/view）没有这两个
 * 字段，若按 0 处理会把「共 0 个节点」显示在明明有节点的图上。
 */
export function graphScale(view: GraphViewPayload | null | undefined): GraphScale {
  const shownNodes = view?.nodes?.length ?? 0
  const shownEdges = view?.edges?.length ?? 0
  const totalNodes = typeof view?.total_nodes === 'number' ? view.total_nodes : shownNodes
  const totalEdges = typeof view?.total_edges === 'number' ? view.total_edges : shownEdges
  return {
    shownNodes,
    shownEdges,
    totalNodes: Math.max(totalNodes, shownNodes),
    totalEdges: Math.max(totalEdges, shownEdges),
    truncated: Boolean(view?.truncated) || totalNodes > shownNodes,
  }
}

/** API 响应可能是 {success,data} 信封，也可能已经是载荷本身。 */
export function unwrapGraphPayload<T>(res: unknown): T | null {
  if (!res || typeof res !== 'object') return null
  const body = res as { data?: unknown; available?: unknown }
  if (body.data && typeof body.data === 'object') return body.data as T
  return body as T
}

/**
 * 证据条目 → ProvenancePanel 输入。
 *
 * 面板按 `knowledge_title` / `chunk_metadata` 取 L2/L3/L4，字段名与下钻接口不同
 * （接口叫 title），这里做映射；chunk_id 同时作为 input.id 供面板去重与高亮。
 */
export function evidenceToProvenanceInput(evidence: GraphEvidence): ProvenanceInput {
  return {
    id: evidence.chunk_id,
    knowledge_id: evidence.knowledge_id || '',
    knowledge_title: evidence.title || '',
    content: evidence.snippet || '',
    chunk_metadata: evidence.chunk_metadata ?? null,
  }
}

/** 邻居列表归一化：过滤空 id、去重，并按关系类型稳定排序（同类型聚在一起更好读）。 */
export function neighborRows(detail: GraphEntityDetail | null | undefined): GraphNeighbor[] {
  const seen = new Set<string>()
  const rows: GraphNeighbor[] = []
  for (const n of detail?.neighbors || []) {
    if (!n?.id || seen.has(n.id)) continue
    seen.add(n.id)
    rows.push(n)
  }
  return rows.sort((a, b) => {
    const ra = a.relation_type || ''
    const rb = b.relation_type || ''
    if (ra !== rb) return ra < rb ? -1 : 1
    return a.id < b.id ? -1 : a.id > b.id ? 1 : 0
  })
}

/**
 * 邻居数是否有截断。后端用 `neighbor_ids >= max_neighbors` 判定，因此
 * neighbor_total 是「去重后的邻居数」而不是行数——两者不一致时不报截断。
 */
export function neighborsTruncated(detail: GraphEntityDetail | null | undefined): boolean {
  const rows = detail?.neighbors?.length ?? 0
  const total = detail?.neighbor_total
  return typeof total === 'number' && total > rows
}

/** LightRAG 多值字段分隔符（thirdparty/lightrag/lightrag/constants.py）。 */
const GRAPH_FIELD_SEP = '<SEP>'

/**
 * 取实体的描述列表。
 *
 * LightRAG 把同一实体的多段描述用 `<SEP>` 合并成一个字符串（operate.py:2486），
 * 原样渲染用户会看到「描述甲<SEP>描述乙」。新数据面给了 `descriptions`；
 * 这里对旧数据面（只有合并串）也做一次拆解兜底——直接显示 `<SEP>` 是最糟的结果。
 */
export function descriptionsOf(
  entity: { description?: string; descriptions?: string[] } | null | undefined,
): string[] {
  const list = entity?.descriptions
  if (Array.isArray(list) && list.length) {
    return list.map((d) => String(d).trim()).filter(Boolean)
  }
  const raw = entity?.description
  if (!raw) return []
  return raw
    .split(GRAPH_FIELD_SEP)
    .map((d) => d.trim())
    .filter(Boolean)
}

/** 关系列表归一化：合并同一条关系的多值字段，去掉完全重复的条目。 */
export function relationRows(detail: GraphEdgeDetail | null | undefined): GraphRelation[] {
  const rows: GraphRelation[] = []
  const seen = new Set<string>()
  for (const r of detail?.relations || []) {
    const types = descriptionsOf({ description: r.relation_type, descriptions: r.relation_types })
    const descs = descriptionsOf(r)
    const key = `${r.source}\u0000${r.target}\u0000${types.join('\u0000')}\u0000${descs.join('\u0000')}`
    if (seen.has(key)) continue
    seen.add(key)
    rows.push({ ...r, relation_type: types.join('、'), descriptions: descs })
  }
  return rows
}

/**
 * `?edge=` 的编码：两段各自 encodeURIComponent 后用 `|` 连接。
 *
 * 实体名是 LLM 抽取的自由文本，可能含 `|`；先编码再拼接才能保证分隔符无歧义
 * （名字里的 `|` 会变成 %7C，不会被误当分隔符）。
 */
export function edgeParam(source: string, target: string): string {
  return `${encodeURIComponent(source)}|${encodeURIComponent(target)}`
}

/** 解析 `?edge=`；形状不合法返回 null（外部传入的 URL 不可信）。 */
export function parseEdgeParam(value: unknown): { source: string; target: string } | null {
  if (typeof value !== 'string') return null
  const parts = value.split('|')
  if (parts.length !== 2) return null
  try {
    const source = decodeURIComponent(parts[0]).trim()
    const target = decodeURIComponent(parts[1]).trim()
    if (!source || !target || source === target) return null
    return { source, target }
  } catch {
    // decodeURIComponent 对畸形百分号编码会抛错；外部 URL 不能让它冒到渲染层
    return null
  }
}

/**
 * 按文档过滤子图（WS1.1b：文档列表徽标 → 图谱页聚焦）。
 *
 * 依据：图谱节点/边的 source_id 是 LightRAG chunk key（`{docID}-chunk-NNN`，
 * `<SEP>` 连接），key 前段就是 WeKnora knowledge doc id（A0 口径），所以
 * 「与该文档相关的实体」可以纯前端判定，不需要新后端接口。
 *
 * 返回过滤后的 nodes/edges 以及命中数；docId 为空返回原数据（matched=-1 表示
 * 未启用过滤，与「过滤后 0 个」区分开）。
 */
export function filterGraphByDoc(
  nodes: GraphNodeDatum[],
  edges: GraphEdgeDatum[],
  docId: string,
): { nodes: GraphNodeDatum[]; edges: GraphEdgeDatum[]; matched: number } {
  const id = String(docId || '').trim()
  if (!id) return { nodes, edges, matched: -1 }
  const marker = `${id}-chunk-`
  const kept = nodes.filter((n) => String(n.source_id || '').includes(marker))
  const keptIds = new Set(kept.map((n) => n.id))
  const keptEdges = edges.filter((e) => keptIds.has(e.source) && keptIds.has(e.target))
  return { nodes: kept, edges: keptEdges, matched: kept.length }
}

/**
 * 当前子图的实体类型分布（P0-4：类型图例过滤条）。
 * 只统计展示中的节点——图例数字随 ego/搜索/文档过滤联动，所见即所滤。
 */
export function typeCounts(nodes: GraphNodeDatum[]): { type: string; count: number }[] {
  const counts = new Map<string, number>()
  for (const n of nodes || []) {
    const t = n?.entity_type || UNTYPED
    counts.set(t, (counts.get(t) || 0) + 1)
  }
  return [...counts.entries()]
    .map(([type, count]) => ({ type, count }))
    .sort((a, b) => b.count - a.count || (a.type < b.type ? -1 : 1))
}

/** ego 视图的状态摘要（状态条文案的数据形状；center 为空 = overview）。 */
export function egoSummary(view: GraphViewPayload | null | undefined): {
  isEgo: boolean
  center: string
  depth: number
  visible: boolean
} {
  const isEgo = view?.mode === 'ego' && !!view?.center
  return {
    isEgo,
    center: isEgo ? String(view?.center || '') : '',
    depth: typeof view?.depth === 'number' ? view.depth : 1,
    visible: isEgo,
  }
}

