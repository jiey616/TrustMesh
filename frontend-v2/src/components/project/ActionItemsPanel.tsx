import { Card, Typography } from 'antd'
import { CheckSquareOutlined } from '@ant-design/icons'

const { Title, Paragraph } = Typography

interface Props {
  projectId: string
}

export function ActionItemsPanel({ projectId }: Props) {
  return (
    <Card bordered={false} style={{ textAlign: 'center', padding: 48, background: 'rgba(255,255,255,0.03)' }}>
      <CheckSquareOutlined style={{ fontSize: 48, color: '#22d3ee', marginBottom: 16 }} />
      <Title level={5}>待办事项</Title>
      <Paragraph type="secondary">待确认待办面板开发中（project: {projectId}）</Paragraph>
    </Card>
  )
}