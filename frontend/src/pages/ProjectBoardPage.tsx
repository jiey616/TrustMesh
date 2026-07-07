import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { MessageSquarePlus, MoreHorizontal, Pencil, Archive, Loader2, FolderOpen } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger, DropdownMenuSeparator } from '@/components/ui/dropdown-menu'
import { AgentStatusDot, ProjectStatusBadge, ProjectWorkStatusBadge } from '@/components/shared/StatusBadge'
import { Avatar } from '@/components/ui/avatar'
import { TaskListView } from '@/components/task/TaskListView'
import { TaskWorkspace } from '@/components/task/TaskWorkspace'
import { EditProjectDialog } from '@/components/project/EditProjectDialog'
import { ArchiveProjectDialog } from '@/components/project/ArchiveProjectDialog'
import { FileExplorer } from '@/components/project/FileExplorer'
import { useProject } from '@/hooks/useProjects'
import { useTasks } from '@/hooks/useTasks'
import { formatDateTime, formatRelativeTime } from '@/lib/utils'
import { useAssistantStore } from '@/stores/assistantStore'
import type { TaskListItem } from '@/types'

interface TaskSelectionState {
  observedTasks: TaskListItem[] | undefined
  prevStatusMap: Record<string, string>
  autoSelectedTaskId: string | null
}

type WorkspaceState =
  | { kind: 'task'; taskId: string }
  | { kind: 'draft'; projectId: string }
  | null

type ProjectTab = 'tasks' | 'files'

export function ProjectBoardPage() {
  const navigate = useNavigate()
  const { projectId } = useParams<{ projectId: string }>()
  const { data: project } = useProject(projectId)
  const { data: tasks, isLoading } = useTasks(projectId)
  const [workspace, setWorkspace] = useState<WorkspaceState>(null)
  const [editOpen, setEditOpen] = useState(false)
  const [archiveOpen, setArchiveOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<ProjectTab>('tasks')
  const setFabVisibility = useAssistantStore((state) => state.setFabVisibility)
  const [taskSelectionState, setTaskSelectionState] = useState<TaskSelectionState>({
    observedTasks: undefined,
    prevStatusMap: {},
    autoSelectedTaskId: null,
  })
  const projectArchived = project?.status === 'archived'

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

  const activeSelectedTaskId = taskSelectionState.autoSelectedTaskId ?? (workspace?.kind === 'task' ? workspace.taskId : null)
  const hasWorkspace = !!workspace || !!taskSelectionState.autoSelectedTaskId

  useEffect(() => {
    setFabVisibility(hasWorkspace ? 'hidden' : 'visible')
    return () => setFabVisibility('visible')
  }, [hasWorkspace, setFabVisibility])

  const selectTask = (taskId: string | null) => {
    setTaskSelectionState((prev) => (
      prev.autoSelectedTaskId
        ? { ...prev, autoSelectedTaskId: null }
        : prev
    ))
    setWorkspace(taskId ? { kind: 'task', taskId } : null)
  }

  const openDraftWorkspace = () => {
    if (!projectId) {
      return
    }
    setTaskSelectionState((prev) => (
      prev.autoSelectedTaskId
        ? { ...prev, autoSelectedTaskId: null }
        : prev
    ))
    setWorkspace({ kind: 'draft', projectId })
  }

  return (
    <div className="flex h-full flex-col">
      {/* Project Header */}
      <div className="flex items-start justify-between gap-4 border-b px-6 py-4">
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-3">
            <h1 className="truncate text-lg font-semibold">{project?.name ?? '...'}</h1>
            {project && (
              <>
                <ProjectStatusBadge status={project.status} />
                <ProjectWorkStatusBadge status={project.task_summary.work_status} />
              </>
            )}
          </div>
          {project?.description && (
            <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">
              {project.description}
            </p>
          )}
          {project && (
            <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
              <div className="flex items-center gap-1.5">
                <Avatar
                  fallback={project.pm_agent.name}
                  seed={project.pm_agent.id}
                  kind="agent"
                  role="pm"
                  size="sm"
                />
                <AgentStatusDot status={project.pm_agent.status} />
                <span>PM: {project.pm_agent.name}</span>
              </div>
              <span>
                任务: {project.task_summary.task_total}
                {project.task_summary.in_progress_count > 0 && ` · ${project.task_summary.in_progress_count} 执行中`}
                {project.task_summary.pending_count > 0 && ` · ${project.task_summary.pending_count} 待处理`}
                {project.task_summary.failed_count > 0 && ` · ${project.task_summary.failed_count} 失败`}
                {project.task_summary.canceled_count > 0 && ` · ${project.task_summary.canceled_count} 已取消`}
              </span>
              <span title={formatDateTime(project.updated_at)}>
                最后更新: {formatRelativeTime(project.updated_at)}
              </span>
              {project.task_summary.latest_task_at && (
                <span title={formatDateTime(project.task_summary.latest_task_at)}>
                  最近任务: {formatRelativeTime(project.task_summary.latest_task_at)}
                </span>
              )}
              <span title={formatDateTime(project.created_at)}>
                创建于: {formatDateTime(project.created_at)}
              </span>
            </div>
          )}
          </div>
        <div className="flex items-center gap-2">
          <Button size="sm" disabled={projectArchived} onClick={openDraftWorkspace}>
            <MessageSquarePlus className="size-4 mr-1.5" />
            {projectArchived ? '项目已归档' : '新任务'}
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger className="p-2 rounded-md hover:bg-muted">
              <MoreHorizontal className="size-4" />
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <DropdownMenuItem disabled={!project} onClick={() => setEditOpen(true)}>
                <Pencil className="size-3.5 mr-2" />
                编辑项目
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="text-destructive"
                disabled={!project || projectArchived}
                onClick={() => setArchiveOpen(true)}
              >
                <Archive className="size-3.5 mr-2" />
                归档项目
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex items-center gap-0 border-b px-6">
        <button
          onClick={() => setActiveTab('tasks')}
          className={`flex items-center gap-1.5 px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${
            activeTab === 'tasks'
              ? 'border-primary text-primary'
              : 'border-transparent text-muted-foreground hover:text-foreground'
          }`}
        >
          <MessageSquarePlus className="size-4" />
          任务
        </button>
        <button
          onClick={() => setActiveTab('files')}
          className={`flex items-center gap-1.5 px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${
            activeTab === 'files'
              ? 'border-primary text-primary'
              : 'border-transparent text-muted-foreground hover:text-foreground'
          }`}
        >
          <FolderOpen className="size-4" />
          文件
        </button>
      </div>

      {/* Content */}
      {activeTab === 'files' ? (
        <div className="flex-1 min-h-0">
          <FileExplorer projectId={projectId!} />
        </div>
      ) : isLoading ? (
        <div className="flex flex-1 items-center justify-center">
          <Loader2 className="size-8 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <div className="flex flex-1 min-h-0">
          {/* Left: Task List */}
          <div className={`px-6 h-full ${hasWorkspace ? 'w-1/2 border-r' : 'w-full'}`}>
            <TaskListView
              tasks={tasks ?? []}
              selectedTaskId={activeSelectedTaskId}
              onTaskClick={(id) => selectTask(id === activeSelectedTaskId ? null : id)}
            />
          </div>

          {/* Right: Task Workspace */}
          {hasWorkspace && (
            <div className="w-1/2 h-full">
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
              ) : workspace?.kind === 'draft' ? (
                <TaskWorkspace
                  key="draft-planning"
                  projectId={workspace.projectId}
                  onClose={() => setWorkspace(null)}
                  onTaskCreated={(taskId) => setWorkspace({ kind: 'task', taskId })}
                />
              ) : null
              }
            </div>
          )}
        </div>
      )}

      <EditProjectDialog open={editOpen} onOpenChange={setEditOpen} project={project} />
      <ArchiveProjectDialog
        open={archiveOpen}
        onOpenChange={setArchiveOpen}
        project={project}
        onArchived={() => navigate('/projects', { replace: true })}
      />
    </div>
  )
}
