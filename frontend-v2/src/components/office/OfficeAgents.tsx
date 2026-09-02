import { useMemo } from 'react'
import { useOfficeStore } from '@/stores/officeStore'
import { AgentSprite } from './AgentSprite'
import { assignAgentColors } from './officeLayout'

// ─── 落座 Agent 集合 ───
// 从 store 读取花名册与座位分配，只渲染有工位的 Agent
// （超出容量上限的在页面的「待命区」侧栏以列表形式呈现）。
//
// 这里订阅 store 是低频的（事件驱动的状态变更），
// 每帧的动画不经过这里 —— 全部在 AgentSprite 的 useFrame 里完成。

export function OfficeAgents() {
  const agents = useOfficeStore((s) => s.agents)
  const seating = useOfficeStore((s) => s.seating)
  // 单击 = 相机聚焦（不跳走）；「查看详情」由页面底部信息条提供
  const setFocus = useOfficeStore((s) => s.setFocus)
  // 每个小人一个专属颜色：按 id 稳定分配，花名册变化前不重排
  const colorMap = useMemo(
    () => assignAgentColors(Object.keys(agents)),
    [agents],
  )

  return (
    <group>
      {Object.values(agents).map((visual) => {
        const slot = seating[visual.id]
        if (!slot) return null
        return (
          <AgentSprite
            key={visual.id}
            visual={visual}
            slot={slot}
            color={colorMap[visual.id] ?? '#9aa3b2'}
            onClick={setFocus}
          />
        )
      })}
    </group>
  )
}
