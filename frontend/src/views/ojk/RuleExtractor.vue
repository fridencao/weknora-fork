<template>
  <div class="ojk-stage1">
    <!-- 四阶段导航 -->
    <t-card :bordered="false" class="block">
      <t-steps :current="0" readonly>
        <t-step-item :title="$t('ojk.stage1.stageKb')" />
        <t-step-item :title="$t('ojk.stage1.stageMaterial')" />
        <t-step-item :title="$t('ojk.stage1.stageAiCheck')" />
        <t-step-item
          :title="$t('ojk.stage1.stageWorkbench')"
          :style="{ cursor: currentDoneRun ? 'pointer' : 'default' }"
          @click="goReview(currentDoneRun?.run_id)"
        />
      </t-steps>
    </t-card>

    <!-- 零幻觉协议 -->
    <t-card :bordered="false" class="block zhp">
      <div class="zhp-row">
        <t-icon name="secured" class="zhp-icon" />
        <div class="zhp-main">
          <div class="zhp-title">
            {{ $t('ojk.stage1.zhpTitle') }}
            <t-tag theme="danger" variant="light" size="small">{{ $t('ojk.stage1.zhpBadge') }}</t-tag>
          </div>
          <div class="zhp-body">{{ $t('ojk.stage1.zhpBody') }}</div>
        </div>
        <code class="zhp-store">{{ $t('ojk.stage1.storeBadge', { version: currentVersionLabel }) }}</code>
      </div>
    </t-card>

    <!-- 法规知识库 -->
    <t-card :bordered="false" class="block">
      <t-loading :loading="kbLoading">
        <div class="regkb">
          <t-icon name="books" class="regkb-icon" />
          <div class="regkb-main">
            <div class="regkb-title">
              {{ $t('ojk.stage1.regKbTitle') }}
              <t-tag theme="success" variant="light" size="small">{{ $t('ojk.stage1.regKbBadge') }}</t-tag>
            </div>
            <div class="regkb-chips">
              <code v-for="t in regTitles" :key="t" class="regchip">{{ t }}</code>
            </div>
            <div class="regkb-body">
              {{ $t('ojk.stage1.regKbBody', { sections: regKbMeta?.pasal_sections ?? '—', docs: regKbMeta?.docs ?? '—', items: currentDoneRun?.total_items ?? 0 }) }}
            </div>
          </div>
          <div class="regkb-badges">
            <span class="pill">{{ $t('ojk.stage1.badgeGraph') }}</span>
            <span class="pill pill--ok">{{ $t('ojk.stage1.badgeReadonly') }}</span>
            <span class="pill">{{ $t('ojk.stage1.badgeControlled') }}</span>
          </div>
        </div>
      </t-loading>
    </t-card>

    <!-- 版本资产库 -->
    <t-card :bordered="false" class="block">
      <template #title>
        <div class="store-title">
          <t-icon name="layers" />
          <span>{{ $t('ojk.stage1.storeTitle') }}</span>
        </div>
      </template>
      <template #actions>
        <t-button theme="primary" @click="openDraft">
          {{ $t('ojk.stage1.newDraft') }}
        </t-button>
      </template>
      <t-loading :loading="runsLoading">
        <t-row :gutter="16">
          <!-- CURRENT -->
          <t-col :span="6">
            <div class="ver-card ver-card--current" :class="{ selected: viewRunId === currentDoneRun?.run_id }" @click="viewRunId = currentDoneRun?.run_id">
              <div class="ver-head">
                <t-tag theme="primary" size="large">{{ $t('ojk.stage1.currentProd') }}</t-tag>
                <span class="ver-name">{{ $t('ojk.stage1.currentVersion', { version: currentVersionLabel }) }}</span>
                <t-tag v-if="currentDoneRun" theme="success" variant="light" size="small">{{ $t('ojk.stage1.dualReviewed') }}</t-tag>
              </div>
              <template v-if="currentDoneRun">
                <div class="ver-body">{{ $t('ojk.stage1.currentBody', { items: currentDoneRun.total_items, flagged: currentDoneRun.flagged_items }) }}</div>
                <div class="ver-dims">
                  <span v-for="d in dimSummary" :key="d.area" class="dim-chip">
                    {{ d.area }} · {{ d.count }}
                  </span>
                </div>
                <div class="ver-meta">
                  <span>{{ $t('ojk.stage1.signedAt', { time: formatTime(currentDoneRun.updated_at) }) }}</span>
                  <t-button size="small" theme="default" variant="text" @click.stop="viewRunId = currentDoneRun.run_id">
                    {{ $t('ojk.stage1.browseItems', { items: currentDoneRun.total_items }) }}
                  </t-button>
                </div>
              </template>
              <t-empty v-else size="small" :description="$t('ojk.stage1.noProduction')" />
            </div>
          </t-col>
          <!-- STAGING -->
          <t-col :span="6">
            <div class="ver-card" :class="{ selected: viewRunId === stagingRun?.run_id }" @click="stagingRun && (viewRunId = stagingRun.run_id)">
              <div class="ver-head">
                <t-tag theme="warning" size="large" variant="light">{{ $t('ojk.stage1.staging') }}</t-tag>
                <span v-if="stagingRun" class="ver-name">{{ $t('ojk.stage1.currentVersion', { version: stagingRun.skill_version }) }}</span>
                <t-tag v-if="stagingRun" theme="warning" variant="light" size="small">
                  {{ $t('ojk.stage1.stagingProgress', { done: stagingRun.slices_done ?? 0, total: stagingRun.slices_total ?? 0 }) }}
                </t-tag>
              </div>
              <template v-if="stagingRun">
                <div class="ver-body">
                  {{ stagingRun.status === 'failed' ? stagingRun.error : $t('ojk.stage1.stagingBody') }}
                </div>
                <t-progress
                  :percentage="stagingPct"
                  :status="stagingRun.status === 'failed' ? 'error' : undefined"
                />
                <div class="ver-meta">
                  <t-button size="small" theme="default" variant="text" @click.stop="goReview(stagingRun.run_id)">
                    {{ $t('ojk.stage1.openReview') }}
                  </t-button>
                </div>
              </template>
              <t-empty v-else size="small" :description="$t('ojk.stage1.stagingIdle')" />
            </div>
          </t-col>
        </t-row>
      </t-loading>
    </t-card>

    <!-- 清单明细 -->
    <t-card :bordered="false" class="block">
      <template #title>
        <div class="store-title">
          <t-icon name="view-list" />
          <span>{{ $t('ojk.stage1.detailTitle') }}</span>
        </div>
      </template>
      <template #actions>
        <t-space>
          <t-input v-model="itemSearch" :placeholder="$t('ojk.stage1.searchPlaceholder')" clearable style="width: 220px">
            <template #prefixIcon><t-icon name="search" /></template>
          </t-input>
          <span class="total-chip">{{ $t('ojk.stage1.totalReq', { n: filteredItems.length }) }}</span>
        </t-space>
      </template>
      <t-loading :loading="itemsLoading">
        <t-table
          :data="pagedItems"
          :columns="itemColumns"
          row-key="id"
          :pagination="itemPagination"
          @page-change="onItemPage"
        >
          <template #dim="{ record }">
            <t-tag v-if="record.area" theme="primary" variant="light" size="small">{{ record.area }}</t-tag>
            <span v-else>-</span>
          </template>
          <template #severity="{ record }">
            <t-tag :theme="record.severity === 'critical' ? 'danger' : (record.severity === 'clarification' ? 'warning' : 'default')" variant="light" size="small">
              {{ record.severity }}
            </t-tag>
          </template>
          <template #status="{ record }">
            <t-tag :theme="itemStatusTheme(record.status)" variant="light" size="small">
              {{ itemStatusLabel(record.status) }}
            </t-tag>
          </template>
          <template #action="{ record }">
            <t-button size="small" theme="default" variant="text" @click="goReview(record.run_id)">
              {{ $t('ojk.stage1.actionReview') }}
            </t-button>
          </template>
        </t-table>
      </t-loading>
    </t-card>

    <!-- 底部当前设定 -->
    <div class="footer-bar">
      <div class="footer-current">
        <t-icon name="check-circle" theme="success" />
        <span>{{ $t('ojk.stage1.footerCurrent', { kb: regKbMeta?.kb_name ?? '—', version: currentVersionLabel }) }}</span>
      </div>
      <t-space>
        <t-button theme="default" @click="exportJson">{{ $t('ojk.stage1.exportJson') }}</t-button>
        <t-button theme="default" @click="exportExcel">{{ $t('ojk.stage1.exportExcel') }}</t-button>
        <t-button theme="default" @click="reloadAll">{{ $t('ojk.stage1.reload') }}</t-button>
        <t-tooltip :content="$t('ojk.stage1.nextStageTip')">
          <t-button theme="primary" disabled>{{ $t('ojk.stage1.nextStage') }}</t-button>
        </t-tooltip>
      </t-space>
    </div>

    <!-- New Draft 对话框 -->
    <t-dialog
      v-model:visible="draftVisible"
      :header="$t('ojk.stage1.newDraftTitle')"
      :confirm-btn="$t('ojk.stage1.newDraftConfirm')"
      @confirm="createDraft"
    >
      <t-form layout="vertical">
        <t-form-item :label="$t('ojk.stage1.newDraftKb')">
          <t-radio-group v-model="draftKbId">
            <div class="draft-kb-list">
              <t-radio v-for="kb in regulationKbs" :key="kb.id" :value="kb.id">
                {{ kb.name }}（{{ kbMeta[kb.id]?.pasal_sections ?? 0 }} Pasal）
              </t-radio>
            </div>
          </t-radio-group>
        </t-form-item>
        <t-alert v-if="!regulationKbs.length" theme="warning" :message="$t('ojk.wizard.noRegulationKb')" />
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { listKnowledgeBases } from '@/api/knowledge-base'
import {
  createOJKRun, getOJKRun, listOJKRuns, listOJKItems, preflightOJK,
  type OJKRun, type OJKItem, type OJKPreflight,
} from '@/api/ojk'

const router = useRouter()
const { t } = useI18n()

// ---- 知识库（角色分拣：只有法规库进入本页面语义）----
const kbs = ref<Array<{ id: string; name: string; description?: string }>>([])
const kbMeta = ref<Record<string, OJKPreflight>>({})
const kbLoading = ref(false)
const selectedKbId = ref('')

const regulationKbs = computed(() => kbs.value.filter(kb => (kbMeta.value[kb.id]?.pasal_sections ?? 0) > 0))
const regKbMeta = computed(() => kbMeta.value[selectedKbId.value] || null)
const regTitles = computed(() =>
  (regKbMeta.value?.titles || []).map(t =>
    t.replace(/\.(pdf|PDF)$/, '').replace(/_/g, ' ').trim()))

// ---- runs：CURRENT = 最新 done；STAGING = 最新 running/pending ----
const runs = ref<OJKRun[]>([])
const runsLoading = ref(false)
let pollTimer: number | null = null

const currentDoneRun = computed(() => runs.value.find(r => r.status === 'done') || null)
const stagingRun = computed(() =>
  runs.value.find(r => r.status === 'running' || r.status === 'pending') || null)
const currentVersionLabel = computed(() => {
  if (currentDoneRun.value) return `v${currentDoneRun.value.skill_version}`
  return '—'
})

const stagingPct = computed(() => {
  const r = stagingRun.value
  if (!r) return 0
  if (r.status === 'done') return 100
  if (r.slices_total && (r.slices_done ?? 0) > 0) {
    return Math.round(((r.slices_done ?? 0) / r.slices_total) * 100)
  }
  return 5
})

// ---- 明细表：当前查看版本的全体条目（全量拉取，客户端过滤/分页/导出）----
const viewRunId = ref('')
const items = ref<OJKItem[]>([])
const itemsLoading = ref(false)
const itemSearch = ref('')
const itemPage = ref(1)
const PAGE_SIZE = 20

const filteredItems = computed(() => {
  const q = itemSearch.value.trim().toLowerCase()
  if (!q) return items.value
  return items.value.filter(it =>
    [it.pasal, it.requirement, it.area, it.regulation, it.id]
      .some(v => (v || '').toLowerCase().includes(q)))
})

const pagedItems = computed(() => {
  const start = (itemPage.value - 1) * PAGE_SIZE
  return filteredItems.value.slice(start, start + PAGE_SIZE)
})

const itemPagination = computed(() => ({
  current: itemPage.value,
  pageSize: PAGE_SIZE,
  total: filteredItems.value.length,
  showJumper: true,
}))

function onItemPage(page: number) {
  itemPage.value = page
}

watch(viewRunId, () => { itemPage.value = 1; loadItems() })
watch(itemSearch, () => { itemPage.value = 1 })

const dimSummary = computed(() => {
  const counts = new Map<string, number>()
  for (const it of items.value) {
    const key = it.area || '—'
    counts.set(key, (counts.get(key) || 0) + 1)
  }
  return [...counts.entries()]
    .map(([area, count]) => ({ area, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 5)
})

const itemColumns = [
  { colKey: 'id', title: t('ojk.stage1.colId'), width: 150 },
  { colKey: 'area', title: t('ojk.stage1.colDim'), width: 130, cell: 'dim' },
  { colKey: 'pasal', title: t('ojk.stage1.colAnchor'), width: 200 },
  { colKey: 'requirement', title: t('ojk.stage1.colCriteria'), ellipsis: true },
  { colKey: 'evidence_type', title: t('ojk.stage1.colEvidence'), width: 160 },
  { colKey: 'severity', title: t('ojk.stage1.colSeverity'), width: 100, cell: 'severity' },
  { colKey: 'status', title: t('ojk.stage1.colStatus'), width: 110, cell: 'status' },
  { colKey: 'action', title: t('ojk.stage1.colAction'), width: 100, cell: 'action' },
]

function itemStatusTheme(status: string): string {
  const map: Record<string, string> = {
    pending: 'warning', confirmed: 'success', rejected: 'danger',
  }
  return map[status] || 'default'
}

function itemStatusLabel(status: string): string {
  return t(`ojk.status.${status}`)
}

function formatTime(iso: string): string {
  return (iso || '').replace('T', ' ').slice(0, 16)
}

async function fetchKbs() {
  kbLoading.value = true
  try {
    const res = await listKnowledgeBases()
    const payload = (res as any)?.data
    kbs.value = Array.isArray(payload) ? payload : (payload?.items || payload?.list || [])
    await Promise.all(kbs.value.map(async kb => {
      try {
        kbMeta.value[kb.id] = await preflightOJK(kb.id)
      } catch {
        kbMeta.value[kb.id] = { kb_id: kb.id, kb_name: kb.name, docs: 0, pasal_sections: 0 }
      }
    }))
    // 唯一法规库自动选定
    if (!selectedKbId.value && regulationKbs.value.length === 1) {
      selectedKbId.value = regulationKbs.value[0].id
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to load knowledge bases')
  } finally {
    kbLoading.value = false
  }
}

async function fetchRuns() {
  runsLoading.value = true
  try {
    const res = await listOJKRuns(50)
    runs.value = res.runs || []
    // 默认查看最新定版
    if (!viewRunId.value || !runs.value.some(r => r.run_id === viewRunId.value)) {
      viewRunId.value = currentDoneRun.value?.run_id || stagingRun.value?.run_id || ''
    }
    // staging 进行中 → 持续轮询
    if (stagingRun.value) startPolling()
    else stopPolling()
  } finally {
    runsLoading.value = false
  }
}

async function loadItems() {
  if (!viewRunId.value) { items.value = []; return }
  itemsLoading.value = true
  try {
    const all: OJKItem[] = []
    let page = 1
    for (;;) {
      const res = await listOJKItems(viewRunId.value, undefined, page, 100)
      all.push(...(res.items || []))
      if (all.length >= (res.total || 0) || !(res.items || []).length) break
      page += 1
    }
    items.value = all
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to load checklist items')
  } finally {
    itemsLoading.value = false
  }
}

function startPolling() {
  if (pollTimer) return
  pollTimer = window.setInterval(async () => {
    const staging = stagingRun.value
    if (!staging) { stopPolling(); return }
    try {
      const run = await getOJKRun(staging.run_id)
      const idx = runs.value.findIndex(r => r.run_id === run.run_id)
      if (idx >= 0) runs.value[idx] = run
      if (run.status === 'done' || run.status === 'failed') {
        stopPolling()
        fetchRuns()
      }
    } catch { /* 轮询容错 */ }
  }, 3000)
}

function stopPolling() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}

// ---- New Draft ----
const draftVisible = ref(false)
const draftKbId = ref('')
const creating = ref(false)

function openDraft() {
  draftKbId.value = selectedKbId.value || regulationKbs.value[0]?.id || ''
  draftVisible.value = true
}

async function createDraft() {
  if (!draftKbId.value) {
    MessagePlugin.warning(t('ojk.wizard.noRegulationKb'))
    return
  }
  creating.value = true
  try {
    const run = await createOJKRun(draftKbId.value)
    MessagePlugin.success(t('ojk.stage1.draftCreated', { id: run.run_id }))
    draftVisible.value = false
    viewRunId.value = run.run_id
    await fetchRuns()
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to create run')
  } finally {
    creating.value = false
  }
}

// ---- 导出 ----
function download(name: string, blob: Blob) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

function exportJson() {
  const blob = new Blob([JSON.stringify(items.value, null, 2)], { type: 'application/json' })
  download(`ojk-checklist-${viewRunId.value}.json`, blob)
}

async function exportExcel() {
  try {
    const XLSX: any = await import('xlsx')
    const ws = XLSX.utils.json_to_sheet(items.value.map(it => ({
      ID: it.id, Regulation: it.regulation, Pasal: it.pasal,
      Area: it.area || '', Requirement: it.requirement,
      Evidence: it.evidence_type || '', CheckMethod: it.check_method || '',
      Severity: it.severity, Status: it.status,
    })))
    const wb = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(wb, ws, 'Checklist')
    XLSX.writeFile(wb, `ojk-checklist-${viewRunId.value}.xlsx`)
  } catch {
    // xlsx 加载失败兜底 CSV（带 BOM 防 Excel 中文乱码）
    const header = 'ID,Regulation,Pasal,Area,Requirement,Evidence,CheckMethod,Severity,Status'
    const rows = items.value.map(it =>
      [it.id, it.regulation, it.pasal, it.area || '', it.requirement,
       it.evidence_type || '', it.check_method || '', it.severity, it.status]
        .map(v => `"${String(v).replace(/"/g, '""')}"`).join(','))
    const blob = new Blob(['\uFEFF' + header + '\n' + rows.join('\n')], { type: 'text/csv' })
    download(`ojk-checklist-${viewRunId.value}.csv`, blob)
  }
}

function reloadAll() {
  fetchKbs()
  fetchRuns()
  loadItems()
}

function goReview(runId?: string) {
  if (!runId) return
  router.push(`/platform/ojk/review/${runId}`)
}

onMounted(async () => {
  await Promise.all([fetchKbs(), fetchRuns()])
})

onUnmounted(stopPolling)
</script>

<style scoped>
.ojk-stage1 {
  padding: var(--app-space-md, 16px);
  display: flex;
  flex-direction: column;
  gap: var(--app-space-md, 16px);
  padding-bottom: 72px;
}
.block { border-radius: var(--app-radius-md, 8px); }

.zhp { border-left: 3px solid var(--td-brand-color); }
.zhp-row { display: flex; gap: var(--app-space-md, 12px); align-items: flex-start; }
.zhp-icon { font-size: var(--app-text-2xl, 22px); color: var(--td-brand-color); }
.zhp-title { font-weight: 600; display: flex; align-items: center; gap: var(--app-space-xs, 6px); }
.zhp-body { font-size: var(--app-text-sm, 12px); color: var(--td-text-color-secondary); margin-top: var(--app-space-xs, 4px); }
.zhp-store { font-size: var(--app-text-sm, 12px); background: var(--td-bg-color-container); padding: 4px 10px; border-radius: var(--app-radius-sm, 6px); white-space: nowrap; }

.regkb { display: flex; gap: var(--app-space-md, 12px); align-items: flex-start; }
.regkb-icon { font-size: var(--app-text-2xl, 22px); color: var(--td-brand-color); }
.regkb-main { flex: 1; }
.regkb-title { font-weight: 600; display: flex; align-items: center; gap: var(--app-space-xs, 6px); }
.regkb-chips { display: flex; flex-wrap: wrap; gap: var(--app-space-xs, 6px); margin: var(--app-space-xs, 4px) 0; }
.regchip { font-size: var(--app-text-xs, 11px); background: var(--td-bg-color-secondarycontainer); padding: 1px 8px; border-radius: var(--app-radius-sm, 6px); }
.regkb-body { font-size: var(--app-text-sm, 12px); color: var(--td-text-color-secondary); }
.regkb-badges { display: flex; flex-direction: column; gap: var(--app-space-xs, 4px); }
.pill { font-size: var(--app-text-xs, 11px); border: 1px solid var(--td-component-stroke); border-radius: var(--app-radius-pill, 999px); padding: 1px 10px; white-space: nowrap; }
.pill--ok { color: var(--td-success-color); border-color: var(--td-success-color); }

.store-title { display: flex; align-items: center; gap: var(--app-space-xs, 6px); }
.ver-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-md, 12px);
  cursor: pointer;
  min-height: 150px;
}
.ver-card--current { border-color: var(--td-brand-color); }
.ver-card.selected { box-shadow: 0 0 0 2px color-mix(in srgb, var(--td-brand-color) 25%, transparent); }
.ver-head { display: flex; align-items: center; gap: var(--app-space-xs, 6px); flex-wrap: wrap; }
.ver-name { font-weight: 600; }
.ver-body { font-size: var(--app-text-sm, 12px); color: var(--td-text-color-secondary); margin: var(--app-space-xs, 6px) 0; }
.ver-dims { display: flex; flex-wrap: wrap; gap: var(--app-space-xs, 4px); margin-bottom: var(--app-space-xs, 6px); }
.dim-chip { font-size: var(--app-text-xs, 11px); background: var(--td-bg-color-secondarycontainer); border-radius: var(--app-radius-sm, 6px); padding: 1px 8px; }
.ver-meta { display: flex; justify-content: space-between; align-items: center; font-size: var(--app-text-xs, 11px); color: var(--td-text-color-placeholder); }
.total-chip { font-size: var(--app-text-sm, 12px); color: var(--td-text-color-secondary); white-space: nowrap; }

.footer-bar {
  position: fixed;
  left: 232px;
  right: 0;
  bottom: 0;
  z-index: 20;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--app-space-sm, 10px) var(--app-space-lg, 20px);
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-stroke);
}
.footer-current { display: flex; align-items: center; gap: var(--app-space-xs, 6px); font-size: var(--app-text-sm, 12px); color: var(--td-text-color-secondary); }
.draft-kb-list { display: flex; flex-direction: column; gap: var(--app-space-xs, 4px); }
</style>
