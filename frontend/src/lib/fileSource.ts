import type { ProjectFileSource } from '@/types'

export interface SourceMeta {
  label: string
  cls: string
}

/** 项目文件来源的中文标签与配色，用于选择器/列表的来源徽章。 */
export const SOURCE_META: Record<string, SourceMeta> = {
  user_upload: {
    label: '我的上传',
    cls: 'bg-blue-500/10 text-blue-600 border-blue-500/20',
  },
  agent_artifact: {
    label: '智能体产物',
    cls: 'bg-purple-500/10 text-purple-600 border-purple-500/20',
  },
  meeting_minutes: {
    label: '会议纪要',
    cls: 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20',
  },
}

export function sourceMeta(src?: string | ProjectFileSource): SourceMeta {
  return (
    SOURCE_META[src ?? ''] ?? {
      label: '文件',
      cls: 'bg-muted text-muted-foreground border-border/60',
    }
  )
}
