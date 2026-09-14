import { useQuery } from '@tanstack/react-query'
import * as agentsApi from '@/api/agents'

export function useAgentInsights(id: string | undefined) {
  return useQuery({
    queryKey: ['agents', id, 'insights'],
    queryFn: async () => {
      const res = await agentsApi.getAgentInsights(id!)
      return res.data
    },
    enabled: !!id,
    staleTime: 30_000,
  })
}
