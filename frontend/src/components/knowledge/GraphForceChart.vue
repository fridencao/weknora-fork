<template>
  <div class="graph-force-chart" :style="{ height }">
    <div v-if="!nodes.length" class="graph-force-chart__empty">
      <slot name="empty">{{ emptyText }}</slot>
    </div>
    <div v-show="nodes.length" ref="chartEl" class="graph-force-chart__canvas"></div>
  </div>
</template>

<script setup lang="ts">
/**
 * M5-1 / M6-1 · KB 知识图谱力导图（echarts graph）。
 *
 * 纯展示组件：数据从 props 进、交互从 emits 出——不发请求、不碰路由、不知道自己
 * 被谁托着（KB 设置页 / M6-1 的独立图谱浏览器页）。
 *
 * 拆件的理由：WS1.2 的下钻（节点→实体卡片、边→证据 chunk→溯源面板）与 WS1.3 的
 * 检索联动（对话里的实体→高亮节点）都要复用同一张图，且后者来自另一个页面。
 * 配色/分类/option 组装等纯逻辑在 ./graphForceChart.ts，可单测。
 */
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts'
import {
  buildGraphOption, toEdgeKey,
  type GraphEdgeDatum, type GraphLayout, type GraphNodeDatum,
} from './graphForceChart'

const props = withDefaults(defineProps<{
  nodes: GraphNodeDatum[]
  edges: GraphEdgeDatum[]
  /** 画布高度。设置页给 400px；独立图谱页给 100%（宿主容器需有确定高度）。 */
  height?: string
  /** WS1.3：从对话/文档跳进来时高亮的目标节点 id。 */
  highlightId?: string
  layout?: GraphLayout
  /** 实体类型 → 颜色覆盖；未命中的类型走内置配色或哈希取色。 */
  palette?: Record<string, string>
  showLegend?: boolean
  emptyText?: string
}>(), {
  height: '400px',
  highlightId: '',
  layout: 'force',
  palette: () => ({}),
  showLegend: true,
  emptyText: '',
})

const emit = defineEmits<{
  (e: 'node-click', node: GraphNodeDatum): void
  (e: 'edge-click', edge: GraphEdgeDatum): void
  /** 点空白处：宿主用它收起下钻面板。 */
  (e: 'background-click'): void
  (e: 'ready'): void
}>()

const chartEl = ref<HTMLElement>()
let chart: echarts.ECharts | null = null
let resizeObserver: ResizeObserver | null = null

function disposeChart() {
  resizeObserver?.disconnect()
  resizeObserver = null
  if (chart) { chart.dispose(); chart = null }
}

function observeContainer() {
  const el = chartEl.value
  if (resizeObserver || !el || typeof ResizeObserver === 'undefined') return
  // 用容器自身尺寸变化而非 window.resize：宿主可能用 v-show 切换（隐藏期只有
  // 0×0）、也可能被折叠，只有容器尺寸变化能覆盖全部情况。
  resizeObserver = new ResizeObserver(() => chart?.resize())
  resizeObserver.observe(el)
}

/** 重绘：首次挂载、数据整批替换（切 KB）、宿主主动重置视图。 */
async function render() {
  await nextTick()
  const el = chartEl.value
  if (!el || !props.nodes.length) { disposeChart(); return }
  if (!chart || chart.getDom() !== el) { disposeChart(); chart = echarts.init(el) }
  // notMerge：数据整批替换，避免上一次的节点/分类残留
  chart.setOption(buildGraphOption({
    nodes: props.nodes,
    edges: props.edges,
    layout: props.layout,
    palette: props.palette,
    showLegend: props.showLegend,
    highlightId: props.highlightId,
  }), true)
  observeContainer()
  chart.resize()
  bindEvents()
  emit('ready')
}

function bindEvents() {
  if (!chart) return
  chart.off('click')
  chart.on('click', (p: any) => {
    if (p?.dataType === 'edge') {
      const hit = props.edges.find((e) => toEdgeKey(e.source, e.target) === toEdgeKey(p.data?.source, p.data?.target))
      if (hit) emit('edge-click', hit)
      return
    }
    const node = props.nodes.find((n) => n.id === p?.data?.name)
    if (node) emit('node-click', node)
  })
  const zr = chart.getZr()
  zr.off('click')
  zr.on('click', (e: any) => { if (!e?.target) emit('background-click') })
}

/** 仅高亮变化时不必重建 option。 */
function syncHighlight() {
  if (!chart) return
  chart.setOption(buildGraphOption({
    nodes: props.nodes,
    edges: props.edges,
    layout: props.layout,
    palette: props.palette,
    showLegend: props.showLegend,
    highlightId: props.highlightId,
  }))
}

watch(() => [props.nodes, props.edges, props.layout], render)
watch(() => props.highlightId, syncHighlight)

onMounted(render)
onBeforeUnmount(disposeChart)

defineExpose({
  resize: () => { observeContainer(); chart?.resize() },
  /** 重置为初始布局（力导图重新收敛 / 圆形图复位）。 */
  resetView: render,
  /** 导出当前视图为 PNG dataURL（下载由宿主负责）。 */
  toDataURL: () => chart?.getDataURL({ pixelRatio: 2, backgroundColor: '#fff' }) || '',
})
</script>

<style lang="less" scoped>
.graph-force-chart {
  position: relative;
  width: 100%;
  min-height: 240px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: var(--td-bg-color-container);
  overflow: hidden;

  &__canvas {
    width: 100%;
    height: 100%;
  }

  &__empty {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: var(--td-text-color-placeholder);
    font-size: var(--app-text-sm, 13px);
  }
}
</style>
