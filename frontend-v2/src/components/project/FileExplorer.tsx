import { useRef, useState, Fragment, type ReactNode } from 'react'
import { Button, Table, Tag, App, Modal, Input, Tooltip, Space } from 'antd'
import {
  UploadOutlined,
  DownloadOutlined,
  DeleteOutlined,
  FileTextOutlined,
  EyeOutlined,
  FolderOutlined,
  FolderAddOutlined,
  HomeOutlined,
  EditOutlined,
  ArrowLeftOutlined,
} from '@ant-design/icons'
import dayjs from 'dayjs'
import {
  useBrowseProjectFiles,
  useUploadProjectFile,
  useDeleteProjectFile,
  useCreateProjectFolder,
  useRenameProjectFile,
} from '@/hooks/useProjectFiles'
import { downloadProjectFile } from '@/api/projectFiles'
import { FileViewer } from '@/components/task/FileViewer'
import type { ProjectFile } from '@/types'

interface Props {
  projectId: string
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

/** 可点击的面包屑项：hover 有背景高亮反馈；active 为当前目录（不可点击） */
function CrumbItem({ label, onClick, active }: { label: ReactNode; onClick: () => void; active?: boolean }) {
  if (active) {
    return <span style={{ color: 'var(--text-primary)', fontWeight: 500 }}>{label}</span>
  }
  return (
    <span
      onClick={onClick}
      style={{
        color: 'var(--text-tertiary)',
        cursor: 'pointer',
        padding: '2px 8px',
        borderRadius: 'var(--radius-control)',
        transition: 'background 0.12s ease, color 0.12s ease',
        display: 'inline-flex',
        alignItems: 'center',
        gap: 4,
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.background = 'rgba(109,95,245,0.14)'
        e.currentTarget.style.color = 'var(--signal-hover)'
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.background = 'transparent'
        e.currentTarget.style.color = 'rgba(255,255,255,0.55)'
      }}
    >
      {label}
    </span>
  )
}

export function FileExplorer({ projectId }: Props) {
  const { message } = App.useApp()
  const [parentId, setParentId] = useState('')
  const { data: browse, isLoading } = useBrowseProjectFiles(projectId, parentId)
  const uploadFile = useUploadProjectFile(projectId, parentId)
  const deleteFile = useDeleteProjectFile(projectId, parentId)
  const createFolder = useCreateProjectFolder(projectId, parentId)
  const renameFile = useRenameProjectFile(projectId, parentId)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const [folderModalOpen, setFolderModalOpen] = useState(false)
  const [folderName, setFolderName] = useState('')
  const [renaming, setRenaming] = useState<ProjectFile | null>(null)
  const [renameName, setRenameName] = useState('')

  // 文件预览
  const [preview, setPreview] = useState<ProjectFile | null>(null)
  const [previewBlob, setPreviewBlob] = useState<Blob | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)

  const breadcrumbs = browse?.breadcrumbs ?? []
  const folders = browse?.folders ?? []
  const files = browse?.files ?? []
  const virtualFolders = browse?.virtual_folders ?? []

  const handleUpload = async (selectedFiles: FileList | null) => {
    if (!selectedFiles || selectedFiles.length === 0) return
    const formData = new FormData()
    for (let i = 0; i < selectedFiles.length; i++) {
      formData.append('file', selectedFiles[i])
    }
    if (parentId) formData.append('parent_id', parentId)
    try {
      await uploadFile.mutateAsync(formData)
      message.success('上传成功')
      if (fileInputRef.current) fileInputRef.current.value = ''
    } catch {
      message.error('上传失败')
    }
  }

  const handleDownload = async (file: ProjectFile) => {
    try {
      const blob = await downloadProjectFile(projectId, file.id)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = file.file_name
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    } catch {
      message.error('下载失败')
    }
  }

  /** 打开文件预览（文本/markdown/代码/图片/PDF） */
  const handlePreview = async (file: ProjectFile) => {
    if (file.is_folder) return
    setPreview(file)
    setPreviewBlob(null)
    setPreviewLoading(true)
    try {
      const blob = await downloadProjectFile(projectId, file.id)
      setPreviewBlob(blob)
    } catch {
      message.error('预览加载失败')
      setPreview(null)
    } finally {
      setPreviewLoading(false)
    }
  }

  const handleDelete = async (file: ProjectFile) => {
    const label = file.is_folder ? '文件夹' : '文件'
    if (!confirm(`确定删除${label} "${file.file_name}" 吗？${file.is_folder ? '其内容将一并删除。' : ''}`)) return
    try {
      await deleteFile.mutateAsync(file.id)
      message.success(`${label}已删除`)
    } catch {
      message.error('删除失败')
    }
  }

  const handleCreateFolder = async () => {
    const name = folderName.trim()
    if (!name) return
    try {
      await createFolder.mutateAsync({ name, parent_id: parentId || undefined })
      message.success('文件夹已创建')
      setFolderModalOpen(false)
      setFolderName('')
    } catch {
      message.error('创建失败')
    }
  }

  const handleRename = async () => {
    if (!renaming) return
    const name = renameName.trim()
    if (!name) return
    try {
      await renameFile.mutateAsync({ fileId: renaming.id, name })
      message.success('重命名成功')
      setRenaming(null)
    } catch {
      message.error('重命名失败')
    }
  }

  const columns = [
    {
      title: '名称',
      dataIndex: 'file_name',
      render: (text: string, record: ProjectFile) => (
        <span
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 8,
            cursor: record.is_folder ? 'pointer' : 'pointer',
            transition: 'color 0.12s ease',
          }}
          onMouseEnter={(e) => {
            if (!record.is_folder) e.currentTarget.style.color = '#a5b4fc'
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.color = 'inherit'
          }}
          onClick={() => {
            if (record.is_folder) setParentId(record.id)
            else void handlePreview(record)
          }}
        >
          {record.is_folder ? <FolderOutlined style={{ color: 'var(--warning)', fontSize: 16 }} /> : <FileTextOutlined style={{ color: 'var(--text-tertiary)', fontSize: 15 }} />}
          <span>{text}</span>
          {record.is_folder && <Tag style={{ marginLeft: 2 }}>文件夹</Tag>}
          {!record.is_folder && record.kind === 'deliverable' && (
            <Tooltip title={`最终交付文件 · 输出绑定：${record.output_name || '-'}`}>
              <Tag color="green" style={{ marginLeft: 2 }}>交付</Tag>
            </Tooltip>
          )}
          {!record.is_folder && record.kind === 'process' && (
            <Tooltip title="过程文件（草稿/中间稿，不作为下游步骤输入）">
              <Tag style={{ marginLeft: 2 }}>过程</Tag>
            </Tooltip>
          )}
        </span>
      ),
    },
    {
      title: '来源',
      dataIndex: 'source',
      width: 110,
      render: (s: string) => (
        <Tag>{s === 'user_upload' ? '用户上传' : s === 'agent_artifact' ? '数字员工产物' : s === 'meeting_minutes' ? '会议纪要' : s === '__virtual__' ? '系统目录' : s}</Tag>
      ),
    },
    { title: '大小', dataIndex: 'file_size', width: 90, render: (v: number, record: ProjectFile) => (record.is_folder ? '—' : formatFileSize(v)) },
    { title: '创建时间', dataIndex: 'created_at', width: 130, render: (v: string) => dayjs(v).format('MM-DD HH:mm') },
    {
      title: '操作',
      key: 'action',
      width: 150,
      render: (_: unknown, record: ProjectFile) => (
        <span style={{ display: 'inline-flex', gap: 4 }}>
          {!record.is_folder && (
            <>
              <Tooltip title="预览">
                <Button type="text" size="small" icon={<EyeOutlined />} onClick={() => void handlePreview(record)} loading={previewLoading && preview?.id === record.id} />
              </Tooltip>
              <Tooltip title="下载">
                <Button type="text" size="small" icon={<DownloadOutlined />} onClick={() => handleDownload(record)} />
              </Tooltip>
            </>
          )}
          <Tooltip title="重命名">
            <Button
              type="text"
              size="small"
              icon={<EditOutlined />}
              onClick={() => {
                setRenaming(record)
                setRenameName(record.file_name)
              }}
            />
          </Tooltip>
          <Tooltip title="删除">
            <Button type="text" size="small" danger icon={<DeleteOutlined />} onClick={() => handleDelete(record)} />
          </Tooltip>
        </span>
      ),
    },
  ]

  const virtualRows = virtualFolders.map((vf) => ({
    key: vf.id,
    id: vf.id,
    project_id: '',
    file_name: vf.name,
    file_size: 0,
    mime_type: '',
    is_folder: true,
    source: '__virtual__' as ProjectFile['source'],
    item_count: vf.item_count,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  }))

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      {/* 顶部工具条：面包屑 + 操作按钮 */}
      <div style={{ padding: '12px 0', display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
        <div style={{ flex: 1, minWidth: 200, display: 'flex', alignItems: 'center', gap: 4, flexWrap: 'wrap' }}>
          <CrumbItem label={<HomeOutlined />} active={breadcrumbs.length === 0} onClick={() => setParentId('')} />
          {breadcrumbs.map((crumb, i) => (
            <Fragment key={crumb.id}>
              <span style={{ color: 'var(--text-quaternary)' }}>/</span>
              <CrumbItem label={crumb.name} active={i === breadcrumbs.length - 1} onClick={() => setParentId(crumb.id)} />
            </Fragment>
          ))}
        </div>
        <Space>
          {parentId && (
            <Button icon={<ArrowLeftOutlined />} onClick={() => setParentId(breadcrumbs.length > 1 ? breadcrumbs[breadcrumbs.length - 2].id : '')}>
              返回上级
            </Button>
          )}
          <input ref={fileInputRef} type="file" style={{ display: 'none' }} multiple onChange={(e) => handleUpload(e.target.files)} />
          <Button type="primary" icon={<UploadOutlined />} onClick={() => fileInputRef.current?.click()} loading={uploadFile.isPending}>
            上传文件
          </Button>
          <Button icon={<FolderAddOutlined />} onClick={() => setFolderModalOpen(true)}>
            新建文件夹
          </Button>
        </Space>
      </div>

      <div style={{ flex: 1, overflowY: 'auto', paddingRight: 2 }}>
        <Table
          dataSource={[...virtualRows, ...folders, ...files]}
          rowKey="id"
          loading={isLoading}
          pagination={false}
          size="small"
          columns={columns}
          locale={{ emptyText: parentId ? '此文件夹为空' : '暂无文件，点击"上传文件"或"新建文件夹"开始' }}
        />
      </div>

      {/* 新建文件夹 */}
      <Modal
        title="新建文件夹"
        open={folderModalOpen}
        onCancel={() => setFolderModalOpen(false)}
        onOk={handleCreateFolder}
        okText="创建"
        confirmLoading={createFolder.isPending}
      >
        <Input
          placeholder="文件夹名称"
          value={folderName}
          onChange={(e) => setFolderName(e.target.value)}
          onPressEnter={handleCreateFolder}
          autoFocus
        />
      </Modal>

      {/* 重命名 */}
      <Modal
        title="重命名"
        open={!!renaming}
        onCancel={() => setRenaming(null)}
        onOk={handleRename}
        okText="保存"
        confirmLoading={renameFile.isPending}
      >
        <Input
          placeholder="新名称"
          value={renameName}
          onChange={(e) => setRenameName(e.target.value)}
          onPressEnter={handleRename}
          autoFocus
        />
      </Modal>

      {/* 文件预览 */}
      <FileViewer
        open={!!preview}
        onOpenChange={(o) => {
          if (!o) {
            setPreview(null)
            setPreviewBlob(null)
          }
        }}
        blob={previewBlob}
        fileName={preview?.file_name ?? ''}
        onDownload={preview ? () => handleDownload(preview) : undefined}
      />
    </div>
  )
}
