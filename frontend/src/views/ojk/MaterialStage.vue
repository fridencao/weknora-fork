<template>
  <div class="cq">
    <!-- 标题行 -->
    <div class="cq-head">
      <div class="cq-head-main">
        <div class="cq-title">
          {{ $t('ojk.stage2.queueTitle') }}
          <t-tag theme="primary" variant="light" size="small">Fit &amp; Proper Mandate</t-tag>
        </div>
        <p class="cq-desc">{{ $t('ojk.stage2.queueDesc') }}</p>
      </div>
      <t-space>
        <t-button theme="default" @click="demo('templateZip')">
          <template #icon><t-icon name="cloud-download" /></template>
          {{ $t('ojk.stage2.templateZip') }}
        </t-button>
        <t-button theme="primary" @click="demo('batchIngest')">
          <template #icon><t-icon name="upload" /></template>
          {{ $t('ojk.stage2.batchIngest') }}
        </t-button>
      </t-space>
    </div>

    <!-- 摄入枢纽 + 吞吐面板 -->
    <div class="cq-hub">
      <div class="cq-drop" @click="demo('chooseLocal')">
        <t-icon name="upload" class="cq-drop-icon" />
        <div class="cq-drop-text">
          <div>
            {{ $t('ojk.stage2.dropTitle') }}
            <t-tag theme="primary" variant="light" size="small">OCR Auto-trigger</t-tag>
          </div>
          <div class="cq-drop-sub">{{ $t('ojk.stage2.dropSub') }}</div>
        </div>
        <t-button theme="default" @click.stop="demo('chooseLocal')">{{ $t('ojk.stage2.chooseLocal') }}</t-button>
      </div>
      <div class="cq-tp">
        <div class="cq-tp-head">
          <span>{{ $t('ojk.stage2.tpTitle') }}</span>
          <span class="cq-tp-online">● {{ $t('ojk.stage2.tpNodes', { n: 4 }) }}</span>
        </div>
        <div class="cq-tp-row">
          <span>{{ $t('ojk.stage2.tpThroughput') }}</span>
          <b>1,240 {{ $t('ojk.stage2.tpPagesUnit') }}</b>
        </div>
        <div class="cq-tp-row">
          <span>{{ $t('ojk.stage2.tpQueue') }}</span>
          <span>{{ $t('ojk.stage2.tpQueueVal') }}</span>
        </div>
        <div class="cq-tp-row">
          <span>GPU</span>
          <b>48%</b>
        </div>
      </div>
    </div>

    <!-- 工具条 -->
    <div class="cq-toolbar">
      <t-input v-model="search" clearable :placeholder="$t('ojk.stage2.searchCand')" style="width: 230px">
        <template #prefixIcon><t-icon name="search" /></template>
      </t-input>
      <t-radio-group v-model="statusTab" variant="default-filled" @change="page = 1">
        <t-radio-button value="">{{ $t('ojk.reviewWorkflow.tabAll', { n: candidates.length }) }}</t-radio-button>
        <t-radio-button value="parsed">{{ $t('ojk.stage2.tabParsed', { n: countBy('parsed') }) }}</t-radio-button>
        <t-radio-button value="parsing">{{ $t('ojk.stage2.tabParsing', { n: countBy('parsing') }) }}</t-radio-button>
        <t-radio-button value="queued">{{ $t('ojk.stage2.tabQueued', { n: countBy('queued') }) }}</t-radio-button>
      </t-radio-group>
      <t-select v-model="instFilter" clearable :placeholder="$t('ojk.stage2.instFilter')" style="width: 160px">
        <t-option v-for="inst in institutions" :key="inst" :value="inst" :label="inst" />
      </t-select>
      <span class="cq-checked">{{ $t('ojk.stage2.checkedN', { n: selected.size }) }}</span>
      <t-button theme="primary" style="margin-left: auto" :disabled="selected.size === 0" @click="demo('batchParse')">
        <template #icon><t-icon name="play-circle" /></template>
        {{ $t('ojk.stage2.execBatch') }}
      </t-button>
    </div>

    <!-- 候选人表格 -->
    <t-table
      :data="pagedCandidates"
      :columns="columns"
      row-key="id"
      :selected-row-keys="[...selected]"
      @select-change="onSelectChange"
      :pagination="pagination"
      @page-change="onPageChange"
      size="small"
    >
      <template #candidate="{ row }">
        <div class="cq-cand">
          <div class="cq-avatar" :class="{ sel: selected.has(row.id) }">{{ avatar(row.name) }}</div>
          <div>
            <div class="cq-cand-name">
              {{ row.name }}
              <t-icon v-if="row.status === 'parsed'" name="check-circle" theme="success" />
            </div>
            <div class="cq-cand-nik">NIK: {{ row.nik }}</div>
          </div>
        </div>
      </template>
      <template #position="{ row }">
        <div class="cq-pos">
          <div class="cq-pos-main">{{ row.position }}</div>
          <div class="cq-pos-sub">{{ row.positionSub }}</div>
        </div>
      </template>
      <template #institution="{ row }">
        <div class="cq-pos">
          <div class="cq-pos-main">{{ row.institution }}</div>
          <div class="cq-pos-sub">{{ row.instSub }}</div>
        </div>
      </template>
      <template #dossier="{ row }">
        <template v-if="row.status === 'parsed'">
          <t-tag theme="success" variant="light" size="small">
            ● {{ row.pages }}{{ $t('ojk.stage2.pagesShort') }} / {{ row.files }}{{ $t('ojk.stage2.filesShort') }} · {{ $t('ojk.stage2.parsedDone') }}
          </t-tag>
          <div class="cq-cell-sub">{{ row.statusNote }}</div>
        </template>
        <template v-else-if="row.status === 'parsing'">
          <t-tag theme="primary" variant="light" size="small">
            ⋛ {{ $t('ojk.stage2.parsingN', { pct: row.progress, pages: row.pages }) }}
          </t-tag>
          <div class="cq-progress"><t-progress :percentage="row.progress" :stroke-width="6" /></div>
        </template>
        <template v-else-if="row.status === 'queued'">
          <t-tag theme="default" variant="light" size="small">
            ● {{ row.pages }}{{ $t('ojk.stage2.pagesShort') }} · {{ $t('ojk.stage2.queuedNote') }}
          </t-tag>
          <div class="cq-cell-sub">{{ row.statusNote }}</div>
        </template>
        <template v-else>
          <t-tag theme="danger" variant="light" size="small">⚠ {{ $t('ojk.stage2.missingDoc') }}</t-tag>
          <div class="cq-cell-sub">{{ row.pages }}{{ $t('ojk.stage2.parsedPageShort') }} / 1{{ $t('ojk.stage2.missingOne') }}</div>
        </template>
      </template>
      <template #rules="{ row }">
        <t-tag variant="outline" size="small">{{ row.rules }} {{ $t('ojk.stage2.rulesCell') }}</t-tag>
      </template>
      <template #actions="{ row }">
        <t-space size="small">
          <t-button
            :theme="row.status === 'parsed' ? 'primary' : 'default'"
            size="small"
            @click="openDossier(row)"
          >
            {{ actionLabel(row) }}
          </t-button>
          <t-button variant="text" shape="square" size="small" @click="demo('rowMore')">
            <template #icon><t-icon name="ellipsis" /></template>
          </t-button>
        </t-space>
      </template>
    </t-table>

    <!-- 页脚汇总 -->
    <div class="cq-foot">
      <span>{{ $t('ojk.stage2.tableSummary', { from: 1, to: pagedCandidates.length, n: filteredCandidates.length }) }}</span>
      <span class="cq-foot-mid">{{ $t('ojk.stage2.totalParsed', { pages: totalPages, files: totalFiles }) }}</span>
      <t-space>
        <t-button variant="outline" size="small" disabled><template #icon><t-icon name="chevron-left" /></template></t-button>
        <t-button variant="outline" size="small" disabled>1</t-button>
        <t-button variant="outline" size="small" disabled><template #icon><t-icon name="chevron-right" /></template></t-button>
      </t-space>
    </div>

    <!-- 候选人卷宗滑窗 -->
    <DossierDrawer v-model:visible="drawerVisible" :candidate="drawerCandidate" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import DossierDrawer, { type DossierCandidate } from '@/views/ojk/DossierDrawer.vue'

const { t } = useI18n()
const route = useRoute()

interface CandidateRow extends DossierCandidate {
  id: string
  positionSub: string
  instSub: string
  status: 'parsed' | 'parsing' | 'queued' | 'incomplete'
  statusNote: string
  progress?: number
  updated: string
}

// ---- 原型演示数据：候选人批次卷宗（候选人/材料后端模型建设中）----
const candidates = ref<CandidateRow[]>([
  {
    id: 'rina', name: 'Rina Wijaya', nik: '3174055209840003',
    position: 'Direktur IT', positionSub: 'Fit & Proper Grade: Key Dir',
    institution: 'PT Bank Nusantara Digital Tbk', instSub: 'KBMI 2 · Digital Bank',
    status: 'parsed', statusNote: 'VLM 4.1 多模态要素 100% 就绪',
    pages: 158, files: 7, rules: 82, updated: t('ojk.stage2.upd10m'),
    ocrAvg: '99.4',
  },
  {
    id: 'bambang', name: 'Bambang Soedarmono', nik: '3273101504780001',
    position: 'Direktur Kepatuhan & Risiko', positionSub: 'Statutory Compliance Mandate',
    institution: 'PT Bank Mandiri (Persero) Tbk', instSub: 'KBMI 4 · SOE Commercial',
    status: 'parsing', statusNote: '', progress: 68,
    pages: 92, files: 63, rules: 78, updated: t('ojk.stage2.upd25m'),
  },
  {
    id: 'sri', name: 'Sri Mulyani P.', nik: '3171056209700004',
    position: 'Komisaris Utama (Independent)', positionSub: 'Governance Supervision',
    institution: 'PT Bank Central Asia Tbk', instSub: 'KBMI 4 · Private Commercial',
    status: 'queued', statusNote: t('ojk.stage2.hashVerified'),
    pages: 130, files: 8, rules: 64, updated: t('ojk.stage2.upd1h'),
  },
  {
    id: 'hendra', name: 'Hendra Gunawan', nik: '3578011203790008',
    position: 'Direktur Keuangan & Operasi', positionSub: 'CFO Mandate',
    institution: 'PT Bank Mega Tbk', instSub: 'KBMI 3 · Private Commercial',
    status: 'incomplete', statusNote: '',
    pages: 84, files: 6, rules: 80, updated: t('ojk.stage2.upd3h'),
  },
  {
    id: 'dewi', name: 'Dewi Lestari', nik: '3175024806820005',
    position: 'Komisaris Independen', positionSub: 'Independent Committee Chair',
    institution: 'PT Bank BTPN Syariah Tbk', instSub: 'Syariah Banking Mandate',
    status: 'parsed', statusNote: t('ojk.stage2.syariahReady'),
    pages: 112, files: 6, rules: 69, updated: t('ojk.stage2.updYesterday'),
  },
])

// ---- 筛选/选择/分页 ----
const search = ref('')
const statusTab = ref('')
const instFilter = ref('')
const page = ref(1)
const pageSize = 5
const selected = ref<Set<string>>(new Set(['rina']))

const institutions = computed(() => [...new Set(candidates.value.map(c => c.institution))])
function countBy(status: string) {
  return candidates.value.filter(c => c.status === status).length
}

const filteredCandidates = computed(() => {
  let list = candidates.value
  if (statusTab.value) list = list.filter(c => c.status === statusTab.value)
  if (instFilter.value) list = list.filter(c => c.institution === instFilter.value)
  const q = search.value.trim().toLowerCase()
  if (q) {
    list = list.filter(c =>
      [c.name, c.nik, c.institution, c.position].some(v => (v || '').toLowerCase().includes(q)))
  }
  return list
})

const pagedCandidates = computed(() => {
  const start = (page.value - 1) * pageSize
  return filteredCandidates.value.slice(start, start + pageSize)
})

const pagination = computed(() => ({
  current: page.value,
  pageSize,
  total: filteredCandidates.value.length,
}))

function onPageChange(p: { current?: number }) {
  page.value = p?.current ?? 1
}

function onSelectChange(keys: unknown[]) {
  selected.value = new Set((keys as string[]) || [])
}

const totalPages = computed(() => candidates.value.reduce((s, c) => s + c.pages, 0))
const totalFiles = computed(() => candidates.value.reduce((s, c) => s + c.files, 0))

// ---- 列定义 ----
const columns = [
  { colKey: 'row-select', width: 46 },
  { colKey: 'candidate', title: t('ojk.stage2.colCand'), minWidth: 210, cell: 'candidate' },
  { colKey: 'position', title: t('ojk.stage2.colPosition'), minWidth: 160, cell: 'position' },
  { colKey: 'institution', title: t('ojk.stage2.colInst'), minWidth: 160, cell: 'institution' },
  { colKey: 'dossier', title: t('ojk.stage2.colDossier'), minWidth: 220, cell: 'dossier' },
  { colKey: 'rules', title: t('ojk.stage2.colRules'), width: 100, cell: 'rules' },
  { colKey: 'updated', title: t('ojk.stage2.colUpdated'), width: 95 },
  { colKey: 'actions', title: t('ojk.stage2.colActions'), width: 160, cell: 'actions' },
]

// ---- 滑窗 ----
const drawerVisible = ref(false)
const drawerCandidate = ref<DossierCandidate | null>(null)

function openDossier(row: CandidateRow) {
  drawerCandidate.value = row
  drawerVisible.value = true
}

// 深链：?stage=2&open=rina 直开候选人卷宗滑窗
if (route.query.open) {
  const target = candidates.value.find(c => c.id === String(route.query.open))
  if (target) {
    drawerCandidate.value = target
    // TDesign Drawer 对"挂载时即 visible=true"不会渲染内容体——
    // 必须先挂载（visible=false）再在下一帧置 true 触发打开
    nextTick(() => { drawerVisible.value = true })
  }
}

function actionLabel(row: CandidateRow): string {
  if (row.status === 'parsed') return t('ojk.stage2.actView')
  if (row.status === 'parsing') return t('ojk.stage2.actConfig')
  if (row.status === 'queued') return t('ojk.stage2.actStart')
  return t('ojk.stage2.actRemediate')
}

function avatar(name: string): string {
  return name.split(/\s+/).map(w => w[0]).slice(0, 2).join('').toUpperCase()
}

// ---- 演示动作 ----
function demo(key: string) {
  MessagePlugin.info(t('ojk.stage2.demoAction', { action: t(`ojk.stage2.demo_${key}`) }))
}
</script>

<style scoped>
.cq { display: flex; flex-direction: column; gap: var(--app-space-md, 14px); }

.cq-head { display: flex; align-items: flex-start; gap: var(--app-space-md, 14px); }
.cq-head-main { flex: 1; }
.cq-title { font-size: var(--app-text-lg, 16px); font-weight: 700; display: flex; align-items: center; gap: 8px; }
.cq-desc { color: var(--td-text-color-secondary); margin: 4px 0 0; line-height: 1.5; }

/* 摄入枢纽 */
.cq-hub { display: grid; grid-template-columns: 1fr 320px; gap: var(--app-space-md, 14px); }
.cq-drop {
  display: flex; align-items: center; gap: var(--app-space-md, 14px);
  border: 1.5px dashed var(--td-brand-color);
  background: color-mix(in srgb, var(--td-brand-color) 4%, transparent);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-md, 14px) var(--app-space-lg, 18px);
  cursor: pointer;
}
.cq-drop-icon { font-size: 28px; color: var(--td-brand-color); }
.cq-drop-text { flex: 1; display: flex; flex-direction: column; gap: 4px; }
.cq-drop-sub { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); }
.cq-tp {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 12px);
  display: flex; flex-direction: column; gap: 6px;
  font-size: var(--app-text-xs, 11px);
}
.cq-tp-head { display: flex; justify-content: space-between; font-weight: 600; }
.cq-tp-online { color: var(--td-success-color); }
.cq-tp-row { display: flex; justify-content: space-between; color: var(--td-text-color-secondary); }

/* 工具条 */
.cq-toolbar { display: flex; align-items: center; gap: var(--app-space-sm, 10px); flex-wrap: wrap; }
.cq-checked { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); }

/* 表格单元格 */
.cq-cand { display: flex; align-items: center; gap: 10px; }
.cq-avatar {
  width: 36px; height: 36px; border-radius: 50%;
  background: color-mix(in srgb, var(--td-brand-color) 14%, transparent);
  color: var(--td-brand-color); font-weight: 700; font-size: var(--app-text-xs, 11px);
  display: flex; align-items: center; justify-content: center;
}
.cq-avatar.sel { background: var(--td-brand-color); color: #fff; }
.cq-cand-name { font-weight: 600; display: flex; align-items: center; gap: 4px; }
.cq-cand-nik { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); font-family: var(--td-font-family, monospace); }
.cq-pos-main { font-weight: 600; }
.cq-pos-sub { color: var(--td-text-color-placeholder); font-size: var(--app-text-xs, 11px); }
.cq-cell-sub { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); margin-top: 2px; }
.cq-progress { width: 150px; }

/* 页脚 */
.cq-foot {
  display: flex; align-items: center; gap: var(--app-space-md, 14px);
  color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px);
}
.cq-foot-mid { margin-right: auto; }
</style>
