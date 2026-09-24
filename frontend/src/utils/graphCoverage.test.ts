import assert from 'node:assert/strict'
import test from 'node:test'
import { coverageSummary } from './graphCoverage.ts'

test('coverageSummary computes percent from ready over eligible', () => {
  const s = coverageSummary({
    available: true, total_docs: 15, exempt_manual: 3, eligible: 12,
    coverage: { ready: 6, pending: 2, building: 1, failed: 3 },
  })
  assert.equal(s.percent, 50)
  assert.equal(s.eligible, 12, '分母剔除粘贴类豁免（D3 口径）')
  assert.equal(s.uncovered, 6)
})

test('coverageSummary falls back to tracked sum when eligible is absent', () => {
  // 老数据面没有 eligible 字段
  const s = coverageSummary({ coverage: { ready: 2, pending: 1, failed: 1 } })
  assert.equal(s.eligible, 4)
  assert.equal(s.percent, 50)
})

test('coverageSummary never exceeds 100 when the denominator is distorted', () => {
  const s = coverageSummary({ eligible: 5, coverage: { ready: 9 } })
  assert.equal(s.eligible, 9, '分母失真时以 ready 为准')
  assert.equal(s.percent, 100)
})

test('coverageSummary reports zero explicitly and guards negative counts', () => {
  const s = coverageSummary({ available: true, eligible: 4, coverage: { ready: 0, failed: -3 } })
  assert.equal(s.percent, 0)
  assert.equal(s.failed, 0)
  assert.equal(s.uncovered, 4)
})

test('coverageSummary handles an empty KB and a missing payload', () => {
  const empty = coverageSummary({ available: true, total_docs: 0, eligible: 0, coverage: {} })
  assert.equal(empty.percent, null, '无可用文档时不渲染进度条')
  assert.equal(empty.uncovered, 0)
  assert.equal(coverageSummary(null).percent, null)
  assert.equal(coverageSummary(null).uncovered, 0)
})
