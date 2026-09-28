<template>
  <div class="ojk-rule-extractor">
    <t-space direction="vertical" :size="16" style="width: 100%">
      <!-- Header -->
      <t-card :title="$t('ojk.ruleExtractor.title')" :bordered="false">
        <template #actions>
          <t-button theme="primary" @click="handleCreateRun" :loading="creating">
            {{ $t('ojk.ruleExtractor.createRun') }}
          </t-button>
        </template>
        <t-alert theme="info" :message="$t('ojk.ruleExtractor.desc')" />
      </t-card>

      <!-- Recent runs -->
      <t-card :title="$t('ojk.ruleExtractor.recentRuns')" :bordered="false">
        <t-loading :loading="loading" size="large">
          <t-table
            :data="runs"
            :columns="runColumns"
            rowKey="run_id"
            :pagination="false"
            :empty-text="$t('ojk.ruleExtractor.noRuns')"
          >
            <template #status="{ record }">
              <t-tag :theme="runStatusTheme(record.status)" variant="light">
                {{ runStatusLabel(record.status) }}
              </t-tag>
            </template>
            <template #actions="{ record }">
              <t-space>
                <t-button theme="default" size="small" @click="goToReview(record.run_id)">
                  {{ $t('ojk.ruleExtractor.review') }}
                </t-button>
                <t-button theme="primary" size="small" @click="refreshRun(record.run_id)">
                  {{ $t('ojk.ruleExtractor.refresh') }}
                </t-button>
              </t-space>
            </template>
          </t-table>
        </t-loading>
      </t-card>
    </t-space>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { createOJKRun, getOJKRun, type OJKRun } from '@/api/ojk'

const router = useRouter()
const runs = ref<OJKRun[]>([])
const loading = ref(false)
const creating = ref(false)
const pollingTimer = ref<number | null>(null)

const runColumns = [
  { colKey: 'run_id', title: 'Run ID', width: 220 },
  { colKey: 'skill_version', title: 'Version', width: 100 },
  { colKey: 'status', title: 'Status', width: 120 },
  { colKey: 'total_items', title: 'Items', width: 80, align: 'center' },
  { colKey: 'flagged_items', title: 'Flagged', width: 80, align: 'center' },
  { colKey: 'created_at', title: 'Created', width: 180 },
  { colKey: 'actions', title: 'Actions', width: 200, fixed: 'right' },
]

function runStatusTheme(status: string): string {
  const map: Record<string, string> = {
    pending: 'default', running: 'warning', done: 'success', failed: 'danger',
  }
  return map[status] || 'default'
}

function runStatusLabel(status: string): string {
  const map: Record<string, string> = {
    pending: 'Pending', running: 'Running', done: 'Done', failed: 'Failed',
  }
  return map[status] || status
}

async function fetchRuns() {
  // For now, list runs from a stub — will be added to API later
  // Placeholder: in production this would call GET /ojk/runs
  loading.value = true
  try {
    // runs.value = await listOJKRuns()
  } finally {
    loading.value = false
  }
}

async function handleCreateRun() {
  creating.value = true
  try {
    const res = await createOJKRun('1.0.0')
    MessagePlugin.success('Run created')
    await startPolling(res.run_id)
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to create run')
  } finally {
    creating.value = false
  }
}

async function startPolling(runId: string) {
  if (pollingTimer.value) clearInterval(pollingTimer.value)
  const tick = async () => {
    try {
      const run = await getOJKRun(runId)
      if (run.status === 'done' || run.status === 'failed') {
        if (pollingTimer.value) clearInterval(pollingTimer.value)
        pollingTimer.value = null
        await fetchRuns()
        if (run.status === 'done') {
          MessagePlugin.success(`Run ${runId} completed: ${run.total_items} items`)
          goToReview(runId)
        }
      }
    } catch { /* ignore */ }
  }
  tick()
  pollingTimer.value = window.setInterval(tick, 3000)
}

function goToReview(runId: string) {
  router.push(`/platform/ojk/review/${runId}`)
}

function refreshRun(runId: string) {
  startPolling(runId)
}

onMounted(fetchRuns)
</script>

<style scoped>
.ojk-rule-extractor {
  padding: 16px;
}
</style>
