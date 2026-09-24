import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./index.vue', import.meta.url), 'utf8')

// Slice the whole reference-hydration block: the retry scheduler and the
// provenance predicate sit above hydrateMessageReferences.
function hydrationBlock() {
  const start = source.indexOf('const HYDRATE_MAX_ATTEMPTS')
  const end = source.indexOf('const loadFollowUpSuggestions = async', start)
  assert.notEqual(start, -1)
  assert.notEqual(end, -1)
  return source.slice(start, end)
}

// 流式 references 事件在 Redis 往返后曾丢失 chunk_metadata.sbk_*，面板因此空态；
// 水合是兜底路径，必须能等到服务端权威引用落库，而不是一次拉取失败就放弃。
test('reference hydration retries until server-side provenance lands', () => {
  const block = hydrationBlock()
  assert.match(block, /const HYDRATE_MAX_ATTEMPTS = 3/)
  assert.match(block, /const scheduleHydrateRetry = /)
  assert.match(block, /scheduleHydrateRetry\(message, sid, attempt\)/)
  assert.match(block, /attempt \+ 1/)
  // Guard against navigating away mid-retry.
  assert.match(block, /session_id\.value === sid\) void hydrateMessageReferences\(message, attempt \+ 1\)/)
})

test('reference hydration matches the local row by persisted or request id', () => {
  const block = hydrationBlock()
  assert.match(block, /m\.id === mid \|\| m\.request_id === mid \|\| m\.assistant_message_id === mid/)
})

test('reference hydration keeps thin references but keeps retrying for provenance', () => {
  const block = hydrationBlock()
  assert.match(block, /const hasUsableProvenance = \(refs\)/)
  assert.match(block, /meta\.sbk_blocks \|\| meta\.sbk_pages \|\| meta\.sbk_method/)
  assert.match(block, /if \(refs\.length\) \{\s*message\.knowledge_references = refs/)
  assert.match(block, /if \(!hasUsableProvenance\(refs\)\) \{\s*scheduleHydrateRetry/)
})
