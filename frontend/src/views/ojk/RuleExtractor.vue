<template>
  <div class="ojk-stage1">
    <!-- 四阶段 Tab 导航（原型样式：编号圆点 + 选中态下划线） -->
    <nav class="stage-tabs">
      <div class="stage-crumb">StarKB / {{ $t('menu.ojk') }}</div>
      <div class="stage-tab" :class="{ active: activeStage === 1 }" @click="activeStage = 1">
        <span class="stage-num">1</span>{{ $t('ojk.stage1.stageKb') }}
      </div>
      <div class="stage-tab" :class="{ active: activeStage === 2 }" @click="activeStage = 2">
        <span class="stage-num">2</span>{{ $t('ojk.stage1.stageMaterial') }}
      </div>
      <div class="stage-tab" :class="{ active: activeStage === 3 }" @click="activeStage = 3">
        <span class="stage-num">3</span>{{ $t('ojk.stage1.stageAiCheck') }}
      </div>
      <div
        class="stage-tab"
        :class="{ active: activeStage === 4, disabled: !currentDoneRun }"
        @click="currentDoneRun && (activeStage = 4)"
      >
        <span class="stage-num">4</span>{{ $t('ojk.stage1.stageWorkbench') }}
      </div>
      <div class="stage-tab" :class="{ active: activeStage === 5, disabled: !currentDoneRun }" @click="currentDoneRun && (activeStage = 5)">
        <span class="stage-num">5</span>{{ $t('ojk.stage1.stageReport') }}
      </div>
    </nav>

    <template v-if="activeStage === 1">
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
        <t-tooltip
          :content="$t('ojk.stage1.draftLocked', { done: stagingRun?.slices_done ?? 0, total: stagingRun?.slices_total ?? 0 })"
          :disabled="!stagingRun"
        >
          <t-button theme="primary" :disabled="!!stagingRun" @click="openDraft">
            {{ $t('ojk.stage1.newDraft') }}
          </t-button>
        </t-tooltip>
      </template>
      <t-loading :loading="runsLoading">
        <t-row :gutter="16" class="ver-row">
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
                <div v-if="dimSummary.length" class="ver-dims">
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
        <div v-if="doneRunsHistory.length" class="ver-history">
          <span class="ver-history-title">{{ $t('ojk.stage1.versionHistory') }}</span>
          <t-tag
            v-for="r in doneRunsHistory"
            :key="r.run_id"
            :theme="viewRunId === r.run_id ? 'primary' : 'default'"
            variant="light"
            size="medium"
            class="ver-history-chip"
            @click="viewRunId = r.run_id"
          >
            v{{ r.skill_version }}
          </t-tag>
          <t-button size="small" variant="text" theme="default" class="ver-history-manage" @click="manageVisible = true">
            <template #icon><t-icon name="setting" /></template>
            {{ $t('ojk.stage1.manageVersions') }}
          </t-button>
        </div>
      </t-loading>
    </t-card>

    <!-- 清单明细 -->
    <t-card :bordered="false" class="block">
      <template #title>
        <div class="store-title">
          <t-icon name="view-list" />
          <span>{{ $t('ojk.stage1.detailTitle') }}</span>
          <t-tag v-if="viewedRun" theme="primary" variant="light" size="small">
            v{{ viewedRun.skill_version }} · {{ formatTime(viewedRun.updated_at) }} · {{ $t('ojk.stage1.itemCountN', { n: viewedRun.total_items }) }}
          </t-tag>
          <t-tag
            v-if="viewedRun && currentDoneRun && viewedRun.run_id !== currentDoneRun.run_id"
            theme="warning" variant="light" size="small"
          >
            {{ $t('ojk.stage1.viewingHistory') }}
          </t-tag>
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
        <!-- data 传全量：排序/筛选要作用整个版本，分页由 TDesign 内部切片 -->
        <t-table
          :data="filteredItems"
          :columns="itemColumns"
          row-key="id"
          :pagination="itemPagination"
          @page-change="onItemPage"
          @filter-change="onItemFilterChange"
          @sort-change="onItemSortChange"
        >
          <template #dim="{ row }">
            <t-tag v-if="row.area" theme="primary" variant="light" size="small">{{ row.area }}</t-tag>
            <span v-else>-</span>
          </template>
          <template #severity="{ row }">
            <t-tag :theme="row.severity === 'critical' ? 'danger' : (row.severity === 'clarification' ? 'warning' : 'default')" variant="light" size="small">
              {{ row.severity }}
            </t-tag>
          </template>
          <template #status="{ row }">
            <t-tag :theme="itemStatusTheme(row.status)" variant="light" size="small">
              {{ itemStatusLabel(row.status) }}
            </t-tag>
          </template>
          <template #action="{ row }">
            <t-button size="small" theme="default" variant="text" @click="goReview(row.run_id)">
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

    <!-- 版本管理对话框：浏览 / 评审 / 重命名 / 删除 -->
    <t-dialog
      v-model:visible="manageVisible"
      :header="$t('ojk.stage1.manageTitle')"
      :footer="false"
      width="760px"
    >
      <t-table :data="doneRunsHistory" :columns="manageColumns" row-key="run_id" size="medium">
        <template #mVersion="{ row }">
          <t-space size="small">
            <t-tag v-if="row.run_id === currentDoneRun?.run_id" theme="primary" variant="light" size="small">
              {{ $t('ojk.stage1.currentProd') }}
            </t-tag>
            <span>v{{ row.skill_version }}</span>
          </t-space>
        </template>
        <template #mTime="{ row }">{{ formatTime(row.updated_at) }}</template>
        <template #mOp="{ row }">
          <t-space size="small">
            <t-link theme="primary" @click="viewRunId = row.run_id; manageVisible = false">
              {{ $t('ojk.stage1.opBrowse') }}
            </t-link>
            <t-link theme="default" @click="manageVisible = false; goReview(row.run_id)">{{ $t('ojk.stage1.opReview') }}</t-link>
            <t-link theme="warning" @click="openRename(row)">{{ $t('ojk.stage1.opRename') }}</t-link>
            <t-link
              v-if="row.status === 'running' || row.status === 'pending'"
              theme="danger" :disabled="true"
            >
              {{ $t('ojk.stage1.opDelete') }}
            </t-link>
            <t-popconfirm
              v-else
              :content="$t('ojk.stage1.deleteConfirm', { items: row.total_items })"
              @confirm="removeRun(row)"
            >
              <t-link theme="danger">{{ $t('ojk.stage1.opDelete') }}</t-link>
            </t-popconfirm>
          </t-space>
        </template>
      </t-table>
    </t-dialog>

    <!-- 重命名版本对话框 -->
    <t-dialog
      v-model:visible="renameVisible"
      :header="$t('ojk.stage1.renameTitle')"
      :confirm-btn="{ content: $t('ojk.stage1.renameConfirm'), loading: renaming }"
      @confirm="submitRename"
    >
      <t-form layout="vertical">
        <t-form-item :label="$t('ojk.stage1.renameLabel')">
          <t-input v-model="renameValue" :placeholder="$t('ojk.stage1.renamePlaceholder')" maxlength="32" />
        </t-form-item>
      </t-form>
    </t-dialog>

    </template>

    <!-- 阶段 2：材料摄入与规则配置 -->
    <MaterialStage
      v-else-if="activeStage === 2"
      :kb-name="regKbMeta?.kb_name ?? '—'"
      :sections="regKbMeta?.pasal_sections ?? 0"
      :docs="regKbMeta?.docs ?? 0"
      :titles="regKbMeta?.titles ?? []"
      @prev="activeStage = 1"
    />

    <!-- 阶段 3：智能核验调度与候选人比对中心 -->
    <VerificationStage
      v-else-if="activeStage === 3"
      @open-review="openWorkbench"
    />

    <!-- 阶段 4：审查工作台与溯源（分组清单 + Pasal 原文对照 + 裁定） -->
    <WorkbenchStage
      v-else-if="activeStage === 4 && currentDoneRun"
      :run-id="currentDoneRun.run_id"
    />

    <!-- 阶段 5：成果交付与双人复核 -->
    <DeliverableStage
      v-else-if="activeStage === 5 && currentDoneRun"
      :run-id="currentDoneRun.run_id"
    />
    <!-- 溯源审核右侧滑窗 -->
    <ReviewDrawer
      v-model:visible="reviewVisible"
      :run-id="reviewRunId"
      @close="onReviewClosed"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { listKnowledgeBases } from '@/api/knowledge-base'
import ReviewDrawer from '@/views/ojk/ReviewDrawer.vue'
import MaterialStage from '@/views/ojk/MaterialStage.vue'
import VerificationStage from '@/views/ojk/VerificationStage.vue'
import WorkbenchStage from '@/views/ojk/WorkbenchStage.vue'
import DeliverableStage from '@/views/ojk/DeliverableStage.vue'
import {
  createOJKRun, getOJKRun, listOJKRuns, listOJKItems, preflightOJK,
  renameOJKRun, deleteOJKRun,
  type OJKRun, type OJKItem, type OJKPreflight,
} from '@/api/ojk'

const { t } = useI18n()
const route = useRoute()
// 阶段深链：?stage=2 直达材料摄入与规则配置
const activeStage = ref([2, 3, 4, 5].includes(Number(route.query.stage)) ? Number(route.query.stage) : 1)
// 阶段 3「进入审查工作台」→ 右侧滑窗打开最新定版的清单复核
function openWorkbench() {
  if (!currentDoneRun.value) return
  reviewRunId.value = currentDoneRun.value.run_id
  reviewVisible.value = true
}

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
const doneRunsHistory = computed(() => runs.value.filter(r => r.status === 'done'))
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
const viewedRun = computed(() => runs.value.find(r => r.run_id === viewRunId.value) || null)
const items = ref<OJKItem[]>([])
const itemsLoading = ref(false)
const itemSearch = ref('')
const itemPage = ref(1)
const PAGE_SIZE = 20
// 列头筛选（severity/status）：TDesign 只渲染筛选 UI，谓词由我们接进数据链
const severityFilter = ref<string[]>([])
const statusFilter = ref<string[]>([])
// 列头排序：sorter:true 只出 UI（远程排序模式），排序由我们应用在数据上，
// 不依赖 TDesign 内部排序与内部分页的配合
const itemSort = ref<{ sortBy: string; descending: boolean } | null>(null)

const filteredItems = computed(() => {
  let list = items.value
  if (severityFilter.value.length) {
    list = list.filter(it => severityFilter.value.includes(it.severity))
  }
  if (statusFilter.value.length) {
    list = list.filter(it => statusFilter.value.includes(it.status))
  }
  const { sortBy, descending } = itemSort.value || {}
  if (sortBy) {
    const rank = sortBy === 'severity' ? severityRank : statusRank
    list = [...list].sort((a, b) => {
      const r = (rank[a[sortBy]] ?? 9) - (rank[b[sortBy]] ?? 9)
      return descending ? -r : r
    })
  }
  const q = itemSearch.value.trim().toLowerCase()
  if (!q) return list
  return list.filter(it =>
    [it.pasal, it.requirement, it.area, it.regulation, it.id]
      .some(v => (v || '').toLowerCase().includes(q)))
})

// 分页切片由 TDesign 内部完成（data 长度 > pageSize 时自动启用），
// 这里只维护受控的当前页码
const itemPagination = computed(() => ({
  current: itemPage.value,
  pageSize: PAGE_SIZE,
  total: filteredItems.value.length,
  showJumper: true,
}))

// TDesign page-change 回调传的是 { current, previous, pageSize } 对象，
// 不是页码数字——之前当数字赋给 itemPage，切片算出 NaN 导致翻页后表格永远为空
function onItemPage(pageInfo: { current?: number }) {
  itemPage.value = pageInfo?.current ?? 1
}

// filter-change 回调传整个 filterValue 映射 { colKey: 选中值数组 }
function onItemFilterChange(filterValue: Record<string, string[]>) {
  severityFilter.value = filterValue?.severity || []
  statusFilter.value = filterValue?.status || []
  itemPage.value = 1
}

// sort-change 回调传 { sortBy, descending }；取消排序时 sortBy 为 undefined
function onItemSortChange(sort: { sortBy?: string; descending?: boolean } | undefined) {
  itemSort.value = sort?.sortBy
    ? { sortBy: sort.sortBy, descending: !!sort.descending }
    : null
  itemPage.value = 1
}

watch(viewRunId, () => { itemPage.value = 1; loadItems() })
watch(itemSearch, () => { itemPage.value = 1 })

const dimSummary = computed(() => {
  const counts = new Map<string, number>()
  for (const it of items.value) {
    // 空维度（"无锚定维度"的条目）不进分布：只有真实维度才有展示价值，
    // 否则 area 全空时会退化成孤零零的 "— · N"（与正文条数重复）
    if (!it.area) continue
    counts.set(it.area, (counts.get(it.area) || 0) + 1)
  }
  return [...counts.entries()]
    .map(([area, count]) => ({ area, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 5)
})

// 严重程度/状态的业务排序（表头 sort 图标触发，TDesign 本地排序在分页前生效）
const severityRank: Record<string, number> = { critical: 0, clarification: 1, info: 2 }
const statusRank: Record<string, number> = { pending: 0, confirmed: 1, rejected: 2 }

const itemColumns = [
  { colKey: 'id', title: t('ojk.stage1.colId'), width: 150 },
  { colKey: 'area', title: t('ojk.stage1.colDim'), width: 130, cell: 'dim' },
  { colKey: 'pasal', title: t('ojk.stage1.colAnchor'), width: 200 },
  { colKey: 'requirement', title: t('ojk.stage1.colCriteria'), ellipsis: true },
  { colKey: 'evidence_type', title: t('ojk.stage1.colEvidence'), width: 160 },
  {
    colKey: 'severity', title: t('ojk.stage1.colSeverity'), width: 130, cell: 'severity',
    sorter: true,
    filter: {
      list: ['critical', 'clarification', 'info'].map(v => ({ label: v, value: v })),
      type: 'multiple',
    },
  },
  {
    colKey: 'status', title: t('ojk.stage1.colStatus'), width: 130, cell: 'status',
    sorter: true,
    filter: {
      list: ['pending', 'confirmed', 'rejected'].map(v => ({ label: itemStatusLabel(v), value: v })),
      type: 'multiple',
    },
  },
  { colKey: 'action', title: t('ojk.stage1.colAction'), width: 100, cell: 'action' },
]

function itemStatusTheme(status: string): string {
  const map: Record<string, string> = {
    pending: 'warning', confirmed: 'success', rejected: 'danger',
  }
  return map[status] || 'default'
}

function itemStatusLabel(status: string): string {
  // 条目级状态（pending/confirmed/rejected）与 run 级状态字典分开
  return t(`ojk.itemStatus.${status}`)
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
    if (e?.status === 409 || /already active/i.test(e?.error || e?.message || '')) {
      MessagePlugin.warning(t('ojk.stage1.runActiveMsg'))
    } else {
      MessagePlugin.error(e?.error || e?.message || 'Failed to create run')
    }
  } finally {
    creating.value = false
  }
}

// ---- 版本管理（浏览/评审/重命名/删除）----
// ---- 溯源审核右侧滑窗 ----
const reviewVisible = ref(false)
const reviewRunId = ref('')
function onReviewClosed() {
  // 复核可能改动了条目状态，关闭时刷新明细
  loadItems()
}

const manageVisible = ref(false)
const renameVisible = ref(false)
const renameTarget = ref<OJKRun | null>(null)
const renameValue = ref('')
const renaming = ref(false)
const deletingId = ref('')

const manageColumns = [
  { colKey: 'skill_version', title: t('ojk.stage1.colVersion'), cell: 'mVersion', width: 200 },
  { colKey: 'updated_at', title: t('ojk.stage1.colSignedAt'), cell: 'mTime', width: 170 },
  { colKey: 'total_items', title: t('ojk.stage1.colItems'), width: 100 },
  { colKey: 'op', title: t('ojk.stage1.colOp'), cell: 'mOp' },
]

function openRename(run: OJKRun) {
  renameTarget.value = run
  renameValue.value = run.skill_version
  renameVisible.value = true
}

async function submitRename() {
  if (!renameTarget.value) return
  renaming.value = true
  try {
    const updated = await renameOJKRun(renameTarget.value.run_id, renameValue.value)
    const idx = runs.value.findIndex(r => r.run_id === updated.run_id)
    if (idx >= 0) runs.value[idx] = updated
    MessagePlugin.success(t('ojk.stage1.renamedOk'))
    renameVisible.value = false
  } catch (e: any) {
    MessagePlugin.error(e?.error || e?.message || 'Failed to rename version')
  } finally {
    renaming.value = false
  }
}

async function removeRun(run: OJKRun) {
  deletingId.value = run.run_id
  try {
    await deleteOJKRun(run.run_id)
    runs.value = runs.value.filter(r => r.run_id !== run.run_id)
    // 删除的是正在查看的版本 → 回退到最新定版（watch 触发明细重载）
    if (viewRunId.value === run.run_id) {
      viewRunId.value = currentDoneRun.value?.run_id || ''
      if (!viewRunId.value) { items.value = []; loadItems() }
    }
    MessagePlugin.success(t('ojk.stage1.deletedOk'))
  } catch (e: any) {
    MessagePlugin.error(e?.error || e?.message || 'Failed to delete version')
  } finally {
    deletingId.value = ''
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

function stageTodo() {
  MessagePlugin.info(t('ojk.stage1.nextStageTip'))
}

function goReview(runId?: string) {
  if (!runId) return
  // 溯源审核改为右侧滑窗（不再整页跳转）
  reviewRunId.value = runId
  reviewVisible.value = true
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
  /* 父级 .platform-route-outlet 是 flex 列 + overflow:hidden（滚动交给页面自己）。
     页面根必须占满并自滚动，否则首屏之外的内容（明细表的行）永远滚不出来。 */
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  /* 卡片保持自然高度溢出滚动，禁止被 flex 压扁 */
  > * {
    flex-shrink: 0;
  }
}

/* 四阶段 Tab 栏（原型样式）：负 margin 出血到容器两侧，贴住页面顶 */
.stage-tabs {
  display: flex;
  align-items: stretch;
  margin: calc(-1 * var(--app-space-md, 16px)) calc(-1 * var(--app-space-md, 16px)) 0;
  padding: 0 var(--app-space-lg, 20px);
  background: var(--td-bg-color-container);
  border-bottom: 1px solid var(--td-component-stroke);
}
.stage-crumb {
  display: flex;
  align-items: center;
  padding: 0 var(--app-space-lg, 20px) 0 0;
  margin-right: var(--app-space-md, 12px);
  border-right: 1px solid var(--td-component-stroke);
  font-weight: 600;
  font-size: var(--app-text-base, 14px);
  white-space: nowrap;
}
.stage-tab {
  display: flex;
  align-items: center;
  gap: var(--app-space-xs, 6px);
  padding: 14px 18px;
  font-size: var(--app-text-base, 14px);
  color: var(--td-text-color-secondary);
  border-bottom: 2px solid transparent;
  cursor: pointer;
  white-space: nowrap;
  user-select: none;
}
.stage-tab.active {
  color: var(--td-brand-color);
  font-weight: 600;
  border-bottom-color: var(--td-brand-color);
}
.stage-tab.todo {
  color: var(--td-text-color-placeholder);
}
.stage-tab.disabled {
  color: var(--td-text-color-placeholder);
  cursor: not-allowed;
}
.stage-num {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  font-size: var(--app-text-xs, 11px);
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
}
.stage-tab.active .stage-num {
  background: var(--td-brand-color);
  color: #fff;
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
/* TDesign 行默认 align-top：两列不等高。覆盖为 stretch，让右卡跟随左卡高度；
   列设为 flex、卡片 flex:1 填充（height:100% 在这层嵌套里会溢出压住历史行） */
.ver-row {
  align-items: stretch;
}
.ver-row .t-col {
  display: flex;
}
.ver-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 12px);
  cursor: pointer;
  /* 跟随同行另一张卡的高度（左卡内容多，右卡等高拉伸），内容垂直居中 */
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
}
/* 压缩空态占位（默认 empty 图 + 文案会把卡片撑到 ~270px 高） */
.ver-card :deep(.t-empty) {
  padding: 10px 0;
}
.ver-card :deep(.t-empty__image) {
  display: none;
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
  position: sticky;
  bottom: calc(-1 * var(--app-space-md, 16px));
  z-index: 20;
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 0 calc(-1 * var(--app-space-md, 16px)) calc(-1 * var(--app-space-md, 16px));
  padding: var(--app-space-sm, 10px) var(--app-space-lg, 20px);
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-stroke);
}
.ver-history {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--app-space-xs, 6px);
  margin-top: var(--app-space-md, 12px);
}
.ver-history-title {
  font-size: var(--app-text-sm, 12px);
  color: var(--td-text-color-secondary);
  margin-right: var(--app-space-xs, 4px);
}
.ver-history-chip {
  cursor: pointer;
}
.ver-history-manage { margin-left: auto; }
.footer-current { display: flex; align-items: center; gap: var(--app-space-xs, 6px); font-size: var(--app-text-sm, 12px); color: var(--td-text-color-secondary); }
.draft-kb-list { display: flex; flex-direction: column; gap: var(--app-space-xs, 4px); }
</style>
