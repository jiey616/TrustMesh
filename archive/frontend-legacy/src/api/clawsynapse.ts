import { api } from './client'
import type { ApiResponse, ClawSynapseHealth } from '@/types'

export async function getClawSynapseHealth() {
  return api.get('clawsynapse/health').json<ApiResponse<ClawSynapseHealth>>()
}
