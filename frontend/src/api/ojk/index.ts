import { get, post, patch, del } from '@/utils/request'

export interface OJKRun {
  run_id: string
  tenant_id: number
  kb_id: string
  kb_name?: string
  skill_version: string
  status: 'pending' | 'running' | 'done' | 'failed'
  slices_total?: number
  slices_done?: number
  total_slices?: number
  total_items: number
  flagged_items: number
  error?: string
  created_at: string
  updated_at: string
}

export interface OJKItem {
  id: string
  run_id: string
  regulation: string
  pasal: string
  pasal_text: string
  area?: string
  requirement: string
  requirement_id?: string
  evidence_type?: string
  check_method?: string
  applicable_roles: string[]
  severity: 'critical' | 'clarification' | 'info'
  keywords: string[]
  source: 'normal' | 'penjelasan'
  _flag?: string
  status: 'pending' | 'confirmed' | 'rejected'
  reviewer_note?: string
  created_at: string
  updated_at: string
}

export interface OJKItemStats {
  [status: string]: number
}

export interface OJKItemListResponse {
  items: OJKItem[]
  total: number
  page: number
  page_size: number
}

// skillVersion 不传时由后端自动递增（取历史最大点分数字版本末段 +1）
export function createOJKRun(kbId: string, skillVersion?: string): Promise<OJKRun> {
  const body: Record<string, string> = { kb_id: kbId }
  if (skillVersion) body.skill_version = skillVersion
  return post<OJKRun>('/api/v1/ojk/runs', body)
}

export function listOJKRuns(limit = 20): Promise<{ runs: OJKRun[]; total: number }> {
  return get<OJKRunsResponse>(`/api/v1/ojk/runs?limit=${limit}`)
}

export interface OJKRunsResponse {
  runs: OJKRun[]
  total: number
}


export interface OJKPreflight {
  kb_id: string
  kb_name: string
  docs: number
  pasal_sections: number
}

export function preflightOJK(kbId: string): Promise<OJKPreflight> {
  return get<OJKPreflight>(`/api/v1/ojk/preflight?kb_id=${kbId}`)
}

export function getOJKRun(runId: string): Promise<OJKRun> {
  return get<OJKRun>(`/api/v1/ojk/runs/${runId}`)
}

export function renameOJKRun(runId: string, skillVersion: string): Promise<OJKRun> {
  return patch<OJKRun>(`/api/v1/ojk/runs/${runId}`, { skill_version: skillVersion })
}

export function deleteOJKRun(runId: string): Promise<{ deleted: string }> {
  return del<{ deleted: string }>(`/api/v1/ojk/runs/${runId}`)
}

export function listOJKItems(
  runId: string,
  status?: string,
  page = 1,
  pageSize = 20,
  severity?: string,
  sortBy?: string,
  sortOrder?: string,
): Promise<OJKItemListResponse> {
  const params = new URLSearchParams({ run_id: runId, page: String(page), page_size: String(pageSize) })
  if (status) params.append('status', status)
  if (severity) params.append('severity', severity)
  if (sortBy) {
    params.append('sort_by', sortBy)
    params.append('sort_order', sortOrder || 'asc')
  }
  return get<OJKItemListResponse>(`/api/v1/ojk/items?${params}`)
}

export function resolveOJKItem(itemId: string, status: 'confirmed' | 'rejected', note = '') {
  return patch(`/api/v1/ojk/items/${itemId}/resolve`, { status, note })
}

export function getOJKItemStats(runId: string): Promise<OJKItemStats> {
  return get<OJKItemStats>(`/api/v1/ojk/runs/${runId}/stats`)
}
