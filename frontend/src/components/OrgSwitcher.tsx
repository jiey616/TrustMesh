import { useEffect, useMemo } from 'react'
import { Building2, ChevronsUpDown, User, Check } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { useQueryClient } from '@tanstack/react-query'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/authStore'
import { useOrganizations } from '@/hooks/useOrgs'

export const PERSONAL_ORG_KEY = '__personal__'

const roleLabels: Record<string, string> = {
  owner: 'Owner',
  admin: 'Admin',
  member: '成员',
}

// 侧边栏底部租户切换器（旧轨，与 frontend-v2 OrgSwitcher 语义一致）。
// 选择「个人空间」时 activeOrgId=null（不携带 X-Org-Id，与无头请求行为完全一致）；
// 选择企业租户则写入 org id，由 api/client.ts 注入 X-Org-Id。
// 切换后清空 react-query 缓存，全站数据按新租户上下文重新拉取（不重登）。
export function OrgSwitcher({ collapsed }: { collapsed: boolean }) {
  const activeOrgId = useAuthStore((s) => s.activeOrgId)
  const setActiveOrg = useAuthStore((s) => s.setActiveOrg)
  const { data: orgs } = useOrganizations()
  const qc = useQueryClient()

  const personalOrg = useMemo(() => (orgs ?? []).find((o) => o.kind === 'personal'), [orgs])
  const enterpriseOrgs = useMemo(() => (orgs ?? []).filter((o) => o.kind === 'enterprise'), [orgs])
  const current = useMemo(
    () => (orgs ?? []).find((o) => o.id === activeOrgId) ?? null,
    [orgs, activeOrgId],
  )
  const currentName = current ? current.name : personalOrg?.name || '个人空间'
  const canSwitch = (orgs?.length ?? 0) > 1

  // 防御：持久化的 activeOrgId 若已不在我的租户列表（被移出/数据回退），自动回落个人空间
  useEffect(() => {
    if (activeOrgId && orgs && !orgs.some((o) => o.id === activeOrgId)) {
      setActiveOrg(null)
      qc.removeQueries()
    }
  }, [activeOrgId, orgs, qc, setActiveOrg])

  const handleSelect = (key: string) => {
    if (!canSwitch) return
    setActiveOrg(key === PERSONAL_ORG_KEY ? null : key)
    qc.removeQueries()
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            className={cn(
              'w-full justify-start gap-2 text-muted-foreground hover:text-foreground',
              collapsed ? 'px-0 justify-center' : 'px-2'
            )}
            disabled={!canSwitch}
            title={collapsed ? currentName : undefined}
          >
            <Building2 className="size-4 shrink-0" />
            {!collapsed && (
              <>
                <span className="truncate text-sm">{currentName}</span>
                {canSwitch && <ChevronsUpDown className="ml-auto size-3.5 shrink-0" />}
              </>
            )}
          </Button>
        }
      />
      <DropdownMenuContent align="start" side="right" sideOffset={8}>
        <DropdownMenuLabel>切换工作区</DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => handleSelect(PERSONAL_ORG_KEY)}>
          <User className="size-4" />
          <span>{personalOrg ? personalOrg.name : '个人空间'}</span>
          {activeOrgId === null && <Check className="ml-auto size-4" />}
        </DropdownMenuItem>
        {enterpriseOrgs.map((o) => (
          <DropdownMenuItem key={o.id} onClick={() => handleSelect(o.id)}>
            <Building2 className="size-4" />
            <span>{o.name}</span>
            {o.my_role !== 'member' && (
              <Badge variant="secondary" className="ml-auto">
                {roleLabels[o.my_role]}
              </Badge>
            )}
            {activeOrgId === o.id && <Check className="ml-auto size-4" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

