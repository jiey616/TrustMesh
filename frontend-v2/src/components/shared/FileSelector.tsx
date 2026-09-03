import { useState } from 'react'
import { Checkbox, Tag, Empty, Spin } from 'antd'
import { PaperClipOutlined } from '@ant-design/icons'
import { useProjectFiles } from '@/hooks/useProjectFiles'
import type { ProjectFile, ProjectFileSource } from '@/types'

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

const sourceLabel: Record<ProjectFileSource, string> = {
  user_upload: '我的上传',
  agent_artifact: '数字员工产物',
  meeting_minutes: '会议纪要',
}

interface FileSelectorProps {
  projectId: string
  selectedIds: string[]
  onToggle: (id: string) => void
  source?: ProjectFileSource
  label?: string
}

export function FileSelector({
  projectId,
  selectedIds,
  onToggle,
  label = '附加项目文件',
}: FileSelectorProps) {
  const [expanded, setExpanded] = useState(false)
  const { data: files, isLoading } = useProjectFiles(projectId)
  const fileList = (files ?? []).filter((f) => !f.is_folder)
  const selectedCount = selectedIds.filter((id) => fileList.some((f) => f.id === id)).length

  return (
    <div className="flex flex-col gap-2">
      <button
        type="button"
        className="flex items-center gap-1.5 text-sm font-medium text-white/70 hover:text-white transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        <PaperClipOutlined />
        <span>{label}</span>
        {selectedCount > 0 && <span className="text-xs text-[color:var(--signal)]">({selectedCount} 个已选)</span>}
      </button>

      {expanded && (
        <div className="rounded-lg border border-white/10 bg-white/[0.03] p-2">
          {isLoading ? (
            <div className="py-4 text-center">
              <Spin size="small" />
            </div>
          ) : fileList.length === 0 ? (
            <Empty description="暂无项目文件" image={Empty.PRESENTED_IMAGE_SIMPLE} />
          ) : (
            <div className="max-h-48 space-y-0.5 overflow-y-auto">
              {fileList.map((file: ProjectFile) => (
                <label
                  key={file.id}
                  className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-white/5"
                >
                  <Checkbox
                    checked={selectedIds.includes(file.id)}
                    onChange={() => onToggle(file.id)}
                  />
                  <span className="flex-1 truncate text-white/80">{file.file_name}</span>
                  <Tag className="!text-[10px]" color="default">
                    {sourceLabel[file.source] ?? file.source}
                  </Tag>
                  <span className="shrink-0 text-xs text-white/40">{formatFileSize(file.file_size)}</span>
                </label>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
