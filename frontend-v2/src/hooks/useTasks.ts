import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as tasksApi from '@/api/tasks'
import type { CreateTaskInput, AddTodoInput, UpdateTodoInput } from '@/api/tasks'
import type { ChatAttachment, Event, TaskDetail, TaskStatus, UIResponse, Workflow } from '@/types'

export function useTasks(projectId: string | undefined, status?: string) {
  return useQuery({
    queryKey: ['tasks', projectId, status],
    queryFn: async () => {
      const res = await tasksApi.listProjectTasks(projectId!, status)
      return res.data.items
    },
    enabled: !!projectId,
    staleTime: 30_000,
    // SSE 实时为主，低频轮询兜底（任务执行过程/SSE 断线时也能更新列表）
    refetchInterval: 30_000,
  })
}

export function useTask(id: string | undefined) {
  return useQuery({
    queryKey: ['tasks', 'detail', id],
    queryFn: async () => {
      const res = await tasksApi.getTask(id!)
      return res.data
    },
    enabled: !!id,
    staleTime: 5_000,
    refetchInterval: (currentQuery) => {
      const task = currentQuery.state.data as { status: string } | undefined
      // 含 awaiting_review / waiting_user：待人工确认/等待用户时对话与状态仍需刷新，
      // 否则头部状态会与执行过程事件流对不齐。
      return !task || ['planning', 'review', 'pending', 'in_progress', 'awaiting_review', 'waiting_user'].includes(task.status) ? 3_000 : false
    },
  })
}

// 任务处于这些状态时视为「活跃」：对话(useTask)与执行过程(useTaskEvents)应同步刷新。
const LIVE_TASK_STATUSES = ['planning', 'review', 'pending', 'in_progress', 'awaiting_review', 'waiting_user']

export function useTaskEvents(id: string | undefined) {
  const qc = useQueryClient()
  const queryKey = ['tasks', 'detail', id, 'events'] as const
  return useQuery({
    queryKey,
    queryFn: async () => {
      // 增量轮询：已有历史时只带上次最后一条的 created_at，后端只回之后的。
      //
      // 🔴 刻意**不截断**历史（不做「只回最近 N 条」那种分页）：`collectPendingItems`
      // 靠遍历完整事件流找未答复的 `todo_ask_received`，一旦截断就会漏判
      // 「数字员工在等你回答」—— 那是用户最不能漏的一类待办。所以缓存里始终保存
      // 完整历史，增量只省掉重传（生产实测单任务 336 条 / 542 KB）。
      const prev = qc.getQueryData<Event[]>(queryKey)
      const since = prev?.length ? prev[prev.length - 1]?.created_at : undefined
      const res = await tasksApi.listTaskEvents(id!, since)
      const incoming = res.data.items
      if (!since || !prev?.length) return incoming
      return mergeEventsById(prev, incoming)
    },
    enabled: !!id,
    staleTime: 3_000,
    refetchInterval: () => {
      // 与 useTask 保持一致：只要任务活跃就轮询执行过程，避免「对话刷新了、执行过程没刷新」的错位。
      const task = qc.getQueryData(['tasks', 'detail', id]) as { status: string } | undefined
      return task && LIVE_TASK_STATUSES.includes(task.status) ? 4_000 : false
    },
  })
}

export function useTaskComments(id: string | undefined) {
  return useQuery({
    queryKey: ['tasks', 'detail', id, 'comments'],
    queryFn: async () => {
      const res = await tasksApi.listTaskComments(id!)
      return res.data.items
    },
    enabled: !!id,
    staleTime: 10_000,
  })
}

export function useAppendTaskMessage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, content, uiResponse, attachments }: { taskId: string; content: string; uiResponse?: UIResponse; attachments?: ChatAttachment[] }) =>
      tasksApi.appendTaskMessage(taskId, { content, ui_response: uiResponse, attachments }),
    onSuccess: (_res, { taskId }) => {
      invalidateTask(qc, taskId)
    },
  })
}

export function useAddTaskComment() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, content, mentions, attachments }: { taskId: string; content: string; mentions?: Array<{ agent_id: string }>; attachments?: ChatAttachment[] }) =>
      tasksApi.addTaskComment(taskId, { content, mentions, attachments }),
    onSuccess: (_res, { taskId }) => {
      invalidateTask(qc, taskId)
    },
  })
}

export function useCreateTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, input }: { projectId: string; input: CreateTaskInput }) =>
      tasksApi.createTask(projectId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['projects'] })
      qc.invalidateQueries({ queryKey: ['workflow-progress'] })
    },
  })
}

export function useCreateTaskFromText() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { projectId: string; content: string; agent_id?: string; file_ids?: string[]; workflow?: Workflow; workflow_index?: number; step_from?: number; step_to?: number; attachments?: ChatAttachment[] }) =>
      tasksApi.createTaskFromText(input.projectId, {
        content: input.content,
        agent_id: input.agent_id,
        file_ids: input.file_ids,
        workflow: input.workflow,
        workflow_index: input.workflow_index,
        step_from: input.step_from,
        step_to: input.step_to,
        attachments: input.attachments,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['projects'] })
      qc.invalidateQueries({ queryKey: ['workflow-progress'] })
    },
  })
}

export function useCancelTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, reason }: { taskId: string; reason: string }) => tasksApi.cancelTask(taskId, reason),
    onSuccess: (_res, { taskId }) => {
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['tasks', 'detail', taskId] })
      qc.invalidateQueries({ queryKey: ['projects'] })
    },
  })
}

export function useApprovePlan() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId }: { taskId: string }) => tasksApi.approvePlan(taskId),
    // 乐观更新：`plan_review` 待确认项由 `task.status === 'review'` 驱动
    // （见 lib/pendingItems.ts），先就地改掉，方案确认卡立刻从待确认列表消失，
    // 不让用户盯着已点过的按钮等一个公网往返（实测 1.4–2.3 s）。
    onMutate: ({ taskId }) => ({
      snapshot: patchTaskDetail(qc, taskId, (task) => ({ ...task, status: 'in_progress' as TaskStatus })),
    }),
    onError: (_err, { taskId }, ctx) => restoreTaskDetail(qc, taskId, ctx?.snapshot),
    onSuccess: (res, { taskId }) => {
      // 后端返回权威 TaskDetail（handler/task.go:815），直接写缓存，省掉一次往返。
      if (res?.data) qc.setQueryData(['tasks', 'detail', taskId], res.data)
    },
    onSettled: (_res, _err, { taskId }) => invalidateTask(qc, taskId, { skipDetail: true }),
  })
}

export function useRejectPlan() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, feedback }: { taskId: string; feedback: string }) => tasksApi.rejectPlan(taskId, { feedback }),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

export function useReviewTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId, action, reason }: { taskId: string; todoId: string; action: 'approve' | 'reject'; reason?: string }) =>
      tasksApi.reviewTodo(taskId, todoId, action, reason),
    // 乐观更新：`todo_review` 待确认项由 `todo.review_status === 'pending_approval'`
    // 驱动（见 lib/pendingItems.ts），先就地改掉，卡片立刻消失。
    onMutate: ({ taskId, todoId, action }) => ({
      snapshot: patchTaskDetail(qc, taskId, (task) => ({
        ...task,
        todos: (task.todos ?? []).map((t) =>
          t.id === todoId
            ? { ...t, review_status: action === 'approve' ? ('approved' as const) : ('rejected' as const) }
            : t,
        ),
      })),
    }),
    onError: (_err, { taskId }, ctx) => restoreTaskDetail(qc, taskId, ctx?.snapshot),
    onSuccess: (res, { taskId }) => {
      // 后端返回权威 TaskDetail（handler/task.go:379）；注意它已含 approve 后
      // 重新派发的下一个 todo，比前端猜的准。
      if (res?.data) qc.setQueryData(['tasks', 'detail', taskId], res.data)
    },
    onSettled: (_res, _err, { taskId }) => invalidateTask(qc, taskId, { skipDetail: true }),
  })
}

export function useDispatchTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId }: { taskId: string; todoId: string }) => tasksApi.dispatchTodo(taskId, todoId),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

export function useAnswerTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId, questionId, answer }: { taskId: string; todoId: string; questionId: string; answer: string }) =>
      tasksApi.answerTodo(taskId, todoId, { question_id: questionId, answer }),
    // 乐观更新：`todo_ask` 待确认项靠遍历事件流找「未答复的 todo_ask_received」
    // （metadata.answer == null，见 lib/pendingItems.ts），所以把该事件的 answer
    // 就地填上，条目立刻消失。
    onMutate: ({ taskId, questionId, answer }) => ({
      snapshot: patchTaskEvents(qc, taskId, (events) =>
        events.map((ev) =>
          ev.event_type === 'todo_ask_received' && ev.metadata?.question_id === questionId
            ? { ...ev, metadata: { ...ev.metadata, answer } }
            : ev,
        ),
      ),
    }),
    onError: (_err, { taskId }, ctx) => restoreTaskEvents(qc, taskId, ctx?.snapshot),
    // 🔴 刻意不在 onSuccess 里写详情缓存：该端点后端返回的是
    // `{"status":"ok","question_id":...}`（handler/task.go:507），不是 TaskDetail。
    // 之前 api/tasks.ts 把它错误地声明成 ApiResponse<TaskDetail>，照类型写缓存会把
    // {status:'ok'} 当成任务对象塞进去，页面直接炸。类型已修正，这里只失效。
    onSettled: (_res, _err, { taskId }) => invalidateTask(qc, taskId),
  })
}

export function useAddTaskTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, input }: { taskId: string; input: AddTodoInput }) => tasksApi.addTaskTodo(taskId, input),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

export function useUpdateTaskTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId, input }: { taskId: string; todoId: string; input: UpdateTodoInput }) =>
      tasksApi.updateTaskTodo(taskId, todoId, input),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

export function useRemoveTaskTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId }: { taskId: string; todoId: string }) => tasksApi.removeTaskTodo(taskId, todoId),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

// 重开失败/已取消的 todo（回到 in_progress）。重开不自动派发，用户需在任务评论里 @ 执行员工。
export function useReopenTaskTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId, reason }: { taskId: string; todoId: string; reason?: string }) =>
      tasksApi.reopenTodo(taskId, todoId, reason),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

// 失败任务的「重试/继续」：理由必填。与 useReopenTaskTodo 的区别是后端会**同时**
// 把 todo 重开为 in_progress 并发出带 resume 标记的 todo.remind（执行侧据此继续干活，
// 而不是只报进度），所以不需要用户再去评论区 @ 执行员工。
export function useResumeTaskTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId, reason }: { taskId: string; todoId: string; reason: string }) =>
      tasksApi.resumeTodo(taskId, todoId, reason),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

// 把归档的过程文件提升为某工作流步骤的交付物。绑定成功后会触发任务详情刷新，
// 让文件 badge 从「过程」变成「交付 · 输出位名」，并把它接到工作流图的下游步骤上。
export function useBindArtifactOutput() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      taskId,
      todoId,
      input,
    }: {
      taskId: string
      todoId: string
      input: { artifact_id: string; output_name: string }
    }) => tasksApi.bindArtifactOutput(taskId, todoId, input),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

/**
 * 失效「本次操作真正影响到」的查询。
 *
 * 🔴 绝不用 `['tasks']` 这个宽前缀：React Query 的失效是**前缀匹配**，
 * `['tasks']` 会命中缓存里**每一个**任务的 detail / events / comments，以及所有
 * 项目的任务列表。生产实测：单次人工确认后日志里 53 ms 内并发 5 个请求、
 * 350 ms 后又重复同样 5 个 —— 而任务事件流单次就有 542 KB，这就是「待确认弹框
 * 出不来、确认完了框还在」的直接原因之一（请求排队 + 视图反复过期）。
 *
 * 改成 predicate 精确区分两类：
 *   - `['tasks','detail',<taskId>,...]` → 只动**当前这个任务**的详情/事件/评论
 *   - `['tasks',<projectId>,<status>]`  → 任务列表（第二段不是 'detail'，仍要刷）
 *
 * @param opts.skipDetail 调用方已用后端返回的权威 TaskDetail 直接写过缓存时置 true，
 *   避免紧接着再拉一次同样的详情（events / comments / 列表仍会失效）。
 */
function invalidateTask(
  qc: ReturnType<typeof useQueryClient>,
  taskId: string,
  opts?: { skipDetail?: boolean },
) {
  qc.invalidateQueries({
    predicate: (q) => {
      const k = q.queryKey
      if (k[0] !== 'tasks') return false
      if (k[1] === 'detail') {
        if (k[2] !== taskId) return false
        // 长度 3 = 详情本身；长度 4 = 该任务的事件流 / 评论。
        return !(opts?.skipDetail && k.length === 3)
      }
      return true
    },
  })
  qc.invalidateQueries({ queryKey: ['projects'] })
}

/** 就地修改某任务的详情缓存，返回改动前的快照（供 onError 回滚）。无缓存时返回 undefined。 */
function patchTaskDetail(
  qc: ReturnType<typeof useQueryClient>,
  taskId: string,
  patch: (task: TaskDetail) => TaskDetail,
): TaskDetail | undefined {
  const key = ['tasks', 'detail', taskId]
  const prev = qc.getQueryData<TaskDetail>(key)
  if (!prev) return undefined
  qc.setQueryData(key, patch(prev))
  return prev
}

function restoreTaskDetail(
  qc: ReturnType<typeof useQueryClient>,
  taskId: string,
  snapshot: TaskDetail | undefined,
) {
  if (snapshot === undefined) return
  qc.setQueryData(['tasks', 'detail', taskId], snapshot)
}

/** 就地修改某任务的事件流缓存，返回改动前的快照。无缓存时返回 undefined。 */
function patchTaskEvents(
  qc: ReturnType<typeof useQueryClient>,
  taskId: string,
  patch: (events: Event[]) => Event[],
): Event[] | undefined {
  const key = ['tasks', 'detail', taskId, 'events']
  const prev = qc.getQueryData<Event[]>(key)
  if (!prev) return undefined
  qc.setQueryData(key, patch(prev))
  return prev
}

function restoreTaskEvents(
  qc: ReturnType<typeof useQueryClient>,
  taskId: string,
  snapshot: Event[] | undefined,
) {
  if (snapshot === undefined) return
  qc.setQueryData(['tasks', 'detail', taskId, 'events'], snapshot)
}

/**
 * 把增量事件按 id 并入既有事件流（保持 created_at 升序）。
 *
 * 后端游标语义是 `created_at >= since`，所以与游标同毫秒的旧事件会重复回来 ——
 * 靠 id 去重吃掉。这样**既不漏也不重**：换成 `>` 会吞掉边界那条（表现为执行过程
 * 偶发缺一条），不做去重则会渲染出重复条目。
 *
 * 无新增时返回原引用，让 React Query 的 `Object.is` 判断生效、跳过无谓重渲染。
 */
function mergeEventsById(prev: Event[], incoming: Event[]): Event[] {
  if (!incoming.length) return prev
  const seen = new Set(prev.map((e) => e.id))
  const added = incoming.filter((e) => !seen.has(e.id))
  if (!added.length) return prev
  return [...prev, ...added].sort((a, b) => a.created_at.localeCompare(b.created_at))
}

export function useDistillTaskWorkflowTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, name, description }: { taskId: string; name?: string; description?: string }) =>
      tasksApi.distillTaskWorkflowTemplate(taskId, { name, description }),
    // 沉淀会新建一个全局模板 → 让模板列表页立即刷新
    onSuccess: () => qc.invalidateQueries({ queryKey: ['workflow-templates'] }),
  })
}
