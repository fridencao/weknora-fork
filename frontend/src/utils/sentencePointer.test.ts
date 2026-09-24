import assert from 'node:assert/strict'
import test from 'node:test'
import {
  findRangeInText,
  resolveClaimSentence,
  stripAnchorsWithMap,
  verifySpan,
} from './sentencePointer.ts'
import type { ProvenanceSentence } from './provenance.ts'

const sent = (text: string, start: number, end: number, blockId = 'p001-b001'): ProvenanceSentence =>
  ({ text, start, end, blockId })

const CONTENT = '宁德时代发布2024年年报。<!--sbk:p001-b002-->营收为3620亿元，同比增长9.7%。'

test('stripAnchorsWithMap removes anchors and keeps an old→new coordinate map', () => {
  const { text, map } = stripAnchorsWithMap(CONTENT)
  assert.equal(text, '宁德时代发布2024年年报。营收为3620亿元，同比增长9.7%。')
  // 平移正确性：原坐标里的字符经映射后在新文本中是同一个字符
  const rawIdx = CONTENT.indexOf('3620')
  assert.equal(text[map[rawIdx]], '3')
  // 锚点区间映射为 -1
  assert.ok(map.slice(14, 20).every((v) => v === -1))
})

test('resolveClaimSentence prefers exact, then containment, and rejects fuzzy similarity', () => {
  const sentences = [
    sent('营收为3620亿元。', 10, 18),
    sent('同比增长9.7%。', 18, 26),
  ]
  assert.equal(resolveClaimSentence(sentences, '营收为3620亿元。')?.tier, 'exact')
  // 论断是句子的改写片段（最常见形态）：不带句末标点
  assert.equal(
    resolveClaimSentence(sentences, '营收为3620亿元')?.tier,
    'sentence_contains_claim',
  )
  // 二元组会给出高分：「营收增长」与两句都相似——但没有包含关系，必须拒绝
  assert.equal(resolveClaimSentence(sentences, '营收增长'), null)
  assert.equal(resolveClaimSentence([], 'x'), null)
})

test('resolveClaimSentence breaks ties toward the longer (more specific) sentence', () => {
  const sentences = [
    sent('营收增长。', 0, 5),
    sent('公司营收实现同比增长。', 0, 11),
  ]
  const r = resolveClaimSentence(sentences, '营收实现同比增长')
  assert.equal(r?.sentence.text, '公司营收实现同比增长。')
})

test('verifySpan translates through anchors and validates the slice text', () => {
  // 原文坐标：`宁德时代发布2024年年报。` 占 0–13，锚点 14–33，句从 34 到文末 53
  const s = sent('营收为3620亿元，同比增长9.7%。', 34, 53)
  const v = verifySpan(CONTENT, s)
  assert.equal(v.ok, true)
  if (v.ok) {
    const { text } = stripAnchorsWithMap(CONTENT)
    assert.equal(text.slice(v.start, v.end), '营收为3620亿元，同比增长9.7%。')
  }
})

test('verifySpan fails closed on drift and on out-of-range spans', () => {
  // 内容已漂移（数字被改）：这就是 text_hash 校验要拦的形态
  assert.equal(verifySpan('营收为9999亿元。', sent('营收为3620亿元', 0, 10)).ok, false)
  assert.equal(verifySpan(CONTENT, sent('x', 9999, 10000)).ok, false)
  const v = verifySpan('营收为9999亿元。', sent('营收为3620亿元', 0, 10))
  assert.equal(v.ok ? null : v.reason, 'text_mismatch')
})

test('findRangeInText is whitespace-insensitive and maps back to raw offsets', () => {
  const haystack = '营收为 3620 亿元，\n同比增长 9.7%。'
  const r = findRangeInText(haystack, '3620亿元')
  assert.ok(r)
  assert.equal(haystack.slice(r.start, r.end).replace(/\s+/g, ''), '3620亿元')
  assert.equal(findRangeInText(haystack, '不存在的内容'), null)
})
