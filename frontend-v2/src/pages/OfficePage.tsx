import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { HomeOutlined, SwapOutlined, TeamOutlined } from '@ant-design/icons'
import { Button, Segmented, Switch, Tooltip, Typography } from 'antd'
import { PageHeader } from '@/components/shared/PageHeader'
import { OfficeCanvas } from '@/components/office/OfficeCanvas'
import { useActiveOfficeTasks, useOfficeData } from '@/components/office/useOfficeData'
import { STATE_COLORS } from '@/components/office/officeLayout'
import { useOfficeStore } from '@/stores/officeStore'
import type { AgentVisualState } from '@/types/office'

const { Text } = Typography

const LEGEND: ReadonlyArray<{ state: AgentVisualState; label: string }> = [
  { state: 'working', label: '执行中' },
  { state: 'asking', label: '等你答复' },
  { state: 'thinking', label: '规划中' },
  { state: 'meeting', label: '会议中' },
  { state: 'idle', label: '空闲' },
]

/** 角色中文名（后端 role 是英文枚举） */
const ROLE_LABEL: Record<string, string> = {
  pm: '项目经理',
  developer: '执行',
  reviewer: '审核',
  custom: '自定义',
}

/** 任务状态 → 办公室视觉语言（状态点颜色 + 中文标签） */
const TASK_STATUS_META: Record<string, { color: string; label: string }> = {
  in_progress: { color: STATE_COLORS.working, label: '执行中' },
  awaiting_review: { color: STATE_COLORS.asking, label: '待评审' },
  waiting_user: { color: STATE_COLORS.asking, label: '等你答复' },
  review: { color: STATE_COLORS.thinking, label: '评审中' },
  planning: { color: STATE_COLORS.thinking, label: '规划中' },
  pending: { color: 'var(--info)', label: '排队中' },
}

// ─── 顶部仪表盘卡片 ───
// 与 V2 卡片规范一致：深色半透明 + 细边框 + 16 圆角。
const cardStyle: React.CSSProperties = {
  background: 'var(--surface)',
  border: '1px solid var(--line)',
  backdropFilter: 'var(--glass-blur)',
  borderRadius: 'var(--radius-structure)',
  padding: '12px 14px',
  display: 'flex',
  flexDirection: 'column',
  minWidth: 0,
  flexShrink: 0,
}

const cardHeaderStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'baseline',
  justifyContent: 'space-between',
  gap: 8,
  marginBottom: 8,
}

const cardTitleStyle: React.CSSProperties = {
  fontSize: 12,
  color: 'var(--text-tertiary)',
  fontWeight: 500,
}

/** 可点击的横向 chip（任务 / 员工） */
const chipStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 6,
  padding: '5px 9px',
  borderRadius: 'var(--radius-control)',
  fontSize: 12,
  cursor: 'pointer',
  flexShrink: 0,
  maxWidth: 230,
  background: 'var(--surface)',
  border: '1px solid var(--line)',
  transition: 'background 0.15s, border-color 0.15s',
}

// ─── AI 办公室 ───
// 用 3D 办公空间把多智能体协作变成看得见的空间叙事：
// 顶部一排仪表盘卡片（团队状态 / 执行中任务 / 操作 / 待命区），
// 下方为 3D 场景；Agent 按角色落座，任务状态实时反映在状态光环 / 气泡 / 贴纸上。
export function OfficePage() {
  // 默认高画质：开阴影 + 更高 dpr。场景规模不大（12 工位），
  // 现代设备基本无压力；需要省电时可手动切「流畅」。
  const [highQuality, setHighQuality] = useState(true)
  const navigate = useNavigate()

  // 日/夜主题（选择持久化在 localStorage，由 officeStore 管理）
  const theme = useOfficeStore((s) => s.theme)
  const setTheme = useOfficeStore((s) => s.setTheme)

  useOfficeData()
  const agents = useOfficeStore((s) => s.agents)
  const benched = useOfficeStore((s) => s.benched)
  const focusAgentId = useOfficeStore((s) => s.focusAgentId)
  const setFocus = useOfficeStore((s) => s.setFocus)
  // 进行中的任务（与 useOfficeData 共享同一批请求缓存，不额外发请求）
  const activeTasks = useActiveOfficeTasks()
  // 消息流：3D 气泡只放短摘要，这里可滚动查看完整文本
  const feed = useOfficeStore((s) => s.feed)

  const benchedVisuals = benched.map((id) => agents[id]).filter(Boolean)
  const focused = focusAgentId ? agents[focusAgentId] : null

  /** 各状态的实时人数（含待命区员工） */
  const stats = useMemo(() => {
    const list = Object.values(agents)
    return {
      working: list.filter((a) => a.state === 'working').length,
      asking: list.filter((a) => a.state === 'asking').length,
      thinking: list.filter((a) => a.state === 'thinking').length,
      meeting: list.filter((a) => a.state === 'meeting').length,
      idle: list.filter((a) => a.state === 'idle').length,
      total: list.length,
    }
  }, [agents])

  /**
   * 手动触发一次走动（演示 / 排查用）。
   * 真实场景里走动由 SSE 事件驱动（派单、提问、交付评审），
   * 但等待真实任务流转来验证太慢，这里给个即时入口。
   */
  const triggerDemoVisit = () => {
    const s = useOfficeStore.getState()
    const seated = Object.keys(s.seating)
    const pm = seated.find((id) => s.agents[id]?.role === 'pm') ?? seated[0]
    const worker = seated.find((id) => id !== pm)
    if (!pm || !worker) return
    s.startVisit(pm, worker, '这个任务交给你了')
  }

  return (
    <div style={{ height: 'calc(100vh - 48px)' }}>
      <PageHeader
        title="AI 办公室"
        icon={<HomeOutlined />}
        subtitle="多智能体协作实时全景"
        actions={
          <>
            <Tooltip title="模拟一次派单：让项目经理走到某位员工工位当面交代">
              <Button icon={<SwapOutlined />} onClick={triggerDemoVisit}>
                演示走动
              </Button>
            </Tooltip>
            <Tooltip title="切换白天（亮系）/ 夜晚（暗系）主题，选择会被记住">
              <Segmented
                value={theme}
                onChange={(v) => setTheme(v as 'night' | 'day')}
                options={[
                  { label: '☀ 夜晚', value: 'night' },
                  { label: '◐ 白天', value: 'day' },
                ]}
              />
            </Tooltip>
            <Tooltip title="高画质会开启阴影并提高渲染分辨率，弱设备建议关闭">
              <Switch
                checked={highQuality}
                onChange={setHighQuality}
                checkedChildren="高画质"
                unCheckedChildren="流畅"
              />
            </Tooltip>
          </>
        }
      />

      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          gap: 12,
          height: 'calc(100% - 52px)',
        }}
      >
        {/* ─── 顶部仪表盘卡片行 ─── */}
        <div style={{ display: 'flex', gap: 12, flexShrink: 0, overflowX: 'auto' }}>
          {/* 团队状态 */}
          <div style={cardStyle}>
            <div style={cardHeaderStyle}>
              <Text style={cardTitleStyle}>团队状态</Text>
              <Text style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>
                共 {stats.total} 人
              </Text>
            </div>
            <div style={{ display: 'flex', gap: 18, alignItems: 'flex-end' }}>
              {LEGEND.map((item) => {
                const count = stats[item.state] ?? 0
                const color = STATE_COLORS[item.state]
                const active = count > 0
                return (
                  <div key={item.state} style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
                      <span
                        style={{
                          width: 7,
                          height: 7,
                          borderRadius: 'var(--radius-avatar)',
                          background: color,
                          boxShadow: active ? `0 0 8px ${color}` : undefined,
                          opacity: active ? 1 : 0.4,
                        }}
                      />
                      <Text
                        style={{
                          fontSize: 11,
                          color: active ? 'var(--text-secondary)' : 'var(--text-quaternary)',
                        }}
                      >
                        {item.label}
                      </Text>
                    </div>
                    <span
                      style={{
                        fontSize: 20,
                        fontWeight: 600,
                        lineHeight: 1.1,
                        color: active ? color : 'var(--text-quaternary)',
                        fontVariantNumeric: 'tabular-nums',
                      }}
                    >
                      {count}
                    </span>
                  </div>
                )
              })}
            </div>
          </div>

          {/* 执行中任务（横向 chips，点击直达任务详情） */}
          <div style={{ ...cardStyle, flex: 1, minWidth: 240 }}>
            <div style={cardHeaderStyle}>
              <Text style={cardTitleStyle}>执行中任务</Text>
              <Text style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>
                {activeTasks.length} 个 · 点击打开
              </Text>
            </div>
            <div
              style={{
                display: 'flex',
                gap: 6,
                alignItems: 'center',
                overflowX: 'auto',
                paddingBottom: 2,
                minHeight: 28,
              }}
            >
              {activeTasks.length === 0 ? (
                <Text style={{ fontSize: 12, color: 'var(--text-quaternary)' }}>
                  暂无执行中的任务
                </Text>
              ) : (
                activeTasks.map((task) => {
                  const meta = TASK_STATUS_META[task.status] ?? {
                    color: 'var(--text-quaternary)',
                    label: task.status,
                  }
                  return (
                    <div
                      key={task.id}
                      style={chipStyle}
                      title={task.title}
                      onClick={() => navigate(`/projects/${task.projectId}?task=${task.id}`)}
                      onMouseEnter={(e) => {
                        e.currentTarget.style.background = 'rgba(255,255,255,0.09)'
                        e.currentTarget.style.borderColor = 'rgba(255,255,255,0.16)'
                      }}
                      onMouseLeave={(e) => {
                        e.currentTarget.style.background = 'rgba(255,255,255,0.04)'
                        e.currentTarget.style.borderColor = 'rgba(255,255,255,0.07)'
                      }}
                    >
                      <span
                        style={{
                          width: 7,
                          height: 7,
                          borderRadius: 'var(--radius-avatar)',
                          background: meta.color,
                          boxShadow: `0 0 6px ${meta.color}`,
                          flexShrink: 0,
                        }}
                      />
                      <span
                        style={{
                          color: 'var(--text-secondary)',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                        }}
                      >
                        {task.title}
                      </span>
                      <span style={{ fontSize: 10, color: meta.color, flexShrink: 0, opacity: 0.9 }}>
                        {meta.label}
                      </span>
                    </div>
                  )
                })
              )}
            </div>
          </div>

          {/* 最新动态：3D 气泡放不下的完整消息，这里滚动展示 */}
          <div style={{ ...cardStyle, flex: 1.4, minWidth: 300, minHeight: 0 }}>
            <div style={cardHeaderStyle}>
              <Text style={cardTitleStyle}>最新动态</Text>
              <Text style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>
                {feed.length} 条 · 点击跳转任务
              </Text>
            </div>
            <div
              style={{
                maxHeight: 132,
                overflowY: 'auto',
                display: 'flex',
                flexDirection: 'column',
                gap: 3,
                paddingRight: 4,
              }}
            >
              {feed.length === 0 ? (
                <Text style={{ fontSize: 12, color: 'var(--text-quaternary)' }}>
                  暂无动态
                </Text>
              ) : (
                feed.map((item) => (
                  <div
                    key={item.id}
                    onClick={() =>
                      item.taskId &&
                      item.projectId &&
                      navigate(`/projects/${item.projectId}?task=${item.taskId}`)
                    }
                    style={{
                      display: 'flex',
                      alignItems: 'baseline',
                      gap: 6,
                      padding: '3px 6px',
                      borderRadius: 'var(--radius-control)',
                      cursor: item.taskId ? 'pointer' : 'default',
                      fontSize: 12,
                      lineHeight: 1.45,
                    }}
                    onMouseEnter={(e) => {
                      e.currentTarget.style.background = 'var(--surface-raised)'
                    }}
                    onMouseLeave={(e) => {
                      e.currentTarget.style.background = 'transparent'
                    }}
                  >
                    <span
                      style={{
                        width: 5,
                        height: 5,
                        borderRadius: 'var(--radius-avatar)',
                        flexShrink: 0,
                        marginTop: 5,
                        background:
                          item.kind === 'question' ? STATE_COLORS.asking : 'var(--text-tertiary)',
                      }}
                    />
                    <Text
                      style={{
                        fontSize: 12,
                        color: 'var(--text-secondary)',
                        flexShrink: 0,
                      }}
                    >
                      {item.agentName}
                    </Text>
                    <Text
                      style={{
                        fontSize: 12,
                        color: 'var(--text-secondary)',
                        flex: 1,
                        minWidth: 0,
                      }}
                      title={item.text}
                    >
                      {item.text}
                    </Text>
                  </div>
                ))
              )}
            </div>
          </div>

          {/* 操作提示 */}
          <div style={cardStyle}>
            <div style={cardHeaderStyle}>
              <Text style={cardTitleStyle}>操作</Text>
            </div>
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: '1fr 1fr',
                columnGap: 14,
                rowGap: 2,
              }}
            >
              {[
                '滚轮 — 缩放',
                '拖拽 — 旋转',
                '点员工 — 聚焦',
                '点空白 — 回全景',
              ].map((tip) => (
                <Text key={tip} style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>
                  {tip}
                </Text>
              ))}
            </div>
          </div>

          {/* 待命区（有人才显示） */}
          {benchedVisuals.length > 0 && (
            <div style={cardStyle}>
              <div style={cardHeaderStyle}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <TeamOutlined style={{ color: 'var(--text-tertiary)', fontSize: 11 }} />
                  <Text style={cardTitleStyle}>待命区</Text>
                </div>
                <Text style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>
                  {benchedVisuals.length} 人未落座
                </Text>
              </div>
              <div
                style={{ display: 'flex', gap: 6, alignItems: 'center', overflowX: 'auto', minHeight: 28 }}
              >
                {benchedVisuals.map((visual) => (
                  <div
                    key={visual.id}
                    style={chipStyle}
                    title={visual.name}
                    onClick={() => navigate(`/agents/${visual.id}`)}
                    onMouseEnter={(e) => {
                      e.currentTarget.style.background = 'rgba(255,255,255,0.09)'
                    }}
                    onMouseLeave={(e) => {
                      e.currentTarget.style.background = 'rgba(255,255,255,0.04)'
                    }}
                  >
                    <span
                      style={{
                        width: 7,
                        height: 7,
                        borderRadius: 'var(--radius-avatar)',
                        background: STATE_COLORS[visual.state],
                        flexShrink: 0,
                      }}
                    />
                    <span
                      style={{
                        color: 'var(--text-secondary)',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {visual.name}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* ─── 3D 场景 ─── */}
        <div
          style={{
            flex: 1,
            minHeight: 0,
            borderRadius: 'var(--radius-structure)',
            overflow: 'hidden',
            border: '1px solid var(--line)',
            background: 'var(--canvas)',
            position: 'relative',
          }}
        >
          <OfficeCanvas highQuality={highQuality} />

          {/* 聚焦信息条：单击 Agent 后浮出，提供跳转与取消聚焦 */}
          {focused && (
            <div
              style={{
                position: 'absolute',
                left: 12,
                right: 12,
                bottom: 12,
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                padding: '10px 14px',
                borderRadius: 'var(--radius-control)',
                background: 'rgba(12, 12, 22, 0.82)',
                border: '1px solid var(--line)',
                backdropFilter: 'var(--glass-blur)',
              }}
            >
              <span
                style={{
                  width: 9,
                  height: 9,
                  borderRadius: 'var(--radius-avatar)',
                  background: STATE_COLORS[focused.state],
                  flexShrink: 0,
                }}
              />
              <Text style={{ fontSize: 14, color: 'var(--text-primary)', fontWeight: 600 }}>
                {focused.name}
              </Text>
              <Text type="secondary" style={{ fontSize: 12, flexShrink: 0 }}>
                {ROLE_LABEL[focused.role] ?? focused.role}
              </Text>
              {focused.bubble && (
                <Text
                  style={{
                    fontSize: 12,
                    color: 'var(--text-tertiary)',
                    flex: 1,
                    minWidth: 0,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {focused.bubble}
                </Text>
              )}
              <div style={{ flex: focused.bubble ? undefined : 1 }} />
              <Button
                size="small"
                type="primary"
                onClick={() => navigate(`/agents/${focused.id}`)}
              >
                查看详情
              </Button>
              <Button size="small" onClick={() => setFocus(null)}>
                取消聚焦
              </Button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
