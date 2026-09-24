<template>
  <div class="graph-settings" :class="{ 'graph-settings--embedded': embedded }">
    <div v-if="!embedded" class="section-header">
      <h2>{{ t('graphSettings.lightragTitle') }}</h2>
      <p class="section-description">{{ t('graphSettings.lightragDescription') }}</p>
    </div>

    <!-- A1/A3：KB 级自动建图开关（保存随 KB 配置提交 auto_graph_config） -->
    <div class="setting-row">
      <div class="setting-info">
        <label>{{ t('graphSettings.autoBuildLabel') }}</label>
        <p class="desc">{{ t('graphSettings.autoBuildDescription') }}</p>
      </div>
      <div class="setting-control">
        <t-switch
          :value="localGraphConfig.autoBuild"
          @change="(v: boolean) => handleGraphConfigChange(v)"
        />
      </div>
    </div>

    <!-- M5-1：图谱可视化力导图 -->
    <div v-if="kbId && graphData && graphData.available" class="setting-row vertical">
      <div class="setting-info">
        <label>{{ t('graphSettings.graphViewLabel') }}</label>
      </div>
      <div class="setting-control full-width">
        <div ref="chartEl" class="graph-chart"></div>
      </div>
    </div>

    <!-- 图谱健康度（编辑模式且拿到 kbId 时展示；数据来自 starkb-api 健康度端点） -->
    <div v-if="kbId" class="setting-row vertical">
      <div class="setting-info">
        <label>{{ t('graphSettings.statusTitle') }}</label>
        <p class="desc">{{ t('graphSettings.statusDescription') }}</p>
      </div>
      <div class="setting-control full-width">
        <div class="status-card">
          <t-loading :loading="statusLoading" size="small">
            <template v-if="status && status.graph && status.graph.available">
              <div class="stat-grid">
                <div class="stat-item">
                  <span class="stat-value">{{ status.graph.docs_processed ?? 0 }}</span>
                  <span class="stat-label">{{ t('graphSettings.statDocs') }}</span>
                </div>
                <div class="stat-item">
                  <span class="stat-value">{{ status.graph.graph_nodes ?? 0 }}</span>
                  <span class="stat-label">{{ t('graphSettings.statEntities') }}</span>
                </div>
                <div class="stat-item">
                  <span class="stat-value">{{ status.graph.graph_edges ?? 0 }}</span>
                  <span class="stat-label">{{ t('graphSettings.statRelations') }}</span>
                </div>
                <div class="stat-item">
                  <span class="stat-value">{{ status.graph.chunks ?? 0 }}</span>
                  <span class="stat-label">{{ t('graphSettings.statChunks') }}</span>
                </div>
              </div>
              <div v-if="(status.graph.jobs || []).length" class="job-list">
                <div v-for="job in (status.graph.jobs || []).slice(0, 5)" :key="job.job_id" class="job-item">
                  <t-tag :theme="jobTheme(job.status)" size="small" variant="light">
                    {{ job.status }}
                  </t-tag>
                  <span class="job-msg">{{ job.message || job.job_id }}</span>
                  <span class="job-time">{{ formatTime(job.updated_at) }}</span>
                </div>
              </div>
            </template>
            <div v-else-if="status && status.graph && !status.graph.available" class="status-unavailable">
              <t-icon name="info-circle" />
              <span>{{ t('graphSettings.statusUnavailable') }}{{ status.graph.reason ? '：' + status.graph.reason : '' }}</span>
            </div>
            <div class="status-actions">
              <t-button size="small" theme="default" :loading="statusLoading" @click="loadStatus">
                {{ t('graphSettings.refresh') }}
              </t-button>
            </div>
          </t-loading>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// A3（docs/09 WS1.3）：本页从「上游内置实体关系抽取示例」改造为 LightRAG 图谱
// KB 控制页。内置抽取（extract_config / graphExtract）已被 D2 决策关闭，原示例
// 控件移除；props 保留 graphExtract 以兼容父组件的保存载荷。
// 自动建图开关写 KB 的 auto_graph_config（A1 钩子消费）；健康度来自
// GET /knowledge-bases/:id/graph/status（Go 代理 starkb-api /graph/status）。
import { ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import * as echarts from 'echarts'
import { useI18n } from 'vue-i18n'
import { getKnowledgeBaseGraphStatus, getKnowledgeBaseGraphView } from '@/api/knowledge-base'

const TYPE_COLORS: Record<string, string> = {
  公司: '#4f7cf0', 人物: '#e0666c', 产品: '#48b884', 行业: '#f0a04f',
  概念: '#9b6ff0', 事件: '#f0cf4f', 政策: '#5fc9e0', 指标: '#e0919b',
}

const { t } = useI18n()

interface GraphConfig {
  autoBuild: boolean
  buildModelId?: string
}

interface Props {
  graphExtract: Record<string, unknown> // 兼容保留（内置抽取已废弃）
  modelId: string
  allModels?: any[]
  embedded?: boolean
  kbId?: string
  graphConfig?: GraphConfig
}

const props = withDefaults(defineProps<Props>(), {
  embedded: false,
  kbId: '',
  graphConfig: () => ({ autoBuild: false }),
})

const emit = defineEmits<{
  // 内置抽取已废弃（D2），payload 保持透传；父组件各自的类型不统一，用宽松签名兼容
  'update:graphExtract': [value: any]
  'update:graphConfig': [value: GraphConfig]
}>()

const localGraphConfig = ref<GraphConfig>({ ...props.graphConfig })

watch(() => props.graphConfig, (v) => {
  localGraphConfig.value = { ...(v || { autoBuild: false }) }
}, { deep: true })

const handleGraphConfigChange = (v: boolean) => {
  localGraphConfig.value = { autoBuild: !!v }
  emit('update:graphConfig', { ...localGraphConfig.value })
}

// ---- 图谱健康度 ----
const status = ref<any>(null)
const statusLoading = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null

const graphData = ref<any>(null)
const chartEl = ref<HTMLElement>()
let chart: echarts.ECharts | null = null

// echarts 实例不随 v-if 自动回收：KB 切换（graphData 置空使容器卸载）与页面离开
// 都必须显式 dispose，否则残留实例持有已移除的 DOM。
// 尺寸变化用 ResizeObserver 而非 window.resize：容器在 UploadConfirmDialog 里靠
// v-show 切换，隐藏期 init 得到 0×0，只有容器自身尺寸变化才能触发重排。
let resizeObserver: ResizeObserver | null = null

const disposeChart = () => {
  resizeObserver?.disconnect()
  resizeObserver = null
  if (chart) { chart.dispose(); chart = null }
}

const observeContainer = () => {
  if (resizeObserver || !chartEl.value || typeof ResizeObserver === 'undefined') return
  resizeObserver = new ResizeObserver(() => chart?.resize())
  resizeObserver.observe(chartEl.value)
}

const renderGraph = () => {
  if (!chartEl.value || !graphData.value?.nodes?.length) return
  if (!chart) chart = echarts.init(chartEl.value)
  const nodes = graphData.value.nodes.map((n: any) => ({
    id: n.id,
    name: n.id,
    symbolSize: Math.min(10 + (n.degree || 0) * 1.5, 40),
    category: n.entity_type || '其他',
    itemStyle: { color: TYPE_COLORS[n.entity_type] || '#8ca3b8' },
  }))
  const edges = graphData.value.edges.map((e: any) => ({
    source: e.source, target: e.target,
    lineStyle: { width: 1, color: 'source', opacity: 0.3 },
  }))
  chart.setOption({
    series: [{
      type: 'graph', layout: 'force', roam: true, draggable: true,
      data: nodes, links: edges,
      force: { repulsion: 120, edgeLength: [40, 120], gravity: 0.1 },
      label: { show: true, fontSize: 10, position: 'right' },
      emphasis: { focus: 'adjacency', lineStyle: { width: 3 } },
      lineStyle: { curveness: 0.1 },
    }],
  })
  observeContainer()
  chart.resize()
}

const loadGraphData = async () => {
  if (!props.kbId) return
  try {
    const res = await getKnowledgeBaseGraphView(props.kbId)
    const d = res?.data || res
    if (d?.available) { graphData.value = d; await nextTick(); renderGraph() }
  } catch { /* 静默 */ }
}

const loadStatus = async () => {
  if (!props.kbId) return
  statusLoading.value = true
  try {
    const res = await getKnowledgeBaseGraphStatus(props.kbId)
    status.value = res?.data || res
  } catch {
    status.value = null
  } finally {
    statusLoading.value = false
  }
  // 有进行中的任务时保持 15s 轻轮询；空闲即停
  const running = (status.value?.graph?.jobs || []).some(
    (j: any) => j.status === 'running' || j.status === 'queued')
  if (running && !pollTimer) {
    pollTimer = setInterval(loadStatus, 15000)
  } else if (!running && pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

const jobTheme = (s: string) => {
  switch (s) {
    case 'completed': return 'success'
    case 'failed': return 'danger'
    case 'partial': return 'warning'
    default: return 'default'
  }
}

const formatTime = (ts: number | string) => {
  const n = typeof ts === 'string' ? Number(ts) : ts
  if (!n || Number.isNaN(n)) return ''
  const d = new Date(n * (n < 1e12 ? 1000 : 1))
  return d.toLocaleString()
}

onMounted(() => {
  loadStatus()
  loadGraphData()
})

// 同一实例内切换 KB（KnowledgeBaseEditorModal 传 activeKbId，无 :key 重挂载）时
// 必须重载，否则展示上一个 KB 的图与健康度。
watch(() => props.kbId, () => {
  disposeChart()
  graphData.value = null
  loadStatus()
  loadGraphData()
})

onBeforeUnmount(() => {
  if (pollTimer) clearInterval(pollTimer)
  disposeChart()
})
</script>

<style lang="less" scoped>
.graph-settings {
  width: 100%;
}

.section-header {
  margin-bottom: 20px;

  h2 {
    font-size: var(--app-text-3xl);
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 6px 0;
  }

  .section-description {
    font-size: var(--app-text-base);
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.5;
  }
}

.setting-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 16px 0;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child {
    border-bottom: none;
  }

  &.vertical {
    flex-direction: column;
    gap: 12px;

    .setting-control {
      width: 100%;
      max-width: 100%;
    }
  }
}

.setting-info {
  flex: 0 0 40%;
  max-width: 40%;
  padding-right: 24px;

  label {
    font-size: var(--app-text-lg);
    font-weight: 500;
    color: var(--td-text-color-primary);
    display: block;
    margin-bottom: 4px;
  }

  .desc {
    font-size: var(--app-text-md);
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.5;
  }
}

.setting-control {
  flex: 0 0 55%;
  max-width: 55%;
  display: flex;
  justify-content: flex-end;
  align-items: center;

  &.full-width {
    width: 100%;
    max-width: 100%;
    flex-direction: column;
    align-items: flex-start;
    gap: 12px;
  }
}

.status-card {
  width: 100%;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md);
  padding: 16px;
  background: var(--td-bg-color-container);
}

.stat-grid {
  display: flex;
  gap: 24px;
  flex-wrap: wrap;
}

.stat-item {
  display: flex;
  flex-direction: column;

  .stat-value {
    font-size: var(--app-text-3xl);
    font-weight: 600;
    color: var(--td-brand-color);
  }

  .stat-label {
    font-size: var(--app-text-md);
    color: var(--td-text-color-secondary);
  }
}

.job-list {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.job-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: var(--app-text-md);

  .job-msg {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--td-text-color-secondary);
  }

  .job-time {
    color: var(--td-text-color-placeholder);
    font-size: var(--app-text-md);
  }
}

.status-unavailable {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--td-text-color-secondary);
  font-size: var(--app-text-md);
}

.status-actions {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}

.graph-settings--embedded {
  .setting-row:not(.vertical) {
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
    padding: 12px 0;
  }

  .setting-row:not(.vertical) .setting-info {
    flex: none;
    max-width: none;
    padding-right: 0;
  }

  .setting-row:not(.vertical) .setting-control {
    align-self: flex-start;
  }
}

.graph-chart {
  width: 100%;
  height: 400px;
  min-height: 300px;
}
</style>
