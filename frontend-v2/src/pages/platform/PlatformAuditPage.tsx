import { useState } from 'react'
import { Button, Card, Input, Select, Space, Table, Tag, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import { useAuditLogs } from '@/hooks/usePlatformAdmin'
import type { AuditLogQuery, AuditLogView } from '@/types'

const { Text } = Typography

/** 审计 action 枚举（与后端 model.AuditAction* 一致）。 */
const ACTION_OPTIONS = [
  'org.create',
  'org.disable',
  'org.restore',
  'org.member.add',
  'org.member.remove',
  'org.member.role_change',
  'org.role.create',
  'org.role.update',
  'org.role.delete',
  'org.menu_override.update',
  'project.archive',
  'agent.delete',
  'platform.config.update',
  'platform.admin.login',
].map((v) => ({ value: v, label: v }))

const fmtTime = (s: string) => (s ? new Date(s).toLocaleString() : '—')

/**
 * 平台管理 · 审计日志（设计文档 §6.4）：全局查询，本期仅平台侧可见。
 *
 * 企业敏感操作与平台操作都落这里；记录只追加、只读展示（TTL 保留 180 天）。
 */
export function PlatformAuditPage() {
  const [scope, setScope] = useState<string | undefined>(undefined)
  const [action, setAction] = useState<string | undefined>(undefined)
  const [actor, setActor] = useState('')

  const query: AuditLogQuery = { scope, action, actor_user_id: actor.trim() || undefined, limit: 200 }
  const { data: logs, isLoading, isFetching, refetch } = useAuditLogs(query)

  const columns = [
    {
      title: '时间',
      key: 'created_at',
      width: 170,
      render: (_: unknown, l: AuditLogView) => (
        <Text type="secondary" style={{ fontSize: 12 }}>{fmtTime(l.created_at)}</Text>
      ),
    },
    {
      title: '操作者',
      key: 'actor',
      width: 200,
      render: (_: unknown, l: AuditLogView) => (
        <Space direction="vertical" size={0}>
          <Text style={{ fontSize: 13 }}>{l.actor_email || '—'}</Text>
          <Text type="secondary" style={{ fontSize: 11 }}>{l.actor_user_id}</Text>
        </Space>
      ),
    },
    {
      title: '动作',
      key: 'action',
      width: 200,
      render: (_: unknown, l: AuditLogView) => (
        <Tag style={{ marginInlineEnd: 0, fontSize: 11 }}>{l.action}</Tag>
      ),
    },
    {
      title: '作用域',
      key: 'scope',
      width: 220,
      render: (_: unknown, l: AuditLogView) => (
        <Text type="secondary" style={{ fontSize: 12 }}>
          {l.scope === 'platform' ? '平台' : l.scope}
        </Text>
      ),
    },
    {
      title: '目标',
      key: 'target',
      width: 200,
      render: (_: unknown, l: AuditLogView) => (
        <Text type="secondary" style={{ fontSize: 12 }}>
          {l.target_type}:{l.target_id}
        </Text>
      ),
    },
    {
      title: '详情',
      key: 'detail',
      render: (_: unknown, l: AuditLogView) => (
        <Text code style={{ fontSize: 11 }}>
          {l.detail ? JSON.stringify(l.detail) : '—'}
        </Text>
      ),
    },
  ]

  return (
    <div style={{ paddingBottom: 40 }}>
      <PageHeader
        title="平台管理 · 审计日志"
        subtitle="全局操作留痕（企业敏感操作 + 平台操作），保留 180 天"
        actions={
          <Button icon={<ReloadOutlined />} onClick={() => void refetch()} loading={isFetching}>
            刷新
          </Button>
        }
      />

      <Card style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}>
        <Space style={{ marginBottom: 12 }} wrap>
          <Input
            placeholder="按操作者 user_id 过滤"
            allowClear
            style={{ width: 260 }}
            onChange={(e) => setActor(e.target.value)}
            onPressEnter={(e) => setActor((e.target as HTMLInputElement).value)}
          />
          <Input
            placeholder="作用域：platform 或具体 org_id"
            allowClear
            style={{ width: 240 }}
            value={scope}
            onChange={(e) => setScope(e.target.value)}
          />
          <Select
            placeholder="动作"
            allowClear
            style={{ width: 220 }}
            value={action}
            onChange={(v) => setAction(v)}
            options={ACTION_OPTIONS}
          />
        </Space>
        <Table<AuditLogView>
          rowKey="id"
          size="small"
          loading={isLoading}
          dataSource={logs ?? []}
          columns={columns as never}
          pagination={{ pageSize: 20, hideOnSinglePage: true }}
        />
      </Card>
    </div>
  )
}