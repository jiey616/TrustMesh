import { useState } from 'react'
import { Download, Eye, FileText, Loader2 } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { ApiRequestError } from '@/api/client'
import { getTaskArtifactContent } from '@/api/tasks'
import { cn } from '@/lib/utils'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { toast } from 'sonner'
import { FileViewer } from '@/components/task/FileViewer'
import { normalizeEscapedText } from '@/lib/utils'
import type { TaskResult, TaskArtifact } from '@/types'

interface TaskResultViewProps {
  taskId: string
  result: TaskResult
  artifacts: TaskArtifact[]
}

export function TaskResultView({ taskId, result, artifacts }: TaskResultViewProps) {
  const safeArtifacts = artifacts ?? []
  const summaryText = normalizeEscapedText(result.summary)
  const finalOutputText = normalizeEscapedText(result.final_output, { preserveMarkdownCode: true })
  const hasResult = summaryText || finalOutputText
  const hasArtifacts = safeArtifacts.length > 0
  const [loadingArtifactId, setLoadingArtifactId] = useState<string | null>(null)
  const [downloadingArtifactId, setDownloadingArtifactId] = useState<string | null>(null)
  const [transferErrors, setTransferErrors] = useState<Record<string, string>>({})
  const [viewerBlob, setViewerBlob] = useState<Blob | null>(null)
  const [viewerFileName, setViewerFileName] = useState('')
  const [viewerOpen, setViewerOpen] = useState(false)

  if (!hasResult && !hasArtifacts) {
    return <p className="py-8 text-center text-sm text-muted-foreground">任务尚未产出结果</p>
  }

  const handlePreview = async (artifact: TaskArtifact) => {
    setLoadingArtifactId(artifact.transfer_id)
    setTransferErrors((current) => ({ ...current, [artifact.transfer_id]: '' }))
    try {
      const fileBlob = await getTaskArtifactContent(taskId, artifact.transfer_id)
      setViewerBlob(fileBlob)
      setViewerFileName(artifact.file_name)
      setViewerOpen(true)
    } catch (err) {
      const message = err instanceof ApiRequestError ? err.message : '打开文件失败'
      toast.error(message)
      setTransferErrors((current) => ({ ...current, [artifact.transfer_id]: message }))
    } finally {
      setLoadingArtifactId(null)
    }
  }

  const handleDownloadArtifact = async (artifact: TaskArtifact) => {
    setDownloadingArtifactId(artifact.transfer_id)
    setTransferErrors((current) => ({ ...current, [artifact.transfer_id]: '' }))
    try {
      const fileBlob = await getTaskArtifactContent(taskId, artifact.transfer_id)
      downloadBlob(fileBlob, artifact.file_name)
    } catch (err) {
      const message = err instanceof ApiRequestError ? err.message : '下载文件失败'
      toast.error(message)
      setTransferErrors((current) => ({ ...current, [artifact.transfer_id]: message }))
    } finally {
      setDownloadingArtifactId(null)
    }
  }

  const handleViewerDownload = () => {
    if (viewerBlob && viewerFileName) {
      downloadBlob(viewerBlob, viewerFileName)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      {hasResult && (
        <Card>
          <CardContent className="flex flex-col gap-2 p-4">
            {summaryText && (
              <div>
                <h4 className="text-sm font-medium mb-1">摘要</h4>
                <p className="text-sm text-muted-foreground whitespace-pre-wrap">{summaryText}</p>
              </div>
            )}
            {finalOutputText && (
              <div>
                <h4 className="text-sm font-medium mb-1">最终产出</h4>
                <div className="rounded-md bg-muted p-3">
                  <div
                    className={cn(
                      'prose prose-sm max-w-none text-foreground dark:prose-invert',
                      'prose-p:my-2 prose-headings:my-3',
                      'prose-ul:my-2 prose-ol:my-2 prose-li:my-0.5',
                      'prose-pre:overflow-x-auto prose-pre:text-xs',
                      'prose-code:text-xs prose-table:text-xs'
                    )}
                  >
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>
                      {finalOutputText}
                    </ReactMarkdown>
                  </div>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {hasArtifacts && (
        <div>
          <h4 className="text-sm font-medium mb-2">交付物</h4>
          <div className="flex flex-col gap-2">
            {safeArtifacts.map((artifact) => {
              const transferError = transferErrors[artifact.transfer_id]
              const sizeLabel = formatFileSize(artifact.file_size)

              return (
                <Card key={artifact.transfer_id}>
                  <CardContent className="flex flex-col gap-3 p-3">
                    <div className="flex items-center gap-3">
                      <div className="flex size-8 items-center justify-center rounded-lg bg-muted">
                        <FileText className="size-4 text-muted-foreground" />
                      </div>
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-medium truncate">{artifact.file_name}</p>
                        <p className="text-xs text-muted-foreground truncate">
                          {artifact.mime_type}{sizeLabel ? ` · ${sizeLabel}` : ''}
                        </p>
                      </div>
                    </div>

                    <div className="flex items-center gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={loadingArtifactId === artifact.transfer_id}
                        onClick={() => void handlePreview(artifact)}
                      >
                        {loadingArtifactId === artifact.transfer_id ? (
                          <>
                            <Loader2 className="size-3.5 animate-spin" />
                            加载中
                          </>
                        ) : (
                          <>
                            <Eye className="size-3.5" />
                            预览
                          </>
                        )}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={downloadingArtifactId === artifact.transfer_id}
                        onClick={() => void handleDownloadArtifact(artifact)}
                      >
                        {downloadingArtifactId === artifact.transfer_id ? (
                          <>
                            <Loader2 className="size-3.5 animate-spin" />
                            下载中
                          </>
                        ) : (
                          <>
                            <Download className="size-3.5" />
                            下载
                          </>
                        )}
                      </Button>
                    </div>

                    {transferError && (
                      <p className="text-xs text-destructive">{transferError}</p>
                    )}
                  </CardContent>
                </Card>
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
        onDownload={handleViewerDownload}
      />
    </div>
  )
}

function formatFileSize(bytes: number) {
  if (!bytes || bytes <= 0) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
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
