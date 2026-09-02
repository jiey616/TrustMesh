import { useState } from 'react'
import { Paperclip, ChevronDown, ChevronRight, File } from 'lucide-react'
import { Checkbox } from '@/components/ui/checkbox'
import { useProjectFiles } from '@/hooks/useProjectFiles'
import type { ProjectFile, ProjectFileSource } from '@/types'
import { sourceMeta } from '@/lib/fileSource'

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

type FilterKey = 'all' | ProjectFileSource

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'user_upload', label: '我的上传' },
  { key: 'agent_artifact', label: '智能体产物' },
  { key: 'meeting_minutes', label: '会议纪要' },
]

interface FileSelectorProps {
  projectId: string
  selectedIds: string[]
  onToggle: (id: string) => void
  /** 限定单一来源；不传则展示全部项目文件（含智能体产物、会议纪要）。 */
  source?: ProjectFileSource
  label?: string
}

function FileCheckboxItem({
  file,
  checked,
  onToggle,
}: {
  file: ProjectFile
  checked: boolean
  onToggle: (id: string) => void
}) {
  const meta = sourceMeta(file.source)
  return (
    <label className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-muted/50 cursor-pointer text-sm">
      <Checkbox checked={checked} onCheckedChange={() => onToggle(file.id)} />
      <File className="size-3.5 shrink-0 text-muted-foreground" />
      <span className="flex-1 truncate">{file.file_name}</span>
      <span
        className={`shrink-0 text-[10px] leading-none px-1.5 py-0.5 rounded-full border ${meta.cls}`}
      >
        {meta.label}
      </span>
      <span className="shrink-0 text-xs text-muted-foreground">{formatFileSize(file.file_size)}</span>
    </label>
  )
}

export function FileSelector({
  projectId,
  selectedIds,
  onToggle,
  source,
  label = '附加项目文件',
}: FileSelectorProps) {
  const [expanded, setExpanded] = useState(false)
  const locked = source !== undefined
  const [filter, setFilter] = useState<FilterKey>(source ?? 'all')
  const effectiveFilter = locked ? (source as FilterKey) : filter

  const { data: files, isLoading } = useProjectFiles(
    projectId,
    effectiveFilter === 'all' ? undefined : { source: effectiveFilter },
  )

  const fileList = (files ?? []).filter((f) => !f.is_folder)
  const selectedCount = selectedIds.filter((id) => fileList.some((f) => f.id === id)).length

  return (
    <div className="flex flex-col gap-2">
      <button
        type="button"
        className="flex items-center gap-1.5 text-sm font-medium text-muted-foreground hover:text-foreground transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        {expanded ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
        <Paperclip className="size-3.5" />
        <span>{label}</span>
        {selectedCount > 0 && (
          <span className="text-xs text-primary">({selectedCount} 个已选)</span>
        )}
      </button>

      {expanded && (
        <div className="rounded-md border bg-muted/30 overflow-hidden">
          {!locked && (
            <div className="flex items-center gap-1 px-2 py-1.5 border-b bg-muted/40">
              {FILTERS.map((f) => (
                <button
                  key={f.key}
                  type="button"
                  onClick={() => setFilter(f.key)}
                  className={`text-[11px] px-2 py-0.5 rounded-full border transition-colors ${
                    filter === f.key
                      ? 'bg-primary/10 border-primary text-primary'
                      : 'border-transparent text-muted-foreground hover:text-foreground'
                  }`}
                >
                  {f.label}
                </button>
              ))}
            </div>
          )}
          <div className="max-h-48 overflow-y-auto">
            {isLoading ? (
              <p className="px-3 py-4 text-xs text-muted-foreground text-center">加载中...</p>
            ) : fileList.length === 0 ? (
              <p className="px-3 py-4 text-xs text-muted-foreground text-center">
                暂无项目文件
              </p>
            ) : (
              fileList.map((file) => (
                <FileCheckboxItem
                  key={file.id}
                  file={file}
                  checked={selectedIds.includes(file.id)}
                  onToggle={onToggle}
                />
              ))
            )}
          </div>
        </div>
      )}
    </div>
  )
}
