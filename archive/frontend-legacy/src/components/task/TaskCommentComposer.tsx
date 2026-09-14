import { useEffect, useRef, useState } from 'react'
import { AtSign, Send } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export interface TaskMentionCandidate {
  id: string
  name: string
  roleLabel: string
}

export interface TaskCommentSubmitInput {
  content: string
  mentionAgentIds: string[]
}

interface MentionSearch {
  start: number
  end: number
  query: string
}

interface TaskCommentComposerProps {
  candidates: TaskMentionCandidate[]
  disabled?: boolean
  onSubmit: (input: TaskCommentSubmitInput) => Promise<boolean> | boolean
  placeholder?: string
}

function getMentionSearch(value: string, cursor: number): MentionSearch | null {
  const prefix = value.slice(0, cursor)
  const match = /(?:^|\s)@([^\s@]*)$/.exec(prefix)
  if (!match) {
    return null
  }

  return {
    start: cursor - match[1].length - 1,
    end: cursor,
    query: match[1],
  }
}

export function TaskCommentComposer({
  candidates,
  disabled,
  onSubmit,
  placeholder = '输入评论... (输入 @ 提及 Agent，Enter 发送，Shift+Enter 换行)',
}: TaskCommentComposerProps) {
  const [value, setValue] = useState('')
  const [isComposing, setIsComposing] = useState(false)
  const [mentionSearch, setMentionSearch] = useState<MentionSearch | null>(null)
  const [activeIndex, setActiveIndex] = useState(0)
  const [selectedMentions, setSelectedMentions] = useState<Record<string, string>>({})
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const filteredCandidates = mentionSearch
    ? candidates
      .filter((candidate) => {
        const query = mentionSearch.query.trim().toLowerCase()
        if (!query) {
          return true
        }
        return candidate.name.toLowerCase().includes(query) || candidate.roleLabel.toLowerCase().includes(query)
      })
      .slice(0, 6)
    : []

  useEffect(() => {
    const textarea = textareaRef.current
    if (!textarea) {
      return
    }
    textarea.style.height = 'auto'
    textarea.style.height = `${Math.min(textarea.scrollHeight, 120)}px`
  }, [value])

  const updateMentionSearch = (nextValue: string, cursor: number | null | undefined) => {
    if (cursor == null) {
      setMentionSearch(null)
      return
    }
    setMentionSearch(getMentionSearch(nextValue, cursor))
  }

  const handleSelectCandidate = (candidate: TaskMentionCandidate) => {
    if (!mentionSearch) {
      return
    }

    const nextValue = `${value.slice(0, mentionSearch.start)}@${candidate.name} ${value.slice(mentionSearch.end)}`
    const nextCursor = mentionSearch.start + candidate.name.length + 2
    setValue(nextValue)
    setSelectedMentions((current) => ({ ...current, [candidate.id]: candidate.name }))
    setMentionSearch(null)
    setActiveIndex(0)

    requestAnimationFrame(() => {
      const textarea = textareaRef.current
      if (!textarea) {
        return
      }
      textarea.focus()
      textarea.setSelectionRange(nextCursor, nextCursor)
    })
  }

  const handleSubmit = async () => {
    const trimmed = value.trim()
    if (!trimmed || disabled) {
      return
    }

    const mentionAgentIds = Object.entries(selectedMentions)
      .filter(([, name]) => trimmed.includes(`@${name}`))
      .map(([agentId]) => agentId)

    const ok = await onSubmit({ content: trimmed, mentionAgentIds })
    if (!ok) {
      return
    }

    setValue('')
    setSelectedMentions({})
    setMentionSearch(null)
    setActiveIndex(0)
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }
  }

  const handleKeyDown = async (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    const safeActiveIndex = filteredCandidates.length === 0 ? 0 : Math.min(activeIndex, filteredCandidates.length - 1)

    if (mentionSearch && filteredCandidates.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setActiveIndex((current) => (current + 1) % filteredCandidates.length)
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setActiveIndex((current) => (current - 1 + filteredCandidates.length) % filteredCandidates.length)
        return
      }
      if ((e.key === 'Enter' || e.key === 'Tab') && !e.shiftKey && !isComposing) {
        e.preventDefault()
        handleSelectCandidate(filteredCandidates[safeActiveIndex] ?? filteredCandidates[0])
        return
      }
    }

    if (e.key === 'Escape' && mentionSearch) {
      e.preventDefault()
      setMentionSearch(null)
      return
    }

    if (e.key === 'Enter' && !e.shiftKey && !isComposing) {
      e.preventDefault()
      await handleSubmit()
    }
  }

  return (
    <div className="relative">
      {mentionSearch && (
        <div className="absolute inset-x-0 bottom-full mb-2 overflow-hidden rounded-xl border bg-popover shadow-lg">
          <div className="border-b px-3 py-2 text-xs text-muted-foreground">
            选择要提及的任务参与 Agent
          </div>
          {filteredCandidates.length > 0 ? (
            <div className="max-h-56 overflow-y-auto p-1.5">
              {filteredCandidates.map((candidate, index) => (
                <button
                  key={candidate.id}
                  type="button"
                  className={cn(
                    'flex w-full items-center justify-between rounded-lg px-3 py-2 text-left transition-colors',
                    index === (filteredCandidates.length === 0 ? 0 : Math.min(activeIndex, filteredCandidates.length - 1))
                      ? 'bg-accent text-accent-foreground'
                      : 'hover:bg-accent/60',
                  )}
                  onMouseDown={(event) => {
                    event.preventDefault()
                    handleSelectCandidate(candidate)
                  }}
                >
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium">@{candidate.name}</div>
                    <div className="text-xs text-muted-foreground">{candidate.roleLabel}</div>
                  </div>
                  <AtSign className="size-3.5 shrink-0 text-muted-foreground" />
                </button>
              ))}
            </div>
          ) : (
            <div className="px-3 py-3 text-sm text-muted-foreground">没有匹配的 Agent</div>
          )}
        </div>
      )}

      <div className="flex items-end gap-2 rounded-xl border bg-background px-3 py-2 focus-within:ring-1 focus-within:ring-ring">
        <textarea
          ref={textareaRef}
          className="min-h-[36px] max-h-[120px] flex-1 resize-none bg-transparent text-sm placeholder:text-muted-foreground focus-visible:outline-none"
          placeholder={placeholder}
          rows={1}
          value={value}
          disabled={disabled}
          onChange={(event) => {
            setValue(event.target.value)
            updateMentionSearch(event.target.value, event.target.selectionStart)
          }}
          onClick={(event) => updateMentionSearch(event.currentTarget.value, event.currentTarget.selectionStart)}
          onKeyUp={(event) => updateMentionSearch(event.currentTarget.value, event.currentTarget.selectionStart)}
          onKeyDown={handleKeyDown}
          onCompositionStart={() => setIsComposing(true)}
          onCompositionEnd={(event) => {
            setIsComposing(false)
            updateMentionSearch(event.currentTarget.value, event.currentTarget.selectionStart)
          }}
        />
        <Button
          size="icon"
          className="size-9 shrink-0"
          disabled={disabled || !value.trim()}
          onClick={() => {
            void handleSubmit()
          }}
        >
          <Send className="size-4" />
        </Button>
      </div>

    </div>
  )
}
