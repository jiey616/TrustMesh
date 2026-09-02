import { useQuery } from '@tanstack/react-query'
import * as marketApi from '@/api/market'
import type { ListRolesParams } from '@/api/market'

const STALE_TIME = 5 * 60 * 1000 // 5 分钟（静态数据）

export function useMarketDepts() {
  return useQuery({
    queryKey: ['market-depts'],
    queryFn: async () => {
      const res = await marketApi.listDepts()
      return res.data.items
    },
    staleTime: STALE_TIME,
  })
}

export function useMarketRoles(params?: ListRolesParams) {
  return useQuery({
    queryKey: ['market-roles', params],
    queryFn: async () => {
      const res = await marketApi.listRoles(params)
      return res.data.items
    },
    staleTime: STALE_TIME,
  })
}

export function useMarketRole(id: string | undefined) {
  return useQuery({
    queryKey: ['market-roles', 'detail', id],
    queryFn: async () => {
      const res = await marketApi.getRole(id!)
      return res.data
    },
    enabled: !!id,
    staleTime: STALE_TIME,
  })
}
