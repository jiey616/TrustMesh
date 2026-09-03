import { useState } from 'react'
import { Button, Drawer, App, Tooltip, Empty, Typography, Modal, Checkbox, Tag, Spin } from 'antd'
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  NodeIndexOutlined,
  ClockCircleOutlined,
  CloseOutlined,
  FlagOutlined,
  SyncOutlined,
  LinkOutlined,
  DisconnectOutlined,
} from '@ant-design/icons'
import { useUpdateProject } from '@/hooks/useProjects'
import {
  useWorkflowTemplates,
  useInheritWorkflowTemplate,
  useApplyWorkflowSync,
  useDetachWorkflowTemplate,
  useWorkflowSyncDiff,
} from '@/hooks/useWorkflows'
import { useAgents } from '@/hooks/useAgents'
import { WorkflowCanvasEditor } from '@/components/project/WorkflowCanvasEditor'
import { AgentAvatar } from '@/components/shared/AgentAvatar'
import type { Project, Workflow } from '@/types'

const { Text, Paragraph } = Typography

interface Props {
  project: Project | undefined
}

function cloneWorkflow(wf: Workflow): Workflow {
  return {
    id: wf.id,
    parent_template_id: wf.parent_template_id,
    template_version: wf.template_version,
    name: wf.name,
    steps: wf.steps.map((s) => ({ ...s })),
  }
}

export function WorkflowListPanel({ project }: Props) {
  const { message, modal } = App.useApp()
  const updateProject = useUpdateProject()
  const { data: agents } = useAgents()
  const { data: templates = [] } = useWorkflowTemplates()
  const inheritMutation = useInheritWorkflowTemplate()
  const applySyncMutation = useApplyWorkflowSync()
  const detachMutation = useDetachWorkflowTemplate()

  const [editing, setEditing] = useState<{ index: number; wf: Workflow } | null>(null)
  const [pending, setPending] = useState(false)
  const [deleting, setDeleting] = useState<number | null>(null)
  const [inheritOpen, setInheritOpen] = useState(false)
  const [syncTarget, setSyncTarget] = useState<string | null>(null)
  const [removeSteps, setRemoveSteps] = useState<string[]>([])

  const workflows = project?.workflows ?? []
  const primaryIndex = project?.primary_workflow_index ?? -1

  const { data: syncDiff, isLoading: diffLoading } = useWorkflowSyncDiff(
    project?.id,
    syncTarget ?? undefined,
  )

  // 模板当前版本 map，用于判断继承工作流是否有可同步的更新
  const templateVersions = new Map(templates.map((t) => [t.id, t.version]))

  const hasUpdate = (wf: Workflow): boolean => {
    if (!wf.parent_template_id) return false
    const cur = templateVersions.get(wf.parent_template_id)
    return cur != null && cur > (wf.template_version ?? 0)
  }

  const handleSetPrimary = async (index: number) => {
    if (!project) return
    setPending(true)
    try {
      await updateProject.mutateAsync({ id: project.id, input: { primary_workflow_index: index } })
      message.success(index >= 0 ? '已设为项目总流程' : '已取消总流程标记')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '设置失败')
    } finally {
      setPending(false)
    }
  }

  const handleInherit = async (templateId: string) => {
    if (!project) return
    setPending(true)
    try {
      await inheritMutation.mutateAsync({ projectId: project.id, templateId })
      message.success('已从模板继承，可在项目内二次修改')
      setInheritOpen(false)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '继承失败')
    } finally {
      setPending(false)
    }
  }

  const openSync = (wfId: string) => {
    setRemoveSteps([])
    setSyncTarget(wfId)
  }

  const handleApplySync = async () => {
    if (!project || !syncTarget) return
    setPending(true)
    try {
      await applySyncMutation.mutateAsync({
        projectId: project.id,
        workflowId: syncTarget,
        removeSteps,
      })
      message.success('已同步模板更新')
      setSyncTarget(null)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '同步失败')
    } finally {
      setPending(false)
    }
  }

  const handleDetach = async (wfId: string) => {
    if (!project) return
    modal.confirm({
      title: '解除继承？',
      content: '解除后该工作流成为纯私有工作流，不再接收模板更新提示。此操作不可逆。',
      okText: '解除继承',
      okButtonProps: { danger: true },
      onOk: async () => {
        try {
          await detachMutation.mutateAsync({ projectId: project.id, workflowId: wfId })
          message.success('已解除继承')
        } catch (err) {
          message.error(err instanceof Error ? err.message : '解除失败')
        }
      },
    })
  }

  const validate = (wf: Workflow): string | null => {
    if (!wf.name.trim()) return '请填写工作流名称'
    if (wf.steps.length === 0) return '请至少添加一个步骤'
    if (wf.steps.some((s) => !s.name.trim() || (!s.role?.trim() && !s.agent_id))) {
      return '每个步骤都需要填写名称并绑定数字员工'
    }
    return null
  }

  const handleSave = async () => {
    if (!project || !editing) return
    const errMsg = validate(editing.wf)
    if (errMsg) {
      message.error(errMsg)
      return
    }
    setPending(true)
    try {
      const next = [...workflows]
      if (editing.index === -1) next.push(editing.wf)
      else next[editing.index] = editing.wf
      await updateProject.mutateAsync({ id: project.id, input: { workflows: next } })
      message.success('工作流已保存')
      setEditing(null)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setPending(false)
    }
  }

  const handleDelete = async () => {
    if (!project || deleting === null) return
    setPending(true)
    try {
      const next = workflows.filter((_, i) => i !== deleting)
      await updateProject.mutateAsync({ id: project.id, input: { workflows: next } })
      message.success('工作流已删除')
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
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16, flexShrink: 0 }}>
        <div>
          <Text type="secondary" style={{ fontSize: 13 }}>
            预定义工作流，新建任务时可选择（不选则 PM 自由规划）；可从全局模板继承
          </Text>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <Button
            icon={<LinkOutlined />}
            disabled={!project}
            onClick={() => setInheritOpen(true)}
          >
            从模板继承
          </Button>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            disabled={!project}
            onClick={() => setEditing({ index: -1, wf: { name: '', steps: [] } })}
          >
            新建工作流
          </Button>
        </div>
      </div>

      <div style={{ flex: 1, overflowY: 'auto', paddingRight: 2 }}>
        {workflows.length === 0 ? (
          <Empty
            description={
              <span style={{ color: 'var(--text-tertiary)' }}>
                还没有工作流，创建一条流水线让 PM 按固定步骤规划任务
              </span>
            }
            style={{ padding: '48px 0' }}
          >
            <Button type="primary" icon={<PlusOutlined />} disabled={!project} onClick={() => setEditing({ index: -1, wf: { name: '', steps: [] } })}>
              新建工作流
            </Button>
          </Empty>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(420px, 1fr))', gap: 12 }}>
            {workflows.map((wf, idx) => (
              <div
                key={idx}
                style={{
                  borderRadius: 'var(--radius-control)',
                  border: '1px solid var(--line)',
                  background: 'var(--surface)',
                  padding: '14px 16px',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 10,
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <NodeIndexOutlined style={{ color: 'var(--signal)', fontSize: 15 }} />
                  <Text strong style={{ color: 'var(--text-primary)', fontSize: 14, flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {wf.name || '未命名工作流'}
                  </Text>
                  <span
                    style={{
                      fontSize: 11,
                      color: 'var(--text-tertiary)',
                      background: 'var(--surface-raised)',
                      borderRadius: 'var(--radius-pill)',
                      padding: '1px 8px',
                      flexShrink: 0,
                    }}
                  >
                    {wf.steps.length} 步
                  </span>
                  {primaryIndex === idx && (
                    <span
                      style={{
                        fontSize: 11,
                        color: 'var(--warning)',
                        background: 'rgba(245,158,11,0.12)',
                        borderRadius: 'var(--radius-pill)',
                        padding: '1px 8px',
                        flexShrink: 0,
                      }}
                    >
                      总流程
                    </span>
                  )}
                  {wf.parent_template_id && (
                    <span
                      style={{
                        fontSize: 11,
                        color: 'var(--signal)',
                        background: 'rgba(109,95,245,0.15)',
                        borderRadius: 'var(--radius-pill)',
                        padding: '1px 8px',
                        flexShrink: 0,
                      }}
                      title="继承自全局模板，可在项目内二次修改"
                    >
                      继承 v{wf.template_version ?? 0}
                    </span>
                  )}
                  {hasUpdate(wf) && (
                    <span
                      style={{
                        fontSize: 11,
                        color: 'var(--success)',
                        background: 'rgba(16,185,129,0.12)',
                        borderRadius: 'var(--radius-pill)',
                        padding: '1px 8px',
                        flexShrink: 0,
                      }}
                    >
                      模板可更新
                    </span>
                  )}
                  {primaryIndex === idx ? (
                    <Tooltip title="取消总流程标记">
                      <Button type="text" size="small" icon={<FlagOutlined />} style={{ color: 'var(--warning)' }} onClick={() => handleSetPrimary(-1)} />
                    </Tooltip>
                  ) : (
                    <Tooltip title="设为项目总流程（项目详情顶部展示整体进度）">
                      <Button type="text" size="small" icon={<FlagOutlined />} style={{ color: 'var(--text-quaternary)' }} onClick={() => handleSetPrimary(idx)} />
                    </Tooltip>
                  )}
                  {wf.parent_template_id && hasUpdate(wf) && (
                    <Tooltip title="同步模板更新">
                      <Button type="text" size="small" icon={<SyncOutlined />} style={{ color: 'var(--success)' }} onClick={() => openSync(wf.id!)} />
                    </Tooltip>
                  )}
                  {wf.parent_template_id && (
                    <Tooltip title="解除继承">
                      <Button type="text" size="small" icon={<DisconnectOutlined />} style={{ color: 'var(--text-quaternary)' }} onClick={() => handleDetach(wf.id!)} />
                    </Tooltip>
                  )}
                  <Tooltip title="编辑">
                    <Button type="text" size="small" icon={<EditOutlined />} style={{ color: 'var(--text-tertiary)' }} onClick={() => setEditing({ index: idx, wf: cloneWorkflow(wf) })} />
                  </Tooltip>
                  <Tooltip title="删除">
                    <Button type="text" size="small" icon={<DeleteOutlined />} style={{ color: 'var(--text-quaternary)' }} onClick={() => setDeleting(idx)} />
                  </Tooltip>
                </div>

                {/* 步骤链预览 */}
                <div style={{ display: 'flex', alignItems: 'center', gap: 4, flexWrap: 'wrap', paddingLeft: 23 }}>
                  {wf.steps.map((s, si) => {
                    const agent = findAgent(s.agent_id)
                    return (
                      <span key={si} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                        <span
                          style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: 6,
                            borderRadius: 'var(--radius-control)',
                            border: '1px solid var(--line)',
                            background: 'var(--surface)',
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
                            <span
                              style={{
                                width: 18,
                                height: 18,
                                borderRadius: 'var(--radius-avatar)',
                                border: '1px dashed var(--line-strong)',
                                display: 'inline-flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                fontSize: 10,
                                color: 'var(--text-tertiary)',
                                flexShrink: 0,
                              }}
                            >
                              {si + 1}
                            </span>
                          )}
                          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                            {s.name || '未命名'}
                          </span>
                          {s.need_review && (
                            <ClockCircleOutlined style={{ color: 'var(--warning)', fontSize: 11, flexShrink: 0 }} />
                          )}
                        </span>
                        {si < wf.steps.length - 1 && (
                          <span style={{ color: 'var(--text-quaternary)', fontSize: 11 }}>→</span>
                        )}
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
        title={editing?.index === -1 ? '新建工作流' : '编辑工作流'}
        open={!!editing}
        onClose={() => !pending && setEditing(null)}
        width={560}
        styles={{ body: { paddingTop: 16 } }}
        extra={
          <Button type="primary" loading={pending} disabled={!editing?.wf.name.trim()} onClick={handleSave}>
            保存工作流
          </Button>
        }
      >
        {editing && (
          <WorkflowCanvasEditor
            workflow={editing.wf}
            onChange={(wf) => setEditing({ ...editing, wf })}
            agents={agents ?? []}
          />
        )}
        <div style={{ marginTop: 24, paddingTop: 16, borderTop: '1px solid var(--line)', display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <Button icon={<CloseOutlined />} disabled={pending} onClick={() => setEditing(null)}>
            取消
          </Button>
        </div>
      </Drawer>

      {/* 删除确认 */}
      <Drawer
        title="删除工作流？"
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
          已创建的任务不受影响（任务保存的是工作流快照）。此操作不可撤销。
        </Paragraph>
      </Drawer>

      {/* 从模板继承 */}
      <Modal
        title="从全局模板继承"
        open={inheritOpen}
        onCancel={() => !pending && setInheritOpen(false)}
        footer={null}
        width={520}
      >
        <Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 12 }}>
          选择后会在项目内生成一份克隆副本，可自由二次修改；模板后续更新可选择同步
        </Text>
        <div style={{ maxHeight: 420, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 8 }}>
          {templates.length === 0 ? (
            <Empty description={<span style={{ color: 'var(--text-tertiary)' }}>还没有全局模板，请先到"工作流"页创建</span>} />
          ) : (
            templates.map((tpl) => (
              <div
                key={tpl.id}
                onClick={() => handleInherit(tpl.id)}
                style={{
                  borderRadius: 'var(--radius-control)',
                  border: '1px solid var(--line-strong)',
                  background: 'var(--surface)',
                  padding: '10px 14px',
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 10,
                  transition: 'border-color 0.15s',
                }}
                onMouseEnter={(e) => (e.currentTarget.style.borderColor = 'var(--signal)')}
                onMouseLeave={(e) => (e.currentTarget.style.borderColor = 'rgba(255,255,255,0.1)')}
              >
                <NodeIndexOutlined style={{ color: 'var(--signal)', fontSize: 14 }} />
                <Text strong style={{ color: 'var(--text-primary)', fontSize: 13, flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {tpl.name}
                </Text>
                <Tag color="purple" style={{ marginInlineEnd: 0 }}>v{tpl.version}</Tag>
                <span style={{ fontSize: 11, color: 'var(--text-tertiary)', background: 'var(--surface-raised)', borderRadius: 'var(--radius-pill)', padding: '1px 8px', flexShrink: 0 }}>
                  {tpl.steps.length} 步
                </span>
              </div>
            ))
          )}
        </div>
      </Modal>

      {/* 同步差异确认 */}
      <Modal
        title={syncDiff ? `同步模板更新 · ${syncDiff.template_name}（v${syncDiff.project_version} → v${syncDiff.current_version}）` : '同步模板更新'}
        open={syncTarget !== null}
        onCancel={() => !pending && setSyncTarget(null)}
        width={560}
        footer={
          <>
            <Button disabled={pending} onClick={() => setSyncTarget(null)}>
              取消
            </Button>
            <Button
              type="primary"
              icon={<SyncOutlined />}
              loading={pending}
              disabled={!syncDiff || syncDiff.changes.length === 0}
              onClick={handleApplySync}
            >
              确认同步
            </Button>
          </>
        }
      >
        {diffLoading || !syncDiff ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '32px 0' }}>
            <Spin />
          </div>
        ) : syncDiff.changes.length === 0 ? (
          <Paragraph type="secondary" style={{ marginBottom: 0 }}>
            模板当前版本与项目一致，没有可同步的变更。
          </Paragraph>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {syncDiff.changes.map((ch, i) => {
              const kindMeta = {
                add: { color: 'var(--success)', label: '新增' },
                update: { color: 'var(--info)', label: '更新' },
                keep: { color: 'var(--warning)', label: '保留（项目已修改）' },
                keep_project: { color: 'var(--text-tertiary)', label: '保留（项目自定义）' },
                remove_pending: { color: 'var(--error)', label: '待确认删除' },
              } as const
              const meta = kindMeta[ch.kind]
              const isRemove = ch.kind === 'remove_pending'
              return (
                <div
                  key={`${ch.name}-${i}`}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 10,
                    borderRadius: 'var(--radius-control)',
                    border: '1px solid var(--line)',
                    background: 'var(--surface)',
                    padding: '8px 12px',
                  }}
                >
                  {isRemove ? (
                    <Checkbox
                      checked={removeSteps.includes(ch.name)}
                      onChange={(e) =>
                        setRemoveSteps((prev) =>
                          e.target.checked ? [...prev, ch.name] : prev.filter((n) => n !== ch.name),
                        )
                      }
                    >
                      <Text style={{ color: 'var(--text-primary)', fontSize: 13 }}>{ch.name}</Text>
                    </Checkbox>
                  ) : (
                    <Text style={{ color: 'var(--text-primary)', fontSize: 13, flex: 1 }}>{ch.name}</Text>
                  )}
                  <Tag color={meta.color} style={{ marginInlineEnd: 0 }}>
                    {meta.label}
                  </Tag>
                </div>
              )
            })}
            <Paragraph type="secondary" style={{ marginBottom: 0, marginTop: 8, fontSize: 12 }}>
              未打勾的"待确认删除"步骤将保留在项目中；项目已修改的步骤不会被模板覆盖。
            </Paragraph>
          </div>
        )}
      </Modal>
    </div>
  )
}
