import { useMemo, useState } from 'react'
import { Button, App, Card, Modal, Select, Spin } from 'antd'
import {
  CheckCircleOutlined,
  DownloadOutlined,
  EyeOutlined,
  FileTextOutlined,
} from '@ant-design/icons'
import { getTaskArtifactContent } from '@/api/tasks'
import { useBindArtifactOutput } from '@/hooks/useTasks'
import { Markdown } from './Markdown'
import { FileViewer } from './FileViewer'
import type { TaskArtifact, TaskResult, Todo, Workflow } from '@/types'

interface TaskResultViewProps {
  taskId: string
  result: TaskResult
  artifacts: TaskArtifact[]
  /** 用于推导出每个 artifact 所属 todo 的工作流步骤声明输出位；不传则隐藏绑定按钮 */
  workflow?: Workflow | null
  todos?: Todo[]
}

function downloadBlob(blob: Blob, fileName: string) {
  const objectURL = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = objectURL
  anchor.download = fileName
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  window.setTimeout(() => URL.revokeObjectURL(objectURL), 60_000)
}

function formatFileSize(bytes: number) {
  if (!bytes || bytes <= 0) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function TaskResultView({ taskId, result, artifacts, workflow, todos }: TaskResultViewProps) {
  const { message } = App.useApp()
  const safeArtifacts = artifacts ?? []
  const safeTodos = todos ?? []
  const summaryText = result?.summary ?? ''
  const finalOutputText = result?.final_output ?? ''
  const hasResult = summaryText || finalOutputText
  const hasArtifacts = safeArtifacts.length > 0
  const [loadingArtifactId, setLoadingArtifactId] = useState<string | null>(null)
  const [downloadingArtifactId, setDownloadingArtifactId] = useState<string | null>(null)
  const [viewerBlob, setViewerBlob] = useState<Blob | null>(null)
  const [viewerFileName, setViewerFileName] = useState('')
  const [viewerOpen, setViewerOpen] = useState(false)

  // 绑定状态：被点击的 artifact + Modal 是否打开
  const [bindArtifact, setBindArtifact] = useState<TaskArtifact | null>(null)
  const [bindOutputName, setBindOutputName] = useState<string | undefined>(undefined)
  const bindMutation = useBindArtifactOutput()

  // 计算每个 todo 对应的步骤声明输出位。匹配规则与后端 webhook 一致：
  // todo.order 优先，回退到 todos 数组下标。空数组 = 该步骤没声明输出位 → 隐藏绑定按钮。
  const declaredByTodoId = useMemo(() => {
    const map = new Map<string, { stepName: string; slots: { name: string; description?: string }[] }>()
    if (!workflow?.steps?.length || !safeTodos.length) return map
    safeTodos.forEach((td, idx) => {
      const stepIdx = typeof td.order === 'number' && td.order >= 0 && td.order < workflow.steps.length
        ? td.order
        : idx
      if (stepIdx < 0 || stepIdx >= workflow.steps.length) return
      const step = workflow.steps[stepIdx]
      const slots = (step.outputs ?? [])
        .filter((o) => !!o.name)
        .map((o) => ({ name: o.name, description: o.description }))
      if (slots.length === 0) return
      // 已被占用的输出位不显示（同一 todo 多次绑同一个位会被后端拒）
      const used = new Set((td.outputs ?? []).map((o) => o.output_name).filter(Boolean))
      const available = slots.filter((s) => !used.has(s.name))
      if (available.length === 0) return
      map.set(td.id, { stepName: step.name, slots: available })
    })
    return map
  }, [workflow, safeTodos])

  if (!hasResult && !hasArtifacts) {
    return <div style={{ padding: 32, textAlign: 'center', color: 'var(--text-quaternary)', fontSize: 13 }}>任务尚未产出结果</div>
  }

  const handlePreview = async (artifact: TaskArtifact) => {
    setLoadingArtifactId(artifact.transfer_id)
    try {
      const blob = await getTaskArtifactContent(taskId, artifact.transfer_id)
      setViewerBlob(blob)
      setViewerFileName(artifact.file_name)
      setViewerOpen(true)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '打开文件失败')
    } finally {
      setLoadingArtifactId(null)
    }
  }

  const handleDownload = async (artifact: TaskArtifact) => {
    setDownloadingArtifactId(artifact.transfer_id)
    try {
      const blob = await getTaskArtifactContent(taskId, artifact.transfer_id)
      downloadBlob(blob, artifact.file_name)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '下载文件失败')
    } finally {
      setDownloadingArtifactId(null)
    }
  }

  const openBindModal = (artifact: TaskArtifact) => {
    setBindArtifact(artifact)
    setBindOutputName(undefined)
  }

  const closeBindModal = () => {
    if (bindMutation.isPending) return
    setBindArtifact(null)
    setBindOutputName(undefined)
  }

  const submitBind = async () => {
    if (!bindArtifact || !bindOutputName) return
    const todoId = bindArtifact.todo_id
    if (!todoId) {
      message.error('该文件未关联 todo，无法绑定（后端仅支持按 todo 维度绑定）')
      return
    }
    try {
      const res = await bindMutation.mutateAsync({
        taskId,
        todoId,
        input: { artifact_id: bindArtifact.transfer_id, output_name: bindOutputName },
      })
      const updated = res.data
      message.success(`已绑定到「${updated.output_name}」`)
      closeBindModal()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '绑定失败')
    }
  }

  const bindInfo = bindArtifact?.todo_id ? declaredByTodoId.get(bindArtifact.todo_id) : null

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      {hasResult && (
        <Card
          style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}
          styles={{ body: { padding: 16 } }}
        >
          {summaryText && (
            <div style={{ marginBottom: finalOutputText ? 14 : 0 }}>
              <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 6 }}>摘要</div>
              <div style={{ fontSize: 13, color: 'var(--text-secondary)', whiteSpace: 'pre-wrap' }}>{summaryText}</div>
            </div>
          )}
          {finalOutputText && (
            <div>
              <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 6 }}>最终产出</div>
              <div style={{ background: 'var(--surface-inset)', borderRadius: 'var(--radius-control)', padding: 14 }}>
                <Markdown content={finalOutputText} />
              </div>
            </div>
          )}
        </Card>
      )}

      {hasArtifacts && (
        <div>
          <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 8 }}>交付物</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {safeArtifacts.map((artifact) => {
              const bindable =
                artifact.kind === 'process' && !!artifact.todo_id && declaredByTodoId.has(artifact.todo_id)
              return (
                <div
                  key={artifact.transfer_id}
                  style={{
                    borderRadius: 'var(--radius-control)',
                    border: '1px solid var(--line)',
                    background: 'var(--surface)',
                    padding: 12,
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                    <div style={{ width: 32, height: 32, borderRadius: 'var(--radius-control)', background: 'var(--surface-raised)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                      <FileTextOutlined style={{ color: 'var(--text-tertiary)' }} />
                    </div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontSize: 13, color: 'var(--text-primary)', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', display: 'flex', alignItems: 'center', gap: 6 }}>
                        <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{artifact.file_name}</span>
                        {artifact.kind === 'deliverable' && (
                          <span style={{ flexShrink: 0, fontSize: 11, lineHeight: '18px', padding: '0 6px', borderRadius: 'var(--radius-control)', background: 'rgba(99,153,34,0.18)', color: '#97c459', border: '0.5px solid rgba(99,153,34,0.4)' }}>
                            交付{artifact.output_name ? ` · ${artifact.output_name}` : ''}
                          </span>
                        )}
                        {artifact.kind === 'process' && (
                          <span style={{ flexShrink: 0, fontSize: 11, lineHeight: '18px', padding: '0 6px', borderRadius: 'var(--radius-control)', background: 'var(--surface-raised)', color: 'var(--text-tertiary)', border: '0.5px solid var(--line-strong)' }}>
                            过程
                          </span>
                        )}
                      </div>
                      <div style={{ fontSize: 13, color: 'var(--text-quaternary)', marginTop: 2 }}>
                        {artifact.mime_type}{formatFileSize(artifact.file_size) ? ` · ${formatFileSize(artifact.file_size)}` : ''}
                      </div>
                    </div>
                    <div style={{ display: 'flex', gap: 6 }}>
                      {bindable && (
                        <Button
                          size="small"
                          icon={<CheckCircleOutlined />}
                          onClick={() => openBindModal(artifact)}
                        >
                          绑定为交付物
                        </Button>
                      )}
                      <Button
                        size="small"
                        icon={loadingArtifactId === artifact.transfer_id ? <Spin size="small" /> : <EyeOutlined />}
                        onClick={() => handlePreview(artifact)}
                        disabled={loadingArtifactId === artifact.transfer_id}
                      >
                        预览
                      </Button>
                      <Button
                        size="small"
                        icon={downloadingArtifactId === artifact.transfer_id ? <Spin size="small" /> : <DownloadOutlined />}
                        onClick={() => handleDownload(artifact)}
                        disabled={downloadingArtifactId === artifact.transfer_id}
                      >
                        下载
                      </Button>
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}

      <FileViewer
        open={viewerOpen}
        onOpenChange={setViewerOpen}
        blob={viewerBlob}
        fileName={viewerFileName}
        onDownload={() => viewerBlob && viewerFileName && downloadBlob(viewerBlob, viewerFileName)}
      />

      <Modal
        title="绑定为交付物"
        open={!!bindArtifact}
        onCancel={closeBindModal}
        onOk={submitBind}
        okText="绑定"
        cancelText="取消"
        confirmLoading={bindMutation.isPending}
        destroyOnClose
        okButtonProps={{ disabled: !bindOutputName }}
      >
        {bindArtifact && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12, paddingTop: 8 }}>
            <div style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
              文件：<span style={{ color: 'var(--text-primary)' }}>{bindArtifact.file_name}</span>
            </div>
            <div style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
              所属步骤：<span style={{ color: 'var(--text-primary)' }}>{bindInfo?.stepName ?? '—'}</span>
            </div>
            <div style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
              选择该步骤的输出位（占位名请原样保留，不要替换）：
            </div>
            <Select
              placeholder="选择工作流输出位"
              value={bindOutputName}
              onChange={setBindOutputName}
              style={{ width: '100%' }}
              options={(bindInfo?.slots ?? []).map((s) => ({
                value: s.name,
                label: s.description ? `${s.name}  ·  ${s.description}` : s.name,
              }))}
              notFoundContent="该步骤未声明输出位，或所有输出位已被绑定"
            />
          </div>
        )}
      </Modal>
    </div>
  )
}