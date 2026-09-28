<template>
  <div class="ojk-review-workflow">
    <t-space direction="vertical" :size="16" style="width: 100%">
      <!-- Run header -->
      <t-card :bordered="false">
        <template #title>
          <t-space>
            <t-button theme="default" shape="circle" @click="router.back()">
              <t-icon name="arrow-left" />
            </t-button>
            <span>{{ $t('ojk.reviewWorkflow.title') }}</span>
            <t-tag v-if="run" :theme="runStatusTheme(run.status)" variant="light">
              {{ runStatusLabel(run.status) }}
            </t-tag>
          </t-space>
        </template>
        <template #extra>
          <t-space>
            <t-button @click="loadRun" :loading="loadingRun">
              {{ $t('ojk.reviewWorkflow.refresh') }}
            </t-button>
            <t-button theme="primary" @click="goBack">
              {{ $t('ojk.reviewWorkflow.back') }}
            </t-button>
          </t-space>
        </template>
        <t-descriptions v-if="run" :column="3" bordered size="large">
          <t-descriptions-item :label="$t('ojk.reviewWorkflow.runId')">{{ run.run_id }}</t-descriptions-item>
          <t-descriptions-item :label="$t('ojk.reviewWorkflow.version')">{{ run.skill_version }}</t-descriptions-item>
          <t-descriptions-item :label="$t('ojk.reviewWorkflow.items')">{{ run.total_items }}</t-descriptions-item>
          <t-descriptions-item :label="$t('ojk.reviewWorkflow.flagged')">{{ run.flagged_items }}</t-descriptions-item>
          <t-descriptions-item :label="$t('ojk.reviewWorkflow.createdAt')">{{ run.created_at }}</t-descriptions-item>
          <t-descriptions-item v-if="run.error" :label="$t('ojk.reviewWorkflow.error')" :span="3">{{ run.error }}</t-descriptions-item>
        </t-descriptions>
      </t-card>

      <!-- Stats -->
      <t-row :gutter="16" v-if="stats">
        <t-col :span="6" v-for="(count, status) in stats" :key="status">
          <t-card :bordered="false">
            <t-statistic :title="statusLabel(status)" :value="count" />
          </t-card>
        </t-col>
      </t-row>

      <!-- Items table -->
      <t-card :bordered="false" :title="$t('ojk.reviewWorkflow.itemsTitle')">
        <template #extra>
          <t-radio-group v-model="filterStatus" @change="loadItems">
            <t-radio-button value="">All</t-radio-button>
            <t-radio-button value="pending">Pending</t-radio-button>
            <t-radio-button value="confirmed">Confirmed</t-radio-button>
            <t-radio-button value="rejected">Rejected</t-radio-button>
          </t-radio-group>
        </template>

        <t-loading :loading="loadingItems" size="large">
          <t-table
            :data="items"
            :columns="itemColumns"
            rowKey="id"
            :pagination="pagination"
            @page-change="onPageChange"
            :scroll="{ x: 1600 }"
          >
            <template #severity="{ record }">
              <t-tag :theme="severityTheme(record.severity)" variant="light">
                {{ record.severity }}
              </t-tag>
            </template>
            <template #status="{ record }">
              <t-tag :theme="itemStatusTheme(record.status)" variant="light">
                {{ record.status }}
              </t-tag>
            </template>
            <template #pasal_text="{ record }">
              <t-popup trigger="hover" placement="right">
                <template #content>{{ record.pasal_text }}</template>
                <span class="text-ellipsis">{{ record.pasal_text.slice(0, 60) }}...</span>
              </t-popup>
            </template>
            <template #actions="{ record }">
              <t-space v-if="record.status === 'pending'">
                <t-button theme="success" size="small" @click="resolveItem(record, 'confirmed')">
                  {{ $t('ojk.reviewWorkflow.confirm') }}
                </t-button>
                <t-button theme="danger" size="small" @click="showReject(record)">
                  {{ $t('ojk.reviewWorkflow.reject') }}
                </t-button>
              </t-space>
              <span v-else class="text-muted">{{ record.status }}</span>
            </template>
          </t-table>
        </t-loading>
      </t-card>
    </t-space>

    <!-- Reject dialog -->
    <t-dialog
      v-model:visible="rejectVisible"
      :title="$t('ojk.reviewWorkflow.rejectTitle')"
      @confirm="handleReject"
    >
      <t-form layout="vertical" v-if="rejectTarget">
        <t-form-item :label="$t('ojk.reviewWorkflow.reason')" name="note">
          <t-textarea v-model="rejectNote" :rows="3" />
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { getOJKRun, listOJKItems, resolveOJKItem, getOJKItemStats, type OJKRun, type OJKItem } from '@/api/ojk'

const route = useRoute()
const router = useRouter()
const runId = computed(() => route.params.id as string)

const run = ref<OJKRun | null>(null)
const items = ref<OJKItem[]>([])
const stats = ref<Record<string, number>>({})
const filterStatus = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const loadingRun = ref(false)
const loadingItems = ref(false)

const pagination = computed(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: total.value,
  showTotal: true,
  showJumper: true,
}))

const itemColumns = [
  { colKey: 'pasal', title: 'Pasal', width: 160, fixed: 'left' },
  { colKey: 'regulation', title: 'Regulation', width: 140 },
  { colKey: 'area', title: 'Area', width: 130 },
  { colKey: 'requirement', title: 'Requirement', minWidth: 280 },
  { colKey: 'pasal_text', title: 'Pasal Text', minWidth: 240 },
  { colKey: 'severity', title: 'Severity', width: 100 },
  { colKey: 'source', title: 'Source', width: 90 },
  { colKey: 'status', title: 'Status', width: 100 },
  { colKey: '_flag', title: 'Flag', width: 140 },
  { colKey: 'actions', title: 'Actions', width: 180, fixed: 'right' },
]

let pollTimer: number | null = null

function runStatusTheme(s: string) {
  return { pending: 'default', running: 'warning', done: 'success', failed: 'danger' }[s] || 'default'
}
function runStatusLabel(s: string) {
  return { pending: 'Pending', running: 'Running', done: 'Done', failed: 'Failed' }[s] || s
}
function severityTheme(s: string) {
  return { critical: 'danger', clarification: 'warning', info: 'default' }[s] || 'default'
}
function itemStatusTheme(s: string) {
  return { pending: 'default', confirmed: 'success', rejected: 'danger' }[s] || 'default'
}
function statusLabel(s: string) {
  return { pending: 'Pending', confirmed: 'Confirmed', rejected: 'Rejected' }[s] || s
}

async function loadRun() {
  loadingRun.value = true
  try {
    run.value = await getOJKRun(runId.value)
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to load run')
  } finally {
    loadingRun.value = false
  }
}

async function loadItems() {
  loadingItems.value = true
  try {
    const res = await listOJKItems(runId.value, filterStatus.value || undefined, page.value, pageSize.value)
    items.value = res.items
    total.value = res.total
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to load items')
  } finally {
    loadingItems.value = false
  }
}

async function loadStats() {
  try {
    stats.value = await getOJKItemStats(runId.value)
  } catch { /* ignore */ }
}

async function resolveItem(item: OJKItem, status: 'confirmed' | 'rejected') {
  await resolveOJKItem(item.id, status, '')
  MessagePlugin.success(`Item ${status}`)
  await Promise.all([loadItems(), loadStats(), loadRun()])
}

const rejectVisible = ref(false)
const rejectTarget = ref<OJKItem | null>(null)
const rejectNote = ref('')

function showReject(item: OJKItem) {
  rejectTarget.value = item
  rejectNote.value = ''
  rejectVisible.value = true
}

async function handleReject() {
  if (!rejectTarget.value) return
  await resolveItem(rejectTarget.value, 'rejected')
  rejectVisible.value = false
}

function onPageChange(p: number) {
  page.value = p
  loadItems()
}

function goBack() {
  router.push('/platform/ojk/rules')
}

function startPolling() {
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = window.setInterval(async () => {
    if (run.value?.status === 'pending' || run.value?.status === 'running') {
      await loadRun()
      if (run.value?.status === 'done' || run.value?.status === 'failed') {
        if (pollTimer) clearInterval(pollTimer)
        pollTimer = null
        await loadItems()
        await loadStats()
      }
    }
  }, 3000)
}

onMounted(async () => {
  await Promise.all([loadRun(), loadItems(), loadStats()])
  startPolling()
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<style scoped>
.ojk-review-workflow {
  padding: 16px;
}
.text-ellipsis {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: block;
  max-width: 300px;
}
.text-muted {
  color: #999;
}
</style>
