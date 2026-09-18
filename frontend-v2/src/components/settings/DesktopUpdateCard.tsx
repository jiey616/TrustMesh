import { useEffect, useState } from 'react'
import { App, Button, Card, Progress, Space, Tag, Typography } from 'antd'
import { CloudDownloadOutlined, ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons'
import {
  describeUpdateStatus,
  fetchDesktopUpdateState,
  hasDesktopUpdateBridge,
  subscribeDesktopUpdate,
} from '@/lib/desktopUpdate'
import type { DesktopUpdateState } from '@/lib/desktopUpdate'

const { Text } = Typography

/**
 * 设置页的「桌面端更新」卡片 —— 兜底入口（方案 §6）。
 *
 * 主提示走 MainLayout 里的非模态通知；这张卡存在的原因是：
 *   - 用户想知道「我现在是什么版本、更新源在哪」时有地方可查；
 *   - 后台检查失败/被静默处理（例如服务端还没发过版）时，用户能主动再查一次。
 *
 * 只在桌面壳内渲染（`isDesktopShell()`）：Web 端出现一个永远不能用的按钮只会制造困惑。
 */
export function DesktopUpdateCard() {
  const { message } = App.useApp()
  const [state, setState] = useState<DesktopUpdateState | null>(null)
  const [appVersion, setAppVersion] = useState<string | null>(null)
  const [checking, setChecking] = useState(false)

  useEffect(() => {
    if (!hasDesktopUpdateBridge()) return
    let alive = true
    void fetchDesktopUpdateState().then((s) => {
      if (alive && s) setState(s)
    })
    void window.desktop?.getAppVersion?.().then((v) => {
      if (alive) setAppVersion(v)
    })
    const off = subscribeDesktopUpdate((next) => {
      if (alive) setState(next)
    })
    return () => {
      alive = false
      off()
    }
  }, [])

  const handleCheck = async () => {
    setChecking(true)
    try {
      const next = await window.desktop?.updateCheck?.()
      if (next) setState(next)
    } catch {
      message.error('检查更新失败')
    } finally {
      setChecking(false)
    }
  }

  const handleInstall = async () => {
    const ok = await window.desktop?.updateInstall?.()
    if (!ok) message.warning('当前没有已下载的更新')
  }

  return (
    <Card
      title={
        <Space size={8}>
          <ThunderboltOutlined />
          <span>桌面端更新</span>
        </Space>
      }
      style={{ marginBottom: 16 }}
    >
      <div style={{ marginBottom: 8 }}>
        <Text type="secondary" style={{ fontSize: 13 }}>
          当前版本：
        </Text>
        <Text code style={{ fontSize: 13 }}>
          {appVersion ?? '未知'}
        </Text>
        {state?.status === 'downloaded' && state.version && (
          <Tag color="success" style={{ marginLeft: 8 }}>
            新版本 {state.version} 已就绪
          </Tag>
        )}
      </div>

      <div style={{ marginBottom: 12 }}>
        <Text type="secondary" style={{ fontSize: 13 }}>
          {describeUpdateStatus(state)}
        </Text>
      </div>

      {state?.status === 'downloading' && (
        <Progress percent={state.percent} size="small" style={{ maxWidth: 420 }} />
      )}

      <Space>
        <Button
          icon={<ReloadOutlined />}
          loading={checking}
          disabled={state?.status === 'downloading'}
          onClick={() => void handleCheck()}
        >
          检查更新
        </Button>
        {state?.status === 'downloaded' && (
          <Button type="primary" icon={<CloudDownloadOutlined />} onClick={() => void handleInstall()}>
            重启更新
          </Button>
        )}
      </Space>

      <div style={{ marginTop: 8 }}>
        <Text type="secondary" style={{ fontSize: 12 }}>
          更新由平台管理员统一发布。升级会保留登录状态与本地配置（含信任自签名证书开关）。
        </Text>
        {state?.feedUrl && (
          <div>
            <Text type="secondary" style={{ fontSize: 12 }}>
              更新源：{state.feedUrl}
            </Text>
          </div>
        )}
      </div>
    </Card>
  )
}
