<template>
  <!-- 失败态：点开看原因 + 重试入口（ADR-008 决策 4） -->
  <t-popup v-if="status === 'failed'" v-model:visible="panelOpen" trigger="click"
    placement="top" overlay-class-name="graph-badge-popup" :disabled="!interactive">
    <span class="graph-badge graph-badge--failed" role="button" tabindex="0"
      :aria-label="`${label} · ${t('knowledgeBase.graphBadge.errorTitle')}`" @click.stop>
      <t-icon name="error-circle" />
      <span>{{ label }}</span>
    </span>
    <template #content>
      <div class="graph-badge-panel" @click.stop>
        <div class="graph-badge-panel__title">{{ t('knowledgeBase.graphBadge.errorTitle') }}</div>
        <div class="graph-badge-panel__msg">{{ error || t('knowledgeBase.graphBadge.unknownError') }}</div>
        <div v-if="attempts" class="graph-badge-panel__attempts">
          {{ t('knowledgeBase.graphBadge.attempts', { n: attempts }) }}
        </div>
        <t-button v-if="interactive" size="small" theme="primary" :loading="retrying"
          @click.stop="emit('retry')">
          {{ retrying ? t('knowledgeBase.graphBadge.retrying') : t('knowledgeBase.graphBadge.retry') }}
        </t-button>
      </div>
    </template>
  </t-popup>

  <t-tooltip v-else :content="tip" placement="top">
    <span class="graph-badge" :class="`graph-badge--${stateKey}`">
      <t-icon v-if="icon" :name="icon" :class="{ 'icon-spin': spin }" />
      <span>{{ label }}</span>
    </span>
  </t-tooltip>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

/**
 * ADR-008 决策 4：文档列表页每行的 LightRAG 图谱状态徽标。
 *
 * 状态取值与 starkb-api graph_doc_state.status 一一对应：
 * none | pending | building | ready | failed | stale | deleting
 */
const props = defineProps<{
  status: string
  error?: string
  attempts?: number
  /** 有写权限才给重试按钮；只读用户只看原因。 */
  interactive?: boolean
  retrying?: boolean
}>()

const emit = defineEmits<{ (e: 'retry'): void }>()

const { t } = useI18n()
const panelOpen = ref(false)

const interactive = computed(() => props.interactive !== false)

/** starkb-api 将来加了新状态时，兜底成「未建图」而不是把 i18n key 渲染到界面上。 */
const KNOWN_STATES = ['none', 'pending', 'building', 'ready', 'failed', 'stale', 'deleting']
const stateKey = computed(() => (KNOWN_STATES.includes(props.status) ? props.status : 'none'))

const label = computed(() => t(`knowledgeBase.graphBadge.state.${stateKey.value}`))

/** 图标只用在「进行中」与「已建图」上，其余保持纯文字，避免一行里图标打架。 */
const icon = computed(() => {
  switch (stateKey.value) {
    case 'pending':
    case 'building':
    case 'deleting':
      return 'loading'
    case 'ready':
      return 'check-circle'
    case 'stale':
      return 'time'
    default:
      return ''
  }
})

const spin = computed(() =>
  stateKey.value === 'pending' || stateKey.value === 'building' || stateKey.value === 'deleting')

const tip = computed(() => t(`knowledgeBase.graphBadge.tip.${stateKey.value}`))
</script>

<style scoped lang="less">
.graph-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex-shrink: 0;
  padding: 0 6px;
  height: 20px;
  border-radius: var(--td-radius-small);
  font-size: 11px;
  line-height: 1;
  white-space: nowrap;
  cursor: default;

  .t-icon {
    font-size: 12px;
  }
}

/* 未建图是最常见的状态，压到最轻，避免整列都在喊。 */
.graph-badge--none {
  color: var(--td-text-color-placeholder);
  background: var(--td-bg-color-container-hover);
}

.graph-badge--pending,
.graph-badge--stale {
  color: var(--td-warning-color);
  background: var(--td-warning-color-light);
}

.graph-badge--building,
.graph-badge--deleting {
  color: var(--td-brand-color);
  background: var(--td-brand-color-light);
}

.graph-badge--ready {
  color: var(--td-success-color);
  background: var(--td-success-color-light);
}

.graph-badge--failed {
  color: var(--td-error-color);
  background: var(--td-error-color-light);
  cursor: pointer;
}

.icon-spin {
  animation: graph-badge-spin 1s linear infinite;
}

@keyframes graph-badge-spin {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}
</style>

<style lang="less">
.graph-badge-popup .graph-badge-panel {
  max-width: 320px;
  padding: 4px 2px;
  display: flex;
  flex-direction: column;
  gap: 6px;

  &__title {
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  &__msg {
    color: var(--td-text-color-secondary);
    font-size: 12px;
    line-height: 1.5;
    word-break: break-all;
    max-height: 140px;
    overflow: auto;
  }

  &__attempts {
    color: var(--td-text-color-placeholder);
    font-size: 12px;
  }
}
</style>
