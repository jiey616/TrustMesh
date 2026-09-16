import { Button, Card, Col, Row, Space, Statistic, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import { usePlatformUsage } from '@/hooks/usePlatformAdmin'

const { Text } = Typography

function formatBytes(n: number) {
  if (n < 0) return '不限'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}

/**
 * 平台管理 · 用量总览（设计文档 §3.2）：count 级聚合，不含任何业务内容。
 */
export function PlatformUsagePage() {
  const { data: usage, isLoading, isFetching, refetch } = usePlatformUsage()

  const items: { title: string; value: number | string }[] = [
    { title: '租户总数', value: usage?.orgs_total ?? 0 },
    { title: '组织租户', value: usage?.orgs_enterprise ?? 0 },
    { title: '已禁用组织', value: usage?.orgs_disabled ?? 0 },
    { title: '用户数', value: usage?.users ?? 0 },
    { title: '数字员工数', value: usage?.agents ?? 0 },
    { title: '项目数', value: usage?.projects ?? 0 },
    { title: '任务数', value: usage?.tasks ?? 0 },
    { title: '项目文件存储', value: formatBytes(usage?.storage_bytes ?? 0) },
  ]

  return (
    <div style={{ paddingBottom: 40 }}>
      <PageHeader
        title="平台管理 · 用量总览"
        subtitle="全平台聚合计数（不含业务内容）"
        actions={
          <Button icon={<ReloadOutlined />} onClick={() => void refetch()} loading={isFetching}>
            刷新
          </Button>
        }
      />

      <Card
        loading={isLoading}
        style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}
      >
        <Row gutter={[16, 16]}>
          {items.map((it) => (
            <Col key={it.title} xs={12} sm={8} lg={6}>
              <Space direction="vertical" size={2}>
                <Statistic title={it.title} value={it.value} />
              </Space>
            </Col>
          ))}
        </Row>
        <Text type="secondary" style={{ fontSize: 12 }}>
          存储量为项目文件累计字节数；各项均为实时 count 聚合，不读取任何业务正文。
        </Text>
      </Card>
    </div>
  )
}