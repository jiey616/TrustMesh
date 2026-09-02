import { useState } from 'react'
import { Boxes, ExternalLink, Pencil, Plus, Trash2 } from 'lucide-react'
import { PageContainer } from '@/components/layout/PageContainer'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { EmptyState } from '@/components/shared/EmptyState'
import { ExternalAppFormDialog } from '@/components/external/ExternalAppFormDialog'
import { useDeleteExternalApp, useExternalApps, useLaunchExternalApp } from '@/hooks/useExternalApps'
import { ApiRequestError } from '@/api/client'
import { toast } from 'sonner'
import type { ExternalAppView } from '@/types'

export function ExternalAppsPage() {
  const { data: apps, isLoading } = useExternalApps()
  const deleteApp = useDeleteExternalApp()
  const launchApp = useLaunchExternalApp()
  const [showForm, setShowForm] = useState(false)
  const [editing, setEditing] = useState<ExternalAppView | null>(null)

  const handleLaunch = async (app: ExternalAppView) => {
    try {
      const res = await launchApp.mutateAsync({ id: app.id })
      window.open(res.launch_url, '_blank', 'noopener,noreferrer')
    } catch (err) {
      toast.error(err instanceof ApiRequestError ? err.message : '启动失败')
    }
  }

  const handleDelete = async (app: ExternalAppView) => {
    if (!window.confirm(`确定断开「${app.name}」？断开后将无法再免密打开该平台。`)) return
    try {
      await deleteApp.mutateAsync(app.id)
      toast.success('已断开外部平台')
    } catch (err) {
      toast.error(err instanceof ApiRequestError ? err.message : '删除失败')
    }
  }

  const openCreate = () => {
    setEditing(null)
    setShowForm(true)
  }
  const openEdit = (app: ExternalAppView) => {
    setEditing(app)
    setShowForm(true)
  }

  return (
    <PageContainer>
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl font-bold">外部应用</h1>
          <p className="text-muted-foreground mt-1">
            通过 SSO 连接外部平台，在 TrustMesh 内免密打开。统一在此登记与管理。
          </p>
        </div>
        <Button onClick={openCreate}>
          <Plus className="size-4 mr-2" />
          新增外部平台
        </Button>
      </div>

      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
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
      ) : apps && apps.length > 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {apps.map((app) => (
            <Card key={app.id} className="flex flex-col">
              <CardHeader>
                <div className="flex items-start justify-between gap-2">
                  <CardTitle className="flex items-center gap-2 text-base">
                    <Boxes className="size-4 shrink-0 text-primary" />
                    {app.name}
                  </CardTitle>
                  <StatusBadge status={app.status} />
                </div>
                <CardDescription className="break-all">{app.base_url}</CardDescription>
              </CardHeader>
              <CardContent className="flex flex-col gap-3 flex-1">
                <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                  <Badge variant="outline">client_id: {app.client_id}</Badge>
                  <Badge variant="outline">{app.sso_type}</Badge>
                  <Badge variant="outline">{app.frame_mode === 'iframe' ? 'iframe' : '新标签页'}</Badge>
                  {app.scopes && <Badge variant="outline">scopes: {app.scopes}</Badge>}
                </div>
                <div className="flex items-center gap-2 mt-auto">
                  <Button size="sm" onClick={() => handleLaunch(app)} disabled={app.status !== 'enabled'}>
                    <ExternalLink className="size-4 mr-1" />
                    打开
                  </Button>
                  <Button size="sm" variant="outline" onClick={() => openEdit(app)}>
                    <Pencil className="size-4 mr-1" />
                    编辑
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="text-destructive hover:text-destructive"
                    onClick={() => handleDelete(app)}
                  >
                    <Trash2 className="size-4 mr-1" />
                    删除
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        <EmptyState
          icon={Boxes}
          title="还没有连接外部平台"
          description="登记一个外部平台，之后即可在 TrustMesh 内免密打开它。"
          action={
            <Button onClick={openCreate}>
              <Plus className="size-4 mr-2" />
              新增外部平台
            </Button>
          }
        />
      )}

      <ExternalAppFormDialog open={showForm} onOpenChange={setShowForm} app={editing} />
    </PageContainer>
  )
}

function StatusBadge({ status }: { status: ExternalAppView['status'] }) {
  if (status === 'enabled') {
    return (
      <Badge className="bg-emerald-500/15 text-emerald-600 hover:bg-emerald-500/15 dark:text-emerald-400">
        已启用
      </Badge>
    )
  }
  return (
    <Badge variant="secondary" className="text-muted-foreground">
      已停用
    </Badge>
  )
}
