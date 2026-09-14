import { useEffect, useMemo, useRef } from 'react'
import { AlertCircle, MessageSquare } from 'lucide-react'
import { toast } from 'sonner'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import { MessageBubble } from '@/components/task-thread/MessageBubble'
import { MessageInput } from '@/components/task-thread/MessageInput'
import { ThinkingIndicator } from '@/components/task-thread/ThinkingIndicator'
import { UIResponsePanel } from '@/components/task-thread/UIResponsePanel'
import { useAppendTaskMessage, useTask } from '@/hooks/useTasks'
import { ApiRequestError } from '@/api/client'
import type { ChatAttachment, TaskMessage, UIResponse } from '@/types'

interface TaskThreadSheetProps {
  taskId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

function findNextUserResponse(messages: TaskMessage[], index: number): TaskMessage | undefined {
  if (index + 1 < messages.length && messages[index + 1].role === 'user') {
    return messages[index + 1]
  }
  return undefined
}

export function TaskThreadSheet({ taskId, open, onOpenChange }: TaskThreadSheetProps) {
  const { data: task } = useTask(open ? taskId : undefined)
  const appendTaskMessage = useAppendTaskMessage()
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const scrollAreaRef = useRef<HTMLDivElement>(null)
  const isPlanning = task?.status === 'planning'
  const isReview = task?.status === 'review'
  const messageList = task?.messages
  const messages = messageList ?? []

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messageList])

  const pendingUIBlocks = useMemo(() => {
    if (!isPlanning || !messageList || messageList.length === 0) {
      return null
    }
    const lastMessage = messageList[messageList.length - 1]
    if (lastMessage.role === 'pm_agent' && lastMessage.ui_blocks && lastMessage.ui_blocks.length > 0) {
      return lastMessage.ui_blocks
    }
    return null
  }, [isPlanning, messageList])

  useEffect(() => {
    if (!open || (!isPlanning && !isReview)) {
      return
    }
    scrollAreaRef.current?.scrollTo({
      top: scrollAreaRef.current.scrollHeight,
      behavior: 'smooth',
    })
  }, [open, isPlanning, isReview, messageList, pendingUIBlocks])

  const handleSend = async (content: string, uiResponse?: UIResponse, attachments?: ChatAttachment[]) => {
    try {
      await appendTaskMessage.mutateAsync({
        taskId,
        input: { content, ui_response: uiResponse, attachments },
      })
    } catch (error) {
      const message = error instanceof ApiRequestError ? error.message : '发送消息失败'
      toast.error(message)
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" showCloseButton className="w-full max-w-4xl! gap-0! p-0 overflow-hidden">
        <div className="flex items-center gap-2 px-4 py-3 border-b shrink-0">
          <MessageSquare className="size-4 text-muted-foreground" />
          <SheetTitle className="text-sm font-medium flex-1 p-0! m-0!">需求对话记录</SheetTitle>
        </div>

        {!task ? (
          <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">加载中...</div>
        ) : messages.length === 0 ? (
          <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">暂无需求对话</div>
        ) : (
          <ScrollArea ref={scrollAreaRef} className="flex-1 min-h-0 p-4">
            <div className="flex flex-col gap-4">
              {messages.map((message, index) => {
                const isLastMessage = index === messages.length - 1
                const hasPendingBlocks = isLastMessage && !!pendingUIBlocks
                return (
                  <MessageBubble
                    key={message.id}
                    message={message}
                    nextUserResponse={
                      message.role === 'pm_agent' && message.ui_blocks?.length
                        ? findNextUserResponse(messages, index)
                        : undefined
                    }
                    hideUIBlocks={hasPendingBlocks}
                  />
                )
              })}
              {isPlanning && messages[messages.length - 1]?.role === 'user' && <ThinkingIndicator />}
              <div ref={messagesEndRef} />
            </div>
          </ScrollArea>
        )}

        <div className="px-4 py-3 shrink-0 flex justify-center border-t">
          <div className="w-full max-w-2xl">
            {isPlanning ? (
              pendingUIBlocks ? (
                <UIResponsePanel
                  blocks={pendingUIBlocks}
                  onSubmit={(content, uiResponse) => handleSend(content, uiResponse)}
                  disabled={appendTaskMessage.isPending}
                />
              ) : (
                <MessageInput
                  onSend={(content, attachments) => handleSend(content, undefined, attachments)}
                  disabled={appendTaskMessage.isPending}
                  placeholder="继续补充需求或回答 PM 的问题..."
                />
              )
            ) : (
              <div className="flex items-center gap-2 rounded-lg bg-muted/50 px-3 py-2 text-sm text-muted-foreground">
                <AlertCircle className="size-4 shrink-0" />
                该需求已进入执行阶段，对话记录仅供查看。
              </div>
            )}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  )
}
