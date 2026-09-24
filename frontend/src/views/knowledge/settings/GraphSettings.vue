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

    <!-- ADR-008：建图抽取模型（graph_config.build_model_id）。空 = 跟随知识库
      模型；可单独换成快速/关思考模型——图谱抽取是批量吞吐场景，思考模型单
      chunk 拖数分钟（实证 30 倍差距）。 -->
    <div class="setting-row">
      <div class="setting-info">
        <label>{{ t('graphSettings.buildModelLabel') }}</label>
        <p class="desc">{{ t('graphSettings.buildModelDescription') }}</p>
      </div>
      <div class="setting-control">
        <t-select
          :value="localGraphConfig.buildModelId || ''"
          clearable
          :placeholder="kbModelPlaceholder"
          @change="(v: unknown) => handleBuildModelChange(String(v ?? ''))"
        >
          <t-option
            v-for="m in chatModels"
            :key="m.id"
            :value="m.id"
            :label="m.display_name || m.name"
          />
        </t-select>
      </div>
    </div>

    <!-- M6-1：图谱浏览（下钻 / 证据跳溯源 / 深链）搬到独立路由页。设置页只留配置与
         入口——400px 表单列放不下全屏画布，下钻也会变成模态套模态，且对话侧无法深链。 -->
    <div v-if="kbId" class="setting-row">
      <div class="setting-info">
        <label>{{ t('graphSettings.graphViewLabel') }}</label>
      </div>
      <div class="setting-control">
        <t-button variant="outline" size="small" @click="openExplorer">
          {{ t('graphSettings.openExplorer') }}
        </t-button>
      </div>
    </div>

    <!-- M5-1：图谱可视化力导图（只读预览；完整交互在独立图谱页） -->
    <div v-if="kbId && graphData && graphData.available" class="setting-row vertical">
      <div class="setting-info">
        <label>{{ t('graphSettings.graphViewLabel') }}</label>
      </div>
      <div class="setting-control full-width">
        <GraphForceChart :nodes="graphData.nodes" :edges="graphData.edges" height="400px"
          :empty-text="t('graphSettings.graphViewEmpty')" />
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
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { getKnowledgeBaseGraphStatus, getKnowledgeBaseGraphView } from '@/api/knowledge-base'
import GraphForceChart from '@/components/knowledge/GraphForceChart.vue'
import { useUIStore } from '@/stores/ui'

const { t } = useI18n()
const router = useRouter()
const uiStore = useUIStore()

/**
 * M6-1：跳图谱浏览器独立页。
 *
 * KB 设置是以弹层呈现的（uiStore.showKBEditorModal），不先收起就会在新页面上压着
 * 一层设置。只在弹层确实打开时收起：本组件也被上传确认弹窗内嵌使用（那里不传
 * kbId，按钮不渲染），不能无条件动弹层状态。
 */
function openExplorer() {
  if (!props.kbId) return
  if (uiStore.showKBEditorModal) uiStore.closeKBEditor()
  void router.push({ name: 'knowledgeBaseGraph', params: { kbId: props.kbId } })
}

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
  // 切换开关不得清掉已选的建图模型
  localGraphConfig.value = { autoBuild: !!v, buildModelId: localGraphConfig.value.buildModelId || '' }
  emit('update:graphConfig', { ...localGraphConfig.value })
}

// 知识库问答模型列表（建图模型候选）：Embedding/ASR 等不参与抽取
const chatModels = computed(() =>
  (props.allModels || []).filter((m: any) => m.type === 'KnowledgeQA'))

// 未单独指定时的默认 = 知识库模型（父组件 formData.modelConfig.llmModelId）
const kbModelPlaceholder = computed(() => {
  const kbModel = (props.allModels || []).find((m: any) => m.id === props.modelId)
  return kbModel ? (kbModel.display_name || kbModel.name) : t('graphSettings.buildModelFollowKb')
})

const handleBuildModelChange = (v: string) => {
  localGraphConfig.value = { autoBuild: localGraphConfig.value.autoBuild, buildModelId: v || '' }
  emit('update:graphConfig', { ...localGraphConfig.value })
}

// ---- 图谱健康度 ----
const status = ref<any>(null)
const statusLoading = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null

const graphData = ref<any>(null)

const loadGraphData = async () => {
  if (!props.kbId) return
  try {
    const res = await getKnowledgeBaseGraphView(props.kbId)
    const d = res?.data || res
    if (d?.available) graphData.value = d
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
  graphData.value = null
  loadStatus()
  loadGraphData()
})

onBeforeUnmount(() => {
  if (pollTimer) clearInterval(pollTimer)
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
</style>
