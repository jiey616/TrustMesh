// ─── API 通用类型 ───

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
  data: {
    items: T[]
  }
  meta: ApiListMeta
}

// ─── 外部应用（SSO 连接平台）───

export type ExternalAppStatus = 'enabled' | 'disabled'
export type ExternalAppSSOType = 'trustmesh_jwt'
export type ExternalAppFrameMode = 'newtab' | 'iframe'

// 安全视图：不含 client_secret
export interface ExternalAppView {
  id: string
  name: string
  base_url: string
  client_id: string
  sso_type: ExternalAppSSOType
  frame_mode: ExternalAppFrameMode
  scopes: string
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
}

export interface UpdateExternalAppRequest {
  name?: string
  base_url?: string
  sso_type?: ExternalAppSSOType
  frame_mode?: ExternalAppFrameMode
  scopes?: string
  status?: ExternalAppStatus
}

export interface LaunchExternalAppResponse {
  app_id: string
  app_name: string
  launch_url: string
  expires_in: number
}

// ─── 领域模型 ───

export interface User {
  id: string
  email: string
  name: string
  created_at: string
  updated_at: string
}

export interface PMAgentSummary {
  id: string
  name: string
  node_id: string
  status: 'online' | 'offline' | 'busy'
}

export type ProjectStatus = 'active' | 'archived'
export type ProjectWorkStatus = 'empty' | 'idle' | 'queued' | 'running' | 'attention' | 'archived'

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
  created_at: string
  updated_at: string
}

// ─── UI Block 类型（交互式问题澄清） ───

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

export interface ChatAttachment {
  id: string
  file_name: string
  file_size: number
  mime_type: string
  url?: string
}

export interface TaskMessage {
  id: string
  role: 'user' | 'pm_agent' | 'agent'
  content: string
  ui_blocks?: UIBlock[]
  ui_response?: UIResponse
  attachments?: ChatAttachment[]
  created_at: string
}

export interface TaskSummary {
  id: string
  title: string
  status: TaskStatus
  priority: TaskPriority
  todo_count: number
  completed_todo_count: number
  created_at: string
  updated_at: string
}

export interface ConversationMessage {
  id: string
  role: 'user' | 'pm_agent'
  content: string
  ui_blocks?: UIBlock[]
  ui_response?: UIResponse
  created_at: string
}

export interface ConversationListItem {
  id: string
  project_id: string
  status: 'active' | 'resolved'
  last_message: ConversationMessage
  linked_task: TaskSummary | null
  created_at: string
  updated_at: string
}

export interface ConversationDetail {
  id: string
  project_id: string
  status: 'active' | 'resolved'
  messages: ConversationMessage[]
  linked_task: TaskSummary | null
  created_at: string
  updated_at: string
}

export interface ConversationStreamSnapshot {
  conversation: ConversationDetail
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
export type AgentRole = 'pm' | 'developer' | 'reviewer' | 'custom'
export type AgentStatus = 'online' | 'offline' | 'busy'
export type AgentProduct = 'trustmesh' | 'hermes' | 'opc' | 'openclaw' | string

export interface AgentUsage {
  project_count: number
  task_count: number
  todo_count: number
  total_count: number
  in_use: boolean
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
  name: string
  steps: WorkflowStep[]
}

export interface Agent {
  id: string
  name: string
  description: string
  role: AgentRole
  capabilities: string[]
  node_id: string
  product: AgentProduct
  status: AgentStatus
  archived: boolean
  last_seen_at: string | null
  usage: AgentUsage
  created_at: string
  updated_at: string
}

export interface TodoAssignee {
  agent_id: string
  name: string
  node_id: string
}

export interface TodoResult {
  summary: string
  output: string
  metadata: Record<string, unknown>
  action_items?: ActionItem[]
}

export interface ActionItem {
  title: string
  description?: string
  assignee_node_id?: string
  assignee_role?: string
  status: 'pending' | 'awaiting_confirmation' | 'converted'
  converted_task_id?: string
  confirmed_by?: string
  created_at: string
}

export interface ActionItemRefDTO {
  task_id: string
  todo_id: string
  task_title: string
  todo_title: string
  item_index: number
  title: string
  description?: string
  assignee_node_id?: string
  assignee_role?: string
  status: string
  converted_task_id?: string
  created_at: string
}

export type TaskStatus = 'planning' | 'review' | 'pending' | 'in_progress' | 'awaiting_review' | 'waiting_user' | 'done' | 'failed' | 'canceled'
export type TaskPriority = 'low' | 'medium' | 'high' | 'urgent'

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

export interface ActorRef {
  actor_type: string
  actor_id: string
  actor_name: string
}

export interface Todo {
  id: string
  title: string
  description: string
  status: TaskStatus
  assignee: TodoAssignee
  started_at: string | null
  completed_at: string | null
  failed_at: string | null
  canceled_at: string | null
  error: string | null
  cancel_reason: string | null
  result: TodoResult
  created_at: string
  review_status?: '' | 'pending_approval' | 'approved' | 'rejected'
  review_reason?: string | null
  rework_count?: number
  max_reworks?: number
  questions?: TodoQuestion[]
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
  created_at: string
}

export interface TaskResult {
  summary: string
  final_output: string
  metadata: Record<string, unknown>
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

export interface TaskAttachedFile {
  id: string
  file_name: string
  file_size: number
  mime_type: string
  source: ProjectFileSource
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
  pm_agent: PMAgentSummary
  messages?: TaskMessage[]
  todos: Todo[]
  artifacts: TaskArtifact[]
  attached_files?: TaskAttachedFile[]
  result: TaskResult
  version: number
  canceled_at: string | null
  canceled_by: ActorRef | null
  cancel_reason: string | null
  created_at: string
  updated_at: string
}

export type EventType =
  | 'task_created'
  | 'task_plan_ready'
  | 'task_status_changed'
  | 'todo_assigned'
  | 'todo_started'
  | 'todo_progress'
  | 'todo_completed'
  | 'todo_failed'
  | 'task_comment'
  | 'planning_reply'
  | 'agent_status_changed'
  | 'artifact_received'
  | 'todo_ask_received'
  | 'todo_answer_received'

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

export interface CommentMention {
  agent_id: string
  agent_name: string
  node_id: string
  role: string
}

export interface CommentMentionDelivery {
  agent_id: string
  agent_name: string
  status: 'sent' | 'failed'
  error?: string
}

export interface AddTaskCommentResult {
  comment: Comment
  mention_deliveries: CommentMentionDelivery[]
}

export interface TaskStreamSnapshot {
  task: TaskDetail
  events: Event[]
}

export interface Notification {
  id: string
  event_id: string
  project_id: string
  task_id?: string
  actor_type?: string
  actor_name?: string
  title: string
  body: string
  category: 'task' | 'todo' | 'agent'
  priority: 'low' | 'medium' | 'high'
  is_read: boolean
  read_at: string | null
  created_at: string
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
  aging: AgentInsightAgingRow[]
  priority_breakdown: AgentInsightPriorityRow[]
  project_contribution: AgentInsightProjectRow[]
  risk_items: AgentInsightRiskItem[]
}

export interface AgentTaskItem {
  id: string
  project_id: string
  project_name: string
  title: string
  description: string
  status: TaskStatus
  priority: TaskPriority
  pm_agent: PMAgentSummary
  relation: 'pm' | 'executor'
  todo_count: number
  completed_todo_count: number
  failed_todo_count: number
  created_at: string
  updated_at: string
}

// --- 节点能力查询（capability 契约，阶段 1 只读） ---

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

// cron 定时任务执行记录（capability.executions 契约，第 1 层随 capabilities 读回、第 2 层详情查询）
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

// --- 节点能力写回（capability 契约，阶段 2） ---

export interface ProviderConfig {
  name?: string
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

export interface ClawSynapseHealth {
  online: boolean
  node_id?: string
  did?: string
  trust_mode?: string
  error?: string
}

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

// ─── 请求类型 ───

export interface AuthRegisterRequest {
  email: string
  name: string
  password: string
}

export interface AuthLoginRequest {
  email: string
  password: string
}

export interface AuthSuccessData {
  access_token: string
  refresh_token: string
  expires_in: number
  user: User
}

export interface RefreshSuccessData {
  access_token: string
  refresh_token: string
  expires_in: number
}

export interface CreateProjectRequest {
  name: string
  description: string
  pm_agent_id: string
}

export interface UpdateProjectRequest {
  name?: string
  description?: string
  pm_agent_id?: string
  workflows?: Workflow[]
}

export interface CreatePlanningTaskRequest {
  content: string
}

export interface AppendTaskMessageRequest {
  content: string
  ui_response?: UIResponse
  attachments?: ChatAttachment[]
}

export interface RejectPlanRequest {
  feedback: string
}

export interface SendAgentChatMessageRequest {
  content: string
  attachments?: ChatAttachment[]
}

export interface ListProjectTasksQuery {
  status?: TaskStatus
}

export interface CreateAgentRequest {
  node_id: string
  name: string
  role: AgentRole
  description: string
  capabilities: string[]
}

export interface UpdateAgentRequest {
  name?: string
  role?: AgentRole
  description?: string
  capabilities?: string[]
}

// ─── 入职申请 ───

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

// ─── 知识库 ───

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

// ─── Assistant ───

export interface AssistantMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  toolCalls?: AssistantToolCall[]
  results?: AssistantResult[]
  navigateAction?: { path: string; label: string }
  timestamp: number
  isStreaming?: boolean
}

export interface AssistantToolCall {
  tool: string
  args: Record<string, unknown>
  status: 'running' | 'done'
}

export type AssistantResult =
  | { type: 'knowledge'; items: KnowledgeSearchResult[] }
  | { type: 'tasks'; items: TaskListItem[] }
  | { type: 'task_detail'; task: TaskDetail }
  | { type: 'stats'; stats: DashboardStats }

export interface AssistantChatRequest {
  message: string
  context?: {
    current_page: string
    project_id?: string
  }
  history?: { role: string; content: string }[]
}

export interface AssistantSSEEvent {
  event: string
  data: Record<string, unknown>
}

// ─── 项目文件管理 ───

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
  uploaded_by: string
  created_at: string
}

export interface ProjectFileTreeNode {
  id: string
  name: string
  type: 'directory' | 'file'
  files?: ProjectFile[]
  children?: ProjectFileTreeNode[]
}

export interface ProjectFileTree {
  uploads: ProjectFileTreeNode[]
  tasks: ProjectFileTreeNode[]
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

export interface RenameProjectFileRequest {
  name: string
}

export interface MoveProjectFileRequest {
  parent_id?: string
}

export interface BatchDeleteProjectFilesRequest {
  ids: string[]
}

export interface BatchDeleteResult {
  deleted: number
  failed: string[]
}

// ─── 文件浏览（v2） ───

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

export interface ArtifactAgentGroup {
  agent_id: string
  agent_name: string
  files: ProjectFile[]
}

export interface ArtifactTaskGroup {
  task_id: string
  task_name: string
  agents: ArtifactAgentGroup[]
}

// ─── 工作岗位市场 ───

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
  my_role: OrgRole
  quota: OrgQuota
  created_at: string
}

export interface OrgMemberView {
  id: string
  user_id: string
  role: OrgRole
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
  role?: 'admin' | 'member'
}
