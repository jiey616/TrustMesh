import { useState } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import { Button, Tabs, Skeleton, Typography, Space, App } from 'antd'
import { ArrowLeftOutlined, DownloadOutlined, FileTextOutlined, ThunderboltOutlined, BookOutlined } from '@ant-design/icons'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { motion } from 'framer-motion'
import { useMarketRole } from '@/hooks/useMarket'
import { downloadRole } from '@/api/market'
import { InstallGuide } from '@/components/InstallGuide'
import { NeonBadge } from '@/components/NeonBadge'

const { Paragraph } = Typography

const DEPT_NEON: Record<string, 'purple' | 'blue' | 'cyan' | 'green' | 'amber' | 'rose'> = {
  engineering: 'blue',
  marketing: 'purple',
  design: 'rose',
  product: 'amber',
  'project-management': 'amber',
  testing: 'rose',
  support: 'green',
  specialized: 'purple',
  'creative-tech': 'cyan',
  finance: 'green',
  hr: 'cyan',
  legal: 'blue',
  'sales-marketing': 'purple',
  'supply-chain': 'amber',
  academic: 'cyan',
}

function MarkdownBody({ content }: { content: string }) {
  return (
    <div style={{ fontSize: 14, lineHeight: 1.8, color: '#c8ccd8' }}>
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
      <style>{`
        .role-markdown h1, .role-markdown h2, .role-markdown h3 { color: #f4f4f8; font-weight: 600; margin: 1.2em 0 0.6em; }
        .role-markdown h1 { font-size: 1.4em; }
        .role-markdown h2 { font-size: 1.2em; }
        .role-markdown h3 { font-size: 1.05em; }
        .role-markdown p, .role-markdown li { color: #c8ccd8; }
        .role-markdown a { color: #8b7ff8; }
        .role-markdown code { background: rgba(255,255,255,0.06); padding: 2px 6px; border-radius: 4px; font-size: 0.85em; color: #a78bfa; }
        .role-markdown pre { background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.07); padding: 12px; border-radius: 8px; overflow: auto; }
        .role-markdown pre code { background: transparent; padding: 0; color: #e2e8f0; }
        .role-markdown blockquote { border-left: 3px solid rgba(109,95,245,0.5); margin: 0.8em 0; padding-left: 12px; color: #8b8f9e; }
        .role-markdown table { border-collapse: collapse; margin: 0.8em 0; width: 100%; }
        .role-markdown th, .role-markdown td { border: 1px solid rgba(255,255,255,0.1); padding: 6px 10px; }
        .role-markdown th { background: rgba(255,255,255,0.04); }
      `}</style>
    </div>
  )
}

function RoleDetailSkeleton() {
  return (
    <div>
      <div style={{ marginBottom: 16 }}><Skeleton.Input active size="small" style={{ width: 120 }} /></div>
      <Skeleton active paragraph={{ rows: 2 }} />
      <div style={{ display: 'flex', gap: 16, marginTop: 24 }}>
        <div style={{ flex: 1 }}><Skeleton active paragraph={{ rows: 8 }} /></div>
        <div style={{ width: 340 }}><Skeleton active paragraph={{ rows: 4 }} /></div>
      </div>
    </div>
  )
}

export function RoleDetailPage() {
  const { message } = App.useApp()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const location = useLocation()
  const [downloading, setDownloading] = useState(false)
  const { data: role, isLoading } = useMarketRole(id)

  function handleBack() {
    const fromSearch = location.state?.fromSearch as string | undefined
    navigate(fromSearch ? `/market?${fromSearch}` : '/market')
  }

  async function handleDownload() {
    if (!id) return
    setDownloading(true)
    try {
      await downloadRole(id)
      message.success('角色包下载成功')
    } catch {
      message.error('下载失败，请重试')
    } finally {
      setDownloading(false)
    }
  }

  if (isLoading || !role) return <RoleDetailSkeleton />

  const deptNeon = DEPT_NEON[role.dept_id] ?? 'purple'

  return (
    <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.3, ease: 'easeOut' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
        <Button type="text" icon={<ArrowLeftOutlined />} onClick={handleBack}>返回岗位市场</Button>
        <Button type="primary" icon={<DownloadOutlined />} onClick={handleDownload} loading={downloading}>
          {downloading ? '下载中...' : '下载角色包'}
        </Button>
      </div>

      <div style={{ borderRadius: 16, border: '1px solid rgba(255,255,255,0.07)', background: 'rgba(255,255,255,0.03)', backdropFilter: 'blur(24px)', padding: '20px 24px', marginBottom: 16, position: 'relative', overflow: 'hidden' }}>
        <div style={{ position: 'absolute', top: -60, right: -40, width: 220, height: 220, borderRadius: '50%', background: 'radial-gradient(circle, rgba(109,95,245,0.18), transparent 70%)', pointerEvents: 'none' }} />
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 24 }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ marginBottom: 8 }}><NeonBadge label={role.dept_name} variant={deptNeon} size="sm" /></div>
            <h1 style={{ margin: 0, fontSize: 24, fontWeight: 700, letterSpacing: '-0.5px', color: '#f4f4f8' }}>{role.name}</h1>
            <Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0, maxWidth: 560 }}>{role.description}</Paragraph>
          </div>
          <div style={{ flexShrink: 0, borderRadius: 12, border: '1px solid rgba(255,255,255,0.07)', background: 'rgba(255,255,255,0.02)', padding: '12px 16px', minWidth: 260 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16, fontSize: 12, color: '#8b8f9e', marginBottom: 8 }}>
              <span>ID</span>
              <code style={{ fontFamily: 'var(--font-mono)', color: '#f4f4f8', fontSize: 11, wordBreak: 'break-all', textAlign: 'right', maxWidth: 180 }}>{role.id}</code>
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16, fontSize: 12, color: '#8b8f9e', marginBottom: 8 }}>
              <span>部门</span>
              <span style={{ color: '#f4f4f8' }}>{role.dept_name}</span>
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16, fontSize: 12, color: '#8b8f9e' }}>
              <span>格式</span>
              <span style={{ color: '#f4f4f8' }}>OpenClaw 数字员工</span>
            </div>
          </div>
        </div>
      </div>

      <div style={{ display: 'flex', gap: 16, alignItems: 'flex-start' }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ borderRadius: 16, border: '1px solid rgba(255,255,255,0.07)', background: 'rgba(255,255,255,0.02)', padding: '8px 24px 0' }}>
            <Tabs
              defaultActiveKey="soul"
              items={[
                { key: 'identity', label: <Space size={6}><FileTextOutlined />简介</Space>, children: <div style={{ padding: '20px 0 28px', maxHeight: 'calc(100vh - 380px)', overflow: 'auto' }}><MarkdownBody content={role.identity_content} /></div> },
                { key: 'soul', label: <Space size={6}><ThunderboltOutlined />人格</Space>, children: <div style={{ padding: '20px 0 28px', maxHeight: 'calc(100vh - 380px)', overflow: 'auto' }}><MarkdownBody content={role.soul_content} /></div> },
                { key: 'agents', label: <Space size={6}><BookOutlined />工作规范</Space>, children: <div style={{ padding: '20px 0 28px', maxHeight: 'calc(100vh - 380px)', overflow: 'auto' }}><MarkdownBody content={role.agents_content} /></div> },
              ]}
            />
          </div>
        </div>
        <div style={{ width: 380, flexShrink: 0 }}><InstallGuide role={role} /></div>
      </div>
    </motion.div>
  )
}