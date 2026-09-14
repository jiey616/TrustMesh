import { useState, useEffect } from 'react'

const thinkingMessages = [
  '项目经理正在理解你的需求...',
  '正在分析需求细节...',
  '正在梳理关键要点...',
  '正在思考最佳方案...',
  '正在整理回复内容...',
]

export function ThinkingIndicator() {
  const [messageIndex, setMessageIndex] = useState(0)

  useEffect(() => {
    const interval = setInterval(() => {
      setMessageIndex((prev) => (prev + 1) % thinkingMessages.length)
    }, 4000)
    return () => clearInterval(interval)
  }, [])

  return (
    <div className="flex items-start gap-3">
      <div className="rounded-3xl bg-muted px-5 py-3.5 text-sm">
        <div className="flex items-center gap-2 text-muted-foreground">
          <span className="flex gap-1">
            <span className="size-1.5 rounded-full bg-current animate-bounce [animation-delay:0ms]" />
            <span className="size-1.5 rounded-full bg-current animate-bounce [animation-delay:150ms]" />
            <span className="size-1.5 rounded-full bg-current animate-bounce [animation-delay:300ms]" />
          </span>
          <span className="transition-opacity duration-300">{thinkingMessages[messageIndex]}</span>
        </div>
      </div>
    </div>
  )
}
