import { useEffect, useRef } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { AssistantMessage } from '@/types'
import { AssistantMessageBubble } from './AssistantMessageBubble'

interface Props {
  messages: AssistantMessage[]
  getToolLabel: (tool: string) => string
}

export function AssistantMessageList({ messages, getToolLabel }: Props) {
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  if (messages.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center p-6">
        <div className="text-center text-muted-foreground text-sm space-y-2">
          <p className="text-lg">✨</p>
          <p>有什么可以帮你的？</p>
          <p className="text-xs">试试问我关于任务、知识库或项目的问题</p>
        </div>
      </div>
    )
  }

  return (
    <ScrollArea className="flex-1">
      <div className="flex flex-col gap-3 p-4">
        {messages.map((msg) => (
          <AssistantMessageBubble
            key={msg.id}
            message={msg}
            getToolLabel={getToolLabel}
          />
        ))}
        <div ref={bottomRef} />
      </div>
    </ScrollArea>
  )
}
