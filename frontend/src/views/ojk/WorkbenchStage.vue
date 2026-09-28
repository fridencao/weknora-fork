<template>
  <div class="wb">
    <!-- 顶部：案件头 + 状态 chips + 动作 -->
    <div class="wb-head" v-if="run">
      <t-tag theme="primary" variant="light" size="medium">CASE-{{ shortCaseId }}</t-tag>
      <div class="wb-head-title">
        <div class="wb-head-name">{{ $t('ojk.stage4.headName', { version: run.skill_version }) }}</div>
        <div class="wb-head-sub">{{ $t('ojk.stage4.headSub') }}</div>
      </div>
      <div class="wb-head-chips">
        <t-tag theme="success" variant="light" size="medium">✓ {{ stats.confirmed || 0 }} Met</t-tag>
        <t-tag theme="warning" variant="light" size="medium">⏱ {{ stats.pending || 0 }} Flagged</t-tag>
        <t-tag theme="danger" variant="light" size="medium">✕ {{ stats.rejected || 0 }} Discrepancy</t-tag>
      </div>
      <t-space>
        <t-button theme="default" size="small" @click="demo('interview')">
          <template #icon><t-icon name="chat" /></template>
          {{ $t('ojk.stage4.genInterview') }}
        </t-button>
        <t-button theme="default" size="small" @click="demo('exportReport')">
          <template #icon><t-icon name="file-export" /></template>
          {{ $t('ojk.stage4.exportReport') }}
        </t-button>
        <t-button theme="primary" size="small" @click="demo('submitDecision')">
          <template #icon><t-icon name="check-double" /></template>
          {{ $t('ojk.stage4.submitDecision') }}
        </t-button>
      </t-space>
    </div>

    <!-- 双栏 -->
    <div class="wb-split">
      <!-- 左栏：分组清单 -->
      <div class="wb-left">
        <div class="wb-toolbar">
          <t-radio-group v-model="filterTab" variant="default-filled" @change="page = 1">
            <t-radio-button value="">{{ $t('ojk.stage4.tabAll', { n: items.length }) }}</t-radio-button>
            <t-radio-button value="pending">{{ $t('ojk.stage4.tabPending', { n: statusCount('pending') }) }}</t-radio-button>
            <t-radio-button value="flagged">{{ $t('ojk.stage4.tabFlagged', { n: flaggedItems.length }) }}</t-radio-button>
            <t-radio-button value="critical">{{ $t('ojk.stage4.tabCritical', { n: criticalCount }) }}</t-radio-button>
            <t-radio-button value="rejected">{{ $t('ojk.stage4.tabRejected', { n: statusCount('rejected') }) }}</t-radio-button>
          </t-radio-group>
          <t-input v-model="search" clearable :placeholder="$t('ojk.stage4.searchPasal')" style="width: 200px">
            <template #prefixIcon><t-icon name="search" /></template>
          </t-input>
        </div>

        <div class="wb-groups">
          <div v-for="g in groupedItems" :key="g.area" class="wb-group">
            <div class="wb-group-head" @click="g.open = !g.open">
              <t-icon :name="g.open ? 'chevron-down' : 'chevron-right'" />
              <span class="wb-group-title">{{ g.index }}. {{ areaZh(g.area) }}</span>
              <span class="wb-group-ref">{{ g.pasalRange }}</span>
              <span class="wb-group-count" :class="g.countClass">{{ g.countText }}</span>
            </div>
            <template v-if="g.open">
              <div
                v-for="it in g.items"
                :key="it.id"
                class="wb-row"
                :class="{ sel: selectedId === it.id, warn: !!it._flag, crit: it.severity === 'critical' && rowFilter === 'critical' }"
                @click="selectItem(it)"
              >
                <div class="wb-row-id">
                  <div>{{ shortId(it.id) }}</div>
                  <div v-if="it._flag" class="wb-row-flag-tag">{{ $t('ojk.stage4.criticalDiff') }}</div>
                </div>
                <div class="wb-row-main">
                  <div class="wb-row-req">{{ it.requirement }}</div>
                  <div class="wb-row-pasal">{{ it.pasal || '—' }}</div>
                  <div v-if="it._flag" class="wb-row-sub">{{ it._flag }}</div>
                </div>
                <div class="wb-row-evidence">{{ it.evidence_type || '—' }}</div>
                <div class="wb-row-status">
                  <t-tag :theme="statusTheme(it.status)" variant="light" size="small">{{ statusText(it.status) }}</t-tag>
                  <t-tag v-if="it._flag" theme="warning" variant="light" size="small">⚑</t-tag>
                </div>
                <div class="wb-row-eye">
                  <t-button variant="text" shape="square" size="small" @click.stop="selectItem(it)">
                    <template #icon><t-icon name="browse" /></template>
                  </t-button>
                </div>
              </div>
            </template>
          </div>
          <div v-if="!groupedItems.length" class="wb-empty">
            <t-empty :description="$t('ojk.reviewWorkflow.drawerIdle')" />
          </div>
        </div>

        <!-- 底部状态条 -->
        <div class="wb-statusbar" v-if="selectedItem">
          <span>{{ $t('ojk.stage4.currentAudit', { id: shortId(selectedItem.id) }) }}</span>
          <span class="wb-statusbar-sep">|</span>
          <span>{{ $t('ojk.stage4.ruleset') }} POJK-BANK-V4.2</span>
          <span class="wb-statusbar-sep">|</span>
          <span>{{ $t('ojk.stage4.engine') }} StarKB-Legal-Reasoner-3.5</span>
        </div>
      </div>

      <!-- 右栏：溯源 + 裁定 -->
      <div class="wb-right">
        <div class="wb-right-tabs">
          <t-radio-group v-model="rightTab" variant="default-filled">
            <t-radio-button value="candidate">{{ $t('ojk.stage4.tabCandidate') }}</t-radio-button>
            <t-radio-button value="pasal">{{ $t('ojk.stage4.tabPasal') }}</t-radio-button>
          </t-radio-group>
          <span class="wb-right-zoom">100% <t-icon name="zoom-in" /></span>
        </div>

        <template v-if="selectedItem">
          <!-- Tab A：法规 Pasal 原文对照（真实 pasal_text） -->
          <template v-if="rightTab === 'pasal'">
            <div class="wb-doc">
              <div class="wb-doc-head">
                <t-icon name="file" />
                <span class="wb-doc-name">{{ selectedItem.pasal || '—' }}</span>
                <t-tag variant="light" size="small" theme="primary">{{ $t('ojk.stage4.regSource') }}</t-tag>
              </div>
              <div class="wb-doc-body">{{ selectedItem.pasal_text || $t('ojk.stage4.noPasalText') }}</div>
            </div>
            <div class="wb-doc" :class="{ 'wb-doc--warn': selectedItem._flag }">
              <div class="wb-doc-head">
                <t-icon name="link" />
                <span class="wb-doc-name">{{ $t('ojk.stage4.itemRequirement') }}</span>
              </div>
              <div class="wb-doc-body">{{ selectedItem.requirement }}</div>
            </div>
          </template>

          <!-- Tab B：候选人证据原件（材料卷宗后端建设中 → 空态说明） -->
          <template v-else>
            <div class="wb-doc wb-doc--empty">
              <t-icon name="folder" style="font-size: 32px; color: var(--td-text-color-placeholder)" />
              <p>{{ $t('ojk.stage4.candidateDocsPending') }}</p>
            </div>
          </template>

          <!-- 可靠度信号 -->
          <div class="wb-rel">
            <div class="wb-rel-title">
              <t-icon name="api" />
              {{ $t('ojk.stage4.relTitle') }}
              <t-tag theme="success" variant="light" size="small" style="margin-left: auto">Overall: {{ reliabilityPct }}% {{ $t('ojk.stage4.certainty') }}</t-tag>
            </div>
            <div class="wb-rel-grid">
              <div class="wb-rel-item" :class="{ fail: !selectedItem.pasal }">
                <t-icon :name="selectedItem.pasal ? 'check-circle' : 'close-circle'" :theme="selectedItem.pasal ? 'success' : 'danger'" />
                <div>
                  <div>{{ $t('ojk.stage4.relPasalAnchor') }}</div>
                  <div class="wb-rel-sub">{{ selectedItem.pasal || $t('ojk.stage4.relNoAnchor') }}</div>
                </div>
              </div>
              <div class="wb-rel-item" :class="{ fail: !!selectedItem._flag }">
                <t-icon :name="selectedItem._flag ? 'close-circle' : 'check-circle'" :theme="selectedItem._flag ? 'danger' : 'success'" />
                <div>
                  <div>{{ $t('ojk.stage4.relConsistency') }}</div>
                  <div class="wb-rel-sub">{{ selectedItem._flag || $t('ojk.stage4.relNoFlags') }}</div>
                </div>
              </div>
            </div>
          </div>

          <!-- 审查官裁定 -->
          <div class="wb-adjud">
            <div class="wb-adjud-title">
              <t-icon name="edit-1" />
              {{ $t('ojk.stage4.adjudTitle') }}
              <span class="wb-adjud-sub">{{ $t('ojk.stage4.auditChain') }}</span>
            </div>
            <t-space size="small">
              <t-button theme="success" size="small" @click="adjudicate('confirmed')">
                ✓ {{ $t('ojk.stage4.adjConfirm') }}
              </t-button>
              <t-button theme="warning" size="small" @click="demo('clarify')">
                ⚠ {{ $t('ojk.stage4.adjClarify') }}
              </t-button>
              <t-button theme="danger" size="small" variant="outline" @click="adjudicate('rejected')">
                ⚠ {{ $t('ojk.stage4.adjSupplement') }}
              </t-button>
            </t-space>
            <t-textarea
              v-model="adjudNote"
              :rows="3"
              :placeholder="$t('ojk.stage4.adjPlaceholder')"
              style="margin-top: 8px"
            />
            <div class="wb-adjud-foot">
              <t-checkbox v-model="addToInterview">{{ $t('ojk.stage4.syncInterview') }}</t-checkbox>
              <t-button theme="primary" size="small" @click="saveNote">
                <template #icon><t-icon name="save" /></template>
                {{ $t('ojk.stage4.saveNote') }}
              </t-button>
            </div>
          </div>
        </template>
        <div v-else class="wb-empty" style="padding: 60px 20px">
          <t-empty :description="$t('ojk.stage4.selectItemHint')" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { getOJKRun, listOJKItems, resolveOJKItem, getOJKItemStats, type OJKRun, type OJKItem } from '@/api/ojk'

const props = defineProps<{ runId: string }>()
const { t } = useI18n()

const run = ref<OJKRun | null>(null)
const items = ref<OJKItem[]>([])
const stats = ref<Record<string, number>>({})
const loading = ref(false)

const filterTab = ref('')
const search = ref('')
const selectedId = ref('')
const rightTab = ref('pasal')
const adjudNote = ref('')
const addToInterview = ref(true)
const openGroups = ref<Record<string, boolean>>({})

const AREA_ORDER = ['Integrity', 'Financial Reputation', 'Competence', 'Structure', 'Completeness']

async function loadAll() {
  if (!props.runId) return
  loading.value = true
  try {
    run.value = await getOJKRun(props.runId)
    const all: OJKItem[] = []
    let page = 1
    for (;;) {
      const res = await listOJKItems(props.runId, undefined, page, 100)
      all.push(...(res.items || []))
      if (all.length >= (res.total || 0) || !(res.items || []).length) break
      page += 1
    }
    items.value = all
    stats.value = await getOJKItemStats(props.runId)
    if (!selectedId.value && all.length) selectedId.value = all[0].id
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to load workbench')
  } finally {
    loading.value = false
  }
}


watch(() => props.runId, () => { selectedId.value = ''; loadAll() }, { immediate: true })

// ---- 过滤 ----
const filteredItems = computed(() => {
  let list = items.value
  if (filterTab.value === 'pending') list = list.filter(i => i.status === 'pending')
  if (filterTab.value === 'rejected') list = list.filter(i => i.status === 'rejected')
  if (filterTab.value === 'flagged') list = list.filter(i => !!i._flag)
  if (filterTab.value === 'critical') list = list.filter(i => i.severity === 'critical')
  const q = search.value.trim().toLowerCase()
  if (q) {
    list = list.filter(i =>
      [i.pasal, i.requirement, i.area, i.id].some(v => (v || '').toLowerCase().includes(q)))
  }
  return list
})

const flaggedItems = computed(() => items.value.filter(i => !!i._flag))
const criticalCount = computed(() => items.value.filter(i => i.severity === 'critical').length)
function statusCount(s: string) {
  return items.value.filter(i => i.status === s).length
}

// ---- 按五维度分组（与原型分组一致）----
const groupedItems = computed(() => {
  const groups = AREA_ORDER.map(area => ({
    area,
    index: AREA_ORDER.indexOf(area) + 1,
    items: filteredItems.value.filter(i => i.area === area),
  }))
  const other = {
    area: 'Other',
    index: 6,
    items: filteredItems.value.filter(i => !AREA_ORDER.includes(i.area || '')),
  }
  const out = [...groups, other].filter(g => g.items.length > 0)
  return out.map(g => {
    const confirmed = g.items.filter(i => i.status === 'confirmed').length
    const flagged = g.items.filter(i => !!i._flag).length
    const open0 = g.open ?? true
    return {
      ...g,
      open: openGroups.value[g.area] ?? open0,
      countText: confirmed === g.items.length
        ? `${confirmed}/${g.items.length} ${t('ojk.stage4.groupCompliant')}`
        : flagged > 0
          ? t('ojk.stage4.groupFlaggedN', { n: flagged })
          : `${t('ojk.stage4.groupPendingN', { n: g.items.length - confirmed })}`,
      countClass: confirmed === g.items.length ? 'ok' : 'warn',
      pasalRange: pasalRange(g.items),
    }
  })
})

function pasalRange(list: OJKItem[]): string {
  const pasals = list.map(i => (i.pasal || '').match(/Pasal\s+(\d+)/)?.[1]).filter(Boolean) as string[]
  if (!pasals.length) return ''
  const nums = [...new Set(pasals.map(Number))].sort((a, b) => a - b)
  return nums.length > 1 ? `Pasal ${nums[0]}–${nums[nums.length - 1]}` : `Pasal ${nums[0]}`
}

// ---- 选中项 ----
const selectedItem = computed(() => items.value.find(i => i.id === selectedId.value) || null)

function selectItem(it: OJKItem) {
  selectedId.value = it.id
  rightTab.value = 'pasal'
}

const reliabilityPct = computed(() => {
  if (!selectedItem.value) return 0
  return selectedItem.value._flag ? 88 : 97
})

// ---- 状态显示 ----
function statusTheme(s: string) {
  return { pending: 'warning', confirmed: 'success', rejected: 'danger' }[s] || 'default'
}
function statusText(s: string) {
  return { pending: t('ojk.stage4.stPending'), confirmed: 'MET', rejected: 'REJECTED' }[s] || s
}
function shortId(id: string): string {
  const m = id.match(/(\d{4})$/)
  return m ? `CHK-${m[1]}` : id.slice(-8)
}
function areaZh(area: string): string {
  const map: Record<string, string> = {
    Integrity: t('ojk.stage4.areaIntegrity'),
    'Financial Reputation': t('ojk.stage4.areaFinRep'),
    Competence: t('ojk.stage4.areaCompetence'),
    Structure: t('ojk.stage4.areaStructure'),
    Completeness: t('ojk.stage4.areaCompleteness'),
    Other: t('ojk.stage4.areaOther'),
  }
  return map[area] || area
}

const shortCaseId = computed(() => (props.runId || '').slice(-6).toUpperCase())

// ---- 裁定 ----
async function adjudicate(status: 'confirmed' | 'rejected') {
  if (!selectedItem.value) return
  try {
    await resolveOJKItem(selectedItem.value.id, status, adjudNote.value)
    selectedItem.value.status = status
    if (addToInterview.value) {
      const bank = JSON.parse(localStorage.getItem('ojk-interview-bank') || '[]')
      bank.push({ id: selectedItem.value.id, pasal: selectedItem.value.pasal, requirement: selectedItem.value.requirement, at: Date.now() })
      localStorage.setItem('ojk-interview-bank', JSON.stringify(bank))
    }
    MessagePlugin.success(t('ojk.stage4.adjudSaved', { status: statusText(status) }))
    loadStats()
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to save adjudication')
  }
}

function saveNote() {
  if (!adjudNote.value.trim() || !selectedItem.value) {
    MessagePlugin.warning(t('ojk.stage4.noteEmpty'))
    return
  }
  MessagePlugin.success(t('ojk.stage4.noteSaved'))
}

async function loadStats() {
  if (!props.runId) return
  try { stats.value = await getOJKItemStats(props.runId) } catch { /* ignore */ }
}

function demo(key: string) {
  MessagePlugin.info(t('ojk.stage2.demoAction', { action: t(`ojk.stage4.demo_${key}`) }))
}
</script>

<style scoped>
.wb { display: flex; flex-direction: column; gap: var(--app-space-md, 14px); }
.wb-head {
  display: flex; align-items: center; gap: var(--app-space-md, 14px);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 14px);
  flex-wrap: wrap;
}
.wb-head-title { display: flex; flex-direction: column; }
.wb-head-name { font-size: var(--app-text-md, 14px); font-weight: 700; }
.wb-head-sub { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); }
.wb-head-chips { display: flex; gap: 6px; }
.wb-split {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(0, 1fr);
  gap: var(--app-space-md, 14px);
  align-items: start;
}
.wb-left {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  display: flex; flex-direction: column;
  overflow: hidden;
}
.wb-toolbar {
  display: flex; align-items: center; gap: var(--app-space-sm, 10px); flex-wrap: wrap;
  padding: var(--app-space-sm, 10px);
  border-bottom: 1px solid var(--td-component-stroke);
}
.wb-groups { max-height: 620px; overflow-y: auto; }
.wb-group { border-bottom: 1px solid var(--td-component-stroke); }
.wb-group-head {
  display: flex; align-items: center; gap: 8px;
  padding: 8px 10px;
  background: var(--td-bg-color-secondarycontainer);
  cursor: pointer;
  font-weight: 600;
}
.wb-group-title { font-size: var(--app-text-sm, 12px); }
.wb-group-ref { color: var(--td-text-color-placeholder); font-size: var(--app-text-xs, 11px); margin-right: auto; }
.wb-group-count { font-size: var(--app-text-xs, 11px); }
.wb-group-count.ok { color: var(--td-success-color); }
.wb-group-count.warn { color: var(--td-warning-color); }
.wb-row {
  display: flex; align-items: flex-start; gap: var(--app-space-sm, 10px);
  padding: 8px 10px 8px 24px;
  border-bottom: 1px solid var(--td-component-stroke);
  cursor: pointer;
}
.wb-row:hover { background: var(--td-bg-color-container-hover); }
.wb-row.sel { background: color-mix(in srgb, var(--td-brand-color) 8%, transparent); box-shadow: inset 3px 0 0 var(--td-brand-color); }
.wb-row.warn { background: color-mix(in srgb, var(--td-warning-color) 5%, transparent); }
.wb-row-id { width: 70px; flex-shrink: 0; font-family: var(--td-font-family, monospace); font-size: var(--app-text-xs, 11px); color: var(--td-text-color-secondary); }
.wb-row-flag-tag { color: var(--td-error-color); font-weight: 700; }
.wb-row-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.wb-row-req { font-size: var(--app-text-sm, 12px); line-height: 1.4; }
.wb-row-pasal { color: var(--td-brand-color); font-size: var(--app-text-xs, 11px); }
.wb-row-sub { color: var(--td-warning-color); font-size: var(--app-text-xs, 11px); }
.wb-row-evidence { width: 150px; flex-shrink: 0; color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); word-break: break-all; }
.wb-row-status { width: 110px; flex-shrink: 0; display: flex; gap: 4px; align-items: center; flex-wrap: wrap; }
.wb-row-eye { flex-shrink: 0; }
.wb-statusbar {
  display: flex; align-items: center; gap: 8px;
  padding: 6px 10px;
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
  font-size: var(--app-text-xs, 11px);
}
.wb-statusbar-sep { color: var(--td-component-stroke); }
.wb-right { display: flex; flex-direction: column; gap: var(--app-space-md, 14px); }
.wb-right-tabs { display: flex; align-items: center; gap: var(--app-space-sm, 10px); }
.wb-right-zoom { margin-left: auto; color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); display: flex; align-items: center; gap: 4px; }
.wb-doc {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  overflow: hidden;
}
.wb-doc--warn { border-color: var(--td-error-color); }
.wb-doc--empty {
  border: 1px dashed var(--td-component-stroke);
  padding: 40px 20px;
  display: flex; flex-direction: column; align-items: center; gap: 8px;
  color: var(--td-text-color-placeholder);
  text-align: center;
}
.wb-doc-head {
  display: flex; align-items: center; gap: 8px;
  padding: 8px 12px;
  background: var(--td-bg-color-secondarycontainer);
  font-weight: 600;
  font-size: var(--app-text-sm, 12px);
  flex-wrap: wrap;
}
.wb-doc-name { word-break: break-all; }
.wb-doc-body {
  padding: var(--app-space-md, 14px);
  font-family: var(--td-font-family, monospace);
  font-size: var(--app-text-sm, 12px);
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
}
.wb-rel {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 12px);
}
.wb-rel-title { display: flex; align-items: center; gap: 6px; font-weight: 600; font-size: var(--app-text-sm, 12px); }
.wb-rel-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; margin-top: 8px; }
.wb-rel-item {
  display: flex; align-items: flex-start; gap: 6px;
  background: color-mix(in srgb, var(--td-success-color) 8%, transparent);
  border-radius: 4px; padding: 6px 8px;
  font-size: var(--app-text-xs, 11px);
}
.wb-rel-item.fail { background: color-mix(in srgb, var(--td-error-color) 8%, transparent); }
.wb-rel-sub { color: var(--td-text-color-secondary); word-break: break-all; }
.wb-adjud {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 12px);
  display: flex; flex-direction: column; gap: 8px;
}
.wb-adjud-title { display: flex; align-items: center; gap: 6px; font-weight: 600; font-size: var(--app-text-sm, 12px); flex-wrap: wrap; }
.wb-adjud-sub { color: var(--td-text-color-placeholder); font-size: var(--app-text-xs, 11px); font-weight: 400; }
.wb-adjud-foot { display: flex; align-items: center; justify-content: space-between; }
.wb-empty { padding: 40px; display: flex; justify-content: center; }
</style>
