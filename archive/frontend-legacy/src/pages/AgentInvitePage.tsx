import { useState } from 'react'
import { Copy, Check, X, Network, UserPlus, ClipboardCheck, Wrench } from 'lucide-react'
import { PageContainer } from '@/components/layout/PageContainer'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Avatar } from '@/components/ui/avatar'
import { Input } from '@/components/ui/input'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@/components/ui/select'
import { useInvitePrompt, useJoinRequests, useApproveJoinRequest, useRejectJoinRequest } from '@/hooks/useJoinRequests'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { ApiRequestError } from '@/api/client'
import { toast } from 'sonner'
import type { JoinRequest, AgentRole } from '@/types'

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
    description: '适用于 PM Agent，提供需求澄清、任务规划、任务创建的完整工作流。',
    url: 'https://github.com/yuanjun5681/TrustMesh/blob/main/skills/tm-task-plan/SKILL.md',
  },
  {
    key: 'tm-task-exec',
    title: '执行 Agent 任务执行 Skill',
    description: '适用于执行 Agent，提供 Todo 接收、进度回报、结果交付的完整工作流。',
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

  const skillInstructions = SKILL_DEFINITIONS.map((skill) => ({
    ...skill,
    prompt: buildSkillPrompt(skillAdapter, skill.key, skill.url),
  }))

  const handleCopy = async () => {
    if (!invite?.prompt) return
    const ok = await copy(invite.prompt, INVITE_PROMPT_KEY)
    if (ok) {
      toast.success('提示词已复制到剪贴板')
    } else {
      toast.error('复制失败，请手动选择复制')
    }
  }

  const handleSkillCopy = async (key: string, prompt: string) => {
    const ok = await copy(prompt, key)
    if (ok) {
      toast.success('指令已复制到剪贴板')
    } else {
      toast.error('复制失败，请手动选择复制')
    }
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
      toast.success(`已录用 Agent「${overrides?.name || jr.name}」`)
      setEditingId(null)
    } catch (err) {
      toast.error(err instanceof ApiRequestError ? err.message : '批准失败')
    }
  }

  const handleReject = async (jr: JoinRequest) => {
    try {
      await rejectRequest.mutateAsync(jr.id)
      toast.success(`已拒绝 Agent「${jr.name}」的入职申请`)
    } catch (err) {
      toast.error(err instanceof ApiRequestError ? err.message : '拒绝失败')
    }
  }

  const pendingCount = requests?.length ?? 0

  return (
    <PageContainer className="h-full overflow-hidden">
      <div className="flex h-full gap-6 min-h-0">
        {/* Left: Invite Prompt + Skill Instructions */}
        <div className="w-1/2 min-w-0 overflow-y-auto space-y-6">
          {/* Invite Prompt */}
          <div className="flex flex-col min-h-0">
            <div className="flex items-center gap-2 mb-4">
              <UserPlus className="size-5 text-muted-foreground" />
              <h2 className="text-lg font-semibold">招聘智能体</h2>
            </div>
            <p className="text-sm text-muted-foreground mb-3">
              复制以下提示词并发送给 Agent，Agent 将自动发起入职申请。
            </p>
            {promptLoading ? (
              <div className="h-48 rounded-lg bg-muted animate-pulse" />
            ) : (
              <div className="relative">
                <pre className="rounded-lg bg-muted p-4 text-sm leading-relaxed whitespace-pre-wrap">
                  {invite?.prompt}
                </pre>
                <Button
                  size="sm"
                  variant="outline"
                  className="absolute top-2 right-2"
                  onClick={handleCopy}
                >
                  {copiedKey === INVITE_PROMPT_KEY ? <Check className="size-4 mr-1" /> : <Copy className="size-4 mr-1" />}
                  {copiedKey === INVITE_PROMPT_KEY ? '已复制' : '复制'}
                </Button>
              </div>
            )}
          </div>

          {/* Skill Instructions */}
          <div>
            <div className="flex items-center justify-between gap-3 mb-2">
              <div className="flex items-center gap-2">
                <Wrench className="size-5 text-muted-foreground" />
                <h2 className="text-lg font-semibold">添加 / 更新 Skill</h2>
              </div>
              <Select value={skillAdapter} onValueChange={(value) => setSkillAdapter(value as SkillAdapter)}>
                <SelectTrigger className="w-36 h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="openclaw">openclaw</SelectItem>
                  <SelectItem value="opencode">opencode</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <p className="text-sm text-muted-foreground mb-4">
              Agent 入职后，复制以下指令发送给对应 Agent，使其安装或更新 Skill。
            </p>
            <div className="space-y-4">
              {skillInstructions.map((skill) => (
                <div key={skill.key} className="rounded-lg border p-4 space-y-2">
                  <div className="flex items-center justify-between">
                    <div>
                      <h3 className="text-sm font-semibold">{skill.title}</h3>
                      <p className="text-xs text-muted-foreground mt-0.5">{skill.description}</p>
                    </div>
                  </div>
                  <div className="relative">
                    <pre className="rounded-md bg-muted p-3 pr-16 text-sm leading-relaxed whitespace-pre-wrap">
                      {skill.prompt}
                    </pre>
                    <Button
                      size="sm"
                      variant="outline"
                      className="absolute top-1.5 right-1.5 h-7 text-xs"
                      onClick={() => handleSkillCopy(skill.key, skill.prompt)}
                    >
                      {copiedKey === skill.key ? <Check className="size-3.5 mr-1" /> : <Copy className="size-3.5 mr-1" />}
                      {copiedKey === skill.key ? '已复制' : '复制'}
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Right: Pending Requests */}
        <div className="flex flex-col w-1/2 min-w-0">
          <div className="flex items-center gap-2 mb-4">
            <ClipboardCheck className="size-5 text-muted-foreground" />
            <h2 className="text-lg font-semibold">入职审批</h2>
            {pendingCount > 0 && (
              <Badge variant="secondary" className="bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200">
                {pendingCount}
              </Badge>
            )}
          </div>

          {pendingCount === 0 ? (
            <div className="flex-1 flex items-center justify-center rounded-lg border border-dashed">
              <p className="text-sm text-muted-foreground">暂无待审批的入职申请</p>
            </div>
          ) : (
            <div className="flex-1 min-h-0 overflow-y-auto space-y-3">
              {requests!.map((jr) => (
                <div key={jr.id} className="rounded-lg border p-4 space-y-3">
                  {editingId === jr.id ? (
                    <div className="space-y-2">
                      <Input
                        value={editName}
                        onChange={(e) => setEditName(e.target.value)}
                        placeholder="Agent 名称"
                      />
                      <Select value={editRole} onValueChange={(v) => setEditRole(v as AgentRole)}>
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="pm">PM</SelectItem>
                          <SelectItem value="developer">开发者</SelectItem>
                          <SelectItem value="reviewer">审核者</SelectItem>
                          <SelectItem value="custom">自定义</SelectItem>
                        </SelectContent>
                      </Select>
                      <Input
                        value={editDescription}
                        onChange={(e) => setEditDescription(e.target.value)}
                        placeholder="描述"
                      />
                    </div>
                  ) : (
                    <div className="flex gap-3">
                      <Avatar
                        fallback={jr.name}
                        seed={jr.node_id}
                        kind="agent"
                        role={jr.role}
                        size="lg"
                        className="shrink-0"
                      />
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 flex-wrap">
                          <span className="font-semibold text-base">{jr.name}</span>
                          <Badge variant="outline">{ROLE_LABELS[jr.role] ?? jr.role}</Badge>
                          {jr.agent_product && (
                            <Badge variant="secondary">{jr.agent_product}</Badge>
                          )}
                        </div>
                        <p className="text-sm text-muted-foreground mt-1.5 line-clamp-2">
                          {jr.description || '无描述'}
                        </p>
                        <div className="flex items-center gap-3 mt-2 text-xs text-muted-foreground">
                          <span className="inline-flex items-center gap-1">
                            <Network className="size-3" />
                            {jr.node_id}
                          </span>
                          {jr.capabilities?.length > 0 && (
                            <span>能力: {jr.capabilities.join(', ')}</span>
                          )}
                        </div>
                      </div>
                    </div>
                  )}

                  <div className="flex items-center justify-end gap-2 pt-1 border-t">
                    {editingId === jr.id ? (
                      <Button size="sm" variant="ghost" onClick={() => setEditingId(null)}>
                        取消
                      </Button>
                    ) : (
                      <Button size="sm" variant="ghost" onClick={() => startEdit(jr)}>
                        编辑
                      </Button>
                    )}
                    <Button
                      size="sm"
                      variant="outline"
                      className="text-red-600 hover:text-red-700"
                      onClick={() => handleReject(jr)}
                      disabled={rejectRequest.isPending}
                    >
                      <X className="size-4 mr-1" />
                      拒绝
                    </Button>
                    <Button
                      size="sm"
                      onClick={() => handleApprove(jr)}
                      disabled={approveRequest.isPending}
                    >
                      <Check className="size-4 mr-1" />
                      批准
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </PageContainer>
  )
}
