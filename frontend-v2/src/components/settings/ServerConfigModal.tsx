import { useState } from 'react'
import { App, Button, Checkbox, Input, Modal, Space, Typography } from 'antd'
import { ApiOutlined, CheckCircleOutlined } from '@ant-design/icons'
import {
  DEFAULT_SERVER_URL,
  getEffectiveServerUrl,
  isDesktopShell,
  normalizeServerUrl,
  useServerConfigStore,
} from '@/stores/serverConfigStore'
import { probeServer } from '@/lib/serverProbe'

const { Text } = Typography

interface ServerConfigModalProps {
  open: boolean
  onClose: () => void
}

/**
 * 登录前可用的服务端地址配置弹窗。
 * 保存后整页刷新，所有 API/SSE 客户端按新地址重建。
 *
 * 标题与站内统一为「设置」（登录后同名入口在用户菜单里，落在 /profile 的设置页）。
 * 登录前拿不到账号，这是唯一能改地址的通道，故保留此入口。
 */
export function ServerConfigModal({ open, onClose }: ServerConfigModalProps) {
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
    window.setTimeout(() => window.location.reload(), 400)
  }

  const handleSave = () => {
    const raw = value.trim()
    if (!raw) {
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
    <Modal
      title={
        <Space size={8}>
          <ApiOutlined />
          <span>设置</span>
        </Space>
      }
      open={open}
      onCancel={onClose}
      footer={null}
      destroyOnHidden={false}
      width={480}
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

      <Space.Compact style={{ width: '100%' }}>
        <Input
          placeholder={DEFAULT_SERVER_URL}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onPressEnter={handleSave}
          allowClear
        />
        <Button icon={<CheckCircleOutlined />} loading={testing} onClick={handleTest}>
          测试
        </Button>
      </Space.Compact>

      <div style={{ marginTop: 8, marginBottom: 16 }}>
        <Text type="secondary" style={{ fontSize: 12 }}>
          例如 http://192.168.1.10:8080 或 https://api.example.com；留空保存则恢复默认。
        </Text>
      </div>

      {desktop && (
        <div style={{ marginBottom: 16 }}>
          <Checkbox
            checked={trustInsecureTls}
            onChange={(e) => setTrustInsecureTls(e.target.checked)}
          >
            信任自签名 / 私有 CA 证书
          </Checkbox>
          <div style={{ marginTop: 4 }}>
            <Text type="secondary" style={{ fontSize: 12 }}>
              仅在内网可信环境下开启：自签名 HTTPS 后端必须开启才能连接。
            </Text>
          </div>
        </div>
      )}

      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
        <Button onClick={onClose}>取消</Button>
        <Button type="primary" onClick={handleSave} disabled={!dirty && !!value}>
          保存并重连
        </Button>
      </div>
    </Modal>
  )
}
