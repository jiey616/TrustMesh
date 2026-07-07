import { useState } from 'react'
import { Link } from 'react-router-dom'
import { FolderKanban, Plus } from 'lucide-react'
import { PageContainer } from '@/components/layout/PageContainer'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { AgentStatusDot, ProjectWorkStatusBadge } from '@/components/shared/StatusBadge'
import { EmptyState } from '@/components/shared/EmptyState'
import { Avatar } from '@/components/ui/avatar'
import { CreateProjectDialog } from '@/components/project/CreateProjectDialog'
import { useProjects } from '@/hooks/useProjects'
import { formatRelativeTime } from '@/lib/utils'

export function ProjectListPage() {
  const { data: projects, isLoading } = useProjects()
  const [showCreate, setShowCreate] = useState(false)
  const activeProjects = (projects ?? []).filter((p) => p.status === 'active')
  const archivedProjects = (projects ?? []).filter((p) => p.status === 'archived')

  return (
    <PageContainer>
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl font-bold">项目</h1>
          <p className="text-muted-foreground mt-1">管理你的 AI Agent 协作项目</p>
        </div>
        <Button onClick={() => setShowCreate(true)}>
          <Plus className="size-4 mr-2" />
          新建项目
        </Button>
      </div>

      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {[1, 2, 3].map((i) => (
            <Card key={i}>
              <CardHeader>
                <Skeleton className="h-5 w-32" />
                <Skeleton className="h-4 w-48 mt-2" />
              </CardHeader>
              <CardContent>
                <Skeleton className="h-4 w-24" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : projects?.length === 0 ? (
        <EmptyState
          icon={FolderKanban}
          title="还没有项目"
          description="创建你的第一个项目，开始用 AI Agent 协作完成任务"
          action={
            <Button onClick={() => setShowCreate(true)}>
              <Plus className="size-4 mr-2" />
              创建项目
            </Button>
          }
        />
      ) : (
        <>
          {activeProjects.length === 0 && archivedProjects.length > 0 && (
            <div className="mb-4 p-3 border rounded-md bg-muted/30 text-sm text-muted-foreground">
              当前没有活跃项目，以下为已归档项目。
            </div>
          )}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            {[...activeProjects, ...archivedProjects].map((project) => (
            <Link key={project.id} to={`/projects/${project.id}`}>
              <Card className={`transition-all hover:shadow-md hover:border-primary/30 cursor-pointer group ${project.status === 'archived' ? 'opacity-60 hover:opacity-80' : ''}`}>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2 text-base group-hover:text-primary transition-colors">
                    <FolderKanban className="size-4 shrink-0" />
                    {project.name}
                    {project.status === 'archived' && (
                      <span className="text-xs px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-normal">
                        已归档
                      </span>
                    )}
                  </CardTitle>
                  <div>
                    <ProjectWorkStatusBadge status={project.task_summary.work_status} />
                  </div>
                  <CardDescription className="line-clamp-2">
                    {project.description}
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="space-y-2 text-xs text-muted-foreground">
                    <div className="flex items-center justify-between">
                      <span>
                        {project.task_summary.task_total} 个任务
                        {project.task_summary.in_progress_count > 0 && ` · ${project.task_summary.in_progress_count} 执行中`}
                        {project.task_summary.failed_count > 0 && ` · ${project.task_summary.failed_count} 失败`}
                        {project.task_summary.canceled_count > 0 && ` · ${project.task_summary.canceled_count} 已取消`}
                      </span>
                      <span>{formatRelativeTime(project.updated_at)}</span>
                    </div>
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-1.5">
                        <Avatar
                          fallback={project.pm_agent.name}
                          seed={project.pm_agent.id}
                          kind="agent"
                          role="pm"
                          size="sm"
                        />
                        <span>{project.pm_agent.name}</span>
                        <AgentStatusDot status={project.pm_agent.status} />
                      </div>
                      {project.task_summary.latest_task_at && (
                        <span>任务更新 {formatRelativeTime(project.task_summary.latest_task_at)}</span>
                      )}
                    </div>
                  </div>
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
        </>
      )}

      <CreateProjectDialog open={showCreate} onOpenChange={setShowCreate} />
    </PageContainer>
  )
}
