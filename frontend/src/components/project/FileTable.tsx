import { useState, useCallback } from 'react'
import { Folder, MoreVertical, Download, Eye, Pencil, Trash2, Bot } from 'lucide-react'
import type { ProjectFile, VirtualFolder } from '@/types'
import { cn } from '@/lib/utils'

// ─── Helpers ───

function formatSize(bytes: number): string {
  if (bytes === 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

function mimeIcon(mime: string): string {
  if (!mime) return '📄'
  if (mime.startsWith('image/')) return '🖼️'
  if (mime.includes('pdf')) return '📑'
  if (mime.includes('spreadsheet') || mime.includes('excel') || mime.includes('csv')) return '📊'
  if (mime.includes('presentation') || mime.includes('powerpoint')) return '📽️'
  if (mime.startsWith('text/')) return '📝'
  if (mime.includes('zip') || mime.includes('tar') || mime.includes('rar')) return '📦'
  return '📄'
}

// ─── ContextMenu ───

interface ContextMenuState {
  x: number
  y: number
  item: ProjectFile
}

function ContextMenuDropdown({ state, onPreview, onDownload, onRename, onDelete, onClose }: {
  state: ContextMenuState
  onPreview: (f: ProjectFile) => void
  onDownload: (f: ProjectFile) => void
  onRename: (f: ProjectFile) => void
  onDelete: (f: ProjectFile) => void
  onClose: () => void
}) {
  return (
    <>
      {/* Backdrop */}
      <div className="fixed inset-0 z-40" onClick={onClose} onContextMenu={(e) => { e.preventDefault(); onClose() }} />
      {/* Menu */}
      <div
        className="fixed z-50 bg-popover border rounded-lg shadow-lg py-1 min-w-[140px] text-sm"
        style={{ left: state.x, top: state.y }}
      >
        {/* Preview (files only) */}
        {!state.item.is_folder && (
          <button
            className="flex items-center gap-2 px-3 py-1.5 w-full hover:bg-muted transition-colors text-left"
            onClick={() => { onPreview(state.item); onClose() }}
          >
            <Eye className="size-3.5" />
            预览
          </button>
        )}
        {/* Download (files only) */}
        {!state.item.is_folder && (
          <button
            className="flex items-center gap-2 px-3 py-1.5 w-full hover:bg-muted transition-colors text-left"
            onClick={() => { onDownload(state.item); onClose() }}
          >
            <Download className="size-3.5" />
            下载
          </button>
        )}
        <button
          className="flex items-center gap-2 px-3 py-1.5 w-full hover:bg-muted transition-colors text-left"
          onClick={() => { onRename(state.item); onClose() }}
        >
          <Pencil className="size-3.5" />
          重命名
        </button>
        <div className="border-t my-1" />
        <button
          className="flex items-center gap-2 px-3 py-1.5 w-full hover:bg-red-50 text-red-600 transition-colors text-left"
          onClick={() => { onDelete(state.item); onClose() }}
        >
          <Trash2 className="size-3.5" />
          删除
        </button>
      </div>
    </>
  )
}

// ─── Props ───

interface Props {
  virtualFolders?: VirtualFolder[]
  folders: ProjectFile[]
  files: ProjectFile[]
  selectedIds: Set<string>
  onSelectedChange: (ids: Set<string>) => void
  onFolderClick: (folder: ProjectFile) => void
  onVirtualFolderClick: (vf: VirtualFolder) => void
  onFilePreview: (file: ProjectFile) => void
  onFileDoubleClick: (file: ProjectFile) => void
  onDownload: (file: ProjectFile) => void
  onRename: (item: { id: string; name: string; isFolder: boolean }) => void
  onDelete: (file: ProjectFile) => void
}

// ─── Component ───

export function FileTable({
  virtualFolders = [],
  folders,
  files,
  selectedIds,
  onSelectedChange,
  onFolderClick,
  onVirtualFolderClick,
  onFilePreview,
  onFileDoubleClick,
  onDownload,
  onRename,
  onDelete,
}: Props) {
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)
  const [hoveredId, setHoveredId] = useState<string | null>(null)

  const allItems = [...virtualFolders.map(vf => ({ id: vf.id, is_folder: true })), ...folders, ...files]
  const allSelected = allItems.length > 0 && selectedIds.size === allItems.length

  const toggleSelect = useCallback((id: string, ctrl: boolean) => {
    const next = new Set(selectedIds)
    if (ctrl) {
      if (next.has(id)) next.delete(id)
      else next.add(id)
    } else {
      if (next.has(id)) next.delete(id)
      else { next.clear(); next.add(id) }
    }
    onSelectedChange(next)
  }, [selectedIds, onSelectedChange])

  const toggleSelectAll = useCallback(() => {
    if (allSelected) {
      onSelectedChange(new Set())
    } else {
      onSelectedChange(new Set(allItems.map(i => i.id)))
    }
  }, [allSelected, allItems, onSelectedChange])

  const handleContextMenu = useCallback((e: React.MouseEvent, item: ProjectFile) => {
    e.preventDefault()
    setContextMenu({ x: e.clientX, y: e.clientY, item })
  }, [])

  const handleRename = useCallback((item: ProjectFile) => {
    onRename({ id: item.id, name: item.file_name, isFolder: item.is_folder })
  }, [onRename])

  if (allItems.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-2">
        <Folder className="size-10 opacity-20" />
        <p className="text-sm">此文件夹为空</p>
        <p className="text-xs">上传文件或新建文件夹以开始</p>
      </div>
    )
  }

  return (
    <>
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b bg-muted/30 text-left text-xs text-muted-foreground">
            <th className="w-8 px-3 py-2">
              <input
                type="checkbox"
                checked={allSelected}
                onChange={toggleSelectAll}
                className="size-3.5 rounded border-muted-foreground/30 accent-primary"
              />
            </th>
            <th className="px-3 py-2 font-medium">名称</th>
            <th className="px-3 py-2 font-medium w-24">大小</th>
            <th className="px-3 py-2 font-medium w-20">类型</th>
            <th className="px-3 py-2 font-medium w-16">来源</th>
            <th className="px-3 py-2 font-medium w-28">日期</th>
            <th className="w-8 px-2 py-2" />
          </tr>
        </thead>
        <tbody>
          {/* Virtual folders (artifact hierarchy) */}
          {virtualFolders.map(vf => (
            <tr
              key={vf.id}
              className="border-b border-muted/30 cursor-pointer transition-colors hover:bg-muted/20"
              onClick={() => onVirtualFolderClick(vf)}
            >
              <td className="px-3 py-2" onClick={(e) => e.stopPropagation()}>
                <input
                  type="checkbox"
                  checked={selectedIds.has(vf.id)}
                  onChange={() => toggleSelect(vf.id, false)}
                  onClick={(e) => { e.stopPropagation(); toggleSelect(vf.id, e.ctrlKey || e.metaKey) }}
                  className="size-3.5 rounded border-muted-foreground/30 accent-primary"
                />
              </td>
              <td className="px-3 py-2">
                <div className="flex items-center gap-2">
                  <Bot className="size-4 shrink-0 text-blue-500" />
                  <span className="font-medium truncate max-w-[300px]">{vf.name}</span>
                </div>
              </td>
              <td className="px-3 py-2 text-muted-foreground text-xs">
                {vf.item_count != null ? `${vf.item_count} 项` : '-'}
              </td>
              <td className="px-3 py-2 text-muted-foreground text-xs">智能体产物</td>
              <td className="px-3 py-2 text-muted-foreground text-xs">🤖 系统</td>
              <td className="px-3 py-2 text-muted-foreground text-xs">-</td>
              <td className="w-8 px-2 py-2" />
            </tr>
          ))}

          {/* Folders first */}
          {folders.map(folder => (
            <tr
              key={folder.id}
              className={cn(
                'border-b border-muted/30 cursor-pointer transition-colors',
                selectedIds.has(folder.id) ? 'bg-primary/5' : hoveredId === folder.id ? 'bg-muted/20' : 'hover:bg-muted/20'
              )}
              onMouseEnter={() => setHoveredId(folder.id)}
              onMouseLeave={() => setHoveredId(null)}
              onClick={(e) => {
                if (e.ctrlKey || e.metaKey) {
                  toggleSelect(folder.id, true)
                } else {
                  onFolderClick(folder)
                }
              }}
              onContextMenu={(e) => handleContextMenu(e, folder)}
            >
              <td className="px-3 py-2" onClick={(e) => e.stopPropagation()}>
                <input
                  type="checkbox"
                  checked={selectedIds.has(folder.id)}
                  onChange={() => toggleSelect(folder.id, false)}
                  onClick={(e) => { e.stopPropagation(); toggleSelect(folder.id, e.ctrlKey || e.metaKey) }}
                  className="size-3.5 rounded border-muted-foreground/30 accent-primary"
                />
              </td>
              <td className="px-3 py-2">
                <div className="flex items-center gap-2">
                  <Folder className="size-4 shrink-0 text-amber-500" />
                  <span className="font-medium truncate max-w-[300px]">{folder.file_name}</span>
                </div>
              </td>
              <td className="px-3 py-2 text-muted-foreground text-xs">-</td>
              <td className="px-3 py-2 text-muted-foreground text-xs">文件夹</td>
              <td className="px-3 py-2 text-muted-foreground text-xs">👤 用户</td>
              <td className="px-3 py-2 text-muted-foreground text-xs">{formatDate(folder.created_at)}</td>
              <td className="px-2 py-2">
                <button
                  className="p-0.5 rounded hover:bg-muted transition-colors opacity-0 group-hover:opacity-100"
                  style={{ opacity: hoveredId === folder.id ? 1 : 0 }}
                  onClick={(e) => { e.stopPropagation(); handleContextMenu(e as any, folder) }}
                >
                  <MoreVertical className="size-3.5 text-muted-foreground" />
                </button>
              </td>
            </tr>
          ))}

          {/* Files */}
          {files.map(file => (
            <tr
              key={file.id}
              className={cn(
                'border-b border-muted/30 cursor-pointer transition-colors',
                selectedIds.has(file.id) ? 'bg-primary/5' : hoveredId === file.id ? 'bg-muted/20' : 'hover:bg-muted/20'
              )}
              onMouseEnter={() => setHoveredId(file.id)}
              onMouseLeave={() => setHoveredId(null)}
              onClick={(e) => {
                if (e.ctrlKey || e.metaKey) {
                  toggleSelect(file.id, true)
                }
              }}
              onDoubleClick={() => onFileDoubleClick(file)}
              onContextMenu={(e) => handleContextMenu(e, file)}
            >
              <td className="px-3 py-2" onClick={(e) => e.stopPropagation()}>
                <input
                  type="checkbox"
                  checked={selectedIds.has(file.id)}
                  onChange={() => toggleSelect(file.id, false)}
                  onClick={(e) => { e.stopPropagation(); toggleSelect(file.id, e.ctrlKey || e.metaKey) }}
                  className="size-3.5 rounded border-muted-foreground/30 accent-primary"
                />
              </td>
              <td className="px-3 py-2">
                <div className="flex items-center gap-2">
                  <span className="text-sm">{mimeIcon(file.mime_type)}</span>
                  <span className="truncate max-w-[300px]">{file.file_name}</span>
                </div>
              </td>
              <td className="px-3 py-2 text-muted-foreground text-xs">{formatSize(file.file_size)}</td>
              <td className="px-3 py-2 text-muted-foreground text-xs">
                {file.mime_type ? file.mime_type.split('/')[1]?.toUpperCase() || file.mime_type : '-'}
              </td>
              <td className="px-3 py-2 text-muted-foreground text-xs">
                {file.source === 'agent_artifact' ? '🤖 智能体' : '👤 用户'}
              </td>
              <td className="px-3 py-2 text-muted-foreground text-xs">{formatDate(file.created_at)}</td>
              <td className="px-2 py-2">
                <button
                  className="p-0.5 rounded hover:bg-muted transition-colors"
                  style={{ opacity: hoveredId === file.id ? 1 : 0 }}
                  onClick={(e) => { e.stopPropagation(); handleContextMenu(e as any, file) }}
                >
                  <MoreVertical className="size-3.5 text-muted-foreground" />
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {/* Context menu */}
      {contextMenu && (
        <ContextMenuDropdown
          state={contextMenu}
          onPreview={onFilePreview}
          onDownload={onDownload}
          onRename={(f) => handleRename(f)}
          onDelete={onDelete}
          onClose={() => setContextMenu(null)}
        />
      )}
    </>
  )
}
