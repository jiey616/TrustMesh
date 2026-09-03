import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Button, Typography, Space, Tag, Spin, App, Modal, Form, Input, Select, Dropdown, Empty } from 'antd'
import { ArrowLeftOutlined, PlusOutlined, FileTextOutlined, VideoCameraOutlined, CheckSquareOutlined, NodeIndexOutlined, MessageOutlined, MoreOutlined, EditOutlined, DeleteOutlined, ApiOutlined } from '@ant-design/icons'
import { useProject, useUpdateProject, useArchiveProject } from '@/hooks/useProjects'
import { useTasks } from '@/hooks/useTasks'
import { TaskListView } from '@/components/task/TaskListView'
import { TaskWorkspace } from '@/components/task/TaskWorkspace'
import { FileExplorer } from '@/components/project/FileExplorer'
import { ActionItemsPanel } from '@/components/project/ActionItemsPanel'
import { WorkflowListPanel } from '@/components/project/WorkflowListPanel'
import { WorkflowProgressPanel } from '@/components/project/WorkflowProgressPanel'
import { ProjectTabRail, type ProjectTabItem } from '@/components/project/ProjectTabRail'
import { ExternalAppFrame } from '@/components/external/ExternalAppFrame'
import { AgentAvatar } from '@/components/shared/AgentAvatar'
import { MeetingListPage } from '@/pages/MeetingListPage'
import { useAgents } from '@/hooks/useAgents'
import { useExternalApps } from '@/hooks/useExternalApps'
import { hasPlacement, type TaskListItem } from '@/types'

const { Title, Text } = Typography

type BuiltinTab = 'tasks' | 'files' | 'meetings' | 'todos' | 'workflows'
/** 内置 tab，或外部平台 tab（app:<external_app_id>） */
type ProjectTab = BuiltinTab | `app:${string}`
const APP_TAB_PREFIX = 'app:'

type WorkspaceState =
  | { kind: 'task'; taskId: string }
  | { kind: 'draft'; projectId: string }
  | null

interface TaskSelectionState {
  observedTasks: TaskListItem[] | undefined
  prevStatusMap: Record<string, string>
  autoSelectedTaskId: string | null
}

const statusMap: Record<string, { color: string; label: string }> = {
  active: { color: 'green', label: '开放中' },
  archived: { color: 'default', label: '已归档' },
}

const workStatusMap: Record<string, { color: string; label: string }> = {
  empty: { color: 'default', label: '空' },
  idle: { color: 'blue', label: '空闲' },
  queued: { color: 'gold', label: '排队中' },
  running: { color: 'cyan', label: '执行中' },
  attention: { color: 'red', label: '需关注' },
  archived: { color: 'default', label: '已归档' },
}

const pmStatusColor: Record<string, string> = {
  online: 'var(--success)',
  busy: 'var(--warning)',
  offline: 'var(--text-quaternary)',
}

export function ProjectBoardPage() {
  const navigate = useNavigate()
  const { id: projectId } = useParams<{ id: string }>()
  // 支持 ?task=<id> 直达某个任务（AI 办公室侧栏点击任务跳转过来）
  const [searchParams, setSearchParams] = useSearchParams()
  const urlTaskId = searchParams.get('task')
  const { data: project, isLoading: projectLoading } = useProject(projectId)
  const { data: tasks } = useTasks(projectId)
  const updateProject = useUpdateProject()
  const archiveProject = useArchiveProject()
  const { data: agents } = useAgents()
  const { data: externalApps, isLoading: externalAppsLoading } = useExternalApps()
  const [editForm] = Form.useForm()
  // tab 状态同步到 URL，刷新/分享后可回到同一个外部平台 tab
  const [activeTab, setActiveTab] = useState<ProjectTab>(
    (searchParams.get('tab') as ProjectTab) || 'tasks',
  )
  const [workspace, setWorkspace] = useState<WorkspaceState>(
    urlTaskId ? { kind: 'task', taskId: urlTaskId } : null,
  )
  const [editOpen, setEditOpen] = useState(false)
  const [archiveOpen, setArchiveOpen] = useState(false)
  const { message } = App.useApp()

  // URL 参数只用于初始化选中任务，用完即清（刷新后回到默认视图）
  const consumedUrlTask = useRef(false)
  useEffect(() => {
    if (urlTaskId && !consumedUrlTask.current) {
      consumedUrlTask.current = true
      // 只清掉 task 参数，保留 tab（可能被外部平台 tab 占用）
      const next = new URLSearchParams(searchParams)
      next.delete('task')
      setSearchParams(next, { replace: true })
    }
  }, [urlTaskId, setSearchParams, searchParams])

  // 声明了 project_tab 挂载点的启用中外部平台，追加在内置 tab 之后。
  // 必须在下面所有提前 return 之前声明：Hooks 不能放在条件分支之后，
  // 否则 loading → loaded 的渲染切换会触发 "Rendered more hooks than
  // during the previous render"。
  const projectTabApps = useMemo(
    () =>
      (externalApps ?? []).filter(
        (a) => a.status === 'enabled' && hasPlacement(a.placement, 'project_tab'),
      ),
    [externalApps],
  )

  const [taskSelectionState, setTaskSelectionState] = useState<TaskSelectionState>({
    observedTasks: undefined,
    prevStatusMap: {},
    autoSelectedTaskId: null,
  })

  // Auto-select a task that has just transitioned into `in_progress`
  if (tasks !== taskSelectionState.observedTasks) {
    const currentMap: Record<string, string> = {}
    let nextAutoSelectedTaskId = taskSelectionState.autoSelectedTaskId

    for (const task of tasks ?? []) {
      const prev = taskSelectionState.prevStatusMap[task.id]
      if (task.status === 'in_progress' && prev !== undefined && prev !== 'in_progress') {
        nextAutoSelectedTaskId = task.id
      }
      currentMap[task.id] = task.status
    }

    setTaskSelectionState({
      observedTasks: tasks,
      prevStatusMap: currentMap,
      autoSelectedTaskId: nextAutoSelectedTaskId,
    })
  }

  const activeSelectedTaskId =
    taskSelectionState.autoSelectedTaskId ?? (workspace?.kind === 'task' ? workspace.taskId : null)

  const projectArchived = project?.status === 'archived'
  const pmAgents = agents?.filter((a) => a.role === 'pm') || []

  const selectTask = (taskId: string | null) => {
    setTaskSelectionState((prev) =>
      prev.autoSelectedTaskId ? { ...prev, autoSelectedTaskId: null } : prev,
    )
    setWorkspace(taskId ? { kind: 'task', taskId } : null)
  }

  const openDraftWorkspace = () => {
    if (!projectId) return
    setTaskSelectionState((prev) =>
      prev.autoSelectedTaskId ? { ...prev, autoSelectedTaskId: null } : prev,
    )
    setWorkspace({ kind: 'draft', projectId })
  }

  const handleEdit = async () => {
    const values = await editForm.validateFields()
    try {
      await updateProject.mutateAsync({ id: projectId!, input: values })
      message.success('项目已更新')
      setEditOpen(false)
    } catch {
      message.error('更新失败')
    }
  }

  const handleArchive = async () => {
    try {
      await archiveProject.mutateAsync(projectId!)
      message.success('项目已归档')
      setArchiveOpen(false)
      navigate('/projects', { replace: true })
    } catch {
      message.error('归档失败')
    }
  }

  if (projectLoading) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
        <Spin size="large" />
      </div>
    )
  }

  if (!project) {
    return <div style={{ padding: 24, color: 'var(--text-tertiary)' }}>项目不存在</div>
  }

  const ts = project.task_summary
  const total = ts.task_total ?? 0
  const pending = ts.pending_count ?? 0
  const inProgress = ts.in_progress_count ?? 0
  const failed = ts.failed_count ?? 0
  const pm = project.pm_agent

  // 右侧竖向 tab 条用的定义（图标与文字分开，折叠态只渲染图标）
  const tabDefs: ProjectTabItem[] = [
    { key: 'tasks', label: '任务', icon: <MessageOutlined /> },
    { key: 'files', label: '文件', icon: <FileTextOutlined /> },
    { key: 'meetings', label: '会议室', icon: <VideoCameraOutlined /> },
    { key: 'todos', label: '待办', icon: <CheckSquareOutlined /> },
    { key: 'workflows', label: '工作流', icon: <NodeIndexOutlined /> },
    ...projectTabApps.map((a) => ({
      key: `${APP_TAB_PREFIX}${a.id}`,
      label: a.name,
      icon: a.icon_url ? (
        <img src={a.icon_url} alt="" style={{ width: 14, height: 14, objectFit: 'contain' }} />
      ) : (
        <ApiOutlined />
      ),
    })),
  ]

  const handleTabChange = (k: string) => {
    const tab = k as ProjectTab
    setActiveTab(tab)
    const next = new URLSearchParams(searchParams)
    if (tab === 'tasks') next.delete('tab')
    else next.set('tab', tab)
    setSearchParams(next, { replace: true })
  }

  const activeAppId = activeTab.startsWith(APP_TAB_PREFIX)
    ? activeTab.slice(APP_TAB_PREFIX.length)
    : null
  const activeApp = activeAppId
    ? projectTabApps.find((a) => a.id === activeAppId)
    : undefined

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <div style={{ padding: '8px 16px', borderBottom: '1px solid var(--line)', background: 'var(--surface)', backdropFilter: 'var(--glass-blur)', flexShrink: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, flex: 1, minWidth: 0 }}>
            <Button type="text" icon={<ArrowLeftOutlined />} onClick={() => navigate('/projects')} style={{ flexShrink: 0 }} />
            <div style={{ minWidth: 0, flex: 1 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
                <Title
                  level={5}
                  style={{ margin: 0, color: 'var(--text-primary)', fontSize: 16 }}
                  ellipsis={{ tooltip: project.name }}
                >
                  {project.name}
                </Title>
                <Tag style={{ margin: 0, fontSize: 10, lineHeight: '16px', height: 18 }} color={statusMap[project.status]?.color}>
                  {statusMap[project.status]?.label}
                </Tag>
                <Tag style={{ margin: 0, fontSize: 10, lineHeight: '16px', height: 18 }} color={workStatusMap[ts.work_status]?.color}>
                  {workStatusMap[ts.work_status]?.label}
                </Tag>
                <span style={{ flex: 1 }} />
                <Space size={12} style={{ flexShrink: 0, fontSize: 12 }}>
                  <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 12, color: 'var(--text-tertiary)' }}>
                    <AgentAvatar name={pm.name} role="pm" seed={pm.node_id} size={18} />
                    <span
                      style={{ width: 6, height: 6, borderRadius: 'var(--radius-avatar)', background: pmStatusColor[pm.status] || '#6b7280', display: 'inline-block' }}
                      title={pm.status}
                    />
                    {pm.name}
                  </span>
                  <span style={{ color: 'var(--text-tertiary)' }}>
                    任务: {total}
                    {inProgress > 0 && ` · ${inProgress} 执行中`}
                    {failed > 0 && ` · ${failed} 失败`}
                    {pending > 0 && ` · ${pending} 待处理`}
                  </span>
                </Space>
              </div>
              {project.description && (
                <Text type="secondary" ellipsis style={{ marginTop: 2, display: 'block', maxWidth: 640, fontSize: 12 }}>
                  {project.description}
                </Text>
              )}
            </div>
          </div>
          <Space style={{ flexShrink: 0 }}>
            <Button size="small" icon={<PlusOutlined />} disabled={projectArchived} onClick={openDraftWorkspace}>
              {projectArchived ? '项目已归档' : '新任务'}
            </Button>
            <Dropdown
              menu={{
                items: [
                  { key: 'edit', icon: <EditOutlined />, label: '编辑项目', disabled: !project },
                  { type: 'divider' },
                  {
                    key: 'archive',
                    icon: <DeleteOutlined />,
                    label: projectArchived ? '已归档' : '归档项目',
                    danger: true,
                    disabled: !project || projectArchived,
                  },
                ],
                onClick: ({ key }) => {
                  if (key === 'edit') {
                    editForm.setFieldsValue({
                      name: project?.name,
                      description: project?.description,
                      pm_agent_id: project?.pm_agent.id,
                    })
                    setEditOpen(true)
                  } else if (key === 'archive') {
                    setArchiveOpen(true)
                  }
                },
              }}
            >
              <Button size="small" icon={<MoreOutlined />} />
            </Dropdown>
          </Space>
        </div>

        {/* 项目进度（Tabs 上方，不占内容区空间） */}
        <div style={{ display: 'flex', alignItems: 'center', paddingBottom: 2, width: '100%', minWidth: 0 }}>
          <WorkflowProgressPanel projectId={projectId!} />
        </div>

      </div>

      {/* 右侧预留 72px 给折叠态 tab 条，内容不被遮挡；
          展开态是浮层，向左覆盖在内容之上，不会挤压布局 */}
      <div style={{ flex: 1, overflow: 'hidden', padding: '16px 72px 0 16px', position: 'relative' }}>
        <ProjectTabRail tabs={tabDefs} activeKey={activeTab} onChange={handleTabChange} />

        {activeTab === 'tasks' && (
          <div style={{ display: 'flex', height: '100%', gap: 16 }}>
            {/* 主区域:默认新建任务对话框(类 Codex),点击任务后切换为任务流程界面 */}
            <div style={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
              {taskSelectionState.autoSelectedTaskId ? (
                <TaskWorkspace
                  key={taskSelectionState.autoSelectedTaskId}
                  taskId={taskSelectionState.autoSelectedTaskId}
                  onClose={() => selectTask(null)}
                />
              ) : workspace?.kind === 'task' ? (
                <TaskWorkspace
                  key={workspace.taskId}
                  taskId={workspace.taskId}
                  onClose={() => selectTask(null)}
                />
              ) : (
                <TaskWorkspace
                  key={workspace?.kind === 'draft' ? 'draft-new' : 'draft-default'}
                  projectId={projectId!}
                  onClose={() => setWorkspace(null)}
                  onTaskCreated={(taskId) => setWorkspace({ kind: 'task', taskId })}
                  onOpenTask={(taskId) => selectTask(taskId)}
                  closable={workspace?.kind === 'draft'}
                />
              )}
            </div>

            {/* 右侧窄条:任务列表 */}
            <div
              style={{
                width: 320,
                flexShrink: 0,
                borderLeft: '1px solid var(--line)',
                background: 'var(--surface-sunken)',
                borderRadius: '12px 12px 0 0',
                padding: 10,
                display: 'flex',
                flexDirection: 'column',
                minHeight: 0,
              }}
            >
              <TaskListView
                tasks={tasks ?? []}
                selectedTaskId={activeSelectedTaskId}
                onTaskClick={(id) => selectTask(id === activeSelectedTaskId ? null : id)}
              />
            </div>
          </div>
        )}
        {activeTab === 'files' && <FileExplorer projectId={projectId!} />}
        {activeTab === 'meetings' && <MeetingListPage projectId={projectId!} />}
        {activeTab === 'todos' && <ActionItemsPanel projectId={projectId!} />}
        {activeTab === 'workflows' && <WorkflowListPanel project={project} />}
        {activeApp && <ExternalAppFrame app={activeApp} projectId={projectId} />}
        {activeAppId && !activeApp && !externalAppsLoading && (
          <Empty description="该外部平台不可用，可能已被断开或取消了项目页挂载" />
        )}
        {activeAppId && !activeApp && externalAppsLoading && (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '64px 0' }}>
            <Spin />
          </div>
        )}
      </div>

      {/* Edit project dialog */}
      <Modal
        title="编辑项目"
        open={editOpen}
        onCancel={() => setEditOpen(false)}
        onOk={handleEdit}
        confirmLoading={updateProject.isPending}
      >
        <Form form={editForm} layout="vertical">
          <Form.Item name="name" label="项目名称" rules={[{ required: true }]}>
            <Input placeholder="输入项目名称" />
          </Form.Item>
          <Form.Item name="description" label="项目描述">
            <Input.TextArea rows={3} placeholder="输入项目描述" />
          </Form.Item>
          <Form.Item name="pm_agent_id" label="PM 数字员工">
            <Select placeholder="选择 PM 数字员工" options={pmAgents.map((a) => ({ label: a.name, value: a.id }))} />
          </Form.Item>
        </Form>
      </Modal>

      {/* Archive project dialog */}
      <Modal
        title="归档项目"
        open={archiveOpen}
        onCancel={() => setArchiveOpen(false)}
        onOk={handleArchive}
        okButtonProps={{ danger: true }}
        confirmLoading={archiveProject.isPending}
      >
        <Text type="secondary">
          确定要归档项目「{project.name}」吗？归档后项目将不再接收新任务。
        </Text>
      </Modal>
    </div>
  )
}
