import { Modal, App, Alert } from 'antd'
import { useDeleteAgent } from '@/hooks/useAgents'
import type { Agent } from '@/types'

interface Props {
  open: boolean
  onClose: (open: boolean) => void
  agent: Agent | undefined
  onArchived?: () => void
}

function formatUsage(agent: Agent) {
  const parts: string[] = []
  if (agent.usage.project_count > 0) parts.push(`${agent.usage.project_count} 个项目`)
  if (agent.usage.task_count > 0) parts.push(`${agent.usage.task_count} 个任务`)
  if (agent.usage.todo_count > 0) parts.push(`${agent.usage.todo_count} 个 Todo`)
  return parts.join('、')
}

export function ArchiveAgentModal({ open, onClose, agent, onArchived }: Props) {
  const { message } = App.useApp()
  const deleteAgent = useDeleteAgent()
  const alreadyArchived = agent?.archived === true

  const handleArchive = async () => {
    if (!agent || agent.archived) return
    try {
      await deleteAgent.mutateAsync(agent.id)
      message.success(`数字员工 "${agent.name}" 已离职`)
      onClose(false)
      onArchived?.()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '离职处理失败')
    }
  }

  return (
    <Modal
      open={open}
      title="数字员工离职"
      onCancel={() => onClose(false)}
      onOk={handleArchive}
      okText={deleteAgent.isPending ? '处理中...' : '确认离职'}
      okButtonProps={{ danger: true, loading: deleteAgent.isPending, disabled: !agent || alreadyArchived }}
      cancelText="取消"
      destroyOnClose
    >
      <p className="text-sm text-white/70">
        {alreadyArchived
          ? '这个数字员工已经离职，无需重复操作。'
          : '离职后数字员工会从列表中隐藏，且不能被分配到新的项目或任务。'}
      </p>
      <Alert
        className="mt-4"
        type="warning"
        showIcon
        message={agent?.name ?? '当前数字员工'}
        description={
          alreadyArchived
            ? '数字员工已经处于离职状态。'
            : agent?.usage.in_use
              ? `当前被 ${formatUsage(agent)} 引用。离职不会影响已关联的历史任务和数据，但该数字员工的节点 ID 将被释放。`
              : '离职不会影响已关联的历史任务和数据，但该数字员工的节点 ID 将被释放。'
        }
      />
    </Modal>
  )
}
