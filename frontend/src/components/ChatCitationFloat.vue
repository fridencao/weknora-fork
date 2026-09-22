<template>
  <Teleport to="body">
    <div v-if="float.visible" class="chat-citation-float" :style="{ top: `${float.top}px`, left: `${float.left}px` }"
      @mouseenter="onEnter?.()" @mouseleave="onLeave?.()">
      <template v-if="float.type === 'web'">
        <div class="chat-citation-float__title">{{ float.title || float.url }}</div>
        <a v-if="float.url" class="chat-citation-float__link" :href="float.url" target="_blank"
          rel="noopener noreferrer">{{ float.url }}</a>
      </template>
      <template v-else>
        <div class="chat-citation-float__title">{{ float.title }}</div>
        <div v-if="float.loading" class="chat-citation-float__muted">{{ loadingText }}</div>
        <div v-else-if="float.error" class="chat-citation-float__error">{{ float.error }}</div>
        <!-- 预览模式：chunk 原文按 markdown 渲染（标题/列表/代码块生效，
             sbk 溯源注释等 HTML comment 渲染后自然不可见），消毒后再注入 -->
        <div
          v-else
          ref="bodyRef"
          class="chat-citation-float__body chat-citation-float__body--md"
          v-html="renderedContent"
        ></div>
      </template>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { marked } from 'marked'
import { useI18n } from 'vue-i18n'
import type { CitationFloatState } from '@/composables/useChatCitationPopover'
import { configureMarkedForChatMarkdown } from '@/utils/chatMarkdownRenderer'
import { sanitizeMarkdownHTML } from '@/utils/security'

const props = defineProps<{
  float: CitationFloatState
  onEnter?: () => void
  onLeave?: () => void
}>()

const { t } = useI18n()
const loadingText = t('common.loading')

const renderedContent = computed(() => {
  const raw = props.float.content || ''
  if (!raw.trim()) return ''
  try {
    configureMarkedForChatMarkdown()
    return sanitizeMarkdownHTML(marked.parse(raw, { breaks: true, async: false }) as string)
  } catch {
    return ''
  }
})

const bodyRef = ref<HTMLElement | null>(null)

// CJK 相邻二元组集合：引用句 vs 块文本的粗粒度相似度足够定位同段
const bigrams = (s: string): Set<string> => {
  const t = (s || '').replace(/\s+/g, '')
  const out = new Set<string>()
  for (let i = 0; i < t.length - 1; i++) out.add(t.slice(i, i + 2))
  return out
}

// 大 chunk 常混多个章节，从头展示会让用户觉得「对不上」。
// 用角标所在句子与各块的二元组重合度找最相关段落，滚动定位并高亮。
const locateRelevantBlock = async () => {
  await nextTick()
  const body = bodyRef.value
  if (!body) return
  body.querySelectorAll('.chat-citation-float__hit').forEach((el) => {
    el.classList.remove('chat-citation-float__hit')
  })
  const context = props.float.context || ''
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
  el.classList.add('chat-citation-float__hit')
  const elTop = el.offsetTop
  body.scrollTop = Math.max(0, elTop - body.clientHeight / 3)
}

watch(
  () => [props.float.visible, props.float.content, props.float.context],
  locateRelevantBlock,
)
</script>

<style lang="less">
@import './css/chat-citations.less';

/* 定位命中的相关段落：品牌底色轻高亮 */
.chat-citation-float .chat-citation-float__hit {
  background: var(--td-brand-color-1);
  border-radius: var(--app-radius-xs);
  box-shadow: 0 0 0 3px var(--td-brand-color-1);
  color: var(--td-text-color-primary);
}
</style>

<!-- markdown 排版必须 scoped：mixin 内 85 条 :deep() 规则只在 scoped 下编译生效，
     Teleport 到 body 的节点带本组件 data-v，:deep 能命中 v-html 内容 -->
<style lang="less" scoped>
@import './css/chat-markdown.less';

.chat-citation-float__body--md {
  white-space: normal;
  .chat-markdown-typography();

  // 卡片场景压扁标题层级：chunk 里的 #/## 只作分段强调，不按文档标题渲染
  :deep(h1),
  :deep(h2),
  :deep(h3),
  :deep(h4),
  :deep(h5),
  :deep(h6) {
    font-size: 1em;
    font-weight: 600;
    margin: 0.875em 0 0.25em;
    line-height: 1.5;
  }

  :deep(*:first-child) {
    margin-top: 0;
  }

  :deep(*:last-child) {
    margin-bottom: 0;
  }
}
</style>
