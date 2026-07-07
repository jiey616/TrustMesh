import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'
import {
  FolderPlus,
  Pencil,
  Trash2,
  Download,
} from 'lucide-react'

export interface ContextMenuTarget {
  fileId: string
  fileName: string
  isFolder: boolean
}

interface FileTreeContextMenuProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  target: ContextMenuTarget | null
  onNewFolder: (parentId: string) => void
  onRename: (fileId: string) => void
  onDelete: (fileId: string) => void
  onDownload?: (fileId: string) => void
}

export function FileTreeContextMenu({
  open,
  onOpenChange,
  target,
  onNewFolder,
  onRename,
  onDelete,
  onDownload,
}: FileTreeContextMenuProps) {
  if (!target) return null

  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <div className="fixed" />
      <DropdownMenuContent
        className="w-44"
        align="start"
      >
        {target.isFolder && (
          <>
            <DropdownMenuItem
              onSelect={() => onNewFolder(target.fileId)}
            >
              <FolderPlus className="size-4" />
              新建子文件夹
            </DropdownMenuItem>
            <DropdownMenuSeparator />
          </>
        )}
        {!target.isFolder && onDownload && (
          <>
            <DropdownMenuItem
              onSelect={() => onDownload(target.fileId)}
            >
              <Download className="size-4" />
              下载
            </DropdownMenuItem>
            <DropdownMenuSeparator />
          </>
        )}
        <DropdownMenuItem
          onSelect={() => onRename(target.fileId)}
        >
          <Pencil className="size-4" />
          重命名
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          variant="destructive"
          onSelect={() => onDelete(target.fileId)}
        >
          <Trash2 className="size-4" />
          删除
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
