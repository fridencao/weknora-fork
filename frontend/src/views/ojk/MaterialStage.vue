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
        <t-button theme="primary" @click="openImport()">
          <template #icon><t-icon name="upload" /></template>
          {{ $t('ojk.stage2.batchIngest') }}
        </t-button>
      </t-space>
    </div>

    <!-- 摄入枢纽 + 吞吐面板 -->
    <div class="cq-hub">
      <div class="cq-drop" @click="openImport()">
        <t-icon name="upload" class="cq-drop-icon" />
        <div class="cq-drop-text">
          <div>
            {{ $t('ojk.stage2.dropTitle') }}
            <t-tag theme="primary" variant="light" size="small">OCR Auto-trigger</t-tag>
          </div>
          <div class="cq-drop-sub">{{ $t('ojk.stage2.dropSub') }}</div>
        </div>
        <t-button theme="default" @click.stop="openImport()">{{ $t('ojk.stage2.chooseLocal') }}</t-button>
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
          <span>{{ parsingCount }} {{ $t('ojk.stage2.tpProcessing') }}</span>
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
        <t-radio-button v-if="countBy('failed')" value="failed">{{ $t('ojk.stage2.tabFailed', { n: countBy('failed') }) }}</t-radio-button>
      </t-radio-group>
      <t-select v-model="instFilter" clearable :placeholder="$t('ojk.stage2.instFilter')" style="width: 160px">
        <t-option v-for="inst in institutions" :key="inst" :value="inst" :label="inst" />
      </t-select>
      <span class="cq-checked">{{ $t('ojk.stage2.checkedN', { n: selected.size }) }}</span>
      <t-button theme="primary" style="margin-left: auto" :disabled="selected.size === 0" @click="refreshSelected">
        <template #icon><t-icon name="play-circle" /></template>
        {{ $t('ojk.stage2.execBatch') }}
      </t-button>
    </div>

    <!-- 候选人表格（自绘行） -->
    <div class="cq-table">
      <div class="cq-tr cq-tr--head">
        <div class="cq-td cq-td--check">
          <t-checkbox :checked="allChecked" @change="(v: unknown) => toggleAll(v)" />
        </div>
        <div class="cq-td">{{ $t('ojk.stage2.colCand') }}</div>
        <div class="cq-td">{{ $t('ojk.stage2.colPosition') }}</div>
        <div class="cq-td">{{ $t('ojk.stage2.colInst') }}</div>
        <div class="cq-td">{{ $t('ojk.stage2.colDossier') }}</div>
        <div class="cq-td">{{ $t('ojk.stage2.colRules') }}</div>
        <div class="cq-td">{{ $t('ojk.stage2.colUpdated') }}</div>
        <div class="cq-td">{{ $t('ojk.stage2.colActions') }}</div>
      </div>
      <div
        v-for="row in pagedCandidates"
        :key="row.id"
        class="cq-tr"
        :class="{ sel: selected.has(row.id) }"
      >
        <div class="cq-td cq-td--check">
          <t-checkbox :checked="selected.has(row.id)" @change="(v: unknown) => toggleOne(row.id, v)" />
        </div>
        <div class="cq-td">
          <div class="cq-cand">
            <div class="cq-avatar">{{ avatar(row.name) }}</div>
            <div>
              <div class="cq-cand-name">
                {{ row.name }}
                <t-icon v-if="row.status === 'parsed'" name="check-circle" theme="success" />
              </div>
              <div class="cq-cand-nik">NIK: {{ row.nik || '—' }}</div>
            </div>
          </div>
        </div>
        <div class="cq-td">{{ row.position || '—' }}</div>
        <div class="cq-td">{{ row.institution || '—' }}</div>
        <div class="cq-td">
          <t-tag
            :theme="row.status === 'parsed' ? 'success' : (row.status === 'parsing' ? 'primary' : (row.status === 'failed' ? 'danger' : 'default'))"
            variant="light" size="small"
          >
            {{ statusText(row) }}
          </t-tag>
        </div>
        <div class="cq-td">—</div>
        <div class="cq-td cq-cell-sub">{{ formatTime(row.created_at) }}</div>
        <div class="cq-td">
          <t-space size="small">
            <t-button theme="primary" size="small" @click="openDossier(row)">
              {{ $t('ojk.stage2.actView') }}
            </t-button>
            <t-button variant="text" shape="square" size="small" @click="demo('rowMore')">
              <template #icon><t-icon name="ellipsis" /></template>
            </t-button>
          </t-space>
        </div>
      </div>
      <div v-if="!pagedCandidates.length" class="cq-empty">
        <t-empty :description="$t('ojk.stage2.drawerIdle')" />
      </div>
    </div>


    <!-- 页脚汇总 -->
    <div class="cq-foot">
      <span>{{ $t('ojk.stage2.tableSummary', { from: 1, to: pagedCandidates.length, n: filteredCandidates.length }) }}</span>
      <span class="cq-foot-mid">{{ $t('ojk.stage2.totalParsedFiles', { files: totalFiles }) }}</span>
    </div>

    <!-- 导入对话框：登记候选人 + 多文件上传 -->
    <t-dialog
      v-model:visible="importVisible"
      :header="$t('ojk.stage2.batchIngest')"
      :confirm-btn="{ content: $t('ojk.stage2.importGo'), loading: importing }"
      width="560px"
      @confirm="submitImport"
    >
      <t-form layout="vertical" label-width="96">
        <t-form-item :label="$t('ojk.stage2.formName')" required-mark>
          <t-input v-model="form.name" :placeholder="$t('ojk.stage2.formNamePh')" />
        </t-form-item>
        <t-form-item :label="$t('ojk.stage2.formNik')">
          <t-input v-model="form.nik" />
        </t-form-item>
        <t-form-item :label="$t('ojk.stage2.formPosition')">
          <t-input v-model="form.position" :placeholder="$t('ojk.stage2.formPositionPh')" />
        </t-form-item>
        <t-form-item :label="$t('ojk.stage2.formInst')">
          <t-input v-model="form.institution" />
        </t-form-item>
        <t-form-item :label="$t('ojk.stage2.formFiles')">
          <div class="cq-file-row">
            <input
              ref="fileInputRef"
              type="file"
              multiple
              accept=".pdf,.zip,.rar"
              style="display: none"
              @change="onFileChosen"
            />
            <t-button theme="default" @click="fileInputRef?.click()">
              <template #icon><t-icon name="attach" /></template>
              {{ $t('ojk.stage2.chooseFilesBtn') }}
            </t-button>
            <div class="cq-file-list">
              <t-tag
                v-for="f in chosenFiles"
                :key="f.name"
                theme="primary" variant="light" size="medium"
                closable
                @close="removeFile(f.name)"
              >
                {{ f.name }}
              </t-tag>
              <span v-if="!chosenFiles.length" class="cq-file-none">{{ $t('ojk.stage2.noFileChosen') }}</span>
            </div>
          </div>
          <div class="cq-file-hint">{{ $t('ojk.stage2.fileHint') }}</div>
        </t-form-item>
      </t-form>
    </t-dialog>

    <!-- 候选人卷宗滑窗 -->
    <DossierDrawer v-model:visible="drawerVisible" :candidate="drawerCandidate" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import DossierDrawer, { type DossierCandidate, type DossierFile } from '@/views/ojk/DossierDrawer.vue'
import {
  listOJKCandidates, createOJKCandidate, getOJKCandidate,
  type OJKCandidate,
} from '@/api/ojk'

const { t } = useI18n()

// ---- 真实候选人数据 ----
const candidates = ref<OJKCandidate[]>([])
const loading = ref(false)
const search = ref('')
const statusTab = ref('')
const instFilter = ref('')
const page = ref(1)
const pageSize = 5
const selected = ref<Set<string>>(new Set())

async function loadCandidates() {
  loading.value = true
  try {
    const res = await listOJKCandidates()
    candidates.value = res.candidates || []
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Failed to load candidates')
  } finally {
    loading.value = false
  }
}
loadCandidates()

const institutions = computed(() => [...new Set(candidates.value.map(c => c.institution).filter(Boolean))])
function countBy(status: string) {
  return candidates.value.filter(c => c.status === status).length
}
const parsingCount = computed(() => countBy('parsing'))

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

const totalFiles = computed(() => candidates.value.reduce((s, c) => s + c.docs, 0))

function toggleAll(v: unknown) {
  selected.value = v ? new Set(pagedCandidates.value.map(c => c.id)) : new Set()
}
function toggleOne(id: string, v: unknown) {
  const next = new Set(selected.value)
  if (v) next.add(id)
  else next.delete(id)
  selected.value = next
}

function statusText(row: OJKCandidate): string {
  if (row.status === 'parsed') return `${t('ojk.stage2.parsedDone')} · ${row.docs}${t('ojk.stage2.filesShort')}`
  if (row.status === 'parsing') return `${t('ojk.stage2.parsingN', { pct: row.parse_pct, pages: row.parsed })}`
  if (row.status === 'failed') return t('ojk.stage2.parseFailed')
  return t('ojk.stage2.queuedNote')
}

const allChecked = computed(() =>
  pagedCandidates.value.length > 0 && pagedCandidates.value.every(c => selected.value.has(c.id)))

function refreshSelected() {
  // 上传即自动解析——此处刷新各候选人解析状态
  loadCandidates()
  MessagePlugin.success(t('ojk.stage2.statusRefreshed'))
}



// ---- 导入对话框：登记候选人 + 多文件上传到其专属 KB ----
const importVisible = ref(false)
const importing = ref(false)
const form = ref({ name: '', nik: '', position: '', institution: '' })
const chosenFiles = ref<File[]>([])

function openImport(files?: File[]) {
  form.value = { name: '', nik: '', position: '', institution: '' }
  chosenFiles.value = files ? [...files] : []
  importVisible.value = true
}

function onFileChosen(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files) chosenFiles.value = [...input.files]
}

function removeFile(name: string) {
  chosenFiles.value = chosenFiles.value.filter(f => f.name !== name)
}

async function submitImport() {
  if (!form.value.name.trim()) {
    MessagePlugin.warning(t('ojk.stage2.formNameRequired'))
    return
  }
  importing.value = true
  try {
    // 1) 登记候选人（自动创建专属材料 KB）
    const cand = await createOJKCandidate({
      name: form.value.name.trim(),
      nik: form.value.nik.trim() || undefined,
      position: form.value.position.trim() || undefined,
      institution: form.value.institution.trim() || undefined,
    })
    // 2) 逐份上传材料到其专属 KB（走现成的解析→向量化管线）
    let uploaded = 0
    for (const f of chosenFiles.value) {
      const fd = new FormData()
      fd.append('file', f)
      const resp = await fetch(`/api/v1/knowledge-bases/${cand.kb_id}/knowledge/file`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${localStorage.getItem('weknora_token')}` },
        body: fd,
      })
      if (resp.ok) uploaded++
    }
    MessagePlugin.success(t('ojk.stage2.importDone', { name: cand.name, files: uploaded }))
    importVisible.value = false
    chosenFiles.value = []
    await loadCandidates()
  } catch (e: any) {
    MessagePlugin.error(e?.message || 'Import failed')
  } finally {
    importing.value = false
  }
}

// ---- 滑窗 ----
const drawerVisible = ref(false)
const drawerCandidate = ref<DossierCandidate | null>(null)

async function openDossier(row: OJKCandidate) {
  let files: DossierFile[] = []
  try {
    const detail = await getOJKCandidate(row.id)
    files = (detail.documents || []).map(doc => ({
      name: doc.Title,
      pages: '—',
      pageCount: 0,
      desc: `${t('ojk.stage2.parseStatusLabel')}: ${parseStatusZh(doc.ParseStatus)}`,
      statusText: parseStatusZh(doc.ParseStatus),
      statusTheme: doc.ParseStatus === 'completed' ? 'success' : (doc.ParseStatus === 'failed' ? 'danger' : 'primary'),
      token: '—',
      icon: 'file',
    }))
  } catch { /* 拉取失败按空态展示 */ }
  drawerCandidate.value = {
    id: row.id, name: row.name, nik: row.nik,
    position: row.position || '', institution: row.institution || '',
    pages: row.docs, files: row.docs, rules: 0,
    files_detail: files,
  }
  drawerVisible.value = true
}

function parseStatusZh(s: string): string {
  return ({
    completed: '已解析', finished: '已解析',
    processing: '解析中', pending: '排队中', finalizing: '解析中',
    failed: '解析失败',
  } as Record<string, string>)[s] || s
}

function avatar(name: string): string {
  return name.split(/\s+/).map(w => w[0]).slice(0, 2).join('').toUpperCase()
}

function formatTime(iso: string): string {
  return (iso || '').replace('T', ' ').slice(5, 16)
}

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
.cq-file-row { display: flex; align-items: center; gap: 10px; width: 100%; }
.cq-file-list { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
.cq-file-none { color: var(--td-text-color-placeholder); font-size: var(--app-text-xs, 11px); }
.cq-file-hint { color: var(--td-text-color-placeholder); font-size: var(--app-text-xs, 11px); }

/* 表格单元格 */
.cq-cand { display: flex; align-items: center; gap: 10px; }
.cq-avatar {
  width: 36px; height: 36px; border-radius: 50%;
  background: color-mix(in srgb, var(--td-brand-color) 14%, transparent);
  color: var(--td-brand-color); font-weight: 700; font-size: var(--app-text-xs, 11px);
  display: flex; align-items: center; justify-content: center;
}
.cq-avatar.sel { background: var(--td-brand-color); color: #fff; }
.cq-cand-name { font-weight: 600; }
.cq-cand-nik { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); font-family: var(--td-font-family, monospace); }
.cq-cell-sub { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); margin-top: 2px; }

/* 页脚 */
.cq-foot {
  display: flex; align-items: center; gap: var(--app-space-md, 14px);
  color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px);
}
.cq-foot-mid { margin-right: auto; }
</style>

<style scoped>
/* 自绘表格（t-table 在本页曾稳定失效，改自绘行——与阶段4工作台同思路） */
.cq-table { border: 1px solid var(--td-component-stroke); border-radius: var(--app-radius-md, 8px); overflow: hidden; background: var(--td-bg-color-container); }
.cq-tr {
  display: grid;
  grid-template-columns: 44px minmax(180px, 1.2fr) minmax(120px, 1fr) minmax(150px, 1fr) minmax(160px, 1.1fr) 80px 100px 130px;
  align-items: center;
  gap: var(--app-space-sm, 10px);
  padding: var(--app-space-sm, 10px) var(--app-space-md, 12px);
  border-bottom: 1px solid var(--td-component-stroke);
}
.cq-tr--head {
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
  font-size: var(--app-text-xs, 11px);
  font-weight: 600;
}
.cq-tr.sel { background: color-mix(in srgb, var(--td-brand-color) 8%, transparent); }
.cq-td--check { display: flex; align-items: center; }
.cq-cand { display: flex; align-items: center; gap: 10px; }
.cq-avatar {
  width: 34px; height: 34px; border-radius: 50%; flex-shrink: 0;
  background: color-mix(in srgb, var(--td-brand-color) 14%, transparent);
  color: var(--td-brand-color); font-weight: 700; font-size: var(--app-text-xs, 11px);
  display: flex; align-items: center; justify-content: center;
}
.cq-cand-name { font-weight: 600; display: flex; align-items: center; gap: 4px; }
.cq-cand-nik { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); font-family: var(--td-font-family, monospace); }
.cq-cell-sub { color: var(--td-text-color-secondary); font-size: var(--app-text-xs, 11px); }
.cq-empty { padding: 40px; display: flex; justify-content: center; }
</style>
