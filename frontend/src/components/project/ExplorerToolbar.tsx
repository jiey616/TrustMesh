import { type KeyboardEvent } from 'react'
import { FolderPlus, Upload, Home, Trash2, ChevronRight } from 'lucide-react'
import type { BreadcrumbNode } from '@/types'

interface Props {
  bready: BreadcrumbNode[]
  folderId: string
  selectedCount: number
  onBreadcrumbClick: (index: number, id: string) => void
  onUpload: () => void
  onBatchDelete: () => void
  newFolderName: string
  onNewFolderNameChange: (v: string) => void
  onCreateFolder: () => void
}

export function ExplorerToolbar({
  bready,
  folderId,
  selectedCount,
  onBreadcrumbClick,
  onUpload,
  onBatchDelete,
  newFolderName,
  onNewFolderNameChange,
  onCreateFolder,
}: Props) {
  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') onCreateFolder()
    if (e.key === 'Escape') onNewFolderNameChange('')
  }

  return (
    <div className="border-b bg-background px-3 py-2 space-y-2">
      {/* Breadcrumb */}
      <div className="flex items-center gap-1 text-sm text-muted-foreground">
        <button
          onClick={() => onBreadcrumbClick(-1, '')}
          className={`flex items-center gap-1 px-1.5 py-0.5 rounded hover:bg-muted transition-colors ${
            folderId === '' ? 'text-foreground font-medium' : ''
          }`}
        >
          <Home className="size-3.5" />
          项目根目录
        </button>
        {bready.map((crumb, i) => (
          <span key={crumb.id} className="flex items-center gap-1">
            <ChevronRight className="size-3" />
            <button
              onClick={() => onBreadcrumbClick(i, crumb.id)}
              className={`px-1.5 py-0.5 rounded hover:bg-muted transition-colors truncate max-w-[200px] ${
                crumb.id === folderId ? 'text-foreground font-medium' : ''
              }`}
            >
              {crumb.name}
            </button>
          </span>
        ))}
      </div>

      {/* Actions */}
      <div className="flex items-center gap-2">
        <button
          onClick={onUpload}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium bg-primary text-primary-foreground rounded-md hover:bg-primary/90 transition-colors"
        >
          <Upload className="size-3.5" />
          上传文件
        </button>

        {/* Inline new folder input */}
        <div className="flex items-center gap-1">
          <input
            type="text"
            value={newFolderName}
            onChange={e => onNewFolderNameChange(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="新建文件夹..."
            className="w-32 px-2 py-1.5 text-xs border rounded-md bg-background focus:outline-none focus:ring-1 focus:ring-primary"
          />
          <button
            onClick={onCreateFolder}
            disabled={!newFolderName.trim()}
            className="inline-flex items-center gap-1 px-2 py-1.5 text-xs font-medium border rounded-md hover:bg-muted disabled:opacity-40 transition-colors"
          >
            <FolderPlus className="size-3.5" />
            新建
          </button>
        </div>

        {/* Batch delete (when items selected) */}
        {selectedCount > 0 && (
          <button
            onClick={onBatchDelete}
            className="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-medium text-red-600 border border-red-200 rounded-md hover:bg-red-50 transition-colors"
          >
            <Trash2 className="size-3.5" />
            删除 {selectedCount} 项
          </button>
        )}
      </div>
    </div>
  )
}
