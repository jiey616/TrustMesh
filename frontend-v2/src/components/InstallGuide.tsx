import { useState } from 'react'
import { Card, Button, Input, Typography, Space, Tabs, App } from 'antd'
import { CopyOutlined, DownloadOutlined, CodeOutlined, FileTextOutlined, RobotOutlined } from '@ant-design/icons'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import type { MarketRoleDetail } from '@/types'

const { Paragraph, Text } = Typography

interface Props {
  role: MarketRoleDetail
}

function buildDownloadUrl(roleId: string) {
  const origin = window.location.origin.replace(/\/$/, '')
  return `${origin}market/roles/${roleId}/download`
}

function buildOpenClawPrompt(role: MarketRoleDetail) {
  const agentId = role.id
  const zipUrl = buildDownloadUrl(role.id)
  const localZipPath = `~/Downloads/${role.id}.zip`
  const workspacePath = '~/.openclaw/workspace'

  return `## 安装角色包

我需要你帮我安装一个角色包到 Openclaw workspace。

### 角色包信息

- Agent ID: \`${agentId}\`
- URL: \`${zipUrl}\`
- 本地路径（如果我已经下载完成）: \`${localZipPath}\`

### 安装步骤

\`\`\`bash
# 下载角色包（如果需要）
curl -L -o /tmp/${agentId}.zip "${zipUrl}"

# 解压并覆盖到 workspace 目录（-j 忽略 zip 内子目录，直接平铺）
unzip -o -j /tmp/${agentId}.zip -d ${workspacePath}
\`\`\`

解压完成后直接生效，无需额外配置。

如果 URL 无法直接访问，请改用我本地已下载的 zip 文件路径继续安装。`
}

export function InstallGuide({ role }: Props) {
  const { copiedKey, copy } = useCopyToClipboard(2000)
  const { message } = App.useApp()
  const [agentName, setAgentName] = useState(role.name)

  const promptText = `请扮演一个角色：

## 角色名称
${role.name}

## 角色描述
${role.description}

## 身份设定
${role.identity_content}

## 人格特征
${role.soul_content}

## 工作规范
${role.agents_content}
`

  const openclawPrompt = buildOpenClawPrompt(role)

  const handleCopyPrompt = async (text: string, key: string, okMsg: string) => {
    const ok = await copy(text, key)
    if (ok) message.success(okMsg)
    else message.error('复制失败，请手动选择复制')
  }

  return (
    <Card bordered={false} style={{ background: 'var(--surface)' }}>
      <Tabs
        defaultActiveKey="openclaw"
        items={[
          {
            key: 'openclaw',
            label: <span><RobotOutlined /> OpenClaw 安装</span>,
            children: (
              <Space direction="vertical" style={{ width: '100%' }}>
                <Paragraph type="secondary" style={{ fontSize: 12 }}>
                  复制下面这段提示词到 Openclaw，即可自动下载并安装当前角色包。
                </Paragraph>
                <Input addonBefore="数字员工名称" value={agentName} onChange={(e) => setAgentName(e.target.value)} />
                <div style={{ position: 'relative' }}>
                  <Input.TextArea
                    value={openclawPrompt}
                    rows={12}
                    readOnly
                    style={{ fontFamily: 'var(--font-mono)', fontSize: 12, whiteSpace: 'pre-wrap' }}
                  />
                  <Button
                    type="primary"
                    size="small"
                    icon={copiedKey === role.id ? <CopyOutlined /> : <CopyOutlined />}
                    style={{ position: 'absolute', top: 8, right: 8 }}
                    onClick={() => handleCopyPrompt(openclawPrompt, role.id, 'Openclaw 提示词已复制')}
                  >
                    {copiedKey === role.id ? '已复制' : '复制'}
                  </Button>
                </div>
                <Paragraph type="secondary" style={{ fontSize: 11 }}>
                  提示词已带上当前角色 ID、名称、部门和角色包下载地址，Openclaw 可直接继续安装与注册。
                </Paragraph>
              </Space>
            ),
          },
          {
            key: 'prompt',
            label: <span><FileTextOutlined /> 角色提示词</span>,
            children: (
              <Space direction="vertical" style={{ width: '100%' }}>
                <Paragraph type="secondary" style={{ fontSize: 12 }}>复制以下提示词到你的 AI 客户端（ChatGPT / Claude / 其他）</Paragraph>
                <div style={{ position: 'relative' }}>
                  <Input.TextArea value={promptText} rows={10} readOnly style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }} />
                  <Button
                    type="primary"
                    size="small"
                    icon={copiedKey === 'prompt' ? <CopyOutlined /> : <CopyOutlined />}
                    style={{ position: 'absolute', top: 8, right: 8 }}
                    onClick={() => handleCopyPrompt(promptText, 'prompt', '提示词已复制')}
                  >
                    {copiedKey === 'prompt' ? '已复制' : '复制'}
                  </Button>
                </div>
              </Space>
            ),
          },
          {
            key: 'package',
            label: <span><CodeOutlined /> 完整包</span>,
            children: (
              <Space direction="vertical" style={{ width: '100%' }}>
                <Paragraph type="secondary" style={{ fontSize: 12 }}>
                  点击下方按钮下载完整的角色定义包（含 identity / soul / agents 三段配置）
                </Paragraph>
                <Button type="primary" icon={<DownloadOutlined />} block onClick={() => handleCopyPrompt(`下载地址: ${buildDownloadUrl(role.id)}`, 'pkg', '下载地址已复制')}>
                  复制角色包下载地址
                </Button>
                <Text type="secondary" style={{ fontSize: 11 }}>{buildDownloadUrl(role.id)}</Text>
              </Space>
            ),
          },
        ]}
      />
    </Card>
  )
}