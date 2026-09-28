<template>
  <div class="vf">
    <!-- 标题卡 -->
    <div class="vf-head">
      <div class="vf-head-main">
        <div class="vf-title">
          <t-tag theme="primary" variant="light" size="small">MODULE 03</t-tag>
          {{ $t('ojk.stage3.title') }}
        </div>
        <p class="vf-desc">{{ $t('ojk.stage3.desc') }}</p>
      </div>
      <t-space>
        <t-button theme="default" @click="demo('refresh')">
          <template #icon><t-icon name="refresh" /></template>
          {{ $t('ojk.stage3.refresh') }}
        </t-button>
        <t-button theme="default" @click="demo('pause')">
          <template #icon><t-icon name="pause-circle" /></template>
          {{ $t('ojk.stage3.pause') }}
        </t-button>
        <t-button theme="primary" @click="demo('batchRun')">
          <template #icon><t-icon name="play-circle" /></template>
          {{ $t('ojk.stage3.batchRun') }}
        </t-button>
      </t-space>
    </div>

    <!-- 四统计卡 -->
    <div class="vf-stats">
      <div class="vf-stat">
        <div class="vf-stat-head">
          <span>{{ $t('ojk.stage3.statProcessing') }}</span>
          <t-icon name="hourglass" />
        </div>
        <div class="vf-stat-num">2<span class="vf-stat-unit">/5</span></div>
        <div class="vf-stat-cap">{{ $t('ojk.stage3.statProcessingCap') }}</div>
        <t-progress :percentage="40" :stroke-width="6" />
        <div class="vf-stat-foot">● DOC AI + GraphRAG · {{ $t('ojk.stage3.parallelNodes', { n: 4 }) }}</div>
      </div>
      <div class="vf-stat">
        <div class="vf-stat-head">
          <span>{{ $t('ojk.stage3.statConflicts') }}</span>
          <t-icon name="error-triangle" theme="warning" />
        </div>
        <div class="vf-stat-num vf-stat-warn">6</div>
        <div class="vf-stat-tags">
          <t-tag theme="danger" variant="light" size="small">{{ $t('ojk.stage3.tagSevere', { n: 1 }) }}</t-tag>
          <t-tag theme="warning" variant="light" size="small">{{ $t('ojk.stage3.tagClarify', { n: 3 }) }}</t-tag>
          <t-tag theme="primary" variant="light" size="small">{{ $t('ojk.stage3.tagConcurrent', { n: 2 }) }}</t-tag>
        </div>
        <div class="vf-stat-foot">{{ $t('ojk.stage3.conflictSensitivity') }}</div>
      </div>
      <div class="vf-stat">
        <div class="vf-stat-head">
          <span>{{ $t('ojk.stage3.statCoverage') }}</span>
          <t-icon name="check-circle" theme="success" />
        </div>
        <div class="vf-stat-num vf-stat-ok">98.6%<span class="vf-stat-unit vf-stat-ok">+1.2%</span></div>
        <t-progress :percentage="98.6" :stroke-width="6" theme="success" />
        <div class="vf-stat-foot">{{ $t('ojk.stage3.coverageCap') }}</div>
      </div>
      <div class="vf-stat">
        <div class="vf-stat-head">
          <span>{{ $t('ojk.stage3.statAgents') }}</span>
          <t-icon name="api" theme="primary" />
        </div>
        <div class="vf-stat-num vf-stat-ok">3-Agent <span class="vf-stat-unit">Pipeline</span></div>
        <div class="vf-agent-row"><span>Rule Agent</span><span class="vf-stat-ok">100% {{ $t('ojk.stage3.agentCacheReady') }}</span></div>
        <div class="vf-agent-row"><span>Matching Agent</span><span>{{ $t('ojk.stage3.agentMatchingRun') }}</span></div>
        <div class="vf-agent-row vf-stat-warn"><span>Finding Agent</span><span>{{ $t('ojk.stage3.agentFinding', { n: 4 }) }}</span></div>
      </div>
    </div>

    <!-- 批次进度条 -->
    <div class="vf-batch">
      <t-icon name="sync" />
      <span>{{ $t('ojk.stage3.batchLabel') }}</span>
      <t-tag theme="primary" variant="light" size="small">
        {{ $t('ojk.stage3.batchProgress', { pct: 76, done: 194, total: 256 }) }}
      </t-tag>
      <span class="vf-batch-eta">{{ $t('ojk.stage3.batchEta', { time: '01m 15s' }) }}</span>
      <t-link theme="primary" size="small" @click="demo('liveLog')">{{ $t('ojk.stage3.liveLog') }}</t-link>
    </div>

    <!-- 筛选 tabs + 搜索 -->
    <div class="vf-toolbar">
      <t-radio-group v-model="verifyTab" variant="default-filled" @change="page = 1">
        <t-radio-button value="">{{ $t('ojk.stage3.tabAll', { n: rows.length }) }}</t-radio-button>
        <t-radio-button value="verifying">{{ $t('ojk.stage3.tabVerifying', { n: countBy('verifying') }) }}</t-radio-button>
        <t-radio-button value="findings">{{ $t('ojk.stage3.tabFindings', { n: countBy('findings') }) }}</t-radio-button>
        <t-radio-button value="blocked">{{ $t('ojk.stage3.tabBlocked', { n: countBy('blocked') }) }}</t-radio-button>
        <t-radio-button value="passed">{{ $t('ojk.stage3.tabPassed', { n: countBy('passed') }) }}</t-radio-button>
        <t-radio-button value="queued">{{ $t('ojk.stage3.tabQueued', { n: countBy('queued') }) }}</t-radio-button>
      </t-radio-group>
      <t-input v-model="search" clearable :placeholder="$t('ojk.stage3.searchCand')" style="width: 240px">
        <template #prefixIcon><t-icon name="search" /></template>
      </t-input>
      <t-button variant="outline" shape="square"><template #icon><t-icon name="filter" /></template></t-button>
    </div>

    <!-- 勾选操作条 -->
    <div class="vf-selectbar" v-if="selected.size > 0">
      <t-checkbox :checked="allSelected" @change="toggleAll">{{ $t('ojk.stage2.checkedN', { n: selected.size }) }}</t-checkbox>
      <span class="vf-selectbar-text">{{ $t('ojk.stage3.selectedHint', { names: selectedNames }) }}</span>
      <t-space style="margin-left: auto">
        <t-button theme="primary" @click="demo('batchVerify')">
          <template #icon><t-icon name="play-circle" /></template>
          {{ $t('ojk.stage3.runSelected') }}
        </t-button>
        <t-button theme="default" @click="demo('exportBrief')">
          <template #icon><t-icon name="download" /></template>
          {{ $t('ojk.stage3.exportBrief') }}
        </t-button>
        <t-button variant="text" theme="default" @click="clearSelection">{{ $t('ojk.stage3.clearSel') }}</t-button>
      </t-space>
    </div>

    <!-- 候选人核验表格 -->
    <t-table
      :data="pagedRows"
      :columns="columns"
      row-key="id"
      :pagination="pagination"
      @page-change="p => (page = p?.current ?? 1)"
      size="small"
    >
      <template #candidate="{ row }">
        <div class="vf-cand">
          <div class="vf-avatar">{{ avatar(row.name) }}</div>
          <div>
            <div class="vf-cand-name">
              {{ row.name }}
              <t-tag theme="primary" variant="light" size="small">{{ row.grade }}</t-tag>
            </div>
            <div class="vf-cand-nik">NIK: {{ row.nik }}</div>
            <div class="vf-cand-sub">{{ row.bio }}</div>
          </div>
        </div>
      </template>
      <template #position="{ row }">
        <div class="vf-cell">
          <div class="vf-cell-main">{{ row.position }}</div>
          <div class="vf-cell-sub">{{ row.institution }}</div>
          <t-tag variant="outline" size="small" style="margin-top: 4px">{{ row.regRef }}</t-tag>
          <div class="vf-cell-sub vf-hash-ok">✓ {{ $t('ojk.stage3.hashMatch') }}</div>
        </div>
      </template>
      <template #dossier="{ row }">
        <div class="vf-cell">
          <div class="vf-cell-main">{{ row.pages }} {{ $t('ojk.stage2.pagesShort') }} / {{ row.files }} {{ $t('ojk.stage2.filesShort') }}</div>
          <div class="vf-cell-sub">{{ row.rules }} {{ $t('ojk.stage3.rulesBase') }}</div>
          <t-tag theme="success" variant="light" size="small" style="margin-top: 4px">
            PDF {{ $t('ojk.stage3.parseDonePct', { pct: 100 }) }}
          </t-tag>
        </div>
      </template>
      <template #verify="{ row }">
        <template v-if="row.verify === 'verifying'">
          <t-tag theme="primary" variant="light" size="small">● {{ $t('ojk.stage3.verifyingN', { pct: row.progress }) }}</t-tag>
          <div class="vf-cell-sub">{{ $t('ojk.stage3.verifyingJudged', { done: row.judged, total: row.rules }) }}</div>
          <div class="vf-progress"><t-progress :percentage="row.progress" :stroke-width="6" theme="warning" /></div>
        </template>
        <template v-else-if="row.verify === 'findings'">
          <t-tag theme="warning" variant="light" size="small">⚠ {{ $t('ojk.stage3.doneClarify', { n: row.clarify }) }}</t-tag>
          <div class="vf-cell-sub">{{ $t('ojk.stage3.doneWithTime', { time: row.elapsed, done: row.rules }) }}</div>
          <div class="vf-progress"><t-progress :percentage="100" :stroke-width="6" theme="warning" /></div>
        </template>
        <template v-else-if="row.verify === 'passed'">
          <t-tag theme="success" variant="light" size="small">✓ {{ $t('ojk.stage3.passedAll') }}</t-tag>
          <div class="vf-cell-sub">{{ $t('ojk.stage3.doneWithTime', { time: row.elapsed, done: row.rules }) }}</div>
        </template>
        <template v-else-if="row.verify === 'blocked'">
          <t-tag theme="danger" variant="light" size="small">⚠ {{ $t('ojk.stage3.blockedMissing') }}</t-tag>
        </template>
        <template v-else>
          <t-tag theme="default" variant="light" size="small">{{ $t('ojk.stage3.queuedLabel') }}</t-tag>
        </template>
      </template>
      <template #findings="{ row }">
        <div class="vf-findings">
          <div v-for="(f, i) in row.findings" :key="i" class="vf-finding" :class="'vf-finding--' + f.theme">
            {{ f.text }}
          </div>
          <span v-if="!row.findings.length" class="vf-cell-sub">—</span>
        </div>
      </template>
      <template #reliability="{ row }">
        <div class="vf-rel">
          <div v-for="(r, i) in row.reliability" :key="i" class="vf-rel-row">
            <t-icon :name="r.icon" :theme="r.hot ? 'danger' : 'success'" size="12px" />
            <span>{{ r.label }}</span>
            <b v-if="r.value">{{ r.value }}</b>
          </div>
        </div>
      </template>
      <template #actions="{ row }">
        <div class="vf-actions">
          <t-button theme="primary" size="small" @click="$emit('open-review')">
            {{ $t('ojk.stage3.actWorkbench') }}
            <template #icon><t-icon name="arrow-right" /></template>
          </t-button>
          <t-link theme="default" size="small" @click="demo('reverify')">
            <template #icon><t-icon name="refresh" /></template>
            {{ $t('ojk.stage3.actReverify') }}
          </t-link>
          <t-link theme="default" size="small" @click="demo('detail')">{{ $t('ojk.stage3.actDetail') }}</t-link>
        </div>
      </template>
    </t-table>

    <!-- 底部导引 -->
    <div class="vf-guide">
      <t-icon name="bulb" theme="primary" />
      <div class="vf-guide-text">
        <b>{{ $t('ojk.stage3.guideTitle') }}</b>
        <span>{{ $t('ojk.stage3.guideBody') }}</span>
      </div>
      <t-button theme="primary" @click="$emit('open-review')">
        {{ $t('ojk.stage3.guideBtn') }}
        <template #icon><t-icon name="arrow-right" /></template>
      </t-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const emit = defineEmits<{ (e: 'prev'): void; (e: 'open-review'): void }>()

interface Finding { text: string; theme: 'danger' | 'warning' | 'primary' }
interface RelItem { label: string; value?: string; icon: string; hot?: boolean }

interface VerifyRow {
  id: string
  name: string
  grade: string
  nik: string
  bio: string
  position: string
  institution: string
  regRef: string
  pages: number
  files: number
  rules: number
  verify: 'verifying' | 'findings' | 'passed' | 'blocked' | 'queued'
  progress?: number
  judged?: number
  clarify?: number
  elapsed?: string
  findings: Finding[]
  reliability: RelItem[]
}

// ---- 原型演示数据（与阶段 2 候选人批次贯通；核验后端为阶段 3 课题）----
const rows = ref<VerifyRow[]>([
  {
    id: 'rina', name: 'Rina Wijaya', grade: 'KEY-DIR', nik: '3174055209840003',
    bio: t('ojk.stage3.bioRina'),
    position: 'Direktur IT (首席信息官)', institution: 'PT Bank Nusantara Digital Tbk',
    regRef: 'POJK.12/2023 Pasal 4-7', pages: 158, files: 7, rules: 82,
    verify: 'findings', progress: 100, clarify: 4, elapsed: '1m 42s',
    findings: [
      { text: t('ojk.stage3.findGap', { n: 8, pasal: 'Pasal 14' }), theme: 'danger' },
      { text: t('ojk.stage3.findConcurrent', { org: 'PT Fintech Mandiri' }), theme: 'warning' },
      { text: t('ojk.stage3.findSlik'), theme: 'primary' },
    ],
    reliability: [
      { label: 'Doc Match', value: '(96%)', icon: 'check-circle' },
      { label: 'PDF BBox', value: '(94%)', icon: 'check-circle' },
      { label: 'Cross-doc', icon: 'error-triangle', hot: true },
    ],
  },
  {
    id: 'bambang', name: 'Bambang Soedarmono', grade: 'RISK-DIR', nik: '3273111405780002',
    bio: t('ojk.stage3.bioBambang'),
    position: 'Direktur Kepatuhan & Risiko', institution: 'PT Bank Mandiri (Persero) Tbk',
    regRef: 'POJK.12/2023 Pasal 8-12', pages: 92, files: 5, rules: 78,
    verify: 'verifying', progress: 68, judged: 53,
    findings: [{ text: t('ojk.stage3.findDikti'), theme: 'warning' }],
    reliability: [
      { label: 'Matching Agent', icon: 'api' },
      { label: t('ojk.stage3.vectorSim'), value: '0.912', icon: 'chart' },
    ],
  },
  {
    id: 'hendra', name: 'Hendra Gunawan', grade: 'CFO', nik: '3578011203790008',
    bio: t('ojk.stage3.bioHendra'),
    position: 'Direktur Keuangan & Operasi', institution: 'PT Bank Mega Tbk',
    regRef: 'POJK.12/2023 Pasal 8-12', pages: 84, files: 5, rules: 80,
    verify: 'blocked', findings: [], reliability: [],
  },
  {
    id: 'dewi', name: 'Dewi Lestari', grade: 'IND-KOM', nik: '3175024806820005',
    bio: t('ojk.stage3.bioDewi'),
    position: 'Komisaris Independen', institution: 'PT Bank BTPN Syariah Tbk',
    regRef: 'POJK.12/2023 Pasal 15', pages: 112, files: 6, rules: 69,
    verify: 'passed', elapsed: '0m 58s',
    findings: [], reliability: [{ label: t('ojk.stage3.allChecksPass'), icon: 'check-circle' }],
  },
  {
    id: 'sri', name: 'Sri Mulyani P.', grade: 'IND-KOM', nik: '3171056209700004',
    bio: t('ojk.stage3.bioSri'),
    position: 'Komisaris Utama (Independent)', institution: 'PT Bank Central Asia Tbk',
    regRef: 'POJK.12/2023 Pasal 15', pages: 130, files: 8, rules: 64,
    verify: 'queued', findings: [], reliability: [],
  },
])

// ---- 筛选/分页/勾选 ----
const verifyTab = ref('')
const search = ref('')
const page = ref(1)
const pageSize = 5
const selected = ref<Set<string>>(new Set(['rina', 'hendra']))

function countBy(tab: string) {
  if (tab === 'verifying') return rows.value.filter(r => r.verify === 'verifying').length
  if (tab === 'findings') return rows.value.filter(r => r.verify === 'findings').length
  if (tab === 'blocked') return rows.value.filter(r => r.verify === 'blocked').length
  if (tab === 'passed') return rows.value.filter(r => r.verify === 'passed').length
  return rows.value.filter(r => r.verify === 'queued').length
}

const filteredRows = computed(() => {
  let list = rows.value
  if (verifyTab.value) {
    if (verifyTab.value === 'findings') list = list.filter(r => r.verify === 'findings' || r.findings.length > 0)
    else list = list.filter(r => r.verify === verifyTab.value)
  }
  const q = search.value.trim().toLowerCase()
  if (q) {
    list = list.filter(r =>
      [r.name, r.nik, r.institution, r.position].some(v => (v || '').toLowerCase().includes(q)))
  }
  return list
})

const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize
  return filteredRows.value.slice(start, start + pageSize)
})

const pagination = computed(() => ({
  current: page.value,
  pageSize,
  total: filteredRows.value.length,
}))

const selectedNames = computed(() =>
  rows.value.filter(r => selected.value.has(r.id)).map(r => r.name).join(', '))
const allSelected = computed(() =>
  pagedRows.value.length > 0 && pagedRows.value.every(r => selected.value.has(r.id)))

function toggleAll(v: unknown) {
  const on = !!v
  if (on) pagedRows.value.forEach(r => selected.value.add(r.id))
  else pagedRows.value.forEach(r => selected.value.delete(r.id))
  selected.value = new Set(selected.value)
}
function clearSelection() {
  selected.value = new Set()
}

const columns = [
  { colKey: 'row-select', width: 46 },
  { colKey: 'candidate', title: t('ojk.stage3.colCandidate'), minWidth: 220, cell: 'candidate' },
  { colKey: 'position', title: t('ojk.stage3.colPosition'), minWidth: 180, cell: 'position' },
  { colKey: 'dossier', title: t('ojk.stage3.colDossier'), width: 140, cell: 'dossier' },
  { colKey: 'verify', title: t('ojk.stage3.colVerify'), minWidth: 190, cell: 'verify' },
  { colKey: 'findings', title: t('ojk.stage3.colFindings'), minWidth: 190, cell: 'findings' },
  { colKey: 'reliability', title: t('ojk.stage3.colReliability'), width: 150, cell: 'reliability' },
  { colKey: 'actions', title: t('ojk.stage2.colActions'), width: 140, cell: 'actions' },
]

function avatar(name: string): string {
  return name.split(/\s+/).map(w => w[0]).slice(0, 2).join('').toUpperCase()
}

function demo(key: string) {
  MessagePlugin.info(t('ojk.stage3.demoAction', { action: t(`ojk.stage3.demo_${key}`) }))
}
</script>

<style scoped>
.vf { display: flex; flex-direction: column; gap: var(--app-space-md, 14px); }

.vf-head { display: flex; align-items: flex-start; gap: var(--app-space-md, 14px); }
.vf-head-main { flex: 1; }
.vf-title { font-size: var(--app-text-lg, 16px); font-weight: 700; display: flex; align-items: center; gap: 8px; }
.vf-desc { color: var(--td-text-color-secondary); margin: 6px 0 0; line-height: 1.5; }

.vf-stats { display: grid; grid-template-columns: repeat(4, 1fr); gap: var(--app-space-md, 14px); }
.vf-stat {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-md, 14px);
  display: flex; flex-direction: column; gap: 6px;
}
.vf-stat-head { display: flex; justify-content: space-between; align-items: center; color: var(--td-text-color-secondary); }
.vf-stat-num { font-size: 30px; font-weight: 700; line-height: 1.1; }
.vf-stat-ok { color: var(--td-success-color); }
.vf-stat-warn { color: var(--td-warning-color); }
.vf-stat-unit { font-size: var(--app-text-sm, 12px); font-weight: 400; margin-left: 4px; }
.vf-stat-cap { color: var(--td-text-color-placeholder); font-size: var(--app-text-xs, 11px); }
.vf-stat-tags { display: flex; gap: 4px; flex-wrap: wrap; }
.vf-stat-foot { color: var(--td-text-color-placeholder); font-size: var(--app-text-xs, 11px); }
.vf-agent-row { display: flex; justify-content: space-between; color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); }

.vf-batch {
  display: flex; align-items: center; gap: var(--app-space-sm, 10px);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 14px);
  color: var(--td-text-color-secondary);
  font-size: var(--app-text-sm, 12px);
  flex-wrap: wrap;
}
.vf-batch-eta { margin-left: auto; }

.vf-toolbar { display: flex; align-items: center; gap: var(--app-space-sm, 10px); flex-wrap: wrap; }

.vf-selectbar {
  display: flex; align-items: center; gap: var(--app-space-md, 14px);
  background: color-mix(in srgb, var(--td-brand-color) 6%, transparent);
  border: 1px solid color-mix(in srgb, var(--td-brand-color) 25%, transparent);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 14px);
  font-size: var(--app-text-sm, 12px);
}
.vf-selectbar-text { color: var(--td-text-color-secondary); }

.vf-cell { display: flex; flex-direction: column; gap: 3px; }
.vf-cell-main { font-weight: 600; }
.vf-cell-sub { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); }
.vf-hash-ok { color: var(--td-success-color); }
.vf-progress { width: 160px; }

.vf-cand { display: flex; align-items: flex-start; gap: 10px; }
.vf-avatar {
  width: 40px; height: 40px; border-radius: 50%;
  background: color-mix(in srgb, var(--td-brand-color) 14%, transparent);
  color: var(--td-brand-color); font-weight: 700;
  display: flex; align-items: center; justify-content: center; flex-shrink: 0;
}
.vf-cand-name { font-weight: 600; display: flex; align-items: center; gap: 4px; }
.vf-cand-nik { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); font-family: var(--td-font-family, monospace); }
.vf-cand-sub { color: var(--td-text-color-placeholder); font-size: var(--app-text-xs, 11px); }

.vf-findings { display: flex; flex-direction: column; gap: 4px; }
.vf-finding {
  padding: 4px 8px;
  border-radius: 4px;
  font-size: var(--app-text-xs, 11px);
  line-height: 1.4;
}
.vf-finding--danger { background: color-mix(in srgb, var(--td-error-color) 10%, transparent); color: var(--td-error-color); }
.vf-finding--warning { background: color-mix(in srgb, var(--td-warning-color) 12%, transparent); color: var(--td-warning-color); }
.vf-finding--primary { background: color-mix(in srgb, var(--td-brand-color) 10%, transparent); color: var(--td-brand-color); }

.vf-rel { display: flex; flex-direction: column; gap: 3px; }
.vf-rel-row { display: flex; align-items: center; gap: 4px; font-size: var(--app-text-xs, 11px); color: var(--td-text-color-secondary); }
.vf-rel-row b { color: var(--td-text-color-primary); }

.vf-actions { display: flex; flex-direction: column; gap: 4px; align-items: flex-start; }

.vf-guide {
  display: flex; align-items: center; gap: var(--app-space-md, 14px);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-md, 14px);
}
.vf-guide-text { flex: 1; display: flex; flex-direction: column; gap: 2px; }
.vf-guide-text b { font-size: var(--app-text-sm, 13px); }
.vf-guide-text span { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); line-height: 1.5; }
</style>
