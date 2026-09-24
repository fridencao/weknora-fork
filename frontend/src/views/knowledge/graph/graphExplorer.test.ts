import assert from 'node:assert/strict'
import test from 'node:test'
import {
  evidenceToProvenanceInput,
  graphScale,
  neighborRows,
  neighborsTruncated,
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
