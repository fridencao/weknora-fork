<template>
  <div class="graph-explorer">
    <header class="graph-explorer__head">
      <t-button variant="text" size="small" @click="goBack">
        <template #icon><t-icon name="chevron-left" /></template>
        {{ t('knowledgeGraph.back') }}
      </t-button>
      <h2 class="graph-explorer__title">{{ t('knowledgeGraph.title') }}</h2>
      <t-tag v-if="scale.truncated" theme="warning" variant="light" size="small">
        {{ t('knowledgeGraph.truncatedNodes', { shown: scale.shownNodes, total: scale.totalNodes }) }}
      </t-tag>
      <!-- M6-1 D3 口径：有未覆盖文档时明示，避免用户把子集当全集 -->
      <t-tag v-if="uncovered > 0" theme="warning" variant="outline" size="small">
        {{ t('knowledgeGraph.uncoveredDocs', { ready: coverageInfo.ready, eligible: coverageInfo.eligible }) }}
      </t-tag>
      <span class="graph-explorer__spacer" />
      <!-- P0-1：实体搜索——命中后 pivot 成 ego 视图（万物可达） -->
      <t-select
        class="graph-explorer__search"
        :value="searchValue"
        filterable
        clearable
        :loading="searching"
        :placeholder="t('knowledgeGraph.searchPlaceholder')"
        :empty="searchEmpty ? t('knowledgeGraph.searchNoResults') : undefined"
        :options="searchOptions"
        @search="onSearch"
        @change="onSearchSelect"
        @clear="onSearchClear"
      />
      <t-button variant="outline" size="small" :loading="loading" @click="reload">
        <template #icon><t-icon name="refresh" /></template>
        {{ t('knowledgeGraph.refresh') }}
      </t-button>
    </header>

    <!-- P0-2/P0-4：ego 状态条 + 类型过滤条（所见即所滤，计数随当前子图联动） -->
    <div class="graph-explorer__toolbar">
      <div v-if="ego.isEgo" class="graph-explorer__ego">
        <t-icon name="focus" size="var(--app-icon-sm)" />
        <span>{{ t('knowledgeGraph.egoBanner', { center: ego.center, depth: ego.depth, n: scale.shownNodes }) }}</span>
        <t-button variant="text" size="small" @click="exitEgo">{{ t('knowledgeGraph.backToOverview') }}</t-button>
      </div>
      <div class="graph-explorer__types">
        <t-tag
          v-for="tc in typeDistribution" :key="tc.type"
          :variant="typeActive(tc.type) ? 'light' : 'outline'"
          :style="typeActive(tc.type) ? { color: colorFor(tc.type, {}) } : {}"
          class="graph-explorer__type-chip"
          size="small"
          @click="toggleType(tc.type)"
        >
          {{ tc.type }} · {{ tc.count }}
        </t-tag>
        <t-tag v-if="activeTypes.length" variant="outline" size="small" class="graph-explorer__type-chip"
          @click="clearTypes">{{ t('knowledgeGraph.typeFilterClear') }}</t-tag>
      </div>
    </div>

    <div class="graph-explorer__body">
      <section class="graph-explorer__canvas">
        <!-- P1-7：文档切换器——已建图文档下拉，?doc= 过滤子图（文档徽标深链也落在这里） -->
        <div v-if="graphedDocs.length || docFilterId" class="graph-explorer__docfilter">
          <t-icon name="filter" size="var(--app-icon-sm)" />
          <t-select
            class="graph-explorer__docselect"
            :value="docFilterId || ''"
            size="small"
            clearable
            filterable
            :placeholder="t('knowledgeGraph.docSwitcherAll')"
            :options="docOptions"
            @change="onDocSelect"
          />
          <span v-if="docFilterId && docFilterMatched >= 0" class="graph-explorer__muted">
            {{ docFilterMatched > 0
              ? t('knowledgeGraph.docFilterBanner', { n: docFilterMatched })
              : t('knowledgeGraph.docFilterEmpty') }}
          </span>
        </div>
        <div v-if="loading && !nodes.length" class="graph-explorer__hint">
          {{ t('knowledgeGraph.loading') }}
        </div>
        <div v-else-if="!nodes.length" class="graph-explorer__hint">
          {{ viewError || t('knowledgeGraph.empty') }}
        </div>
        <GraphForceChart
          v-show="shownNodes.length"
          ref="chartRef"
          :nodes="shownNodes"
          :edges="shownEdges"
          height="100%"
          :highlight-id="selectedId"
          :empty-text="docFilterId ? t('knowledgeGraph.docFilterEmpty') : t('knowledgeGraph.empty')"
          @node-click="onNodeClick"
          @edge-click="onEdgeClick"
          @background-click="clearSelection"
        />
        <p v-if="!selectedId && shownNodes.length" class="graph-explorer__hint graph-explorer__hint--float">
          {{ t('knowledgeGraph.selectHint') }}
        </p>
      </section>

      <aside v-if="selectedId" class="graph-explorer__panel">
        <div v-if="detailLoading" class="graph-explorer__panel-hint">
          {{ t('knowledgeGraph.loading') }}
        </div>
        <template v-else-if="detail && detail.available">
          <h3 class="graph-explorer__entity">{{ detail.entity?.id || selectedId }}</h3>
          <dl class="graph-explorer__fields">
            <div class="graph-explorer__field">
              <dt>{{ t('knowledgeGraph.entityType') }}</dt>
              <dd>
                <t-tag v-if="detail.entity?.entity_type" variant="light" size="small">
                  {{ detail.entity.entity_type }}
                </t-tag>
                <span v-else class="graph-explorer__muted">—</span>
              </dd>
            </div>
            <div class="graph-explorer__field">
              <dt>{{ t('knowledgeGraph.degree') }}</dt>
              <dd>{{ detail.entity?.degree ?? 0 }}</dd>
            </div>
          </dl>
          <t-button variant="outline" size="small" block class="graph-explorer__focus-btn"
            :disabled="ego.isEgo && ego.center === selectedId"
            @click="focusOnCenter(selectedId)">
            <template #icon><t-icon name="focus" /></template>
            {{ t('knowledgeGraph.focusCenter') }}
          </t-button>
          <t-button variant="outline" size="small" block class="graph-explorer__focus-btn"
            @click="askEntity(selectedId)">
            <template #icon><t-icon name="chat" /></template>
            {{ t('knowledgeGraph.askEntity') }}
          </t-button>
          <p v-for="(d, i) in entityDescriptions" :key="i" class="graph-explorer__description">
            {{ d }}
          </p>

          <h4 class="graph-explorer__section">{{ t('knowledgeGraph.neighbors') }}</h4>
          <ul v-if="neighbors.length" class="graph-explorer__list">
            <li v-for="n in neighbors" :key="n.id" class="graph-explorer__neighbor"
              @click="selectNode(n.id)">
              <span class="graph-explorer__neighbor-name">{{ n.id }}</span>
              <t-tag v-if="n.relation_type" variant="outline" size="small">{{ n.relation_type }}</t-tag>
              <t-icon :name="n.direction === 'in' ? 'arrow-left' : 'arrow-right'"
                size="var(--app-icon-sm)" class="graph-explorer__muted" />
            </li>
          </ul>
          <p v-else class="graph-explorer__muted">{{ t('knowledgeGraph.neighborsEmpty') }}</p>

          <h4 class="graph-explorer__section">{{ t('knowledgeGraph.evidence') }}</h4>
          <ul v-if="evidence.length" class="graph-explorer__list">
            <li v-for="e in evidence" :key="e.chunk_id" class="graph-explorer__evidence">
              <div class="graph-explorer__evidence-head">
                <span class="graph-explorer__evidence-title">{{ e.title || e.knowledge_id }}</span>
                <t-button variant="text" size="small" @click="openProvenance(e)">
                  {{ t('knowledgeGraph.openProvenance') }}
                </t-button>
              </div>
              <p class="graph-explorer__evidence-snippet">{{ e.snippet }}</p>
              <!-- P1-6：图谱原文上下文——LLM 抽取时看到的段落，解释「为什么抽到这条关系」 -->
              <details v-if="e.source_context" class="graph-explorer__context">
                <summary>{{ t('knowledgeGraph.sourceContext') }}</summary>
                <p class="graph-explorer__evidence-snippet graph-explorer__context-body">{{ e.source_context }}</p>
              </details>
            </li>
          </ul>
          <p v-else class="graph-explorer__muted">{{ t('knowledgeGraph.evidenceEmpty') }}</p>
          <p v-if="detail.dropped_chunks" class="graph-explorer__muted">
            {{ t('knowledgeGraph.droppedEvidence', { count: detail.dropped_chunks }) }}
          </p>

          <!-- P1-8：证据文档的图表资产（空/失败时整段隐藏） -->
          <template v-if="chartAssets.length">
            <h4 class="graph-explorer__section">{{ t('knowledgeGraph.chartAssets') }}</h4>
            <ul class="graph-explorer__list">
              <li v-for="a in chartAssets" :key="a.asset_id" class="graph-explorer__chart">
                <span class="graph-explorer__neighbor-name">{{ a.title || a.chart_ref }}</span>
                <t-tag v-if="a.chart_type" variant="outline" size="small">{{ a.chart_type }}</t-tag>
                <span v-if="a.page_no" class="graph-explorer__muted">P{{ a.page_no }}</span>
              </li>
            </ul>
          </template>
        </template>
        <p v-else class="graph-explorer__panel-hint">
          {{ detail?.reason || t('knowledgeGraph.unavailable') }}
        </p>
      </aside>

      <!-- M6-1 WS1.2：点边下钻。实体的证据是「节点 ∪ 邻居」合并集，答不了
           「这条关系从哪句话抽出来的」，所以边单独取、单独展示。 -->
      <aside v-else-if="selectedEdge" class="graph-explorer__panel">
        <div v-if="edgeLoading" class="graph-explorer__panel-hint">
          {{ t('knowledgeGraph.loading') }}
        </div>
        <template v-else-if="edgeDetail && edgeDetail.available">
          <h3 class="graph-explorer__entity graph-explorer__entity--edge">
            <span>{{ selectedEdge.source }}</span>
            <t-icon name="arrow-right" size="var(--app-icon-sm)" />
            <span>{{ selectedEdge.target }}</span>
          </h3>

          <h4 class="graph-explorer__section">{{ t('knowledgeGraph.relations') }}</h4>
          <ul v-if="relations.length" class="graph-explorer__list">
            <li v-for="(r, i) in relations" :key="i" class="graph-explorer__relation">
              <div class="graph-explorer__relation-tags">
                <t-tag v-for="rt in r.relation_type ? r.relation_type.split('、') : []"
                  :key="rt" variant="light" size="small">{{ rt }}</t-tag>
                <t-tag v-if="r.direction === 'in'" theme="warning" variant="outline" size="small">
                  {{ t('knowledgeGraph.directionIn') }}
                </t-tag>
              </div>
              <p v-for="(d, j) in r.descriptions" :key="j" class="graph-explorer__evidence-snippet">
                {{ d }}
              </p>
            </li>
          </ul>
          <p v-else class="graph-explorer__muted">{{ t('knowledgeGraph.noRelations') }}</p>

          <h4 class="graph-explorer__section">{{ t('knowledgeGraph.evidence') }}</h4>
          <ul v-if="edgeEvidence.length" class="graph-explorer__list">
            <li v-for="e in edgeEvidence" :key="e.chunk_id" class="graph-explorer__evidence">
              <div class="graph-explorer__evidence-head">
                <span class="graph-explorer__evidence-title">{{ e.title || e.knowledge_id }}</span>
                <t-button variant="text" size="small" @click="openProvenance(e)">
                  {{ t('knowledgeGraph.openProvenance') }}
                </t-button>
              </div>
              <p class="graph-explorer__evidence-snippet">{{ e.snippet }}</p>
              <details v-if="e.source_context" class="graph-explorer__context">
                <summary>{{ t('knowledgeGraph.sourceContext') }}</summary>
                <p class="graph-explorer__evidence-snippet graph-explorer__context-body">{{ e.source_context }}</p>
              </details>
            </li>
          </ul>
          <p v-else class="graph-explorer__muted">{{ t('knowledgeGraph.edgeEvidenceEmpty') }}</p>
          <p v-if="edgeDetail.dropped_chunks" class="graph-explorer__muted">
            {{ t('knowledgeGraph.droppedEvidence', { count: edgeDetail.dropped_chunks }) }}
          </p>
        </template>
        <p v-else class="graph-explorer__panel-hint">
          {{ edgeDetail?.reason || t('knowledgeGraph.unavailable') }}
        </p>
      </aside>
    </div>

    <ProvenancePanel />
  </div>
</template>

<script setup lang="ts">
/**
 * M6-1 WS1.2 · 知识图谱浏览器（独立路由页）。
 *
 * 为什么是独立页而不是 KB 设置页里的一块：设置页是模态内的 400px 表单列，
 * 下钻会变成模态套模态，且对话侧无法深链。这里给全屏画布 + `?node=` 深链，
 * 设置页只留配置与入口。
 *
 * 数据来源：`/graph/view`（节点/边 + 截断标记）+ `/graph/entity`（下钻，证据已由
 * Go 侧回跳为 WeKnora 子 chunk）；证据可直接喂 ProvenancePanel。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import GraphForceChart from '@/components/knowledge/GraphForceChart.vue'
import ProvenancePanel from '@/components/ProvenancePanel.vue'
import { coverageSummary, type GraphCoveragePayload } from '@/utils/graphCoverage'
import { provideProvenancePanel } from '@/composables/useProvenancePanel'
import { useMenuStore } from '@/stores/menu'
import { getKnowledgeBaseGraphCharts, getKnowledgeBaseGraphCoverage, getKnowledgeBaseGraphDocStatus, getKnowledgeBaseGraphEdge, getKnowledgeBaseGraphEntity, getKnowledgeBaseGraphView, listKnowledgeFiles, searchKnowledgeBaseGraphEntities } from '@/api/knowledge-base'
import { colorFor, type GraphEdgeDatum, type GraphNodeDatum } from '@/components/knowledge/graphForceChart'
import {
  descriptionsOf,
  edgeParam,
  egoSummary,
  evidenceToProvenanceInput,
  filterGraphByDoc,
  graphScale,
  neighborRows,
  parseEdgeParam,
  relationRows,
  typeCounts,
  unwrapGraphPayload,
  type GraphEdgeDetail,
  type GraphEntityDetail,
  type GraphEntitySearchPayload,
  type GraphEvidence,
  type GraphViewPayload,
} from './graphExplorer'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const provenancePanel = provideProvenancePanel()

const kbId = computed(() => String(route.params.kbId || ''))
const nodes = ref<GraphNodeDatum[]>([])
const edges = ref<GraphEdgeDatum[]>([])
const viewTotal = ref<GraphViewPayload | null>(null)
const viewError = ref('')
const loading = ref(false)

const selectedId = ref('')
const detail = ref<GraphEntityDetail | null>(null)
const detailLoading = ref(false)

const selectedEdge = ref<{ source: string; target: string } | null>(null)
const edgeDetail = ref<GraphEdgeDetail | null>(null)
const edgeLoading = ref(false)

const scale = computed(() => graphScale(viewTotal.value))
const neighbors = computed(() => neighborRows(detail.value))
const evidence = computed(() => detail.value?.evidence || [])
const entityDescriptions = computed(() => descriptionsOf(detail.value?.entity))
const relations = computed(() => relationRows(edgeDetail.value))
const edgeEvidence = computed(() => edgeDetail.value?.evidence || [])

// P0（图谱浏览器规划 2026-09-25）：ego 视图与类型过滤状态。视图参数变化都走
// loadGraph 重取——过滤语义是"服务端重取"（wiki 图谱验证过的做法），不是客户端隐藏。
const viewMode = ref<'overview' | 'ego'>('overview')
const centerNode = ref('')
const activeTypes = ref<string[]>([])
const ego = computed(() => egoSummary(viewTotal.value))

const typeDistribution = computed(() => typeCounts(nodes.value))
function typeActive(type: string) {
  return activeTypes.value.includes(type)
}
function toggleType(type: string) {
  activeTypes.value = typeActive(type)
    ? activeTypes.value.filter((t) => t !== type)
    : [...activeTypes.value, type]
  void loadGraph()
}
function clearTypes() {
  activeTypes.value = []
  void loadGraph()
}

// P0-1：实体搜索（远程搜索 → 选中 pivot 成 ego 视图 → 飞入 + 选中开面板）
const searchValue = ref('')
const searching = ref(false)
const searchOptions = ref<{ label: string; value: string }[]>([])
const searchEmpty = ref(false)

async function onSearch(keyword: string) {
  const q = String(keyword || '').trim()
  searchEmpty.value = false
  if (!q) {
    searchOptions.value = []
    return
  }
  searching.value = true
  try {
    const res = await searchKnowledgeBaseGraphEntities(kbId.value, q, activeTypes.value)
    const payload = unwrapGraphPayload<GraphEntitySearchPayload>(res)
    const results = payload?.results || []
    searchOptions.value = results.map((r) => ({
      label: `[${r.entity_type || '—'}] ${r.id}（${r.degree ?? 0}）`,
      value: r.id,
    }))
    searchEmpty.value = results.length === 0
  } catch {
    searchOptions.value = []
    searchEmpty.value = true
  } finally {
    searching.value = false
  }
}

function onSearchSelect(value: unknown) {
  const id = String(value || '').trim()
  if (!id) return
  focusOnCenter(id)
  searchValue.value = ''
  searchOptions.value = []
}

function onSearchClear() {
  searchValue.value = ''
  searchOptions.value = []
}

/** P0-2：以某实体为中心切入 ego 子图（depth=1），并选中开面板。 */
async function focusOnCenter(id: string) {
  if (!id) return
  viewMode.value = 'ego'
  centerNode.value = id
  await loadGraph()
  selectNode(id)
}

function exitEgo() {
  viewMode.value = 'overview'
  centerNode.value = ''
  void loadGraph()
}

// P1-9：问这个实体——经 menuStore 预填队列跳新建对话（S3 场景：
// 图→问→答（带 C2 图谱召回与溯源）→证据→图）。问题带类型引导语，
// 让图谱通道有机会召回该实体的邻域。
function askEntity(id: string) {
  if (!id) return
  const type = detail.value?.entity?.entity_type || ''
  const question = type
    ? `请结合知识库介绍「${id}」（类型：${type}）及其关联关系`
    : `请结合知识库介绍「${id}」及其关联关系`
  useMenuStore().setPrefillQuery(question)
  void router.push('/platform/creatChat')
}

// WS1.1b：?doc=<knowledgeId> 按文档过滤子图。依据：节点/边 source_id 里的
// LightRAG chunk key 前段就是 WeKnora doc id（A0 口径），纯前端可判定。
const docFilterId = computed(() => String(route.query.doc || '').trim())
const filteredGraph = computed(() =>
  filterGraphByDoc(nodes.value, edges.value, docFilterId.value))
const shownNodes = computed(() => filteredGraph.value.nodes)
const shownEdges = computed(() => filteredGraph.value.edges)
const docFilterMatched = computed(() => filteredGraph.value.matched)

function clearDocFilter() {
  const query = { ...route.query }
  delete query.doc
  void router.replace({ query })
}

// P1-7：文档切换器。已建图（doc-status=ready）文档清单是下拉的数据源；
// 列表一次拉全（page_size 200 对单 KB 足够），失败静默——切换器是辅助入口。
const graphedDocs = ref<{ id: string; title: string }[]>([])
const docOptions = computed(() => graphedDocs.value.map((d) => ({ label: d.title, value: d.id })))

async function loadGraphedDocs() {
  if (!kbId.value) return
  try {
    const res = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 200 })
    const body = res as { data?: unknown }
    const items = Array.isArray(body?.data) ? body.data : []
    const docs = items
      .map((d: any) => ({ id: String(d?.id || ''), title: String(d?.file_name || d?.title || d?.id || '') }))
      .filter((d) => d.id)
    if (!docs.length) return
    const st = await getKnowledgeBaseGraphDocStatus(kbId.value, docs.map((d) => d.id))
    const states = (unwrapGraphPayload<{ states?: Record<string, string> }>(st))?.states || {}
    graphedDocs.value = docs.filter((d) => states[d.id] === 'ready')
  } catch {
    graphedDocs.value = []
  }
}

function onDocSelect(value: unknown) {
  const id = String(value || '').trim()
  const query = { ...route.query }
  if (id) {
    void router.replace({ query: { ...query, doc: id } })
  } else {
    delete query.doc
    void router.replace({ query })
  }
}

// M6-1 D3 口径：覆盖率标注（失败静默——它是补充信息，不是面板的主体）
const coverage = ref<GraphCoveragePayload | null>(null)
const coverageInfo = computed(() => coverageSummary(coverage.value))
const uncovered = computed(() => coverageInfo.value.uncovered)

const loadCoverage = async () => {
  try {
    const res = await getKnowledgeBaseGraphCoverage(kbId.value)
    coverage.value = unwrapGraphPayload<GraphCoveragePayload>(res)
  } catch {
    coverage.value = null
  }
}

async function loadGraph() {
  if (!kbId.value) return
  loading.value = true
  viewError.value = ''
  try {
    const params = viewMode.value === 'ego'
      ? { mode: 'ego' as const, center: centerNode.value, depth: 1, types: activeTypes.value }
      : { types: activeTypes.value.length ? activeTypes.value : undefined }
    const res = await getKnowledgeBaseGraphView(kbId.value, params)
    const payload = unwrapGraphPayload<GraphViewPayload>(res)
    if (payload?.available) {
      nodes.value = payload.nodes || []
      edges.value = payload.edges || []
      viewTotal.value = payload
      // ego-miss（中心实体不在可见范围）时数据面返回 available+reason+空集，
      // 此时把 reason 显示出来而不是落进通用空态。
      if (!nodes.value.length && payload.reason) viewError.value = payload.reason
    } else {
      nodes.value = []
      edges.value = []
      viewError.value = payload?.reason || t('knowledgeGraph.unavailable')
    }
  } catch (e: any) {
    nodes.value = []
    edges.value = []
    viewError.value = e?.message || t('knowledgeGraph.unavailable')
  } finally {
    loading.value = false
  }
}

async function loadEntity(name: string) {
  if (!kbId.value || !name) return
  detailLoading.value = true
  try {
    const res = await getKnowledgeBaseGraphEntity(kbId.value, name)
    detail.value = unwrapGraphPayload<GraphEntityDetail>(res)
  } catch {
    detail.value = { available: false }
  } finally {
    detailLoading.value = false
  }
  // P1-8：证据文档的图表资产跟随实体下钻加载（失败静默）
  const docIds = [...new Set((detail.value?.evidence || [])
    .map((e) => e.knowledge_id).filter(Boolean))] as string[]
  void loadChartsFor(docIds.slice(0, 5))
}

async function loadEdge(source: string, target: string) {
  if (!kbId.value || !source || !target) return
  edgeLoading.value = true
  try {
    const res = await getKnowledgeBaseGraphEdge(kbId.value, source, target)
    edgeDetail.value = unwrapGraphPayload<GraphEdgeDetail>(res)
  } catch {
    edgeDetail.value = { available: false }
  } finally {
    edgeLoading.value = false
  }
}

// P1-8：实体证据涉及的文档 → 图表资产（解析管线注册；表为空或代理不可达时
// 恒为空数组、区块隐藏）。loadEntity 成功后触发。
const chartAssets = ref<Record<string, any>[]>([])

async function loadChartsFor(docIds: string[]) {
  chartAssets.value = []
  if (!kbId.value || !docIds.length) return
  try {
    const res = await getKnowledgeBaseGraphCharts(kbId.value, docIds)
    const payload = unwrapGraphPayload<{ available?: boolean; assets?: Record<string, any>[] }>(res)
    if (payload?.available) chartAssets.value = payload.assets || []
  } catch {
    chartAssets.value = []
  }
}

/** 选中并在 URL 上留痕（`?node=` / `?edge=`），对话侧/文档侧才能深链。 */
function selectNode(id: string) {
  if (!id) return
  selectedId.value = id
  selectedEdge.value = null
  edgeDetail.value = null
  void loadEntity(id)
  const query = { ...route.query }
  delete query.edge
  if (String(route.query.node || '') === id) {
    void router.replace({ query })
    return
  }
  void router.replace({ query: { ...query, node: id } })
}

function selectEdge(source: string, target: string) {
  if (!source || !target) return
  selectedEdge.value = { source, target }
  selectedId.value = ''
  detail.value = null
  void loadEdge(source, target)
  const query = { ...route.query }
  delete query.node
  const next = edgeParam(source, target)
  if (String(route.query.edge || '') === next) {
    void router.replace({ query })
    return
  }
  void router.replace({ query: { ...query, edge: next } })
}

function onNodeClick(node: GraphNodeDatum) {
  selectNode(node?.id || '')
}

function onEdgeClick(edge: GraphEdgeDatum) {
  if (!edge?.source || !edge?.target) return
  selectEdge(edge.source, edge.target)
}

function clearSelection() {
  selectedId.value = ''
  detail.value = null
  selectedEdge.value = null
  edgeDetail.value = null
  if (route.query.node || route.query.edge) {
    const query = { ...route.query }
    delete query.node
    delete query.edge
    void router.replace({ query })
  }
}

function openProvenance(item: GraphEvidence) {
  if (!provenancePanel) return
  provenancePanel.open({
    inputs: [evidenceToProvenanceInput(item)],
    title: item.title || '',
    context: item.snippet || '',
  })
}

function reload() {
  void loadGraph()
  if (selectedId.value) void loadEntity(selectedId.value)
}

function goBack() {
  void router.push({ name: 'knowledgeBaseDetail', params: { kbId: kbId.value } })
}

onMounted(async () => {
  void loadCoverage()
  void loadGraphedDocs()
  await loadGraph()
  // 深链：?node= 或 ?edge=（二选一，node 优先）
  const deepLink = String(route.query.node || '')
  if (deepLink) {
    selectNode(deepLink)
    return
  }
  const edge = parseEdgeParam(route.query.edge)
  if (edge) selectEdge(edge.source, edge.target)
})

watch(() => route.query.node, (value) => {
  const next = String(value || '')
  if (next === selectedId.value) return
  if (next) {
    selectNode(next)
    return
  }
  // node 被移除也可能是选边时顺手删的，此时不能把刚选的边清掉
  if (selectedEdge.value) return
  clearSelection()
})

watch(() => route.query.edge, (value) => {
  const next = parseEdgeParam(value)
  if (!next) return
  // URL 是自己写的（replace）时再触发会重复请求，这里按端点判重
  if (selectedEdge.value
    && selectedEdge.value.source === next.source
    && selectedEdge.value.target === next.target) return
  selectEdge(next.source, next.target)
})
</script>

<style lang="less" scoped>
.graph-explorer {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  padding: 12px 16px 16px;
  background: var(--td-bg-color-page);

  &__head {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 0 0 auto;
    padding-bottom: 8px;
  }

  &__title {
    margin: 0;
    font-size: var(--app-text-lg, 16px);
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  &__spacer { flex: 1 1 auto; }

  &__search {
    width: 240px;
  }

  &__toolbar {
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    padding: 0 0 8px;
  }

  &__ego {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 3px 10px;
    border: 1px solid var(--td-brand-color-4, #b5c7ff);
    border-radius: 6px;
    background: var(--td-brand-color-1, #ecf2ff);
    color: var(--td-text-color-primary);
    font-size: var(--app-text-sm, 13px);
  }

  &__types {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: wrap;
  }

  &__type-chip {
    cursor: pointer;
    user-select: none;
  }

  &__focus-btn {
    margin-bottom: 10px;
  }

  &__body {
    display: flex;
    flex: 1 1 auto;
    min-height: 0;
    gap: 12px;
  }

  &__canvas {
    position: relative;
    flex: 1 1 auto;
    min-width: 0;
  }

  &__docfilter {
    position: absolute;
    top: 8px;
    left: 8px;
    z-index: 2;
    display: flex;
    align-items: center;
    gap: 6px;
    max-width: calc(100% - 16px);
    padding: 4px 8px;
    border: 1px solid var(--td-component-stroke);
    border-radius: 6px;
    background: var(--td-bg-color-container);
    color: var(--td-text-color-secondary);
    font-size: var(--app-text-sm, 13px);
  }

  &__docselect {
    width: 260px;
  }

  &__hint {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: var(--td-text-color-placeholder);
    font-size: var(--app-text-sm, 13px);

    &--float {
      position: absolute;
      inset: auto 0 12px 0;
      height: auto;
      pointer-events: none;
    }
  }

  &__panel {
    flex: 0 0 320px;
    overflow-y: auto;
    padding: 12px;
    border: 1px solid var(--td-component-stroke);
    border-radius: 6px;
    background: var(--td-bg-color-container);
  }

  &__panel-hint {
    color: var(--td-text-color-placeholder);
    font-size: var(--app-text-sm, 13px);
  }

  &__entity {
    margin: 0 0 8px;
    font-size: var(--app-text-base, 14px);
    font-weight: 600;
    color: var(--td-text-color-primary);
    word-break: break-all;

    &--edge {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
    }
  }

  &__relation {
    padding: 4px 0;
  }

  &__relation-tags {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: wrap;
  }

  &__fields {
    display: flex;
    gap: 16px;
    margin: 0 0 8px;
  }

  &__field {
    dt {
      color: var(--td-text-color-placeholder);
      font-size: var(--app-text-xs, 12px);
    }
    dd { margin: 2px 0 0; }
  }

  &__description {
    margin: 0 0 12px;
    font-size: var(--app-text-sm, 13px);
    line-height: 1.6;
    color: var(--td-text-color-secondary);
    white-space: pre-wrap;
    word-break: break-word;
  }

  &__section {
    margin: 12px 0 6px;
    font-size: var(--app-text-sm, 13px);
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  &__list {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  &__neighbor {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 0;
    cursor: pointer;

    &:hover .graph-explorer__neighbor-name { color: var(--td-brand-color); }
  }

  &__neighbor-name {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--app-text-sm, 13px);
  }

  &__evidence {
    padding: 6px 0;
    border-bottom: 1px dashed var(--td-component-stroke);
  }

  &__evidence-head {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  &__evidence-title {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--app-text-sm, 13px);
    font-weight: 500;
  }

  &__evidence-snippet {
    margin: 2px 0 0;
    font-size: var(--app-text-xs, 12px);
    line-height: 1.5;
    color: var(--td-text-color-secondary);
  }

  &__context {
    margin-top: 4px;

    summary {
      cursor: pointer;
      color: var(--td-brand-color);
      font-size: var(--app-text-xs, 12px);
      user-select: none;
    }
  }

  &__context-body {
    margin: 4px 0 0;
    padding: 6px 8px;
    border-left: 2px solid var(--td-component-stroke);
    max-height: 220px;
    overflow-y: auto;
    white-space: pre-wrap;
  }

  &__muted {
    color: var(--td-text-color-placeholder);
    font-size: var(--app-text-sm, 13px);
  }
}
</style>
