import { useState, useCallback, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import type { ProjectFile, BreadcrumbNode } from '@/types'
import { useBrowseFiles, useCreateProjectFolder, useDeleteProjectFile, useBatchDeleteProjectFiles, useRenameProjectFile, useUploadProjectFile } from '@/hooks/useProjectFiles'
import { ExplorerToolbar } from './ExplorerToolbar'
import { ExplorerSidebar } from './ExplorerSidebar'
import { FileTable } from './FileTable'
import { FilePreview } from './FilePreview'
import { UploadFileDialog } from './UploadFileDialog'
import { RenameDialog } from './RenameDialog'
import { downloadProjectFile } from '@/api/projectFiles'

interface Props {
  projectId: string
}

export function FileExplorer({ projectId }: Props) {
  const qc = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)

  // ─── navigation state ───
  const [folderId, setFolderId] = useState('')  // '' = root
  const [bready, setBready] = useState<BreadcrumbNode[]>([])

  // ─── selection ───
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())

  // ─── dialogs ───
  const [uploadOpen, setUploadOpen] = useState(false)
  const [renaming, setRenaming] = useState<{ id: string; name: string; isFolder: boolean } | null>(null)
  const [newFolderName, setNewFolderName] = useState('')

  // ─── preview state ───
  const [previewFile, setPreviewFile] = useState<ProjectFile | null>(null)

  // ─── data ───
  const { data, isLoading, isError } = useBrowseFiles(projectId, folderId || undefined)

  // ─── mutations ───
  const createFolder = useCreateProjectFolder(projectId)
  const deleteFile = useDeleteProjectFile(projectId)
  const batchDelete = useBatchDeleteProjectFiles(projectId)
  const renameFile = useRenameProjectFile(projectId)
  const uploadFile = useUploadProjectFile(projectId)

  // ─── derived ───
  const virtualFolders = data?.virtual_folders ?? []
  const folders = data?.folders ?? []
  const files = data?.files ?? []

  // Collect all non-folder files in current view for preview navigation
  const previewFiles = files.filter(f => !f.is_folder)

  const refresh = useCallback(() => {
    qc.invalidateQueries({ queryKey: ['project-files', projectId] })
  }, [qc, projectId])

  // ─── navigation ───
  const navigateTo = useCallback((id: string, breadcrumbs: BreadcrumbNode[]) => {
    setFolderId(id)
    setBready(breadcrumbs)
    setSelectedIds(new Set())
  }, [])

  const handleBreadcrumbClick = useCallback((_index: number, id: string) => {
    if (id === folderId) return
    setFolderId(id)
    // Trim breadcrumbs to clicked level
    if (id === '') {
      setBready([])
    } else {
      const node = bready.find(b => b.id === id)
      if (node) {
        const idx = bready.indexOf(node)
        setBready(bready.slice(0, idx + 1))
      }
    }
    setSelectedIds(new Set())
  }, [folderId, bready])

  // Sync breadcrumbs from API response
  const effectiveBready = data?.breadcrumbs ?? bready

  // ─── folder click ───
  const handleFolderClick = useCallback((folder: ProjectFile) => {
    const newCrumbs = [...effectiveBready, { id: folder.id, name: folder.file_name }]
    navigateTo(folder.id, newCrumbs)
  }, [effectiveBready, navigateTo])

  // ─── virtual folder click (from sidebar tree) ───
  const handleVirtualFolderNavigate = useCallback((id: string, crumbs: BreadcrumbNode[]) => {
    // Virtual folders don't have proper breadcrumbs in the tree path,
    // so we let the browse API populate them. Fall back to provided crumbs.
    navigateTo(id, crumbs)
  }, [navigateTo])

  // ─── virtual folder click in FileTable ───
  const handleVirtualFolderClick = useCallback((vf: { id: string; name: string }) => {
    const newCrumbs = [...effectiveBready, { id: vf.id, name: vf.name }]
    navigateTo(vf.id, newCrumbs)
  }, [effectiveBready, navigateTo])

  // ─── file preview ───
  const handlePreview = useCallback((file: ProjectFile) => {
    setPreviewFile(file)
  }, [])

  // ─── preview navigation ───
  const handlePreviewNavigate = useCallback((file: ProjectFile) => {
    setPreviewFile(file)
  }, [])

  // ─── file download ───
  const handleDownload = useCallback(async (file: ProjectFile) => {
    if (file.is_folder) return
    try {
      const blob = await downloadProjectFile(projectId, file.id)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = file.file_name
      document.body.appendChild(a)
      a.click()
      a.remove()
      setTimeout(() => URL.revokeObjectURL(url), 60_000)
    } catch (e) {
      console.error('download failed', e)
      toast.error('下载失败')
    }
  }, [projectId])

  // ─── delete ───
  const handleDelete = useCallback(async (file: ProjectFile) => {
    const name = file.is_folder ? `文件夹 "${file.file_name}"` : `文件 "${file.file_name}"`
    if (!confirm(`确定删除 ${name}？${file.is_folder ? '文件夹内的所有内容也会被删除。' : ''}`)) return
    try {
      await deleteFile.mutateAsync(file.id)
      toast.success(`${file.is_folder ? '文件夹' : '文件'}已删除`)
      refresh()
    } catch {
      toast.error('删除失败')
    }
  }, [deleteFile, refresh])

  // ─── batch delete ───
  const handleBatchDelete = useCallback(async () => {
    if (selectedIds.size === 0) return
    if (!confirm(`确定删除选中的 ${selectedIds.size} 项？文件夹内的内容也会被删除。`)) return
    try {
      const result = await batchDelete.mutateAsync(Array.from(selectedIds))
      toast.success(`已删除 ${result.data.deleted} 项`)
      setSelectedIds(new Set())
      refresh()
    } catch {
      toast.error('批量删除失败')
    }
  }, [selectedIds, batchDelete, refresh])

  // ─── rename ───
  const handleRename = useCallback(async (name: string) => {
    if (!renaming) return
    try {
      await renameFile.mutateAsync({ fileId: renaming.id, name })
      toast.success('已重命名')
      setRenaming(null)
      refresh()
    } catch {
      toast.error('重命名失败')
    }
  }, [renaming, renameFile, refresh])

  // ─── upload ───
  const handleUpload = useCallback(async (formData: FormData) => {
    if (folderId) formData.append('parent_id', folderId)
    try {
      await uploadFile.mutateAsync(formData)
      toast.success('上传成功')
      setUploadOpen(false)
      refresh()
    } catch {
      toast.error('上传失败')
    }
  }, [folderId, uploadFile, refresh])

  // ─── new folder ───
  const handleCreateFolder = useCallback(async () => {
    const name = newFolderName.trim()
    if (!name) return
    try {
      await createFolder.mutateAsync({ name, parent_id: folderId || undefined })
      toast.success(`文件夹 "${name}" 已创建`)
      setNewFolderName('')
      refresh()
    } catch {
      toast.error('创建失败')
    }
  }, [newFolderName, folderId, createFolder, refresh])

  return (
    <div className="flex h-full">
      {/* Sidebar */}
      <ExplorerSidebar
        projectId={projectId}
        currentFolderId={folderId}
        onNavigate={handleVirtualFolderNavigate}
      />

      {/* Main */}
      <div className="flex-1 flex flex-col min-w-0">
        <ExplorerToolbar
          bready={effectiveBready}
          folderId={folderId}
          selectedCount={selectedIds.size}
          onBreadcrumbClick={handleBreadcrumbClick}
          onUpload={() => setUploadOpen(true)}
          onBatchDelete={handleBatchDelete}
          newFolderName={newFolderName}
          onNewFolderNameChange={setNewFolderName}
          onCreateFolder={handleCreateFolder}
        />

        <div className="flex-1 min-h-0 overflow-auto">
          {isLoading ? (
            <div className="flex items-center justify-center h-32 text-muted-foreground text-sm">
              加载中...
            </div>
          ) : isError ? (
            <div className="flex items-center justify-center h-32 text-red-500 text-sm">
              加载失败，请刷新重试
            </div>
          ) : (
            <FileTable
              virtualFolders={virtualFolders}
              folders={folders}
              files={files}
              selectedIds={selectedIds}
              onSelectedChange={setSelectedIds}
              onFolderClick={handleFolderClick}
              onVirtualFolderClick={handleVirtualFolderClick}
              onFilePreview={handlePreview}
              onFileDoubleClick={handlePreview}
              onDownload={handleDownload}
              onRename={setRenaming}
              onDelete={handleDelete}
            />
          )}
        </div>
      </div>

      {/* Hidden file input for upload (for toolbar button to trigger) */}
      <input
        ref={fileInputRef}
        type="file"
        className="hidden"
        multiple
        onChange={(e) => {
          const selected = e.target.files
          if (!selected || selected.length === 0) return
          const fd = new FormData()
          for (let i = 0; i < selected.length; i++) {
            fd.append('file', selected[i])
          }
          handleUpload(fd)
          e.target.value = ''
        }}
      />

      <UploadFileDialog
        projectId={projectId}
        open={uploadOpen}
        onOpenChange={setUploadOpen}
        onUpload={handleUpload}
      />

      <RenameDialog
        open={!!renaming}
        onOpenChange={(open) => { if (!open) setRenaming(null) }}
        currentName={renaming?.name ?? ''}
        isFolder={renaming?.isFolder ?? false}
        onConfirm={handleRename}
      />

      <FilePreview
        file={previewFile}
        projectId={projectId}
        files={previewFiles}
        onClose={() => setPreviewFile(null)}
        onNavigate={handlePreviewNavigate}
      />
    </div>
  )
}
