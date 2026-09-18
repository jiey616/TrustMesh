import { useState } from 'react'
import { Outlet } from 'react-router-dom'
import { Layout, Tooltip } from 'antd'
import { ApiOutlined } from '@ant-design/icons'
import { FloatingOrbs } from '@/components/FloatingOrbs'
import { ServerConfigModal } from '@/components/settings/ServerConfigModal'
import { getEffectiveServerUrl, isElectronRuntime } from '@/stores/serverConfigStore'

const { Content } = Layout

// 服务器地址配置仅供桌面端使用；Web（含容器部署）固定走同源 /api/v1/，不暴露此入口。
const DESKTOP = typeof window !== 'undefined' && isElectronRuntime()

export function AuthLayout() {
  const [serverModalOpen, setServerModalOpen] = useState(false)

  return (
    <Layout style={{ minHeight: '100vh', background: 'transparent', position: 'relative' }}>
      <FloatingOrbs />
      <Content
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          padding: 0,
          position: 'relative',
          zIndex: 1,
        }}
      >
        <Outlet />
      </Content>

      {/* 登录前可配置服务端地址（仅桌面端）。名字与站内统一为「设置」：登录后同名入口在用户菜单里。 */}
      {DESKTOP && (
        <>
          <Tooltip title={`服务器：${getEffectiveServerUrl()}`} placement="left">
            <div
              role="button"
              tabIndex={0}
              aria-label="设置"
              onClick={() => setServerModalOpen(true)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  setServerModalOpen(true)
                }
              }}
              style={{
                position: 'fixed',
                right: 20,
                bottom: 20,
                zIndex: 1000,
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                padding: '8px 12px',
                borderRadius: 999,
                background: 'var(--surface)',
                border: '1px solid var(--line)',
                color: 'var(--text-secondary)',
                cursor: 'pointer',
                fontSize: 12,
                backdropFilter: 'var(--glass-blur)',
                boxShadow: 'var(--shadow-float)',  // 主题感知：近白主题自动变轻
                userSelect: 'none',
              }}
            >
              <ApiOutlined style={{ fontSize: 14 }} />
              <span style={{ maxWidth: 220, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {getEffectiveServerUrl()}
              </span>
            </div>
          </Tooltip>

          <ServerConfigModal open={serverModalOpen} onClose={() => setServerModalOpen(false)} />
        </>
      )}

    </Layout>
  )
}