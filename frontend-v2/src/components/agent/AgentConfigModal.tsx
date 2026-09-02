import { useEffect, useState } from 'react'
import { Modal, Form, Input, Select, App, Tag } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useUpdateAgent } from '@/hooks/useAgents'
import type { Agent, AgentRole } from '@/types'

interface Props {
  open: boolean
  onClose: (open: boolean) => void
  agent: Agent | undefined
}

const roleOptions: { value: AgentRole; label: string }[] = [
  { value: 'pm', label: 'PM' },
  { value: 'developer', label: '开发者' },
  { value: 'reviewer', label: '审核者' },
  { value: 'custom', label: '自定义' },
]

export function AgentConfigModal({ open, onClose, agent }: Props) {
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const updateAgent = useUpdateAgent()
  const [capInput, setCapInput] = useState('')
  const [capabilities, setCapabilities] = useState<string[]>([])

  useEffect(() => {
    if (open && agent) {
      form.setFieldsValue({
        name: agent.name,
        role: agent.role,
        description: agent.description,
      })
      setCapabilities([...(agent.capabilities ?? [])])
      setCapInput('')
    }
  }, [open, agent, form])

  const addCapability = () => {
    const v = capInput.trim()
    if (v && !capabilities.includes(v)) setCapabilities([...capabilities, v])
    setCapInput('')
  }

  const removeCapability = (cap: string) => setCapabilities(capabilities.filter((c) => c !== cap))

  const handleSubmit = async () => {
    if (!agent) return
    try {
      const values = await form.validateFields()
      await updateAgent.mutateAsync({
        id: agent.id,
        input: {
          name: values.name,
          role: values.role,
          description: values.description,
          capabilities,
        },
      })
      message.success('数字员工已更新')
      onClose(false)
    } catch (err) {
      if (err && typeof err === 'object' && 'errorFields' in err) return
      message.error(err instanceof Error ? err.message : '更新失败')
    }
  }

  return (
    <Modal
      open={open}
      title="编辑数字员工"
      onCancel={() => onClose(false)}
      onOk={handleSubmit}
      confirmLoading={updateAgent.isPending}
      okText="保存"
      cancelText="取消"
      destroyOnClose
    >
      <Form form={form} layout="vertical" className="mt-4">
        <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
          <Input placeholder="数字员工名称" />
        </Form.Item>
        <Form.Item name="role" label="角色" rules={[{ required: true, message: '请选择角色' }]}>
          <Select options={roleOptions} />
        </Form.Item>
        <Form.Item name="description" label="描述" rules={[{ required: true, message: '请输入描述' }]}>
          <Input.TextArea rows={2} placeholder="描述数字员工的职责" />
        </Form.Item>
        <div className="flex flex-col gap-2">
          <span className="text-sm font-medium text-white/80">能力标签</span>
          <div className="flex gap-2">
            <Input
              value={capInput}
              onChange={(e) => setCapInput(e.target.value)}
              onPressEnter={(e) => {
                e.preventDefault()
                addCapability()
              }}
              placeholder="输入后回车添加"
            />
            <button
              type="button"
              onClick={addCapability}
              className="flex items-center gap-1 rounded-md border border-white/15 px-3 text-sm text-white/80 hover:bg-white/5"
            >
              <PlusOutlined /> 添加
            </button>
          </div>
          {capabilities.length > 0 && (
            <div className="mt-1 flex flex-wrap gap-1">
              {capabilities.map((cap) => (
                <Tag
                  key={cap}
                  closable
                  onClose={() => removeCapability(cap)}
                  className="!text-[11px]"
                  color="purple"
                >
                  {cap}
                </Tag>
              ))}
            </div>
          )}
        </div>
      </Form>
    </Modal>
  )
}
