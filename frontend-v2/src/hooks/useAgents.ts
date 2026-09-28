import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as agentsApi from '@/api/agents'
import type { CreateAgentRequest, CronExecutionsResult, SetCapabilityRequest, UpdateAgentRequest } from '@/types'

export function useAgents() {
  return useQuery({
    queryKey: ['agents'],
    queryFn: async () => {
      const res = await agentsApi.listAgents()
      return res.data.items
    },
  })
}

export function useAgent(id: string | undefined) {
  return useQuery({
    queryKey: ['agents', id],
    queryFn: async () => {
      const res = await agentsApi.getAgent(id!)
      return res.data
    },
    enabled: !!id,
  })
}

export function useCreateAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateAgentRequest) => agentsApi.createAgent(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  })
}

export function useUpdateAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateAgentRequest }) => agentsApi.updateAgent(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  })
}

export function useAgentTasks(id: string | undefined, status?: string) {
  return useQuery({
    queryKey: ['agents', id, 'tasks', status ?? 'all'],
    queryFn: async () => {
      const res = await agentsApi.listAgentTasks(id!, status)
      return res.data.items
    },
    enabled: !!id,
    staleTime: 30_000,
  })
}

export function useAgentStats(id: string | undefined) {
  return useQuery({
    queryKey: ['agents', id, 'stats'],
    queryFn: async () => {
      const res = await agentsApi.getAgentStats(id!)
      return res.data
    },
    enabled: !!id,
    staleTime: 30_000,
  })
}

export function useAgentEvents(id: string | undefined, limit = 50) {
  return useQuery({
    queryKey: ['agents', id, 'events', limit],
    queryFn: async () => {
      const res = await agentsApi.getAgentEvents(id!, limit)
      return res.data.items
    },
    enabled: !!id,
    staleTime: 15_000,
  })
}

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

export function useAgentCapabilities(id: string | undefined, enabled = true) {
  return useQuery({
    queryKey: ['agents', id, 'capabilities'],
    queryFn: async () => {
      const res = await agentsApi.getAgentCapabilities(id!)
      /**
       * 🔴 后端在读失败时**仍返回 HTTP 200**，真实状态写在 body 的 available/reason 里
       * （契约见 backend/internal/handler/agent.go 的 GetCapabilities）。
       * 若照原样把 available:false 当「成功数据」交给 RQ，会同时踩两个坑：
       *   ① `retry` 只对**抛出的错误**生效 ⇒ 不会重试，只能靠用户手动刷新；
       *   ② 它会覆盖掉缓存里的上一次成功数据 ⇒ 整个 Tab 退化成错误占位，
       *      而实际只是节点重启刚结束、这一两秒读不到而已。
       * 所以在这一层把「业务失败」翻译成「查询失败」，retry / 保留旧 data / isError
       * 三件事就全部免费拿到了。
       */
      if (!res.data?.available) {
        throw new Error(res.data?.reason || '节点未响应能力查询')
      }
      return res.data
    },
    enabled: !!id && enabled,
    staleTime: 30_000,
    /**
     * 写回会重启节点 gateway（生产实测 25.5s），重启收尾的那几秒读路径也还没恢复；
     * daemon 自己给读 8s 的预算（capability_handlers.go 的 capabilityQueryTimeout），
     * 而这里的 httpClient 只有 3s（CLAWSYNAPSE_TIMEOUT 默认值）⇒ 单次几乎必然失败。
     * 指数退避重试约 30s（1+2+4+8+8），让界面自己收敛，用户不必手动刷新。
     */
    retry: 5,
    retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 8_000),
  })
}

export function useDeleteAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => agentsApi.deleteAgent(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  })
}

export function useSetAgentCapabilities(id: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: SetCapabilityRequest) => agentsApi.setAgentCapabilities(id!, input),
    /**
     * 写回结束后**立刻**重拉一次能力。
     *
     * 这一步大概率会失败（gateway 刚重启完，读路径还要几秒才恢复），但这正是我们要的：
     * 它把「节点正在同步」这个中间态**显式暴露**给界面 —— 配合 useAgentCapabilities 的
     * 指数退避重试，用户看到的是「同步中…然后自动恢复」，而不是一个假死的列表。
     *
     * 🔴 用 onSettled 而不是 onSuccess：写回失败也照样重拉。
     * 契约里 daemon 失败时返回 **200 + ok=false**（handler/agent.go 的 SetCapabilities），
     * 只看 HTTP 状态会把「节点侧其实已经改完」判成失败 —— 上一版就是这么错的：
     * 5s 延迟的 onError 从来没执行过，因为请求压根没抛错。
     */
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ['agents', id, 'capabilities'] })
    },
  })
}

export function useUploadSkillFile(id: string | undefined) {
  return useMutation({
    mutationFn: (file: File) => agentsApi.uploadSkillFile(id!, file),
  })
}

export function useCronExecutions(id: string | undefined, jobId?: string, enabled = true) {
  return useQuery({
    queryKey: ['agents', id, 'cron-executions', jobId],
    queryFn: async () => {
      const res = await agentsApi.getCronExecutions(id!, jobId)
      return res.data as CronExecutionsResult
    },
    enabled: !!id && enabled,
    staleTime: 10_000,
  })
}
