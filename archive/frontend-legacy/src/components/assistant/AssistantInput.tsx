import { useState, useRef, useEffect } from 'react'
import { Send, Square } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface AssistantInputProps {
  onSend: (content: string) => void
  onCancel: () => void
  disabled?: boolean
  isProcessing?: boolean
}

export function AssistantInput({ onSend, onCancel, disabled, isProcessing }: AssistantInputProps) {
  const [value, setValue] = useState('')
  const [isComposing, setIsComposing] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
      textareaRef.current.style.height = Math.min(textareaRef.current.scrollHeight, 120) + 'px'
    }
  }, [value])

  const handleSubmit = () => {
    const trimmed = value.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setValue('')
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey && !isComposing) {
      e.preventDefault()
      handleSubmit()
    }
  }

  return (
    <div className="flex items-end gap-2 rounded-2xl border bg-card p-2.5 shadow-sm transition-shadow focus-within:shadow-md focus-within:border-primary/30">
      <textarea
        ref={textareaRef}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        onCompositionStart={() => setIsComposing(true)}
        onCompositionEnd={() => setIsComposing(false)}
        placeholder="输入你的问题..."
        disabled={disabled}
        rows={1}
        className="flex-1 resize-none bg-transparent px-1 py-1 text-sm leading-relaxed outline-none placeholder:text-muted-foreground disabled:opacity-50"
      />
      {isProcessing ? (
        <Button
          size="icon"
          variant="ghost"
          className="size-8 shrink-0 rounded-lg"
          onClick={onCancel}
        >
          <Square className="size-3.5" />
        </Button>
      ) : (
        <Button
          size="icon"
          className="size-8 shrink-0 rounded-lg"
          onClick={handleSubmit}
          disabled={disabled || !value.trim()}
        >
          <Send className="size-3.5" />
        </Button>
      )}
    </div>
  )
}
