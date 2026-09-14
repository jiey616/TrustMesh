import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { Select, SelectTrigger, SelectContent, SelectItem } from '@/components/ui/select'
import { ApiRequestError } from '@/api/client'
import { truncateNodeId } from '@/lib/utils'
import { ProjectDialog } from '@/components/project/ProjectDialog'
import { useUpdateProject } from '@/hooks/useProjects'
import { useAgents } from '@/hooks/useAgents'
import type { Project } from '@/types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  project: Project | undefined
}

export function EditProjectDialog({ open, onOpenChange, project }: Props) {
  const [error, setError] = useState('')
  const [pmAgentId, setPmAgentId] = useState(project?.pm_agent?.id ?? '')
  const updateProject = useUpdateProject()
  const { data: agents } = useAgents()

  const pmAgents = agents?.filter((a) => a.role === 'pm') ?? []
  const selectedAgent = pmAgents.find((a) => a.id === pmAgentId)
  const displayLabel = selectedAgent
    ? `${selectedAgent.name} (${truncateNodeId(selectedAgent.node_id)}) - ${selectedAgent.status === 'online' ? '在线' : '离线'}`
    : project?.pm_agent
      ? `${project.pm_agent.name} (${truncateNodeId(project.pm_agent.node_id)})`
      : '选择 PM Agent...'

  const initialValues = useMemo(
    () => ({
      name: project?.name ?? '',
      description: project?.description ?? '',
    }),
    [project?.description, project?.name],
  )

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setError('')
      setPmAgentId(project?.pm_agent?.id ?? '')
    }
    onOpenChange(nextOpen)
  }

  const handleSubmit = async ({ name, description }: { name: string; description: string }) => {
    if (!project) {
      return
    }

    setError('')
    try {
      await updateProject.mutateAsync({
        id: project.id,
        input: {
          name,
          description,
          ...(pmAgentId !== project.pm_agent?.id ? { pm_agent_id: pmAgentId } : {}),
        },
      })
      toast.success('项目已更新')
      handleOpenChange(false)
    } catch (err) {
      const message = err instanceof ApiRequestError ? err.message : '更新失败'
      toast.error(message)
      setError(message)
    }
  }

  return (
    <ProjectDialog
      key={`${project?.id ?? 'project'}-${open ? 'open' : 'closed'}`}
      open={open}
      onOpenChange={handleOpenChange}
      title="编辑项目"
      description="更新项目名称、描述和 PM Agent"
      submitLabel="保存修改"
      pending={updateProject.isPending}
      error={error}
      submitDisabled={!project || !pmAgentId}
      initialName={initialValues.name}
      initialDescription={initialValues.description}
      onSubmit={handleSubmit}
    >
      <div className="flex flex-col gap-2">
        <label className="text-sm font-medium">PM Agent</label>
        <Select value={pmAgentId} onValueChange={(val) => setPmAgentId(val ?? '')}>
          <SelectTrigger className="w-full">
            <span className="truncate">{displayLabel}</span>
          </SelectTrigger>
          <SelectContent>
            {pmAgents.map((agent) => (
              <SelectItem key={agent.id} value={agent.id}>
                {agent.name} ({truncateNodeId(agent.node_id)}) - {agent.status === 'online' ? '在线' : '离线'}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </ProjectDialog>
  )
}
