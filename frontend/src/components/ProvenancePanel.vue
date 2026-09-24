<template>
  <t-drawer
    v-model:visible="visible"
    class="provenance-panel"
    placement="right"
    :size="420"
    :z-index="1600"
    attach="body"
    :footer="false"
    :close-on-overlay-click="true"
    :close-on-esc-keydown="true"
    :destroy-on-close="false"
    :header="panelTitle"
    @close="close"
  >
    <div v-if="!model" class="provenance-panel__empty">
      <t-icon name="info-circle" size="var(--app-icon-xl)" />
      <p>{{ t('chat.provenance.empty') }}</p>
    </div>

    <div v-else class="provenance-panel__body">
      <!-- L1 信息源 -->
      <section class="provenance-panel__section">
        <h4 class="provenance-panel__level">
          <span class="provenance-panel__badge provenance-panel__badge--l1">L1</span>
          {{ t('chat.provenance.source') }}
        </h4>
        <dl class="provenance-panel__fields">
          <div class="provenance-panel__field">
            <dt>{{ t('chat.provenance.channel') }}</dt>
            <dd>
              <t-tag v-if="model.channel" variant="outline" size="small">{{ model.channel }}</t-tag>
              <span v-else class="provenance-panel__missing">{{ t('chat.provenance.missing') }}</span>
            </dd>
          </div>
          <div class="provenance-panel__field">
            <dt>{{ t('chat.provenance.reliability') }}</dt>
            <dd>
              <span v-if="model.reliability !== null" class="provenance-panel__stars" role="img"
                :aria-label="t('chat.provenance.reliabilityAria', { level: model.reliability })">
                <t-icon v-for="i in 5" :key="i" name="star-filled" size="var(--app-icon-sm)"
                  :class="{ 'is-dim': i > (model?.reliability ?? 0) }" />
              </span>
              <span v-else class="provenance-panel__missing">{{ t('chat.provenance.unregistered') }}</span>
            </dd>
          </div>
        </dl>
      </section>

      <!-- L2 源文档 -->
      <section class="provenance-panel__section">
        <h4 class="provenance-panel__level">
          <span class="provenance-panel__badge provenance-panel__badge--l2">L2</span>
          {{ t('chat.provenance.document') }}
        </h4>
        <dl class="provenance-panel__fields">
          <div class="provenance-panel__field">
            <dt>{{ t('chat.provenance.documentTitle') }}</dt>
            <dd class="provenance-panel__text">{{ model.document.title || t('chat.provenance.missing') }}</dd>
          </div>
          <div v-if="model.document.id" class="provenance-panel__field">
            <dt>{{ t('chat.provenance.documentId') }}</dt>
            <dd class="provenance-panel__mono">{{ model.document.id }}</dd>
          </div>
          <div v-if="model.document.knowledgeBaseId" class="provenance-panel__field">
            <dt>{{ t('chat.provenance.knowledgeBase') }}</dt>
            <dd class="provenance-panel__mono">{{ model.document.knowledgeBaseId }}</dd>
          </div>
        </dl>
        <a
          v-if="model.document.knowledgeBaseId"
          class="provenance-panel__jump"
          :href="documentHref"
          target="_blank"
          rel="noopener noreferrer"
          @click.stop
        >
          {{ t('chat.provenance.openDocument') }}
          <t-icon name="jump" size="var(--app-icon-sm)" />
        </a>
      </section>

      <!-- L3 章节位置 -->
      <section class="provenance-panel__section">
        <h4 class="provenance-panel__level">
          <span class="provenance-panel__badge provenance-panel__badge--l3">L3</span>
          {{ t('chat.provenance.location') }}
        </h4>
        <div v-if="model.pages.length" class="provenance-panel__pages">
          <t-tag v-for="page in model.pages" :key="page" variant="light" size="small">
            {{ t('chat.provenance.pageItem', { page }) }}
          </t-tag>
        </div>
        <p v-else class="provenance-panel__missing">{{ t('chat.provenance.noPages') }}</p>
      </section>

      <!-- L4 chunk 锚点 -->
      <section class="provenance-panel__section">
        <h4 class="provenance-panel__level">
          <span class="provenance-panel__badge provenance-panel__badge--l4">L4</span>
          {{ t('chat.provenance.chunk') }}
        </h4>
        <dl class="provenance-panel__fields">
          <div class="provenance-panel__field">
            <dt>{{ t('chat.provenance.anchorCount') }}</dt>
            <dd>
              <span v-if="model.chunk.blockIds.length" class="provenance-panel__strong">
                {{ t('chat.provenance.anchorCountValue', { count: model.chunk.blockIds.length }) }}
              </span>
              <span v-else class="provenance-panel__missing">{{ t('chat.provenance.noAnchors') }}</span>
            </dd>
          </div>
          <div class="provenance-panel__field">
            <dt>{{ t('chat.provenance.method') }}</dt>
            <dd>
              <t-tag v-if="model.chunk.method" :theme="methodTheme" variant="light" size="small">
                {{ methodLabel }}
              </t-tag>
              <span v-else class="provenance-panel__missing">{{ t('chat.provenance.missing') }}</span>
            </dd>
          </div>
          <div v-if="model.chunk.id" class="provenance-panel__field">
            <dt>{{ t('chat.provenance.chunkId') }}</dt>
            <dd class="provenance-panel__mono">{{ model.chunk.id }}</dd>
          </div>
        </dl>
        <div v-if="model.chunk.content" class="provenance-panel__excerpt">
          <span class="provenance-panel__excerpt-label">
            {{ t('chat.provenance.excerpt') }}
            <!-- M6-2：粒度明示——句级确定性定位成功，或降级为块级 -->
            <t-tag v-if="granularity" size="small" variant="light"
              :theme="granularity === 'sentence' ? 'success' : 'warning'">
              {{ granularity === 'sentence'
                ? t('chat.provenance.granularitySentence')
                : t('chat.provenance.granularityBlock') }}
            </t-tag>
          </span>
          <div ref="excerptBody" class="provenance-panel__excerpt-body" v-html="excerptHtml"></div>
        </div>
      </section>

      <!-- L5 表格行列：需生成期论断指针绑定（chat_pipeline 插件），数据面就绪前不做假高亮 -->
      <p class="provenance-panel__note">{{ t('chat.provenance.l5Pending') }}</p>
    </div>
  </t-drawer>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { marked } from 'marked'
import { useProvenancePanel } from '@/composables/useProvenancePanel'
import { extractProvenance, type ProvenanceModel } from '@/utils/provenance'
import { findRangeInText, resolveClaimSentence, verifySpan } from '@/utils/sentencePointer'
import { configureMarkedForChatMarkdown } from '@/utils/chatMarkdownRenderer'
import { sanitizeMarkdownHTML } from '@/utils/security'

const { t } = useI18n()
const router = useRouter()
const panel = useProvenancePanel()

const visible = computed({
  get: () => panel?.visible.value ?? false,
  set: (value: boolean) => {
    if (!value) close()
  },
})

const model = computed<ProvenanceModel | null>(() =>
  panel ? extractProvenance(panel.inputs.value) : null,
)

const claimContext = computed(() => panel?.context.value || '')

const panelTitle = computed(() =>
  panel?.title.value || t('chat.provenance.title'),
)

// 摘录按 markdown 预览渲染（与引用悬浮卡同一管线），展示父 chunk 全文——
// 旧版开头截 320 字常常只剩相邻章节，与问句「对不上」。
// 渲染完成后用角标所在句子（claimContext）做二元组相似度定位，滚动 + 高亮相关段。
const excerptBody = ref<HTMLElement | null>(null)

const bigrams = (s: string): Set<string> => {
  const t = (s || '').replace(/\s+/g, '')
  const out = new Set<string>()
  for (let i = 0; i < t.length - 1; i++) out.add(t.slice(i, i + 2))
  return out
}

const excerptHtml = computed(() => {
  const raw = model.value?.chunk.content || ''
  const cleaned = raw.replace(/<!--[\s\S]*?(?:-->|$)/g, '')
  if (!cleaned.trim()) return ''
  try {
    configureMarkedForChatMarkdown()
    return sanitizeMarkdownHTML(marked.parse(cleaned, { breaks: true, async: false }) as string)
  } catch {
    return ''
  }
})

const locateRelevantBlock = async () => {
  await nextTick()
  const body = excerptBody.value
  if (!body) return
  body.querySelectorAll('.provenance-panel__hit').forEach((el) => {
    el.classList.remove('provenance-panel__hit')
  })
  const context = claimContext.value
  const ctxGrams = bigrams(context)
  if (!ctxGrams.size) return
  let best: Element | null = null
  let bestScore = 0
  body.querySelectorAll('p, li, h1, h2, h3, h4, h5, h6').forEach((block) => {
    const grams = bigrams(block.textContent || '')
    if (!grams.size) return
    let hit = 0
    grams.forEach((g) => {
      if (ctxGrams.has(g)) hit++
    })
    const score = hit / Math.sqrt(grams.size)
    if (score > bestScore) {
      bestScore = score
      best = block
    }
  })
  if (!best || bestScore < 0.5) return
  const el = best as HTMLElement
  el.classList.add('provenance-panel__hit')
  body.scrollTop = Math.max(0, (el as HTMLElement).offsetTop - body.clientHeight / 3)
}

// ---- M6-2 · L5 句级定位（确定性指针，优先于上面的块级模糊兜底） ----
const granularity = ref<'' | 'sentence' | 'block'>('')

/** 清掉上一轮的高亮（块级 class + 句级 mark）。 */
const clearHighlights = (body: HTMLElement) => {
  body.querySelectorAll('.provenance-panel__hit').forEach((el) => {
    el.classList.remove('provenance-panel__hit')
  })
  body.querySelectorAll('mark.provenance-panel__sentence').forEach((mark) => {
    const parent = mark.parentNode
    if (!parent) return
    while (mark.firstChild) parent.insertBefore(mark.firstChild, mark)
    parent.removeChild(mark)
    parent.normalize()
  })
}

/**
 * 句级高亮：L5 指针匹配（确定性）→ span 校验（text_hash 角色，防内容漂移）→
 * DOM 文本节点游标放置（空白不敏感子串定位，markdown 渲染不改字符序列）。
 * 任何一层失败都返回 false，由块级路径兜底。
 */
const highlightSentence = (body: HTMLElement): boolean => {
  const sentences = model.value?.chunk.sentences || []
  const raw = model.value?.chunk.content || ''
  const claim = claimContext.value
  if (!sentences.length || !claim || !raw) return false
  const resolved = resolveClaimSentence(sentences, claim)
  if (!resolved) return false
  const verified = verifySpan(raw, resolved.sentence)
  if (!verified.ok) return false

  const walker = document.createTreeWalker(body, NodeFilter.SHOW_TEXT)
  const nodes: { node: Text; start: number; end: number }[] = []
  let total = 0
  let current: Node | null
  while ((current = walker.nextNode())) {
    const textNode = current as Text
    const len = textNode.textContent?.length || 0
    nodes.push({ node: textNode, start: total, end: total + len })
    total += len
  }
  const haystack = nodes.map((x) => x.node.textContent).join('')
  const range = findRangeInText(haystack, resolved.sentence.text)
  if (!range) return false

  const marks: HTMLElement[] = []
  for (const item of nodes) {
    if (item.end <= range.start || item.start >= range.end) continue
    const len = item.node.textContent?.length || 0
    const localStart = Math.max(range.start - item.start, 0)
    const localEnd = Math.min(range.end - item.start, len)
    if (localEnd <= localStart) continue
    const mark = document.createElement('mark')
    mark.className = 'provenance-panel__sentence'
    const tail = item.node.splitText(localEnd)
    mark.appendChild(item.node.splitText(localStart))
    item.node.parentNode?.insertBefore(mark, tail)
    marks.push(mark)
  }
  if (!marks.length) return false
  body.scrollTop = Math.max(0, marks[0].offsetTop - body.clientHeight / 3)
  return true
}

const locateClaim = async () => {
  await nextTick()
  const body = excerptBody.value
  if (!body) return
  clearHighlights(body)
  granularity.value = ''
  if (!claimContext.value) return
  if (highlightSentence(body)) {
    granularity.value = 'sentence'
    return
  }
  // 降级：块级模糊定位（粒度标签如实标注，不假装句级）
  granularity.value = 'block'
  locateRelevantBlock()
}

watch([excerptHtml, claimContext], locateClaim)

const methodLabel = computed(() => {
  const method = model.value?.chunk.method || ''
  const key = `chat.provenance.method_${method}`
  const translated = t(key)
  // 未登记的对齐方法直接展示原值
  return translated === key ? method : translated
})

const methodTheme = computed(() => {
  switch (model.value?.chunk.method) {
    case 'exact':
      return 'success'
    case 'fuzzy':
      return 'warning'
    case 'failed':
      return 'danger'
    default:
      return 'default'
  }
})

const documentHref = computed(() => {
  const knowledgeBaseId = model.value?.document.knowledgeBaseId
  if (!knowledgeBaseId) return ''
  const query: Record<string, string> = {}
  const knowledgeId = model.value?.document.id
  if (knowledgeId) query.knowledge_id = knowledgeId
  return router.resolve({
    path: `/platform/knowledge-bases/${knowledgeBaseId}`,
    query,
  }).href
})

function close() {
  panel?.close()
}
</script>

<style scoped lang="less">
.provenance-panel__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 48px 16px;
  color: var(--td-text-color-placeholder);
  text-align: center;

  p {
    margin: 0;
    font-size: var(--app-text-md);
    line-height: 1.6;
  }
}

.provenance-panel__body {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 2px 2px 16px;
}

.provenance-panel__section {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md);
  padding: 12px 14px;
  background: var(--td-bg-color-container);
}

.provenance-panel__level {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 10px;
  font-size: var(--app-text-base);
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.provenance-panel__badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 26px;
  height: 18px;
  padding: 0 5px;
  border-radius: var(--app-radius-xs);
  font-size: var(--app-text-xs);
  font-weight: 600;
  color: var(--td-brand-color);
  background: color-mix(in srgb, var(--td-brand-color) 10%, transparent);
}

.provenance-panel__fields {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
}

.provenance-panel__field {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;

  dt {
    flex-shrink: 0;
    min-width: 72px;
    font-size: var(--app-text-sm);
    color: var(--td-text-color-placeholder);
  }

  dd {
    flex: 1;
    min-width: 0;
    margin: 0;
    font-size: var(--app-text-sm);
    color: var(--td-text-color-primary);
    word-break: break-all;
  }
}

.provenance-panel__text {
  white-space: pre-wrap;
}

.provenance-panel__mono {
  font-family: var(--app-font-mono, monospace);
  font-size: var(--app-text-xs);
  color: var(--td-text-color-secondary);
  word-break: break-all;
}

.provenance-panel__missing {
  color: var(--td-text-color-placeholder);
}

.provenance-panel__strong {
  font-weight: 600;
}

.provenance-panel__stars {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  color: var(--td-warning-color);

  .is-dim {
    color: var(--td-bg-color-component-disabled);
  }
}

.provenance-panel__jump {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-top: 10px;
  font-size: var(--app-text-sm);
  color: var(--td-brand-color);
  text-decoration: none;

  &:hover {
    text-decoration: underline;
  }
}

.provenance-panel__pages {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.provenance-panel__excerpt {
  margin-top: 10px;

  .provenance-panel__excerpt-label {
    display: block;
    font-size: var(--app-text-sm);
    color: var(--td-text-color-placeholder);
    margin-bottom: 4px;
  }

  .provenance-panel__excerpt-body {
    margin: 0;
    padding: 8px 10px;
    border-radius: var(--app-radius-xs);
    background: var(--td-bg-color-secondarycontainer);
    font-size: var(--app-text-sm);
    line-height: 1.6;
    color: var(--td-text-color-secondary);
    word-break: break-word;
    max-height: 180px;
    overflow-y: auto;

    // markdown 预览模式：块级元素自然流式排布 + 标题压扁为加粗正文
    :deep(p),
    :deep(h1),
    :deep(h2),
    :deep(h3),
    :deep(h4),
    :deep(h5),
    :deep(h6),
    :deep(ul),
    :deep(ol) {
      margin: 0 0 0.375em;
      font-size: inherit;
      white-space: normal;
    }

    :deep(h1),
    :deep(h2),
    :deep(h3),
    :deep(h4),
    :deep(h5),
    :deep(h6) {
      font-weight: 600;
      color: var(--td-text-color-primary);
    }

    :deep(*:first-child) {
      margin-top: 0;
    }

    :deep(*:last-child) {
      margin-bottom: 0;
    }

    :deep(.provenance-panel__hit) {
      background: var(--td-brand-color-1);
      border-radius: var(--app-radius-xs);
      box-shadow: 0 0 0 3px var(--td-brand-color-1);
      color: var(--td-text-color-primary);
    }
  }
}

.provenance-panel__note {
  margin: 0;
  font-size: var(--app-text-xs);
  color: var(--td-text-color-placeholder);
  text-align: center;
}
</style>

<style lang="less">
/* 非 scoped：句级高亮的 <mark> 是运行时 DOM 注入的，带不上 scoped 属性选择器 */
.provenance-panel__excerpt-body mark.provenance-panel__sentence {
  background: var(--td-warning-color-light);
  color: inherit;
  border-radius: 2px;
  padding: 0 1px;
}
</style>
