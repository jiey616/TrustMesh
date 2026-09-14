import { useState } from 'react'
import { Button, Drawer, App, Tooltip, Empty, Typography, Tag, Switch } from 'antd'
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  CopyOutlined,
  NodeIndexOutlined,
  ClockCircleOutlined,
  ApartmentOutlined,
  StarFilled,
  StarOutlined,
} from '@ant-design/icons'
import {
  useWorkflowTemplates,
  useCreateWorkflowTemplate,
  useUpdateWorkflowTemplate,
  useCopyWorkflowTemplate,
  useDeleteWorkflowTemplate,
  useCurateWorkflowTemplate,
} from '@/hooks/useWorkflows'
import { useAgents } from '@/hooks/useAgents'
import { WorkflowCanvasEditor } from '@/components/project/WorkflowCanvasEditor'
import { AgentAvatar } from '@/components/shared/AgentAvatar'
import { PageHeader } from '@/components/shared/PageHeader'
import type { WorkflowTemplate } from '@/types'

const { Text, Paragraph } = Typography

const emptyTemplate = (): WorkflowTemplate => ({
  id: '',
  name: '',
  description: '',
  steps: [],
  version: 1,
  created_at: '',
  updated_at: '',
})

export function WorkflowTemplatesPage() {
  const { message } = App.useApp()
  const { data: templates = [] } = useWorkflowTemplates()
  const { data: agents } = useAgents()
  const createMutation = useCreateWorkflowTemplate()
  const updateMutation = useUpdateWorkflowTemplate()
  const copyMutation = useCopyWorkflowTemplate()
  const deleteMutation = useDeleteWorkflowTemplate()
  const curateMutation = useCurateWorkflowTemplate()
  const [onlyCurated, setOnlyCurated] = useState(false)

  const sortedTemplates = [...templates].sort((a, b) => {
    const ac = a.curated ? 1 : 0
    const bc = b.curated ? 1 : 0
    if (ac !== bc) return bc - ac
    return (b.created_at || '').localeCompare(a.created_at || '')
  })
  const visibleTemplates = onlyCurated ? sortedTemplates.filter((t) => t.curated) : sortedTemplates

  const [editing, setEditing] = useState<{ isNew: boolean; tpl: WorkflowTemplate } | null>(null)
  const [pending, setPending] = useState(false)
  const [deleting, setDeleting] = useState<string | null>(null)

  const validate = (tpl: WorkflowTemplate): string | null => {
    if (!tpl.name.trim()) return '请填写模板名称'
    if (tpl.steps.length === 0) return '请至少添加一个步骤'
    if (tpl.steps.some((s) => !s.name.trim() || (!s.role?.trim() && !s.agent_id))) {
      return '每个步骤都需要填写名称并绑定数字员工'
    }
    return null
  }

  const handleSave = async () => {
    if (!editing) return
    const errMsg = validate(editing.tpl)
    if (errMsg) {
      message.error(errMsg)
      return
    }
    setPending(true)
    try {
      if (editing.isNew) {
        await createMutation.mutateAsync({
          name: editing.tpl.name,
          description: editing.tpl.description,
          steps: editing.tpl.steps,
        })
        message.success('模板已创建')
      } else {
        await updateMutation.mutateAsync({
          id: editing.tpl.id,
          input: {
            name: editing.tpl.name,
            description: editing.tpl.description,
            steps: editing.tpl.steps,
          },
        })
        message.success('模板已保存')
      }
      setEditing(null)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setPending(false)
    }
  }

  const handleCopy = async (id: string) => {
    setPending(true)
    try {
      await copyMutation.mutateAsync(id)
      message.success('已复制为新模板')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '复制失败')
    } finally {
      setPending(false)
    }
  }

  const handleDelete = async () => {
    if (!deleting) return
    setPending(true)
    try {
      await deleteMutation.mutateAsync(deleting)
      message.success('模板已删除')
      setDeleting(null)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    } finally {
      setPending(false)
    }
  }

  const findAgent = (agentId?: string) => (agents ?? []).find((a) => a.id === agentId)

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <PageHeader
        title="全局工作流模板"
        icon={<ApartmentOutlined />}
        subtitle="模板可被多个项目继承并二次修改；模板更新后，继承它的项目可手动同步"
        actions={
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <span style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>只看精选</span>
            <Switch size="small" checked={onlyCurated} onChange={setOnlyCurated} />
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setEditing({ isNew: true, tpl: emptyTemplate() })}>
              新建模板
            </Button>
          </div>
        }
      />

      <div style={{ flex: 1, overflowY: 'auto', paddingRight: 2 }}>
        {templates.length === 0 ? (
          <Empty
            description={<span style={{ color: 'var(--text-tertiary)' }}>还没有全局工作流模板</span>}
            style={{ padding: '48px 0' }}
          >
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setEditing({ isNew: true, tpl: emptyTemplate() })}>
              新建模板
            </Button>
          </Empty>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(420px, 1fr))', gap: 12 }}>
            {visibleTemplates.map((tpl) => (
              <div
                key={tpl.id}
                className="glass-panel"
                style={{
                  padding: '14px 16px',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 10,
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <NodeIndexOutlined style={{ color: 'var(--signal)', fontSize: 15 }} />
                  <Text strong style={{ color: 'var(--text-primary)', fontSize: 14, flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {tpl.name || '未命名模板'}
                  </Text>
                  <Tag color="purple" style={{ marginInlineEnd: 0 }}>v{tpl.version}</Tag>
                  {tpl.curated && <Tag color="gold" style={{ marginInlineEnd: 0 }}>精选</Tag>}
                  <span style={{ fontSize: 11, color: 'var(--text-tertiary)', background: 'var(--surface-raised)', borderRadius: 'var(--radius-pill)', padding: '1px 8px', flexShrink: 0 }}>
                    {tpl.steps.length} 步
                  </span>
                  <Tooltip title={tpl.curated ? '取消精选' : '标为精选'}>
                    <Button
                      type="text"
                      size="small"
                      icon={tpl.curated ? <StarFilled /> : <StarOutlined />}
                      style={{ color: tpl.curated ? 'var(--warning)' : 'var(--text-tertiary)' }}
                      loading={curateMutation.isPending && curateMutation.variables?.id === tpl.id}
                      onClick={() => {
                        curateMutation
                          .mutateAsync({ id: tpl.id, curated: !tpl.curated })
                          .catch((err: unknown) => message.error(err instanceof Error ? err.message : '操作失败'))
                      }}
                    />
                  </Tooltip>
                  <Tooltip title="复制为新模板">
                    <Button type="text" size="small" icon={<CopyOutlined />} style={{ color: 'var(--text-tertiary)' }} onClick={() => handleCopy(tpl.id)} />
                  </Tooltip>
                  <Tooltip title="编辑">
                    <Button type="text" size="small" icon={<EditOutlined />} style={{ color: 'var(--text-tertiary)' }} onClick={() => setEditing({ isNew: false, tpl: { ...tpl, steps: tpl.steps.map((s) => ({ ...s })) } })} />
                  </Tooltip>
                  <Tooltip title="删除">
                    <Button type="text" size="small" icon={<DeleteOutlined />} style={{ color: 'var(--text-quaternary)' }} onClick={() => setDeleting(tpl.id)} />
                  </Tooltip>
                </div>

                {tpl.description ? (
                  <Text type="secondary" style={{ fontSize: 12, paddingLeft: 23 }} ellipsis={{ tooltip: tpl.description }}>
                    {tpl.description}
                  </Text>
                ) : null}

                {/* 步骤链预览 */}
                <div style={{ display: 'flex', alignItems: 'center', gap: 4, flexWrap: 'wrap', paddingLeft: 23 }}>
                  {tpl.steps.map((s, si) => {
                    const agent = findAgent(s.agent_id)
                    return (
                      <span key={si} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                        <span
                          className="glass-panel"
                          style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: 6,
                            padding: '3px 8px 3px 3px',
                            fontSize: 12,
                            color: 'var(--text-secondary)',
                            maxWidth: 220,
                          }}
                          title={`${s.name}${agent ? ` @${agent.name}` : s.role ? ` (${s.role})` : ''}`}
                        >
                          {agent ? (
                            <AgentAvatar name={agent.name} role={agent.role} seed={agent.node_id} size={18} />
                          ) : (
                            <span style={{ width: 18, height: 18, borderRadius: 'var(--radius-avatar)', border: '1px dashed var(--line-strong)', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, color: 'var(--text-tertiary)', flexShrink: 0 }}>
                              {si + 1}
                            </span>
                          )}
                          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                            {s.name || '未命名'}
                          </span>
                          {s.need_review && <ClockCircleOutlined style={{ color: 'var(--warning)', fontSize: 11, flexShrink: 0 }} />}
                        </span>
                        {si < tpl.steps.length - 1 && <span style={{ color: 'var(--text-quaternary)', fontSize: 11 }}>→</span>}
                      </span>
                    )
                  })}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 新建 / 编辑画布 Drawer */}
      <Drawer
        title={editing?.isNew ? '新建模板' : '编辑模板'}
        open={!!editing}
        onClose={() => !pending && setEditing(null)}
        width={560}
        styles={{ body: { paddingTop: 16 } }}
        extra={
          <Button type="primary" loading={pending} disabled={!editing?.tpl.name.trim()} onClick={handleSave}>
            {editing?.isNew ? '创建模板' : '保存模板'}
          </Button>
        }
      >
        {editing && (
          <>
            <div style={{ marginBottom: 16 }}>
              <Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 6 }}>
                模板名称
              </Text>
              <input
                value={editing.tpl.name}
                onChange={(e) => setEditing({ ...editing, tpl: { ...editing.tpl, name: e.target.value } })}
                placeholder="如：画宗AIGC产线工作流"
                className="ant-input"
                style={{
                  width: '100%',
                  padding: '8px 12px',
                  borderRadius: 'var(--radius-control)',
                  border: '1px solid var(--line-strong)',
                  background: 'var(--surface)',
                  color: 'var(--text-primary)',
                  fontSize: 14,
                  outline: 'none',
                }}
              />
            </div>
            <WorkflowCanvasEditor
              workflow={{ name: editing.tpl.name, steps: editing.tpl.steps }}
              onChange={(wf) => setEditing({ ...editing, tpl: { ...editing.tpl, name: wf.name, steps: wf.steps } })}
              agents={agents ?? []}
            />
          </>
        )}
      </Drawer>

      {/* 删除确认 */}
      <Drawer
        title="删除模板？"
        open={deleting !== null}
        onClose={() => !pending && setDeleting(null)}
        width={360}
        extra={
          <>
            <Button style={{ marginRight: 8 }} disabled={pending} onClick={() => setDeleting(null)}>
              取消
            </Button>
            <Button danger type="primary" loading={pending} onClick={handleDelete}>
              删除
            </Button>
          </>
        }
      >
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          已继承该模板的项目不受影响（项目内保留的是克隆副本）。此操作不可撤销。
        </Paragraph>
      </Drawer>
    </div>
  )
}
