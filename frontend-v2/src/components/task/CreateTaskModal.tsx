import { useEffect, useState } from 'react'
import { Modal, Form, Input, Select, App, Typography } from 'antd'
import { useCreateTask } from '@/hooks/useTasks'
import { useProjects, useProject } from '@/hooks/useProjects'
import { useAgents } from '@/hooks/useAgents'
import { FileSelector } from '@/components/shared/FileSelector'
import type { TaskPriority } from '@/types'

const { Text } = Typography

interface Props {
  open: boolean
  onClose: (open: boolean) => void
  projectId?: string
  defaultAgentId?: string
  onCreated?: (taskId: string) => void
}

const priorityOptions: { value: TaskPriority; label: string }[] = [
  { value: 'low', label: '低' },
  { value: 'medium', label: '中' },
  { value: 'high', label: '高' },
  { value: 'urgent', label: '紧急' },
]

export function CreateTaskModal({ open, onClose, projectId: fixedProjectId, defaultAgentId, onCreated }: Props) {
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const createTask = useCreateTask()
  const { data: agents } = useAgents()
  const { data: projects } = useProjects()
  const [selectedProjectId, setSelectedProjectId] = useState(fixedProjectId ?? '')
  const effectiveProjectId = fixedProjectId ?? selectedProjectId
  const { data: project } = useProject(effectiveProjectId)
  const [agentId, setAgentId] = useState(defaultAgentId ?? '')
  const [selectedFileIds, setSelectedFileIds] = useState<string[]>([])
  const [stepRange, setStepRange] = useState<{ from: number; to: number } | null>(null)

  const executorAgents = (Array.isArray(agents) ? agents : []).filter((a) => !a.archived && a.role !== 'pm')
  const activeProjects = (Array.isArray(projects) ? projects : []).filter((p) => p.status !== 'archived')

  // 项目总流程（primary workflow）及其步骤段选择
  const primaryWf =
    project && project.primary_workflow_index >= 0 && Array.isArray(project.workflows)
      ? project.workflows[project.primary_workflow_index]
      : undefined
  useEffect(() => {
    setStepRange(null)
  }, [effectiveProjectId])
  const rangeValue = stepRange ?? (primaryWf ? { from: 0, to: primaryWf.steps.length - 1 } : null)
  const stepOptions = (primaryWf?.steps ?? []).map((s, i) => ({
    value: i,
    label: `步骤 ${i + 1} · ${s.name}`,
  }))

  const handleClose = (next: boolean) => {
    if (!next) {
      setSelectedFileIds([])
      form.resetFields()
    }
    onClose(next)
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()
      if (!effectiveProjectId) {
        message.error('请选择所属项目')
        return
      }
      if (!agentId) {
        message.error('请选择执行数字员工')
        return
      }
      const res = await createTask.mutateAsync({
        projectId: effectiveProjectId,
        input: {
          title: values.title.trim(),
          description: values.description.trim(),
          priority: values.priority,
          assignee_agent_id: agentId,
          file_ids: selectedFileIds.length > 0 ? selectedFileIds : undefined,
          // 绑定项目总流程的负责步骤段（如果有总流程）
          workflow_index: primaryWf && project ? project.primary_workflow_index : undefined,
          step_from: primaryWf && rangeValue ? rangeValue.from : undefined,
          step_to: primaryWf && rangeValue ? rangeValue.to : undefined,
        },
      })
      message.success('任务已创建')
      const taskId = (res.data as { id: string }).id
      handleClose(false)
      onCreated?.(taskId)
    } catch (err) {
      if (err && typeof err === 'object' && 'errorFields' in err) return
      message.error(err instanceof Error ? err.message : '创建任务失败')
    }
  }

  return (
    <Modal
      open={open}
      title="创建任务"
      onCancel={() => handleClose(false)}
      onOk={handleSubmit}
      confirmLoading={createTask.isPending}
      okText="创建任务"
      cancelText="取消"
      destroyOnClose
      width={560}
    >
      <Form form={form} layout="vertical" className="mt-4" initialValues={{ priority: 'medium' }}>
        {!fixedProjectId && (
          <Form.Item label="所属项目" required>
            <Select
              value={selectedProjectId || undefined}
              onChange={(v) => {
                setSelectedProjectId(v)
                setSelectedFileIds([])
              }}
              placeholder="选择项目..."
              options={activeProjects?.map((p) => ({ value: p.id, label: p.name }))}
              notFoundContent={activeProjects?.length === 0 ? '暂无可用项目，请先创建项目' : null}
            />
          </Form.Item>
        )}

        <Form.Item name="title" label="任务标题" rules={[{ required: true, message: '请输入任务标题' }]}>
          <Input placeholder="例如：实现用户登录功能" />
        </Form.Item>

        <Form.Item name="description" label="任务描述" rules={[{ required: true, message: '请输入任务描述' }]}>
          <Input.TextArea rows={3} placeholder="描述任务的目标和要求" />
        </Form.Item>

        <Form.Item name="priority" label="优先级">
          <Select options={priorityOptions} />
        </Form.Item>

        {!defaultAgentId ? (
          <Form.Item label="执行数字员工" required>
            <Select
              value={agentId || undefined}
              onChange={setAgentId}
              placeholder="选择执行数字员工..."
              options={executorAgents.map((a) => ({
                value: a.id,
                label: `${a.name} (${a.role}) - ${a.status === 'online' ? '在线' : '离线'}`,
              }))}
              notFoundContent={executorAgents.length === 0 ? '暂无可用的执行数字员工' : null}
            />
          </Form.Item>
        ) : (
          <Form.Item label="执行数字员工">
            <div className="rounded-md border border-white/10 bg-white/[0.03] px-3 py-2 text-sm text-white/70">
              {agents?.find((a) => a.id === defaultAgentId)?.name ?? defaultAgentId}
            </div>
          </Form.Item>
        )}

        {effectiveProjectId && primaryWf && rangeValue && (
          <Form.Item
            label="负责总流程步骤"
            tooltip="该任务在项目总流程中负责的步骤段；前序任务的最终产物会按输入定义传给后续任务"
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <Select
                style={{ flex: 1, minWidth: 0 }}
                value={rangeValue.from}
                onChange={(v: number) => setStepRange({ from: v, to: Math.max(v, rangeValue.to) })}
                options={stepOptions}
              />
              <Text type="secondary" style={{ fontSize: 12 }}>至</Text>
              <Select
                style={{ flex: 1, minWidth: 0 }}
                value={rangeValue.to}
                onChange={(v: number) => setStepRange({ from: Math.min(rangeValue.from, v), to: v })}
                options={stepOptions}
              />
            </div>
          </Form.Item>
        )}

        {effectiveProjectId && (
          <FileSelector
            projectId={effectiveProjectId}
            selectedIds={selectedFileIds}
            onToggle={(id) =>
              setSelectedFileIds((prev) => (prev.includes(id) ? prev.filter((f) => f !== id) : [...prev, id]))
            }
          />
        )}
      </Form>
    </Modal>
  )
}
