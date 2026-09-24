// StarKB 五级溯源（WS2 前端最小可用版）：从消息引用 / 检索结果项自带的
// chunk metadata 中提取 L1–L4 溯源字段并归一化。
//
// 数据契约（后端 SearchResult JSON 序列化，见 internal/types/search.go）：
// - `metadata`（map<string,string>，文档/源级）：reliability（1–5）、knowledge_channel 等
// - `chunk_metadata`（JSON 对象，chunk 级）：sbk_blocks（区块锚点数组）、
//   sbk_pages（命中页码数组）、sbk_method（对齐方法 exact/fuzzy/failed/…）
//
// 后端契约归一层未写 sbk_* 字段时，这里全部返回空值，由面板渲染空态；
// 本模块不做任何网络请求，也不负责写入。

export type ProvenanceInput = {
  id?: string
  content?: string
  knowledge_id?: string
  knowledge_title?: string
  knowledge_filename?: string
  knowledge_base_id?: string
  knowledge_channel?: string
  chunk_type?: string
  metadata?: Record<string, unknown> | null
  chunk_metadata?: unknown
}

export type ProvenanceDocument = {
  id: string
  title: string
  fileName?: string
  knowledgeBaseId: string
}

export type ProvenanceChunk = {
  id: string
  content: string
  /** sbk_blocks 命中的区块锚点（L4 指针） */
  blockIds: string[]
  /** 对齐方法：exact / fuzzy / failed / … */
  method: string
  /** M6-2：sbk_sentences 句级指针（L5，正文句切分 + 字符偏移）。缺失 = 数据面未升级。 */
  sentences: ProvenanceSentence[]
}

/** L5 句级指针（docs/04 §4）：offset 相对 chunk.content 原文坐标系。 */
export type ProvenanceSentence = {
  text: string
  start: number
  end: number
  /** 句起点之前最近的内联契约锚点；空串 = 无前置锚点，消费方以 sbk_blocks 主块兜底 */
  blockId: string
}

export type ProvenanceModel = {
  /** L1 信息源渠道（web/api/wechat/…，可能为空） */
  channel: string
  /** L1 源可靠度 1–5；未注册时为 null */
  reliability: number | null
  /** L2 源文档 */
  document: ProvenanceDocument
  /** L3 命中页码（去重升序） */
  pages: number[]
  /** L4 chunk 块锚点信息 */
  chunk: ProvenanceChunk
}

/** 解析 chunk_metadata：后端为 JSON 对象；防御历史链路上可能出现的 JSON 字符串。 */
export function parseChunkMetadata(value: unknown): Record<string, unknown> | null {
  if (!value) return null
  if (typeof value === 'string') {
    const raw = value.trim()
    if (!raw) return null
    try {
      const parsed: unknown = JSON.parse(raw)
      return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
        ? (parsed as Record<string, unknown>)
        : null
    } catch {
      return null
    }
  }
  if (typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, unknown>
  }
  return null
}

/** chunk 级溯源字段是否随本次结果下发（sbk_* 任一存在即算有）。 */
export function hasChunkProvenance(meta: Record<string, unknown> | null): boolean {
  if (!meta) return false
  return hasField(meta, 'sbk_blocks') || hasField(meta, 'sbk_pages') || hasField(meta, 'sbk_method')
}

function hasField(meta: Record<string, unknown>, key: string): boolean {
  const value = meta[key]
  if (value === undefined || value === null || value === '') return false
  if (Array.isArray(value)) return value.length > 0
  return true
}

/** 归一化 sbk_blocks：字符串数组（数字/杂项转字符串，过滤空值）。 */
function normalizeBlockIds(value: unknown): string[] {
  if (value === undefined || value === null) return []
  const rawList = Array.isArray(value) ? value : [value]
  const out: string[] = []
  for (const item of rawList) {
    if (item === undefined || item === null) continue
    const text = String(item).trim()
    if (text) out.push(text)
  }
  return out
}

/** 归一化 sbk_pages：接受数字与数字字符串，去重升序。 */
function normalizePages(value: unknown): number[] {
  if (value === undefined || value === null) return []
  const rawList = Array.isArray(value) ? value : [value]
  const seen = new Set<number>()
  for (const item of rawList) {
    let page: number | null = null
    if (typeof item === 'number' && Number.isFinite(item)) {
      page = Math.trunc(item)
    } else if (typeof item === 'string' && item.trim()) {
      const parsed = Number.parseInt(item.trim(), 10)
      if (Number.isFinite(parsed)) page = parsed
    }
    if (page !== null && page > 0) seen.add(page)
  }
  return Array.from(seen).sort((a, b) => a - b)
}

/**
 * 归一化 sbk_sentences（L5，M6-2）：[{text,start,end,block_id}] → ProvenanceSentence[]。
 * 防御历史/异常数据：非正区间、end≤start、空文本的条目丢弃，不做截断——
 * 偏移指针的价值就在于精确，模糊修补只会把错误藏进高亮里。
 */
function normalizeSentences(value: unknown): ProvenanceSentence[] {
  if (!Array.isArray(value)) return []
  const out: ProvenanceSentence[] = []
  for (const item of value) {
    if (!item || typeof item !== 'object') continue
    const text = typeof item.text === 'string' ? item.text.trim() : ''
    const start = typeof item.start === 'number' ? Math.trunc(item.start) : Number.NaN
    const end = typeof item.end === 'number' ? Math.trunc(item.end) : Number.NaN
    if (!text || !Number.isFinite(start) || !Number.isFinite(end)) continue
    if (start < 0 || end <= start) continue
    out.push({
      text,
      start,
      end,
      blockId: typeof item.block_id === 'string' ? item.block_id.trim() : '',
    })
  }
  return out
}

/** 归一化 sbk_method：任意标量转小写字符串，缺失返回空串。 */
function normalizeMethod(value: unknown): string {  if (value === undefined || value === null) return ''
  if (typeof value === 'string') return value.trim().toLowerCase()
  return ''
}

/** 解析可靠度 1–5；非法值返回 null（面板显示“未注册”）。 */
function normalizeReliability(value: unknown): number | null {
  let parsed: number | null = null
  if (typeof value === 'number' && Number.isFinite(value)) {
    parsed = Math.trunc(value)
  } else if (typeof value === 'string' && value.trim()) {
    const candidate = Number.parseInt(value.trim(), 10)
    if (Number.isFinite(candidate)) parsed = candidate
  }
  if (parsed === null || parsed < 1 || parsed > 5) return null
  return parsed
}

function firstString(...values: Array<unknown>): string {
  for (const value of values) {
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

/**
 * 从若干条原始引用/检索结果中提取溯源模型。
 * 返回 null 表示没有任何溯源数据随结果下发（面板渲染空态）。
 * 多条输入（同一文档的多个 chunk）时页码取并集，chunk 段取首条带锚点的记录。
 */
export function extractProvenance(
  inputs: ProvenanceInput[] | null | undefined,
): ProvenanceModel | null {
  const list = Array.isArray(inputs) ? inputs.filter(Boolean) : []
  if (!list.length) return null

  let reliability: number | null = null
  let channel = ''
  let document: ProvenanceDocument | null = null
  const pages = new Set<number>()
  let chunk: ProvenanceChunk | null = null
  let method = ''

  for (const input of list) {
    const meta = input.metadata && typeof input.metadata === 'object' ? input.metadata : null
    const chunkMeta = parseChunkMetadata(input.chunk_metadata)

    if (reliability === null && meta) {
      reliability = normalizeReliability(meta.reliability)
    }
    if (!channel) {
      // 顶层 knowledge_channel 优先，其次退回 metadata 中的同名键
      channel = firstString(
        input.knowledge_channel,
        meta ? (meta.knowledge_channel as string | undefined) : undefined,
        meta ? (meta.channel as string | undefined) : undefined,
        meta ? (meta.source_channel as string | undefined) : undefined,
      )
    }

    if (!document) {
      const id = firstString(input.knowledge_id)
      const title = firstString(input.knowledge_title, input.knowledge_filename, input.knowledge_id)
      if (id || title) {
        document = {
          id,
          title,
          fileName: firstString(input.knowledge_filename) || undefined,
          knowledgeBaseId: firstString(input.knowledge_base_id),
        }
      }
    }

    if (!input.knowledge_channel) {
      // top-level channel 字段优先于 metadata 中的同名键
      channel = firstString(input.knowledge_channel) || channel
    } else {
      channel = input.knowledge_channel.trim()
    }

    if (chunkMeta && hasChunkProvenance(chunkMeta)) {
      const blockIds = normalizeBlockIds(chunkMeta.sbk_blocks)
      for (const page of normalizePages(chunkMeta.sbk_pages)) pages.add(page)
      method = normalizeMethod(chunkMeta.sbk_method) || method
      if (!chunk && (blockIds.length || method)) {
        chunk = {
          id: firstString(input.id),
          content: typeof input.content === 'string' ? input.content : '',
          blockIds,
          method: normalizeMethod(chunkMeta.sbk_method),
          sentences: normalizeSentences(chunkMeta.sbk_sentences),
        }
      }
    }
  }

  if (!chunk && !method && pages.size === 0 && reliability === null) {
    // chunk 级与源级溯源字段均未随下发
    return null
  }

  return {
    channel,
    reliability,
    document:
      document ||
      { id: '', title: '', fileName: undefined, knowledgeBaseId: '' },
    pages: Array.from(pages).sort((a, b) => a - b),
    chunk: chunk || { id: '', content: '', blockIds: [], method, sentences: [] },
  }
}
