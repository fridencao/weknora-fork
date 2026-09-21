import assert from 'node:assert/strict'
import test from 'node:test'

import {
  extractProvenance,
  hasChunkProvenance,
  parseChunkMetadata,
  type ProvenanceInput,
} from './provenance.ts'

const provenanceInput = (overrides: Partial<ProvenanceInput> = {}): ProvenanceInput => ({
  id: 'chunk-1',
  content: '2024 年营收增长 12%',
  knowledge_id: 'doc-1',
  knowledge_title: '年报.pdf',
  knowledge_filename: '年报.pdf',
  knowledge_base_id: 'kb-1',
  knowledge_channel: 'web',
  metadata: { reliability: '4' },
  chunk_metadata: {
    sbk_blocks: ['blk-a', 'blk-b'],
    sbk_pages: [3, 1, 3],
    sbk_method: 'exact',
  },
  ...overrides,
})

test('extractProvenance normalizes the full L1-L4 model from one reference', () => {
  const model = extractProvenance([provenanceInput()])
  assert.ok(model)
  assert.equal(model.channel, 'web')
  assert.equal(model.reliability, 4)
  assert.equal(model.document.id, 'doc-1')
  assert.equal(model.document.title, '年报.pdf')
  assert.deepEqual(model.pages, [1, 3])
  assert.deepEqual(model.chunk.blockIds, ['blk-a', 'blk-b'])
  assert.equal(model.chunk.method, 'exact')
})

test('extractProvenance unions pages across merged chunks and keeps first anchored chunk', () => {
  const first = provenanceInput()
  const second = provenanceInput({
    id: 'chunk-2',
    chunk_metadata: { sbk_blocks: ['blk-c'], sbk_pages: [5], sbk_method: 'fuzzy' },
  })
  const model = extractProvenance([first, second])
  assert.ok(model)
  assert.deepEqual(model.pages, [1, 3, 5])
  assert.equal(model.chunk.id, 'chunk-1')
  assert.equal(model.chunk.method, 'exact')
})

test('extractProvenance tolerates string-encoded chunk_metadata and numeric pages', () => {
  const model = extractProvenance([
    provenanceInput({
      metadata: { reliability: '9' },
      chunk_metadata: JSON.stringify({ sbk_blocks: [7], sbk_pages: ['2'], sbk_method: 'FAILED' }),
    }),
  ])
  assert.ok(model)
  assert.equal(model.reliability, null)
  assert.deepEqual(model.chunk.blockIds, ['7'])
  assert.deepEqual(model.pages, [2])
  assert.equal(model.chunk.method, 'failed')
})

test('extractProvenance returns null when no provenance fields came down', () => {
  const bare: ProvenanceInput = {
    id: 'chunk-1',
    content: 'text',
    knowledge_id: 'doc-1',
    knowledge_title: 'legacy.pdf',
  }
  assert.equal(extractProvenance([bare]), null)
  assert.equal(extractProvenance([]), null)
  assert.equal(extractProvenance(null), null)
})

test('extractProvenance still surfaces L1 reliability without chunk anchors', () => {
  const model = extractProvenance([provenanceInput({ chunk_metadata: {} })])
  assert.ok(model)
  assert.equal(model.reliability, 4)
  assert.deepEqual(model.chunk.blockIds, [])
  assert.equal(model.chunk.method, '')
})

test('parseChunkMetadata rejects malformed payloads', () => {
  assert.equal(parseChunkMetadata('{not json'), null)
  assert.equal(parseChunkMetadata('[1,2]'), null)
  assert.equal(parseChunkMetadata(''), null)
  assert.equal(parseChunkMetadata(undefined), null)
  assert.deepEqual(parseChunkMetadata('{"sbk_method":"exact"}'), { sbk_method: 'exact' })
})

test('hasChunkProvenance detects any sbk field', () => {
  assert.equal(hasChunkProvenance({ sbk_blocks: [] }), false)
  assert.equal(hasChunkProvenance({ sbk_method: 'exact' }), true)
  assert.equal(hasChunkProvenance({ sbk_pages: [1] }), true)
  assert.equal(hasChunkProvenance(null), false)
})
