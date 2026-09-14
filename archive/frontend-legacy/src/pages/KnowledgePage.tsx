import { useState, useRef } from 'react'
import {
  FileText,
  Upload,
  Trash2,
  RefreshCw,
  Search,
  ChevronDown,
  ChevronUp,
  Loader2,
  CheckCircle2,
  XCircle,
  Clock,
} from 'lucide-react'
import { toast } from 'sonner'
import { PageContainer } from '@/components/layout/PageContainer'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { EmptyState } from '@/components/shared/EmptyState'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  useKnowledgeDocs,
  useUploadDocument,
  useDeleteDocument,
  useReprocessDocument,
  useKnowledgeSearch,
  useKnowledgeChunks,
} from '@/hooks/useKnowledge'
import { cn, formatRelativeTime } from '@/lib/utils'
import type { KnowledgeDocument, KnowledgeSearchResult } from '@/types'

const STATUS_FILTERS = [
  { value: 'all', label: '全部' },
  { value: 'ready', label: '已就绪' },
  { value: 'processing', label: '处理中' },
  { value: 'failed', label: '失败' },
] as const

function StatusIcon({ status }: { status: string }) {
  switch (status) {
    case 'ready':
      return <CheckCircle2 className="size-4 text-green-500" />
    case 'processing':
      return <Loader2 className="size-4 text-blue-500 animate-spin" />
    case 'failed':
      return <XCircle className="size-4 text-red-500" />
    default:
      return <Clock className="size-4 text-muted-foreground" />
  }
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function UploadDialog({
  open,
  onOpenChange,
  onUpload,
  pending,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onUpload: (formData: FormData) => Promise<void>
  pending: boolean
}) {
  const [file, setFile] = useState<File | null>(null)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [tags, setTags] = useState('')
  const fileInputRef = useRef<HTMLInputElement>(null)

  const reset = () => {
    setFile(null)
    setTitle('')
    setDescription('')
    setTags('')
    if (fileInputRef.current) fileInputRef.current.value = ''
  }

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]
    if (f) {
      setFile(f)
      if (!title) setTitle(f.name.replace(/\.[^.]+$/, ''))
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!file) return

    const formData = new FormData()
    formData.append('file', file)
    formData.append('title', title.trim() || file.name)
    if (description.trim()) formData.append('description', description.trim())
    if (tags.trim()) formData.append('tags', tags.trim())

    await onUpload(formData)
    reset()
  }

  const handleOpenChange = (v: boolean) => {
    if (!v) reset()
    onOpenChange(v)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>上传文档</DialogTitle>
          <DialogDescription>上传文档到知识库，供 Agent 检索使用</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="mt-4 flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <label className="text-sm font-medium">文件 *</label>
            <div className="flex items-center gap-2">
              <input
                ref={fileInputRef}
                type="file"
                className="hidden"
                accept=".md,.txt,.text"
                onChange={handleFileChange}
              />
              <Button type="button" variant="outline" size="sm" onClick={() => fileInputRef.current?.click()}>
                选择文件
              </Button>
              {file ? (
                <span className="text-sm text-muted-foreground truncate flex-1">{file.name}</span>
              ) : (
                <span className="text-sm text-muted-foreground">支持 .md, .txt 格式</span>
              )}
            </div>
          </div>
          <div className="flex flex-col gap-2">
            <label className="text-sm font-medium">标题 *</label>
            <Input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="文档标题"
              required
            />
          </div>
          <div className="flex flex-col gap-2">
            <label className="text-sm font-medium">描述</label>
            <Textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="简要描述文档内容"
              rows={2}
            />
          </div>
          <div className="flex flex-col gap-2">
            <label className="text-sm font-medium">标签</label>
            <Input
              value={tags}
              onChange={(e) => setTags(e.target.value)}
              placeholder="用逗号分隔，如：架构,设计,核心"
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={pending || !file || !title.trim()}>
              {pending ? <Loader2 className="size-4 mr-1.5 animate-spin" /> : <Upload className="size-4 mr-1.5" />}
              {pending ? '上传中...' : '上传'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function DocRow({
  doc,
  onDelete,
  onReprocess,
}: {
  doc: KnowledgeDocument
  onDelete: (id: string) => void
  onReprocess: (id: string) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const { data: chunks } = useKnowledgeChunks(expanded ? doc.id : undefined)

  return (
    <div className="border-b last:border-b-0">
      <div className="flex items-center gap-3 px-3 py-2 hover:bg-muted/50 transition-colors">
        <FileText className="size-4 text-muted-foreground shrink-0" />
        <div className="flex items-center gap-2 min-w-0 flex-1">
          <span className="font-medium text-sm truncate shrink-0 max-w-[40%]">{doc.title}</span>
          <StatusIcon status={doc.status} />
          {doc.description && (
            <span className="text-xs text-muted-foreground truncate">{doc.description}</span>
          )}
        </div>
        <div className="flex items-center gap-3 text-xs text-muted-foreground shrink-0">
          {doc.tags.length > 0 && (
            <div className="hidden md:flex gap-1">
              {doc.tags.map((tag) => (
                <Badge key={tag} variant="secondary" className="text-[11px] px-1.5 py-0">
                  {tag}
                </Badge>
              ))}
            </div>
          )}
          <span className="tabular-nums">{formatFileSize(doc.file_size)}</span>
          <span className="tabular-nums">{doc.chunk_count} 块</span>
          <span className="hidden sm:inline">{formatRelativeTime(doc.created_at)}</span>
        </div>
        <div className="flex items-center shrink-0">
          {doc.status === 'failed' && (
            <Button variant="ghost" size="icon" className="size-7" onClick={() => onReprocess(doc.id)} title="重新处理">
              <RefreshCw className="size-3.5" />
            </Button>
          )}
          <Button variant="ghost" size="icon" className="size-7" onClick={() => setExpanded(!expanded)} title={expanded ? '收起' : '分块'}>
            {expanded ? <ChevronUp className="size-3.5" /> : <ChevronDown className="size-3.5" />}
          </Button>
          <Button variant="ghost" size="icon" className="size-7 text-destructive hover:text-destructive" onClick={() => onDelete(doc.id)} title="删除">
            <Trash2 className="size-3.5" />
          </Button>
        </div>
      </div>

      {expanded && (
        <div className="px-3 pb-2 pt-1 bg-muted/30">
          {!chunks ? (
            <div className="space-y-1.5">
              {[1, 2].map((i) => <Skeleton key={i} className="h-10 rounded" />)}
            </div>
          ) : chunks.length === 0 ? (
            <p className="text-xs text-muted-foreground py-1">暂无分块数据</p>
          ) : (
            <div className="space-y-1.5">
              {chunks.map((chunk) => (
                <div key={chunk.id} className="rounded bg-background p-2 text-xs border">
                  <div className="flex items-center gap-2 mb-0.5">
                    <Badge variant="outline" className="text-[11px] px-1 py-0">#{chunk.chunk_index}</Badge>
                    <span className="text-muted-foreground">{chunk.token_count} tokens</span>
                  </div>
                  <p className="text-muted-foreground line-clamp-2 whitespace-pre-wrap">{chunk.content}</p>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function SearchPanel() {
  const [query, setQuery] = useState('')
  const searchMutation = useKnowledgeSearch()
  const [results, setResults] = useState<KnowledgeSearchResult[]>([])

  const handleSearch = async () => {
    if (!query.trim()) return
    try {
      const res = await searchMutation.mutateAsync({ query: query.trim(), top_k: 5, min_score: 0.3 })
      setResults(res.data.items)
    } catch {
      toast.error('搜索失败')
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex gap-2">
        <Input
          placeholder="输入语义搜索内容..."
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
        />
        <Button onClick={handleSearch} disabled={searchMutation.isPending || !query.trim()}>
          {searchMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <Search className="size-4" />}
          <span className="ml-2">搜索</span>
        </Button>
      </div>

      {results.length > 0 && (
        <div className="space-y-2">
          {results.map((result) => (
            <Card key={result.chunk_id} className="p-3">
              <div className="flex items-center gap-2 mb-1">
                <span className="font-medium text-sm">{result.document_title}</span>
                <Badge variant="outline" className="text-xs">#{result.chunk_index}</Badge>
                <span className="text-xs text-muted-foreground ml-auto">
                  相似度 {(result.score * 100).toFixed(1)}%
                </span>
              </div>
              <p className="text-sm text-muted-foreground whitespace-pre-wrap line-clamp-4">{result.content}</p>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}

export function KnowledgePage() {
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [activeTab, setActiveTab] = useState<'documents' | 'search'>('documents')
  const [showUpload, setShowUpload] = useState(false)

  const queryParams = statusFilter === 'all' ? undefined : { status: statusFilter }
  const { data: docs, isLoading } = useKnowledgeDocs(queryParams)
  const uploadMutation = useUploadDocument()
  const deleteMutation = useDeleteDocument()
  const reprocessMutation = useReprocessDocument()

  const handleUpload = async (formData: FormData) => {
    try {
      await uploadMutation.mutateAsync(formData)
      toast.success('文档上传成功，正在处理中...')
      setShowUpload(false)
    } catch {
      toast.error('上传失败')
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await deleteMutation.mutateAsync(id)
      toast.success('文档已删除')
    } catch {
      toast.error('删除失败')
    }
  }

  const handleReprocess = async (id: string) => {
    try {
      await reprocessMutation.mutateAsync(id)
      toast.success('正在重新处理...')
    } catch {
      toast.error('重新处理失败')
    }
  }

  return (
    <PageContainer>
      {/* Header: title + tabs + actions in one row */}
      <div className="flex items-center gap-4 mb-4">
        <h1 className="text-lg font-bold shrink-0">知识库</h1>
        <div className="flex gap-0.5 rounded-lg bg-muted p-0.5">
          {[
            { key: 'documents' as const, label: '文档管理' },
            { key: 'search' as const, label: '语义搜索' },
          ].map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={cn(
                'px-2.5 py-1 text-xs rounded-md transition-colors',
                activeTab === tab.key
                  ? 'bg-background text-foreground shadow-sm font-medium'
                  : 'text-muted-foreground hover:text-foreground'
              )}
            >
              {tab.label}
            </button>
          ))}
        </div>
        <div className="flex-1" />
        <Button size="sm" onClick={() => setShowUpload(true)}>
          <Upload className="size-3.5 mr-1.5" />
          上传文档
        </Button>
      </div>

      <UploadDialog
        open={showUpload}
        onOpenChange={setShowUpload}
        onUpload={handleUpload}
        pending={uploadMutation.isPending}
      />

      {activeTab === 'search' ? (
        <SearchPanel />
      ) : (
        <>
          {/* Status filter */}
          <div className="flex gap-0.5 mb-3 rounded-lg bg-muted p-0.5 w-fit">
            {STATUS_FILTERS.map((filter) => (
              <button
                key={filter.value}
                onClick={() => setStatusFilter(filter.value)}
                className={cn(
                  'px-2.5 py-1 text-xs rounded-md transition-colors',
                  statusFilter === filter.value
                    ? 'bg-background text-foreground shadow-sm font-medium'
                    : 'text-muted-foreground hover:text-foreground'
                )}
              >
                {filter.label}
              </button>
            ))}
          </div>

          {/* Document list */}
          {isLoading ? (
            <div className="space-y-1">
              {[1, 2, 3].map((i) => <Skeleton key={i} className="h-10 rounded" />)}
            </div>
          ) : !docs || docs.length === 0 ? (
            <EmptyState
              icon={FileText}
              title="暂无知识文档"
              description="上传文档后，Agent 在执行任务时可以检索相关知识"
              action={
                <Button size="sm" onClick={() => setShowUpload(true)}>
                  <Upload className="size-3.5 mr-1.5" />
                  上传文档
                </Button>
              }
            />
          ) : (
            <Card className="overflow-hidden">
              {docs.map((doc) => (
                <DocRow
                  key={doc.id}
                  doc={doc}
                  onDelete={handleDelete}
                  onReprocess={handleReprocess}
                />
              ))}
            </Card>
          )}
        </>
      )}
    </PageContainer>
  )
}
