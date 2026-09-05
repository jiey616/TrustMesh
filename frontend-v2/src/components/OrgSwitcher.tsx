import { useEffect, useMemo } from 'react'
import { Dropdown, Tag } from 'antd'
import { SwapOutlined, CrownOutlined, UserOutlined, DownOutlined } from '@ant-design/icons'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '@/stores/authStore'
import { useOrganizations } from '@/hooks/useOrgs'
import type { OrgView } from '@/types'

const roleLabels: Record<string, string> = {
  owner: 'Owner',
  admin: 'Admin',
  member: '成员',
}

export const PERSONAL_ORG_KEY = '__personal__'

// 侧边栏底部租户切换器。
// 语义：选择「个人空间」时 activeOrgId=null（不携带 X-Org-Id，与无头请求行为完全一致）；
// 选择企业租户则写入 org id，由 api/client.ts 注入 X-Org-Id。
// 切换后清空 react-query 缓存，全站数据按新租户上下文重新拉取（不重登）。
export function OrgSwitcher({ collapsed }: { collapsed: boolean }) {
  const { activeOrgId, setActiveOrg } = useAuthStore()
  const { data: orgs } = useOrganizations()
  const qc = useQueryClient()

  const personalOrg = useMemo(() => (orgs ?? []).find((o) => o.kind === 'personal'), [orgs])
  const enterpriseOrgs = useMemo(
    () => (orgs ?? []).filter((o) => o.kind === 'enterprise'),
    [orgs],
  )
  const current = useMemo(
    () => (orgs ?? []).find((o) => o.id === activeOrgId) ?? null,
    [orgs, activeOrgId],
  )
  const currentName = current ? current.name : '个人空间'

  const items = useMemo(() => {
    const list: { key: string; icon: React.ReactNode; label: React.ReactNode }[] = [
      {
        key: PERSONAL_ORG_KEY,
        icon: <UserOutlined />,
        label: <span>{personalOrg ? personalOrg.name : '个人空间'}</span>,
      },
    ]
    for (const o of enterpriseOrgs) {
      list.push({
        key: o.id,
        icon: (
          <CrownOutlined style={{ color: o.my_role === 'owner' ? 'var(--signal)' : undefined }} />
        ),
        label: (
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
            <span>{o.name}</span>
            <Tag
              style={{
                marginInlineEnd: 0,
                fontSize: 10,
                lineHeight: '16px',
                borderRadius: 'var(--radius-control)',
              }}
            >
              {roleLabels[o.my_role]}
            </Tag>
          </span>
        ),
      })
    }
    return list
  }, [personalOrg, enterpriseOrgs])

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
    // 清空全部缓存，让所有页面按新租户上下文重新拉取
    qc.removeQueries()
  }

  if (collapsed) {
    return (
      <Dropdown menu={{ items, onClick: ({ key }) => handleSelect(key) }} trigger={['click']} disabled={!canSwitch}>
        <div className="tm-iconbtn" title={currentName} style={{ margin: '0 auto', fontSize: 13, fontWeight: 600 }}>
          {currentName.slice(0, 1)}
        </div>
      </Dropdown>
    )
  }

  return (
    <Dropdown menu={{ items, onClick: ({ key }) => handleSelect(key) }} trigger={['click']} disabled={!canSwitch}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          padding: '9px 12px',
          margin: '0 8px',
          borderRadius: 'var(--radius-control)',
          cursor: canSwitch ? 'pointer' : 'default',
          fontSize: 13,
          color: 'var(--text-secondary)',
          whiteSpace: 'nowrap',
          transition: 'background 0.15s',
        }}
      >
        <SwapOutlined style={{ fontSize: 15, flexShrink: 0 }} />
        <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis' }}>{currentName}</span>
        {canSwitch && <DownOutlined style={{ fontSize: 10, color: 'var(--text-quaternary)' }} />}
      </div>
    </Dropdown>
  )
}

export type { OrgView }
