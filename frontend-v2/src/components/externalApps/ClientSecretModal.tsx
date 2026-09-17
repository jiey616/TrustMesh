import { Alert, Button, Input, Modal, Typography, App } from 'antd'

const { Text } = Typography

export interface ClientSecretModalProps {
  open: boolean
  appName: string
  secret: string
  onClose: () => void
}

/**
 * client_secret 仅在创建响应中返回一次：创建成功后立刻弹出要求用户保存，
 * 关闭后无法再次查看（需重新创建平台）。
 */
export function ClientSecretModal({ open, appName, secret, onClose }: ClientSecretModalProps) {
  const { message } = App.useApp()

  const copySecret = async () => {
    try {
      await navigator.clipboard.writeText(secret)
      message.success('client_secret 已复制')
    } catch {
      message.error('复制失败，请手动选择复制')
    }
  }

  return (
    <Modal
      title="保存 client_secret"
      open={open}
      onCancel={onClose}
      footer={[
        <Button key="copy" type="primary" onClick={() => void copySecret()}>
          复制
        </Button>,
        <Button key="done" onClick={onClose}>
          我已保存，完成
        </Button>,
      ]}
    >
      <Alert
        type="warning"
        showIcon
        message="请立即复制并妥善保存 client_secret"
        description="该密钥仅显示这一次，关闭后将无法再次查看，需重新创建平台。"
        style={{ marginBottom: 12 }}
      />
      <Input readOnly value={secret} className="font-mono" style={{ fontSize: 12 }} />
      <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
        {appName}
      </Text>
    </Modal>
  )
}