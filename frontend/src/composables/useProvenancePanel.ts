import { inject, provide, ref, type InjectionKey, type Ref } from 'vue'
import type { ProvenanceInput } from '@/utils/provenance'

export type ProvenancePanelOpenOptions = {
  /** 打开面板所依据的原始引用/检索结果行（携带 metadata / chunk_metadata） */
  inputs: ProvenanceInput[]
  /** 可选标题（如文档名），缺省用 i18n 默认标题 */
  title?: string
  /** 角标所在句子的上下文（用于在 chunk 全文里定位相关段落） */
  context?: string
}

export type ProvenancePanelContext = {
  visible: Ref<boolean>
  inputs: Ref<ProvenanceInput[]>
  title: Ref<string>
  context: Ref<string>
  open: (options: ProvenancePanelOpenOptions) => void
  close: () => void
}

const PROVENANCE_PANEL_KEY: InjectionKey<ProvenancePanelContext> = Symbol('provenancePanel')

export function provideProvenancePanel(): ProvenancePanelContext {
  const visible = ref(false)
  const inputs = ref<ProvenanceInput[]>([])
  const title = ref('')
  const context = ref('')

  const open = (options: ProvenancePanelOpenOptions) => {
    inputs.value = Array.isArray(options.inputs) ? options.inputs.filter(Boolean) : []
    title.value = options.title?.trim() || ''
    context.value = options.context?.trim() || ''
    visible.value = true
  }

  const close = () => {
    visible.value = false
    inputs.value = []
    title.value = ''
    context.value = ''
  }

  const ctx: ProvenancePanelContext = { visible, inputs, title, context, open, close }
  provide(PROVENANCE_PANEL_KEY, ctx)
  return ctx
}

export function useProvenancePanel(): ProvenancePanelContext | null {
  return inject(PROVENANCE_PANEL_KEY, null)
}
