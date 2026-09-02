import { useState } from 'react'
import { Typography, Tag, Button, Space, Input, Select, Empty, Skeleton, App, Avatar } from 'antd'
import { CopyOutlined, CheckOutlined, CloseOutlined, RobotOutlined, ToolOutlined, CheckSquareOutlined, EditOutlined } from '@ant-design/icons'
import { useInvitePrompt, useJoinRequests, useApproveJoinRequest, useRejectJoinRequest } from '@/hooks/useJoinRequests'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { ApiRequestError } from '@/types'
import type { AgentRole, JoinRequest } from '@/types'

const { Title, Text, Paragraph } = Typography

type SkillAdapter = 'openclaw' | 'opencode'

function buildSkillPrompt(adapter: SkillAdapter, skillName: string, url: string) {
  if (adapter === 'opencode') {
    return `请创建或更新 Skill「${skillName}」。

从以下地址获取完整内容：
${url}

请按 OpenCode 的全局 Skill 方式安装：
1. 创建目录 ~/.config/opencode/skills/${skillName}/
2. 将技能内容保存为 ~/.config/opencode/skills/${skillName}/SKILL.md

兼容路径：
- ~/.agents/skills/${skillName}/SKILL.md
- ~/.claude/skills/${skillName}/SKILL.md`
  }
  return `请创建或更新 Skill「${skillName}」。

从以下地址获取完整内容并安装为本地 Skill：
${url}`
}

const SKILL_DEFINITIONS = [
  {
    key: 'tm-task-plan',
    title: 'PM 任务规划 Skill',
    description: '适用于 PM 数字员工，提供需求澄清、任务规划、任务创建的完整工作流。',
    url: 'https://github.com/yuanjun5681/TrustMesh/blob/main/skills/tm-task-plan/SKILL.md',
  },
  {
    key: 'tm-task-exec',
    title: '执行数字员工任务执行 Skill',
    description: '适用于执行数字员工，提供 Todo 接收、进度回报、结果交付的完整工作流。',
    url: 'https://github.com/yuanjun5681/TrustMesh/blob/main/skills/tm-task-exec/SKILL.md',
  },
] as const

const ROLE_LABELS: Record<string, string> = {
  pm: 'PM',
  developer: '开发者',
  reviewer: '审核者',
  custom: '自定义',
}

const INVITE_PROMPT_KEY = 'invite-prompt'

export function AgentInvitePage() {
  const { data: invite, isLoading: promptLoading } = useInvitePrompt(true)
  const { data: requests } = useJoinRequests('pending')
  const approveRequest = useApproveJoinRequest()
  const rejectRequest = useRejectJoinRequest()
  const { copiedKey, copy } = useCopyToClipboard(2000)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [editRole, setEditRole] = useState<AgentRole>('developer')
  const [editDescription, setEditDescription] = useState('')
  const [skillAdapter, setSkillAdapter] = useState<SkillAdapter>('openclaw')
  const { message } = App.useApp()

  const skillInstructions = SKILL_DEFINITIONS.map((skill) => ({
    ...skill,
    prompt: buildSkillPrompt(skillAdapter, skill.key, skill.url),
  }))

  const handleCopy = async () => {
    if (!invite?.prompt) return
    const ok = await copy(invite.prompt, INVITE_PROMPT_KEY)
    if (ok) message.success('提示词已复制到剪贴板')
    else message.error('复制失败，请手动选择复制')
  }

  const handleSkillCopy = async (key: string, prompt: string) => {
    const ok = await copy(prompt, key)
    if (ok) message.success('指令已复制到剪贴板')
    else message.error('复制失败，请手动选择复制')
  }

  const startEdit = (jr: JoinRequest) => {
    setEditingId(jr.id)
    setEditName(jr.name)
    setEditRole(jr.role)
    setEditDescription(jr.description)
  }

  const handleApprove = async (jr: JoinRequest) => {
    try {
      const overrides = editingId === jr.id
        ? { name: editName, role: editRole, description: editDescription }
        : undefined
      await approveRequest.mutateAsync({ id: jr.id, overrides })
      message.success(`已录用数字员工「${overrides?.name || jr.name}」`)
      setEditingId(null)
    } catch (err) {
      message.error(err instanceof ApiRequestError ? err.message : '批准失败')
    }
  }

  const handleReject = async (jr: JoinRequest) => {
    try {
      await rejectRequest.mutateAsync(jr.id)
      message.success(`已拒绝数字员工「${jr.name}」的入职申请`)
    } catch (err) {
      message.error(err instanceof ApiRequestError ? err.message : '拒绝失败')
    }
  }

  const pendingCount = requests?.length ?? 0

  return (
    <div style={{ height: '100%', overflow: 'hidden' }}>
      <div style={{ display: 'flex', gap: 24, height: '100%', minHeight: 0 }}>
        {/* Left: Invite Prompt + Skill Instructions */}
        <div style={{ flex: 1, minWidth: 0, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 24 }}>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
              <RobotOutlined style={{ fontSize: 18, color: '#6d5ff5' }} />
              <Title level={5} style={{ margin: 0 }}>招聘数字员工</Title>
            </div>
            <Paragraph type="secondary" style={{ fontSize: 13 }}>复制以下提示词并发送给 Agent，Agent 将自动发起入职申请。</Paragraph>
            {promptLoading ? (
              <Skeleton active paragraph={{ rows: 4 }} />
            ) : (
              <div style={{ position: 'relative' }}>
                <pre style={{ borderRadius: 12, background: 'rgba(255,255,255,0.05)', padding: 16, fontSize: 13, lineHeight: 1.7, whiteSpace: 'pre-wrap', color: 'rgba(255,255,255,0.85)', maxHeight: 320, overflow: 'auto' }}>
                  {invite?.prompt}
                </pre>
                <Button
                  size="small"
                  style={{ position: 'absolute', top: 8, right: 8 }}
                  icon={copiedKey === INVITE_PROMPT_KEY ? <CheckOutlined /> : <CopyOutlined />}
                  onClick={handleCopy}
                >
                  {copiedKey === INVITE_PROMPT_KEY ? '已复制' : '复制'}
                </Button>
              </div>
            )}
          </div>

          <div>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <ToolOutlined style={{ fontSize: 18, color: '#22d3ee' }} />
                <Title level={5} style={{ margin: 0 }}>添加 / 更新 Skill</Title>
              </div>
              <Select
                value={skillAdapter}
                onChange={(v) => setSkillAdapter(v as SkillAdapter)}
                style={{ width: 140 }}
                options={[
                  { label: 'openclaw', value: 'openclaw' },
                  { label: 'opencode', value: 'opencode' },
                ]}
              />
            </div>
            <Paragraph type="secondary" style={{ fontSize: 13 }}>数字员工入职后，复制以下指令发送给对应 Agent，使其安装或更新 Skill。</Paragraph>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              {skillInstructions.map((skill) => (
                <div key={skill.key} style={{ borderRadius: 12, border: '1px solid rgba(255,255,255,0.08)', padding: 16, background: 'rgba(255,255,255,0.02)' }}>
                  <div style={{ marginBottom: 8 }}>
                    <Text strong>{skill.title}</Text>
                    <Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>{skill.description}</Paragraph>
                  </div>
                  <div style={{ position: 'relative' }}>
                    <pre style={{ borderRadius: 8, background: 'rgba(255,255,255,0.04)', padding: '10px 72px 10px 12px', fontSize: 13, lineHeight: 1.7, whiteSpace: 'pre-wrap', color: 'rgba(255,255,255,0.85)', maxHeight: 160, overflow: 'auto' }}>
                      {skill.prompt}
                    </pre>
                    <Button
                      size="small"
                      style={{ position: 'absolute', top: 6, right: 6, height: 28, fontSize: 12 }}
                      icon={copiedKey === skill.key ? <CheckOutlined /> : <CopyOutlined />}
                      onClick={() => handleSkillCopy(skill.key, skill.prompt)}
                    >
                      {copiedKey === skill.key ? '已复制' : '复制'}
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Right: Pending Requests */}
        <div style={{ width: 480, flexShrink: 0, display: 'flex', flexDirection: 'column', minWidth: 0, borderLeft: '1px solid rgba(255,255,255,0.08)', paddingLeft: 24 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
            <CheckSquareOutlined style={{ fontSize: 18, color: '#f59e0b' }} />
            <Title level={5} style={{ margin: 0 }}>入职审批</Title>
            {pendingCount > 0 && (
              <Tag color="orange">{pendingCount}</Tag>
            )}
          </div>

          {pendingCount === 0 ? (
            <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', borderRadius: 12, border: '1px dashed rgba(255,255,255,0.15)' }}>
              <Empty description="暂无待审批的入职申请" />
            </div>
          ) : (
            <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 12 }}>
              {requests!.map((jr) => (
                <div key={jr.id} style={{ borderRadius: 12, border: '1px solid rgba(255,255,255,0.08)', padding: 16, background: 'rgba(255,255,255,0.02)' }}>
                  {editingId === jr.id ? (
                    <Space direction="vertical" style={{ width: '100%' }}>
                      <Input value={editName} onChange={(e) => setEditName(e.target.value)} placeholder="数字员工名称" />
                      <Select
                        value={editRole}
                        onChange={(v) => setEditRole(v as AgentRole)}
                        style={{ width: '100%' }}
                        options={[
                          { label: 'PM', value: 'pm' },
                          { label: '开发者', value: 'developer' },
                          { label: '审核者', value: 'reviewer' },
                          { label: '自定义', value: 'custom' },
                        ]}
                      />
                      <Input.TextArea value={editDescription} onChange={(e) => setEditDescription(e.target.value)} placeholder="描述" rows={2} />
                    </Space>
                  ) : (
                    <div style={{ display: 'flex', gap: 12 }}>
                      <Avatar
                        size={48}
                        style={{ flexShrink: 0, background: 'linear-gradient(135deg, #3b82f6, #22d3ee)' }}
                      >
                        {jr.name.slice(0, 1).toUpperCase()}
                      </Avatar>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                          <Text strong style={{ fontSize: 15 }}>{jr.name}</Text>
                          <Tag>{ROLE_LABELS[jr.role] ?? jr.role}</Tag>
                          {jr.agent_product && <Tag color="purple">{jr.agent_product}</Tag>}
                        </div>
                        <Paragraph type="secondary" style={{ fontSize: 13, marginTop: 6, marginBottom: 4 }} ellipsis={{ rows: 2 }}>
                          {jr.description || '无描述'}
                        </Paragraph>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 12, fontSize: 12, color: 'rgba(255,255,255,0.5)' }}>
                          <span style={{ fontFamily: "'JetBrains Mono', monospace" }}>{jr.node_id}</span>
                          {jr.capabilities?.length > 0 && <span>能力: {jr.capabilities.join(', ')}</span>}
                        </div>
                      </div>
                    </div>
                  )}

                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 8, paddingTop: 12, marginTop: 12, borderTop: '1px solid rgba(255,255,255,0.08)' }}>
                    {editingId === jr.id ? (
                      <Button size="small" onClick={() => setEditingId(null)}>取消</Button>
                    ) : (
                      <Button size="small" icon={<EditOutlined />} onClick={() => startEdit(jr)}>编辑</Button>
                    )}
                    <Button size="small" danger icon={<CloseOutlined />} onClick={() => handleReject(jr)} loading={rejectRequest.isPending}>
                      拒绝
                    </Button>
                    <Button size="small" type="primary" icon={<CheckOutlined />} onClick={() => handleApprove(jr)} loading={approveRequest.isPending}>
                      批准
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}