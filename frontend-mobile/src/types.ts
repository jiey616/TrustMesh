// 移动端只声明"自己真正消费的"那部分后端契约，不复制桌面端的全量 types。

export interface ApiResponse<T> {
  data: T
}

export interface ApiListResponse<T> {
  data: { items: T[] }
  meta: { count: number }
}

export class ApiRequestError extends Error {
  code: string
  status: number

  constructor(code: string, message: string, status: number) {
    super(message)
    this.name = 'ApiRequestError'
    this.code = code
    this.status = status
  }
}

export interface User {
  id: string
  name: string
  email: string
}

export interface AuthSuccessData {
  access_token: string
  refresh_token: string
  expires_in: number
  user: User
}

export interface OrgView {
  id: string
  name: string
  /** 展示用简称（≤5 字），后端可能不返回，由前端回落截取 */
  short_name?: string
  kind: 'personal' | 'enterprise'
  my_role: string
}

export interface ProjectView {
  id: string
  name: string
  updated_at?: string
  /** 项目总流程在其 workflows 数组里的下标（建任务切片时用） */
  primary_workflow_index?: number
}

// ─── 项目总流程流水线（workflow-progress） ───
export type WorkflowStepStatus =
  | 'pending'
  | 'in_progress'
  | 'awaiting_review'
  | 'done'
  | 'failed'
  | 'canceled'
  | 'unassigned'

export interface WorkflowStepOutput {
  output_name: string
  file_name?: string
  mime_type?: string
  file_size?: number
}

export interface WorkflowStepProgress {
  index: number
  name: string
  role?: string
  status: WorkflowStepStatus
  task_id?: string
  task_title?: string
  /** 该步骤在流程里声明的输出位名称；空 = 未声明 */
  declared_outputs?: string[]
  outputs?: WorkflowStepOutput[]
}

export interface WorkflowProgress {
  workflow_name: string
  steps: WorkflowStepProgress[]
}

export type TaskStatus =
  | 'planning'
  | 'review'
  | 'pending'
  | 'in_progress'
  | 'awaiting_review'
  | 'waiting_user'
  | 'done'
  | 'failed'
  | 'canceled'

export interface TaskListItem {
  id: string
  project_id: string
  title: string
  description: string
  status: TaskStatus
  todo_count: number
  completed_todo_count: number
  failed_todo_count: number
  created_at: string
  updated_at: string
}

/** 🔴 后端真实字段是 agent_id（桌面端同形状）；name 仅用于展示 */
export interface TodoAssignee {
  agent_id?: string
  name?: string
}

export interface TodoQuestion {
  id: string
  question: string
  options?: string[]
  required: boolean
  answer?: string
  answered_at?: string | null
}

export interface Todo {
  id: string
  task_id: string
  title: string
  description: string
  status: string
  assignee: TodoAssignee
  result: string | null | { summary?: string }
  /** 只有 'pending_approval' 才需要人工拍板 */
  review_status?: '' | 'pending_approval' | 'approved' | 'rejected'
  questions?: TodoQuestion[]
  updated_at: string
}

export interface TaskArtifact {
  transfer_id: string
  task_id: string
  todo_id?: string
  file_name: string
  file_size: number
  mime_type: string
  from_agent_name: string
  /** deliverable=最终交付物（已绑定输出位）；process=过程文件；空=历史数据 */
  kind?: 'deliverable' | 'process'
  output_name?: string
  created_at: string
}

export interface TaskMessage {
  id: string
  role: 'user' | 'pm_agent' | 'agent' | string
  ui_blocks?: UIBlock[]
}

export interface TaskDetail {
  id: string
  project_id: string
  title: string
  description: string
  status: TaskStatus
  pm_agent?: { id: string; name: string }
  todos: Todo[]
  artifacts: TaskArtifact[]
  messages?: TaskMessage[]
  updated_at: string
}

/** @ 提及候选（PM + 各步骤执行员工，去重后） */
export interface MentionCandidate {
  id: string
  name: string
  roleLabel: string
}

export interface TaskEvent {
  id: string
  event_type: string
  actor_type?: 'user' | 'agent' | 'system'
  actor_id?: string
  actor_name?: string
  content?: string
  todo_id?: string
  created_at?: string
  metadata?: Record<string, unknown>
}

export interface Comment {
  id: string
  task_id: string
  actor_type: 'user' | 'agent'
  actor_name: string
  content: string
  created_at: string
}

export interface Notification {
  id: string
  event_id?: string
  project_id?: string
  task_id?: string
  actor_type?: 'user' | 'agent' | 'system'
  actor_id?: string
  actor_name?: string
  title: string
  body: string
  category: string
  is_read: boolean
  read_at: string | null
  created_at: string
}

// ─── PM 规划澄清用的动态表单 ───
export type UIBlockType = 'single_select' | 'text_input' | 'confirm' | 'info'

export interface UIBlockOption {
  value: string
  label: string
  description?: string
}

export interface UIBlock {
  id: string
  type: UIBlockType
  label: string
  options?: UIBlockOption[]
  placeholder?: string
  content?: string
}

export interface UIBlockResponse {
  selected?: string[]
  text?: string
  confirmed?: boolean
}
