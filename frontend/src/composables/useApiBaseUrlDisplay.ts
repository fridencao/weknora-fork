import { computed } from 'vue'
import { getApiBaseUrl } from '@/utils/api-base'

export function useApiBaseUrlDisplay() {
  const apiBaseUrlDisplay = computed(() => {
    const configured = getApiBaseUrl().trim().replace(/\/$/, '')
    let origin = typeof window !== 'undefined' ? window.location.origin : ''
    if (!origin || origin === 'null') {
      origin = ''
    }
    const base = configured || origin
    return `${base}/api/v1`
  })

  return { apiBaseUrlDisplay }
}
