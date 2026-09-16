import { Skeleton } from 'antd'

/**
 * 工作区校准门控期（workspaceCalibrated=false）的内容区骨架。
 *
 * 冷启动 / 整页刷新时，在 `GET /organizations` 就绪且校准把最终运行时租户 id 写入
 * store 之前，MainLayout 用本组件替换 <Outlet/>，挡住受保护页面 mount —— 从而杜绝
 * 「无 X-Org-Id 首轮」带来的 user 维度闪变与误写（见 F2 设计 §1.2）。
 *
 * 保持与 Content 区一致的观感；fail-open（orgs 失败 / 6s 超时）后自动让位真实页面。
 */
export function WorkspaceCalibratingSkeleton() {
  return (
    <div style={{ padding: 4 }} aria-busy="true" aria-label="工作区加载中">
      <Skeleton active title={{ width: '30%' }} paragraph={{ rows: 1 }} style={{ marginBottom: 24 }} />
      <Skeleton active title={false} paragraph={{ rows: 6, width: ['100%', '92%', '96%', '88%', '100%', '80%'] }} />
    </div>
  )
}
