import { useState } from 'react'
import { Button, Input, Tag, Card, Skeleton, Empty, Modal, Form, Tabs, Typography, App, Collapse } from 'antd'
import { FileTextOutlined, UploadOutlined, ReloadOutlined, SearchOutlined, DownOutlined, UpOutlined, CheckCircleOutlined, CloseCircleOutlined, LoadingOutlined, DeleteOutlined, ClockCircleOutlined, BookOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { useKnowledgeDocs, useUploadDocument, useDeleteDocument, useReprocessDocument, useKnowledgeSearch, useKnowledgeChunks } from '@/hooks/useKnowledge'
import { PageHeader } from '@/components/shared/PageHeader'
import type { KnowledgeDocument, KnowledgeSearchResult } from '@/types'
import { usePermStore } from '@/stores/permStore'
import { PERM } from '@/lib/perms'

const { Text, Paragraph } = Typography

const STATUS_FILTERS = [
  { value: 'all', label: '全部' },
  { value: 'ready', label: '已就绪' },
  { value: 'processing', label: '处理中' },
  { value: 'failed', label: '失败' },
] as const

function StatusIcon({ status }: { status: string }) {
  if (status === 'ready') return <CheckCircleOutlined style={{ color: 'var(--success)' }} />
  if (status === 'processing') return <LoadingOutlined style={{ color: 'var(--info)' }} spin />
  if (status === 'failed') return <CloseCircleOutlined style={{ color: 'var(--error)' }} />
  return <ClockCircleOutlined style={{ color: 'var(--text-tertiary)' }} />
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function DocRow({ doc, onDelete, onReprocess }: { doc: KnowledgeDocument; onDelete: (id: string) => void; onReprocess: (id: string) => void }) {
  const [expanded, setExpanded] = useState(false)
  const { data: chunks, isLoading: chunksLoading } = useKnowledgeChunks(expanded ? doc.id : undefined)
  // 重建/删除文档 = POST|DELETE /knowledge/documents/**（knowledge.manage）
  const canManageKnowledge = usePermStore((s) => s.hasPerm(PERM.KNOWLEDGE_MANAGE))

  return (
    <div style={{ borderBottom: '1px solid var(--line)' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '12px 16px', cursor: 'pointer' }} onClick={() => setExpanded(!expanded)}>
        <FileTextOutlined style={{ color: 'var(--text-tertiary)' }} />
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0, flex: 1 }}>
          <Text strong style={{ flexShrink: 0, maxWidth: '40%' }} ellipsis>{doc.title}</Text>
          <StatusIcon status={doc.status} />
          {doc.description && <Text type="secondary" ellipsis style={{ fontSize: 12 }}>{doc.description}</Text>}
          {doc.tags?.length > 0 && (
            <span style={{ display: 'inline-flex', gap: 4, flexShrink: 0 }}>
              {doc.tags.map((tag) => (
                <Tag key={tag} style={{ fontSize: 11, padding: '0 6px', margin: 0 }}>{tag}</Tag>
              ))}
            </span>
          )}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, fontSize: 12, color: 'var(--text-tertiary)', flexShrink: 0 }}>
          <span>{formatFileSize(doc.file_size)}</span>
          <span>{doc.chunk_count} 块</span>
          <span>{dayjs(doc.created_at).fromNow()}</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', flexShrink: 0 }} onClick={(e) => e.stopPropagation()}>
          {canManageKnowledge && doc.status === 'failed' && <Button type="text" size="small" icon={<ReloadOutlined />} onClick={() => onReprocess(doc.id)} />}
          <Button type="text" size="small" icon={expanded ? <UpOutlined /> : <DownOutlined />} />
          {canManageKnowledge && <Button type="text" size="small" danger icon={<DeleteOutlined />} onClick={() => onDelete(doc.id)} />}
        </div>
      </div>
      {expanded && (
        <div style={{ padding: '0 16px 12px', background: 'var(--surface)' }}>
          {chunksLoading ? (
            <Skeleton active paragraph={{ rows: 2 }} />
          ) : !chunks || chunks.length === 0 ? (
            <Text type="secondary" style={{ fontSize: 12 }}>暂无分块数据</Text>
          ) : (
            <Collapse ghost items={chunks.map((chunk) => ({
              key: chunk.id,
              label: <Text type="secondary" style={{ fontSize: 12 }}>#{chunk.chunk_index} · {chunk.token_count} tokens</Text>,
              children: <Paragraph style={{ fontSize: 12, color: 'var(--text-primary)' }}>{chunk.content}</Paragraph>,
            }))} />
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
  const { message } = App.useApp()

  const handleSearch = async () => {
    if (!query.trim()) return
    try {
      const res = await searchMutation.mutateAsync({ query: query.trim(), top_k: 5, min_score: 0.3 })
      setResults(res.data.items)
    } catch {
      message.error('搜索失败')
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div style={{ display: 'flex', gap: 8 }}>
        <Input placeholder="输入语义搜索内容..." value={query} onChange={(e) => setQuery(e.target.value)} onPressEnter={handleSearch} style={{ flex: 1 }} />
        <Button type="primary" icon={<SearchOutlined />} onClick={handleSearch} loading={searchMutation.isPending} disabled={!query.trim()}>搜索</Button>
      </div>
      {results.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          {results.map((result) => (
            <Card key={result.chunk_id} size="small" bordered={false} style={{ background: 'var(--surface)' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                <Text strong>{result.document_title}</Text>
                <Tag>#{result.chunk_index}</Tag>
                <Text type="secondary" style={{ marginLeft: 'auto', fontSize: 12 }}>相似度 {(result.score * 100).toFixed(1)}%</Text>
              </div>
              <Paragraph style={{ fontSize: 13, color: 'var(--text-primary)', margin: 0 }} ellipsis={{ rows: 4 }}>{result.content}</Paragraph>
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
  const [uploadForm] = Form.useForm()
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const { message } = App.useApp()
  // 上传文档 = POST /knowledge/documents（knowledge.manage），无权限时隐藏入口
  // （权限视图未就绪时 fail-open，由后端 403 兜底）；语义搜索是查询，不加门禁。
  const canManageKnowledge = usePermStore((s) => s.hasPerm(PERM.KNOWLEDGE_MANAGE))

  const queryParams = statusFilter === 'all' ? undefined : { status: statusFilter }
  const { data: docs, isLoading } = useKnowledgeDocs(queryParams)
  const uploadMutation = useUploadDocument()
  const deleteMutation = useDeleteDocument()
  const reprocessMutation = useReprocessDocument()

  const handleUpload = async () => {
    if (!selectedFile) return
    const values = await uploadForm.validateFields()
    const formData = new FormData()
    formData.append('file', selectedFile)
    formData.append('title', values.title?.trim() || selectedFile.name)
    if (values.description?.trim()) formData.append('description', values.description.trim())
    if (values.tags?.trim()) formData.append('tags', values.tags.trim())
    try {
      await uploadMutation.mutateAsync(formData)
      message.success('文档上传成功，正在处理中...')
      setShowUpload(false)
      setSelectedFile(null)
      uploadForm.resetFields()
    } catch {
      message.error('上传失败')
    }
  }

  const handleDelete = async (id: string) => {
    try { await deleteMutation.mutateAsync(id); message.success('文档已删除') } catch { message.error('删除失败') }
  }

  const handleReprocess = async (id: string) => {
    try { await reprocessMutation.mutateAsync(id); message.success('正在重新处理...') } catch { message.error('重新处理失败') }
  }

  return (
    <div>
      <PageHeader
        title="知识库"
        icon={<BookOutlined />}
        actions={
          <>
            <Tabs
              activeKey={activeTab}
              onChange={(k) => setActiveTab(k as 'documents' | 'search')}
              items={[{ key: 'documents', label: '文档管理' }, { key: 'search', label: '语义搜索' }]}
            />
            {/* 上传文档 = knowledge.manage */}
            {canManageKnowledge && (
              <Button type="primary" icon={<UploadOutlined />} onClick={() => setShowUpload(true)}>上传文档</Button>
            )}
          </>
        }
      />

      <Modal
        title="上传文档"
        open={showUpload}
        onCancel={() => { setShowUpload(false); setSelectedFile(null); uploadForm.resetFields() }}
        onOk={handleUpload}
        confirmLoading={uploadMutation.isPending}
        okButtonProps={{ disabled: !selectedFile }}
      >
        <Form form={uploadForm} layout="vertical">
          <Form.Item label="文件">
            <input type="file" style={{ display: 'none' }} accept=".md,.txt,.text" id="kb-file" onChange={(e) => {
                const f = e.target.files?.[0] ?? null
                setSelectedFile(f)
                if (f && !uploadForm.getFieldValue('title')) uploadForm.setFieldValue('title', f.name.replace(/\.[^.]+$/, ''))
              }} />
            <Button onClick={() => document.getElementById('kb-file')?.click()}>选择文件</Button>
            {selectedFile ? <Text style={{ marginLeft: 8 }}>{selectedFile.name}</Text> : <Text type="secondary" style={{ marginLeft: 8 }}>支持 .md, .txt 格式</Text>}
          </Form.Item>
          <Form.Item name="title" label="标题" rules={[{ required: true }]}>
            <Input placeholder="文档标题" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} placeholder="简要描述文档内容" />
          </Form.Item>
          <Form.Item name="tags" label="标签">
            <Input placeholder="用逗号分隔，如：架构,设计,核心" />
          </Form.Item>
        </Form>
      </Modal>

      {activeTab === 'search' ? (
        <SearchPanel />
      ) : (
        <>
          <div style={{ display: 'flex', gap: 4, marginBottom: 12 }}>
            {STATUS_FILTERS.map((filter) => (
              <Button key={filter.value} type={statusFilter === filter.value ? 'primary' : 'default'} size="small" onClick={() => setStatusFilter(filter.value)}>
                {filter.label}
              </Button>
            ))}
          </div>
          {isLoading ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              <Skeleton active paragraph={{ rows: 2 }} />
              <Skeleton active paragraph={{ rows: 2 }} />
            </div>
          ) : !docs || docs.length === 0 ? (
            <Empty description="暂无知识文档">
              {/* 上传文档 = knowledge.manage（无权限时仅保留 Empty 文案兜底） */}
              {canManageKnowledge && (
                <Button type="primary" icon={<UploadOutlined />} onClick={() => setShowUpload(true)}>上传文档</Button>
              )}
            </Empty>
          ) : (
            <Card bordered={false} style={{ padding: 0, overflow: 'hidden', background: 'var(--surface)' }}>
              {docs.map((doc) => (
                <DocRow key={doc.id} doc={doc} onDelete={handleDelete} onReprocess={handleReprocess} />
              ))}
            </Card>
          )}
        </>
      )}
    </div>
  )
}