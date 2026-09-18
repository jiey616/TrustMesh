import { useCallback, useEffect, useRef, useState } from 'react'
import { Alert, App, Button, Card, Space, Spin, Typography } from 'antd'
import {
  ExportOutlined,
  FullscreenExitOutlined,
  FullscreenOutlined,
  ReloadOutlined,
} from '@ant-design/icons'
import { useLaunchExternalApp } from '@/hooks/useExternalApps'
import { isElementFullscreen } from '@/lib/fullscreen'
import { ApiRequestError } from '@/types'
import type { ExternalAppView } from '@/types'

const { Text, Paragraph } = Typography

/**
 * 外部平台内容容器。
 *
 * - frame_mode=iframe：内嵌渲染，右上角常驻「全屏 / 重新加载 / 新标签打开」。
 *   注意：外部站点若设置 X-Frame-Options: DENY 或 CSP frame-ancestors，
 *   浏览器会拒绝渲染且前端无法可靠探测（跨域），因此不做自动嗅探，
 *   而是常驻降级入口 + 首次进入的一段说明。
 * - frame_mode=newtab：不内嵌，给出打开按钮（浏览器会拦截非手势触发的
 *   window.open，所以不自动开）。
 */
export function ExternalAppFrame({
  app,
  projectId,
  taskId,
}: {
  app: ExternalAppView
  projectId?: string
  taskId?: string
}) {
  const launch = useLaunchExternalApp()
  const { message } = App.useApp()
  const [launchUrl, setLaunchUrl] = useState('')
  const [expiresIn, setExpiresIn] = useState(0)
  const [error, setError] = useState('')
  const [nonce, setNonce] = useState(0)
  const [hintVisible, setHintVisible] = useState(true)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const launchedKey = useRef('')
  const stageRef = useRef<HTMLDivElement>(null)

  // 以 document.fullscreenElement 为准：用户按 Esc、或外部应用自身退出全屏时也能同步回按钮状态。
  const syncFullscreen = useCallback(() => {
    setIsFullscreen(isElementFullscreen(stageRef.current, document.fullscreenElement))
  }, [])

  useEffect(() => {
    document.addEventListener('fullscreenchange', syncFullscreen)
    syncFullscreen()
    return () => document.removeEventListener('fullscreenchange', syncFullscreen)
  }, [syncFullscreen])

  /**
   * 容器挂载/卸载时再同步一次。
   *
   * 首次渲染时凭证尚未就绪，组件走的是 `<Spin>` 分支，容器根本没挂上去；若只靠上面的
   * effect（只在挂载时跑一次，且那一刻容器为 null），就永远拿不到「容器是 null」以外的
   * 结论。用回调 ref 在节点真正 attach 时再判定，状态才与 DOM 一致。
   */
  const attachStage = useCallback(
    (node: HTMLDivElement | null) => {
      stageRef.current = node
      syncFullscreen()
    },
    [syncFullscreen],
  )

  const requestLaunch = useCallback(async () => {
    try {
      const data = await launch.mutateAsync({ id: app.id, projectId, taskId })
      setLaunchUrl(data.launch_url)
      setExpiresIn(data.expires_in ?? 0)
      setError('')
    } catch (err) {
      setError(err instanceof ApiRequestError ? err.message : '获取访问凭证失败')
    }
  }, [app.id, launch, projectId, taskId])

  useEffect(() => {
    // 同一组依赖只自动取一次凭证，避免重复签发 token。
    const key = `${app.id}:${projectId ?? ''}:${taskId ?? ''}:${nonce}`
    if (launchedKey.current === key) return
    launchedKey.current = key
    void requestLaunch()
  }, [app.id, projectId, taskId, nonce, requestLaunch])

  const openInNewTab = () => {
    if (!launchUrl) return
    window.open(launchUrl, '_blank', 'noopener,noreferrer')
  }

  /**
   * 全屏切换。
   *
   * 对「容器」调用 requestFullscreen，而不是对 iframe 元素：
   * 容器是本页自己的节点，不经过任何跨域权限校验，工具栏得以保留在顶部，
   * 用户始终有一个可见的「退出全屏」入口（否则只能靠 Esc）。
   * 副作用是外部应用的全屏按钮（若它自己实现了）走的是另一套权限路径，
   * 由 iframe 的 allow="fullscreen" 决定，二者互不影响。
   *
   * ⚠️ 必须由点击同步触发：requestFullscreen 依赖用户激活，放到
   * await / setTimeout / Promise.then 之后调用会被拒（NotAllowedError）。
   */
  const toggleFullscreen = () => {
    const el = stageRef.current
    if (!el) return
    if (isElementFullscreen(el, document.fullscreenElement)) {
      void document.exitFullscreen()
      return
    }
    void el.requestFullscreen().catch(() => {
      message.error('无法进入全屏，请直接点击本按钮后重试')
    })
  }

  // 凭证是短时 JWT，过期后内嵌页面里的请求会失败，提示用户重新加载。
  const ttlLabel = expiresIn > 0 ? `${Math.round(expiresIn / 60)} 分钟` : '短期'

  if (app.status !== 'enabled') {
    return (
      <Card bordered={false} style={{ background: 'var(--surface)' }}>
        <Text type="secondary">该外部平台已停用，启用后即可打开。</Text>
      </Card>
    )
  }

  if (error) {
    return (
      <Alert
        type="error"
        showIcon
        message="无法获取访问凭证"
        description={error}
        action={
          <Button size="small" onClick={() => setNonce((n) => n + 1)}>
            重试
          </Button>
        }
      />
    )
  }

  if (!launchUrl) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', padding: '64px 0' }}>
        <Spin tip="正在获取访问凭证…" />
      </div>
    )
  }

  if (app.frame_mode !== 'iframe') {
    return (
      <Card bordered={false} style={{ background: 'var(--surface)' }}>
        <Paragraph type="secondary" style={{ marginBottom: 16 }}>
          「{app.name}」配置为新标签页打开。凭证有效期 {ttlLabel}，请尽快完成操作。
        </Paragraph>
        <Button type="primary" icon={<ExportOutlined />} onClick={openInNewTab}>
          打开 {app.name}
        </Button>
      </Card>
    )
  }

  return (
    <div
      ref={attachStage}
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        minHeight: 0,
        gap: 8,
        padding: isFullscreen ? 10 : 0,
        background: 'var(--surface)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
        <Text type="secondary" style={{ fontSize: 12, flex: 1 }}>
          内嵌打开 · 凭证有效期 {ttlLabel}，过期后点「重新加载」
        </Text>
        <Space size={4}>
          <Button
            size="small"
            icon={isFullscreen ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
            onClick={toggleFullscreen}
          >
            {isFullscreen ? '退出全屏' : '全屏'}
          </Button>
          <Button size="small" icon={<ReloadOutlined />} onClick={() => setNonce((n) => n + 1)}>
            重新加载
          </Button>
          <Button size="small" icon={<ExportOutlined />} onClick={openInNewTab}>
            新标签打开
          </Button>
        </Space>
      </div>

      {hintVisible && !isFullscreen && (
        <Alert
          type="info"
          showIcon
          closable
          onClose={() => setHintVisible(false)}
          style={{ flexShrink: 0 }}
          message="若下方区域空白，说明该平台禁止被内嵌（X-Frame-Options / CSP 限制），请改用「新标签打开」。"
        />
      )}

      <iframe
        key={`${app.id}-${nonce}`}
        src={launchUrl}
        title={app.name}
        referrerPolicy="no-referrer"
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-popups-to-escape-sandbox allow-downloads"
        allow="clipboard-read; clipboard-write; fullscreen"
        style={{
          flex: 1,
          minHeight: 0,
          width: '100%',
          border: '1px solid var(--line)',
          borderRadius: 'var(--radius-control)',
          background: 'var(--canvas-elevated)',
        }}
      />
    </div>
  )
}
