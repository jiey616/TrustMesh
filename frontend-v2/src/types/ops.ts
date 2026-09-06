// ========== 运维工单（ops_incident）类型 ==========

export type OpsIncidentStatus =
  | 'open'
  | 'diagnosing'
  | 'guiding'
  | 'resolved'
  | 'escalated'
  | 'ignored'

export type OpsActionKind =
  | 'created'
  | 'diagnosed'
  | 'guided'
  | 'reminded'
  | 'escalated'
  | 'resolved'
  | 'ignored'
  | 'reopened'

export interface OpsAction {
  id: string
  at: string
  level: 'L0' | 'L1'
  kind: OpsActionKind
  template_id?: string
  target?: string
  content?: string
  result: 'sent' | 'throttled' | 'failed' | 'skipped'
  detail?: string
  metadata?: Record<string, unknown>
}

export interface OpsIncident {
  id: string
  org_id?: string
  user_id: string
  rule_id: string
  status: OpsIncidentStatus
  severity: 'warn' | 'critical'
  title: string
  summary?: string
  root_cause?: string
  attr_source?: 'template' | 'llm' | 'none'
  project_id?: string
  task_id?: string
  todo_id?: string
  agent_id?: string
  node_id?: string
  /** 展示辅助字段：后端按 ID 反查填充，不入库 */
  task_title?: string
  project_name?: string
  agent_name?: string
  guide_count: number
  active: boolean
  actions: OpsAction[]
  created_at: string
  updated_at: string
  resolved_at?: string
}
