import { Button, Card, Empty, Result, Skeleton } from 'antd'
import { ApiOutlined } from '@ant-design/icons'
import { useNavigate, useParams } from 'react-router-dom'
import { PageHeader } from '@/components/shared/PageHeader'
import { ExternalAppFrame } from '@/components/external/ExternalAppFrame'
import { useExternalApps } from '@/hooks/useExternalApps'

/**
 * 侧边栏挂载的外部平台外壳页（/app/:id）。
 * 后端已按可见性过滤列表，因此这里查不到 == 不可见或无权限。
 */
export function ExternalAppFramePage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data: apps, isLoading } = useExternalApps()
  const app = apps?.find((a) => a.id === id)

  if (isLoading) {
    return (
      <Card bordered={false} style={{ background: 'var(--surface)' }}>
        <Skeleton active paragraph={{ rows: 6 }} />
      </Card>
    )
  }

  if (!app) {
    return (
      <Result
        status="404"
        title="找不到该外部平台"
        subTitle="它可能已被断开，或设置为私有且不属于你。"
        extra={
          <Button type="primary" onClick={() => navigate('/external-apps')}>
            前往外部应用管理
          </Button>
        }
      />
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
      <PageHeader
        title={app.name}
        icon={<ApiOutlined />}
        subtitle={app.visibility === 'public' ? '公共外部平台' : '我的外部平台'}
      />
      <div style={{ flex: 1, minHeight: 0 }}>
        {app.frame_mode === 'iframe' ? (
          <ExternalAppFrame app={app} />
        ) : (
          <Card bordered={false} style={{ background: 'var(--surface)' }}>
            <Empty
              description={
                <span>
                  「{app.name}」未启用内嵌模式，只能在新标签页打开。
                  <br />
                  如需内嵌，请在外部应用管理中把打开方式改为 iframe。
                </span>
              }
            >
              <ExternalAppFrame app={app} />
            </Empty>
          </Card>
        )}
      </div>
    </div>
  )
}
