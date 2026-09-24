import assert from 'node:assert/strict'
import test from 'node:test'
import {
  descriptionsOf,
  edgeParam,
  evidenceToProvenanceInput,
  filterGraphByDoc,
  graphScale,
  neighborRows,
  neighborsTruncated,
  parseEdgeParam,
  relationRows,
  unwrapGraphPayload,
} from './graphExplorer.ts'

test('graphScale reports truncation when the backend truncated the view', () => {
  const s = graphScale({ nodes: [{ id: 'a' }, { id: 'b' }], edges: [], total_nodes: 500, total_edges: 1200, truncated: true })
  assert.deepEqual(s, {
    shownNodes: 2, shownEdges: 0, totalNodes: 500, totalEdges: 1200, truncated: true,
  })
})

test('graphScale falls back to shown counts when totals are absent', () => {
  // M5-1 的 /graph/view 没有 total_*；按 0 处理会在有节点的图上显示「共 0 个」
  const s = graphScale({ nodes: [{ id: 'a' }], edges: [{ source: 'a', target: 'a' }] })
  assert.equal(s.totalNodes, 1)
  assert.equal(s.totalEdges, 1)
  assert.equal(s.truncated, false)
})

test('graphScale clamps a wrong small total but does not cry truncation', () => {
  // total < shown 只说明后端数据不自洽，不能据此报「已截断」——那会在完整图上
  // 弹出「仅显示前 N 个」的假提示。
  const s = graphScale({ nodes: [{ id: 'a' }, { id: 'b' }], total_nodes: 1 })
  assert.equal(s.totalNodes, 2)
  assert.equal(s.truncated, false)
})

test('graphScale tolerates a null payload', () => {
  assert.deepEqual(graphScale(null), {
    shownNodes: 0, shownEdges: 0, totalNodes: 0, totalEdges: 0, truncated: false,
  })
})

test('unwrapGraphPayload accepts both the enveloped and the bare shape', () => {
  const payload = { nodes: [{ id: 'a' }] }
  assert.deepEqual(unwrapGraphPayload({ success: true, data: payload }), payload)
  assert.deepEqual(unwrapGraphPayload(payload), payload)
  assert.equal(unwrapGraphPayload(null), null)
})

test('evidenceToProvenanceInput maps the field names the panel actually reads', () => {
  const input = evidenceToProvenanceInput({
    chunk_id: 'chunk-1',
    knowledge_id: 'doc-a',
    title: '宁德时代年报',
    snippet: '摘录',
    chunk_metadata: { sbk_blocks: [{ block_id: 'p001-b002' }] },
  })
  // 面板读 knowledge_title / chunk_metadata；字段名对不上会静默显示空态
  assert.equal(input.id, 'chunk-1')
  assert.equal(input.knowledge_title, '宁德时代年报')
  assert.deepEqual(input.chunk_metadata, { sbk_blocks: [{ block_id: 'p001-b002' }] })
})

test('evidenceToProvenanceInput normalises missing optional fields', () => {
  const input = evidenceToProvenanceInput({ chunk_id: 'chunk-1' })
  assert.equal(input.knowledge_id, '')
  assert.equal(input.knowledge_title, '')
  assert.equal(input.chunk_metadata, null)
})

test('neighborRows dedupes, drops blanks and groups by relation type', () => {
  const rows = neighborRows({
    neighbors: [
      { id: 'b', relation_type: '供应商' },
      { id: 'a', relation_type: '创始人' },
      { id: 'b', relation_type: '供应商' },
      { id: '' },
    ],
  })
  // 排序按码位（供 U+4F9B < 创 U+521B）。用 localeCompare 会得到更「好看」的拼音序，
  // 但它随运行环境的 ICU 数据变化——同样的数据在浏览器与 CI 里可能不同序，
  // 而这里要的只是「同类型相邻 + 稳定」。
  assert.deepEqual(rows.map((r) => r.id), ['b', 'a'])
  assert.equal(rows[0].relation_type, '供应商')
})

test('neighborsTruncated only fires when the backend dropped neighbours', () => {
  assert.equal(neighborsTruncated({ neighbors: [{ id: 'a' }], neighbor_total: 1 }), false)
  assert.equal(neighborsTruncated({ neighbors: [{ id: 'a' }], neighbor_total: 9 }), true)
  // 后端没给 total 时不误报
  assert.equal(neighborsTruncated({ neighbors: [{ id: 'a' }] }), false)
})

test('descriptionsOf prefers the structured list and never leaks <SEP>', () => {
  assert.deepEqual(
    descriptionsOf({ descriptions: ['甲', '乙'], description: '旧数据面合并串' }),
    ['甲', '乙'],
  )
  // 旧数据面只有合并串：直接渲染会让用户看到「甲<SEP>乙」
  assert.deepEqual(descriptionsOf({ description: '甲<SEP>乙' }), ['甲', '乙'])
  assert.deepEqual(descriptionsOf({ description: '' }), [])
  assert.deepEqual(descriptionsOf(null), [])
})

test('relationRows flattens merged fields and dedupes identical relations', () => {
  const rows = relationRows({
    relations: [
      { source: 'a', target: 'b', relation_type: '创始人<SEP>股东', description: '甲<SEP>乙' },
      { source: 'a', target: 'b', relation_type: '创始人<SEP>股东', description: '甲<SEP>乙' },
      { source: 'a', target: 'b', relation_type: '供应商', description: '供货' },
    ],
  })
  assert.equal(rows.length, 2)
  assert.equal(rows[0].relation_type, '创始人、股东')
  assert.deepEqual(rows[0].descriptions, ['甲', '乙'])
  assert.equal(rows[1].relation_type, '供应商')
})

test('edge round-trips through the URL even when names contain the separator', () => {
  const param = edgeParam('曾毓群', '宁德时代')
  assert.deepEqual(parseEdgeParam(param), { source: '曾毓群', target: '宁德时代' })
  // 名字里带 | 不行——先编码再拼接才能保证分隔符无歧义
  const tricky = edgeParam('a|b', 'c|d')
  assert.notEqual(tricky.split('|').length, 3)
  assert.deepEqual(parseEdgeParam(tricky), { source: 'a|b', target: 'c|d' })
})

test('parseEdgeParam rejects malformed external input', () => {
  assert.equal(parseEdgeParam('no-separator'), null)
  assert.equal(parseEdgeParam('a|a'), null, '自环没有意义')
  assert.equal(parseEdgeParam('a||b'), null)
  assert.equal(parseEdgeParam(undefined), null)
  assert.equal(parseEdgeParam('%zz|%zz'), null, '畸形百分号编码不能把异常冒到渲染层')
})

const DOC_NODES = [
  { id: '宁德时代', source_id: 'doc-a-chunk-000<SEP>doc-b-chunk-001' },
  { id: '曾毓群', source_id: 'doc-a-chunk-002' },
  { id: '无关实体', source_id: 'doc-c-chunk-000' },
  { id: '无来源', source_id: '' },
]
const DOC_EDGES = [
  { source: '宁德时代', target: '曾毓群' },
  { source: '宁德时代', target: '无关实体' },
]

test('filterGraphByDoc keeps only entities evidenced by the document', () => {
  const out = filterGraphByDoc(DOC_NODES, DOC_EDGES, 'doc-a')
  // 多值 source_id 里含该文档 chunk 的也算命中；跨文档实体被排除
  assert.deepEqual(out.nodes.map((n) => n.id), ['宁德时代', '曾毓群'])
  // 只保留两端都保留的边，悬空边会渲染成断线
  assert.deepEqual(out.edges, [{ source: '宁德时代', target: '曾毓群' }])
  assert.equal(out.matched, 2)
})

test('filterGraphByDoc disabled with empty doc id', () => {
  const out = filterGraphByDoc(DOC_NODES, DOC_EDGES, '  ')
  assert.equal(out.nodes, DOC_NODES, '原样返回，不做浅拷贝')
  assert.equal(out.matched, -1, '与「过滤后 0 个」区分')
})

test('filterGraphByDoc reports zero matches explicitly', () => {
  const out = filterGraphByDoc(DOC_NODES, DOC_EDGES, 'doc-zzz')
  assert.deepEqual(out.nodes, [])
  assert.deepEqual(out.edges, [])
  assert.equal(out.matched, 0, '0 个是真实结果，前端要据此提示「该文档尚未进图谱」')
})
