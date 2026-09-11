import { apiClient } from './client'
import type { ApiResponse } from '@/types'

export interface ClawSynapseHealth {
  did: string
  node_id: string
  online: boolean
  trust_mode: string
}

export async function getClawSynapseHealth() {
  return apiClient.get('clawsynapse/health').json<ApiResponse<ClawSynapseHealth>>()
}
