import { useState } from 'react'
import { App, Button, Card, Checkbox, Typography } from 'antd'
import { EyeInvisibleOutlined, SaveOutlined } from '@ant-design/icons'
import { useUpdateOrgMenuOverrides } from '@/hooks/useOrgs'
import { HIDEABLE_MENUS } from '@/lib/menuDefs'
import { ApiRequestError } from '@/types'
import type { OrgView } from '@/types'

const { Text, Paragraph } = Typography

/**
 * 企业级菜单可见性（设计文档 §4）：owner 勾选「隐藏」的菜单对全员不可见。
 *
 * 语义是**只能缩小**：这里只做隐藏，不能给任何角色增开权限；
 * 而且只改前端菜单可见性，**不改变 API 鉴权**（权限点仍是唯一准绳）。
 */
export function MenuVisibilityCard({ org }: { org: OrgView }) {
  const { message } = App.useApp()
  const [hidden, setHidden] = useState<string[]>(org.menu_overrides ?? [])
  const save = useUpdateOrgMenuOverrides(org.id)

  const handleSave = async () => {
    try {
      await save.mutateAsync(hidden)
      message.success('菜单可见性已更新（对全员即时生效）')
    } catch (e) {
      message.error(e instanceof ApiRequestError ? e.message : '保存失败')
    }
  }

  return (
    <Card
      title={
        <span>
          <EyeInvisibleOutlined style={{ marginRight: 8 }} />
          菜单可见性
        </span>
      }
      style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}
      extra={
        <Button
          type="primary"
          size="small"
          icon={<SaveOutlined />}
          loading={save.isPending}
          onClick={() => void handleSave()}
        >
          保存
        </Button>
      }
    >
      <Paragraph type="secondary" style={{ fontSize: 12 }}>
        勾选的菜单对本组织全员隐藏（包括你自己）。这里只能缩小可见范围，不会放大任何角色的权限；
        且只影响菜单显示，不影响接口鉴权。
      </Paragraph>
      <Checkbox.Group value={hidden} onChange={(v) => setHidden(v as string[])}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          {HIDEABLE_MENUS.map((m) => (
            <Checkbox key={m.key} value={m.key}>
              {m.label}
            </Checkbox>
          ))}
        </div>
      </Checkbox.Group>
      <div style={{ marginTop: 8 }}>
        <Text type="secondary" style={{ fontSize: 11 }}>
          提示：勾选项按「菜单键」记录，被隐藏的菜单不会出现在侧边栏。
        </Text>
      </div>
    </Card>
  )
}