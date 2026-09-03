import { Badge } from 'antd'
import { CrownOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import type { Meeting, Agent as AgentType } from '@/types'
import { apiClient } from '@/api/client'
import type { ApiListResponse } from '@/types'

interface Props {
  meeting: Meeting
}

export function MeetingParticipantPanel({ meeting }: Props) {
  const { data: agents } = useQuery({
    queryKey: ['agents'],
    queryFn: async () => {
      const res = await apiClient.get('/api/v1/agents').json<ApiListResponse<AgentType>>()
      return res.data.items
    },
  })

  const pmAgent = agents?.find((a) => a.id === meeting.host_agent_id)
  const participantAgents = meeting.participants
    .map((p) => agents?.find((a) => a.id === p.agent_id))
    .filter(Boolean)

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div style={{ padding: '10px 12px', borderBottom: '1px solid var(--line)', fontSize: 12, fontWeight: 500, color: 'var(--text-secondary)' }}>
        参会者 ({1 + participantAgents.length})
      </div>
      <div style={{ flex: 1, overflowY: 'auto', padding: 8, display: 'flex', flexDirection: 'column', gap: 4 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '8px 10px', borderRadius: 'var(--radius-control)', background: 'var(--surface)' }}>
          <Badge status="success" />
          <span style={{ fontSize: 13, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{pmAgent?.name ?? 'PM 数字员工'}</span>
          <CrownOutlined style={{ color: 'var(--warning)', fontSize: 12 }} />
        </div>

        {participantAgents.map((agent) =>
          agent ? (
            <div key={agent.id} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '8px 10px', borderRadius: 'var(--radius-control)' }}>
              <Badge status={agent.status === 'online' ? 'success' : 'default'} />
              <span style={{ fontSize: 13, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{agent.name}</span>
              <span style={{ fontSize: 10, color: 'var(--text-tertiary)' }}>{agent.role}</span>
            </div>
          ) : null,
        )}

        {participantAgents.length === 0 && (
          <p style={{ fontSize: 12, color: 'var(--text-tertiary)', padding: 8 }}>暂无其他参会数字员工</p>
        )}
      </div>
    </div>
  )
}