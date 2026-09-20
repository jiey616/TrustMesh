import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Card, Empty, Popconfirm, Space, Spin, Typography, Upload } from 'antd'
import { DeleteOutlined, UploadOutlined } from '@ant-design/icons'
import { App } from 'antd'
import dayjs from 'dayjs'
import { ApiRequestError } from '@/types'
import { usePermStore } from '@/stores/permStore'
import { deletePlatformGuide, getGuide, getPlatformGuide, uploadPlatformGuide } from '@/api/platformGuide'

const { Text } = Typography

const MAX_GUIDE_BYTES = 2 << 20

function GuideFrame({ html }: { html: string }) {
  return (
    // sandbox=""：全限制（最严格档）——脚本/表单/弹窗/同源访问一律禁用。
    // 管理员上传的 HTML 即使被注入脚本也绝不会执行；文档需自包含样式
    // （内联 CSS / data URI 图片），相对路径资源在沙箱内不可达。
    <iframe
      title="使用引导"
      sandbox=""
      srcDoc={html}
      style={{ width: '100%', height: '100%', minHeight: 480, border: 'none', background: '#fff' }}
    />
  )
}

function formatSize(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(2)} MB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${bytes} B`
}

/** 普通用户视图：只读渲染。 */
function GuideViewer() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['guide'],
    queryFn: getGuide,
  })

  if (isLoading) return <Spin />
  const guide = data?.data.guide
  if (error || !guide) {
    return <Empty description={error ? '加载失败' : '暂无使用引导，请联系平台管理员上传'} />
  }
  return <GuideFrame html={guide.html} />
}

/** 平台管理员视图：元信息 + 上传/删除 + 同一沙箱预览。 */
function GuideAdmin() {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [pendingFile, setPendingFile] = useState<File | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['platform-guide'],
    queryFn: getPlatformGuide,
  })
  const guide = data?.data.guide

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['platform-guide'] })
    void queryClient.invalidateQueries({ queryKey: ['guide'] })
  }

  const uploadMutation = useMutation({
    mutationFn: (file: File) => uploadPlatformGuide(file),
    onSuccess: () => {
      message.success('使用引导已更新')
      setPendingFile(null)
      invalidate()
    },
    onError: (err: unknown) => {
      const msg = err instanceof ApiRequestError ? err.message : '上传失败'
      message.error(msg)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deletePlatformGuide,
    onSuccess: () => {
      message.success('已删除使用引导')
      invalidate()
    },
    onError: (err: unknown) => {
      const msg = err instanceof ApiRequestError ? err.message : '删除失败'
      message.error(msg)
    },
  })

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16, height: '100%' }}>
      <Card size="small">
        <Space direction="vertical" size={4} style={{ width: '100%' }}>
          <Space wrap>
            <Upload
              accept=".html"
              maxCount={1}
              beforeUpload={(file) => {
                if (!file.name.toLowerCase().endsWith('.html')) {
                  message.error('仅支持 .html 文件')
                  return Upload.LIST_IGNORE
                }
                if (file.size > MAX_GUIDE_BYTES) {
                  message.error('文件超过 2MiB 上限')
                  return Upload.LIST_IGNORE
                }
                setPendingFile(file)
                return false
              }}
              onRemove={() => setPendingFile(null)}
              fileList={pendingFile ? [{ uid: 'pending', name: pendingFile.name, status: 'done' }] : []}
            >
              <Button icon={<UploadOutlined />}>选择 .html 文件</Button>
            </Upload>
            <Button
              type="primary"
              disabled={!pendingFile}
              loading={uploadMutation.isPending}
              onClick={() => pendingFile && uploadMutation.mutate(pendingFile)}
            >
              上传并覆盖
            </Button>
            <Popconfirm
              title="确认删除当前使用引导？"
              description="删除后所有用户将看到空引导页。"
              onConfirm={() => deleteMutation.mutate()}
              disabled={!guide}
            >
              <Button danger icon={<DeleteOutlined />} disabled={!guide} loading={deleteMutation.isPending}>
                删除
              </Button>
            </Popconfirm>
          </Space>
          {guide ? (
            <Text type="secondary" style={{ fontSize: 12 }}>
              当前版本：{guide.file_name} · {formatSize(guide.size)} · 更新于{' '}
              {dayjs(guide.updated_at).format('YYYY-MM-DD HH:mm')}
            </Text>
          ) : (
            <Text type="secondary" style={{ fontSize: 12 }}>
              尚未上传使用引导。上传即覆盖旧版本；文档请自包含样式（内联 CSS / data URI 图片）。
            </Text>
          )}
        </Space>
      </Card>
      <Alert
        type="info"
        showIcon
        message="渲染安全"
        description="引导页在沙箱 iframe 中展示（禁用脚本）。请上传自包含的静态 HTML，外部脚本与相对路径资源不会生效。"
        style={{ flexShrink: 0 }}
      />
      <div style={{ flex: 1, minHeight: 0 }}>
        {isLoading ? <Spin /> : guide ? <GuideFrame html={guide.html} /> : <Empty description="暂无使用引导" />}
      </div>
    </div>
  )
}

export function GuidePage() {
  const isPlatformAdmin = usePermStore((s) => s.isPlatformAdmin)
  const permReady = usePermStore((s) => s.ready)

  if (!permReady) return <Spin />

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div style={{ flexShrink: 0 }}>
        <Typography.Title level={4} style={{ margin: 0 }}>
          使用引导
        </Typography.Title>
        <Text type="secondary">平台功能说明与操作指引（内容由平台管理员维护）</Text>
      </div>
      <div style={{ flex: 1, minHeight: 0 }}>{isPlatformAdmin ? <GuideAdmin /> : <GuideViewer />}</div>
    </div>
  )
}
