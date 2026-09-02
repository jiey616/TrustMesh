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
  status: MeetingStatus
  created_at: string
  updated_at: string
}

export interface MeetingParticipant {
  agent_id: string
  agent_name: string
  node_id: string
  status: 'invited' | 'joined' | 'left'
}

export type MeetingStatus = 'waiting' | 'in_progress' | 'completed'

export interface MeetingAttachedFile {
  id: string
  file_name: string
  file_size?: number
  mime_type?: string
  source?: string
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
  ui_blocks?: import('./index').UIBlock[]
  created_at: string
}
