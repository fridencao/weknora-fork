// M6-2 · L5 句级指针的消费逻辑（docs/04 §4，替换 CJK 二元组模糊定位）。
//
// 分层（每层可独立失败并降级）：
//   1. resolveClaimSentence —— 论断句 ↔ 证据句的确定性匹配（归一化后包含关系，
//      拒绝模糊相似度）；匹配不到 → 调用方退回块级。
//   2. verifySpan —— text_hash 校验角色的落实：span 平移到去锚点坐标系后，
//      切片必须与句文本一致；不一致 = 内容已漂移 → 块级降级并明示粒度。
//   3. findRangeInText —— DOM 放置用的空白不敏感子串定位（markdown 渲染会改变
//      空白，但不能改变字符序列）。

import type { ProvenanceSentence } from './provenance'

const ANCHOR_RE = /<!--[\s\S]*?-->/g

/**
 * 去掉契约锚点，并给出「原坐标 → 新坐标」映射（句级 span 是原文坐标系，
 * 摘录渲染的是去锚点文本，span 必须经它平移）。锚点字符的映射值为 -1。
 */
export function stripAnchorsWithMap(raw: string): { text: string; map: number[] } {
  const chars: string[] = []
  const map: number[] = new Array(raw.length).fill(-1)
  let last = 0
  for (const m of raw.matchAll(ANCHOR_RE)) {
    const start = m.index ?? 0
    for (let i = last; i < start; i++) {
      chars.push(raw[i])
      map[i] = chars.length - 1
    }
    last = start + m[0].length
  }
  for (let i = last; i < raw.length; i++) {
    chars.push(raw[i])
    map[i] = chars.length - 1
  }
  return { text: chars.join(''), map }
}

/** 匹配归一：去空白 + 小写。保留标点——句边界本身就是定位信号的一部分。 */
function normalizeLoose(s: string): string {
  return s.replace(/\s+/g, '').toLowerCase()
}

export type ClaimMatchTier = 'exact' | 'sentence_contains_claim' | 'claim_contains_sentence'

export type ResolvedClaimSentence = {
  sentence: ProvenanceSentence
  tier: ClaimMatchTier
}

/**
 * 论断句 → 证据句的确定性匹配。
 *
 * 三档（得分即置信序）：全等 > 句子包含论断（论断是句子的改写片段，最常见）>
 * 论断包含句子。无包含关系的候选一律拒绝——这就是与二元组匹配的本质区别：
 * 「相似」不再产生高亮，只有「可解释的包含关系」才产生。
 * 平局取更长的句子（更具体的定位）。
 */
export function resolveClaimSentence(
  sentences: ProvenanceSentence[],
  claim: string,
): ResolvedClaimSentence | null {
  const claimNorm = normalizeLoose(claim)
  if (!claimNorm || !sentences.length) return null
  let best: { sentence: ProvenanceSentence; tier: ClaimMatchTier; score: number } | null = null
  for (const sentence of sentences) {
    const sentNorm = normalizeLoose(sentence.text)
    if (!sentNorm) continue
    let tier: ClaimMatchTier | null = null
    let score = 0
    if (sentNorm === claimNorm) {
      tier = 'exact'
      score = 3
    } else if (sentNorm.includes(claimNorm)) {
      tier = 'sentence_contains_claim'
      score = 2
    } else if (claimNorm.includes(sentNorm)) {
      tier = 'claim_contains_sentence'
      score = 1
    }
    if (!tier) continue
    if (!best || score > best.score ||
        (score === best.score && sentNorm.length > normalizeLoose(best.sentence.text).length)) {
      best = { sentence, tier, score }
    }
  }
  return best ? { sentence: best.sentence, tier: best.tier } : null
}

export type SpanVerification =
  | { ok: true; start: number; end: number }
  | { ok: false; reason: 'out_of_range' | 'text_mismatch' }

/**
 * span 校验 + 坐标平移（text_hash 的落实，且比哈希更强：直接比对归一化全文）。
 * 返回的 start/end 已平移到**去锚点**坐标系（面板摘录渲染的就是去锚点文本）。
 */
export function verifySpan(
  rawContent: string,
  sentence: ProvenanceSentence,
): SpanVerification {
  const { text: cleaned, map } = stripAnchorsWithMap(rawContent)
  if (sentence.start < 0 || sentence.end > rawContent.length || sentence.start >= sentence.end) {
    return { ok: false, reason: 'out_of_range' }
  }
  const start = map[sentence.start]
  const end = map[sentence.end - 1] + 1
  if (start < 0 || end <= start) {
    return { ok: false, reason: 'out_of_range' }
  }
  const slice = cleaned.slice(start, end)
  if (normalizeLoose(slice) !== normalizeLoose(sentence.text)) {
    return { ok: false, reason: 'text_mismatch' }
  }
  return { ok: true, start, end }
}

export type TextRange = { start: number; end: number }

/**
 * 空白不敏感的子串定位：在 haystack 里找 needle（双方都去空白 + 小写），
 * 返回 haystack 原坐标区间。markdown 渲染会改空白与换行，但不能改变字符顺序，
 * 所以 DOM 放置用这个而不是 span 偏移。
 */
export function findRangeInText(haystack: string, needle: string): TextRange | null {
  const needleNorm = normalizeLoose(needle)
  if (!needleNorm) return null
  const norm: string[] = []
  const map: number[] = []
  for (let i = 0; i < haystack.length; i++) {
    const ch = haystack[i]
    if (/\s/.test(ch)) continue
    norm.push(ch.toLowerCase())
    map.push(i)
  }
  const idx = norm.join('').indexOf(needleNorm)
  if (idx < 0) return null
  const start = map[idx]
  const end = map[Math.min(idx + needleNorm.length - 1, map.length - 1)] + 1
  return { start, end }
}
