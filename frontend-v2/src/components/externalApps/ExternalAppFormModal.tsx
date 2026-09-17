import { useEffect } from 'react'
import { Checkbox, Form, Input, InputNumber, Modal, Select, Tag, Typography } from 'antd'
import type { CreateExternalAppRequest, ExternalAppScope, ExternalAppView } from '@/types'

const { Paragraph, Text } = Typography

const SSO_TYPE_OPTIONS = [{ value: 'trustmesh_jwt', label: 'TrustMesh 签发 JWT（推荐）' }]
const FRAME_MODE_OPTIONS = [
  { value: 'newtab', label: '新标签页打开' },
  { value: 'iframe', label: '内嵌 iframe（需外部平台支持）' },
]
/** 挂载点：决定这个平台出现在侧边栏还是项目详情页的 tab */
const PLACEMENT_OPTIONS = [
  { value: 'sidebar', label: '侧边栏菜单' },
  { value: 'project_tab', label: '项目详情 tab' },
]

const SCOPE_LABELS: Record<ExternalAppScope, string> = {
  global: '全局（平台管理员配置，全员可见可用）',
  org: '组织（本组织成员可见可用）',
  personal: '个人（仅我可见可用）',
}

/** 表单里的 placement 是多选数组，提交前压成后端要求的逗号分隔串 */
function normalizeFormValues(
  values: Record<string, unknown>,
  scope?: ExternalAppScope,
): CreateExternalAppRequest {
  const { placement, ...rest } = values as Record<string, unknown> & { placement?: unknown }
  return {
    ...(rest as unknown as CreateExternalAppRequest),
    ...(scope ? { scope } : {}),
    placement: Array.isArray(placement) ? (placement as string[]).join(',') : ((placement as string) ?? ''),
  }
}

export interface ExternalAppFormModalProps {
  open: boolean
  /** 编辑对象；null = 新建 */
  editing: ExternalAppView | null
  /** 新建时写入的作用域（编辑时仅只读展示层级，作用域创建后不可变更） */
  scope: ExternalAppScope
  confirmLoading?: boolean
  onCancel: () => void
  onSubmit: (values: CreateExternalAppRequest) => void
}

/**
 * 外部应用表单（业务侧三级页面与平台侧全局页复用）：
 * 层级只读展示 —— 作用域创建后不可变更，改层级必须重建（避免越权放大可见范围）。
 */
export function ExternalAppFormModal({
  open,
  editing,
  scope,
  confirmLoading,
  onCancel,
  onSubmit,
}: ExternalAppFormModalProps) {
  const [form] = Form.useForm()
  const effectiveScope = editing?.scope ?? scope

  useEffect(() => {
    if (!open) return
    if (editing) {
      form.setFieldsValue({
        name: editing.name,
        base_url: editing.base_url,
        client_id: editing.client_id,
        sso_type: editing.sso_type,
        frame_mode: editing.frame_mode,
        scopes: editing.scopes,
        placement: editing.placement
          ? editing.placement.split(',').map((p) => p.trim()).filter(Boolean)
          : [],
        icon_url: editing.icon_url,
        sort_order: editing.sort_order,
      })
    } else {
      form.resetFields()
      form.setFieldsValue({
        sso_type: 'trustmesh_jwt',
        frame_mode: 'newtab',
        placement: [],
        sort_order: 0,
      })
    }
  }, [open, editing, form])

  const handleOk = async () => {
    const raw = await form.validateFields()
    // 编辑时不带 scope：作用域创建后不可变更，后端也不接受该字段。
    onSubmit(normalizeFormValues(raw, editing ? undefined : scope))
  }

  return (
    <Modal
      title={editing ? '编辑外部平台' : '新增外部平台'}
      open={open}
      onCancel={onCancel}
      onOk={() => void handleOk()}
      confirmLoading={confirmLoading}
      okText={editing ? '保存' : '创建'}
      cancelText="取消"
      destroyOnClose
    >
      <Paragraph style={{ marginBottom: 12 }}>
        <Text type="secondary" style={{ fontSize: 12 }}>
          层级：
        </Text>
        <Tag color={effectiveScope === 'global' ? 'blue' : effectiveScope === 'org' ? 'purple' : 'default'}>
          {SCOPE_LABELS[effectiveScope]}
        </Tag>
        {editing && (
          <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>
            层级创建后不可变更，如需调整请删除后重建。
          </Text>
        )}
      </Paragraph>
      <Paragraph type="secondary" style={{ fontSize: 12 }}>
        {editing
          ? '修改该平台的连接配置。'
          : '登记一个可通过 SSO 免密打开的外部平台。创建后会生成 client_secret，请妥善保存。'}
      </Paragraph>
      <Form form={form} layout="vertical" preserve={false}>
        <Form.Item name="name" label="平台名称" rules={[{ required: true, message: '请输入平台名称' }]}>
          <Input placeholder="例如：画宗 AIGC 工厂" />
        </Form.Item>
        <Form.Item name="base_url" label="基础 URL" rules={[{ required: true, message: '请输入基础 URL' }]}>
          <Input placeholder="https://external.example.com/sso/entry" />
        </Form.Item>
        <Form.Item name="client_id" label="Client ID" rules={[{ required: true, message: '请输入 Client ID' }]}>
          <Input placeholder="外部平台分配给 TrustMesh 的标识" />
        </Form.Item>
        <Form.Item name="sso_type" label="SSO 类型">
          <Select options={SSO_TYPE_OPTIONS} />
        </Form.Item>
        <Form.Item name="frame_mode" label="打开方式">
          <Select options={FRAME_MODE_OPTIONS} />
        </Form.Item>
        <Form.Item name="scopes" label="Scopes（可选）">
          <Input.TextArea rows={2} placeholder="逗号分隔的权限范围，如：read,write" />
        </Form.Item>
        <Form.Item
          name="placement"
          label="挂载位置"
          extra="勾选后该平台会出现在对应入口；侧边栏不依赖 iframe，项目 tab 建议同时开启内嵌模式"
        >
          <Checkbox.Group options={PLACEMENT_OPTIONS} />
        </Form.Item>
        <Form.Item name="icon_url" label="图标 URL（可选）">
          <Input placeholder="https://cdn.example.com/icon.png" />
        </Form.Item>
        <Form.Item name="sort_order" label="排序权重（小的在前）">
          <InputNumber style={{ width: 140 }} min={0} max={9999} />
        </Form.Item>
      </Form>
    </Modal>
  )
}