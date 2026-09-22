import { onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { resolveCitationChunkId, type CitationKnowledgeRef } from '@/utils/citationMarkdown'
import {
  getCitationChunkCache,
  setCitationChunkCache,
} from '@/utils/citationChunkCache'
import { useChatReferencesDrawer } from '@/composables/useChatReferencesDrawer'

export type CitationFloatState = {
  visible: boolean
  type: 'kb' | 'web'
  top: number
  left: number
  title: string
  content: string
  url: string
  loading: boolean
  error: string
  /** 角标所在句子的上下文（悬浮卡用于定位 chunk 内的相关段落） */
  context: string
}

type CitationPopoverOptions = {
  mode: 'chat' | 'embed'
  getKnowledgeReferences?: () => CitationKnowledgeRef[] | null | undefined
  getCacheScope: () => string
  fetchChunk: (chunkId: string) => Promise<{ data?: { content?: unknown } } | undefined>
  notFoundError?: () => string
  loadError: () => string
}

/** Shared interaction lifecycle; entry points retain their access and error policies. */
export function useCitationPopover(rootRef: Ref<HTMLElement | null>, options: CitationPopoverOptions) {
  const referencesDrawer = useChatReferencesDrawer()
  const isChat = options.mode === 'chat'
  const popupSelector = `.${options.mode}-citation-float`
  const float = ref<CitationFloatState>({
    visible: false,
    type: 'kb',
    top: 0,
    left: 0,
    title: '',
    content: '',
    url: '',
    loading: false,
    error: '',
    context: '',
  })

  let hoverTimer: number | null = null
  let closeTimer: number | null = null

  let requestVersion = 0
  let boundRoot: HTMLElement | null = null
  let disposed = false

  const cancelHover = () => {
    if (hoverTimer !== null) window.clearTimeout(hoverTimer)
    hoverTimer = null
  }

  const reset = () => {
    requestVersion++
    cancelHover()
    cancelClose()
    float.value.visible = false
    float.value.loading = false
    float.value.content = ''
    float.value.error = ''
    float.value.context = ''
  }

  const resolveChunkId = (el: HTMLElement) => {
    const raw = el.getAttribute('data-chunk-id') || ''
    return resolveCitationChunkId(raw, {
      doc: el.getAttribute('data-doc') || '',
      kbId: el.getAttribute('data-kb-id') || '',
    }, options.getKnowledgeReferences?.()) || raw
  }

  const positionFor = (el: HTMLElement, offsetY = 0) => {
    const rect = el.getBoundingClientRect()
    float.value.top = rect.bottom + window.scrollY + 6 + offsetY
    float.value.left = Math.min(rect.left + window.scrollX, window.innerWidth - 320)
  }

  const openWeb = (el: HTMLElement) => {
    requestVersion++
    const url = el.getAttribute('data-url') || ''
    float.value.type = 'web'
    float.value.url = url
    float.value.title = el.querySelector('.tip-title')?.textContent || ''
    float.value.content = ''
    float.value.loading = false
    float.value.error = ''
    float.value.context = ''
    float.value.visible = true
    positionFor(el)
  }

  const openKb = async (el: HTMLElement) => {
    // Embedded hover historically loads the raw ID; drawer highlights resolve aliases.
    const chunkId = isChat ? resolveChunkId(el) : el.getAttribute('data-chunk-id') || ''
    const title = el.getAttribute('data-doc') || ''
    if (!chunkId) return
    const version = ++requestVersion
    float.value.type = 'kb'
    float.value.title = title
    float.value.url = ''
    float.value.context =
      el.closest('li, p, dd, h1, h2, h3, h4, h5, h6, tr')?.textContent?.trim() || ''
    float.value.visible = true
    positionFor(el, 4)

    // 优先用引用条目自带的父 chunk 全文：单一子块常只覆盖相邻章节，
    // 从头展示会与声明「对不上」；全文 + 相关段定位可读性更好
    const refs = options.getKnowledgeReferences?.() || []
    const parent = refs.find(
      (r) => Array.isArray(r.sub_chunk_id) && r.sub_chunk_id.includes(chunkId),
    )
    if (parent?.content && parent.content.trim()) {
      float.value.content = parent.content
      float.value.loading = false
      return
    }

    const scope = options.getCacheScope()
    const cached = getCitationChunkCache(scope, chunkId)
    if (cached) {
      float.value.content = cached.content
      float.value.error = cached.error || ''
      float.value.loading = false
      return
    }

    float.value.loading = true
    float.value.error = ''
    float.value.content = ''
    let result: { content: string; error?: string }
    try {
      const res = await options.fetchChunk(chunkId)
      const content = String(res?.data?.content || '').trim()
      result = { content, ...(!content && options.notFoundError ? { error: options.notFoundError() } : {}) }
    } catch {
      result = { content: '', error: options.loadError() }
    }
    // Replaced hovers, roots and access scopes must not receive late responses.
    if (disposed || version !== requestVersion || scope !== options.getCacheScope()) return
    setCitationChunkCache(scope, chunkId, result)
    float.value.content = result.content
    float.value.error = result.error || ''
    float.value.loading = false
  }

  const scheduleClose = () => {
    cancelClose()
    closeTimer = window.setTimeout(() => {
      closeTimer = null
      if (isChat && document.querySelector(`.citation-kb:hover, .citation-web:hover, ${popupSelector}:hover`)) return
      float.value.visible = false
    }, 120)
  }

  const cancelClose = () => {
    if (closeTimer !== null) {
      window.clearTimeout(closeTimer)
      closeTimer = null
    }
  }

  const onMouseOver = (e: Event) => {
    const target = e.target as HTMLElement
    const kbEl = target.closest?.('.citation-kb') as HTMLElement | null
    const webEl = target.closest?.('.citation-web') as HTMLElement | null
    if (!kbEl && !webEl) return
    cancelClose()
    cancelHover()
    hoverTimer = window.setTimeout(() => {
      hoverTimer = null
      if (kbEl) void openKb(kbEl)
      else if (webEl) openWeb(webEl)
    }, kbEl ? 80 : 40)
  }

  const onMouseOut = (e: Event) => {
    const rt = (e as MouseEvent).relatedTarget as HTMLElement | null
    if (rt?.closest?.(`.citation-kb, .citation-web, ${popupSelector}`)) return
    cancelHover()
    scheduleClose()
  }

  const openDrawerForCitation = (payload: { url?: string; chunkId?: string }) => {
    const refs = options?.getKnowledgeReferences?.() || []
    if (!referencesDrawer || !refs.length) return false
    referencesDrawer.open({
      references: refs,
      highlight: payload,
    })
    return true
  }

  const onClick = (e: Event) => {
    const target = e.target as HTMLElement
    const webEl = target.closest?.('.citation-web') as HTMLElement | null
    if (webEl) {
      e.preventDefault()
      e.stopPropagation()
      const url = webEl.getAttribute('data-url') || ''
      if (openDrawerForCitation({ url })) return
      openWeb(webEl)
      return
    }

    const kbEl = target.closest?.('.citation-kb') as HTMLElement | null
    if (kbEl) {
      e.preventDefault()
      e.stopPropagation()
      const chunkId = resolveChunkId(kbEl)
      if (openDrawerForCitation({ chunkId })) return
      void openKb(kbEl)
      return
    }
    if (!isChat && target.closest?.('.citation-wiki')) {
      e.preventDefault()
      e.stopPropagation()
    }
  }

  const onViewportChange = () => {
    if (float.value.visible) scheduleClose()
  }

  const unbind = () => {
    boundRoot?.removeEventListener('mouseover', onMouseOver, true)
    boundRoot?.removeEventListener('mouseout', onMouseOut, true)
    boundRoot?.removeEventListener('click', onClick, true)
    window.removeEventListener('scroll', onViewportChange, true)
    window.removeEventListener('resize', onViewportChange, true)
    boundRoot = null
  }

  const rebind = () => {
    if (disposed || boundRoot === rootRef.value) return
    unbind()
    reset()
    boundRoot = rootRef.value
    if (!boundRoot) return
    boundRoot.addEventListener('mouseover', onMouseOver, true)
    boundRoot.addEventListener('mouseout', onMouseOut, true)
    boundRoot.addEventListener('click', onClick, true)
    if (isChat) {
      window.addEventListener('scroll', onViewportChange, true)
      window.addEventListener('resize', onViewportChange, true)
    }
  }

  watch(rootRef, rebind, { flush: 'post' })
  watch(options.getCacheScope, reset, { flush: 'sync' })
  onMounted(rebind)
  onBeforeUnmount(() => {
    disposed = true
    unbind()
    reset()
  })

  return { float, rebind, cancelClose, scheduleClose }
}
