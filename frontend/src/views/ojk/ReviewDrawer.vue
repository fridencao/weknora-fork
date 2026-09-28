<template>
  <t-drawer
    :visible="visible"
    attach="body"
    size="92%"
    :header="false"
    :footer="false"
    :close-btn="false"
    destroy-on-close
    @close="emitClose"
  >
    <div class="rd">
      <!-- 顶栏：返回 / 标题 / 状态徽标 / 材料库 / 刷新 / 关闭 -->
      <div class="rd-topbar">
        <t-button variant="text" shape="square" @click="emitClose">
          <template #icon><t-icon name="arrow-left" /></template>
        </t-button>
        <span class="rd-title">{{ $t('ojk.reviewWorkflow.drawerTitle') }}</span>
        <t-tag v-if="run" :theme="run.status === 'done' ? 'success' : 'warning'" variant="light" effect="light">
          ● {{ run.status === 'done' ? $t('ojk.reviewWorkflow.badgeDone') : $t('ojk.reviewWorkflow.badgeRunning') }}
        </t-tag>
        <t-space class="rd-topbar-right" align="center">
          <t-select
            v-model="caseKbId"
            clearable
            filterable
            :placeholder="$t('ojk.reviewWorkflow.caseKbPlaceholder')"
            :loading="kbLoading"
            style="width: 200px"
            @change="onCaseKbChange"
          >
            <t-option v-for="kb in kbOptions" :key="kb.id" :value="kb.id" :label="kb.name" />
          </t-select>
          <t-button variant="text" shape="square" :loading="loadingRun" @click="loadAll">
            <template #icon><t-icon name="refresh" /></template>
          </t-button>
          <t-button variant="text" shape="square" @click="emitClose">
            <template #icon><t-icon name="close" /></template>
          </t-button>
        </t-space>
      </div>

      <!-- 运行元信息条 -->
      <div class="rd-meta" v-if="run">
        <div class="rd-meta-cell">
          <span class="rd-meta-label">{{ $t('ojk.reviewWorkflow.runIdLabel') }}</span>
          <div class="rd-meta-value">
            <code class="rd-runid">{{ run.run_id }}</code>
            <t-button variant="text" size="small" @click="copyRunId">
              <template #icon><t-icon name="file-copy" /></template>
            </t-button>
          </div>
          <div class="rd-meta-sub">
            <span>{{ $t('ojk.reviewWorkflow.createdAtLabel') }}</span>
            <span class="rd-meta-mono">{{ formatTime(run.created_at) }}</span>
          </div>
        </div>
        <div class="rd-meta-cell">
          <span class="rd-meta-label">{{ $t('ojk.reviewWorkflow.versionLabel') }}</span>
          <div class="rd-meta-value rd-meta-strong">v{{ run.skill_version }}</div>
          <div class="rd-meta-sub">
            <span>{{ $t('ojk.reviewWorkflow.flaggedLabel') }}</span>
            <t-tag v-if="run.flagged_items > 0" theme="warning" variant="light" size="small">
              ⚠ {{ $t('ojk.reviewWorkflow.flaggedNeed', { n: run.flagged_items }) }}
            </t-tag>
            <t-tag v-else theme="success" variant="light" size="small">0</t-tag>
          </div>
        </div>
        <div class="rd-meta-cell">
          <span class="rd-meta-label">{{ $t('ojk.reviewWorkflow.totalItemsLabel') }}</span>
          <div class="rd-meta-value rd-meta-strong">{{ run.total_items }}
            <span class="rd-meta-unit">{{ $t('ojk.reviewWorkflow.itemsUnit') }}</span>
          </div>
        </div>
      </div>

      <!-- 三统计卡 -->
      <div class="rd-stats" v-if="run">
        <div class="rd-stat rd-stat--ok">
          <div class="rd-stat-head">
            <span>{{ $t('ojk.reviewWorkflow.statConfirmed') }}</span>
            <t-icon name="check-circle" theme="success" size="20px" />
          </div>
          <div class="rd-stat-num">{{ stats.confirmed || 0 }}</div>
          <div class="rd-stat-cap">● {{ $t('ojk.reviewWorkflow.statConfirmedCap') }}</div>
        </div>
        <div class="rd-stat rd-stat--pending">
          <div class="rd-stat-head">
            <span>{{ $t('ojk.reviewWorkflow.statPending') }}</span>
            <t-icon name="hourglass" theme="primary" size="20px" />
          </div>
          <div class="rd-stat-num">{{ stats.pending || 0 }}</div>
          <div class="rd-stat-cap">● {{ $t('ojk.reviewWorkflow.statPendingCap') }}</div>
        </div>
        <div class="rd-stat rd-stat--bad">
          <div class="rd-stat-head">
            <span>{{ $t('ojk.reviewWorkflow.statRejected') }}</span>
            <t-icon name="close-circle" theme="danger" size="20px" />
          </div>
          <div class="rd-stat-num">{{ stats.rejected || 0 }}</div>
          <div class="rd-stat-cap">● {{ $t('ojk.reviewWorkflow.statRejectedCap') }}</div>
        </div>
      </div>

      <!-- 复核进度 -->
      <div class="rd-progress" v-if="run">
        <span>{{ $t('ojk.reviewWorkflow.progressLabel', { done: doneCount, total: run.total_items }) }}</span>
        <div class="rd-progress-bar">
          <t-progress :percentage="progressPct" :stroke-width="8" />
        </div>
        <span class="rd-progress-pct">{{ progressPct }}%</span>
      </div>

      <!-- 清单条目 -->
      <div class="rd-items">
        <div class="rd-items-head">
          <span class="rd-items-title">{{ $t('ojk.reviewWorkflow.itemsTitle') }}</span>
          <t-radio-group v-model="filterStatus" variant="default-filled" @change="onFilterChange">
            <t-radio-button value="">{{ $t('ojk.reviewWorkflow.tabAll', { n: run?.total_items ?? 0 }) }}</t-radio-button>
            <t-radio-button value="pending">{{ $t('ojk.reviewWorkflow.tabPending', { n: stats.pending || 0 }) }}</t-radio-button>
            <t-radio-button value="confirmed">{{ $t('ojk.reviewWorkflow.tabConfirmed', { n: stats.confirmed || 0 }) }}</t-radio-button>
            <t-radio-button value="rejected">{{ $t('ojk.reviewWorkflow.tabRejected', { n: stats.rejected || 0 }) }}</t-radio-button>
          </t-radio-group>
          <t-input v-model="itemSearch" clearable :placeholder="$t('ojk.reviewWorkflow.searchPlaceholder')" style="width: 220px">
            <template #prefixIcon><t-icon name="search" /></template>
          </t-input>
        </div>

        <t-loading :loading="loadingItems" size="large">
          <t-table
            :data="pagedItems"
            :columns="itemColumns"
            row-key="id"
            :pagination="pagination"
            @page-change="onPageChange"
            size="small"
          >
            <template #pasal="{ row }">
              <span class="rd-pasal">{{ row.pasal || '-' }}</span>
            </template>
            <template #area="{ row }">
              <t-tag v-if="row.area" theme="primary" variant="light" size="small">{{ areaLabel(row.area) }}</t-tag>
              <span v-else>-</span>
            </template>
            <template #requirement="{ row }">
              <div class="rd-req">{{ row.requirement }}</div>
              <div v-if="row._flag" class="rd-flag-box">⚠ {{ row._flag }}</div>
            </template>
            <template #pasal_text="{ row }">
              <t-popup trigger="hover" placement="top" :show-arrow="true">
                <template #content>
                  <div class="rd-ptext-pop">{{ row.pasal_text }}</div>
                </template>
                <span class="rd-ptext">{{ row.pasal_text }}</span>
              </t-popup>
            </template>
            <template #actions="{ row }">
              <t-space v-if="row.status === 'pending'" size="small">
                <t-button theme="primary" size="small" variant="outline" @click="aiCheck(row)">
                  <template #icon><t-icon name="robot" /></template>
                  {{ $t('ojk.reviewWorkflow.aiCheck') }}
                </t-button>
                <t-button theme="success" size="small" @click="resolve(row, 'confirmed')">
                  {{ $t('ojk.reviewWorkflow.confirm') }}
                </t-button>
                <t-button theme="danger" size="small" variant="outline" @click="openReject(row)">
                  {{ $t('ojk.reviewWorkflow.reject') }}
                </t-button>
              </t-space>
              <t-space v-else size="small">
                <t-tag :theme="row.status === 'confirmed' ? 'success' : 'danger'" variant="light" size="small">
                  {{ row.status === 'confirmed' ? $t('ojk.reviewWorkflow.tagConfirmed') : $t('ojk.reviewWorkflow.tagRejected') }}
                </t-tag>
                <t-link theme="default" size="small" @click="aiCheck(row)">
                  {{ $t('ojk.reviewWorkflow.aiCheckAgain') }}
                </t-link>
              </t-space>
            </template>
          </t-table>
        </t-loading>
      </div>

      <!-- 底部操作栏 -->
      <div class="rd-footer">
        <t-tooltip :content="$t('ojk.reviewWorkflow.batchAiTip')">
          <t-button theme="default" disabled>
            <template #icon><t-icon name="rocket" /></template>
            {{ $t('ojk.reviewWorkflow.batchAi') }}
          </t-button>
        </t-tooltip>
        <t-button theme="default" :loading="autoConfirming" :disabled="!(stats.pending > 0)" @click="autoConfirmNoFlag">
          <template #icon><t-icon name="check-double" /></template>
          {{ $t('ojk.reviewWorkflow.autoConfirm') }}
        </t-button>
        <t-space class="rd-footer-right">
          <t-button theme="default" @click="emitClose">{{ $t('ojk.reviewWorkflow.cancel') }}</t-button>
          <t-button theme="primary" @click="saveConclusions">
            <template #icon><t-icon name="save" /></template>
            {{ $t('ojk.reviewWorkflow.saveConclusions') }}
          </t-button>
        </t-space>
      </div>

      <!-- 驳回对话框 -->
      <t-dialog
        v-model:visible="rejectVisible"
        :header="$t('ojk.reviewWorkflow.rejectTitle')"
        @confirm="handleReject"
      >
        <t-form layout="vertical" v-if="rejectTarget">
          <t-form-item :label="$t('ojk.reviewWorkflow.reason')" name="note">
            <t-textarea v-model="rejectNote" :rows="3" />
          </t-form-item>
        </t-form>
      </t-dialog>
    </div>
  </t-drawer>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { getOJKRun, listOJKItems, resolveOJKItem, getOJKItemStats, type OJKRun, type OJKItem } from '@/api/ojk'
import { listKnowledgeBases } from '@/api/knowledge-base'
import { useMenuStore } from '@/stores/menu'

const props = defineProps<{ runId: string; visible: boolean }>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'close'): void
}>()

const { t } = useI18n()
const menuStore = useMenuStore()

const run = ref<OJKRun | null>(null)
const items = ref<OJKItem[]>([])
const stats = ref<Record<string, number>>({})
const filterStatus = ref('')
const itemSearch = ref('')
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const loadingRun = ref(false)
const loadingItems = ref(false)
let pollTimer: number | null = null

// ---- 本案材料库（AI 核验前置选择，记忆上次选择）----
const CASE_KB_STORE_KEY = 'ojk-review-case-kb'
const kbOptions = ref<Array<{ id: string; name: string }>>([])
const kbLoading = ref(false)
const caseKbId = ref('')

async function loadKbOptions() {
  kbLoading.value = true
  try {
    const res = await listKnowledgeBases()
    const payload = (res as any)?.data
    kbOptions.value = (Array.isArray(payload) ? payload : payload?.items || [])
      .map((kb: any) => ({ id: kb.id, name: kb.name }))
    const saved = localStorage.getItem(CASE_KB_STORE_KEY)
    if (saved && kbOptions.value.some(kb => kb.id === saved)) {
      caseKbId.value = saved
    }
  } catch { /* 选择器失败不打断复核 */ } finally {
    kbLoading.value = false
  }
}

function onCaseKbChange(id: string) {
  if (id) localStorage.setItem(CASE_KB_STORE_KEY, id)
}

// ---- 数据加载 ----
async function loadRun() {
  if (!props.runId) return
  loadingRun.value = true
  try {
    run.value = await getOJKRun(props.runId)
    if (run.value.status === 'running' || run.value.status === 'pending') startPolling()
    else stopPolling()
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to load run')
  } finally {
    loadingRun.value = false
  }
}

async function loadItems() {
  if (!props.runId) return
  loadingItems.value = true
  try {
    const res = await listOJKItems(
      props.runId, filterStatus.value || undefined, page.value, pageSize.value,
    )
    items.value = res.items
    total.value = res.total
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to load items')
  } finally {
    loadingItems.value = false
  }
}

async function loadStats() {
  if (!props.runId) return
  try {
    stats.value = await getOJKItemStats(props.runId)
  } catch { /* 统计失败不打断 */ }
}

function loadAll() {
  loadRun()
  loadItems()
  loadStats()
}

// run 进行中时轮询状态（抽取异步完成后明细自动就绪）
function startPolling() {
  if (pollTimer) return
  pollTimer = window.setInterval(async () => {
    if (!run.value) return
    try {
      const fresh = await getOJKRun(run.value.run_id)
      run.value = fresh
      if (fresh.status === 'done' || fresh.status === 'failed') {
        stopPolling()
        loadItems()
        loadStats()
      }
    } catch { /* 轮询容错 */ }
  }, 3000)
}
function stopPolling() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}

// ---- 统计/进度 ----
const doneCount = computed(() => (stats.value.confirmed || 0) + (stats.value.rejected || 0))
const progressPct = computed(() => {
  const total = run.value?.total_items || 0
  if (!total) return 0
  return Math.round((doneCount.value / total) * 1000) / 10
})

// ---- 表格 ----
const AREA_LABELS: Record<string, { zh: string; key: string }> = {
  Integrity: { zh: '诚信度', key: 'integrity' },
  'Financial Reputation': { zh: '财务声誉', key: 'finReputation' },
  Competence: { zh: '胜任力', key: 'competence' },
  Structure: { zh: '架构合理性', key: 'structure' },
  Completeness: { zh: '完备性', key: 'completeness' },
}
function areaLabel(area: string): string {
  return AREA_LABELS[area]?.zh || area
}

const itemColumns = [
  { colKey: 'pasal', title: 'PASAL (条款)', width: 150, cell: 'pasal' },
  { colKey: 'regulation', title: 'REGULATION', width: 140 },
  { colKey: 'area', title: 'AREA', width: 120, cell: 'area' },
  { colKey: 'requirement', title: 'REQUIREMENT (法条要求)', minWidth: 260, cell: 'requirement' },
  { colKey: 'pasal_text', title: 'PASAL TEXT', minWidth: 200, cell: 'pasal_text' },
  { colKey: 'actions', title: 'ACTIONS (操作)', width: 240, cell: 'actions' },
]

// 搜索：客户端过滤当前页（服务端无 search 参数）
const pagedItems = computed(() => {
  const q = itemSearch.value.trim().toLowerCase()
  if (!q) return items.value
  return items.value.filter(it =>
    [it.pasal, it.requirement, it.regulation, it.id]
      .some(v => (v || '').toLowerCase().includes(q)))
})

const pagination = computed(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: total.value,
}))

function onPageChange(p: { current?: number }) {
  page.value = p?.current ?? 1
  loadItems()
}

function onFilterChange() {
  page.value = 1
  itemSearch.value = ''
  loadItems()
}

// ---- 裁定 ----
async function resolve(row: OJKItem, status: 'confirmed' | 'rejected', note = '') {
  try {
    await resolveOJKItem(row.id, status, note)
    row.status = status
    loadStats()
    MessagePlugin.success(status === 'confirmed'
      ? t('ojk.reviewWorkflow.confirmDone')
      : t('ojk.reviewWorkflow.rejectDone'))
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to resolve')
  }
}

const rejectVisible = ref(false)
const rejectTarget = ref<OJKItem | null>(null)
const rejectNote = ref('')

function openReject(row: OJKItem) {
  rejectTarget.value = row
  rejectNote.value = ''
  rejectVisible.value = true
}

async function handleReject() {
  if (!rejectTarget.value) return
  await resolve(rejectTarget.value, 'rejected', rejectNote.value)
  rejectVisible.value = false
}

// ---- AI 核验：挂本案材料库 + 法规库跳对话预填核验问题 ----
function aiCheck(item: OJKItem) {
  if (!caseKbId.value) {
    MessagePlugin.warning(t('ojk.reviewWorkflow.caseKbRequired'))
    return
  }
  localStorage.setItem(CASE_KB_STORE_KEY, caseKbId.value)
  const caseKbName = kbOptions.value.find(kb => kb.id === caseKbId.value)?.name || caseKbId.value
  const question = t('ojk.reviewWorkflow.aiCheckPrompt', {
    pasal: item.pasal || '-',
    regulation: item.regulation || '-',
    requirement: item.requirement,
    method: item.check_method || 'document_presence',
    evidence: item.evidence_type || '-',
    roles: (item.applicable_roles || []).join(', ') || '*',
    severity: item.severity,
    caseKb: caseKbName,
    reqId: item.requirement_id || item.id,
  })
  // 双库挂载：法规库（run 抽取来源）+ 本案材料库
  const kbIds = [run.value?.kb_id, caseKbId.value].filter(Boolean) as string[]
  menuStore.setPrefillKbIds(kbIds)
  menuStore.setPrefillQuery(question)
  window.open('/platform/creatChat', '_blank')
}

// ---- 一键确认无异议项（无校验告警的待核验条目）----
const autoConfirming = ref(false)

async function autoConfirmNoFlag() {
  const targets: OJKItem[] = []
  let pageN = 1
  loadingItems.value = true
  try {
    for (;;) {
      const res = await listOJKItems(props.runId, 'pending', pageN, 100)
      targets.push(...(res.items || []).filter(it => !it._flag))
      if (targets.length >= (res.total || 0) || !(res.items || []).length) break
      pageN += 1
    }
  } finally {
    loadingItems.value = false
  }
  if (!targets.length) {
    MessagePlugin.warning(t('ojk.reviewWorkflow.autoConfirmNone'))
    return
  }
  autoConfirming.value = true
  let ok = 0
  for (const it of targets) {
    try {
      await resolveOJKItem(it.id, 'confirmed', '')
      ok++
    } catch { /* 单条失败不中断批量 */ }
  }
  autoConfirming.value = false
  loadItems()
  loadStats()
  MessagePlugin.success(t('ojk.reviewWorkflow.autoConfirmDone', { n: ok }))
}

// ---- 杂项 ----
function formatTime(iso: string): string {
  return (iso || '').replace('T', ' ').slice(0, 19)
}

async function copyRunId() {
  const id = run.value?.run_id || ''
  try {
    await navigator.clipboard.writeText(id)
    MessagePlugin.success(t('ojk.reviewWorkflow.copySuccess'))
  } catch {
    MessagePlugin.error(t('ojk.reviewWorkflow.copyFail'))
  }
}

function saveConclusions() {
  MessagePlugin.success(t('ojk.reviewWorkflow.conclusionsSaved'))
  emitClose()
}

function emitClose() {
  stopPolling()
  emit('update:visible', false)
  emit('close')
}

// 打开时加载全部；runId 变化重置状态。immediate 必须开：
// 深链场景 visible 初始即为 true，没有"变化"可触发。
// 注意 immediate 回调的 oldValue 是 undefined，不能直接解构
watch(
  () => [props.visible, props.runId] as const,
  (nv, ov) => {
    const [vis, rid] = nv
    const wasVisible = Array.isArray(ov) ? ov[0] : false
    if (vis && rid) {
      filterStatus.value = ''
      itemSearch.value = ''
      page.value = 1
      loadAll()
      loadKbOptions()
    }
    if (!vis && wasVisible) stopPolling()
  },
  { immediate: true },
)

onUnmounted(stopPolling)
</script>

<style scoped>
.rd {
  display: flex;
  flex-direction: column;
  gap: var(--app-space-md, 14px);
  min-height: 100%;
  font-size: var(--app-text-sm, 13px);
}

/* 顶栏 */
.rd-topbar {
  display: flex;
  align-items: center;
  gap: var(--app-space-sm, 10px);
  padding-bottom: var(--app-space-sm, 10px);
  border-bottom: 1px solid var(--td-component-stroke);
}
.rd-title { font-size: var(--app-text-lg, 16px); font-weight: 600; }
.rd-topbar-right { margin-left: auto; }

/* 运行元信息条 */
.rd-meta {
  display: grid;
  grid-template-columns: 1.4fr 1fr 1fr;
  gap: var(--app-space-md, 14px);
}
.rd-meta-cell {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 12px);
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.rd-meta-label { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); }
.rd-meta-value { display: flex; align-items: center; gap: 6px; }
.rd-meta-strong { font-size: var(--app-text-lg, 16px); font-weight: 700; }
.rd-meta-unit { font-size: var(--app-text-xs, 11px); color: var(--td-text-color-secondary); font-weight: 400; }
.rd-meta-sub { display: flex; align-items: center; gap: 6px; color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); }
.rd-meta-mono { font-family: var(--td-font-family, monospace); }
.rd-runid { font-family: var(--td-font-family, monospace); font-size: var(--app-text-sm, 12px); background: var(--td-bg-color-secondarycontainer); padding: 2px 6px; border-radius: 4px; }

/* 三统计卡 */
.rd-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--app-space-md, 14px);
}
.rd-stat {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-md, 14px);
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.rd-stat--ok { border-left: 3px solid var(--td-success-color); }
.rd-stat--pending { border-left: 3px solid var(--td-brand-color); }
.rd-stat--bad { border-left: 3px solid var(--td-error-color); }
.rd-stat-head { display: flex; justify-content: space-between; align-items: center; color: var(--td-text-color-secondary); }
.rd-stat-num { font-size: 30px; font-weight: 700; line-height: 1.1; }
.rd-stat-cap { color: var(--td-text-color-placeholder); font-size: var(--app-text-xs, 11px); }

/* 进度条 */
.rd-progress { display: flex; align-items: center; gap: var(--app-space-md, 14px); }
.rd-progress-bar { flex: 1; }
.rd-progress-pct { font-weight: 600; font-variant-numeric: tabular-nums; }

/* 明细表 */
.rd-items { display: flex; flex-direction: column; gap: var(--app-space-sm, 10px); }
.rd-items-head { display: flex; align-items: center; gap: var(--app-space-md, 14px); flex-wrap: wrap; }
.rd-items-title { font-size: var(--app-text-md, 14px); font-weight: 600; margin-right: auto; }
.rd-pasal { color: var(--td-brand-color); }
.rd-req { line-height: 1.5; }
.rd-flag-box {
  margin-top: 6px;
  padding: 6px 8px;
  border: 1px dashed var(--td-warning-color);
  border-radius: 4px;
  color: var(--td-warning-color);
  font-size: var(--app-text-xs, 11px);
}
.rd-ptext {
  display: inline-block;
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}
/* 悬浮全文弹层：限宽 + 自动换行（默认不限宽会拉通整屏） */
.rd-ptext-pop {
  max-width: 420px;
  max-height: 260px;
  overflow-y: auto;
  white-space: normal;
  word-break: break-word;
  line-height: 1.6;
  font-size: var(--app-text-sm, 12px);
}

/* 底部操作栏 */
.rd-footer {
  position: sticky;
  bottom: 0;
  display: flex;
  align-items: center;
  gap: var(--app-space-md, 14px);
  padding: var(--app-space-sm, 10px) 0;
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-stroke);
}
.rd-footer-right { margin-left: auto; }
</style>
