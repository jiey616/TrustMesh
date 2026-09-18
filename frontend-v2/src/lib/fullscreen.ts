/**
 * 全屏状态判定（纯函数，便于单测）。
 *
 * 为什么必须单独抽出来：容器节点的 ref 在「组件已挂载、但对应 DOM 尚未渲染」时是
 * `null`。本组件在拿到 launch_url 之前返回的是 `<Spin>` 分支，不含 `stageRef` 容器，
 * 因此挂载期的 effect 跑起来时 `stageRef.current === null`。
 *
 * 若把判定直接写成 `document.fullscreenElement === stageRef.current`，此刻两侧同为空
 * ⇒ `null === null` 求值为 `true`，组件会误判自己「已在全屏」：首次进入就显示
 * 「退出全屏」，并顺带错误地加上内边距、隐藏空白提示条。
 *
 * 所以先把左侧的 null 挡掉：只有「容器已挂载」**且**「容器就是当前全屏元素」才为真。
 */
export function isElementFullscreen(
  container: Element | null,
  fullscreenElement: Element | null,
): boolean {
  return container !== null && fullscreenElement === container
}
