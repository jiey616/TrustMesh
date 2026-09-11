import { useState } from 'react'
import { App, Button, Card, Checkbox, Input, Space, Typography } from 'antd'
import { ApiOutlined, CheckCircleFilled, ReloadOutlined } from '@ant-design/icons'
import {
  DEFAULT_SERVER_URL,
  getEffectiveServerUrl,
  isDesktopShell,
  normalizeServerUrl,
  useServerConfigStore,
} from '@/stores/serverConfigStore'
import { probeServer } from '@/lib/serverProbe'

const { Text } = Typography

/**
 * 服务端地址配置卡片。
 * 保存后整页刷新，使所有 API/SSE 客户端按新地址重建连接。
 */
export function ServerConfigCard() {
  const { message } = App.useApp()
  const serverUrl = useServerConfigStore((s) => s.serverUrl)
  const setServerUrl = useServerConfigStore((s) => s.setServerUrl)
  const clearServerUrl = useServerConfigStore((s) => s.clearServerUrl)
  const trustInsecureTls = useServerConfigStore((s) => s.trustInsecureTls)
  const setTrustInsecureTls = useServerConfigStore((s) => s.setTrustInsecureTls)
  const desktop = isDesktopShell()

  const effective = getEffectiveServerUrl()
  const [value, setValue] = useState(serverUrl ?? '')
  const [testing, setTesting] = useState(false)

  const dirty = normalizeServerUrl(value) !== (serverUrl ?? null)

  const applyAndReload = (okText: string) => {
    message.success(okText)
    // 延迟一点让用户看到提示
    window.setTimeout(() => window.location.reload(), 400)
  }

  const handleSave = () => {
    const raw = value.trim()
    if (!raw) {
      // 空 = 恢复默认
      clearServerUrl()
      applyAndReload('已恢复默认服务端地址')
      return
    }
    if (!setServerUrl(raw)) {
      message.error('地址格式不正确，请输入 http(s)://主机[:端口] 形式')
      return
    }
    applyAndReload('服务端地址已保存')
  }

  const handleTest = async () => {
    const normalized = normalizeServerUrl(value || DEFAULT_SERVER_URL)
    if (!normalized) {
      message.error('地址格式不正确')
      return
    }
    setTesting(true)
    try {
      const summary = await probeServer(normalized)
      if (summary.level === 'ok') message.success(summary.message)
      else if (summary.level === 'warn') message.warning(summary.message)
      else message.error(summary.message)
    } finally {
      setTesting(false)
    }
  }

  return (
    <Card
      title={
        <Space size={8}>
          <ApiOutlined />
          <span>服务器连接</span>
        </Space>
      }
      style={{ marginBottom: 16 }}
    >
      <div style={{ marginBottom: 12 }}>
        <Text type="secondary" style={{ fontSize: 13 }}>
          当前服务端：
        </Text>
        <Text code style={{ fontSize: 13 }}>
          {effective}
        </Text>
        {!serverUrl && (
          <Text type="secondary" style={{ fontSize: 12, marginLeft: 8 }}>
            （默认）
          </Text>
        )}
      </div>
      <Space.Compact style={{ width: '100%', maxWidth: 560 }}>
        <Input
          placeholder={DEFAULT_SERVER_URL}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onPressEnter={handleSave}
          allowClear
        />
        <Button icon={<CheckCircleFilled />} loading={testing} onClick={handleTest}>
          测试连接
        </Button>
        <Button type="primary" icon={<ReloadOutlined />} onClick={handleSave} disabled={!dirty && !!value}>
          保存并重连
        </Button>
      </Space.Compact>
      <div style={{ marginTop: 8 }}>
        <Text type="secondary" style={{ fontSize: 12 }}>
          例如 http://192.168.1.10:8080 或 https://api.example.com；留空保存则恢复默认。保存后应用会自动刷新。
        </Text>
      </div>
      {desktop && (
        <div style={{ marginTop: 10 }}>
          <Checkbox
            checked={trustInsecureTls}
            onChange={(e) => setTrustInsecureTls(e.target.checked)}
          >
            信任自签名 / 私有 CA 证书
          </Checkbox>
          <div style={{ marginTop: 4 }}>
            <Text type="secondary" style={{ fontSize: 12 }}>
              仅在内网可信环境下开启。开启后桌面端不再校验 HTTPS 证书链，自签名后端才能连接。
            </Text>
          </div>
        </div>
      )}
    </Card>
  )
}
