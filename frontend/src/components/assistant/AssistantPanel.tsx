import { Trash2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAssistant } from '@/hooks/useAssistant'
import { AssistantMessageList } from './AssistantMessageList'
import { AssistantInput } from './AssistantInput'

export function AssistantPanel() {
  const {
    messages,
    isProcessing,
    close,
    sendMessage,
    cancel,
    clearMessages,
    getToolLabel,
  } = useAssistant()

  return (
    <div className="flex flex-col w-[min(480px,calc(100vw-32px))] h-[min(680px,calc(100vh-96px))] rounded-2xl border bg-background shadow-2xl overflow-hidden animate-in fade-in slide-in-from-bottom-4 duration-200">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b bg-muted/30">
        <span className="text-sm font-medium">Assistant</span>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={clearMessages}
            title="清空对话"
          >
            <Trash2 className="size-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={close}
          >
            <X className="size-3.5" />
          </Button>
        </div>
      </div>

      {/* Messages */}
      <AssistantMessageList messages={messages} getToolLabel={getToolLabel} />

      {/* Input */}
      <div className="p-3 border-t">
        <AssistantInput
          onSend={sendMessage}
          onCancel={cancel}
          disabled={isProcessing}
          isProcessing={isProcessing}
        />
      </div>
    </div>
  )
}
