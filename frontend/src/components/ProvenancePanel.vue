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
      <t-icon name="info-circle" size="28px" />
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
                <t-icon v-for="i in 5" :key="i" name="star-filled" size="14px"
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
          <t-icon name="jump" size="14px" />
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
          <span class="provenance-panel__excerpt-label">{{ t('chat.provenance.excerpt') }}</span>
          <div class="provenance-panel__excerpt-body" v-html="excerptHtml"></div>
        </div>
      </section>

      <!-- L5 表格行列暂不在 WS2 范围 -->
      <p class="provenance-panel__note">{{ t('chat.provenance.l5Pending') }}</p>
    </div>
  </t-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { marked } from 'marked'
import { useProvenancePanel } from '@/composables/useProvenancePanel'
import { extractProvenance, type ProvenanceModel } from '@/utils/provenance'
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

const panelTitle = computed(() =>
  panel?.title.value || t('chat.provenance.title'),
)

// 摘录按 markdown 预览渲染（与引用悬浮卡同一管线）；先剥离 sbk 溯源注释，
// 避免 320 字符截断切在注释中间时残留「<!--sbk:xxx」碎片
const excerptHtml = computed(() => {
  const raw = model.value?.chunk.content || ''
  const truncated = raw.length > 320 ? `${raw.slice(0, 320)}…` : raw
  const cleaned = truncated.replace(/<!--[\s\S]*?(?:-->|$)/g, '')
  if (!cleaned.trim()) return ''
  try {
    configureMarkedForChatMarkdown()
    return sanitizeMarkdownHTML(marked.parse(cleaned, { breaks: true, async: false }) as string)
  } catch {
    return ''
  }
})

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
  }
}

.provenance-panel__note {
  margin: 0;
  font-size: var(--app-text-xs);
  color: var(--td-text-color-placeholder);
  text-align: center;
}
</style>
