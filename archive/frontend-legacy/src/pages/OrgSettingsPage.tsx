import { useState } from 'react'
import { Building2, Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { PageContainer } from '@/components/layout/PageContainer'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Label } from '@/components/ui/label'
import { useAuthStore } from '@/stores/authStore'
import {
  useAddOrgMember,
  useCreateOrg,
  useOrganizations,
  useOrgMembers,
  useRemoveOrgMember,
  useUpdateOrgMemberRole,
} from '@/hooks/useOrgs'
import { ApiRequestError } from '@/api/client'
import type { OrgMemberView, OrgView } from '@/types'

const roleTagVariant: Record<string, 'default' | 'secondary' | 'destructive' | 'info'> = {
  owner: 'info',
  admin: 'secondary',
  member: 'default',
}
const roleLabels: Record<string, string> = {
  owner: 'Owner',
  admin: 'Admin',
  member: '成员',
}

const roleOptions = [
  { value: 'admin', label: 'Admin' },
  { value: 'member', label: '成员' },
] as const

type RoleOption = (typeof roleOptions)[number]['value']

function formatBytes(n: number) {
  if (n < 0) return '不限'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}

const fmtDate = (s?: string) => (s ? new Date(s).toLocaleDateString() : '—')

function errMessage(e: unknown) {
  return e instanceof ApiRequestError ? e.message : '操作失败'
}

function CreateOrgDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const createOrg = useCreateOrg()
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')

  const handleCreate = async () => {
    if (!name.trim()) {
      toast.error('请输入企业名称')
      return
    }
    try {
      await createOrg.mutateAsync({ name: name.trim(), slug: slug.trim() || undefined })
      toast.success('企业已创建')
      setName('')
      setSlug('')
      onClose()
    } catch (e) {
      toast.error(errMessage(e))
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>创建企业</DialogTitle>
          <DialogDescription>
            创建后你将成为该企业的 Owner，可在成员管理中邀请已有账号加入。
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4 py-2">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="org-name">企业名称</Label>
            <Input
              id="org-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：山雨影业"
              maxLength={64}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="org-slug">标识（可选）</Label>
            <Input
              id="org-slug"
              value={slug}
              onChange={(e) => setSlug(e.target.value)}
              placeholder="小写字母/数字/连字符，留空自动生成"
              maxLength={64}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button onClick={handleCreate} disabled={createOrg.isPending}>
            {createOrg.isPending ? '创建中…' : '创建'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function MembersCard({ org }: { org: OrgView }) {
  const { data: members, isLoading } = useOrgMembers(org.id)
  const addMember = useAddOrgMember(org.id)
  const updateRole = useUpdateOrgMemberRole(org.id)
  const removeMember = useRemoveOrgMember(org.id)

  const [email, setEmail] = useState('')
  const [role, setRole] = useState<RoleOption>('member')

  const canManage = org.my_role === 'owner' || org.my_role === 'admin'

  const handleAdd = async () => {
    const addr = email.trim()
    if (!addr) return
    try {
      await addMember.mutateAsync({ email: addr, role })
      toast.success(`已添加 ${addr}`)
      setEmail('')
    } catch (e) {
      toast.error(errMessage(e))
    }
  }

  const canTouch = (m: OrgMemberView) =>
    m.role !== 'owner' && (org.my_role === 'owner' || m.role === 'member')

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Building2 className="size-4" />
          成员管理
        </CardTitle>
        <CardDescription>
          仅能邀请已在平台注册的账号；Owner 不可移除或降级（转让功能二期提供）。
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {canManage && (
          <div className="flex flex-wrap items-center gap-2">
            <Input
              className="max-w-64"
              placeholder="对方注册邮箱"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleAdd()}
              disabled={addMember.isPending}
            />
            <Select value={role} onValueChange={(v) => setRole(v as RoleOption)}>
              <SelectTrigger className="w-28">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {roleOptions.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button size="sm" onClick={handleAdd} disabled={addMember.isPending}>
              <Plus className="size-4" />
              添加
            </Button>
          </div>
        )}

        {isLoading ? (
          <Skeleton className="h-24 w-full" />
        ) : (members?.length ?? 0) === 0 ? (
          <p className="text-sm text-muted-foreground">暂无成员</p>
        ) : (
          <div className="flex flex-col divide-y rounded-lg border">
            {(members ?? []).map((m) => (
              <div key={m.id} className="flex items-center gap-3 px-3 py-2.5">
                <div className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-sm font-medium">
                    {m.name || m.email || m.user_id}
                    {m.email && m.name && (
                      <span className="ml-2 text-xs font-normal text-muted-foreground">
                        {m.email}
                      </span>
                    )}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    加入于 {fmtDate(m.joined_at)}
                  </span>
                </div>
                <Badge variant={roleTagVariant[m.role] ?? 'default'}>
                  {roleLabels[m.role] ?? m.role}
                </Badge>
                {canManage && m.role !== 'owner' && canTouch(m) && (
                  <div className="flex items-center gap-2">
                    <Select
                      value={m.role}
                      disabled={!canTouch(m)}
                      onValueChange={async (r) => {
                        try {
                          await updateRole.mutateAsync({
                            userId: m.user_id,
                            role: r as 'admin' | 'member',
                          })
                          toast.success('角色已更新')
                        } catch (e) {
                          toast.error(errMessage(e))
                        }
                      }}
                    >
                      <SelectTrigger className="h-8 w-24">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {roleOptions.map((o) => (
                          <SelectItem key={o.value} value={o.value}>
                            {o.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-8 text-destructive hover:text-destructive"
                      title="移除成员"
                      onClick={async () => {
                        if (!window.confirm(`确定移除「${m.name || m.email || m.user_id}」？`))
                          return
                        try {
                          await removeMember.mutateAsync(m.user_id)
                          toast.success('已移除')
                        } catch (e) {
                          toast.error(errMessage(e))
                        }
                      }}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

export function OrgSettingsPage() {
  const activeOrgId = useAuthStore((s) => s.activeOrgId)
  const { data: orgs, isLoading } = useOrganizations()
  const [createOpen, setCreateOpen] = useState(false)

  const orgsList = orgs ?? []
  const current: OrgView | undefined =
    orgsList.find((o) => o.id === activeOrgId) ?? orgsList.find((o) => o.kind === 'personal')

  const isPersonal = !current || current.kind === 'personal'

  return (
    <PageContainer className="mx-auto max-w-3xl">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">企业管理</h1>
          <p className="text-sm text-muted-foreground">
            租户信息、成员与权限（配额为预留字段，当前不做限额执行）
          </p>
        </div>
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className="size-4" />
          创建企业
        </Button>
      </div>

      {isLoading ? (
        <Skeleton className="h-40 w-full" />
      ) : !current ? (
        <p className="text-sm text-muted-foreground">没有可用租户</p>
      ) : (
        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex flex-wrap items-center gap-2">
                {current.name}
                <Badge variant={isPersonal ? 'secondary' : 'info'}>
                  {isPersonal ? '个人空间' : '企业'}
                </Badge>
                <Badge variant={roleTagVariant[current.my_role] ?? 'default'}>
                  我的角色：{roleLabels[current.my_role]}
                </Badge>
              </CardTitle>
              <CardDescription>
                标识 {current.slug} · 创建于 {fmtDate(current.created_at)}
              </CardDescription>
            </CardHeader>
            <CardContent className="grid grid-cols-2 gap-2 text-sm text-muted-foreground sm:grid-cols-4">
              <span>成员上限：{current.quota.max_members < 0 ? '不限' : current.quota.max_members}</span>
              <span>节点上限：{current.quota.max_nodes < 0 ? '不限' : current.quota.max_nodes}</span>
              <span>项目上限：{current.quota.max_projects < 0 ? '不限' : current.quota.max_projects}</span>
              <span>存储上限：{formatBytes(current.quota.max_storage_bytes)}</span>
            </CardContent>
          </Card>

          {isPersonal ? (
            <Card>
              <CardContent className="py-8 text-center text-sm text-muted-foreground">
                个人空间不支持成员管理。创建企业后即可邀请其他账号协同。
              </CardContent>
            </Card>
          ) : (
            <MembersCard org={current} />
          )}
        </div>
      )}

      <CreateOrgDialog open={createOpen} onClose={() => setCreateOpen(false)} />
    </PageContainer>
  )
}
