import { type KeyboardEvent } from 'react'
import { FolderPlus, Upload, Home, Trash2, ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'
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
    <div className="border-b bg-card px-4 py-2.5 space-y-2.5">
      {/* Breadcrumb */}
      <div className="flex items-center gap-1 text-sm text-muted-foreground flex-wrap">
        <button
          onClick={() => onBreadcrumbClick(-1, '')}
          className={cn(
            'flex items-center gap-1 px-2 py-0.5 rounded-md hover:bg-muted transition-colors text-xs',
            folderId === '' ? 'bg-muted text-foreground font-medium' : ''
          )}
        >
          <Home className="size-3.5" />
          项目根目录
        </button>
        {bready.map((crumb, i) => (
          <span key={crumb.id} className="flex items-center gap-0.5">
            <ChevronRight className="size-3 text-muted-foreground/50" />
            <button
              onClick={() => onBreadcrumbClick(i, crumb.id)}
              className={cn(
                'px-2 py-0.5 rounded-md hover:bg-muted transition-colors text-xs truncate max-w-[200px]',
                crumb.id === folderId ? 'bg-muted text-foreground font-medium' : ''
              )}
            >
              {crumb.name}
            </button>
          </span>
        ))}
      </div>

      {/* Actions */}
      <div className="flex items-center gap-2 flex-wrap">
        <button
          onClick={onUpload}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors shadow-xs"
        >
          <Upload className="size-3.5" />
          上传文件
        </button>

        {/* Inline new folder input */}
        <div className="flex items-center gap-1 bg-muted/30 rounded-lg p-0.5 border">
          <input
            type="text"
            value={newFolderName}
            onChange={e => onNewFolderNameChange(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="新建文件夹..."
            className="w-28 px-2 py-1 text-xs bg-transparent border-0 focus:outline-none placeholder:text-muted-foreground/50"
          />
          <button
            onClick={onCreateFolder}
            disabled={!newFolderName.trim()}
            className="inline-flex items-center gap-1 px-2 py-1 text-xs font-medium rounded-md hover:bg-background disabled:opacity-40 transition-colors"
          >
            <FolderPlus className="size-3.5" />
            新建
          </button>
        </div>

        {/* Batch delete (when items selected) */}
        {selectedCount > 0 && (
          <button
            onClick={onBatchDelete}
            className="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-medium text-red-600 bg-red-50 dark:bg-red-950/30 rounded-lg hover:bg-red-100 dark:hover:bg-red-950/50 transition-colors"
          >
            <Trash2 className="size-3.5" />
            删除 {selectedCount} 项
          </button>
        )}
      </div>
    </div>
  )
}
