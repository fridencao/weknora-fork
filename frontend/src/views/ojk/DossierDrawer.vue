<template>
  <t-drawer
    :visible="visible"
    attach="body"
    size="62%"
    :header="false"
    :footer="false"
    :close-btn="false"
    destroy-on-close
    @close="emitClose"
  >
    <div class="dd" v-if="c">
      <!-- 头部概览 -->
      <div class="dd-head">
        <div class="dd-avatar">{{ initials }}</div>
        <div class="dd-head-main">
          <div class="dd-name">
            {{ c.name }}
            <t-tag theme="success" variant="light" size="small">DOC AI 100% {{ $t('ojk.stage2.parsedReady') }}</t-tag>
          </div>
          <div class="dd-head-sub">
            <span class="rd-mono">NIK: {{ c.nik }}</span>
            <span>·</span>
            <span>{{ $t('ojk.stage2.positionLabel') }}: <b>{{ c.position }}</b></span>
            <span>·</span>
            <span>{{ c.institution }}</span>
          </div>
        </div>
        <t-space>
          <t-button variant="text" shape="square"><template #icon><t-icon name="fullscreen" /></template></t-button>
          <t-button variant="text" shape="square" @click="emitClose">
            <template #icon><t-icon name="close" /></template>
          </t-button>
        </t-space>
      </div>

      <!-- 四元信息卡 -->
      <div class="dd-meta">
        <div class="dd-meta-cell">
          <span class="lbl">{{ $t('ojk.stage2.dossierVolume') }}</span>
          <div class="val">{{ c.pages }}<span class="unit">{{ $t('ojk.stage2.pagesUnit') }}</span></div>
          <div class="sub">{{ c.files }} {{ $t('ojk.stage2.filesUnit') }}</div>
        </div>
        <div class="dd-meta-cell">
          <span class="lbl">{{ $t('ojk.stage2.ocrConfidence') }}</span>
          <div class="val val-brand">{{ ocrAvg }}%</div>
          <div class="sub">{{ $t('ojk.stage2.tokenLevel') }}</div>
        </div>
        <div class="dd-meta-cell">
          <span class="lbl">{{ $t('ojk.stage2.benchmarkRules') }}</span>
          <div class="val val-brand">{{ rulesBound }}{{ $t('ojk.stage2.rulesUnit') }}</div>
          <div class="sub">{{ $t('ojk.stage2.boundRegs', { n: checkedPolicies.length }) }}</div>
        </div>
        <div class="dd-meta-cell">
          <span class="lbl">{{ $t('ojk.stage2.riskWarnings') }}</span>
          <div class="val">0 <span class="unit">{{ $t('ojk.stage2.flawsUnit') }}</span></div>
          <div class="sub">{{ $t('ojk.stage2.completeness') }} 100%</div>
        </div>
      </div>

      <!-- 申请人材料摄入与多模态解析包 -->
      <div class="dd-section">
        <div class="dd-sec-head">
          <t-icon name="folder-open" />
          <span>{{ $t('ojk.stage2.leftTitle') }}</span>
          <code class="dd-engine">DOC AI VLM 4.1</code>
          <t-tag theme="success" variant="light" size="small">100% {{ $t('ojk.stage2.structuredReady') }}</t-tag>
          <t-button size="small" theme="default" style="margin-left: auto" @click="demo('addFile')">
            <template #icon><t-icon name="add" /></template>
            {{ $t('ojk.stage2.addSingleFile') }}
          </t-button>
        </div>
        <p class="dd-sec-desc">{{ $t('ojk.stage2.ddLeftDesc') }}</p>
        <div v-for="f in files" :key="f.name" class="dd-file">
          <t-icon :name="f.icon" class="dd-file-icon" />
          <div class="dd-file-main">
            <div class="dd-file-name">
              {{ f.name }}
              <t-tag variant="light" size="small" theme="default">P.{{ f.pages }}</t-tag>
              <t-tag variant="light" size="small" theme="default">{{ f.pageCount }} {{ $t('ojk.stage2.pagesShort') }}</t-tag>
            </div>
            <div class="dd-file-desc">{{ f.desc }}</div>
          </div>
          <div class="dd-file-right">
            <t-tag :theme="f.statusTheme" variant="light" size="small">{{ f.statusText }}</t-tag>
            <t-tag variant="light" size="small" theme="default">{{ $t('ojk.stage2.tokenLabel') }}: {{ f.token }}%</t-tag>
          </div>
          <t-button variant="text" shape="square" size="small">
            <template #icon><t-icon name="browse" /></template>
          </t-button>
        </div>
      </div>

      <!-- 岗位-法规清单匹配规则配置 -->
      <div class="dd-section">
        <div class="dd-sec-head">
          <t-icon name="layers" />
          <span>{{ $t('ojk.stage2.rightTitle') }}</span>
          <t-tag variant="outline" size="small">Target: {{ c.position.split(' ')[0] }}</t-tag>
          <t-button size="small" theme="default" variant="text" style="margin-left: auto" @click="demo('customConfig')">
            <template #icon><t-icon name="setting" /></template>
            {{ $t('ojk.stage2.customConfig') }}
          </t-button>
        </div>
        <p class="dd-sec-desc">{{ $t('ojk.stage2.ddRuleDesc') }}</p>
        <div v-for="p in policies" :key="p.name" class="dd-policy" :class="{ off: !p.checked }">
          <t-checkbox :checked="p.checked" @change="(v: unknown) => togglePolicy(p.name, v)">
            <span class="dd-policy-name">{{ p.name }}</span>
          </t-checkbox>
          <t-tag theme="primary" variant="light" size="small" class="dd-policy-tag">{{ p.tag }}</t-tag>
          <div class="dd-policy-count">{{ p.count }} {{ $t('ojk.stage2.rulesApply') }}</div>
          <div class="dd-policy-desc">{{ p.desc }}</div>
        </div>
      </div>

      <!-- 底栏三动作 -->
      <div class="dd-footer">
        <t-button theme="default" @click="demo('saveConfig')">
          <template #icon><t-icon name="save" /></template>
          {{ $t('ojk.stage2.saveClose') }}
        </t-button>
        <t-button theme="default" @click="demo('ocrPreview')">
          <template #icon><t-icon name="root-list" /></template>
          {{ $t('ojk.stage2.ocrPreview') }}
        </t-button>
        <t-button theme="primary" style="margin-left: auto" @click="startAiCheck">
          {{ $t('ojk.stage2.startAiCheck') }}
          <template #icon><t-icon name="arrow-right" /></template>
        </t-button>
      </div>
    </div>
  </t-drawer>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'

export interface DossierFile {
  name: string
  pages: string
  pageCount: number
  desc: string
  statusText: string
  statusTheme: string
  token: string
  icon: string
}

export interface DossierCandidate {
  name: string
  nik: string
  position: string
  institution: string
  pages: number
  files: number
  rules: number
  ocrAvg?: string
  files_detail?: DossierFile[]
}

const props = defineProps<{ visible: boolean; candidate: DossierCandidate | null }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'close'): void }>()

const { t } = useI18n()

const c = computed(() => props.candidate)
const initials = computed(() =>
  (c.value?.name || '').split(/\s+/).map(w => w[0]).slice(0, 2).join('').toUpperCase())

const ocrAvg = computed(() => c.value?.ocrAvg || '99.4')
const rulesBound = computed(() => {
  // 勾选的法规模块条数合计（默认全选）
  return policies.value.filter(p => p.checked).reduce((s, p) => s + p.count, 0)
})

// 法规匹配（演示数据：按候选人岗位默认编排，可勾选调整）
const policies = ref([
  {
    name: 'POJK No. 27/POJK.03/2016',
    tag: t('ojk.stage2.polTagFit'),
    count: 35,
    desc: t('ojk.stage2.polDescFit'),
    checked: true,
  },
  {
    name: 'SEOJK No. 39/SEOJK.03/2016',
    tag: t('ojk.stage2.polTagSeo'),
    count: 22,
    desc: t('ojk.stage2.polDescSeo'),
    checked: true,
  },
  {
    name: 'POJK No. 11/POJK.03/2022 (IT专项)',
    tag: t('ojk.stage2.polTagIt'),
    count: 14,
    desc: t('ojk.stage2.polDescIt'),
    checked: true,
  },
])

function togglePolicy(name: string, v: unknown) {
  policies.value = policies.value.map(p => (p.name === name ? { ...p, checked: !!v } : p))
}

// 材料文件明细：Rina 用原型全量数据；其他候选人生成通用骨架
const RINA_FILES: DossierFile[] = [
  { name: 'Curriculum Vitae_Rina_Wijaya_2024.pdf', pages: '1 - 18', pageCount: 18, desc: `${t('ojk.stage2.fileCvDesc')}`, statusText: t('ojk.stage2.fileChecked'), statusTheme: 'success', token: '99.8', icon: 'file' },
  { name: 'SKCK_Polri_PoldaMetroJaya_2024.pdf', pages: '19 - 22', pageCount: 4, desc: t('ojk.stage2.fileSkckDesc'), statusText: t('ojk.stage2.fileSkckStatus'), statusTheme: 'success', token: '99.1', icon: 'secured' },
  { name: 'SLIK_OJK_Credit_History_Dossier.pdf', pages: '23 - 88', pageCount: 66, desc: t('ojk.stage2.fileSlikDesc'), statusText: t('ojk.stage2.fileSlikStatus'), statusTheme: 'success', token: '99.6', icon: 'chart' },
  { name: 'Ijazah_S2_ITB_Kemenristekdikti_Penyetaraan.pdf', pages: '89 - 110', pageCount: 22, desc: t('ojk.stage2.fileIjazahDesc'), statusText: t('ojk.stage2.fileIjazahStatus'), statusTheme: 'success', token: '98.9', icon: 'education' },
  { name: 'Surat_Pernyataan_Integritas_Materai_10000.pdf', pages: '111 - 158', pageCount: 48, desc: t('ojk.stage2.fileSuratDesc'), statusText: t('ojk.stage2.fileSuratStatus'), statusTheme: 'warning', token: '99.4', icon: 'wallet' },
]

const files = computed<DossierFile[]>(() => {
  if (!c.value) return []
  if (c.value.files_detail?.length) return c.value.files_detail
  const name = c.value.name.replace(/\s+/g, '_')
  const generic = (suffix: string, pages: string, n: number, icon: string): DossierFile => ({
    name: `${suffix}_${name}.pdf`, pages, pageCount: n,
    desc: t('ojk.stage2.fileGenericDesc'), statusText: t('ojk.stage2.fileGenericStatus'),
    statusTheme: 'success', token: '99.0', icon,
  })
  return [
    generic('Curriculum_Vitae', '1 - 20', 20, 'file'),
    generic('SKCK_Polri', '21 - 24', 4, 'secured'),
    generic('SLIK_OJK', '25 - 90', 66, 'chart'),
    generic('Ijazah_Dikti', '91 - 120', 30, 'education'),
  ]
})

function demo(key: string) {
  MessagePlugin.info(t('ojk.stage2.demoAction', { action: t(`ojk.stage2.demo_${key}`) }))
}

function startAiCheck() {
  // 候选人材料库建库后挂双库跳转核验；当前为阶段 2 演示动作
  MessagePlugin.info(t('ojk.stage2.startAiCheckDemo'))
}

function emitClose() {
  emit('update:visible', false)
  emit('close')
}
</script>

<style scoped>
.dd { display: flex; flex-direction: column; gap: var(--app-space-md, 14px); font-size: var(--app-text-sm, 13px); }

.dd-head { display: flex; align-items: center; gap: var(--app-space-md, 14px); }
.dd-avatar {
  width: 52px; height: 52px; border-radius: var(--app-radius-md, 8px);
  background: color-mix(in srgb, var(--td-brand-color) 12%, transparent);
  color: var(--td-brand-color); font-weight: 700; font-size: var(--app-text-lg, 16px);
  display: flex; align-items: center; justify-content: center;
}
.dd-head-main { display: flex; flex-direction: column; gap: 4px; }
.dd-name { font-size: var(--app-text-lg, 16px); font-weight: 700; display: flex; align-items: center; gap: 8px; }
.dd-head-sub { display: flex; gap: 8px; flex-wrap: wrap; color: var(--td-text-color-secondary); }
.rd-mono { font-family: var(--td-font-family, monospace); }

.dd-meta { display: grid; grid-template-columns: repeat(4, 1fr); gap: var(--app-space-sm, 10px); }
.dd-meta-cell {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px);
  display: flex; flex-direction: column; gap: 2px;
}
.lbl { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); }
.val { font-size: 20px; font-weight: 700; }
.val-brand { color: var(--td-brand-color); }
.unit { font-size: var(--app-text-xs, 11px); color: var(--td-text-color-secondary); font-weight: 400; margin-left: 2px; }
.sub { color: var(--td-text-color-placeholder); font-size: var(--app-text-xs, 11px); }

.dd-section { display: flex; flex-direction: column; gap: var(--app-space-sm, 10px); }
.dd-sec-head { display: flex; align-items: center; gap: var(--app-space-xs, 6px); font-size: var(--app-text-md, 14px); font-weight: 600; }
.dd-engine { font-size: var(--app-text-xs, 11px); color: var(--td-text-color-secondary); background: var(--td-bg-color-secondarycontainer); padding: 2px 6px; border-radius: 4px; }
.dd-sec-desc { color: var(--td-text-color-secondary); margin: 0; line-height: 1.5; }

.dd-file {
  display: flex; align-items: center; gap: var(--app-space-sm, 10px);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 12px);
}
.dd-file-icon { color: var(--td-brand-color); font-size: 20px; }
.dd-file-main { flex: 1; display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.dd-file-name { font-weight: 600; display: flex; align-items: center; gap: 6px; flex-wrap: wrap; word-break: break-all; }
.dd-file-desc { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); }
.dd-file-right { display: flex; flex-direction: column; align-items: flex-end; gap: 2px; }

.dd-policy {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 12px);
  display: flex; flex-direction: column; gap: 4px;
}
.dd-policy.off { opacity: 0.45; }
.dd-policy-name { font-weight: 600; }
.dd-policy-tag { align-self: flex-start; margin-left: 24px; }
.dd-policy-count { color: var(--td-brand-color); font-weight: 700; margin-left: auto; font-variant-numeric: tabular-nums; }
.dd-policy-desc { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); padding-left: 24px; }

.dd-footer {
  position: sticky;
  bottom: 0;
  display: flex;
  gap: var(--app-space-md, 14px);
  padding: var(--app-space-sm, 10px) 0;
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-stroke);
}
</style>
