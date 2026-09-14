import { MessageInput } from '@/components/task-thread/MessageInput'
import { UIResponsePanel } from '@/components/task-thread/UIResponsePanel'
import { TaskCommentComposer, type TaskCommentSubmitInput, type TaskMentionCandidate } from './TaskCommentComposer'
import type { UIBlock, UIResponse } from '@/types'

interface TaskComposerProps {
  mode: 'planning' | 'building'
  disabled?: boolean
  pendingUIBlocks?: UIBlock[] | null
  planningPlaceholder?: string
  buildingCandidates?: TaskMentionCandidate[]
  onPlanningSubmit?: (content: string, uiResponse?: UIResponse) => Promise<void> | void
  onBuildingSubmit?: (input: TaskCommentSubmitInput) => Promise<boolean> | boolean
}

export function TaskComposer({
  mode,
  disabled,
  pendingUIBlocks,
  planningPlaceholder = '继续补充需求或回答 PM 的问题...',
  buildingCandidates = [],
  onPlanningSubmit,
  onBuildingSubmit,
}: TaskComposerProps) {
  if (mode === 'planning') {
    if (pendingUIBlocks && pendingUIBlocks.length > 0) {
      return (
        <UIResponsePanel
          blocks={pendingUIBlocks}
          onSubmit={(content, uiResponse) => onPlanningSubmit?.(content, uiResponse)}
          disabled={disabled}
        />
      )
    }

    return (
      <MessageInput
        onSend={(content) => onPlanningSubmit?.(content)}
        disabled={disabled}
        placeholder={planningPlaceholder}
      />
    )
  }

  return (
    <TaskCommentComposer
      candidates={buildingCandidates}
      disabled={disabled}
      onSubmit={(input) => onBuildingSubmit?.(input) ?? false}
    />
  )
}
