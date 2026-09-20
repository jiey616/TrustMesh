import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  App,
  Button,
  Card,
  Empty,
  Input,
  Modal,
  Popconfirm,
  Progress,
  Space,
  Table,
  Tag,
  Typography,
  Upload,
} from 'antd'
import type { UploadFile } from 'antd'
import {
  CloudUploadOutlined,
  DeleteOutlined,
  InboxOutlined,
  MobileOutlined,
  RollbackOutlined,
  SendOutlined,
  WindowsOutlined,
} from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import {
  useDeleteDesktopRelease,
  useDesktopReleases,
  usePublishDesktopRelease,
  useRollbackDesktopRelease,
  useUploadDesktopRelease,
} from '@/hooks/useDesktopReleases'
import { deleteMobileAppRelease, getMobileAppRelease, uploadMobileAppRelease } from '@/api/mobileApp'
import { getApiBase } from '@/stores/serverConfigStore'
import { validateUploadPair } from '@/lib/desktopRelease'
import { ApiRequestError } from '@/types'
import type {
  DesktopReleaseMetadata,
  DesktopReleaseStatus,
  PlatformDesktopReleaseView,
} from '@/types'

const { Text } = Typography

/**
 * 平台管理 · 桌面端安装包（自建更新源）。
 *
 * 方案：docs/desktop-app-update-plan-2026-09-18.md
 *
 * 这里是**唯一**能改变「全体桌面客户端拿到哪个版本」的地方，因此：
 *   - 版本号一律来自随包上传的 release.json（由 _deploy_desktop.py 产出），不手填；
 *   - 发布 / 回滚 / 删除全部二次确认，且确认文案写明影响面（尤其回滚的语义边界）；
 *   - 上传带进度条（87MB，无进度等于无可用性）。
 */

const STATUS_META: Record<DesktopReleaseStatus, { color: string; label: string }> = {
  draft: { color: 'default', label: '未发布' },
  published: { color: 'success', label: '分发中' },
  archived: { color: 'default', label: '历史版本' },
}

/**
 * 列宽必须逐列显式给数值。不给的话 antd 走 `table-layout: auto`，宽度由内容决定，
 * 「操作」列里那排按钮会把该列撑得很宽，整表宽度超出容器 —— 100% 缩放下操作列
 * 直接被挤出可视区（表现为「不显示操作列」）。
 *
 * `scroll.x` 取各列之和：容器更窄时改为表内横向滚动 + 操作列固定在右侧（永远可见），
 * 而不是压缩列或让页面整体横向溢出。
 * 数值按内容实测留量：sha512 是 12 位截断的等宽字；上传时间是 zh-CN 本地化串
 * （`2026/9/18 16:22:08`）；操作列 = 3 个 link 按钮（图标 14 + 2 汉字 28 + 内边距 14）≈ 3×64。
 */
const COL_WIDTH = {
  version: 120,
  status: 96,
  file: 280,
  sha512: 130,
  notes: 200,
  createdAt: 160,
  actions: 240,
} as const

const TABLE_SCROLL_X = Object.values(COL_WIDTH).reduce((sum, w) => sum + w, 0)

function formatBytes(n: number): string {
  if (!n) return '-'
  const units = ['B', 'KB', 'MB', 'GB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i += 1
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function formatTime(v?: string): string {
  if (!v) return '-'
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString('zh-CN', { hour12: false })
}

interface FileSlotProps {
  label: string
  hint: string
  accept: string
  required?: boolean
  fileList: UploadFile[]
  onChange: (files: UploadFile[]) => void
  dragger?: boolean
}

/** 只收集文件、不自动上传（beforeUpload 返回 false），由外部按钮统一提交。 */
function FileSlot({ label, hint, accept, required, fileList, onChange, dragger }: FileSlotProps) {
  const body = dragger ? (
    <Upload.Dragger
      accept={accept}
      maxCount={1}
      fileList={fileList}
      beforeUpload={() => false}
      onChange={({ fileList: fl }) => onChange(fl.slice(-1))}
      onRemove={() => onChange([])}
    >
      <p className="ant-upload-drag-icon">
        <InboxOutlined />
      </p>
      <p className="ant-upload-text" style={{ fontSize: 13 }}>
        {hint}
      </p>
    </Upload.Dragger>
  ) : (
    <Upload
      accept={accept}
      maxCount={1}
      fileList={fileList}
      beforeUpload={() => false}
      onChange={({ fileList: fl }) => onChange(fl.slice(-1))}
      onRemove={() => onChange([])}
    >
      <Button icon={<CloudUploadOutlined />}>{hint}</Button>
    </Upload>
  )
  return (
    <div>
      <div style={{ marginBottom: 6, fontSize: 13 }}>
        {label}
        {required ? <Text type="danger"> *</Text> : <Text type="secondary">（可选）</Text>}
      </div>
      {body}
    </div>
  )
}

/**
 * 移动端安装包（Android APK）管理卡片：单条 current 记录，上传即覆盖。
 * 登录页「移动端扫码下载」二维码直接指向该包的公开下载地址（/mobile/app/download），
 * 未上传时登录页自动隐藏扫码入口。
 */
function MobileAppCard() {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [apkFiles, setApkFiles] = useState<UploadFile[]>([])
  const [version, setVersion] = useState('')

  const { data } = useQuery({ queryKey: ['mobile-app'], queryFn: getMobileAppRelease })
  const current = data?.data.mobile_app

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['mobile-app'] })
  }

  const upload = useMutation({
    mutationFn: ({ file, ver }: { file: File; ver: string }) => uploadMobileAppRelease(file, ver),
    onSuccess: (res) => {
      message.success(`移动端安装包已更新到 ${res.data.mobile_app.version}，登录页扫码即刻生效`)
      setApkFiles([])
      setVersion('')
      invalidate()
    },
    onError: (err: unknown) => {
      message.error(err instanceof ApiRequestError ? err.message : '上传失败')
    },
  })

  const remove = useMutation({
    mutationFn: deleteMobileAppRelease,
    onSuccess: () => {
      message.success('已删除移动端安装包，登录页扫码入口将隐藏')
      invalidate()
    },
    onError: (err: unknown) => {
      message.error(err instanceof ApiRequestError ? err.message : '删除失败')
    },
  })

  const handleUpload = () => {
    const file = apkFiles[0]?.originFileObj
    const ver = version.trim()
    if (!file || !ver) {
      message.warning('请选择 .apk 文件并填写版本号')
      return
    }
    if (!file.name.includes(ver)) {
      message.error(`文件名 ${file.name} 不含版本号 ${ver}，请确认没有选错包或填错版本`)
      return
    }
    upload.mutate({ file, ver })
  }

  return (
    <Card
      bordered={false}
      style={{ background: 'var(--surface)', border: '1px solid var(--line)', marginTop: 16 }}
      title={
        <span>
          <MobileOutlined style={{ marginRight: 8 }} />
          移动端安装包（Android）
        </span>
      }
    >
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        {current ? (
          <Text style={{ fontSize: 13 }}>
            当前版本 <b>v{current.version}</b>
            <Text type="secondary" style={{ fontSize: 12 }}>
              {' '}
              · {current.file_name} · {formatBytes(current.size)} · 更新于 {formatTime(current.updated_at)}
            </Text>
          </Text>
        ) : (
          <Empty description="尚未上传移动端安装包，登录页不显示扫码下载入口" imageStyle={{ height: 48 }} />
        )}
        <Space wrap>
          <Upload
            accept=".apk"
            maxCount={1}
            fileList={apkFiles}
            beforeUpload={() => false}
            onChange={({ fileList: fl }) => setApkFiles(fl.slice(-1))}
            onRemove={() => setApkFiles([])}
          >
            <Button icon={<CloudUploadOutlined />}>选择 .apk 安装包</Button>
          </Upload>
          <Input
            placeholder="版本号，如 1.0.0"
            value={version}
            onChange={(e) => setVersion(e.target.value)}
            style={{ width: 180 }}
            allowClear
          />
          <Button type="primary" loading={upload.isPending} onClick={handleUpload}>
            上传并覆盖
          </Button>
          <Popconfirm
            title={`删除移动端安装包 v${current?.version ?? ''}？`}
            description="会同时删除服务器上的 APK 文件，登录页扫码入口随之隐藏，不可撤销。"
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
            onConfirm={() => remove.mutate()}
            disabled={!current}
          >
            <Button danger icon={<DeleteOutlined />} disabled={!current} loading={remove.isPending}>
              删除
            </Button>
          </Popconfirm>
        </Space>
        <Text type="secondary" style={{ fontSize: 12 }}>
          版本号必须出现在文件名里（如 TrustMesh-1.0.0.apk）。上传后登录页「移动端扫码下载」二维码立即指向新包；
          下载地址公开（扫码发生在未登录的手机浏览器里），APK 不含任何租户数据。
        </Text>
      </Space>
    </Card>
  )
}

export function DesktopReleasesPage() {
  const { message } = App.useApp()
  const { data: releases, isLoading } = useDesktopReleases()
  const publishRelease = usePublishDesktopRelease()
  const rollbackRelease = useRollbackDesktopRelease()
  const deleteRelease = useDeleteDesktopRelease()
  const uploadRelease = useUploadDesktopRelease()

  const [modalOpen, setModalOpen] = useState(false)
  const [installerFiles, setInstallerFiles] = useState<UploadFile[]>([])
  const [metadataFiles, setMetadataFiles] = useState<UploadFile[]>([])
  const [blockmapFiles, setBlockmapFiles] = useState<UploadFile[]>([])
  const [percent, setPercent] = useState(0)

  // 更新源地址（web 与桌面端都要显示正确的绝对地址，供手工核对）。
  const feedUrl = useMemo(() => {
    try {
      return new URL(`${getApiBase()}desktop/releases/feed/latest.yml`, window.location.origin).href
    } catch {
      return `${getApiBase()}desktop/releases/feed/latest.yml`
    }
  }, [])

  const resetModal = () => {
    setInstallerFiles([])
    setMetadataFiles([])
    setBlockmapFiles([])
    setPercent(0)
  }

  const closeModal = () => {
    setModalOpen(false)
    resetModal()
  }

  const handlePublish = async (rel: PlatformDesktopReleaseView) => {
    try {
      const res = await publishRelease.mutateAsync(rel.id)
      const pruned = res.data.pruned ?? []
      message.success(
        pruned.length > 0
          ? `已发布 ${rel.version}，并清理旧版本：${pruned.join('、')}`
          : `已发布 ${rel.version}`,
      )
    } catch (err) {
      message.error(err instanceof ApiRequestError ? err.message : '发布失败')
    }
  }

  const handleRollback = async (rel: PlatformDesktopReleaseView) => {
    try {
      await rollbackRelease.mutateAsync(rel.id)
      message.success(`分发指针已指回 ${rel.version}`)
    } catch (err) {
      message.error(err instanceof ApiRequestError ? err.message : '回滚失败')
    }
  }

  const handleDelete = async (rel: PlatformDesktopReleaseView) => {
    try {
      await deleteRelease.mutateAsync(rel.id)
      message.success(`已删除 ${rel.version}`)
    } catch (err) {
      message.error(err instanceof ApiRequestError ? err.message : '删除失败')
    }
  }

  const handleUpload = async () => {
    const installer = installerFiles[0]?.originFileObj
    const metadata = metadataFiles[0]?.originFileObj
    const blockmap = blockmapFiles[0]?.originFileObj
    if (!installer || !metadata) {
      message.warning('请选择安装包与 release.json')
      return
    }

    let meta: DesktopReleaseMetadata
    try {
      meta = JSON.parse(await metadata.text()) as DesktopReleaseMetadata
    } catch {
      message.error('release.json 解析失败，请确认上传的是 _deploy_desktop.py 产出的文件')
      return
    }
    // 配对校验走纯函数（lib/desktopRelease.ts，有单测覆盖）：前端只负责早失败、
    // 省一次 87MB 往返；真正的复核在服务端（重算 size 与 sha512）。
    const pair = validateUploadPair(installer.name, meta)
    if (!pair.ok) {
      message.error(pair.reason ?? '安装包与 release.json 不匹配')
      return
    }

    try {
      setPercent(0)
      const rel = await uploadRelease.mutateAsync({
        input: { installer, metadata, blockmap },
        onProgress: setPercent,
      })
      message.success(`已上传 ${rel.version}（尚未发布，请在列表中确认后发布）`)
      closeModal()
    } catch (err) {
      message.error(
        err instanceof ApiRequestError
          ? err.message
          : err instanceof Error
            ? err.message
            : '上传失败',
      )
    }
  }

  const columns = [
    {
      title: '版本',
      dataIndex: 'version',
      key: 'version',
      width: COL_WIDTH.version,
      render: (version: string, rel: PlatformDesktopReleaseView) => (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
          <b>{version}</b>
          {rel.channel !== 'stable' && <Tag>{rel.channel}</Tag>}
        </span>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: COL_WIDTH.status,
      render: (status: DesktopReleaseStatus) => {
        const meta = STATUS_META[status] ?? { color: 'default', label: status }
        return <Tag color={meta.color}>{meta.label}</Tag>
      },
    },
    {
      title: '安装包',
      dataIndex: 'file_name',
      key: 'file_name',
      width: COL_WIDTH.file,
      render: (name: string, rel: PlatformDesktopReleaseView) => (
        <span>
          <Text ellipsis={{ tooltip: name }} style={{ fontSize: 12 }}>
            {name}
          </Text>
          <div>
            <Text type="secondary" style={{ fontSize: 12 }}>
              {formatBytes(rel.size)}
              {rel.block_map_size ? ` · 差分 ${formatBytes(rel.block_map_size)}` : ' · 无差分包'}
            </Text>
          </div>
        </span>
      ),
    },
    {
      title: 'sha512',
      dataIndex: 'sha512',
      key: 'sha512',
      width: COL_WIDTH.sha512,
      render: (v: string) => (
        <Text code style={{ fontSize: 11 }}>
          {v ? `${v.slice(0, 12)}…` : '-'}
        </Text>
      ),
    },
    {
      title: '说明',
      dataIndex: 'notes',
      key: 'notes',
      width: COL_WIDTH.notes,
      render: (v?: string) =>
        v ? (
          <Text style={{ fontSize: 12 }} ellipsis={{ tooltip: v }}>
            {v}
          </Text>
        ) : (
          <Text type="secondary" style={{ fontSize: 12 }}>
            -
          </Text>
        ),
    },
    {
      title: '上传时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: COL_WIDTH.createdAt,
      render: (v: string) => <Text style={{ fontSize: 12 }}>{formatTime(v)}</Text>,
    },
    {
      title: '操作',
      key: 'actions',
      width: COL_WIDTH.actions,
      // 固定在右侧：容器比表窄而出现横向滚动时，操作列始终可见。
      fixed: 'right' as const,
      render: (_: unknown, rel: PlatformDesktopReleaseView) => (
        <Space size={0}>
          <Popconfirm
            title={`发布 ${rel.version}？`}
            description="发布后，所有尚未升级的桌面客户端会在下次启动时收到更新提示。"
            okText="发布"
            cancelText="取消"
            onConfirm={() => void handlePublish(rel)}
            disabled={rel.status === 'published'}
          >
            <Button
              type="link"
              size="small"
              icon={<SendOutlined />}
              disabled={rel.status === 'published'}
            >
              发布
            </Button>
          </Popconfirm>
          <Popconfirm
            title={`回滚到 ${rel.version}？`}
            description="只把分发指针指回该版本：已升级到更高版本的客户端不受影响（更新器只认「远程版本更新」，没有降级通道）。"
            okText="回滚"
            cancelText="取消"
            onConfirm={() => void handleRollback(rel)}
            disabled={rel.status !== 'archived'}
          >
            <Button
              type="link"
              size="small"
              icon={<RollbackOutlined />}
              disabled={rel.status !== 'archived'}
            >
              回滚
            </Button>
          </Popconfirm>
          <Popconfirm
            title={`删除 ${rel.version}？`}
            description="会同时删除服务器上的安装包文件，不可撤销。"
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
            onConfirm={() => void handleDelete(rel)}
            disabled={rel.status === 'published'}
          >
            <Button
              type="link"
              size="small"
              danger
              icon={<DeleteOutlined />}
              disabled={rel.status === 'published'}
            >
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div style={{ paddingBottom: 40 }}>
      <PageHeader
        title="平台管理 · 桌面端安装包"
        icon={<WindowsOutlined />}
        subtitle="自建更新源：桌面端启动时从这里检查新版本"
        actions={
          <Button type="primary" icon={<CloudUploadOutlined />} onClick={() => setModalOpen(true)}>
            上传新版本
          </Button>
        }
      />

      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message="更新源地址"
        description={
          <span>
            <Text code copyable style={{ fontSize: 12 }}>
              {feedUrl}
            </Text>
            <div style={{ marginTop: 6, fontSize: 12 }}>
              该地址是公开只读的（更新器跑在独立会话分区、不携带登录态，否则登出已久的机器将永远升不了级）。
              保留最近 5 个版本，发布时自动清理最旧的历史版本。
            </div>
          </span>
        }
      />

      <Alert
        type="warning"
        showIcon
        style={{ marginBottom: 16 }}
        message="0.2.0 需要手动安装一次"
        description="当前线上的 0.1.0 不含更新配置，无法自更新。请把 0.2.0 安装到每台机器上一次，之后才能自动升级。"
      />

      <Card bordered={false} style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}>
        <Table
          rowKey="id"
          columns={columns}
          dataSource={releases ?? []}
          loading={isLoading}
          pagination={false}
          scroll={{ x: TABLE_SCROLL_X }}
          locale={{ emptyText: <Empty description="还没有上传过桌面端安装包" /> }}
        />
      </Card>

      <MobileAppCard />

      <Modal
        open={modalOpen}
        title="上传桌面端安装包"
        width={620}
        okText="上传"
        cancelText="取消"
        confirmLoading={uploadRelease.isPending}
        okButtonProps={{ disabled: !installerFiles.length || !metadataFiles.length }}
        onOk={() => void handleUpload()}
        onCancel={closeModal}
        maskClosable={!uploadRelease.isPending}
        closable={!uploadRelease.isPending}
      >
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            message="请先用 _deploy_desktop.py 打包"
            description="版本号来自 release.json，不在界面上填写。上传时服务端会重算 sha512 与文件大小，不一致直接拒绝。"
          />
          <FileSlot
            label="安装包（.exe）"
            hint="拖拽 TrustMesh-Setup-x.y.z.exe 到这里"
            accept=".exe"
            required
            dragger
            fileList={installerFiles}
            onChange={setInstallerFiles}
          />
          <FileSlot
            label="release.json"
            hint="选择 release.json"
            accept=".json"
            required
            fileList={metadataFiles}
            onChange={setMetadataFiles}
          />
          <FileSlot
            label="差分包（.blockmap）"
            hint="选择 .blockmap"
            accept=".blockmap"
            fileList={blockmapFiles}
            onChange={setBlockmapFiles}
          />
          {uploadRelease.isPending && (
            <div>
              <Progress percent={percent} status="active" />
              <Text type="secondary" style={{ fontSize: 12 }}>
                正在上传，请勿关闭窗口（超过 100MB 时可能需要一两分钟）
              </Text>
            </div>
          )}
        </Space>
      </Modal>
    </div>
  )
}
