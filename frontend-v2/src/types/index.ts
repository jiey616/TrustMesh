// ========== API 通用类型 ==========

export interface ApiError {
  code: string
  message: string
  details: Record<string, unknown>
}

export interface ApiResponse<T> {
  data: T
}

export interface ApiListMeta {
  count: number
}

export interface ApiListResponse<T> {
  data: { items: T[] }
  meta: ApiListMeta
}

export class ApiRequestError extends Error {
  code: string
  status: number
  details: Record<string, unknown>

  constructor(code: string, message: string, status: number, details: Record<string, unknown> = {}) {
    super(message)
    this.code = code
    this.status = status
    this.details = details
    this.name = 'ApiRequestError'
  }
}

// ========== 外部应用（SSO 连接平台）==========

export type ExternalAppStatus = 'enabled' | 'disabled'
export type ExternalAppSSOType = 'trustmesh_jwt'
export type ExternalAppFrameMode = 'newtab' | 'iframe'
export type ExternalAppPlacement = 'sidebar' | 'project_tab'
// 三级作用域（2026-09-17）：全局=平台管理员配置、全员可见；组织=本组织成员可见；
// 个人=仅创建者本人可见。作用域创建后不可变更（改层级只能重建）。
export type ExternalAppScope = 'global' | 'org' | 'personal'

// 安全视图：不含 client_secret
export interface ExternalAppView {
  id: string
  /** 归属租户：全局级为空串，组织级为企业租户，个人级为创建者个人租户 */
  org_id?: string
  name: string
  base_url: string
  client_id: string
  sso_type: ExternalAppSSOType
  frame_mode: ExternalAppFrameMode
  scopes: string
  /** 挂载点集合，逗号分隔，如 "sidebar,project_tab"；空串表示不挂载 */
  placement: string
  icon_url: string
  sort_order: number
  scope: ExternalAppScope
  status: ExternalAppStatus
  created_by: string
  created_at: string
  updated_at: string
}

export interface CreateExternalAppRequest {
  name: string
  base_url: string
  client_id: string
  sso_type?: ExternalAppSSOType
  frame_mode?: ExternalAppFrameMode
  scopes?: string
  placement?: string
  icon_url?: string
  sort_order?: number
  /** 业务侧只接受 org|personal；global 仅平台侧接口可创建 */
  scope?: ExternalAppScope
}

export interface UpdateExternalAppRequest {
  name?: string
  base_url?: string
  sso_type?: ExternalAppSSOType
  frame_mode?: ExternalAppFrameMode
  scopes?: string
  status?: ExternalAppStatus
  placement?: string
  icon_url?: string
  sort_order?: number
}

/** placement 字符串是否包含某个挂载点 */
export function hasPlacement(placement: string | undefined, point: ExternalAppPlacement): boolean {
  if (!placement) return false
  return placement.split(',').some((p) => p.trim() === point)
}

export interface LaunchExternalAppResponse {
  app_id: string
  app_name: string
  launch_url: string
  expires_in: number
}

// ========== Auth ==========

export interface User {
  id: string
  email: string
  name: string
  /** 平台管理员（首个注册用户自动获得），可管平台级 LLM 配置 */
  is_admin?: boolean
  created_at: string
  updated_at: string
}

export interface AuthSuccessData {
  access_token: string
  refresh_token: string
  expires_in: number
  user: User
}

export interface AuthLoginRequest {
  email: string
  password: string
}

export interface AuthRegisterRequest {
  name: string
  email: string
  password: string
}

// ========== 数字员工 ==========

export type AgentRole = 'pm' | 'developer' | 'reviewer' | 'custom'
export type AgentStatus = 'online' | 'offline' | 'busy'

export interface AgentUsage {
  project_count: number
  task_count: number
  todo_count: number
  total_count: number
  in_use: number
}

export interface Agent {
  id: string
  name: string
  description: string
  role: AgentRole
  capabilities: string[]
  node_id: string
  product: string
  status: AgentStatus
  archived: boolean
  last_seen_at: string | null
  usage: AgentUsage
  created_at: string
  updated_at: string
}

export interface DailyActivityItem {
  date: string
  completed: number
  failed: number
  created: number
}

export interface WorkloadItem {
  todo_id: string
  todo_title: string
  task_id: string
  task_title: string
  project_id: string
  started_at: string
}

export interface AgentStats {
  role: string

  // executor
  todos_total: number
  todos_done: number
  todos_failed: number
  todos_in_progress: number
  todos_pending: number
  success_rate: number
  avg_response_time_ms: number | null
  avg_completion_time_ms: number | null

  // pm
  projects_managed: number
  tasks_created: number
  tasks_done: number
  tasks_failed: number
  tasks_in_progress: number
  tasks_pending: number
  task_success_rate: number
  planning_replies: number

  daily_activity: DailyActivityItem[]
  current_workload: WorkloadItem[]
}

export interface AgentInsightAgingRow {
  label: string
  count: number
}

export interface AgentInsightPriorityRow {
  priority: TaskPriority
  label: string
  total: number
  done: number
  failed: number
  pending: number
  in_progress: number
  completion_rate: number
}

export interface AgentInsightProjectRow {
  project_id: string
  project_name: string
  total: number
  done: number
  failed: number
  pending: number
  in_progress: number
  completion_rate: number
}

export interface AgentInsightRiskItem {
  id: string
  kind: 'task' | 'todo'
  title: string
  subtitle: string
  project_id: string
  project_name: string
  status: 'pending' | 'in_progress'
  age_ms: number
}

export interface AgentInsights {
  role: string
  total_items: number
  active_items: number
  pending_over_24h: number
  failures_last_7d: number
  completions_last_7d: number
  oldest_pending_ms: number | null
  longest_in_progress_ms: number | null
  response_p50_ms: number | null
  response_p90_ms: number | null
  completion_p50_ms: number | null
  completion_p90_ms: number | null
  summary: string
  strengths: string[]
  weaknesses: string[]
  aging: AgentInsightAgingRow[]
  priority_breakdown: AgentInsightPriorityRow[]
  project_contribution: AgentInsightProjectRow[]
  risk_items: AgentInsightRiskItem[]
}

export interface ChatAttachment {
  id: string
  file_name: string
  file_size: number
  mime_type: string
  /** 短时签名下载链接，只在读取/投递时由后端补全，消息本身不持久化 */
  url?: string
}

export interface AgentChatMessage {
  id: string
  sender_type: 'user' | 'agent'
  direction: 'outbound' | 'inbound'
  content: string
  status: 'pending' | 'sent' | 'failed'
  remote_message_id?: string
  attachments?: ChatAttachment[]
  created_at: string
}

export interface AgentChatDetail {
  id: string
  agent_id: string
  agent_node_id: string
  session_key: string
  status: 'active' | 'closed'
  messages: AgentChatMessage[]
  created_at: string
  updated_at: string
}

export interface AgentChatSessionSummary {
  id: string
  agent_id: string
  session_key: string
  status: 'active' | 'closed'
  message_count: number
  last_message_preview: string
  last_message_at?: string | null
  created_at: string
  updated_at: string
}

export interface AgentTaskItem {
  id: string
  project_id: string
  project_name: string
  title: string
  description: string
  status: TaskStatus
  priority: TaskPriority
  relation: 'pm' | 'executor'
  todo_count: number
  completed_todo_count: number
  failed_todo_count: number
  created_at: string
  updated_at: string
}

export interface CapabilitySkill {
  name: string
  description: string
  category: string
}

export interface CapabilityModel {
  id: string
  provider: string
  model: string
  isDefault: boolean
}

export interface CapabilityJob {
  id: string
  name: string
  schedule: string
  enabled: boolean
  prompt: string
  skills?: string[]
  nextRun?: string
  executions?: CapabilityExecution[]
}

export interface CapabilityExecution {
  executionId: string
  jobId: string
  status: 'running' | 'completed' | 'failed' | 'unknown'
  startedAtMs: number
  finishedAtMs: number
  durationMs: number
  error: string
  outputFile: string
  outputPreview: string
  /** 完整结果 markdown，仅执行历史详情（第 2 层）返回 */
  output?: string
}

export interface CronExecutionsResult {
  executions: CapabilityExecution[]
  error?: string
}

export interface CapabilityInfo {
  product: string
  available: boolean
  skills: CapabilitySkill[]
  models: CapabilityModel[]
  jobs: CapabilityJob[]
  reason?: string
}

export interface CreateAgentRequest {
  name: string
  description: string
  role: AgentRole
  capabilities: string[]
  node_id?: string
}

export interface UpdateAgentRequest {
  name?: string
  description?: string
  role?: AgentRole
  capabilities?: string[]
  node_id?: string
}

export interface ProviderConfig {
  name?: string
  /** OpenAI 兼容 API 地址（如 https://api.deepseek.com/v1），透传节点 custom_providers */
  base_url?: string
  api_mode?: string
  transport?: string
  model?: string
  default_model?: string
  api_key?: string
}

export interface SetCapabilityRequest {
  target: 'skill' | 'model' | 'cron'
  action: string
  skill?: string
  fileIds?: string[]
  model?: string
  provider?: ProviderConfig
  job?: Record<string, unknown>
  jobId?: string
}

export interface SetCapabilityResult {
  ok: boolean
  target: string
  action: string
  skill?: string
  model?: string
  jobId?: string
  restartStatus: 'none' | 'restarted' | 'restart_failed'
  error?: string
}

export type WritebackStatus = 'idle' | 'start' | 'done' | 'failed'

// ========== Project ==========

export type ProjectStatus = 'active' | 'archived'
export type ProjectWorkStatus = 'empty' | 'idle' | 'queued' | 'running' | 'attention' | 'archived'

export interface PMAgentSummary {
  id: string
  name: string
  node_id: string
  status: AgentStatus
}

export interface ProjectTaskSummary {
  task_total: number
  pending_count: number
  in_progress_count: number
  done_count: number
  failed_count: number
  canceled_count: number
  work_status: ProjectWorkStatus
  latest_task_at: string | null
}

export interface Project {
  id: string
  name: string
  description: string
  status: ProjectStatus
  task_summary: ProjectTaskSummary
  pm_agent: PMAgentSummary
  workflows?: Workflow[]
  primary_workflow_index: number
  primary_workflow_id?: string
  created_at: string
  updated_at: string
}

export interface CreateProjectRequest {
  name: string
  description: string
  pm_agent_id?: string
  template_id?: string // 可选：创建项目时直接继承一个全局工作流模板
}

export type EventType =
  | 'task_created'
  | 'task_plan_ready'
  | 'task_plan_rejected'
  | 'task_approved'
  | 'task_status_changed'
  | 'todo_assigned'
  | 'todo_started'
  | 'todo_progress'
  | 'todo_completed'
  | 'todo_failed'
  | 'todo_ask_received'
  | 'todo_answer_received'
  | 'todo_awaiting_review'
  | 'todo_review_approved'
  | 'todo_review_rejected'
  | 'todo_rework_requested'
  | 'todo_rework_exhausted'
  | 'todo_timeout'
  | 'todo_timeout_remind'
  | 'todo_timeout_failed'
  | 'todo_updated'
  | 'todo_appended'
  | 'todo_removed'
  | 'task_comment'
  | 'task_comment_mention'
  | 'planning_reply'
  | 'agent_status_changed'
  | 'join_request_received'
  | 'artifact_received'
  | 'external_app_launch'

export interface Event {
  id: string
  project_id: string
  task_id?: string
  todo_id?: string
  actor_type: 'user' | 'agent' | 'system'
  actor_id: string
  actor_name: string
  event_type: EventType
  content: string | null
  metadata: Record<string, unknown>
  created_at: string
}

export interface UpdateProjectRequest {
  name?: string
  description?: string
  pm_agent_id?: string
  workflows?: Workflow[]
  primary_workflow_index?: number
  primary_workflow_id?: string
}

export interface StepOutput {
  name: string
  description?: string
  mime_type?: string
}

export interface StepIOLink {
  step: string // "prev" 或上游步骤名
  output: string // 该步骤声明的输出名
}

export interface StepInput {
  name: string
  description?: string
  mime_type?: string
  source: StepIOLink
}

export interface WorkflowStep {
  name: string
  role?: string
  agent_id?: string
  need_review?: boolean
  inputs?: StepInput[]
  outputs?: StepOutput[]
}

export interface Workflow {
  id?: string
  // 继承自全局模板时的元数据（见 WorkflowTemplate）
  parent_template_id?: string
  template_version?: number
  name: string
  steps: WorkflowStep[]
}

// WorkflowTemplate 是用户级全局工作流模板（跨项目复用）。项目通过"继承"
// 克隆一份副本进项目（Workflow.parent_template_id），模板每次保存 Version 递增。
export interface WorkflowTemplate {
  id: string
  name: string
  description?: string
  steps: WorkflowStep[]
  version: number
  /** 策展标记：作者可把优质模板标为精选，模板库按 curated 优先排序 */
  curated?: boolean
  curated_at?: string
  created_at: string
  updated_at: string
}

// WorkflowSyncChangeKind 描述一次模板同步里单个步骤的处置方式。
export type WorkflowSyncChangeKind =
  | 'add' // 模板新增步骤：将加入项目
  | 'update' // 模板更新且项目未改：将覆盖为模板新版
  | 'keep' // 冲突：项目改过，保留项目版本
  | 'keep_project' // 项目自加步骤，模板没有：保留
  | 'remove_pending' // 模板删除了该步骤：待用户确认

export interface WorkflowSyncChange {
  name: string
  kind: WorkflowSyncChangeKind
  project_modified?: boolean
}

// WorkflowSyncDiff 是模板版本 X → Y 对项目内某个继承工作流的合并预览。
export interface WorkflowSyncDiff {
  template_id: string
  template_name: string
  current_version: number
  project_version: number
  changes: WorkflowSyncChange[]
}

// WorkflowRef records the slice of the project's primary workflow (总流程)
// that a task owns (inclusive StepFrom..StepTo by index into Workflow.Steps).
export interface WorkflowRef {
  workflow_index: number
  workflow_id?: string
  workflow_name: string
  step_from: number
  step_to: number
}

// WorkflowProgress is the project's overall pipeline progress.
export interface WorkflowStepProgress {
  index: number
  name: string
  role?: string
  agent_id?: string
  status: 'pending' | 'in_progress' | 'awaiting_review' | 'done' | 'failed' | 'canceled' | 'unassigned'
  task_id?: string
  task_title?: string
  /** 该步骤在流程里声明的输出位名称；空 = 未声明，绑定时允许自由命名 */
  declared_outputs?: string[]
  outputs?: { output_name: string; file_id?: string; artifact_id?: string; file_name?: string; mime_type?: string; file_size?: number }[]
}

export interface WorkflowProgress {
  workflow_name: string
  steps: WorkflowStepProgress[]
}

export type ProjectFileSource = 'user_upload' | 'agent_artifact' | 'meeting_minutes'

export interface ProjectFile {
  id: string
  project_id: string
  parent_id?: string
  task_id?: string
  agent_id?: string
  agent_name?: string
  file_name: string
  file_size: number
  mime_type: string
  source: ProjectFileSource
  is_folder: boolean
  transfer_id?: string
  /** agent_artifact 文件性质：deliverable=最终交付文件（已绑定工作流输出），process=过程文件 */
  kind?: 'deliverable' | 'process'
  /** deliverable 绑定的工作流步骤输出名 */
  output_name?: string
  created_at: string
  updated_at: string
}

export interface ListProjectFilesQuery {
  source?: ProjectFileSource
  task_id?: string
  agent_id?: string
}

export interface CreateProjectFolderRequest {
  name: string
  parent_id?: string
}

export interface BreadcrumbNode {
  id: string
  name: string
}

export interface VirtualFolder {
  id: string
  name: string
  item_count?: number
}

export interface BrowseFilesResult {
  parent_id: string
  virtual_folders?: VirtualFolder[]
  folders: ProjectFile[]
  files: ProjectFile[]
  breadcrumbs: BreadcrumbNode[]
}

export interface RenameProjectFileRequest {
  name: string
}

export interface BatchDeleteProjectFilesRequest {
  file_ids: string[]
}

export interface BatchDeleteResult {
  deleted: number
}

// ========== Task ==========

export type TaskStatus =
  | 'planning' | 'review' | 'pending' | 'in_progress'
  | 'awaiting_review' | 'waiting_user' | 'done' | 'failed' | 'canceled'

export type TaskPriority = 'low' | 'medium' | 'high' | 'urgent'

export interface TodoAssignee {
  agent_id: string
  name: string
  node_id: string
}

export interface TodoQuestion {
  id: string
  question: string
  options?: string[]
  required: boolean
  asked_at: string
  answer?: string
  answered_by?: string
  answered_at?: string | null
  timed_out?: boolean
}

/** Todo 实际产出、并已绑定到工作流步骤输出位的文件。字段对齐后端 model.TodoOutput */
export interface TodoOutput {
  /** 匹配该步骤 StepOutput.name */
  output_name: string
  artifact_id?: string
  file_ref?: string
}

export interface Todo {
  id: string
  task_id: string
  order?: number
  title: string
  description: string
  status: string
  assignee: TodoAssignee
  started_at: string | null
  completed_at: string | null
  failed_at: string | null
  canceled_at: string | null
  error: string | null
  cancel_reason: string | null
  result: string | null | { summary?: string; final_output?: string; output?: string; metadata?: Record<string, unknown>; [k: string]: unknown }
  retry_count?: number
  max_retries?: number
  review_status?: '' | 'pending_approval' | 'approved' | 'rejected'
  need_review?: boolean
  review_reason?: string | null
  rework_count?: number
  max_reworks?: number
  questions?: TodoQuestion[]
  /** agent 上传并认领了输出位的文件，下游步骤据此取「上一个流程的输出文件」 */
  outputs?: TodoOutput[]
  created_at: string
  updated_at: string
}

export interface TaskArtifact {
  transfer_id: string
  task_id: string
  todo_id?: string
  file_name: string
  file_size: number
  mime_type: string
  from_node_id: string
  from_agent_id: string
  from_agent_name: string
  /** 文件性质：deliverable=最终交付文件（已绑定工作流输出），process=过程文件 */
  kind?: 'deliverable' | 'process'
  /** deliverable 绑定的工作流步骤输出名 */
  output_name?: string
  created_at: string
}

export interface TaskAttachedFile {
  id: string
  file_name: string
  file_size: number
  mime_type: string
  source: string
}

export interface TaskMessage {
  id: string
  task_id: string
  role: 'user' | 'pm_agent' | 'agent' | string
  content: string
  ui_blocks?: UIBlock[]
  ui_response?: UIResponse
  attachments?: ChatAttachment[]
  created_at: string
}

export interface TaskResult {
  summary: string
  final_output: string
  metadata: Record<string, unknown>
}

export interface TaskDetail {
  id: string
  project_id: string
  title: string
  description: string
  status: TaskStatus
  priority: TaskPriority
  source_task_id?: string
  workflow?: Workflow
  workflow_ref?: WorkflowRef
  pm_agent: PMAgentSummary
  messages?: TaskMessage[]
  todos: Todo[]
  artifacts: TaskArtifact[]
  attached_files?: TaskAttachedFile[]
  result: TaskResult
  version: number
  canceled_at: string | null
  canceled_by: { node_id: string; name: string } | null
  cancel_reason: string | null
  created_at: string
  updated_at: string
}

export interface TaskListItem {
  id: string
  project_id: string
  title: string
  description: string
  status: TaskStatus
  priority: TaskPriority
  pm_agent: PMAgentSummary
  todo_count: number
  completed_todo_count: number
  failed_todo_count: number
  created_at: string
  updated_at: string
}

// ========== Dashboard ==========

export interface DashboardStats {
  agents_online: number
  agents_total: number
  tasks_in_progress: number
  tasks_total: number
  tasks_done_count: number
  tasks_failed_count: number
  success_rate: number
  todos_pending: number
}

export interface DashboardEvent {
  id: string
  type: string
  message: string
  node_id: string
  agent_name: string
  created_at: string
}

// ========== Knowledge ==========

export type KnowledgeDocStatus = 'processing' | 'ready' | 'failed'
export type KnowledgeDocType = 'document' | 'note' | 'snippet' | 'reference'

export interface KnowledgeDocument {
  id: string
  project_id: string | null
  title: string
  description: string
  doc_type: KnowledgeDocType
  mime_type: string
  file_size: number
  status: KnowledgeDocStatus
  chunk_count: number
  tags: string[]
  metadata: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface KnowledgeChunk {
  id: string
  document_id: string
  chunk_index: number
  content: string
  token_count: number
  metadata: Record<string, unknown>
  created_at: string
}

export interface KnowledgeSearchResult {
  chunk_id: string
  document_id: string
  document_title: string
  content: string
  score: number
  chunk_index: number
  metadata?: Record<string, unknown>
}

export interface KnowledgeSearchRequest {
  query: string
  project_id?: string
  top_k?: number
  min_score?: number
}

export interface UpdateKnowledgeDocRequest {
  title?: string
  description?: string
  tags?: string[]
}

// ========== Notification ==========

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
  priority?: string
  is_read: boolean
  read_at: string | null
  created_at: string
  node_id?: string
}

// ========== 数字员工入职申请 ==========

export type JoinRequestStatus = 'pending' | 'approved' | 'rejected'

export interface JoinRequest {
  id: string
  trust_request_id: string
  node_id: string
  name: string
  description: string
  role: AgentRole
  capabilities: string[]
  agent_product: string
  status: JoinRequestStatus
  approved_trustmesh_agent_id?: string
  created_at: string
  resolved_at?: string
}

export interface InvitePrompt {
  prompt: string
  node_id: string
}

export interface JoinRequestOverrides {
  name?: string
  role?: AgentRole
  description?: string
  capabilities?: string[]
}

// ========== 岗位市场 ==========

export interface MarketDeptSummary {
  id: string
  name: string
  count: number
}

export interface MarketRoleListItem {
  id: string
  name: string
  description: string
  dept_id: string
  dept_name: string
}

export interface MarketRoleDetail extends MarketRoleListItem {
  identity_content: string
  soul_content: string
  agents_content: string
}

// ========== Meeting ==========

export interface AgendaAssignee {
  agent_id: string
  weight: number
}

export interface MeetingAgendaItem {
  id?: string
  order: number
  description: string
  assignees: AgendaAssignee[]
}

export interface MeetingTodoItem {
  id: string
  description: string
  responsible_agent_id: string
  responsible_agent_name: string
  status: 'pending' | 'converted'
  converted_task_id?: string
}

export interface MeetingParticipant {
  agent_id: string
  agent_name: string
  node_id: string
  status: 'invited' | 'joined' | 'left'
}

export interface MeetingAttachedFile {
  id: string
  file_name: string
  file_size?: number
  mime_type?: string
  source?: string
}

export type MeetingStatus = 'waiting' | 'in_progress' | 'completed'

export interface Meeting {
  id: string
  project_id: string
  title: string
  agenda: string
  creator_id: string
  host_agent_id: string
  participants: MeetingParticipant[]
  agenda_items?: MeetingAgendaItem[]
  todos?: MeetingTodoItem[]
  file_ids?: string[]
  attached_files?: MeetingAttachedFile[]
  summary_file_id?: string
  participant_count?: number
  todo_count?: number
  status: MeetingStatus
  created_at: string
  updated_at: string
}

export interface MeetingMessage {
  id: string
  meeting_id: string
  sender_type: 'user' | 'agent' | 'system'
  sender_id: string
  sender_name: string
  phase?: string
  target?: string
  context_brief?: string
  content: string
  ui_blocks?: UIBlock[]
  created_at: string
}

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
  multiple?: boolean
  placeholder?: string
  required?: boolean
  content?: string
  default?: string[]
  confirm_label?: string
  cancel_label?: string
}

export interface UIBlockResponse {
  selected?: string[]
  text?: string
  confirmed?: boolean
}

export interface UIResponse {
  blocks: Record<string, UIBlockResponse>
}

// ========== Task Comments & Messages ==========

export interface CommentMention {
  agent_id: string
  agent_name?: string
}

export interface Comment {
  id: string
  task_id: string
  todo_id?: string
  actor_type: 'user' | 'agent'
  actor_id: string
  actor_name: string
  content: string
  mentions?: CommentMention[]
  created_at: string
}

export interface AppendTaskMessageRequest {
  content: string
  ui_response?: unknown
  attachments?: ChatAttachment[]
}

export interface AddTaskCommentInput {
  content: string
  todo_id?: string
  mentions?: Array<{ agent_id: string }>
  attachments?: ChatAttachment[]
}

// 办公室可视化相关类型（定义在 ./office.ts，此处转出保持单一入口）
export type {
  RealtimeEvent,
  AgentVisual,
  AgentVisualState,
  AgentSticker,
} from './office'

// ─── 多租户组织（阶段 4-B） ───

export interface OrgQuota {
  max_members: number
  max_nodes: number
  max_projects: number
  max_storage_bytes: number
}

export type OrgKind = 'personal' | 'enterprise'
export type OrgRole = 'owner' | 'admin' | 'member'

export interface OrgView {
  id: string
  name: string
  slug: string
  kind: OrgKind
  owner_id: string
  /** 内置角色为语义键（owner/admin/member）；自定义角色场景下为 role_id */
  my_role: OrgRole | string
  /** 调用者在 org_roles 里的角色引用（权威）；展示角色名请优先用本字段查角色表 */
  my_role_id?: string
  quota: OrgQuota
  /** 企业级菜单隐藏项（owner 在设置页勾选，只能缩小；只能影响菜单可见性） */
  menu_overrides?: string[]
  created_at: string
}

export interface OrgMemberView {
  id: string
  user_id: string
  /** 兼容字段：内置角色为语义键，自定义角色为 role_id（优先用 role_id/role_name） */
  role: string
  /** org_roles 角色引用（权威） */
  role_id?: string
  /** role_id 对应的展示名（后端解析；找不到时为空，前端回落 role） */
  role_name?: string
  joined_at: string
  email?: string
  name?: string
}

export interface CreateOrgRequest {
  name: string
  slug?: string
}

export interface AddOrgMemberRequest {
  email: string
  /** 内置角色串（admin/member）；与 role_id 同传时以 role_id 为准 */
  role?: string
  /** 自定义角色 id（优先） */
  role_id?: string
}

// ─── 企业角色（设计文档 §5，org_roles 集合） ──

export interface OrgRoleView {
  id: string
  org_id: string
  name: string
  permissions: string[]
  /** 内置三角色（owner/admin/member）：权限集锁定不可改、不可删 */
  builtin: boolean
  member_count: number
  created_at: string
  updated_at: string
}

export interface CreateOrgRoleRequest {
  name: string
  permissions: string[]
}

/** PATCH 语义：省略 name = 不改名；省略 permissions = 不改权限集 */
export interface UpdateOrgRoleRequest {
  name?: string
  permissions?: string[]
}

// ─── 权限视图（设计文档 §7，/users/me） ───

/**
 * /users/me 的权限视图：`permissions` 按 `X-Org-Id` **每请求实时解析**（多实例即时生效），
 * `menu_overrides` 为当前企业租户的菜单隐藏项，`is_platform_admin` 决定菜单组切换。
 */
export interface MeResponse {
  user: User
  permissions: string[]
  menu_overrides: string[]
  is_platform_admin: boolean
}

// ─── 平台管理（设计文档 §3.2 / §6.4，/api/v1/platform/*） ───

export type PlatformOrgStatus = 'active' | 'disabled'

/** 平台侧企业视图：元数据 + count 级用量（读不到企业业务内容） */
export interface PlatformOrgView {
  id: string
  name: string
  slug: string
  status: PlatformOrgStatus
  owner_id: string
  owner_email?: string
  owner_name?: string
  quota: OrgQuota
  menu_overrides?: string[]
  created_at: string
  updated_at: string
  member_count: number
  project_count: number
  task_count: number
}

export interface CreatePlatformOrgRequest {
  name: string
  slug?: string
  /** 必须指定现有账号为 Owner（企业租户恒有唯一 Owner） */
  owner_email: string
}

/** 平台全局配置：默认模型 + 新建企业默认配额 + 节点参数 */
export interface PlatformGlobalConfig {
  default_model: string
  quota: OrgQuota
  node_parameters?: Record<string, string>
  /** 平台全局菜单基线：对所有租户（企业 + 个人空间）隐藏的菜单键，只能缩小可见性 */
  hidden_menus?: string[]
  updated_at?: string
  updated_by?: string
}

// ─── 平台侧用户管理（/api/v1/platform/users*、/platform/orgs/:id/members） ───

/** 用户在某租户下的归属与角色（平台视图） */
export interface PlatformUserOrgRef {
  id: string
  name: string
  kind: OrgKind
  role?: string
  role_id?: string
  role_name?: string
}

export interface PlatformUserView {
  id: string
  email: string
  name: string
  /** 被平台管理员禁用：登录 / refresh 一律拒绝，已发 access token 自然过期 */
  disabled: boolean
  disabled_at?: string
  /** 平台管理员账号（env 种子为准，不可在此禁用） */
  is_platform_admin: boolean
  created_at: string
  orgs?: PlatformUserOrgRef[]
}

/** 企业成员视图（平台视角，比企业侧多一个账号禁用态） */
export interface PlatformOrgMemberView {
  user_id: string
  email?: string
  name?: string
  role: string
  role_id?: string
  role_name?: string
  joined_at: string
  disabled: boolean
}

/** 重置密码结果：临时密码只在本次响应里返回一次 */
export interface ResetPasswordResult {
  user_id: string
  email: string
  temp_password: string
}

/** 平台用量总览（count 级聚合，不含业务内容） */
export interface PlatformUsage {
  orgs_total: number
  orgs_enterprise: number
  orgs_disabled: number
  users: number
  agents: number
  projects: number
  tasks: number
  storage_bytes: number
}

export interface AuditLogView {
  id: string
  actor_user_id: string
  actor_email: string
  /** platform 或企业 org_id */
  scope: string
  action: string
  target_type: string
  target_id: string
  detail?: Record<string, unknown>
  ip: string
  created_at: string
}

export interface AuditLogQuery {
  scope?: string
  action?: string
  actor_user_id?: string
  limit?: number
}

// ─── 工作区记忆（"记住上次选中的空间"）───

/** 工作区种类：个人空间 / 企业空间。 */
export type WorkspaceKind = 'personal' | 'enterprise'

/**
 * 工作区记忆——**提示数据，非运行时授权**。
 *
 * 按 `userId` 维度记录「上次选中的空间」，仅在通过 `userId` 守门 + 本账号 `orgs` 校验
 * 之后才会被采用（见 `lib/workspaceMemory.ts#resolveWorkspaceTarget`）。运行时权威态
 * （`activeOrgId` / `personalOrgId`）**绝不**从记忆恢复。
 */
export interface WorkspaceMemory {
  userId: string
  kind: WorkspaceKind
  /** 仅 kind === 'enterprise' 时存在；kind === 'personal' 时 undefined */
  orgId?: string
}
