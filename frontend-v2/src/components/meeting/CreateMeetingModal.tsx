import { Modal, Form, Input, App, Button, Space, Checkbox, Divider, Typography } from 'antd'
import { PlusOutlined, CloseOutlined } from '@ant-design/icons'
import { useState } from 'react'
import { useAgents } from '@/hooks/useAgents'
import { useCreateMeeting } from '@/hooks/useMeetings'
import type { MeetingParticipant } from '@/types'

const { Text } = Typography

interface Props {
  open: boolean
  onClose: () => void
  projectId: string
}

interface AgendaEntry {
  description: string
  agents: { agent_id: string; weight: number }[]
}

export function CreateMeetingModal({ open, onClose, projectId }: Props) {
  const [form] = Form.useForm()
  const { data: agents } = useAgents()
  const createMeeting = useCreateMeeting(projectId)
  const { message } = App.useApp()
  const [selectedAgentIds, setSelectedAgentIds] = useState<string[]>([])
  const [agendaEntries, setAgendaEntries] = useState<AgendaEntry[]>([{ description: '', agents: [] }])

  const agentsArr = Array.isArray(agents) ? agents : []
  const pmAgent = agentsArr.find((a) => a?.role === 'pm')
  const execAgents = agentsArr.filter((a) => a?.role !== 'pm' && !a?.archived)

  const toggleAgent = (id: string) => {
    setSelectedAgentIds((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id],
    )
  }

  const toggleAgentInEntry = (entryIdx: number, agentId: string) => {
    setAgendaEntries((prev) => {
      const next = prev.map((e) => ({ ...e, agents: [...e.agents] }))
      const entry = next[entryIdx]
      const existing = entry.agents.find((a) => a.agent_id === agentId)
      if (existing) {
        entry.agents = entry.agents.filter((a) => a.agent_id !== agentId)
      } else {
        entry.agents = [...entry.agents, { agent_id: agentId, weight: 5 }]
      }
      return next
    })
  }

  const updateAgentWeight = (entryIdx: number, agentId: string, weight: number) => {
    setAgendaEntries((prev) => {
      const next = prev.map((e) => ({ ...e, agents: [...e.agents] }))
      next[entryIdx].agents = next[entryIdx].agents.map((a) =>
        a.agent_id === agentId ? { ...a, weight } : a,
      )
      return next
    })
  }

  const updateEntryDesc = (idx: number, description: string) => {
    setAgendaEntries((prev) => prev.map((e, i) => (i === idx ? { ...e, description } : e)))
  }

  const addAgendaEntry = () => setAgendaEntries((prev) => [...prev, { description: '', agents: [] }])
  const removeAgendaEntry = (idx: number) => setAgendaEntries((prev) => prev.filter((_, i) => i !== idx))

  const handleSubmit = async () => {
    const values = await form.validateFields()
    if (!pmAgent) {
      message.error('未找到 PM 数字员工，无法创建会议')
      return
    }

    const participants: MeetingParticipant[] = selectedAgentIds.map((id: string) => {
      const a = execAgents.find((e) => e.id === id)
      return {
        agent_id: id,
        agent_name: a?.name ?? id,
        node_id: a?.node_id ?? '',
        status: 'invited' as const,
      }
    })

    const validEntries = agendaEntries.filter((e) => e.description.trim() && e.agents.length > 0)
    const agendaItems = validEntries.map((e, i) => ({
      order: i + 1,
      description: e.description.trim(),
      assignees: e.agents.map((a) => ({ agent_id: a.agent_id, weight: a.weight })),
      status: 'pending' as const,
    }))

    try {
      await createMeeting.mutateAsync({
        title: values.title.trim(),
        agenda: values.agenda?.trim() ?? '',
        host_agent_id: pmAgent.id,
        participants,
        agenda_items: agendaItems.length > 0 ? agendaItems : undefined,
      })
      message.success('会议已创建')
      form.resetFields()
      setSelectedAgentIds([])
      setAgendaEntries([{ description: '', agents: [] }])
      onClose()
    } catch {
      message.error('创建会议失败')
    }
  }

  return (
    <Modal
      title="创建结构化会议"
      open={open}
      onCancel={onClose}
      onOk={handleSubmit}
      confirmLoading={createMeeting.isPending}
      width={640}
    >
      <Form form={form} layout="vertical">
        <Form.Item name="title" label="会议标题" rules={[{ required: true, message: '请输入会议标题' }]}>
          <Input placeholder="输入会议标题" />
        </Form.Item>
        <Form.Item name="agenda" label="会议描述">
          <Input.TextArea rows={2} placeholder="描述本次会议的总体目标和背景" />
        </Form.Item>
        {pmAgent && (
          <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginBottom: 8 }}>
            主持人: <strong>{pmAgent.name}</strong> (PM Agent)
          </div>
        )}

        <Divider style={{ margin: '12px 0' }}>议程项</Divider>
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          {agendaEntries.map((entry, i) => (
            <div key={i} style={{ padding: 12, borderRadius: 'var(--radius-control)', border: '1px solid var(--line)', background: 'var(--surface-sunken)' }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', gap: 8 }}>
                <Input
                  value={entry.description}
                  onChange={(e) => updateEntryDesc(i, e.target.value)}
                  placeholder="议题描述..."
                  style={{ flex: 1, fontSize: 13 }}
                />
                {agendaEntries.length > 1 && (
                  <Button type="text" size="small" icon={<CloseOutlined />} onClick={() => removeAgendaEntry(i)} />
                )}
              </div>
              <div style={{ marginTop: 8 }}>
                <Text style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>选择参与此议题的数字员工:</Text>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 6 }}>
                  {execAgents.filter((a) => selectedAgentIds.includes(a.id)).map((agent) => {
                    const assigned = entry.agents.find((a) => a.agent_id === agent.id)
                    return (
                      <span
                        key={agent.id}
                        onClick={() => toggleAgentInEntry(i, agent.id)}
                        style={{
                          fontSize: 12,
                          padding: '3px 10px',
                          borderRadius: 'var(--radius-structure)',
                          cursor: 'pointer',
                          border: `1px solid ${assigned ? 'var(--signal)' : 'var(--line-strong)'}`,
                          background: assigned ? 'rgba(109,95,245,0.15)' : 'transparent',
                          color: assigned ? 'var(--signal-hover)' : 'var(--text-secondary)',
                        }}
                      >
                        {agent.name}
                        {assigned && <span style={{ marginLeft: 4, opacity: 0.7 }}>w{assigned.weight}</span>}
                      </span>
                    )
                  })}
                  {execAgents.filter((a) => selectedAgentIds.includes(a.id)).length === 0 && (
                    <span style={{ fontSize: 12, color: 'var(--text-quaternary)' }}>请先在下方邀请数字员工</span>
                  )}
                </div>
                {entry.agents.length > 0 && (
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, marginTop: 8 }}>
                    {entry.agents.map((as) => {
                      const agent = execAgents.find((a) => a.id === as.agent_id)
                      return (
                        <div key={as.agent_id} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12 }}>
                          <span style={{ color: 'var(--text-secondary)', maxWidth: 90, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                            {agent?.name ?? as.agent_id}
                          </span>
                          <select
                            value={as.weight}
                            onChange={(e) => updateAgentWeight(i, as.agent_id, Number(e.target.value))}
                            style={{ height: 24, fontSize: 11, border: '1px solid var(--line-strong)', borderRadius: 'var(--radius-control)', background: 'var(--surface)', color: 'var(--text-primary)' }}
                          >
                            {[1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((w) => (
                              <option key={w} value={w} style={{ color: '#000' }}>w={w}</option>
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
          <Button type="dashed" icon={<PlusOutlined />} onClick={addAgendaEntry} block>
            添加议程
          </Button>
        </Space>

        <Divider style={{ margin: '12px 0' }}>参会数字员工（勾选后可分配到议程项）</Divider>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4, maxHeight: 160, overflowY: 'auto', border: '1px solid var(--line)', borderRadius: 'var(--radius-control)', padding: 8 }}>
          {execAgents.length === 0 && <Text style={{ fontSize: 13, padding: 8, color: 'var(--text-quaternary)' }}>暂无可用数字员工</Text>}
          {execAgents.map((agent) => (
            <Checkbox
              key={agent.id}
              checked={selectedAgentIds.includes(agent.id)}
              onChange={() => toggleAgent(agent.id)}
              style={{ padding: '4px 8px', fontSize: 13 }}
            >
              {agent.name}
              <Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>{agent.role}</Text>
            </Checkbox>
          ))}
        </div>
      </Form>
    </Modal>
  )
}