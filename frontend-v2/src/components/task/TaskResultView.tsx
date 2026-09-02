import { useState } from 'react'
import { Button, App, Card, Spin } from 'antd'
import { DownloadOutlined, EyeOutlined, FileTextOutlined } from '@ant-design/icons'
import { getTaskArtifactContent } from '@/api/tasks'
import { Markdown } from './Markdown'
import { FileViewer } from './FileViewer'
import type { TaskArtifact, TaskResult } from '@/types'

interface TaskResultViewProps {
  taskId: string
  result: TaskResult
  artifacts: TaskArtifact[]
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

export function TaskResultView({ taskId, result, artifacts }: TaskResultViewProps) {
  const { message } = App.useApp()
  const safeArtifacts = artifacts ?? []
  const summaryText = result?.summary ?? ''
  const finalOutputText = result?.final_output ?? ''
  const hasResult = summaryText || finalOutputText
  const hasArtifacts = safeArtifacts.length > 0
  const [loadingArtifactId, setLoadingArtifactId] = useState<string | null>(null)
  const [downloadingArtifactId, setDownloadingArtifactId] = useState<string | null>(null)
  const [viewerBlob, setViewerBlob] = useState<Blob | null>(null)
  const [viewerFileName, setViewerFileName] = useState('')
  const [viewerOpen, setViewerOpen] = useState(false)

  if (!hasResult && !hasArtifacts) {
    return <div style={{ padding: 32, textAlign: 'center', color: 'rgba(255,255,255,0.4)', fontSize: 13 }}>任务尚未产出结果</div>
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

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      {hasResult && (
        <Card
          style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.08)' }}
          styles={{ body: { padding: 16 } }}
        >
          {summaryText && (
            <div style={{ marginBottom: finalOutputText ? 14 : 0 }}>
              <div style={{ fontSize: 13, fontWeight: 600, color: 'rgba(255,255,255,0.8)', marginBottom: 6 }}>摘要</div>
              <div style={{ fontSize: 13, color: 'rgba(255,255,255,0.6)', whiteSpace: 'pre-wrap' }}>{summaryText}</div>
            </div>
          )}
          {finalOutputText && (
            <div>
              <div style={{ fontSize: 13, fontWeight: 600, color: 'rgba(255,255,255,0.8)', marginBottom: 6 }}>最终产出</div>
              <div style={{ background: 'rgba(0,0,0,0.3)', borderRadius: 10, padding: 14 }}>
                <Markdown content={finalOutputText} />
              </div>
            </div>
          )}
        </Card>
      )}

      {hasArtifacts && (
        <div>
          <div style={{ fontSize: 13, fontWeight: 600, color: 'rgba(255,255,255,0.8)', marginBottom: 8 }}>交付物</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {safeArtifacts.map((artifact) => (
              <div
                key={artifact.transfer_id}
                style={{
                  borderRadius: 12,
                  border: '1px solid rgba(255,255,255,0.08)',
                  background: 'rgba(255,255,255,0.03)',
                  padding: 12,
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                  <div style={{ width: 32, height: 32, borderRadius: 8, background: 'rgba(255,255,255,0.06)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                    <FileTextOutlined style={{ color: 'rgba(255,255,255,0.5)' }} />
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: 13, color: '#fff', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', display: 'flex', alignItems: 'center', gap: 6 }}>
                      <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{artifact.file_name}</span>
                      {artifact.kind === 'deliverable' && (
                        <span style={{ flexShrink: 0, fontSize: 11, lineHeight: '18px', padding: '0 6px', borderRadius: 4, background: 'rgba(99,153,34,0.18)', color: '#97c459', border: '0.5px solid rgba(99,153,34,0.4)' }}>
                          交付{artifact.output_name ? ` · ${artifact.output_name}` : ''}
                        </span>
                      )}
                      {artifact.kind === 'process' && (
                        <span style={{ flexShrink: 0, fontSize: 11, lineHeight: '18px', padding: '0 6px', borderRadius: 4, background: 'rgba(255,255,255,0.06)', color: 'rgba(255,255,255,0.45)', border: '0.5px solid rgba(255,255,255,0.12)' }}>
                          过程
                        </span>
                      )}
                    </div>
                    <div style={{ fontSize: 13, color: 'rgba(255,255,255,0.4)', marginTop: 2 }}>
                      {artifact.mime_type}{formatFileSize(artifact.file_size) ? ` · ${formatFileSize(artifact.file_size)}` : ''}
                    </div>
                  </div>
                  <div style={{ display: 'flex', gap: 6 }}>
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
            ))}
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
    </div>
  )
}
