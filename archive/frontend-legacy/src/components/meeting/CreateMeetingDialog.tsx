import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { toast } from 'sonner'
import { Plus, X } from 'lucide-react'
import { useAgents } from '@/hooks/useAgents'
import { useCreateMeeting } from '@/hooks/useMeetings'
import { FileSelector } from '@/components/shared/FileSelector'
import type { MeetingParticipant } from '@/types/meeting'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  projectId: string
}

interface AgentAssignment {
  agent_id: string
  weight: number
}

interface AgendaEntry {
  description: string
  agents: AgentAssignment[]
}

export function CreateMeetingDialog({ open, onOpenChange, projectId }: Props) {
  const navigate = useNavigate()
  const { data: agents } = useAgents()
  const createMeeting = useCreateMeeting(projectId)

  const [title, setTitle] = useState('')
  const [agenda, setAgenda] = useState('')
  const [selectedAgentIds, setSelectedAgentIds] = useState<string[]>([])
  const [agendaEntries, setAgendaEntries] = useState<AgendaEntry[]>([
    { description: '', agents: [] },
  ])
  const [selectedFileIds, setSelectedFileIds] = useState<string[]>([])

  const pmAgent = agents?.find(a => a.role === 'pm')
  const execAgents = agents?.filter(a => a.role !== 'pm' && !a.archived) ?? []

  const addAgendaEntry = () => {
    setAgendaEntries(prev => [...prev, { description: '', agents: [] }])
  }

  const removeAgendaEntry = (index: number) => {
    setAgendaEntries(prev => prev.filter((_, i) => i !== index))
  }

  const updateAgendaDesc = (index: number, desc: string) => {
    setAgendaEntries(prev => {
      const next = [...prev]
      next[index] = { ...next[index], description: desc }
      return next
    })
  }

  const toggleAgentInEntry = (entryIdx: number, agentId: string) => {
    setAgendaEntries(prev => {
      const next = [...prev]
      const entry = { ...next[entryIdx] }
      const existing = entry.agents.find(a => a.agent_id === agentId)
      if (existing) {
        entry.agents = entry.agents.filter(a => a.agent_id !== agentId)
      } else {
        entry.agents = [...entry.agents, { agent_id: agentId, weight: 5 }]
      }
      next[entryIdx] = entry
      return next
    })
  }

  const updateAgentWeight = (entryIdx: number, agentId: string, weight: number) => {
    setAgendaEntries(prev => {
      const next = [...prev]
      next[entryIdx] = {
        ...next[entryIdx],
        agents: next[entryIdx].agents.map(a =>
          a.agent_id === agentId ? { ...a, weight } : a,
        ),
      }
      return next
    })
  }

  const toggleFile = (fileId: string) => {
    setSelectedFileIds(prev =>
      prev.includes(fileId) ? prev.filter(id => id !== fileId) : [...prev, fileId],
    )
  }

  const handleSubmit = async () => {
    if (!title.trim()) {
      toast.error('请输入会议标题')
      return
    }
    if (!pmAgent) {
      toast.error('未找到 PM Agent，无法创建会议')
      return
    }

    const participants: MeetingParticipant[] = selectedAgentIds.map(id => {
      const a = execAgents.find(e => e.id === id)
      return {
        agent_id: id,
        agent_name: a?.name ?? id,
        node_id: a?.node_id ?? '',
        status: 'invited' as const,
      }
    })

    const validEntries = agendaEntries.filter(e =>
      e.description.trim() && e.agents.length > 0,
    )
    const agendaItems = validEntries.map((e, i) => ({
      order: i + 1,
      description: e.description.trim(),
      assignees: e.agents.map(a => ({ agent_id: a.agent_id, weight: a.weight })),
      status: 'pending' as const,
    }))

    try {
      const created = await createMeeting.mutateAsync({
        title: title.trim(),
        agenda: agenda.trim(),
        host_agent_id: pmAgent.id,
        participants,
        agenda_items: agendaItems.length > 0 ? agendaItems : undefined,
        file_ids: selectedFileIds.length > 0 ? selectedFileIds : undefined,
      })
      toast.success('会议已创建')
      onOpenChange(false)
      navigate(`/projects/${projectId}/meetings/${created.id}`)
    } catch {
      toast.error('创建会议失败')
    }
  }

  const toggleAgent = (id: string) => {
    setSelectedAgentIds(prev =>
      prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id],
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>创建结构化会议</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          {/* Title */}
          <div>
            <label className="text-sm font-medium mb-1 block">会议标题</label>
            <Input value={title} onChange={e => setTitle(e.target.value)} placeholder="输入会议标题..." />
          </div>

          {/* Description */}
          <div>
            <label className="text-sm font-medium mb-1 block">会议描述</label>
            <Textarea
              value={agenda}
              onChange={e => setAgenda(e.target.value)}
              placeholder="描述本次会议的总体目标和背景..."
              rows={2}
            />
          </div>

          {/* Host */}
          {pmAgent && (
            <div className="text-sm text-muted-foreground">
              主持人: <span className="font-medium">{pmAgent.name}</span> (PM Agent)
            </div>
          )}

          {/* Agenda Items */}
          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="text-sm font-medium">议程项</label>
              <Button variant="ghost" size="sm" className="h-6 text-xs" onClick={addAgendaEntry}>
                <Plus className="size-3 mr-1" /> 添加议程
              </Button>
            </div>
            <div className="space-y-3">
              {agendaEntries.map((entry, i) => (
                <div key={i} className="p-3 border rounded-md bg-muted/20 space-y-2">
                  <div className="flex items-start gap-2">
                    <Input
                      value={entry.description}
                      onChange={e => updateAgendaDesc(i, e.target.value)}
                      placeholder="议题描述..."
                      className="h-8 text-sm flex-1"
                    />
                    {agendaEntries.length > 1 && (
                      <Button variant="ghost" size="icon" className="size-8 shrink-0" onClick={() => removeAgendaEntry(i)}>
                        <X className="size-3" />
                      </Button>
                    )}
                  </div>
                  {/* Agent assignment for this entry */}
                  <div>
                    <p className="text-[11px] text-muted-foreground mb-1">选择参与此议题的智能体:</p>
                    <div className="flex flex-wrap gap-1.5">
                      {execAgents.filter(a => selectedAgentIds.includes(a.id)).map(agent => {
                        const assigned = entry.agents.find(a => a.agent_id === agent.id)
                        return (
                          <button
                            key={agent.id}
                            onClick={() => toggleAgentInEntry(i, agent.id)}
                            className={`text-xs px-2 py-1 rounded-full border transition-colors ${
                              assigned
                                ? 'bg-primary/10 border-primary text-primary'
                                : 'bg-transparent border-border text-muted-foreground hover:border-foreground/30'
                            }`}
                          >
                            {agent.name}
                            {assigned && (
                              <span className="ml-1 opacity-60">w{assigned.weight}</span>
                            )}
                          </button>
                        )
                      })}
                      {execAgents.filter(a => selectedAgentIds.includes(a.id)).length === 0 && (
                        <span className="text-xs text-muted-foreground">请先在下方邀请智能体</span>
                      )}
                    </div>
                    {/* Weight sliders for assigned agents */}
                    {entry.agents.length > 0 && (
                      <div className="mt-2 flex flex-wrap gap-3">
                        {entry.agents.map(as => {
                          const agent = execAgents.find(a => a.id === as.agent_id)
                          return (
                            <div key={as.agent_id} className="flex items-center gap-1.5 text-xs">
                              <span className="text-muted-foreground w-16 truncate">{agent?.name ?? as.agent_id}</span>
                              <select
                                value={as.weight}
                                onChange={e => updateAgentWeight(i, as.agent_id, parseInt(e.target.value))}
                                className="h-6 text-[10px] border rounded px-1"
                              >
                                {[1,2,3,4,5,6,7,8,9,10].map(w => (
                                  <option key={w} value={w}>w={w}</option>
                                ))}
                              </select>
                            </div>
                          )
                        })}
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Participant selection */}
          <div>
            <label className="text-sm font-medium mb-1 block">参会智能体（勾选后可分配到议程项）</label>
            <div className="space-y-1 max-h-28 overflow-y-auto border rounded-md p-2">
              {execAgents.length === 0 && (
                <p className="text-sm text-muted-foreground p-2">暂无可用智能体</p>
              )}
              {execAgents.map(agent => (
                <label
                  key={agent.id}
                  className="flex items-center gap-2 px-2 py-1 rounded hover:bg-muted cursor-pointer text-sm"
                >
                  <input
                    type="checkbox"
                    checked={selectedAgentIds.includes(agent.id)}
                    onChange={() => toggleAgent(agent.id)}
                    className="rounded"
                  />
                  <span>{agent.name}</span>
                  <span className="text-xs text-muted-foreground ml-auto">{agent.role}</span>
                </label>
              ))}
            </div>
          </div>

          {/* Reference files */}
          <FileSelector
            projectId={projectId}
            selectedIds={selectedFileIds}
            onToggle={toggleFile}
            label="参考文件"
          />

          {/* Submit */}
          <div className="flex justify-end gap-2 pt-2 border-t">
            <Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button>
            <Button onClick={handleSubmit} disabled={createMeeting.isPending}>
              {createMeeting.isPending ? '创建中...' : '创建会议'}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
