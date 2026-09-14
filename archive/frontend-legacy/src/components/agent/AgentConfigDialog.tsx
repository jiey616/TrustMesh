import { useState } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import { X } from 'lucide-react'
import { useCreateAgent, useUpdateAgent } from '@/hooks/useAgents'
import { ApiRequestError } from '@/api/client'
import { toast } from 'sonner'
import type { Agent, AgentRole } from '@/types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  agent?: Agent | null
  onCreated?: (id: string) => void
}

interface AgentConfigFormProps {
  agent?: Agent | null
  onOpenChange: (open: boolean) => void
  onCreated?: (id: string) => void
}

function AgentConfigForm({ agent, onOpenChange, onCreated }: AgentConfigFormProps) {
  const [nodeId, setNodeId] = useState(agent?.node_id ?? '')
  const [name, setName] = useState(agent?.name ?? '')
  const [role, setRole] = useState<AgentRole>(agent?.role ?? 'developer')
  const [description, setDescription] = useState(agent?.description ?? '')
  const [capabilities, setCapabilities] = useState<string[]>([...(agent?.capabilities ?? [])])
  const [capInput, setCapInput] = useState('')

  const createAgent = useCreateAgent()
  const updateAgent = useUpdateAgent()
  const isEditing = !!agent

  const addCapability = () => {
    const trimmed = capInput.trim()
    if (trimmed && !capabilities.includes(trimmed)) {
      setCapabilities([...capabilities, trimmed])
      setCapInput('')
    }
  }

  const removeCapability = (cap: string) => {
    setCapabilities(capabilities.filter((c) => c !== cap))
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      if (isEditing) {
        await updateAgent.mutateAsync({
          id: agent.id,
          input: { name, role, description, capabilities },
        })
      } else {
        const res = await createAgent.mutateAsync({
          node_id: nodeId,
          name,
          role,
          description,
          capabilities,
        })
        onCreated?.(res.data.id)
      }
      toast.success(isEditing ? 'Agent 已更新' : 'Agent 已添加')
      onOpenChange(false)
    } catch (err) {
      const message = err instanceof ApiRequestError ? err.message : '操作失败'
      toast.error(message)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="mt-2 flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <label className="text-sm font-medium">节点 ID</label>
        <Input
          value={nodeId}
          onChange={(e) => setNodeId(e.target.value)}
          placeholder="node-dev-001"
          required
          disabled={isEditing}
        />
        {isEditing && (
          <p className="text-xs text-muted-foreground">节点 ID 创建后不可修改</p>
        )}
      </div>
      <div className="flex flex-col gap-2">
        <label className="text-sm font-medium">名称</label>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Backend Agent A"
          required
        />
      </div>
      <div className="flex flex-col gap-2">
        <label className="text-sm font-medium">角色</label>
        <Select value={role} onValueChange={(val) => setRole(val as AgentRole)}>
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="pm">PM</SelectItem>
            <SelectItem value="developer">开发者</SelectItem>
            <SelectItem value="reviewer">审核者</SelectItem>
            <SelectItem value="custom">自定义</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="flex flex-col gap-2">
        <label className="text-sm font-medium">描述</label>
        <Textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="描述 Agent 的职责"
          rows={2}
          required
        />
      </div>
      <div className="flex flex-col gap-2">
        <label className="text-sm font-medium">能力标签</label>
        <div className="flex gap-2">
          <Input
            value={capInput}
            onChange={(e) => setCapInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                addCapability()
              }
            }}
            placeholder="输入后回车添加"
          />
          <Button type="button" variant="outline" onClick={addCapability}>
            添加
          </Button>
        </div>
        {capabilities.length > 0 && (
          <div className="mt-1 flex flex-wrap gap-1">
            {capabilities.map((cap) => (
              <Badge key={cap} variant="secondary" className="gap-1">
                {cap}
                <button type="button" onClick={() => removeCapability(cap)} className="cursor-pointer">
                  <X className="size-3" />
                </button>
              </Badge>
            ))}
          </div>
        )}
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
          取消
        </Button>
        <Button type="submit" disabled={createAgent.isPending || updateAgent.isPending}>
          {isEditing ? '保存' : '添加'}
        </Button>
      </DialogFooter>
    </form>
  )
}

export function AgentConfigDialog({ open, onOpenChange, agent, onCreated }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{agent ? '编辑 Agent' : '添加 Agent'}</DialogTitle>
        </DialogHeader>
        {open ? (
          <AgentConfigForm
            key={`${agent?.id ?? 'new'}:${open ? 'open' : 'closed'}`}
            agent={agent}
            onOpenChange={onOpenChange}
            onCreated={onCreated}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
