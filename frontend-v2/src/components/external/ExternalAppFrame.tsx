import { useCallback, useEffect, useRef, useState } from 'react'
import { Alert, Button, Card, Space, Spin, Typography } from 'antd'
import { ExportOutlined, ReloadOutlined } from '@ant-design/icons'
import { useLaunchExternalApp } from '@/hooks/useExternalApps'
import { ApiRequestError } from '@/types'
import type { ExternalAppView } from '@/types'

const { Text, Paragraph } = Typography

/**
 * 外部平台内容容器。
 *
 * - frame_mode=iframe：内嵌渲染，右上角常驻「新标签打开 / 重新加载」。
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
  const [launchUrl, setLaunchUrl] = useState('')
  const [expiresIn, setExpiresIn] = useState(0)
  const [error, setError] = useState('')
  const [nonce, setNonce] = useState(0)
  const [hintVisible, setHintVisible] = useState(true)
  const launchedKey = useRef('')

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

  // 凭证是短时 JWT，过期后内嵌页面里的请求会失败，提示用户重新加载。
  const ttlLabel = expiresIn > 0 ? `${Math.round(expiresIn / 60)} 分钟` : '短期'

  if (app.status !== 'enabled') {
    return (
      <Card bordered={false} style={{ background: 'rgba(255,255,255,0.03)' }}>
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
      <Card bordered={false} style={{ background: 'rgba(255,255,255,0.03)' }}>
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
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0, gap: 8 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
        <Text type="secondary" style={{ fontSize: 12, flex: 1 }}>
          内嵌打开 · 凭证有效期 {ttlLabel}，过期后点「重新加载」
        </Text>
        <Space size={4}>
          <Button size="small" icon={<ReloadOutlined />} onClick={() => setNonce((n) => n + 1)}>
            重新加载
          </Button>
          <Button size="small" icon={<ExportOutlined />} onClick={openInNewTab}>
            新标签打开
          </Button>
        </Space>
      </div>

      {hintVisible && (
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
          border: '1px solid rgba(255,255,255,0.08)',
          borderRadius: 12,
          background: '#fff',
        }}
      />
    </div>
  )
}
