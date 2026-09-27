import { get, post, patch } from '@/api/utils/request'

export interface OJKRun {
  run_id: string
  tenant_id: number
  skill_version: string
  status: 'pending' | 'running' | 'done' | 'failed'
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

export function createOJKRun(skillVersion = '1.0.0') {
  return post<{ run_id: string }>('/ojk/runs', { skill_version: skillVersion })
}

export function getOJKRun(runId: string): Promise<OJKRun> {
  return get<OJKRun>(`/ojk/runs/${runId}`)
}

export function listOJKItems(runId: string, status?: string, page = 1, pageSize = 20): Promise<OJKItemListResponse> {
  const params = new URLSearchParams({ run_id: runId, page: String(page), page_size: String(pageSize) })
  if (status) params.append('status', status)
  return get<OJKItemListResponse>(`/ojk/items?${params}`)
}

export function resolveOJKItem(itemId: string, status: 'confirmed' | 'rejected', note = '') {
  return patch(`/ojk/items/${itemId}/resolve`, { status, note })
}

export function getOJKItemStats(runId: string): Promise<OJKItemStats> {
  return get<OJKItemStats>(`/ojk/runs/${runId}/stats`)
}
