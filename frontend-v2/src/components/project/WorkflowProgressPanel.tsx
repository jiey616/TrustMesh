import { useState } from 'react'
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
import type { WorkflowStepProgress } from '@/types'

interface Props {
  projectId: string
}

const statusConfig: Record<
  WorkflowStepProgress['status'],
  { color: string; bg: string; border: string; label: string; icon: React.ReactNode }
> = {
  done: { color: '#10b981', bg: 'rgba(16,185,129,0.14)', border: 'rgba(16,185,129,0.4)', label: '已完成', icon: <CheckCircleOutlined /> },
  in_progress: { color: '#3b82f6', bg: 'rgba(59,130,246,0.14)', border: 'rgba(59,130,246,0.5)', label: '进行中', icon: <LoadingOutlined /> },
  awaiting_review: { color: '#f59e0b', bg: 'rgba(245,158,11,0.14)', border: 'rgba(245,158,11,0.5)', label: '待确认', icon: <ClockCircleOutlined /> },
  failed: { color: '#ef4444', bg: 'rgba(239,68,68,0.14)', border: 'rgba(239,68,68,0.4)', label: '失败', icon: <CloseCircleOutlined /> },
  canceled: { color: '#6b7280', bg: 'rgba(107,114,128,0.14)', border: 'rgba(107,114,128,0.35)', label: '已取消', icon: <MinusCircleOutlined /> },
  pending: { color: '#f59e0b', bg: 'rgba(245,158,11,0.10)', border: 'rgba(245,158,11,0.3)', label: '待开始', icon: <ClockCircleOutlined /> },
  unassigned: { color: '#64748b', bg: 'rgba(100,116,139,0.08)', border: 'rgba(100,116,139,0.25)', label: '未编排', icon: <MinusCircleOutlined /> },
}

const rolePalette: { bg: string; color: string }[] = [
  { bg: 'linear-gradient(135deg, #6d5ff5, #6366f1)', color: '#a99cff' },
  { bg: 'linear-gradient(135deg, #3b82f6, #22d3ee)', color: '#7dd3fc' },
  { bg: 'linear-gradient(135deg, #f59e0b, #f43f5e)', color: '#fcd34d' },
  { bg: 'linear-gradient(135deg, #10b981, #22d3ee)', color: '#6ee7b7' },
  { bg: 'linear-gradient(135deg, #ec4899, #a855f7)', color: '#f9a8d4' },
  { bg: 'linear-gradient(135deg, #06b6d4, #3b82f6)', color: '#67e8f9' },
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

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function fileTypeIcon(name?: string) {
  const ext = (name ?? '').toLowerCase()
  const style = { color: '#94a3b8' }
  if (ext.endsWith('.pdf')) return <span style={{ ...style, color: '#f472b6' }}>PDF</span>
  if (/\.(png|jpe?g|gif|webp|svg|bmp)$/.test(ext)) return <span style={{ ...style, color: '#34d399' }}>IMG</span>
  if (/\.(md|markdown)$/.test(ext)) return <span style={{ ...style, color: '#60a5fa' }}>MD</span>
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
        borderRadius: 12,
        border: '1px solid rgba(139,127,248,0.2)',
        background: 'linear-gradient(160deg, rgba(24,24,32,0.6), rgba(16,16,24,0.5))',
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
      `}</style>

      {/* 顶部完成度小条 */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <span style={{ fontSize: 12, color: 'rgba(255,255,255,0.6)' }}>
          <span style={{ color: '#a99cff', fontWeight: 600 }}>{doneCount}</span>/{totalCount} 完成
        </span>
        <span
          style={{
            flex: 1,
            height: 3,
            borderRadius: 999,
            background: 'rgba(255,255,255,0.08)',
            overflow: 'hidden',
          }}
        >
          <span
            style={{
              display: 'block',
              height: '100%',
              width: `${pct}%`,
              background: 'linear-gradient(90deg, #6d5ff5, #22d3ee)',
              borderRadius: 999,
              transition: 'width 0.4s ease',
            }}
          />
        </span>
      </div>

      {/* 节点流（单行横向滚动，已完成灰度） */}
      <div className="wp-flow-scroll" style={{ display: 'flex', flexWrap: 'nowrap', alignItems: 'center', gap: 8, overflowX: 'auto', overflowY: 'hidden', paddingBottom: 6, width: '100%', minWidth: 0 }}>
        {progress.steps.map((step, i) => {
          const cfg = statusConfig[step.status] ?? statusConfig.unassigned
          const isSelected = selected === i
          const isDone = step.status === 'done'
          // 已完成节点灰度展示，未完成保持状态色
          const nodeColor = isDone ? '#6b7280' : cfg.color
          const nodeBorder = isDone ? 'rgba(107,114,128,0.3)' : cfg.border
          const nodeBg = isDone ? 'rgba(107,114,128,0.06)' : isSelected ? cfg.bg : 'rgba(255,255,255,0.03)'
          const nodeText = isDone ? 'rgba(255,255,255,0.35)' : '#f4f4f8'
          const rc = isDone
            ? { bg: 'linear-gradient(135deg, #6b7280, #4b5563)', color: '#9ca3af' }
            : roleColor(step.role, step.agent_id)
          const badge = getBadgeChar(step.task_title, step.role, step.name)
          return (
            <div key={step.index} style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
              <Tooltip title={`${step.name} · ${cfg.label}${step.task_title ? `（${step.task_title}）` : ''}`}>
                <button
                  type="button"
                  onClick={() => setSelected(isSelected ? null : i)}
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 8,
                    padding: '6px 12px 6px 6px',
                    borderRadius: 999,
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
                      borderRadius: '50%',
                      background: rc.bg,
                      color: isDone ? '#9ca3af' : '#fff',
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
                  <span style={{ fontSize: 12.5, color: nodeText, fontWeight: 500, whiteSpace: 'nowrap' }}>{step.name}</span>
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
              {i < progress.steps.length - 1 && (
                <span style={{ color: isDone ? 'rgba(107,114,128,0.4)' : 'rgba(139,127,248,0.5)', fontSize: 14, userSelect: 'none', flexShrink: 0 }}>→</span>
              )}
            </div>
          )
        })}
      </div>

      {/* 选中节点产物区（常驻展开） */}
      {selectedStep && (
        <div
          style={{
            borderRadius: 10,
            border: '1px solid rgba(139,127,248,0.2)',
            background: 'rgba(255,255,255,0.02)',
            padding: '8px 12px',
            display: 'flex',
            flexDirection: 'column',
            gap: 3,
          }}
        >
          <div style={{ fontSize: 12, color: 'rgba(255,255,255,0.5)' }}>
            步骤 {selectedStep.index + 1} · {selectedStep.name} · 最终产物
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
                    borderRadius: 4,
                    background: 'rgba(255,255,255,0.06)',
                    color: '#94a3b8',
                  }}
                >
                  {fileTypeIcon(o.file_name || o.output_name)}
                </span>
                <span style={{ color: 'rgba(255,255,255,0.85)', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {o.file_name || o.output_name || '未命名产物'}
                </span>
                <span style={{ color: 'rgba(255,255,255,0.35)', flexShrink: 0 }}>
                  {o.mime_type || ''} {o.file_size != null ? ` · ${formatFileSize(o.file_size)}` : ''}
                </span>
                <span style={{ flex: 1 }} />
                {o.file_id ? (
                  <Tooltip title="预览">
                    <button
                      type="button"
                      style={{ border: 'none', background: 'transparent', color: '#60a5fa', cursor: 'pointer', fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 3, padding: 0 }}
                      onClick={() => void handlePreview(selectedStep, o.file_id!, o.file_name || o.output_name || '产物')}
                    >
                      <EyeOutlined /> 预览
                    </button>
                  </Tooltip>
                ) : (
                  <span style={{ color: 'rgba(255,255,255,0.3)', fontSize: 11 }}>暂无预览文件</span>
                )}
              </div>
            ))
          ) : (
            <span style={{ fontSize: 12, color: 'rgba(255,255,255,0.35)' }}>
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
    </div>
  )
}
