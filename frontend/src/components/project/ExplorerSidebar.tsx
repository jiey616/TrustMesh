import { useState } from 'react'
import { ChevronRight, Folder, FolderOpen } from 'lucide-react'
import { useProjectFileTree } from '@/hooks/useProjectFiles'
import type { BreadcrumbNode, ProjectFileTreeNode } from '@/types'

interface Props {
  projectId: string
  currentFolderId: string
  onNavigate: (folderId: string, breadcrumbs: BreadcrumbNode[]) => void
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

  const renderNode = (node: ProjectFileTreeNode, depth: number, path: BreadcrumbNode[]) => {
    const nodeKey = node.id || `${depth}-${node.name}`
    const isExpanded = expanded.has(nodeKey)
    const hasChildren = (node.children && node.children.length > 0) || (node.files && node.files.length > 0)
    const isNavigable = !!node.id && (node.type === 'directory')

    return (
      <div key={nodeKey}>
        <div
          className={`flex items-center gap-1 px-2 py-1 text-xs rounded cursor-pointer hover:bg-muted transition-colors ${
            (node.id && node.id === currentFolderId) ? 'bg-primary/10 text-primary font-medium' : 'text-foreground'
          }`}
          style={{ paddingLeft: `${8 + depth * 14}px` }}
          onClick={() => {
            if (isNavigable) {
              const newPath = [...path, { id: node.id!, name: node.name }]
              onNavigate(node.id!, newPath)
            } else {
              toggle(nodeKey)
            }
          }}
        >
          {hasChildren && (
            <ChevronRight
              className={`size-3 shrink-0 transition-transform ${isExpanded ? 'rotate-90' : ''}`}
              onClick={(e) => { e.stopPropagation(); toggle(nodeKey) }}
            />
          )}
          {isExpanded ? (
            <FolderOpen className="size-3.5 shrink-0 text-amber-500" />
          ) : (
            <Folder className="size-3.5 shrink-0 text-amber-500" />
          )}
          <span className="truncate">{node.name}</span>
          {hasChildren && (
            <span className="text-muted-foreground ml-auto text-[10px]">
              {(node.children?.length ?? 0) + (node.files?.length ?? 0)}
            </span>
          )}
        </div>
        {isExpanded && (
          <>
            {node.children?.map(child =>
              renderNode(child, depth + 1, isNavigable ? [...path, { id: node.id!, name: node.name }] : path)
            )}
          </>
        )}
      </div>
    )
  }

  return (
    <div className="w-52 border-r bg-muted/20 flex flex-col min-h-0 shrink-0">
      <div className="flex-1 overflow-auto py-2">
        <div className="px-3 py-1.5">
          <div className="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-1">
            <Folder className="size-3" />
            项目文件
          </div>
          {treeLoading ? (
            <div className="text-xs text-muted-foreground px-2 py-2">加载中...</div>
          ) : (
            <>
              {/* Root folder shortcut */}
              <div
                className={`flex items-center gap-1.5 px-2 py-1 mt-0.5 text-xs rounded cursor-pointer hover:bg-muted transition-colors ${
                  currentFolderId === '' ? 'bg-primary/10 text-primary font-medium' : 'text-foreground'
                }`}
                onClick={() => onNavigate('', [])}
              >
                <Folder className="size-3.5 shrink-0 text-amber-500" />
                <span className="truncate">项目根目录</span>
              </div>
              {/* Tree nodes (includes user folders + artifact subtree merged) */}
              {tree?.uploads?.map(node => renderNode(node, 0, []))}
            </>
          )}
        </div>
      </div>
    </div>
  )
}
