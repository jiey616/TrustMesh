import { Outlet } from 'react-router-dom'
import { Layout } from 'antd'
import { FloatingOrbs } from '@/components/FloatingOrbs'

const { Content } = Layout

export function AuthLayout() {
  return (
    <Layout style={{ minHeight: '100vh', background: 'transparent', position: 'relative' }}>
      <FloatingOrbs />
      <Content
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          padding: 24,
          position: 'relative',
          zIndex: 1,
        }}
      >
        <Outlet />
      </Content>
    </Layout>
  )
}