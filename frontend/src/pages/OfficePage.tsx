import { useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { UserPlus } from 'lucide-react'
import { PageContainer } from '@/components/layout/PageContainer'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { useAgents } from '@/hooks/useAgents'
import { usePageVisibility } from '@/hooks/usePageVisibility'
import { useRealtimeStatus } from '@/realtime/hooks/useRealtimeStatus'
import { OfficeScene } from '@/components/office/officeScene'
import { useOfficeData } from '@/components/office/useOfficeData'

const STATE_LEGEND = [
  { emoji: '⌨️', label: '执行中', desc: '有进行中的 Todo' },
  { emoji: '❓', label: '等待输入', desc: 'Agent 向你提问，点击直达任务' },
  { emoji: '💭', label: '规划中', desc: 'PM 正在澄清/规划任务' },
  { emoji: '🎉', label: '完成', desc: 'Todo 交付成功' },
  { emoji: '😵', label: '失败', desc: 'Todo 执行失败' },
  { emoji: '💤', label: '离线', desc: 'Agent 节点不在线' },
]

// 路由级懒加载页（App.tsx 中 React.lazy 引入），pixi.js 只进这个 chunk。
// 所有画面状态由真实 SSE 事件驱动，场景本身不产生任何"表演"。
export function OfficePage() {
  const navigate = useNavigate()
  const hostRef = useRef<HTMLDivElement>(null)
  const sceneRef = useRef<OfficeScene | null>(null)
  const readyRef = useRef(false)

  const { data: agents } = useAgents()
  const realtimeStatus = useRealtimeStatus()
  const isVisible = usePageVisibility()
  useOfficeData()

  useEffect(() => {
    const host = hostRef.current
    if (!host) return

    let disposed = false
    const scene = new OfficeScene({
      onAgentClick: (agentId) => navigate(`/agents/${agentId}`),
    })
    sceneRef.current = scene

    const ro = new ResizeObserver((entries) => {
      const { width, height } = entries[0].contentRect
      if (width <= 0 || height <= 0) return
      if (!readyRef.current) {
        readyRef.current = true
        void scene
          .init(host, width, height)
          .then(() => {
            if (disposed) {
              scene.destroy()
            } else {
              // init 是异步的，布局可能在 init 期间已变化，用当前实际尺寸校正一次
              scene.resize(host.clientWidth, host.clientHeight)
            }
          })
        return
      }
      scene.resize(width, height)
    })
    ro.observe(host)

    return () => {
      disposed = true
      ro.disconnect()
      readyRef.current = false
      scene.destroy()
      sceneRef.current = null
    }
    // navigate 是稳定引用；场景只创建一次
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    sceneRef.current?.setPaused(!isVisible)
  }, [isVisible])

  const hasAgents = (agents ?? []).filter((a) => !a.archived).length > 0

  return (
    <PageContainer className="flex flex-col h-full gap-4">
      <div className="shrink-0 flex items-center justify-between gap-4 flex-wrap">
        <div>
          <h1 className="text-2xl font-bold">数字员工办公室</h1>
          <p className="text-sm text-muted-foreground mt-1">
            实时呈现智能体的工作状态——所有画面均由真实任务事件驱动
            {realtimeStatus === 'connected' ? '' : '（离线数据，可能滞后）'}
          </p>
        </div>
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          {STATE_LEGEND.map((item) => (
            <span key={item.label} className="inline-flex items-center gap-1" title={item.desc}>
              <span aria-hidden>{item.emoji}</span>
              {item.label}
            </span>
          ))}
        </div>
      </div>

      {agents && !hasAgents ? (
        <Card className="flex-1 flex items-center justify-center">
          <div className="flex flex-col items-center gap-3 text-center py-16">
            <UserPlus className="size-10 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">
              还没有智能体入驻。先去招聘你的第一位数字员工吧。
            </p>
            <Button onClick={() => navigate('/agent-invite')}>招聘智能体</Button>
          </div>
        </Card>
      ) : (
        <Card className="flex-1 min-h-0 overflow-hidden">
          <CardContent className="h-full w-full p-0">
            <div ref={hostRef} className="h-full w-full [&_canvas]:block" />
          </CardContent>
        </Card>
      )}
    </PageContainer>
  )
}

export default OfficePage
