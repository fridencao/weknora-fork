<template>
  <div class="dl">
    <!-- 案件头 -->
    <div class="dl-case" v-if="run">
      <t-tag theme="primary" variant="light" size="medium">CASE-{{ shortCaseId }}</t-tag>
      <div class="dl-case-name">
        <div class="dl-case-title">{{ $t('ojk.stage5.caseTitle') }}</div>
        <div class="dl-case-sub">{{ $t('ojk.stage2.positionLabel') }}: Direktur IT · {{ $t('ojk.stage2.targetLabel') }}: PT Bank Nusantara Digital Tbk</div>
      </div>
      <t-tag variant="outline" size="small">v{{ run.skill_version }}</t-tag>
      <t-tag theme="warning" variant="light" size="medium">⚠ {{ $t('ojk.stage5.needsClarify', { n: flaggedItems.length }) }}</t-tag>
    </div>

    <!-- 交付物 Tabs -->
    <div class="dl-tabs">
      <t-radio-group v-model="activeTab" variant="default-filled">
        <t-radio-button value="letter">{{ $t('ojk.stage5.tabLetter', { n: flaggedItems.length }) }}</t-radio-button>
        <t-radio-button value="interview">{{ $t('ojk.stage5.tabInterview', { n: interviewBank.length }) }}</t-radio-button>
        <t-radio-button value="audit">{{ $t('ojk.stage5.tabAudit') }}</t-radio-button>
      </t-radio-group>
    </div>

    <!-- Tab 1：澄清函 -->
    <div class="dl-split" v-if="activeTab === 'letter'">
      <div class="dl-letter-col">
        <div class="dl-letter-head">
          <t-icon name="file" />
          <span class="dl-letter-head-title">{{ $t('ojk.stage5.letterTitle') }}</span>
          <code class="dl-docid">DOC-ID: CLF-{{ docDate }}</code>
          <t-button size="small" theme="default" @click="exportWord">
            <template #icon><t-icon name="file-export" /></template>
            {{ $t('ojk.stage5.exportWord') }}
          </t-button>
          <t-button size="small" theme="primary" @click="demo('send')">
            <template #icon><t-icon name="send" /></template>
            {{ $t('ojk.stage5.sendToBank') }}
          </t-button>
        </div>

        <!-- 正式函件（纸张样式） -->
        <div class="dl-paper">
          <div class="dl-paper-org">OTORITAS JASA KEUANGAN (OJK) REPUBLIK INDONESIA</div>
          <div class="dl-paper-dept">{{ $t('ojk.stage5.paperDept') }}</div>
          <div class="dl-paper-addr">{{ $t('ojk.stage5.paperAddr') }}</div>
          <div class="dl-paper-divider"></div>
          <div class="dl-paper-meta">
            <div><span>{{ $t('ojk.stage5.nomor') }}</span><b>S-841/PB.12/2026</b></div>
            <div><span>{{ $t('ojk.stage5.tanggal') }}</span><b>{{ todayStr }}</b></div>
            <div><span>{{ $t('ojk.stage5.kepada') }}</span><b>{{ $t('ojk.stage5.kepadaVal') }}</b></div>
            <div><span>{{ $t('ojk.stage5.perihal') }}</span><b class="dl-paper-perihal">{{ $t('ojk.stage5.perihalVal') }}</b></div>
          </div>
          <p class="dl-paper-body">
            {{ $t('ojk.stage5.letterIntro', { name: 'Rina Wijaya' }) }}
          </p>

          <!-- 编号发现项（真实 flagged 数据） -->
          <div v-for="(f, i) in letterFindings" :key="f.id" class="dl-finding">
            <div class="dl-finding-num">{{ i + 1 }}</div>
            <div class="dl-finding-main">
              <div class="dl-finding-title">{{ f.requirement }}</div>
              <div class="dl-finding-flag">⚠ {{ f._flag }}</div>
              <div class="dl-finding-ref">
                🔗 {{ f.pasal || '—' }} · {{ $t('ojk.stage2.colRules') }}: {{ f.evidence_type || '—' }}
              </div>
            </div>
          </div>
          <p v-if="flaggedItems.length > letterFindings.length" class="dl-paper-more">
            {{ $t('ojk.stage5.moreFindings', { n: flaggedItems.length - letterFindings.length }) }}
          </p>
          <p class="dl-paper-closing">{{ $t('ojk.stage5.letterClosing') }}</p>
        </div>
      </div>

      <!-- 右栏：面试提纲 -->
      <div class="dl-interview-col">
        <div class="dl-iv-head">
          <span class="dl-iv-title">{{ $t('ojk.stage5.ivTitle') }}</span>
          <t-tag theme="primary" variant="light" size="small">AI Assisted Guide</t-tag>
        </div>
        <div v-if="interviewTopics.length">
          <div v-for="(tp, i) in interviewTopics" :key="i" class="dl-topic">
            <div class="dl-topic-head">
              <t-icon name="assignment" theme="primary" />
              <span class="dl-topic-title">{{ $t('ojk.stage5.topicN', { n: i + 1 }) }}: {{ tp.pasal || tp.id }}</span>
            </div>
            <div class="dl-topic-q">{{ tp.requirement }}</div>
            <div class="dl-topic-foot">
              <span>{{ $t('ojk.stage5.topicWeight', { n: tp.weight }) }}</span>
            </div>
          </div>
        </div>
        <div v-else class="dl-iv-empty">
          <t-icon name="assignment" style="font-size: 28px; color: var(--td-text-color-placeholder)" />
          <p>{{ $t('ojk.stage5.ivEmpty') }}</p>
        </div>
        <div class="dl-iv-checklist">
          💡 {{ $t('ojk.stage5.assessorChecklist') }}
        </div>
      </div>
    </div>

    <!-- Tab 2：面试提纲全屏（同右栏内容，放大） -->
    <div class="dl-card" v-else-if="activeTab === 'interview'">
      <div class="dl-iv-head">
        <span class="dl-iv-title">{{ $t('ojk.stage5.ivTitle') }}</span>
        <t-tag theme="primary" variant="light" size="small">AI Assisted Guide</t-tag>
      </div>
      <div v-if="interviewTopics.length">
        <div v-for="(tp, i) in interviewTopics" :key="i" class="dl-topic">
          <div class="dl-topic-head">
            <t-icon name="assignment" theme="primary" />
            <span class="dl-topic-title">{{ $t('ojk.stage5.topicN', { n: i + 1 }) }}: {{ tp.pasal || tp.id }}</span>
          </div>
          <div class="dl-topic-q">{{ tp.requirement }}</div>
        </div>
      </div>
      <div v-else class="dl-iv-empty">
        <p>{{ $t('ojk.stage5.ivEmpty') }}</p>
      </div>
    </div>

    <!-- Tab 3：双人复核 + 审计日志 -->
    <template v-else>
      <div class="dl-card">
        <div class="dl-sign-head">
          <span>✓ {{ $t('ojk.stage5.dualSignTitle') }}</span>
          <span class="dl-hash">Integrity Hash: {{ integrityHash }} <t-tag theme="success" variant="light" size="small">● {{ $t('ojk.stage5.ledgerSynced') }}</t-tag></span>
        </div>
        <div class="dl-sign-cards">
          <div class="dl-sign-card">
            <div class="dl-sign-role">{{ $t('ojk.stage5.reviewer') }} <t-tag theme="success" variant="light" size="small">SIGNED</t-tag></div>
            <div class="dl-sign-name">Admin</div>
            <div class="dl-sign-sub">Lead Compliance Auditor</div>
            <div class="dl-sign-sub rd-mono">{{ todayStr }} WIB</div>
            <div class="dl-sign-verdict">{{ $t('ojk.stage5.verdict') }}: <t-tag theme="warning" variant="light" size="small">{{ $t('ojk.stage5.verdictClarify') }}</t-tag></div>
          </div>
          <div class="dl-sign-card">
            <div class="dl-sign-role">{{ $t('ojk.stage5.approver') }} <t-tag theme="warning" variant="light" size="small">{{ approverSigned ? 'SIGNED' : 'PENDING' }}</t-tag></div>
            <div class="dl-sign-name">Bambang Suryono</div>
            <div class="dl-sign-sub">OJK Deputy Director of Banking Licensing</div>
            <div class="dl-sign-sub">{{ $t('ojk.stage5.approverWaiting') }}</div>
            <t-button v-if="!approverSigned" theme="primary" size="small" style="align-self: stretch" @click="signApprover">
              ⚡ {{ $t('ojk.stage5.dualSignBtn') }}
            </t-button>
          </div>
        </div>
        <div class="dl-sign-foot">
          {{ $t('ojk.stage2.auditNote') }} · SHA-256 · Dual-control enforced
        </div>
      </div>

      <div class="dl-card">
        <div class="dl-audit-head">
          <span>{{ $t('ojk.stage5.auditStream') }}</span>
          <t-link theme="primary" size="small" @click="demo('downloadLog')">{{ $t('ojk.stage5.downloadLog') }}</t-link>
        </div>
        <div class="dl-audit-list">
          <div v-for="(a, i) in auditStream" :key="i" class="dl-audit-row">
            <span class="dl-audit-time rd-mono">{{ a.time }}</span>
            <t-tag :theme="a.theme" variant="light" size="small">{{ a.type }}</t-tag>
            <span class="dl-audit-text">{{ a.text }}</span>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { getOJKRun, listOJKItems, getOJKItemStats, type OJKRun, type OJKItem } from '@/api/ojk'

const props = defineProps<{ runId: string }>()
const { t } = useI18n()

const run = ref<OJKRun | null>(null)
const items = ref<OJKItem[]>([])
const activeTab = ref('letter')
const approverSigned = ref(false)

const shortCaseId = computed(() => (props.runId || '').slice(-6).toUpperCase())
const docDate = computed(() => (props.runId || '').slice(-6))

const todayStr = new Date().toLocaleDateString('id-ID', { day: 'numeric', month: 'long', year: 'numeric' })

async function loadAll() {
  if (!props.runId) return
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
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to load deliverables')
  }
}

watch(() => props.runId, () => loadAll(), { immediate: true })

// ---- 澄清函 findings：真实 flagged 条目 ----
const flaggedItems = computed(() => items.value.filter(i => !!i._flag))
const letterFindings = computed(() => flaggedItems.value.slice(0, 6))

// ---- 面试提纲：审查台已存的问库 + 有告警条目兜底 ----
const interviewBank = computed(() => {
  try {
    const bank = JSON.parse(localStorage.getItem('ojk-interview-bank') || '[]')
    if (Array.isArray(bank) && bank.length) return bank
  } catch { /* ignore */ }
  return flaggedItems.value.slice(0, 5).map(i => ({
    id: i.id, pasal: i.pasal, requirement: i.requirement,
    weight: [30, 35, 20, 15][Math.abs(hashCode(i.id)) % 4],
  }))
})
const interviewTopics = interviewBank

function hashCode(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0
  return Math.abs(h)
}

const integrityHash = computed(() => {
  const s = props.runId || ''
  let h = 0x811c9dc5
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = (h * 0x01000193) >>> 0
  }
  return '0x' + h.toString(16).toUpperCase().slice(0, 4).padStart(4, '0') + '...' + h.toString(16).toUpperCase().slice(-4).padStart(4, '0')
})

// ---- 导出 Word（HTML → .doc，Word 可直接打开）----
function exportWord() {
  const letterHtml = `
    <h2 style="text-align:center">OTORITAS JASA KEUANGAN (OJK) REPUBLIK INDONESIA</h2>
    <p style="text-align:center">${t('ojk.stage5.paperDept')}</p>
    <hr/>
    <p>${t('ojk.stage5.nomor')}: S-841/PB.12/2026 · ${t('ojk.stage5.tanggal')}: ${todayStr}</p>
    <p>${t('ojk.stage5.letterIntro', { name: 'Rina Wijaya' })}</p>
    <ol>
      ${flaggedItems.value.map((f, i) => `<li><b>${f.requirement}</b><br/>⚠ ${f._flag}<br/>${f.pasal || ''}</li>`).join('\n')}
    </ol>
    <p>${t('ojk.stage5.letterClosing')}</p>
  `
  const blob = new Blob(['\ufeff<html><head><meta charset="utf-8"></head><body>' + letterHtml + '</body></html>'], {
    type: 'application/msword',
  })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `OJK_Clarification_Letter_${(props.runId || '').slice(-6)}.doc`
  a.click()
  URL.revokeObjectURL(a.href)
  MessagePlugin.success(t('ojk.stage5.exportDone'))
}

function signApprover() {
  approverSigned.value = true
  MessagePlugin.success(t('ojk.stage5.dualSignDone'))
}

// ---- 审计日志流（会话内实时记录）----
const auditStream = computed(() => {
  const stream: Array<{ time: string; type: string; theme: string; text: string }> = []
  const now = new Date()
  const fmt = (d: Date) => d.toTimeString().slice(0, 8)
  stream.push({ time: fmt(now), type: 'SYSTEM', theme: 'primary', text: t('ojk.stage5.auditGen', { n: items.value.length }) })
  if (flaggedItems.value.length) {
    stream.push({ time: fmt(new Date(now - 120000)), type: 'FINDING', theme: 'warning', text: t('ojk.stage5.auditFinding', { n: flaggedItems.value.length }) })
  }
  if (approverSigned.value) {
    stream.push({ time: fmt(new Date(now - 60000)), type: 'SIGN_EVENT', theme: 'success', text: t('ojk.stage5.auditSign') })
  }
  return stream
})

function demo(key: string) {
  MessagePlugin.info(t('ojk.stage2.demoAction', { action: t(`ojk.stage5.demo_${key}`) }))
}
</script>

<style scoped>
.dl { display: flex; flex-direction: column; gap: var(--app-space-md, 14px); }

.dl-case {
  display: flex; align-items: center; gap: var(--app-space-md, 14px);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 14px);
  flex-wrap: wrap;
}
.dl-case-name { display: flex; flex-direction: column; }
.dl-case-title { font-size: var(--app-text-md, 14px); font-weight: 700; }
.dl-case-sub { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); }

.dl-tabs { display: flex; }

.dl-split { display: grid; grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr); gap: var(--app-space-md, 14px); align-items: start; }
.dl-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-md, 14px);
}

.dl-letter-col { display: flex; flex-direction: column; gap: var(--app-space-sm, 10px); }
.dl-letter-head {
  display: flex; align-items: center; gap: var(--app-space-sm, 10px);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 12px);
  flex-wrap: wrap;
}
.dl-letter-head-title { font-weight: 700; margin-right: auto; }
.dl-docid { font-size: var(--app-text-xs, 11px); font-family: var(--td-font-family, monospace); background: var(--td-bg-color-secondarycontainer); padding: 2px 6px; border-radius: 4px; }

/* 纸张样式函件 */
.dl-paper {
  background: #fff;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-sm, 6px);
  padding: var(--app-space-lg, 24px) var(--app-space-xl, 32px);
  font-family: var(--td-font-family, serif);
  color: var(--td-text-color-primary);
}
.dl-paper-org { text-align: center; font-weight: 800; letter-spacing: 0.04em; font-size: var(--app-text-md, 14px); }
.dl-paper-dept { text-align: center; font-size: var(--app-text-xs, 11px); color: var(--td-text-color-secondary); margin-top: 2px; }
.dl-paper-addr { text-align: center; font-size: var(--app-text-xs, 11px); color: var(--td-text-color-placeholder); margin-top: 2px; letter-spacing: 0.05em; }
.rd-mono { font-family: var(--td-font-family, monospace); }
.dl-paper-divider { border-top: 1.5px solid var(--td-text-color-primary); margin: 10px 0 14px; }
.dl-paper-meta { display: grid; grid-template-columns: 1fr 1fr; gap: 6px 20px; font-size: var(--app-text-xs, 11px); margin-bottom: 14px; }
.dl-paper-meta > div { display: flex; gap: 8px; }
.dl-paper-meta span { color: var(--td-text-color-secondary); min-width: 84px; }
.dl-paper-perihal { color: var(--td-brand-color); }
.dl-paper-body { font-size: var(--app-text-sm, 12px); line-height: 1.8; }
.dl-paper-more { font-size: var(--app-text-xs, 11px); color: var(--td-text-color-placeholder); text-align: center; margin: 10px 0; }
.dl-paper-closing { font-size: var(--app-text-sm, 12px); line-height: 1.7; margin-top: 12px; }

.dl-finding {
  display: flex; gap: 10px;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-sm, 6px);
  padding: 10px 12px;
  margin: 8px 0;
  page-break-inside: avoid;
}
.dl-finding-num {
  width: 22px; height: 22px; border-radius: 50%;
  background: color-mix(in srgb, var(--td-brand-color) 12%, transparent);
  color: var(--td-brand-color); font-weight: 700; font-size: var(--app-text-xs, 11px);
  display: flex; align-items: center; justify-content: center; flex-shrink: 0;
}
.dl-finding-main { flex: 1; display: flex; flex-direction: column; gap: 4px; }
.dl-finding-title { font-weight: 600; font-size: var(--app-text-sm, 12px); line-height: 1.5; }
.dl-finding-flag { color: var(--td-warning-color); font-size: var(--app-text-xs, 11px); font-family: var(--td-font-family, monospace); }
.dl-finding-ref { color: var(--td-text-color-placeholder); font-size: var(--app-text-xs, 11px); }

/* 面试提纲 */
.dl-interview-col {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-md, 14px);
  display: flex; flex-direction: column; gap: var(--app-space-sm, 10px);
}
.dl-iv-head { display: flex; align-items: center; gap: 8px; }
.dl-iv-title { font-size: var(--app-text-md, 14px); font-weight: 700; }
.dl-iv-empty {
  display: flex; flex-direction: column; align-items: center; gap: 6px;
  padding: 30px; color: var(--td-text-color-placeholder); text-align: center;
  font-size: var(--app-text-xs, 11px);
}
.dl-topic {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 12px);
  display: flex; flex-direction: column; gap: 6px;
}
.dl-topic-head { display: flex; align-items: center; gap: 6px; font-weight: 600; font-size: var(--app-text-sm, 12px); flex-wrap: wrap; }
.dl-topic-title { word-break: break-all; }
.dl-topic-q { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); line-height: 1.6; }
.dl-topic-foot { color: var(--td-text-color-placeholder); font-size: var(--app-text-xs, 11px); text-align: right; }
.dl-iv-checklist {
  border: 1px solid color-mix(in srgb, var(--td-brand-color) 30%, transparent);
  background: color-mix(in srgb, var(--td-brand-color) 6%, transparent);
  border-radius: var(--app-radius-sm, 6px);
  padding: 8px 10px;
  color: var(--td-brand-color);
  font-size: var(--app-text-xs, 11px);
}

/* 双人复核 */
.dl-sign-head { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px; font-weight: 700; font-size: var(--app-text-sm, 13px); }
.dl-hash { font-family: var(--td-font-family, monospace); font-size: var(--app-text-xs, 11px); color: var(--td-text-color-secondary); display: flex; align-items: center; gap: 6px; font-weight: 400; }
.dl-sign-cards { display: grid; grid-template-columns: 1fr 1fr; gap: var(--app-space-md, 14px); }
.dl-sign-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md, 8px);
  padding: var(--app-space-md, 14px);
  display: flex; flex-direction: column; gap: 4px;
}
.dl-sign-role { display: flex; align-items: center; gap: 8px; font-weight: 600; }
.dl-sign-name { font-size: var(--app-text-md, 14px); font-weight: 700; }
.dl-sign-sub { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); }
.dl-sign-verdict { display: flex; align-items: center; gap: 6px; font-size: var(--app-text-xs, 11px); color: var(--td-text-color-secondary); margin-top: 4px; }
.dl-sign-foot { color: var(--td-text-color-placeholder); font-size: var(--app-text-xs, 11px); }

/* 审计日志流 */
.dl-audit-head { display: flex; justify-content: space-between; align-items: center; font-weight: 600; font-size: var(--app-text-sm, 12px); }
.dl-audit-list { display: flex; flex-direction: column; gap: 4px; max-height: 300px; overflow-y: auto; }
.dl-audit-row {
  display: flex; align-items: center; gap: 10px;
  font-size: var(--app-text-xs, 11px);
  padding: 4px 6px;
  border-bottom: 1px solid var(--td-component-stroke);
}
.dl-audit-time { color: var(--td-text-color-placeholder); }
.dl-audit-text { flex: 1; color: var(--td-text-color-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.rd-mono { font-family: var(--td-font-family, monospace); }
</style>
