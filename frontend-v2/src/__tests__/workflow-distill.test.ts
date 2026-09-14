import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { distillTaskWorkflowTemplate } from '@/api/tasks'
import { useDistillTaskWorkflowTemplate } from '@/hooks/useTasks'
import { useAuthStore } from '@/stores/authStore'

/**
 * T1.9「把已完成任务一键沉淀为全局工作流模板」前端契约。
 *
 * (a) API 契约（行为级，真实触发 ky）：POST 到 `tasks/<id>/distill-template`，
 *     路径**不得以 `/` 开头**（`apiClient` 用 ky `prefixUrl`，前导斜杠会直接抛错），
 *     body 恒为对象（缺省时 `{}`），回包解析成 `ApiResponse<WorkflowTemplate>`。
 * (b) hook 级：`useDistillTaskWorkflowTemplate` mutation 成功后必须以
 *     `['workflow-templates']` 命中 `useWorkflowTemplates` 的 queryKey 前缀去失效。
 *
 * 手法对齐 `src/api/client.test.ts`：stub `fetch` 抓取最终 Request，不引入新依赖。
 */

interface Captured {
  url: string
  method: string
  body: string
}

/** 后端 201 回包形状（ApiResponse<WorkflowTemplate>）。 */
const TEMPLATE = {
  id: 't1',
  name: 'n',
  steps: [],
  version: 1,
  created_at: '',
  updated_at: '',
}

let calls: Captured[]

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}

beforeEach(() => {
  calls = []
  vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = input instanceof Request ? input : new Request(input as string, init)
    const body = req.method === 'GET' || req.method === 'HEAD' ? '' : await req.clone().text()
    calls.push({ url: req.url, method: req.method, body })
    return jsonResponse({ data: TEMPLATE })
  })
  // 清空鉴权/租户上下文，避免冷启动 refresh 门闩介入本次请求链路。
  useAuthStore.setState({
    accessToken: null,
    refreshToken: null,
    user: null,
    activeOrgId: null,
    personalOrgId: null,
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('distillTaskWorkflowTemplate API 契约（T1.9）', () => {
  it('POST 到 tasks/<id>/distill-template，路径不带前导斜杠', async () => {
    await distillTaskWorkflowTemplate('t-42')

    expect(calls).toHaveLength(1)
    expect(calls[0].method).toBe('POST')
    // 精确 URL：prefixUrl(http://localhost:8080/api/v1/) + 相对路径(tasks/…) 的拼接结果。
    // 若路径以 `/` 开头，ky 会直接抛错；即便拼出来也会多一个斜杠 —— 下面两行把它钉死。
    expect(calls[0].url).toBe('http://localhost:8080/api/v1/tasks/t-42/distill-template')
    expect(calls[0].url.endsWith('tasks/t-42/distill-template')).toBe(true)
    expect(calls[0].url).not.toContain('api/v1//')
  })

  it('显式传入 name/description 时，请求体就是该对象', async () => {
    await distillTaskWorkflowTemplate('t-42', { name: 'x', description: 'y' })

    expect(JSON.parse(calls[0].body)).toEqual({ name: 'x', description: 'y' })
  })

  it('缺省参数时请求体是 {}（而不是 undefined / 空串）', async () => {
    await distillTaskWorkflowTemplate('t-42')

    expect(calls[0].body).toBe('{}')
  })

  it('把回包解析为 ApiResponse<WorkflowTemplate>', async () => {
    const res = await distillTaskWorkflowTemplate('t-42')

    expect(res.data.name).toBe('n')
    expect(res.data.id).toBe('t1')
  })
})

describe('useDistillTaskWorkflowTemplate 成功后失效模板列表（T1.9）', () => {
  it('mutation 成功后以 ["workflow-templates"] 调用 invalidateQueries', async () => {
    ;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    const invalidate = vi.spyOn(qc, 'invalidateQueries')

    let trigger: (() => Promise<unknown>) | null = null
    function Probe() {
      const mutation = useDistillTaskWorkflowTemplate()
      trigger = () => mutation.mutateAsync({ taskId: 't-42' })
      return null
    }

    const container = document.createElement('div')
    const root = createRoot(container)
    await act(async () => {
      root.render(createElement(QueryClientProvider, { client: qc }, createElement(Probe)))
    })
    await act(async () => {
      await trigger!()
    })

    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['workflow-templates'] })

    await act(async () => {
      root.unmount()
    })
  })
})
