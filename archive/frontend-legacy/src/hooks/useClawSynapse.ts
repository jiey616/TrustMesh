import { useQuery } from '@tanstack/react-query'
import * as clawSynapseApi from '@/api/clawsynapse'

export function useClawSynapseHealth() {
  return useQuery({
    queryKey: ['clawsynapse', 'health'],
    queryFn: async () => {
      const res = await clawSynapseApi.getClawSynapseHealth()
      return res.data
    },
    staleTime: 10_000,
    refetchInterval: 10_000,
  })
}
