/**
 * M5-1 / M6-1 · KB 知识图谱力导图的**纯逻辑层**（与 echarts 渲染解耦）。
 *
 * 抽出来的理由有二：
 * 1. 本仓库没有组件挂载测试工具（`@vue/test-utils`/jsdom 未安装），组件的可测部分
 *    只有这些纯函数——配色、分类归并、option 组装，正是最容易出错的部分；
 * 2. M6-1 WS1.2 的独立图谱浏览器页要复用同一套映射规则，不应把逻辑埋在 SFC 里。
 */
import type { EChartsOption } from 'echarts'

export interface GraphNodeDatum {
  id: string
  entity_type?: string
  degree?: number
  description?: string
  source_id?: string
  /** 抽取强度（LightRAG 边专属；节点当前无此字段，预留）。 */
  weight?: number | null
  created_at?: string
  /** P2-14：该实体的证据指向用户对话常引用的文档（familiar，渲染为描边）。 */
  familiar?: boolean
}

export interface GraphEdgeDatum {
  source: string
  target: string
  relation_type?: string
  description?: string
  /** 关系强度：LightRAG 把同一条关系的多次抽取 weight 累加进 properties。 */
  weight?: number | null
  keywords?: string
}

export type GraphLayout = 'force' | 'circular'

/** 无 entity_type 的节点归入该分类。 */
export const UNTYPED = '未分类'

/** 兜底色：仅用于「未分类」。已知/未知的具名类型一律走调色板或哈希取色。 */
export const FALLBACK_COLOR = '#8ca3b8'

/** 已知实体类型的默认配色（沿用 M5-1 既有视觉）。 */
export const DEFAULT_PALETTE: Record<string, string> = {
  公司: '#4f7cf0', 人物: '#e0666c', 产品: '#48b884', 行业: '#f0a04f',
  概念: '#9b6ff0', 事件: '#f0cf4f', 政策: '#5fc9e0', 指标: '#e0919b',
}

/**
 * 按类型名哈希取稳定色。
 *
 * 此前类型色是写死的 8 个中文键，`entity_type` 一旦是「组织」「Company」等就统一
 * 掉到灰色兜底——图上看不出类型差异。哈希取色保证：同一类型恒得同色，新增类型
 * 不必改代码。
 */
export function hashColor(key: string): string {
  let h = 0
  for (let i = 0; i < key.length; i++) h = (h * 31 + key.charCodeAt(i)) % 360
  return `hsl(${h}, 55%, 58%)`
}

/** 配色优先级：宿主注入 palette > 内置默认 > 哈希 > 未分类兜底。 */
export function colorFor(type: string, palette: Record<string, string> = {}): string {
  if (palette[type]) return palette[type]
  if (DEFAULT_PALETTE[type]) return DEFAULT_PALETTE[type]
  return type === UNTYPED ? FALLBACK_COLOR : hashColor(type)
}

/** 节点分类清单（去重 + 排序，未分类统一归到 UNTYPED）。 */
export function graphCategories(nodes: GraphNodeDatum[] | null | undefined): string[] {
  const set = new Set<string>()
  for (const n of nodes || []) set.add(n?.entity_type || UNTYPED)
  return [...set].sort()
}

export function nodeType(node: GraphNodeDatum | null | undefined): string {
  return node?.entity_type || UNTYPED
}

/** 节点半径随度数增长，封顶 40，避免 hub 节点盖住整张图。 */
export function symbolSizeFor(degree: number | undefined): number {
  return Math.min(10 + (degree || 0) * 1.5, 40)
}

/**
 * 边的强度视觉编码（P0-5）：weight 归一到 [0,1]，映射为线宽 1→3、透明度 0.2→0.75。
 * weight 缺省（旧数据面）退化为统一细线。同一次渲染内取最大 weight 做归一基准，
 * 避免绝对值差异悬殊时全部贴地。
 */
export function edgeStyleFor(
  weight: number | null | undefined,
  maxWeight: number,
): { width: number; opacity: number } {
  if (!weight || weight <= 0 || !maxWeight || maxWeight <= 0) {
    return { width: 1, opacity: 0.3 }
  }
  const norm = Math.min(weight / maxWeight, 1)
  return { width: 1 + norm * 2, opacity: 0.2 + norm * 0.55 }
}

/** 边的稳定键。同一对节点间可能有同向重复边，取首个即可。 */
export function toEdgeKey(source: string, target: string): string {
  return `${source}\u0000${target}`
}

export interface GraphOptionInput {
  nodes: GraphNodeDatum[]
  edges: GraphEdgeDatum[]
  layout?: GraphLayout
  palette?: Record<string, string>
  showLegend?: boolean
  /** 高亮节点 id；命中则其余节点降透明度（WS1.3 检索联动用）。 */
  highlightId?: string
}

/**
 * 组装 echarts option。
 *
 * 与旧内联实现的差异：
 * - 用 `categories` + `legend`（旧实现给节点塞了 `category` 却没有 categories 数组，
 *   图例无从渲染）；
 * - 颜色改哈希/调色板（旧实现写死 8 个中文类型名）；
 * - tooltip 带类型与描述；边 tooltip 带关系类型（旧实现没有 tooltip）。
 */
export function buildGraphOption(input: GraphOptionInput): EChartsOption {
  const { nodes, edges, layout = 'force', palette = {}, showLegend = true } = input
  const isForce = layout === 'force'
  const categories = graphCategories(nodes)

  const data = nodes.map((n) => ({
    id: n.id,
    name: n.id,
    category: nodeType(n),
    symbolSize: symbolSizeFor(n.degree),
    // P2-14：familiar 描边（金色环）——该实体的证据来自用户对话常引用的文档，
    // 与 Wiki 图谱「熟悉环」同语义的个人化导航信号。
    ...(n.familiar
      ? { itemStyle: { borderColor: '#f0aF4f', borderWidth: 3 } }
      : {}),
    ...(input.highlightId && input.highlightId !== n.id
      ? { itemStyle: { ...(n.familiar ? { borderColor: '#f0aF4f', borderWidth: 3 } : {}), opacity: 0.35 } }
      : {}),
  }))

  const maxWeight = edges.reduce((m, e) => Math.max(m, e.weight || 0), 0)
  const links = edges.map((e) => {
    const style = edgeStyleFor(e.weight, maxWeight)
    return {
      source: e.source,
      target: e.target,
      lineStyle: { width: style.width, color: 'source' as const, opacity: style.opacity },
    }
  })

  const nodeById = new Map(nodes.map((n) => [n.id, n]))
  const edgeByKey = new Map(edges.map((e) => [toEdgeKey(e.source, e.target), e]))

  return {
    tooltip: {
      confine: true,
      // 长描述自动换行限宽 + 字号比正文小一号（默认 nowrap/14px 会溢出屏幕）
      extraCssText: 'max-width: 380px; white-space: normal; word-break: break-word; ' +
        'font-size: var(--app-text-sm, 13px); line-height: 1.6; padding: 10px 12px;',
      formatter: (p: any) => {
        if (p?.dataType === 'edge') {
          const e = edgeByKey.get(toEdgeKey(p.data?.source, p.data?.target))
          const rel = e?.relation_type ? `<b>${e.relation_type}</b><br/>` : ''
          const kw = e?.keywords ? `<span style="color:#999">${e.keywords}</span><br/>` : ''
          return `${rel}${kw}${e?.description || ''}`
        }
        const n = nodeById.get(p?.data?.name)
        const type = n?.entity_type ? `[${n.entity_type}] ` : ''
        const desc = n?.description ? `<br/>${n.description}` : ''
        return `${type}<b>${p?.data?.name || ''}</b>${desc}`
      },
    },
    legend: showLegend ? [{ type: 'scroll', top: 0, data: categories }] : undefined,
    series: [{
      type: 'graph',
      layout,
      roam: true,
      draggable: true,
      categories: categories.map((name) => ({ name, itemStyle: { color: colorFor(name, palette) } })),
      data,
      links,
      force: isForce ? { repulsion: 120, edgeLength: [40, 120], gravity: 0.1 } : undefined,
      circular: isForce ? undefined : { rotateLabel: true },
      label: { show: true, fontSize: 10, position: 'right' },
      emphasis: { focus: 'adjacency', lineStyle: { width: 3 } },
      lineStyle: { curveness: isForce ? 0.1 : 0 },
    }],
  }
}
