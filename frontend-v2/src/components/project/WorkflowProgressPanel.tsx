import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Skeleton, Tooltip, App } from 'antd'
import {
  CheckCircleOutlined,
  LoadingOutlined,
  ClockCircleOutlined,
  CloseCircleOutlined,
  EyeOutlined,
  MinusCircleOutlined,
} from '@ant-design/icons'
import { useWorkflowProgress } from '@/hooks/useProjects'
import { downloadProjectFile } from '@/api/projectFiles'
import { FileViewer } from '@/components/task/FileViewer'
import { BindOutputModal } from '@/components/project/BindOutputModal'
import type { WorkflowStepProgress } from '@/types'

interface Props {
  projectId: string
}

const statusConfig: Record<
  WorkflowStepProgress['status'],
  { color: string; bg: string; border: string; label: string; icon: React.ReactNode }
> = {
  done: { color: 'var(--success)', bg: 'rgba(16,185,129,0.14)', border: 'rgba(16,185,129,0.4)', label: '已完成', icon: <CheckCircleOutlined /> },
  in_progress: { color: 'var(--info)', bg: 'rgba(59,130,246,0.14)', border: 'rgba(59,130,246,0.5)', label: '进行中', icon: <LoadingOutlined /> },
  awaiting_review: { color: 'var(--warning)', bg: 'rgba(245,158,11,0.14)', border: 'rgba(245,158,11,0.5)', label: '待确认', icon: <ClockCircleOutlined /> },
  failed: { color: 'var(--error)', bg: 'rgba(239,68,68,0.14)', border: 'rgba(239,68,68,0.4)', label: '失败', icon: <CloseCircleOutlined /> },
  canceled: { color: 'var(--text-quaternary)', bg: 'rgba(107,114,128,0.14)', border: 'rgba(107,114,128,0.35)', label: '已取消', icon: <MinusCircleOutlined /> },
  pending: { color: 'var(--warning)', bg: 'rgba(245,158,11,0.10)', border: 'rgba(245,158,11,0.3)', label: '待开始', icon: <ClockCircleOutlined /> },
  unassigned: { color: 'var(--text-quaternary)', bg: 'rgba(100,116,139,0.08)', border: 'rgba(100,116,139,0.25)', label: '未编排', icon: <MinusCircleOutlined /> },
}

const rolePalette: { bg: string; color: string }[] = [
  { bg: 'linear-gradient(135deg, var(--signal), var(--signal))', color: 'var(--signal-hover)' },
  { bg: 'linear-gradient(135deg, var(--info), var(--cyan))', color: 'var(--info)' },
  { bg: 'linear-gradient(135deg, var(--warning), var(--error))', color: 'var(--warning)' },
  { bg: 'linear-gradient(135deg, var(--success), var(--cyan))', color: 'var(--success)' },
  { bg: 'linear-gradient(135deg, var(--error), var(--signal))', color: '#f9a8d4' },
  { bg: 'linear-gradient(135deg, var(--cyan), var(--info))', color: 'var(--cyan)' },
]

function getBadgeChar(name?: string, role?: string, stepName?: string): string {
  const src = (name || stepName || role || '?').trim()
  if (!src) return '?'
  const ch = src[0]
  if (/[\u4e00-\u9fa5]/.test(ch)) return ch
  return ch.toUpperCase()
}

function roleColor(role?: string, agentId?: string) {
  const key = agentId || role || 'x'
  let h = 0
  for (let i = 0; i < key.length; i++) h = (h * 31 + key.charCodeAt(i)) >>> 0
  return rolePalette[h % rolePalette.length]
}

/** 连接段最小宽度：步骤多到撑不下时靠它兜底，从而触发横向滚动 */
const MIN_CONNECTOR_WIDTH = 28
/** 单个节点胶囊最大宽度，避免超长步骤名把整条流程顶宽 */
const MAX_PILL_WIDTH = 208

/**
 * 连接段（节点 i → 节点 i+1）的填充比例与配色。
 * 填充端点天然落在节点圆心上，所以进度线与步骤永远是同一套坐标。
 */
function connectorFill(status: WorkflowStepProgress['status']): {
  pct: number
  bg: string
  animate: boolean
} {
  switch (status) {
    case 'done':
      return { pct: 100, bg: 'linear-gradient(90deg, var(--signal), var(--cyan))', animate: false }
    case 'awaiting_review':
      return { pct: 72, bg: 'linear-gradient(90deg, var(--signal), var(--warning))', animate: true }
    case 'in_progress':
      return { pct: 48, bg: 'linear-gradient(90deg, var(--signal), var(--cyan))', animate: true }
    case 'failed':
      return { pct: 100, bg: 'linear-gradient(90deg, var(--error), #b91c1c)', animate: false }
    case 'canceled':
      return { pct: 100, bg: 'var(--line-strong)', animate: false }
    default:
      return { pct: 0, bg: 'transparent', animate: false }
  }
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function fileTypeIcon(name?: string) {
  const ext = (name ?? '').toLowerCase()
  const style = { color: 'var(--text-tertiary)' }
  if (ext.endsWith('.pdf')) return <span style={{ ...style, color: '#f472b6' }}>PDF</span>
  if (/\.(png|jpe?g|gif|webp|svg|bmp)$/.test(ext)) return <span style={{ ...style, color: 'var(--success)' }}>IMG</span>
  if (/\.(md|markdown)$/.test(ext)) return <span style={{ ...style, color: 'var(--info)' }}>MD</span>
  return <span style={style}>FILE</span>
}

/**
 * 项目总流程进度条（常驻显示在项目详情 Tab 上方）：
 * 多行横向流（参考 CI 流水线节点样式），每个节点 = 角色圆形徽标 + 步骤名 + 状态图标；
 * 点击节点下方常驻展开该步最终产物，支持预览/下载。
 */
export function WorkflowProgressPanel({ projectId }: Props) {
  const { message } = App.useApp()
  const { data: progress, isLoading } = useWorkflowProgress(projectId)
  const [selected, setSelected] = useState<number | null>(null)
  const [preview, setPreview] = useState<{ blob: Blob; fileName: string } | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  // 手工绑定交付物：非 null 时打开弹窗并锁定该步骤
  const [bindStepIndex, setBindStepIndex] = useState<number | null>(null)
  const navigate = useNavigate()
  const flowRef = useRef<HTMLDivElement | null>(null)
  const [edge, setEdge] = useState({ left: false, right: false })

  const stepCount = progress?.steps?.length ?? 0
  // 当前焦点步骤：进行中/待确认优先，否则第一个未完成，全完成则停在末尾
  const activeIdx = useMemo(() => {
    const steps = progress?.steps ?? []
    const running = steps.findIndex((s) => s.status === 'in_progress' || s.status === 'awaiting_review')
    if (running >= 0) return running
    const undone = steps.findIndex((s) => s.status !== 'done')
    return undone >= 0 ? undone : Math.max(0, steps.length - 1)
  }, [progress])

  // 横向滚动状态 → 两侧渐隐遮罩（提示"还能滚"）
  useEffect(() => {
    const el = flowRef.current
    if (!el) return
    const sync = () => {
      const max = el.scrollWidth - el.clientWidth
      setEdge({ left: el.scrollLeft > 1, right: max > 1 && el.scrollLeft < max - 1 })
    }
    sync()
    el.addEventListener('scroll', sync, { passive: true })
    const ro = new ResizeObserver(sync)
    ro.observe(el)
    return () => {
      el.removeEventListener('scroll', sync)
      ro.disconnect()
    }
  }, [stepCount])

  // 步骤太多时，把当前步骤滚到可视区中间
  useEffect(() => {
    const el = flowRef.current
    if (!el) return
    const node = el.querySelector<HTMLElement>(`[data-step-index="${activeIdx}"]`)
    if (!node) return
    if (el.scrollWidth <= el.clientWidth + 1) return
    const left = node.offsetLeft - (el.clientWidth - node.offsetWidth) / 2
    const max = el.scrollWidth - el.clientWidth
    el.scrollTo({ left: Math.max(0, Math.min(left, max)), behavior: 'smooth' })
  }, [activeIdx, stepCount])

  // 点击节点 = 展开产出预览；该步骤已绑定任务时同时打开任务工作台（?task= 深链）
  const handleStepClick = (i: number) => {
    const step = progress?.steps?.[i]
    if (!step) return
    const nextSelected = selected === i ? null : i
    setSelected(nextSelected)
    if (nextSelected !== null && step.task_id) {
      navigate(`/projects/${projectId}?task=${step.task_id}`)
    }
  }

  if (isLoading) {
    return (
      <div style={{ padding: '6px 0' }}>
        <Skeleton active title={false} paragraph={{ rows: 1 }} />
      </div>
    )
  }
  if (!progress || !progress.steps || progress.steps.length === 0) {
    return null
  }

  const selectedStep = selected !== null ? progress.steps[selected] : null
  const doneCount = progress.steps.filter((s) => s.status === 'done').length
  const failedCount = progress.steps.filter((s) => s.status === 'failed').length
  const totalCount = progress.steps.length
  const pct = Math.round((doneCount / totalCount) * 100)

  const handlePreview = async (_step: WorkflowStepProgress, fileId: string, fileName: string) => {
    try {
      const blob = await downloadProjectFile(projectId, fileId)
      setPreview({ blob, fileName })
      setPreviewOpen(true)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '预览失败')
    }
  }

  return (
    <div
      style={{
        width: '100%',
        minWidth: 0,
        maxWidth: '100%',
        boxSizing: 'border-box',
        borderRadius: 'var(--radius-control)',
        border: '1px solid rgba(139,127,248,0.2)',
        background: 'var(--surface)',
        padding: '12px 14px',
        display: 'flex',
        flexDirection: 'column',
        gap: 10,
        overflow: 'hidden',
      }}
    >
      <style>{`
        @keyframes wp-pulse2 {
          0%, 100% { opacity: 1; transform: scale(1); }
          50% { opacity: 0.4; transform: scale(0.82); }
        }
        @keyframes wp-flow-slide {
          0% { background-position: 0% 50%; }
          100% { background-position: 200% 50%; }
        }
      `}</style>

      {/* 完成度读数（进度条本体已并入下方节点连接段，保证与步骤对齐） */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
          <span style={{ color: 'var(--signal-hover)', fontWeight: 600 }}>{doneCount}</span>/{totalCount} 完成
        </span>
        {failedCount > 0 && <span style={{ fontSize: 12, color: 'var(--error)' }}>· {failedCount} 个失败</span>}
        <span style={{ flex: 1 }} />
        <span style={{ fontSize: 12, color: 'var(--text-tertiary)', flexShrink: 0 }}>{pct}%</span>
      </div>

      {/* 节点流：连接段 flex:1 撑满剩余宽度；步骤过多时连接段收到 MIN_CONNECTOR_WIDTH 兜底并触发横向滚动。
          进度条即连接段本身，填充端点天然落在节点圆心，与步骤严格对齐。 */}
      <div style={{ position: 'relative', width: '100%', minWidth: 0 }}>
        <div
          ref={flowRef}
          className="wp-flow-scroll"
          style={{
            position: 'relative',
            display: 'flex',
            flexWrap: 'nowrap',
            alignItems: 'center',
            gap: 0,
            overflowX: 'auto',
            overflowY: 'hidden',
            paddingBottom: 6,
            width: '100%',
            minWidth: 0,
          }}
        >
          {progress.steps.map((step, i) => {
            const cfg = statusConfig[step.status] ?? statusConfig.unassigned
            const isSelected = selected === i
            const isDone = step.status === 'done'
            // 已完成节点灰度展示，未完成保持状态色
            const nodeColor = isDone ? 'var(--text-quaternary)' : cfg.color
            const nodeBorder = isDone ? 'var(--line-strong)' : cfg.border
            const nodeBg = isDone ? 'var(--surface-raised)' : isSelected ? cfg.bg : 'var(--surface)'
            const nodeText = isDone ? 'var(--text-quaternary)' : 'var(--text-primary)'
            const rc = isDone
              ? { bg: 'linear-gradient(135deg, var(--text-quaternary), var(--text-quaternary))', color: 'var(--text-tertiary)' }
              : roleColor(step.role, step.agent_id)
            const badge = getBadgeChar(step.task_title, step.role, step.name)
            const fill = connectorFill(step.status)
            return (
              <Fragment key={step.index}>
                <div data-step-index={i} style={{ flex: '0 0 auto', display: 'flex', alignItems: 'center', minWidth: 0 }}>
                  <Tooltip title={`${step.name} · ${cfg.label}${step.task_title ? `（${step.task_title}）` : ''}`}>
                    <button
                      type="button"
                      onClick={() => handleStepClick(i)}
                      style={{
                        display: 'inline-flex',
                        alignItems: 'center',
                        gap: 8,
                        maxWidth: MAX_PILL_WIDTH,
                        padding: '6px 12px 6px 6px',
                        borderRadius: 'var(--radius-pill)',
                        border: `1px solid ${isSelected ? nodeColor : nodeBorder}`,
                        background: nodeBg,
                        boxShadow: isSelected && !isDone ? `0 0 14px ${cfg.color}35` : 'none',
                        cursor: 'pointer',
                        fontFamily: 'inherit',
                        transition: 'border-color 0.15s, box-shadow 0.15s, background 0.15s',
                      }}
                    >
                      <span
                        style={{
                          width: 26,
                          height: 26,
                          borderRadius: 'var(--radius-avatar)',
                          background: rc.bg,
                          color: isDone ? 'var(--text-tertiary)' : '#fff',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          fontSize: 12,
                          fontWeight: 600,
                          flexShrink: 0,
                        }}
                      >
                        {badge}
                      </span>
                      <span
                        style={{
                          fontSize: 12.5,
                          color: nodeText,
                          fontWeight: 500,
                          whiteSpace: 'nowrap',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          minWidth: 0,
                        }}
                      >
                        {step.name}
                      </span>
                      <span
                        style={{
                          color: nodeColor,
                          fontSize: 13,
                          display: 'inline-flex',
                          alignItems: 'center',
                          animation: step.status === 'in_progress' ? 'wp-pulse2 1.8s ease-in-out infinite' : 'none',
                        }}
                      >
                        {cfg.icon}
                      </span>
                    </button>
                  </Tooltip>
                </div>
                {i < progress.steps.length - 1 && (
                  <div
                    style={{
                      flex: `1 1 ${MIN_CONNECTOR_WIDTH}px`,
                      minWidth: MIN_CONNECTOR_WIDTH,
                      height: 2,
                      borderRadius: 'var(--radius-pill)',
                      background: 'var(--surface-raised)',
                      overflow: 'hidden',
                    }}
                  >
                    <span
                      style={{
                        display: 'block',
                        height: '100%',
                        width: `${fill.pct}%`,
                        background: fill.bg,
                        backgroundSize: fill.animate ? '200% 100%' : undefined,
                        animation: fill.animate ? 'wp-flow-slide 2.4s linear infinite' : 'none',
                        borderRadius: 'var(--radius-pill)',
                        transition: 'width 0.4s ease',
                      }}
                    />
                  </div>
                )}
              </Fragment>
            )
          })}
        </div>

        {/* 两侧渐隐：提示横向还能继续滚 */}
        {edge.left && (
          <div
            style={{
              position: 'absolute',
              top: 0,
              bottom: 6,
              left: 0,
              width: 28,
              pointerEvents: 'none',
              background: 'linear-gradient(90deg, var(--surface), transparent)',
            }}
          />
        )}
        {edge.right && (
          <div
            style={{
              position: 'absolute',
              top: 0,
              bottom: 6,
              right: 0,
              width: 28,
              pointerEvents: 'none',
              background: 'linear-gradient(270deg, var(--surface), transparent)',
            }}
          />
        )}
      </div>

      {/* 选中节点产物区（常驻展开） */}
      {selectedStep && (
        <div
          style={{
            borderRadius: 'var(--radius-control)',
            border: '1px solid rgba(139,127,248,0.2)',
            background: 'var(--surface-sunken)',
            padding: '8px 12px',
            display: 'flex',
            flexDirection: 'column',
            gap: 3,
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>
              步骤 {selectedStep.index + 1} · {selectedStep.name} · 最终产物
            </span>
            <span style={{ flex: 1 }} />
            {/* 从步骤视角绑定：允许在这里挑项目里的任意文件（含手工上传的） */}
            <button
              type="button"
              disabled={selectedStep.status === 'unassigned' || !selectedStep.task_id}
              onClick={() => setBindStepIndex(selectedStep.index)}
              style={{
                fontSize: 12,
                lineHeight: '20px',
                padding: '1px 10px',
                borderRadius: 'var(--radius-pill)',
                border: '1px solid rgba(139,127,248,0.4)',
                background: 'rgba(139,127,248,0.12)',
                color: 'var(--signal-hover)',
                cursor: selectedStep.status === 'unassigned' || !selectedStep.task_id ? 'not-allowed' : 'pointer',
                opacity: selectedStep.status === 'unassigned' || !selectedStep.task_id ? 0.45 : 1,
                fontFamily: 'inherit',
                flexShrink: 0,
              }}
            >
              + 绑定交付物
            </button>
          </div>
          {selectedStep.outputs && selectedStep.outputs.length > 0 ? (
            selectedStep.outputs.map((o, oi) => (
              <div key={oi} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, padding: '2px 4px' }}>
                <span
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: 10,
                    fontWeight: 600,
                    minWidth: 32,
                    height: 18,
                    borderRadius: 'var(--radius-control)',
                    background: 'var(--surface-raised)',
                    color: 'var(--text-tertiary)',
                  }}
                >
                  {fileTypeIcon(o.file_name || o.output_name)}
                </span>
                <span style={{ color: 'var(--text-primary)', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {o.file_name || o.output_name || '未命名产物'}
                </span>
                <span style={{ color: 'var(--text-quaternary)', flexShrink: 0 }}>
                  {o.mime_type || ''} {o.file_size != null ? ` · ${formatFileSize(o.file_size)}` : ''}
                </span>
                <span style={{ flex: 1 }} />
                {o.file_id ? (
                  <Tooltip title="预览">
                    <button
                      type="button"
                      style={{ border: 'none', background: 'transparent', color: 'var(--info)', cursor: 'pointer', fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 3, padding: 0 }}
                      onClick={() => void handlePreview(selectedStep, o.file_id!, o.file_name || o.output_name || '产物')}
                    >
                      <EyeOutlined /> 预览
                    </button>
                  </Tooltip>
                ) : (
                  <span style={{ color: 'var(--text-quaternary)', fontSize: 11 }}>暂无预览文件</span>
                )}
              </div>
            ))
          ) : (
            <span style={{ fontSize: 12, color: 'var(--text-quaternary)' }}>
              {selectedStep.status === 'unassigned' ? '该步骤尚未绑定任务' : '该步骤暂无已提交的产物'}
            </span>
          )}
        </div>
      )}

      <FileViewer
        open={previewOpen}
        onOpenChange={setPreviewOpen}
        blob={preview?.blob ?? null}
        fileName={preview?.fileName ?? ''}
      />

      {/* 手工绑定交付物：任意文件 → 该步骤的输出位 */}
      <BindOutputModal
        open={bindStepIndex !== null}
        onOpenChange={(o) => !o && setBindStepIndex(null)}
        projectId={projectId}
        file={null}
        pickFile
        lockedStepIndex={bindStepIndex ?? undefined}
      />
    </div>
  )
}
