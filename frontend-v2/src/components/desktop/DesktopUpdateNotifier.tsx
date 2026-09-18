import { useEffect, useRef, useState } from 'react'
import { App, Button, Space } from 'antd'
import {
  fetchDesktopUpdateState,
  hasDesktopUpdateBridge,
  subscribeDesktopUpdate,
} from '@/lib/desktopUpdate'
import type { DesktopUpdateState } from '@/lib/desktopUpdate'

const NOTIFICATION_KEY = 'tm-desktop-update'

/**
 * 桌面端更新提示（非模态）。
 *
 * 🔴 必须挂在 **MainLayout**（始终挂载）而不是设置页：更新状态是应用级事件，
 *    用户在任一页面工作时都应该看到「新版本已就绪」。挂进 ProfilePage 会导致
 *    「只有恰好停在设置页的人才会被通知」——升级覆盖率直接取决于用户习惯。
 *
 * 交互遵循方案 §1 决策 13：绝不强制重启。提示停留到用户选择为止（duration: 0），
 * 选「稍后」即关闭；下载好的包会在**真正退出应用时**自动安装（main.cjs 的
 * autoInstallOnAppQuit），因此「稍后」不会浪费这次下载。
 */
export function DesktopUpdateNotifier() {
  const { notification } = App.useApp()
  const [state, setState] = useState<DesktopUpdateState | null>(null)
  // 同一版本只弹一次：状态快照会被反复推送（进度、重试），不设闸门就是通知轰炸。
  const notifiedVersion = useRef<string | null>(null)

  useEffect(() => {
    if (!hasDesktopUpdateBridge()) return
    let alive = true
    void fetchDesktopUpdateState().then((initial) => {
      if (alive && initial) setState(initial)
    })
    const off = subscribeDesktopUpdate((next) => {
      if (alive) setState(next)
    })
    return () => {
      alive = false
      off()
    }
  }, [])

  useEffect(() => {
    if (!state || state.status !== 'downloaded') return
    const key = state.version ?? 'unknown'
    if (notifiedVersion.current === key) return
    notifiedVersion.current = key

    const close = () => notification.destroy(NOTIFICATION_KEY)
    notification.open({
      key: NOTIFICATION_KEY,
      placement: 'bottomRight',
      duration: 0, // 需要用户决策，不自动消失
      message: state.version ? `新版本 ${state.version} 已就绪` : '新版本已就绪',
      description: '已在后台下载完成，重启应用即可生效。登录状态与本地配置都会保留。',
      btn: (
        <Space>
          <Button
            size="small"
            type="primary"
            onClick={() => {
              close()
              void window.desktop?.updateInstall?.()
            }}
          >
            重启更新
          </Button>
          <Button size="small" onClick={close}>
            稍后
          </Button>
        </Space>
      ),
    })
  }, [state, notification])

  return null
}
