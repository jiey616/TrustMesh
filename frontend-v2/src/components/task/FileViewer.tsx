import { lazy, Suspense, useCallback, useEffect, useState } from 'react'
import { Modal, Button } from 'antd'
import { DownloadOutlined, LoadingOutlined } from '@ant-design/icons'
import { Markdown } from './Markdown'

// Open File Viewer 懒加载（PDF / Office / 音视频 / 压缩包等格式预览，按需加载 chunk）
const OpenFileViewerLazy = lazy(() => import('./OpenFileViewer').then((m) => ({ default: m.OpenFileViewer })))

function ViewerLoading() {
  return (
    <div style={{ textAlign: 'center', padding: 48 }}>
      <LoadingOutlined style={{ fontSize: 28, color: 'var(--text-quaternary)' }} />
    </div>
  )
}

type FileCategory = 'text' | 'markdown' | 'code' | 'image' | 'pdf' | 'unknown'

interface FileViewerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  blob: Blob | null
  fileName: string
  onDownload?: () => void
}

const CODE_EXTENSIONS = new Set([
  'js', 'jsx', 'ts', 'tsx', 'py', 'go', 'rs', 'java', 'c', 'cpp', 'h', 'hpp',
  'rb', 'php', 'sh', 'bash', 'zsh', 'fish', 'ps1',
  'json', 'yaml', 'yml', 'toml', 'xml', 'html', 'css', 'scss', 'less',
  'sql', 'graphql', 'proto', 'vue', 'svelte', 'astro',
])

const TEXT_LIKE_EXTENSIONS = new Set([
  'txt', 'log', 'csv', 'tsv', 'ini', 'cfg', 'conf', 'env', 'gitignore',
  'editorconfig', 'properties', 'md', 'markdown', 'rst',
])

function categorize(mimeType: string, fileName: string): FileCategory {
  const mime = mimeType.split(';')[0].trim().toLowerCase()
  const ext = fileName.split('.').pop()?.toLowerCase() ?? ''
  if (mime.startsWith('image/')) return 'image'
  if (mime === 'application/pdf') return 'pdf'
  if (ext === 'md' || ext === 'markdown' || mime === 'text/markdown') return 'markdown'
  if (CODE_EXTENSIONS.has(ext) || mime === 'application/json') return 'code'
  if (mime.startsWith('text/') || mime === 'application/xml') return 'text'
  if (TEXT_LIKE_EXTENSIONS.has(ext)) return ext === 'md' || ext === 'markdown' ? 'markdown' : 'text'
  return 'unknown'
}

/** Try to decode binary data as text, with fallback encoding detection. */
async function decodeText(blob: Blob): Promise<string> {
  const buffer = await blob.arrayBuffer()
  const bytes = new Uint8Array(buffer)
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes)
  } catch {
    // not valid UTF-8
  }
  try {
    return new TextDecoder('gbk', { fatal: true }).decode(bytes)
  } catch {
    // not valid GBK
  }
  return new TextDecoder('iso-8859-1').decode(bytes)
}

export function FileViewer({ open, onOpenChange, blob, fileName, onDownload }: FileViewerProps) {
  const [textContent, setTextContent] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const mimeType = blob?.type ?? 'application/octet-stream'
  const category = categorize(mimeType, fileName)

  const loadContent = useCallback(async () => {
    if (!blob) return
    setLoading(true)
    try {
      if (category === 'text' || category === 'markdown' || category === 'code') {
        setTextContent(await decodeText(blob))
      } else {
        setTextContent(null)
      }
    } finally {
      setLoading(false)
    }
  }, [blob, category])

  useEffect(() => {
    if (open && blob) void loadContent()
    if (!open) setTextContent(null)
  }, [open, blob, loadContent])

  return (
    <Modal
      open={open}
      onCancel={() => onOpenChange(false)}
      width="80vw"
      style={{ top: 24 }}
      footer={
        onDownload ? (
          <Button icon={<DownloadOutlined />} onClick={onDownload}>下载</Button>
        ) : null
      }
      title={<span style={{ color: 'var(--text-primary)' }}>{fileName}</span>}
      styles={{ body: { maxHeight: '70vh', overflow: 'auto' } }}
    >
      {loading ? (
        <div style={{ textAlign: 'center', padding: 48 }}>
          <LoadingOutlined style={{ fontSize: 28, color: 'var(--text-quaternary)' }} />
        </div>
      ) : category === 'text' || category === 'code' ? (
        <pre
          style={{
            margin: 0,
            padding: 16,
            borderRadius: 'var(--radius-control)',
            background: 'var(--surface-inset)',
            fontSize: 12,
            lineHeight: 1.6,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            fontFamily: 'Consolas, "Courier New", monospace',
            color: 'var(--text-primary)',
          }}
        >
          {textContent}
        </pre>
      ) : category === 'markdown' ? (
        <div style={{ padding: 8 }}>
          <Markdown content={textContent ?? ''} />
        </div>
      ) : (category === 'image' || category === 'pdf' || category === 'unknown') && blob ? (
        <Suspense fallback={<ViewerLoading />}>
          <OpenFileViewerLazy blob={blob} fileName={fileName} />
        </Suspense>
      ) : (
        <div style={{ textAlign: 'center', padding: 48, color: 'var(--text-quaternary)' }}>
          该文件类型不支持预览
          {onDownload && (
            <div style={{ marginTop: 12 }}>
              <Button icon={<DownloadOutlined />} onClick={onDownload}>下载文件</Button>
            </div>
          )}
        </div>
      )}
    </Modal>
  )
}
