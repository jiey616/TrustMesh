import { useState } from 'react'
import { Select, SelectTrigger, SelectContent, SelectItem } from '@/components/ui/select'
import { useCreateProject } from '@/hooks/useProjects'
import { useAgents } from '@/hooks/useAgents'
import { ApiRequestError } from '@/api/client'
import { toast } from 'sonner'
import { truncateNodeId } from '@/lib/utils'
import { ProjectDialog } from '@/components/project/ProjectDialog'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CreateProjectDialog({ open, onOpenChange }: Props) {
  const [pmAgentId, setPmAgentId] = useState('')
  const [error, setError] = useState('')
  const { data: agents } = useAgents()
  const createProject = useCreateProject()

  const pmAgents = agents?.filter((a) => a.role === 'pm') ?? []
  const selectedAgent = pmAgents.find((a) => a.id === pmAgentId)

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setPmAgentId('')
      setError('')
    }
    onOpenChange(nextOpen)
  }

  const handleSubmit = async ({ name, description }: { name: string; description: string }) => {
    setError('')
    try {
      await createProject.mutateAsync({
        name,
        description,
        pm_agent_id: pmAgentId,
      })
      toast.success('项目已创建')
      setPmAgentId('')
      handleOpenChange(false)
    } catch (err) {
      const message = err instanceof ApiRequestError ? err.message : '创建失败'
      toast.error(message)
      setError(message)
    }
  }

  return (
    <ProjectDialog
      key={open ? 'create-open' : 'create-closed'}
      open={open}
      onOpenChange={handleOpenChange}
      title="创建项目"
      description="新建一个 AI Agent 协作项目"
      submitLabel="创建项目"
      pendingLabel="创建中..."
      pending={createProject.isPending}
      submitDisabled={!pmAgentId}
      error={error}
      onSubmit={handleSubmit}
    >
      <div className="flex flex-col gap-2">
        <label className="text-sm font-medium">PM Agent</label>
        <Select value={pmAgentId} onValueChange={(val) => setPmAgentId(val ?? '')}>
          <SelectTrigger className="w-full">
            <span className="truncate">
              {selectedAgent
                ? `${selectedAgent.name} (${truncateNodeId(selectedAgent.node_id)}) - ${selectedAgent.status === 'online' ? '在线' : '离线'}`
                : '选择 PM Agent...'}
            </span>
          </SelectTrigger>
          <SelectContent>
            {pmAgents.map((agent) => (
              <SelectItem key={agent.id} value={agent.id}>
                {agent.name} ({truncateNodeId(agent.node_id)}) - {agent.status === 'online' ? '在线' : '离线'}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {pmAgents.length === 0 && (
          <p className="text-xs text-muted-foreground">
            暂无可用的 PM Agent，请先在 Agent 管理中添加
          </p>
        )}
      </div>
    </ProjectDialog>
  )
}
