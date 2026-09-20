import { useMemo, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  App,
  Button,
  Card,
  Divider,
  Dropdown,
  Empty,
  Input,
  Popconfirm,
  Select,
  Space,
  Typography,
  Upload,
} from 'antd'
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  DeleteOutlined,
  PlusOutlined,
  UploadOutlined,
} from '@ant-design/icons'
import { ApiRequestError } from '@/types'
import {
  savePlatformManual,
  uploadManualImage,
  type ManualBlock,
  type ManualBlockData,
  type ManualBlockType,
  type ManualSection,
  type PlatformManual,
  type PlatformManualInput,
} from '@/api/platformManual'
import { ManualBlockView } from './ManualBlocks'

const { Text, Title } = Typography

const MAX_IMAGE_BYTES = 8 << 20

const BLOCK_LABELS: Record<ManualBlockType, string> = {
  paragraph: '段落',
  heading: '小节标题',
  steps: '步骤列表',
  tip: '提示条',
  warn: '警示条',
  image: '配图',
  table: '表格',
  flow: '流程链',
  qa: '问答',
}

/** 各类型新建时的初始数据（给出可渲染的最小骨架，避免空块）。 */
function emptyBlock(type: ManualBlockType): ManualBlock {
  const data: ManualBlockData = {}
  switch (type) {
    case 'paragraph':
    case 'heading':
    case 'tip':
    case 'warn':
      data.text = ''
      break
    case 'steps':
    case 'flow':
      data.items = ['']
      break
    case 'qa':
      data.qa_pairs = [{ q: '', a: '' }]
      break
    case 'table':
      data.head = ['列 1', '列 2']
      data.rows = [['', '']]
      break
    case 'image':
      data.url = ''
      data.alt = ''
      data.caption = ''
      break
  }
  return { type, data }
}

/** 短随机 id：仅需页内唯一（锚点 + 编辑定位），无需密码学强度。 */
function shortId(): string {
  return Math.random().toString(36).slice(2, 10)
}

/** 把服务端数据规整成可编辑副本（补齐缺省字段，避免 editor 里到处判空）。 */
function toDraft(m: PlatformManual | null): PlatformManualInput {
  return {
    title: m?.title ?? 'TrustMesh 平台操作手册',
    subtitle: m?.subtitle ?? '',
    footer: m?.footer ?? '',
    sections: (m?.sections ?? []).map((s) => ({
      id: s.id || shortId(),
      title: s.title ?? '',
      lead: s.lead ?? '',
      blocks: (s.blocks ?? []).map((b) => ({ type: b.type, data: { ...(b.data ?? {}) } })),
    })),
  }
}

/** 一行文本输入（用于 items / head / 表格单元格），带删除。 */
function LineRow({
  value,
  onChange,
  onRemove,
  placeholder,
}: {
  value: string
  onChange: (v: string) => void
  onRemove?: () => void
  placeholder?: string
}) {
  return (
    <Space.Compact style={{ width: '100%', marginBottom: 6 }}>
      <Input value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} />
      {onRemove && <Button icon={<DeleteOutlined />} onClick={onRemove} />}
    </Space.Compact>
  )
}

/** 单块编辑器：按 type 渲染对应表单。 */
function BlockEditor({
  block,
  onChange,
}: {
  block: ManualBlock
  onChange: (b: ManualBlock) => void
}) {
  const { message } = App.useApp()
  const type = block.type as ManualBlockType
  const d = block.data ?? {}
  const set = (patch: Partial<ManualBlockData>) => onChange({ ...block, data: { ...d, ...patch } })
  const setItem = (i: number, v: string) => {
    const items = [...(d.items ?? [])]
    items[i] = v
    set({ items })
  }

  const imageMutation = useMutation({
    mutationFn: uploadManualImage,
    onSuccess: (res) => {
      set({ url: res.data.image.url })
      message.success('图片已上传')
    },
    onError: (err: unknown) => {
      message.error(err instanceof ApiRequestError ? err.message : '图片上传失败')
    },
  })

  switch (type) {
    case 'paragraph':
    case 'heading':
    case 'tip':
    case 'warn':
      return (
        <Input.TextArea
          autoSize={{ minRows: 2, maxRows: 8 }}
          value={d.text ?? ''}
          onChange={(e) => set({ text: e.target.value })}
          placeholder={type === 'heading' ? '小节标题' : '支持 **加粗** 标记'}
        />
      )

    case 'steps':
    case 'flow':
      return (
        <div>
          {(d.items ?? []).map((it, i) => (
            <LineRow
              key={i}
              value={it}
              onChange={(v) => setItem(i, v)}
              onRemove={
                (d.items ?? []).length > 1
                  ? () => set({ items: (d.items ?? []).filter((_, k) => k !== i) })
                  : undefined
              }
              placeholder={`第 ${i + 1} 项`}
            />
          ))}
          <Button
            size="small"
            icon={<PlusOutlined />}
            onClick={() => set({ items: [...(d.items ?? []), ''] })}
          >
            添加一项
          </Button>
        </div>
      )

    case 'qa':
      return (
        <div>
          {(d.qa_pairs ?? []).map((p, i) => (
            <Card key={i} size="small" style={{ marginBottom: 8 }} styles={{ body: { padding: 10 } }}>
              <Input
                value={p.q}
                placeholder="问题"
                style={{ marginBottom: 6 }}
                onChange={(e) => {
                  const pairs = [...(d.qa_pairs ?? [])]
                  pairs[i] = { ...pairs[i], q: e.target.value }
                  set({ qa_pairs: pairs })
                }}
              />
              <Input.TextArea
                autoSize={{ minRows: 2, maxRows: 6 }}
                value={p.a}
                placeholder="答案（支持 **加粗**）"
                onChange={(e) => {
                  const pairs = [...(d.qa_pairs ?? [])]
                  pairs[i] = { ...pairs[i], a: e.target.value }
                  set({ qa_pairs: pairs })
                }}
              />
              {(d.qa_pairs ?? []).length > 1 && (
                <Button
                  size="small"
                  danger
                  style={{ marginTop: 6 }}
                  onClick={() => set({ qa_pairs: (d.qa_pairs ?? []).filter((_, k) => k !== i) })}
                >
                  删除此问答
                </Button>
              )}
            </Card>
          ))}
          <Button
            size="small"
            icon={<PlusOutlined />}
            onClick={() => set({ qa_pairs: [...(d.qa_pairs ?? []), { q: '', a: '' }] })}
          >
            添加一问
          </Button>
        </div>
      )

    case 'table': {
      const head = d.head ?? []
      const rows = d.rows ?? []
      const setCell = (ri: number, ci: number, v: string) => {
        const next = rows.map((r) => [...r])
        while (next[ri] && next[ri].length < head.length) next[ri].push('')
        if (next[ri]) next[ri][ci] = v
        set({ rows: next })
      }
      return (
        <div style={{ overflowX: 'auto' }}>
          <table style={{ borderCollapse: 'collapse', width: '100%', minWidth: 360 }}>
            <thead>
              <tr>
                {head.map((h, ci) => (
                  <th key={ci} style={{ padding: 4, minWidth: 120 }}>
                    <Input
                      value={h}
                      placeholder={`列 ${ci + 1}`}
                      onChange={(e) => {
                        const next = [...head]
                        next[ci] = e.target.value
                        set({ head: next })
                      }}
                    />
                  </th>
                ))}
                <th style={{ padding: 4, width: 40 }} />
              </tr>
            </thead>
            <tbody>
              {rows.map((r, ri) => (
                <tr key={ri}>
                  {head.map((_, ci) => (
                    <td key={ci} style={{ padding: 4 }}>
                      <Input value={r[ci] ?? ''} onChange={(e) => setCell(ri, ci, e.target.value)} />
                    </td>
                  ))}
                  <td style={{ padding: 4, textAlign: 'center' }}>
                    <Button
                      size="small"
                      icon={<DeleteOutlined />}
                      onClick={() => set({ rows: rows.filter((_, k) => k !== ri) })}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <Space style={{ marginTop: 8 }}>
            <Button
              size="small"
              icon={<PlusOutlined />}
              onClick={() => set({ rows: [...rows, head.map(() => '')] })}
            >
              添加行
            </Button>
            <Button
              size="small"
              icon={<PlusOutlined />}
              onClick={() => set({ head: [...head, `列 ${head.length + 1}`], rows: rows.map((r) => [...r, '']) })}
            >
              添加列
            </Button>
          </Space>
        </div>
      )
    }

    case 'image':
      return (
        <Space direction="vertical" style={{ width: '100%' }} size={8}>
          <Space wrap>
            <Upload
              accept="image/*"
              showUploadList={false}
              beforeUpload={(file) => {
                if (file.size > MAX_IMAGE_BYTES) {
                  message.error('图片超过 8MiB 上限')
                  return Upload.LIST_IGNORE
                }
                imageMutation.mutate(file)
                return false
              }}
            >
              <Button icon={<UploadOutlined />} loading={imageMutation.isPending}>
                上传图片
              </Button>
            </Upload>
            {d.url && <Text type="secondary" style={{ fontSize: 12 }}>{d.url}</Text>}
          </Space>
          <Input
            value={d.url ?? ''}
            placeholder="或直接粘贴图片地址"
            onChange={(e) => set({ url: e.target.value })}
          />
          <Input
            value={d.alt ?? ''}
            placeholder="替代文字（无障碍）"
            onChange={(e) => set({ alt: e.target.value })}
          />
          <Input
            value={d.caption ?? ''}
            placeholder="图注（显示在图片下方）"
            onChange={(e) => set({ caption: e.target.value })}
          />
        </Space>
      )

    default:
      return <Text type="secondary">未知块类型，无法编辑</Text>
  }
}

/** 单块外壳：类型选择 + 上移/下移/删除 + 内部表单 + 实时预览。 */
function BlockShell({
  block,
  index,
  total,
  onChange,
  onMove,
  onRemove,
}: {
  block: ManualBlock
  index: number
  total: number
  onChange: (b: ManualBlock) => void
  onMove: (dir: -1 | 1) => void
  onRemove: () => void
}) {
  const [showPreview, setShowPreview] = useState(false)
  return (
    <Card
      size="small"
      style={{ marginBottom: 10 }}
      styles={{ body: { padding: 12 } }}
      title={
        <Space size={8}>
          <Select<ManualBlockType>
            size="small"
            value={block.type as ManualBlockType}
            style={{ width: 108 }}
            onChange={(t) => onChange(emptyBlock(t))}
            options={(Object.keys(BLOCK_LABELS) as ManualBlockType[]).map((k) => ({
              value: k,
              label: BLOCK_LABELS[k],
            }))}
          />
          <Text type="secondary" style={{ fontSize: 12 }}>
            #{index + 1}
          </Text>
        </Space>
      }
      extra={
        <Space size={4}>
          <Button
            size="small"
            type={showPreview ? 'primary' : 'default'}
            onClick={() => setShowPreview((v) => !v)}
          >
            预览
          </Button>
          <Button size="small" icon={<ArrowUpOutlined />} disabled={index === 0} onClick={() => onMove(-1)} />
          <Button
            size="small"
            icon={<ArrowDownOutlined />}
            disabled={index === total - 1}
            onClick={() => onMove(1)}
          />
          <Button size="small" danger icon={<DeleteOutlined />} onClick={onRemove} />
        </Space>
      }
    >
      <BlockEditor block={block} onChange={onChange} />
      {showPreview && (
        <div
          style={{
            marginTop: 10,
            padding: 12,
            borderRadius: 'var(--radius-control)',
            background: 'var(--surface-inset)',
            border: '1px solid var(--line)',
          }}
        >
          <Text type="secondary" style={{ fontSize: 11 }}>
            渲染效果
          </Text>
          <ManualBlockView block={block} prefix={`pv-${index}`} />
        </div>
      )}
    </Card>
  )
}

export function ManualEditor({
  initial,
  onClose,
}: {
  initial: PlatformManual | null
  onClose: () => void
}) {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<PlatformManualInput>(() => toDraft(initial))
  const [activeSection, setActiveSection] = useState(0)

  const section = draft.sections[activeSection]

  const saveMutation = useMutation({
    mutationFn: savePlatformManual,
    onSuccess: () => {
      message.success('操作手册已保存')
      void queryClient.invalidateQueries({ queryKey: ['manual'] })
      void queryClient.invalidateQueries({ queryKey: ['platform-manual'] })
      onClose()
    },
    onError: (err: unknown) => {
      message.error(err instanceof ApiRequestError ? err.message : '保存失败')
    },
  })

  const patchSection = (i: number, patch: Partial<ManualSection>) => {
    setDraft((p) => {
      const sections = [...p.sections]
      sections[i] = { ...sections[i], ...patch }
      return { ...p, sections }
    })
  }

  const moveSection = (i: number, dir: -1 | 1) => {
    const j = i + dir
    if (j < 0 || j >= draft.sections.length) return
    setDraft((p) => {
      const sections = [...p.sections]
      ;[sections[i], sections[j]] = [sections[j], sections[i]]
      return { ...p, sections }
    })
    setActiveSection(j)
  }

  const removeSection = (i: number) => {
    setDraft((p) => ({ ...p, sections: p.sections.filter((_, k) => k !== i) }))
    setActiveSection((cur) => Math.max(0, cur >= i ? cur - 1 : cur))
  }

  const addSection = () => {
    setDraft((p) => ({
      ...p,
      sections: [...p.sections, { id: shortId(), title: '新章节', lead: '', blocks: [] }],
    }))
    setActiveSection(draft.sections.length)
  }

  const setBlock = (bi: number, b: ManualBlock) => {
    const blocks = [...section.blocks]
    blocks[bi] = b
    patchSection(activeSection, { blocks })
  }

  const moveBlock = (bi: number, dir: -1 | 1) => {
    const j = bi + dir
    if (j < 0 || j >= section.blocks.length) return
    const blocks = [...section.blocks]
    ;[blocks[bi], blocks[j]] = [blocks[j], blocks[bi]]
    patchSection(activeSection, { blocks })
  }

  const stats = useMemo(
    () => ({
      sections: draft.sections.length,
      blocks: draft.sections.reduce((n, s) => n + s.blocks.length, 0),
    }),
    [draft],
  )

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', gap: 14 }}>
      {/* ── 顶部操作条 ── */}
      <div
        style={{
          flexShrink: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 16,
          flexWrap: 'wrap',
        }}
      >
        <div>
          <Title level={4} style={{ margin: 0 }}>
            编辑操作手册
          </Title>
          <Text type="secondary" style={{ fontSize: 12 }}>
            共 {stats.sections} 章 · {stats.blocks} 个内容块 · 保存后立即对所有用户生效
          </Text>
        </div>
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" loading={saveMutation.isPending} onClick={() => saveMutation.mutate(draft)}>
            保存
          </Button>
        </Space>
      </div>

      {/* ── 封面字段 ── */}
      <Card size="small" styles={{ body: { padding: 12 } }} style={{ flexShrink: 0 }}>
        <Space direction="vertical" style={{ width: '100%' }} size={8}>
          <Input
            addonBefore="标题"
            value={draft.title}
            onChange={(e) => setDraft((p) => ({ ...p, title: e.target.value }))}
          />
          <Input
            addonBefore="副标题"
            value={draft.subtitle ?? ''}
            onChange={(e) => setDraft((p) => ({ ...p, subtitle: e.target.value }))}
          />
          <Input
            addonBefore="页脚"
            value={draft.footer ?? ''}
            placeholder="版本 / 日期等说明"
            onChange={(e) => setDraft((p) => ({ ...p, footer: e.target.value }))}
          />
        </Space>
      </Card>

      {/* ── 主体：章节栏 + 块编辑 ── */}
      <div style={{ flex: 1, minHeight: 0, display: 'flex', gap: 16 }}>
        <aside style={{ flex: '0 0 200px', display: 'flex', flexDirection: 'column', minHeight: 0 }}>
          <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', paddingRight: 4 }}>
            {draft.sections.map((s, i) => {
              const on = i === activeSection
              return (
                <div
                  key={s.id}
                  onClick={() => setActiveSection(i)}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 6,
                    padding: '6px 8px',
                    marginBottom: 2,
                    borderLeft: `2px solid ${on ? 'var(--signal)' : 'transparent'}`,
                    borderRadius: 'var(--radius-control)',
                    background: on ? 'var(--signal-soft)' : 'transparent',
                    color: on ? 'var(--signal)' : 'var(--text-tertiary)',
                    fontSize: 13,
                    cursor: 'pointer',
                  }}
                >
                  <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {i + 1}. {s.title || '未命名'}
                  </span>
                  <ArrowUpOutlined
                    style={{ fontSize: 10, opacity: 0.7 }}
                    onClick={(e) => {
                      e.stopPropagation()
                      moveSection(i, -1)
                    }}
                  />
                  <ArrowDownOutlined
                    style={{ fontSize: 10, opacity: 0.7 }}
                    onClick={(e) => {
                      e.stopPropagation()
                      moveSection(i, 1)
                    }}
                  />
                  <Popconfirm
                    title="删除该章节？"
                    onConfirm={() => removeSection(i)}
                    disabled={draft.sections.length <= 1}
                  >
                    <DeleteOutlined
                      style={{ fontSize: 11, opacity: 0.7 }}
                      onClick={(e) => e.stopPropagation()}
                    />
                  </Popconfirm>
                </div>
              )
            })}
          </div>
          <Button block icon={<PlusOutlined />} onClick={addSection} style={{ marginTop: 8 }}>
            添加章节
          </Button>
        </aside>

        <div style={{ flex: 1, minWidth: 0, minHeight: 0, overflowY: 'auto', paddingRight: 8 }}>
          {!section ? (
            <Empty description="左侧添加一个章节开始编辑" />
          ) : (
            <>
              <Space direction="vertical" style={{ width: '100%' }} size={8}>
                <Input
                  addonBefore="章节标题"
                  value={section.title}
                  onChange={(e) => patchSection(activeSection, { title: e.target.value })}
                />
                <Input
                  addonBefore="章节导语"
                  value={section.lead ?? ''}
                  placeholder="可选，一句话概述本章"
                  onChange={(e) => patchSection(activeSection, { lead: e.target.value })}
                />
              </Space>

              <Divider orientation="left" plain style={{ margin: '16px 0 12px' }}>
                内容块
              </Divider>

              {section.blocks.map((b, bi) => (
                <BlockShell
                  key={bi}
                  block={b}
                  index={bi}
                  total={section.blocks.length}
                  onChange={(nb) => setBlock(bi, nb)}
                  onMove={(dir) => moveBlock(bi, dir)}
                  onRemove={() =>
                    patchSection(activeSection, { blocks: section.blocks.filter((_, k) => k !== bi) })
                  }
                />
              ))}

              <Dropdown
                menu={{
                  items: (Object.keys(BLOCK_LABELS) as ManualBlockType[]).map((k) => ({
                    key: k,
                    label: BLOCK_LABELS[k],
                  })),
                  onClick: ({ key }) =>
                    patchSection(activeSection, {
                      blocks: [...section.blocks, emptyBlock(key as ManualBlockType)],
                    }),
                }}
              >
                <Button block type="dashed" icon={<PlusOutlined />}>
                  添加内容块
                </Button>
              </Dropdown>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
