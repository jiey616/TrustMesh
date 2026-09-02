import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as agentsApi from '@/api/agents'
import type { CreateAgentRequest, SetCapabilityRequest, UpdateAgentRequest } from '@/types'

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
    mutationFn: ({ id, input }: { id: string; input: UpdateAgentRequest }) =>
      agentsApi.updateAgent(id, input),
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

// 节点能力查询（capability 契约）：技能/模型/cron 只读展示
export function useAgentCapabilities(id: string | undefined, enabled = true) {
  return useQuery({
    queryKey: ['agents', id, 'capabilities'],
    queryFn: async () => {
      const res = await agentsApi.getAgentCapabilities(id!)
      return res.data
    },
    enabled: !!id && enabled,
    staleTime: 30_000,
    retry: false,
  })
}

export function useDeleteAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => agentsApi.deleteAgent(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  })
}

// 节点能力写回（capability 契约）：成功后可靠刷新能力查询缓存。
// skill/model 写操作会触发 hermes gateway 重启（3-10s），立即 invalidate 可能
// 命中重启窗口拿到旧数据/失败 → 采用「立即刷新 + 延迟再刷 + 失败重试」策略。
// 关键：仅 available===true 不够——gateway 重启窗口内服务可用但 config 仍是旧值，
// 必须用 verify(info) 校验读回的数据确实反映了本次写回目标，才算刷新成功。
export type WritebackStatus = 'start' | 'done' | 'failed'

export interface CapabilityInfoShape {
  available?: boolean
  skills?: unknown[]
  models?: { provider?: string; isDefault?: boolean }[]
  jobs?: unknown[]
}

export function useSetAgentCapabilities(
  id: string | undefined,
  opts?: {
    onWriteback?: (status: WritebackStatus) => void
    // 校验读回数据是否反映本次写回目标（input 为写回参数）；返回 true 才视为刷新成功
    verify?: (input: SetCapabilityRequest, info: CapabilityInfoShape) => boolean
  },
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: SetCapabilityRequest) => agentsApi.setAgentCapabilities(id!, input),
    onSuccess: (_res, input) => {
      // 1) 立即失效一次（覆盖 cron 等不需要重启的写操作，秒级生效）
      void qc.invalidateQueries({ queryKey: ['agents', id, 'capabilities'] })
      opts?.onWriteback?.('start')

      // 2) 延迟重试刷新：等 gateway 重启完成后再拉最新数据，并校验写回目标已生效。
      const verify = opts?.verify
      let attempt = 0
      const MAX_ATTEMPTS = 12
      const refresh = () => {
        attempt++
        void qc
          .fetchQuery({
            queryKey: ['agents', id, 'capabilities'],
            queryFn: () => agentsApi.getAgentCapabilities(id!).then((res) => res.data),
            staleTime: 0,
            retry: false,
          })
          .then((data) => {
            const info = data as CapabilityInfoShape
            const ok = info?.available === true && (!verify || verify(input, info))
            if (ok) {
              // 拉到了反映写回目标的数据，invalidate 让组件展示最新
              void qc.invalidateQueries({ queryKey: ['agents', id, 'capabilities'] })
              opts?.onWriteback?.('done')
            } else if (attempt < MAX_ATTEMPTS) {
              // gateway 重启中 / 数据未反映写回，2s 后再试
              setTimeout(refresh, 2000)
            } else {
              opts?.onWriteback?.('failed')
            }
          })
          .catch(() => {
            if (attempt < MAX_ATTEMPTS) {
              setTimeout(refresh, 2000)
            } else {
              opts?.onWriteback?.('failed')
            }
          })
      }
      // 初次延迟刷新（gateway 重启需几秒，从 2.5s 开始探测）
      setTimeout(refresh, 2500)
    },
  })
}

// 技能文件上传：返回 fileId 供写回 skill 时引用
export function useUploadSkillFile(id: string | undefined) {
  return useMutation({
    mutationFn: (file: File) => agentsApi.uploadSkillFile(id!, file),
  })
}

// cron 执行历史查询（capability.executions 契约第 2 层）：jobId 可选
export function useCronExecutions(id: string | undefined, jobId?: string, enabled = true) {
  return useQuery({
    queryKey: ['agents', id, 'cron-executions', jobId ?? 'all'],
    queryFn: async () => {
      const res = await agentsApi.getCronExecutions(id!, jobId)
      return res.data
    },
    enabled: !!id && enabled,
    staleTime: 30_000,
    retry: false,
  })
}
