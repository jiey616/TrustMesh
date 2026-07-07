import { useState, useCallback, useRef } from 'react'
import { useProjectFileTree, useCreateProjectFolder, useDeleteProjectFile, useRenameProjectFile, useMoveProjectFile, useBatchDeleteProjectFiles } from '@/hooks/useProjectFiles'
import { downloadProjectFile } from '@/api/projectFiles'
import { FileTreeContextMenu, type ContextMenuTarget } from './FileTreeContextMenu'
import { RenameDialog } from './RenameDialog'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { toast } from 'sonner'
import {
  Folder,
  FolderOpen,
  File,
  ChevronRight,
  ChevronDown,
  Plus,
  Trash2,
  Download,
  CheckSquare,
  Square,
} from 'lucide-react'

// ─── Types ───

interface ProjectFileItem {
  id: string
  file_name: string
  file_size: number
  is_folder: boolean
}

interface TreeNodeData {
  id: string
  name: string
  type: 'directory' | 'file'
  files?: ProjectFileItem[]
  children?: TreeNodeData[]
}

interface ProjectFileTreeProps {
  projectId: string
}

// ─── Component ───

export function ProjectFileTree({ projectId }: ProjectFileTreeProps) {
  const { data: tree, isLoading, isError } = useProjectFileTree(projectId)
  const createFolder = useCreateProjectFolder(projectId)
  const deleteFile = useDeleteProjectFile(projectId)
  const renameFile = useRenameProjectFile(projectId)
  const moveFile = useMoveProjectFile(projectId)
  const batchDelete = useBatchDeleteProjectFiles(projectId)

  // ── State ──
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [contextTarget, setContextTarget] = useState<ContextMenuTarget | null>(null)
  const [contextOpen, setContextOpen] = useState(false)
  const [renamingTarget, setRenamingTarget] = useState<{ fileId: string; name: string; isFolder: boolean } | null>(null)
  const [showNewFolderFor, setShowNewFolderFor] = useState<string | null>(null)
  const [newFolderName, setNewFolderName] = useState('')
  const [dragState, setDragState] = useState<{ sourceId: string; sourceType: 'folder' | 'file' } | null>(null)
  const newFolderInputRef = useRef<HTMLInputElement>(null)

  // ── Expand / Collapse ──
  const toggleExpand = useCallback((id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  // ── Selection ──
  const toggleSelect = useCallback((id: string, ctrlKey = false) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (!ctrlKey) {
        if (next.has(id) && next.size === 1) {
          next.clear()
        } else {
          next.clear()
          next.add(id)
        }
      } else {
        if (next.has(id)) next.delete(id)
        else next.add(id)
      }
      return next
    })
  }, [])

  const clearSelection = useCallback(() => setSelectedIds(new Set()), [])

  // ── Right-click ──
  const handleContextMenu = useCallback((e: React.MouseEvent, fileId: string, fileName: string, isFolder: boolean) => {
    e.preventDefault()
    e.stopPropagation()
    setContextTarget({ fileId, fileName, isFolder })
    setContextOpen(true)
  }, [])

  // ── Rename ──
  const handleRenameConfirm = useCallback(async (newName: string) => {
    if (!renamingTarget) return
    try {
      await renameFile.mutateAsync({ fileId: renamingTarget.fileId, name: newName })
      toast.success('重命名成功')
    } catch {
      toast.error('重命名失败')
    }
    setRenamingTarget(null)
  }, [renamingTarget, renameFile])

  // ── Delete ──
  const handleDelete = useCallback(async (fileId: string) => {
    try {
      await deleteFile.mutateAsync(fileId)
      toast.success('已删除')
      setSelectedIds((prev) => { const n = new Set(prev); n.delete(fileId); return n })
    } catch {
      toast.error('删除失败')
    }
  }, [deleteFile])

  // ── Batch delete ──
  const handleBatchDelete = useCallback(async () => {
    if (selectedIds.size === 0) return
    try {
      await batchDelete.mutateAsync([...selectedIds])
      toast.success(`已删除 ${selectedIds.size} 项`)
      setSelectedIds(new Set())
    } catch {
      toast.error('批量删除失败')
    }
  }, [selectedIds, batchDelete])

  // ── Download ──
  const handleDownload = useCallback(async (fileId: string) => {
    try {
      const blob = await downloadProjectFile(projectId, fileId)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = fileId
      a.click()
      URL.revokeObjectURL(url)
    } catch {
      toast.error('下载失败')
    }
  }, [projectId])

  // ── New Folder ──
  const handleCreateFolder = useCallback(async (parentId: string) => {
    const name = newFolderName.trim()
    if (!name) return
    const request: { name: string; parent_id?: string } = { name }
    if (parentId) request.parent_id = parentId
    try {
      await createFolder.mutateAsync(request)
      toast.success('文件夹已创建')
      setNewFolderName('')
      setShowNewFolderFor(null)
      if (parentId) {
        setExpandedIds((prev) => { const n = new Set(prev); n.add(parentId); return n })
      }
    } catch {
      toast.error('创建失败')
    }
  }, [newFolderName, createFolder])

  const startNewFolder = useCallback((parentId: string) => {
    setContextOpen(false)
    setShowNewFolderFor(parentId)
    setNewFolderName('')
    setTimeout(() => newFolderInputRef.current?.focus(), 100)
  }, [])

  const handleNewFolderKeyDown = useCallback((e: React.KeyboardEvent, parentId: string) => {
    if (e.key === 'Enter') handleCreateFolder(parentId)
    if (e.key === 'Escape') {
      setShowNewFolderFor(null)
      setNewFolderName('')
    }
  }, [handleCreateFolder])

  // ── Drag & Drop ──
  const handleDragStart = useCallback((e: React.DragEvent, fileId: string, sourceType: 'folder' | 'file' = 'folder') => {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', fileId)
    setDragState({ sourceId: fileId, sourceType })
  }, [])

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
  }, [])

  const handleDrop = useCallback(async (e: React.DragEvent, targetFolderId: string) => {
    e.preventDefault()
    if (!dragState || dragState.sourceId === targetFolderId) return
    try {
      await moveFile.mutateAsync({ fileId: dragState.sourceId, parentId: targetFolderId })
      toast.success('移动成功')
      setExpandedIds((prev) => { const n = new Set(prev); n.add(targetFolderId); return n })
    } catch {
      toast.error('移动失败')
    }
    setDragState(null)
  }, [dragState, moveFile])

  const handleDragEnd = useCallback(() => {
    setDragState(null)
  }, [])

  // ── Loading / Error ──
  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
        加载中...
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex items-center justify-center py-12 text-sm text-destructive">
        加载失败
      </div>
    )
  }

  const uploadNodes = (tree?.uploads ?? []) as TreeNodeData[]
  const taskNodes = (tree?.tasks ?? []) as TreeNodeData[]
  const allNodes = [...uploadNodes, ...taskNodes]
  const hasContent = allNodes.length > 0

  return (
    <div className="flex flex-col gap-2">
      {/* ── Batch Toolbar ── */}
      {selectedIds.size > 0 && (
        <div className="flex items-center gap-2 rounded-md bg-accent/50 px-3 py-1.5 text-sm">
          <span className="text-muted-foreground">已选 {selectedIds.size} 项</span>
          <span className="flex-1" />
          <Button variant="ghost" size="sm" onClick={handleBatchDelete}>
            <Trash2 className="size-3.5 mr-1" />
            批量删除
          </Button>
          <Button variant="ghost" size="sm" onClick={clearSelection}>
            取消选择
          </Button>
        </div>
      )}

      {/* ── Tree Content ── */}
      {!hasContent ? (
        <div className="flex flex-col items-center justify-center py-12 text-sm text-muted-foreground gap-2">
          <Folder className="size-8 opacity-30" />
          <span>暂无文件</span>
        </div>
      ) : (
        <div className="rounded-md border">
          {allNodes.map((node, i) => (
            <TreeNode
              key={`root-${i}-${node.id || node.name}`}
              node={node}
              projectId={projectId}
              level={0}
              expandedIds={expandedIds}
              onToggleExpand={toggleExpand}
              selectedIds={selectedIds}
              onToggleSelect={toggleSelect}
              onContextMenu={handleContextMenu}
              onDragStart={handleDragStart}
              onDragOver={handleDragOver}
              onDrop={handleDrop}
              onDragEnd={handleDragEnd}
              dragState={dragState}
              onDownload={handleDownload}
              onDelete={handleDelete}
            />
          ))}
        </div>
      )}

      {/* ── New Folder Inline Input ── */}
      {showNewFolderFor !== null && (
        <div className="flex items-center gap-2 pl-6 py-1">
          <Folder className="size-4 text-amber-500 shrink-0" />
          <input
            ref={newFolderInputRef}
            className="h-7 flex-1 rounded border bg-background px-2 text-sm outline-none focus:ring-1 focus:ring-ring"
            placeholder="文件夹名称"
            value={newFolderName}
            onChange={(e) => setNewFolderName(e.target.value)}
            onKeyDown={(e) => handleNewFolderKeyDown(e, showNewFolderFor)}
            onBlur={() => {
              if (!newFolderName.trim()) {
                setShowNewFolderFor(null)
              }
            }}
          />
          <Button size="sm" variant="ghost" onClick={() => { setShowNewFolderFor(null); setNewFolderName('') }}>
            取消
          </Button>
        </div>
      )}

      {/* ── New Folder Button (root level) ── */}
      {showNewFolderFor === null && (
        <Button
          variant="ghost"
          size="sm"
          className="w-fit text-muted-foreground hover:text-foreground"
          onClick={() => startNewFolder('')}
        >
          <Plus className="size-3.5 mr-1" />
          新建文件夹
        </Button>
      )}

      {/* ── Context Menu ── */}
      <FileTreeContextMenu
        open={contextOpen}
        onOpenChange={setContextOpen}
        target={contextTarget}
        onNewFolder={startNewFolder}
        onRename={(fileId) => {
          setContextOpen(false)
          const node = findNodeById(allNodes, fileId)
          if (node) {
            setRenamingTarget({ fileId, name: node.name, isFolder: node.type === 'directory' })
          }
        }}
        onDelete={(fileId) => {
          setContextOpen(false)
          handleDelete(fileId)
        }}
        onDownload={(fileId) => {
          setContextOpen(false)
          handleDownload(fileId)
        }}
      />

      {/* ── Rename Dialog ── */}
      <RenameDialog
        open={renamingTarget !== null}
        onOpenChange={(open) => { if (!open) setRenamingTarget(null) }}
        currentName={renamingTarget?.name ?? ''}
        isFolder={renamingTarget?.isFolder ?? false}
        onConfirm={handleRenameConfirm}
      />
    </div>
  )
}

// ─── File Row (leaf node inside a folder) ───

interface FileRowProps {
  file: ProjectFileItem
  level: number
  selectedIds: Set<string>
  onToggleSelect: (id: string, ctrlKey?: boolean) => void
  onContextMenu: (e: React.MouseEvent, fileId: string, fileName: string, isFolder: boolean) => void
  onDragStart: (e: React.DragEvent, fileId: string, sourceType: 'folder' | 'file') => void
  onDragOver: (e: React.DragEvent) => void
  onDragEnd: () => void
  onDownload: (fileId: string) => void
  onDelete: (fileId: string) => void
}

function FileRow({
  file,
  level,
  selectedIds,
  onToggleSelect,
  onContextMenu,
  onDragStart,
  onDragOver,
  onDragEnd,
  onDownload,
  onDelete,
}: FileRowProps) {
  const fileId = file.id
  return (
    <div
      className="group flex items-center gap-1 px-2 py-1 text-sm hover:bg-accent/50 transition-colors border-b border-border/20"
      style={{ paddingLeft: `${24 + (level + 1) * 16}px` }}
      onContextMenu={(e) => onContextMenu(e, fileId, file.file_name, false)}
      draggable
      onDragStart={(e) => onDragStart(e, fileId, 'file')}
      onDragOver={onDragOver}
      onDragEnd={onDragEnd}
    >
      <span className="w-4 shrink-0" />
      <span className="w-4 shrink-0" />

      {/* Checkbox */}
      <button
        className="shrink-0 text-muted-foreground hover:text-foreground opacity-0 group-hover:opacity-100 transition-opacity"
        onClick={(e) => {
          e.stopPropagation()
          onToggleSelect(fileId, e.ctrlKey || e.metaKey)
        }}
      >
        {selectedIds.has(fileId) ? (
          <CheckSquare className="size-3.5 text-primary" />
        ) : (
          <Square className="size-3.5" />
        )}
      </button>

      <File className="size-4 text-blue-400 shrink-0" />
      <span className="flex-1 truncate select-none text-muted-foreground">{file.file_name}</span>
      <span className="text-xs text-muted-foreground/60 shrink-0 ml-2">{formatSize(file.file_size)}</span>

      <div className="hidden group-hover:flex items-center gap-0.5 shrink-0">
        <button onClick={() => onDownload(fileId)} className="p-0.5 text-muted-foreground hover:text-foreground" title="下载">
          <Download className="size-3" />
        </button>
        <button onClick={() => onDelete(fileId)} className="p-0.5 text-muted-foreground hover:text-destructive" title="删除">
          <Trash2 className="size-3" />
        </button>
      </div>
    </div>
  )
}

// ─── Recursive TreeNode ───

interface TreeNodeProps {
  node: TreeNodeData
  projectId: string
  level: number
  expandedIds: Set<string>
  onToggleExpand: (id: string) => void
  selectedIds: Set<string>
  onToggleSelect: (id: string, ctrlKey?: boolean) => void
  onContextMenu: (e: React.MouseEvent, fileId: string, fileName: string, isFolder: boolean) => void
  onDragStart: (e: React.DragEvent, fileId: string, sourceType: 'folder' | 'file') => void
  onDragOver: (e: React.DragEvent) => void
  onDrop: (e: React.DragEvent, folderId: string) => void
  onDragEnd: () => void
  dragState: { sourceId: string; sourceType: 'folder' | 'file' } | null
  onDownload: (fileId: string) => void
  onDelete: (fileId: string) => void
}

function TreeNode({
  node,
  projectId: _projectId,
  level,
  expandedIds,
  onToggleExpand,
  selectedIds,
  onToggleSelect,
  onContextMenu,
  onDragStart,
  onDragOver,
  onDrop,
  onDragEnd,
  dragState,
  onDownload,
  onDelete,
}: TreeNodeProps) {
  const folderId = node.id
  const isExpanded = expandedIds.has(folderId)
  const childFolders = node.children ?? []
  const childFiles = node.files ?? []
  const totalChildren = childFolders.length + childFiles.length
  const isDragOver = dragState?.sourceId !== folderId && node.type === 'directory'
  const isVirtual = !folderId // "用户上传" virtual node has no real ID

  return (
    <div>
      {/* ── Folder Header Row ── */}
      <div
        className={cn(
          'group flex items-center gap-1 px-2 py-1 text-sm hover:bg-accent/50 transition-colors border-b border-border/30',
          isDragOver && dragState && 'ring-2 ring-primary/30 rounded'
        )}
        style={{ paddingLeft: `${8 + level * 16}px` }}
        onContextMenu={(e) => {
          if (!isVirtual) onContextMenu(e, folderId, node.name, true)
        }}
        draggable={!isVirtual}
        onDragStart={(e) => { if (!isVirtual) onDragStart(e, folderId, 'folder') }}
        onDragOver={onDragOver}
        onDragEnd={onDragEnd}
        {...(node.type === 'directory'
          ? { onDrop: (e: React.DragEvent) => onDrop(e, folderId) }
          : {}
        )}
      >
        {/* Expand/collapse toggle */}
        {node.type === 'directory' && (
          <button
            onClick={() => onToggleExpand(folderId)}
            className="size-4 flex items-center justify-center shrink-0 text-muted-foreground hover:text-foreground"
          >
            {isExpanded ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
          </button>
        )}
        {node.type === 'file' && <span className="w-4 shrink-0" />}

        {/* Checkbox */}
        {!isVirtual && (
          <button
            className="shrink-0 text-muted-foreground hover:text-foreground opacity-0 group-hover:opacity-100 transition-opacity"
            onClick={(e) => {
              e.stopPropagation()
              onToggleSelect(folderId, e.ctrlKey || e.metaKey)
            }}
          >
            {selectedIds.has(folderId) ? (
              <CheckSquare className="size-3.5 text-primary" />
            ) : (
              <Square className="size-3.5" />
            )}
          </button>
        )}
        {isVirtual && <span className="w-4 shrink-0" />}

        {/* Icon */}
        {node.type === 'directory' ? (
          isExpanded ? <FolderOpen className="size-4 text-amber-500 shrink-0" /> : <Folder className="size-4 text-amber-400 shrink-0" />
        ) : (
          <File className="size-4 text-blue-400 shrink-0" />
        )}

        {/* Name */}
        <span
          className={cn('flex-1 truncate cursor-default select-none', level === 0 && !isVirtual && 'font-medium', isVirtual && 'text-muted-foreground')}
          onClick={() => {
            if (node.type === 'directory') onToggleExpand(folderId)
          }}
        >
          {node.name}
        </span>

        {/* Children count badge */}
        {node.type === 'directory' && totalChildren > 0 && (
          <span className="text-xs text-muted-foreground shrink-0">({totalChildren})</span>
        )}

        {/* Actions */}
        {!isVirtual && (
          <div className="hidden group-hover:flex items-center gap-0.5 shrink-0">
            <button onClick={() => onDelete(folderId)} className="p-0.5 text-muted-foreground hover:text-destructive" title="删除">
              <Trash2 className="size-3" />
            </button>
          </div>
        )}
      </div>

      {/* ── Expanded Children ── */}
      {node.type === 'directory' && isExpanded && (
        <div className="animate-in fade-in-0 slide-in-from-top-1 duration-150">
          {/* Child folders */}
          {childFolders.map((child, i) => (
            <TreeNode
              key={`child-folder-${level + 1}-${i}-${child.id || child.name}`}
              node={child}
              projectId={_projectId}
              level={level + 1}
              expandedIds={expandedIds}
              onToggleExpand={onToggleExpand}
              selectedIds={selectedIds}
              onToggleSelect={onToggleSelect}
              onContextMenu={onContextMenu}
              onDragStart={onDragStart}
              onDragOver={onDragOver}
              onDrop={onDrop}
              onDragEnd={onDragEnd}
              dragState={dragState}
              onDownload={onDownload}
              onDelete={onDelete}
            />
          ))}
          {/* Child files */}
          {childFiles.map((file) => (
            <FileRow
              key={`file-${file.id}`}
              file={file}
              level={level}
              selectedIds={selectedIds}
              onToggleSelect={onToggleSelect}
              onContextMenu={onContextMenu}
              onDragStart={onDragStart}
              onDragOver={onDragOver}
              onDragEnd={onDragEnd}
              onDownload={onDownload}
              onDelete={onDelete}
            />
          ))}
        </div>
      )}
    </div>
  )
}

// ─── Helpers ───

function formatSize(bytes: number): string {
  if (!bytes || bytes === 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function findNodeById(nodes: TreeNodeData[], id: string): TreeNodeData | null {
  for (const node of nodes) {
    if (node.id === id) return node
    if (node.children) {
      const found = findNodeById(node.children, id)
      if (found) return found
    }
  }
  return null
}
