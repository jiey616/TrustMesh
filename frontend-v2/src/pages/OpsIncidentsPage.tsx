import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Button,
  Drawer,
  Empty,
  Input,
  Modal,
  Segmented,
  Skeleton,
  Table,
  Tag,
  Timeline,
  Tooltip,
  Typography,
  App,
} from 'antd'
import {
  AlertOutlined,
  CheckOutlined,
  EyeOutlined,
  StopOutlined,
  ReloadOutlined,
} from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import { useCloseOpsIncident, useIgnoreOpsIncident, useOpsIncidents } from '@/hooks/useOps'
import { ApiRequestError } from '@/types'
import type { OpsAction, OpsIncident, OpsIncidentStatus } from '@/types/ops'
import { usePermStore } from '@/stores/permStore'
import { PERM } from '@/lib/perms'

const { Text, Paragraph } = Typography

// 状态中文化 + 颜色（信号色只作状态标记，不做大面积气氛场）
const STATUS_META: Record<OpsIncidentStatus, { label: string; color: string }> = {
  open: { label: '待处理', color: 'var(--warning)' },
  diagnosing: { label: '归因中', color: 'var(--info)' },
  guiding: { label: '已下发指引', color: 'var(--signal)' },
  resolved: { label: '已关闭', color: 'var(--success)' },
  escalated: { label: '转人工', color: 'var(--error)' },
  ignored: { label: '已忽略', color: 'var(--text-quaternary)' },
}

const RULE_LABEL: Record<string, string> = {
  todo_stalled: '任务卡步',
  task_silent: '任务沉默',
  deliverable_unbound: '交付物未绑定',
  deliverable_rejected: '交付物被拒',
}

const ACTION_META: Record<string, { label: string; color: string }> = {
  created: { label: '工单创建', color: 'var(--warning)' },
  diagnosed: { label: '归因完成', color: 'var(--info)' },
  guided: { label: '下发指引', color: 'var(--signal)' },
  reminded: { label: '超时催办', color: 'var(--cyan)' },
  escalated: { label: '升级人工', color: 'var(--error)' },
  resolved: { label: '关闭', color: 'var(--success)' },
  ignored: { label: '忽略', color: 'var(--text-quaternary)' },
  reopened: { label: '复发重开', color: 'var(--warning)' },
}

type FilterKey = 'active' | 'ignored' | 'resolved' | 'all'

function fmtTime(iso?: string): string {
  if (!iso) return '-'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleString('zh-CN', { hour12: false })
}

function relTime(iso: string): string {
  const d = new Date(iso).getTime()
  if (Number.isNaN(d)) return '-'
  const diff = Date.now() - d
  const min = Math.floor(diff / 60000)
  if (min < 1) return '刚刚'
  if (min < 60) return `${min} 分钟前`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr} 小时前`
  return `${Math.floor(hr / 24)} 天前`
}

export function OpsIncidentsPage() {
  const { message } = App.useApp()
  const navigate = useNavigate()
  const [filter, setFilter] = useState<FilterKey>('active')
  const [detailId, setDetailId] = useState<string | null>(null)
  const [actionModal, setActionModal] = useState<{
    kind: 'ignore' | 'close'
    incident: OpsIncident
  } | null>(null)
  const [reason, setReason] = useState('')
  // 关闭/忽略工单 = POST /ops/incidents/:id/close|ignore（ops.manage），
  // 无权限时隐藏入口（权限视图未就绪时 fail-open，由后端 403 兜底）；
  // 详情/刷新/筛选都是 GET 查询，不加门禁。
  const canManageOps = usePermStore((s) => s.hasPerm(PERM.OPS_MANAGE))

  const { data: incidents, isLoading, isFetching, refetch } = useOpsIncidents(filter)
  const ignoreMut = useIgnoreOpsIncident()
  const closeMut = useCloseOpsIncident()

  const counts = useMemo(() => {
    const all = incidents ?? []
    return {
      total: all.length,
      escalated: all.filter((i) => i.status === 'escalated').length,
      guiding: all.filter((i) => i.status === 'guiding').length,
    }
  }, [incidents])

  const detail = useMemo(
    () => (incidents ?? []).find((i) => i.id === detailId) ?? null,
    [incidents, detailId],
  )

  const submitManualAction = () => {
    if (!actionModal) return
    const { kind, incident } = actionModal
    const mut = kind === 'ignore' ? ignoreMut : closeMut
    mut.mutate(
      { id: incident.id, reason: reason.trim() || undefined },
      {
        onSuccess: () => {
          message.success(kind === 'ignore' ? '已忽略该工单' : '已关闭该工单')
          setActionModal(null)
          setReason('')
        },
        onError: (err) => {
          const msg = err instanceof ApiRequestError ? err.message : '操作失败'
          message.error(msg)
        },
      },
    )
  }

  const columns = [
    {
      title: '严重度',
      dataIndex: 'severity',
      key: 'severity',
      width: 90,
      render: (_: unknown, r: OpsIncident) => (
        <Tag
          style={{
            marginInlineEnd: 0,
            background: 'transparent',
            borderColor: r.severity === 'critical' ? 'var(--error)' : 'var(--warning)',
            color: r.severity === 'critical' ? 'var(--error)' : 'var(--warning)',
          }}
        >
          {r.severity === 'critical' ? '严重' : '警告'}
        </Tag>
      ),
    },
    {
      title: '问题',
      key: 'title',
      render: (_: unknown, r: OpsIncident) => (
        <div style={{ minWidth: 0 }}>
          <div style={{ color: 'var(--text-primary)', fontSize: 13, marginBottom: 2 }}>
            {r.title}
          </div>
          <Text style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>
            {RULE_LABEL[r.rule_id] ?? r.rule_id}
            {r.task_id ? ` · 任务 ${r.task_title || r.task_id.slice(0, 8)}` : ''}
            {r.node_id ? ` · 节点 ${r.node_id.slice(0, 8)}` : ''}
          </Text>
        </div>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 110,
      render: (s: OpsIncidentStatus) => {
        const meta = STATUS_META[s] ?? { label: s, color: 'var(--text-tertiary)' }
        return (
          <Tag
            style={{
              marginInlineEnd: 0,
              background: 'transparent',
              borderColor: meta.color,
              color: meta.color,
            }}
          >
            {meta.label}
          </Tag>
        )
      },
    },
    {
      title: '干预',
      dataIndex: 'guide_count',
      key: 'guide_count',
      width: 70,
      align: 'center' as const,
      render: (n: number) => (
        <Text style={{ fontSize: 12, color: 'var(--text-secondary)' }}>{n} 次</Text>
      ),
    },
    {
      title: '最近动态',
      dataIndex: 'updated_at',
      key: 'updated_at',
      width: 120,
      render: (iso: string) => (
        <Tooltip title={fmtTime(iso)}>
          <Text style={{ fontSize: 12, color: 'var(--text-secondary)' }}>{relTime(iso)}</Text>
        </Tooltip>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 190,
      render: (_: unknown, r: OpsIncident) => {
        const active = r.status !== 'resolved' && r.status !== 'ignored'
        return (
          <div style={{ display: 'flex', gap: 6 }}>
            <Button
              size="small"
              icon={<EyeOutlined />}
              onClick={() => setDetailId(r.id)}
              style={{ fontSize: 12 }}
            >
              详情
            </Button>
            {active && canManageOps && (
              <>
                <Button
                  size="small"
                  icon={<CheckOutlined />}
                  onClick={() => {
                    setReason('')
                    setActionModal({ kind: 'close', incident: r })
                  }}
                  style={{ fontSize: 12 }}
                >
                  关闭
                </Button>
                <Button
                  size="small"
                  icon={<StopOutlined />}
                  danger
                  onClick={() => {
                    setReason('')
                    setActionModal({ kind: 'ignore', incident: r })
                  }}
                  style={{ fontSize: 12 }}
                >
                  忽略
                </Button>
              </>
            )}
          </div>
        )
      },
    },
  ]

  return (
    <div>
      <PageHeader
        title="运维工单"
        icon={<AlertOutlined />}
        subtitle={`活跃工单自动跟踪，人工动作全程留痕`}
        actions={
          <Button
            icon={<ReloadOutlined />}
            onClick={() => void refetch()}
            loading={isFetching}
            style={{ fontSize: 12 }}
          >
            刷新
          </Button>
        }
      />

      {/* 概览条 */}
      <div
        style={{
          display: 'flex',
          gap: 12,
          marginBottom: 16,
          flexWrap: 'wrap',
        }}
      >
        {[
          { label: '本视图工单', value: counts.total, color: 'var(--text-primary)' },
          { label: '等待指引响应', value: counts.guiding, color: 'var(--signal)' },
          { label: '已转人工', value: counts.escalated, color: 'var(--error)' },
        ].map((s) => (
          <div
            key={s.label}
            style={{
              padding: '10px 16px',
              background: 'var(--surface)',
              border: '1px solid var(--line)',
              borderRadius: 'var(--radius-control)',
              minWidth: 130,
            }}
          >
            <div style={{ fontSize: 20, fontWeight: 600, color: s.color }}>{s.value}</div>
            <div style={{ fontSize: 11, color: 'var(--text-tertiary)', marginTop: 2 }}>
              {s.label}
            </div>
          </div>
        ))}
      </div>

      <div style={{ marginBottom: 12 }}>
        <Segmented
          value={filter}
          onChange={(v) => setFilter(v as FilterKey)}
          options={[
            { value: 'active', label: '活跃' },
            { value: 'ignored', label: '已忽略' },
            { value: 'resolved', label: '已关闭' },
            { value: 'all', label: '全部' },
          ]}
        />
      </div>

      <div
        style={{
          background: 'var(--surface)',
          border: '1px solid var(--line)',
          borderRadius: 'var(--radius-control)',
          padding: '4px 8px',
        }}
      >
        <Table
          rowKey="id"
          columns={columns}
          dataSource={incidents ?? []}
          loading={isLoading}
          pagination={{ pageSize: 15, showSizeChanger: false }}
          locale={{
            emptyText: (
              <Empty
                description={
                  filter === 'active' ? '当前没有活跃工单，一切正常' : '暂无工单'
                }
                image={Empty.PRESENTED_IMAGE_SIMPLE}
              />
            ),
          }}
        />
      </div>

      {/* 详情抽屉 */}
      <Drawer
        title={
          detail ? (
            <span style={{ color: 'var(--text-primary)', fontSize: 15 }}>{detail.title}</span>
          ) : null
        }
        placement="right"
        width={560}
        open={!!detailId}
        onClose={() => setDetailId(null)}
        styles={{ body: { padding: '16px 20px' } }}
        footer={
          detail && canManageOps && detail.status !== 'resolved' && detail.status !== 'ignored' ? (
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <Button
                icon={<CheckOutlined />}
                onClick={() => {
                  setReason('')
                  setActionModal({ kind: 'close', incident: detail })
                }}
              >
                关闭工单
              </Button>
              <Button
                danger
                icon={<StopOutlined />}
                onClick={() => {
                  setReason('')
                  setActionModal({ kind: 'ignore', incident: detail })
                }}
              >
                忽略
              </Button>
            </div>
          ) : null
        }
      >
        {!detail ? (
          <Skeleton active />
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            {/* 元信息 */}
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
              <Tag
                style={{
                  marginInlineEnd: 0,
                  background: 'transparent',
                  borderColor:
                    STATUS_META[detail.status]?.color ?? 'var(--text-tertiary)',
                  color: STATUS_META[detail.status]?.color ?? 'var(--text-tertiary)',
                }}
              >
                {STATUS_META[detail.status]?.label ?? detail.status}
              </Tag>
              <Tag style={{ marginInlineEnd: 0, background: 'transparent' }}>
                {RULE_LABEL[detail.rule_id] ?? detail.rule_id}
              </Tag>
              <Tag style={{ marginInlineEnd: 0, background: 'transparent' }}>
                自动干预 {detail.guide_count} 次
              </Tag>
            </div>

            {/* 现象 */}
            <div>
              <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginBottom: 6 }}>
                现象
              </div>
              <Paragraph
                style={{
                  margin: 0,
                  fontSize: 13,
                  color: 'var(--text-primary)',
                  whiteSpace: 'pre-wrap',
                }}
              >
                {detail.summary || '（无）'}
              </Paragraph>
            </div>

            {/* 根因（LLM/模板归因产出） */}
            {detail.root_cause && (
              <div>
                <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginBottom: 6 }}>
                  根因
                  {detail.attr_source ? `（${detail.attr_source === 'llm' ? 'LLM 归因' : detail.attr_source === 'template' ? '模板归因' : '未归因'}）` : ''}
                </div>
                <Paragraph
                  style={{
                    margin: 0,
                    fontSize: 13,
                    color: 'var(--text-primary)',
                    whiteSpace: 'pre-wrap',
                  }}
                >
                  {detail.root_cause}
                </Paragraph>
              </div>
            )}

            {/* 关联主体（名称 + 跳转；名称由后端反查填充，miss 回退显示 ID） */}
            <div>
              <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginBottom: 6 }}>
                关联主体
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                {[
                  {
                    label: '项目',
                    display: detail.project_name || detail.project_id,
                    link: detail.project_id ? `/projects/${detail.project_id}` : null,
                    id: detail.project_id,
                  },
                  {
                    label: '任务',
                    display: detail.task_title || detail.task_id,
                    // 任务深链：项目详情页 ?task= 直达该任务工作区
                    link:
                      detail.task_id && detail.project_id
                        ? `/projects/${detail.project_id}?task=${detail.task_id}`
                        : null,
                    id: detail.task_id,
                  },
                  {
                    label: 'Todo',
                    display: detail.todo_id,
                    link: null,
                    id: detail.todo_id,
                  },
                  {
                    label: 'Agent',
                    display: detail.agent_name || detail.agent_id,
                    link: detail.agent_id ? `/agents/${detail.agent_id}` : null,
                    id: detail.agent_id,
                  },
                  {
                    label: '节点',
                    display: detail.node_id,
                    link: null,
                    id: detail.node_id,
                  },
                ]
                  .filter((r) => !!r.id)
                  .map((r) => (
                    <div
                      key={r.label}
                      style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}
                    >
                      <span
                        style={{ fontSize: 12, color: 'var(--text-secondary)', flexShrink: 0 }}
                      >
                        {r.label}：
                      </span>
                      {r.link ? (
                        <a
                          href={r.link}
                          onClick={(e) => {
                            e.preventDefault()
                            navigate(r.link!)
                          }}
                          style={{ fontSize: 12, color: 'var(--signal)', cursor: 'pointer' }}
                        >
                          {r.display}
                        </a>
                      ) : (
                        <span
                          style={{
                            fontSize: 12,
                            color: 'var(--text-secondary)',
                            wordBreak: 'break-all',
                          }}
                        >
                          {r.display}
                        </span>
                      )}
                    </div>
                  ))}
                {!detail.task_id && !detail.node_id && !detail.project_id && (
                  <Text style={{ fontSize: 12, color: 'var(--text-quaternary)' }}>（无）</Text>
                )}
              </div>
            </div>

            {/* 动作时间线 */}
            <div>
              <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginBottom: 10 }}>
                处理时间线
              </div>
              <Timeline
                items={(detail.actions ?? []).map((a: OpsAction) => {
                  const meta = ACTION_META[a.kind] ?? { label: a.kind, color: 'var(--text-tertiary)' }
                  const resultTag =
                    a.kind === 'guided' ? (
                      <Tag
                        style={{
                          marginInlineStart: 6,
                          fontSize: 10,
                          lineHeight: '16px',
                          background: 'transparent',
                          borderColor:
                            a.result === 'sent'
                              ? 'var(--success)'
                              : a.result === 'failed'
                                ? 'var(--error)'
                                : 'var(--text-quaternary)',
                          color:
                            a.result === 'sent'
                              ? 'var(--success)'
                              : a.result === 'failed'
                                ? 'var(--error)'
                                : 'var(--text-quaternary)',
                        }}
                      >
                        {a.result === 'sent' ? '已下发' : a.result === 'throttled' ? '被节流' : a.result === 'failed' ? '失败' : '跳过'}
                      </Tag>
                    ) : null
                  return {
                    color: meta.color,
                    children: (
                      <div style={{ marginBottom: 4 }}>
                        <div>
                          <span style={{ fontSize: 13, color: 'var(--text-primary)' }}>
                            {meta.label}
                          </span>
                          {resultTag}
                          <span
                            style={{ fontSize: 11, color: 'var(--text-quaternary)', marginLeft: 8 }}
                          >
                            {fmtTime(a.at)}
                          </span>
                        </div>
                        {(a.detail || a.content) && (
                          <div
                            style={{
                              marginTop: 4,
                              padding: '6px 10px',
                              background: 'var(--surface-sunken)',
                              border: '1px solid var(--line)',
                              borderRadius: 'var(--radius-control)',
                              fontSize: 12,
                              color: 'var(--text-secondary)',
                              whiteSpace: 'pre-wrap',
                              wordBreak: 'break-all',
                            }}
                          >
                            {a.content || a.detail}
                          </div>
                        )}
                      </div>
                    ),
                  }
                })}
              />
            </div>
          </div>
        )}
      </Drawer>

      {/* 人工动作确认弹窗（忽略/关闭共用手感，可选理由，操作留痕） */}
      <Modal
        title={
          actionModal?.kind === 'ignore'
            ? '忽略该工单？'
            : actionModal?.kind === 'close'
              ? '关闭该工单？'
              : null
        }
        open={!!actionModal}
        onOk={submitManualAction}
        onCancel={() => setActionModal(null)}
        okText={actionModal?.kind === 'ignore' ? '确认忽略' : '确认关闭'}
        cancelText="取消"
        confirmLoading={ignoreMut.isPending || closeMut.isPending}
        okButtonProps={
          actionModal?.kind === 'ignore' ? { danger: true } : undefined
        }
        width={440}
      >
        <Paragraph style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 12 }}>
          {actionModal?.kind === 'ignore'
            ? '忽略后平台不再对该问题做任何自动干预；同类问题复发时会新开工单，不会被本操作挡住。'
            : '关闭表示你确认该问题已解决或无需跟踪，语义等同自动关闭。'}
        </Paragraph>
        <Input.TextArea
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="备注理由（可选，将记入处理时间线）"
          rows={3}
          maxLength={500}
          showCount
        />
      </Modal>
    </div>
  )
}
