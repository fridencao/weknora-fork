<template>
  <div class="ojk-rule-extractor">
    <t-card :bordered="false" class="wizard-card">
      <t-steps :current="currentStep" readonly>
        <t-step-item :title="$t('ojk.wizard.stepKb')" />
        <t-step-item :title="$t('ojk.wizard.stepConfirm')" />
        <t-step-item :title="$t('ojk.wizard.stepRun')" />
        <t-step-item :title="$t('ojk.wizard.stepDone')" />
      </t-steps>
    </t-card>

    <!-- Step 1 · 选择知识库 -->
    <t-card v-show="currentStep === 0" :title="$t('ojk.wizard.stepKb')" :bordered="false" class="wizard-card">
      <t-alert theme="info" :message="$t('ojk.ruleExtractor.desc')" class="wizard-alert" />
      <t-loading :loading="kbLoading">
        <div v-if="kbs.length" class="kb-grid">
          <div
            v-for="kb in kbs"
            :key="kb.id"
            class="kb-card"
            :class="{ active: selectedKbId === kb.id }"
            @click="selectedKbId = kb.id"
          >
            <div class="kb-name">{{ kb.name }}</div>
            <div class="kb-desc">{{ kb.description || $t('ojk.wizard.noDesc') }}</div>
          </div>
        </div>
        <t-empty v-else :description="$t('ojk.wizard.noKb')" />
      </t-loading>
      <template #footer>
        <t-space>
          <t-button theme="default" @click="fetchKbs" :loading="kbLoading">
            {{ $t('ojk.wizard.refresh') }}
          </t-button>
          <t-button theme="primary" :disabled="!selectedKbId" @click="currentStep = 1">
            {{ $t('ojk.wizard.next') }}
          </t-button>
        </t-space>
      </template>
    </t-card>

    <!-- Step 2 · 确认配置 -->
    <t-card v-show="currentStep === 1" :title="$t('ojk.wizard.stepConfirm')" :bordered="false" class="wizard-card">
      <t-descriptions :column="1" bordered>
        <t-descriptions-item :label="$t('ojk.wizard.confirmKb')">{{ selectedKbName }}</t-descriptions-item>
        <t-descriptions-item :label="$t('ojk.wizard.confirmScope')">{{ $t('ojk.wizard.confirmScopeValue') }}</t-descriptions-item>
        <t-descriptions-item :label="$t('ojk.wizard.confirmVersion')">1.0.0</t-descriptions-item>
      </t-descriptions>
      <template #footer>
        <t-space>
          <t-button theme="default" @click="currentStep = 0">{{ $t('ojk.wizard.back') }}</t-button>
          <t-button theme="primary" :loading="creating" @click="handleCreateRun">
            {{ $t('ojk.ruleExtractor.createRun') }}
          </t-button>
        </t-space>
      </template>
    </t-card>

    <!-- Step 3 · 生成进度 -->
    <t-card v-show="currentStep === 2" :title="$t('ojk.wizard.stepRun')" :bordered="false" class="wizard-card">
      <t-loading :loading="!activeRun">
        <template v-if="activeRun">
          <t-alert v-if="activeRun.status === 'failed'" theme="error" :message="activeRun.error || $t('ojk.wizard.runFailed')" />
          <div class="run-progress">
            <t-progress
              :theme="activeRun.status === 'failed' ? 'error' : (activeRun.status === 'done' ? 'success' : 'active')"
              :percentage="progressPct"
              :status="activeRun.status === 'failed' ? 'error' : undefined"
            />
            <div class="run-progress-text">
              {{ progressText }}
            </div>
          </div>
        </template>
      </t-loading>
      <template #footer>
        <t-space>
          <t-button theme="default" @click="resetToStep0">{{ $t('ojk.wizard.retry') }}</t-button>
          <t-button v-if="activeRun && activeRun.status === 'done'" theme="primary" @click="goToReview(activeRun.run_id)">
            {{ $t('ojk.wizard.goReview') }}
          </t-button>
        </t-space>
      </template>
    </t-card>

    <!-- Step 4 · 完成 -->
    <t-card v-show="currentStep === 3" :title="$t('ojk.wizard.stepDone')" :bordered="false" class="wizard-card">
      <t-alert theme="success" :message="$t('ojk.wizard.doneTitle')" />
      <t-descriptions :column="3" bordered class="done-stats">
        <t-descriptions-item :label="$t('ojk.wizard.doneItems')">{{ activeRun?.total_items ?? 0 }}</t-descriptions-item>
        <t-descriptions-item :label="$t('ojk.ruleExtractor.flaggedItems' in {} ? '' : 'ojk.wizard.doneFlagged')">{{ activeRun?.flagged_items ?? 0 }}</t-descriptions-item>
        <t-descriptions-item :label="$t('ojk.wizard.confirmKb')">{{ activeRun?.kb_name || selectedKbName }}</t-descriptions-item>
      </t-descriptions>
      <template #footer>
        <t-space>
          <t-button theme="default" @click="resetToStep0">{{ $t('ojk.wizard.again') }}</t-button>
          <t-button v-if="activeRun" theme="primary" @click="goToReview(activeRun.run_id)">
            {{ $t('ojk.wizard.goReview') }}
          </t-button>
        </t-space>
      </template>
    </t-card>

    <!-- 最近生成 -->
    <t-card :title="$t('ojk.ruleExtractor.recentRuns')" :bordered="false">
      <template #actions>
        <t-button theme="default" size="small" @click="fetchRuns" :loading="loading">
          {{ $t('ojk.wizard.refresh') }}
        </t-button>
      </template>
      <t-loading :loading="loading" size="large">
        <t-table
          :data="runs"
          :columns="runColumns"
          row-key="run_id"
          :pagination="false"
          :empty-text="$t('ojk.ruleExtractor.noRuns')"
        >
          <template #kb_name="{ record }">{{ record.kb_name || record.kb_id }}</template>
          <template #status="{ record }">
            <t-tag :theme="runStatusTheme(record.status)" variant="light">
              {{ runStatusLabel(record.status) }}
            </t-tag>
          </template>
          <template #progress="{ record }">
            <span v-if="record.status === 'running' && record.slices_total">
              {{ record.slices_done || 0 }} / {{ record.slices_total }}
            </span>
            <span v-else>-</span>
          </template>
          <template #actions="{ record }">
            <t-space size="small">
              <t-button
                v-if="record.status === 'done'"
                theme="primary" size="small"
                @click="goToReview(record.run_id)"
              >
                {{ $t('ojk.ruleExtractor.review') }}
              </t-button>
              <t-button
                v-if="record.status === 'running' || record.status === 'pending'"
                theme="default" size="small"
                @click="watchRun(record.run_id)"
              >
                {{ $t('ojk.wizard.stepRun') }}
              </t-button>
            </t-space>
          </template>
        </t-table>
      </t-loading>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { listKnowledgeBases } from '@/api/knowledge-base'
import {
  createOJKRun, getOJKRun, listOJKRuns,
  type OJKRun,
} from '@/api/ojk'

const router = useRouter()
const { t } = useI18n()

const currentStep = ref(0)
const kbs = ref<Array<{ id: string; name: string; description?: string }>>([])
const kbLoading = ref(false)
const selectedKbId = ref('')
const creating = ref(false)

const activeRun = ref<OJKRun | null>(null)
let pollTimer: number | null = null

const runs = ref<OJKRun[]>([])
const loading = ref(false)

const selectedKbName = computed(() =>
  kbs.value.find(k => k.id === selectedKbId.value)?.name || selectedKbId.value)

const progressPct = computed(() => {
  const run = activeRun.value
  if (!run) return 0
  if (run.status === 'done') return 100
  if (run.status === 'pending') return 0
  if (run.slices_total && (run.slices_done ?? 0) > 0) {
    return Math.round(((run.slices_done ?? 0) / run.slices_total) * 100)
  }
  return 5
})

const progressText = computed(() => {
  const run = activeRun.value
  if (!run) return ''
  switch (run.status) {
    case 'pending': return t('ojk.wizard.queued')
    case 'running':
      return run.slices_total
        ? t('ojk.wizard.runningBatches', { done: run.slices_done ?? 0, total: run.slices_total })
        : t('ojk.wizard.runningPrep')
    case 'done': return t('ojk.wizard.doneSummary', { items: run.total_items, flagged: run.flagged_items })
    case 'failed': return run.error || t('ojk.wizard.runFailed')
    default: return run.status
  }
})

const runColumns = [
  { colKey: 'kb_name', title: t('ojk.wizard.confirmKb') },
  { colKey: 'status', title: t('ojk.wizard.statusTitle'), width: 110 },
  { colKey: 'progress', title: t('ojk.wizard.progressTitle'), width: 120 },
  { colKey: 'total_items', title: t('ojk.wizard.doneItems'), width: 90, align: 'center' },
  { colKey: 'flagged_items', title: t('ojk.wizard.doneFlagged'), width: 90, align: 'center' },
  { colKey: 'created_at', title: t('ojk.wizard.createdAt'), width: 170 },
  { colKey: 'actions', title: t('ojk.wizard.actionsTitle'), width: 150, fixed: 'right' },
]

function runStatusTheme(status: string): string {
  const map: Record<string, string> = {
    pending: 'default', running: 'warning', done: 'success', failed: 'danger',
  }
  return map[status] || 'default'
}

function runStatusLabel(status: string): string {
  return t(`ojk.status.${status}`)
}

async function fetchKbs() {
  kbLoading.value = true
  try {
    const res = await listKnowledgeBases()
    const payload = (res as any)?.data
    kbs.value = Array.isArray(payload) ? payload : (payload?.items || payload?.list || [])
    if (selectedKbId.value && !kbs.value.some(k => k.id === selectedKbId.value)) {
      selectedKbId.value = ''
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to load knowledge bases')
  } finally {
    kbLoading.value = false
  }
}

async function fetchRuns() {
  loading.value = true
  try {
    const res = await listOJKRuns(20)
    runs.value = res.runs || []
  } catch { /* 列表失败不打断向导 */ } finally {
    loading.value = false
  }
}

async function handleCreateRun() {
  if (!selectedKbId.value) return
  creating.value = true
  try {
    const run = await createOJKRun(selectedKbId.value)
    activeRun.value = run
    currentStep.value = 2
    startPolling(run.run_id)
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to create run')
  } finally {
    creating.value = false
  }
}

function startPolling(runId: string) {
  stopPolling()
  pollTimer = window.setInterval(async () => {
    try {
      const run = await getOJKRun(runId)
      activeRun.value = run
      if (run.status === 'done') {
        stopPolling()
        currentStep.value = 3
        fetchRuns()
      } else if (run.status === 'failed') {
        stopPolling()
        fetchRuns()
      }
    } catch { /* 轮询容错 */ }
  }, 2000)
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function watchRun(runId: string) {
  activeRun.value = runs.value.find(r => r.run_id === runId) || null
  currentStep.value = 2
  startPolling(runId)
}

function resetToStep0() {
  stopPolling()
  activeRun.value = null
  currentStep.value = 0
  fetchRuns()
}

function goToReview(runId: string) {
  stopPolling()
  router.push(`/platform/ojk/review/${runId}`)
}

onMounted(() => {
  fetchKbs()
  fetchRuns()
})

onUnmounted(stopPolling)
</script>

<style scoped>
.ojk-rule-extractor {
  padding: var(--app-space-md, 16px);
  display: flex;
  flex-direction: column;
  gap: var(--app-space-md, 16px);
}
.wizard-alert {
  margin-bottom: var(--app-space-sm, 8px);
}
.kb-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: var(--app-space-sm, 12px);
}
.kb-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 12px);
  cursor: pointer;
  transition: border-color var(--app-motion-fast, 120ms);
}
.kb-card.active {
  border-color: var(--td-brand-color);
  background: color-mix(in srgb, var(--td-brand-color) 6%, transparent);
}
.kb-name {
  font-size: var(--app-text-base, 14px);
  font-weight: 600;
}
.kb-desc {
  font-size: var(--app-text-sm, 12px);
  color: var(--td-text-color-secondary);
  margin-top: var(--app-space-xs, 4px);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.run-progress {
  padding: var(--app-space-sm, 12px) 0;
}
.run-progress-text {
  font-size: var(--app-text-sm, 12px);
  color: var(--td-text-color-secondary);
  margin-top: var(--app-space-xs, 4px);
}
.done-stats {
  margin-top: var(--app-space-sm, 8px);
}
</style>
