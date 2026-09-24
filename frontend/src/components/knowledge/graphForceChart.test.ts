import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildGraphOption,
  colorFor,
  graphCategories,
  hashColor,
  symbolSizeFor,
  toEdgeKey,
  DEFAULT_PALETTE,
  FALLBACK_COLOR,
  UNTYPED,
} from './graphForceChart.ts'

const NODES = [
  { id: '宁德时代', entity_type: '公司', degree: 30 },
  { id: '曾毓群', entity_type: '人物', degree: 2 },
  { id: '组织架构', entity_type: '组织', degree: 1 },
  { id: '无类型实体' },
]

const EDGES = [
  { source: '曾毓群', target: '宁德时代', relation_type: '创始人', description: '创始人关系' },
  { source: '组织架构', target: '宁德时代' },
]

test('hashColor is stable per type and differs across types', () => {
  assert.equal(hashColor('组织'), hashColor('组织'))
  assert.notEqual(hashColor('组织'), hashColor('Company'))
  assert.match(hashColor('组织'), /^hsl\(\d{1,3}, 55%, 58%\)$/)
})

test('known types keep the built-in palette, unknown types get a stable hash colour', () => {
  assert.equal(colorFor('公司'), DEFAULT_PALETTE['公司'])
  // 旧实现里「组织」会掉到灰色兜底，图上看不出类型差异
  assert.equal(colorFor('组织'), hashColor('组织'))
  assert.notEqual(colorFor('组织'), FALLBACK_COLOR)
})

test('an injected palette wins over the built-in one', () => {
  assert.equal(colorFor('公司', { 公司: '#123456' }), '#123456')
  assert.equal(colorFor('组织', { 组织: '#abcdef' }), '#abcdef')
})

test('only the untyped bucket falls back to grey', () => {
  assert.equal(colorFor(UNTYPED), FALLBACK_COLOR)
})

test('graphCategories dedupes, sorts and buckets untyped nodes', () => {
  assert.deepEqual(graphCategories(NODES), ['人物', '公司', '组织', UNTYPED].sort())
  assert.deepEqual(graphCategories([]), [])
  assert.deepEqual(graphCategories(null), [])
})

test('symbolSizeFor grows with degree and is capped', () => {
  assert.equal(symbolSizeFor(undefined), 10)
  assert.equal(symbolSizeFor(0), 10)
  assert.equal(symbolSizeFor(4), 16)
  assert.equal(symbolSizeFor(1000), 40)
})

test('toEdgeKey distinguishes direction and cannot collide on 0x00-free ids', () => {
  assert.notEqual(toEdgeKey('a', 'b'), toEdgeKey('b', 'a'))
  assert.equal(toEdgeKey('a', 'bc'), toEdgeKey('a', 'bc'))
})

test('buildGraphOption emits a legend backed by real categories', () => {
  const option = buildGraphOption({ nodes: NODES, edges: EDGES })
  // 旧实现给节点塞了 category 却没有 categories 数组，图例无从渲染
  const series = (option.series as any[])[0]
  assert.deepEqual(
    series.categories.map((c: any) => c.name),
    graphCategories(NODES),
  )
  assert.deepEqual(
    (option.legend as any[])[0].data,
    graphCategories(NODES),
  )
  assert.deepEqual(series.data.map((d: any) => d.category), ['公司', '人物', '组织', UNTYPED])
})

test('buildGraphOption maps degree to symbol size and keeps every node', () => {
  const series = (buildGraphOption({ nodes: NODES, edges: EDGES }).series as any[])[0]
  assert.equal(series.data.length, 4)
  assert.equal(series.data[0].symbolSize, symbolSizeFor(30))
  assert.equal(series.data[3].symbolSize, 10)
})

test('buildGraphOption maps edges to source/target links', () => {
  const series = (buildGraphOption({ nodes: NODES, edges: EDGES }).series as any[])[0]
  assert.deepEqual(
    series.links.map((l: any) => [l.source, l.target]),
    [['曾毓群', '宁德时代'], ['组织架构', '宁德时代']],
  )
})

test('buildGraphOption can drop the legend for compact hosts', () => {
  const option = buildGraphOption({ nodes: NODES, edges: EDGES, showLegend: false })
  assert.equal(option.legend, undefined)
})

test('circular layout swaps force for circular and straightens edges', () => {
  const series = (buildGraphOption({ nodes: NODES, edges: EDGES, layout: 'circular' }).series as any[])[0]
  assert.equal(series.layout, 'circular')
  assert.equal(series.force, undefined)
  assert.equal(series.circular.rotateLabel, true)
  assert.equal(series.lineStyle.curveness, 0)
})

test('highlightId dims the other nodes so the linked entity stands out', () => {
  const series = (buildGraphOption({ nodes: NODES, edges: EDGES, highlightId: '宁德时代' }).series as any[])[0]
  const byName = new Map(series.data.map((d: any) => [d.name, d]))
  assert.equal((byName.get('宁德时代') as any).itemStyle, undefined)
  assert.equal((byName.get('曾毓群') as any).itemStyle.opacity, 0.35)
})

test('a highlightId that is not in the graph dims nothing into invisibility', () => {
  const series = (buildGraphOption({ nodes: NODES, edges: EDGES, highlightId: '不存在' }).series as any[])[0]
  assert.equal(series.data.every((d: any) => d.itemStyle?.opacity === 0.35), true)
})

test('tooltip resolves node and edge payloads by id rather than trusting the label', () => {
  const option = buildGraphOption({ nodes: NODES, edges: EDGES })
  const fmt = (option.tooltip as any).formatter
  assert.equal(fmt({ dataType: 'node', data: { name: '曾毓群' } }), '[人物] <b>曾毓群</b>')
  assert.equal(
    fmt({ dataType: 'edge', data: { source: '曾毓群', target: '宁德时代' } }),
    '<b>创始人</b><br/>创始人关系',
  )
})
