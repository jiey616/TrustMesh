import { useState } from 'react'
import { ChevronRight, Folder, FolderOpen, Home, FileText, Image, FileSpreadsheet, File, Archive } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useProjectFileTree } from '@/hooks/useProjectFiles'
import type { BreadcrumbNode, ProjectFileTreeNode } from '@/types'

interface Props {
  projectId: string
  currentFolderId: string
  onNavigate: (folderId: string, breadcrumbs: BreadcrumbNode[]) => void
}

function guessFileIcon(name: string) {
  const ext = name.split('.').pop()?.toLowerCase()
  if (!ext) return <File className="size-3.5 text-muted-foreground" />
  if (['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg'].includes(ext)) return <Image className="size-3.5 text-blue-400" />
  if (['pdf'].includes(ext)) return <FileText className="size-3.5 text-red-400" />
  if (['xls', 'xlsx', 'csv'].includes(ext)) return <FileSpreadsheet className="size-3.5 text-green-500" />
  if (['doc', 'docx'].includes(ext)) return <FileText className="size-3.5 text-blue-500" />
  if (['zip', 'rar', '7z', 'tar', 'gz'].includes(ext)) return <Archive className="size-3.5 text-purple-400" />
  if (['md', 'txt', 'json', 'yaml', 'yml', 'toml', 'xml'].includes(ext)) return <FileText className="size-3.5 text-sky-400" />
  if (['js', 'ts', 'py', 'go', 'rs', 'java', 'cpp', 'c', 'h'].includes(ext)) return <FileText className="size-3.5 text-emerald-400" />
  return <File className="size-3.5 text-muted-foreground" />
}

export function ExplorerSidebar({ projectId, currentFolderId, onNavigate }: Props) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set())

  const { data: tree, isLoading: treeLoading } = useProjectFileTree(projectId)

  const toggle = (id: string) => {
    setExpanded(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  // Auto-expand first directory on first load
  const firstDir = tree?.uploads?.find(n => n.type === 'directory' && n.id)
  const [initialized, setInitialized] = useState(false)
  if (!initialized && firstDir) {
    setExpanded(prev => new Set(prev).add(firstDir.id!))
    setInitialized(true)
  } else if (!initialized && tree) {
    // No directories to expand, mark as initialized
    setInitialized(true)
  }

  const renderNode = (node: ProjectFileTreeNode, depth: number, path: BreadcrumbNode[]) => {
    const nodeKey = node.id || `${depth}-${node.name}`
    const isExpanded = expanded.has(nodeKey)
    const isDirectory = node.type === 'directory'
    const hasChildren = isDirectory && ((node.children && node.children.length > 0) || (node.files && node.files.length > 0))
    const isNavigable = !!node.id && isDirectory
    const isActive = !!(node.id && node.id === currentFolderId)
    const itemCount = (node.children?.length ?? 0) + (node.files?.length ?? 0)

    return (
      <div key={nodeKey}>
        <div
          className={cn(
            'group flex items-center gap-1 px-2 py-1 text-xs rounded-md cursor-pointer transition-all',
            isActive
              ? 'bg-primary/10 text-primary font-medium shadow-xs'
              : 'text-foreground hover:bg-muted/60 hover:text-foreground'
          )}
          style={{ paddingLeft: `${depth * 16 + 8}px` }}
          onClick={() => {
            if (isNavigable) {
              const newPath = [...path, { id: node.id!, name: node.name }]
              onNavigate(node.id!, newPath)
            } else {
              toggle(nodeKey)
            }
          }}
        >
          {/* Expand/collapse chevron */}
          <span className="shrink-0 w-4 flex items-center justify-center">
            {hasChildren ? (
              <ChevronRight
                className={cn('size-3 transition-transform', isExpanded ? 'rotate-90' : '')}
                onClick={(e) => { e.stopPropagation(); toggle(nodeKey) }}
              />
            ) : (
              <span className="w-3" />
            )}
          </span>

          {/* Folder icon */}
          <span className="shrink-0">
            {isExpanded ? (
              <FolderOpen className={cn('size-3.5', isActive ? 'text-primary' : 'text-amber-500')} />
            ) : (
              <Folder className={cn('size-3.5', isActive ? 'text-primary' : 'text-amber-500')} />
            )}
          </span>

          {/* Name */}
          <span className="truncate flex-1 ml-0.5">{node.name}</span>

          {/* Count badge */}
          {hasChildren && itemCount > 0 && (
            <span className={cn(
              'text-[10px] shrink-0 px-1 rounded',
              isActive ? 'bg-primary/15 text-primary' : 'text-muted-foreground/60 group-hover:text-muted-foreground'
            )}>
              {itemCount}
            </span>
          )}
        </div>

        {/* Children */}
        {isExpanded && node.children?.map(child =>
          renderNode(child, depth + 1, isNavigable ? [...path, { id: node.id!, name: node.name }] : path)
        )}
        {/* Files within folder (leaf nodes) */}
        {isExpanded && node.files?.map(file => (
          <div
            key={file.file_name}
            className={cn(
              'flex items-center gap-1 px-2 py-1 text-xs rounded-md cursor-default transition-colors hover:bg-muted/30'
            )}
            style={{ paddingLeft: `${(depth + 1) * 16 + 8}px` }}
          >
            <span className="shrink-0 w-4 flex items-center justify-center">
              <span className="w-3" />
            </span>
            {guessFileIcon(file.file_name)}
            <span className="truncate ml-0.5 text-muted-foreground">{file.file_name}</span>
          </div>
        ))}
      </div>
    )
  }

  return (
    <div className="w-56 border-r bg-card flex flex-col min-h-0 shrink-0">
      {/* Header */}
      <div className="px-3 py-2.5 border-b bg-muted/20">
        <div className="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
          <Folder className="size-3.5" />
          项目文件
        </div>
      </div>

      {/* Tree */}
      <div className="flex-1 overflow-auto py-1.5">
        {treeLoading ? (
          <div className="flex items-center justify-center py-8 text-xs text-muted-foreground">
            <div className="flex items-center gap-1.5">
              <div className="size-3 border-2 border-muted-foreground/30 border-t-muted-foreground rounded-full animate-spin" />
              加载中...
            </div>
          </div>
        ) : (
          <div className="space-y-0.5 px-1">
            {/* Root folder shortcut */}
            <div
              className={cn(
                'flex items-center gap-1.5 px-2 py-1.5 text-xs rounded-md cursor-pointer transition-all',
                currentFolderId === ''
                  ? 'bg-primary/10 text-primary font-medium shadow-xs'
                  : 'text-foreground hover:bg-muted/60'
              )}
              onClick={() => onNavigate('', [])}
            >
              <span className="shrink-0 w-4 flex items-center justify-center">
                <Home className={cn('size-3', currentFolderId === '' ? 'text-muted-foreground' : 'text-muted-foreground')} />
              </span>
              <span className={cn('truncate', currentFolderId === '' ? 'font-medium' : '')}>项目根目录</span>
            </div>

            {/* Separator */}
            <div className="border-t border-muted/30 my-1 mx-2" />

            {/* Tree nodes */}
            {tree?.uploads?.map(node => renderNode(node, 0, []))}
          </div>
        )}
      </div>
    </div>
  )
}