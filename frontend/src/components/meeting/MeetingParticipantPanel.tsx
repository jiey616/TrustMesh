import { Crown, Circle } from 'lucide-react'
import type { Meeting } from '@/types/meeting'
import { useAgents } from '@/hooks/useAgents'

interface Props {
  meeting: Meeting
}

export function MeetingParticipantPanel({ meeting }: Props) {
  const { data: agents } = useAgents()

  const pmAgent = agents?.find(a => a.id === meeting.host_agent_id)
  const participantAgents = meeting.participants
    .map(p => agents?.find(a => a.id === p.agent_id))
    .filter(Boolean)

  return (
    <div className="h-full flex flex-col">
      <div className="px-3 py-2 border-b text-xs font-medium text-muted-foreground">
        参会者 ({1 + participantAgents.length})
      </div>
      <div className="flex-1 overflow-y-auto p-2 space-y-1">
        {/* Host */}
        <div className="flex items-center gap-2 px-2 py-1.5 rounded-md bg-muted/30">
          <Circle
            className="size-2.5 shrink-0"
            fill={pmAgent?.status === 'online' ? '#22c55e' : '#6b7280'}
            color={pmAgent?.status === 'online' ? '#22c55e' : '#6b7280'}
          />
          <span className="text-sm flex-1 truncate">{pmAgent?.name ?? 'PM Agent'}</span>
          <Crown className="size-3.5 text-amber-500 shrink-0" aria-label="主持人" />
        </div>

        {/* Participants */}
        {participantAgents.map(agent =>
          agent ? (
            <div key={agent.id} className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-muted/30">
              <Circle
                className="size-2.5 shrink-0"
                fill={agent.status === 'online' ? '#22c55e' : '#6b7280'}
                color={agent.status === 'online' ? '#22c55e' : '#6b7280'}
              />
              <span className="text-sm flex-1 truncate">{agent.name}</span>
              <span className="text-[10px] text-muted-foreground">{agent.role}</span>
            </div>
          ) : null,
        )}

        {participantAgents.length === 0 && (
          <p className="text-xs text-muted-foreground p-2">暂无其他参会智能体</p>
        )}
      </div>
    </div>
  )
}
