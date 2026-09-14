import { useCallback, useEffect, useRef, useState } from 'react'
import { Download, Eye, FileText, Loader2, X } from 'lucide-react'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { cn, normalizeEscapedText } from '@/lib/utils'

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
  'sql', 'graphql', 'proto',
  'vue', 'svelte', 'astro',
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
  // application/octet-stream with recognizable extensions
  if (CODE_EXTENSIONS.has(ext)) return 'code'
  if (TEXT_LIKE_EXTENSIONS.has(ext)) return ext === 'md' || ext === 'markdown' ? 'markdown' : 'text'
  return 'unknown'
}

/** Try to decode binary data as text, with fallback encoding detection. */
async function decodeText(blob: Blob): Promise<string> {
  const buffer = await blob.arrayBuffer()
  const bytes = new Uint8Array(buffer)

  // Try UTF-8 first (with fatal: true to detect invalid sequences)
  try {
    const decoder = new TextDecoder('utf-8', { fatal: true })
    return decoder.decode(bytes)
  } catch {
    // not valid UTF-8
  }

  // Try GBK (common for Chinese Linux/Windows files)
  try {
    const decoder = new TextDecoder('gbk', { fatal: true })
    return decoder.decode(bytes)
  } catch {
    // not valid GBK
  }

  // Fallback to ISO-8859-1 (never throws, 1:1 byte mapping)
  const decoder = new TextDecoder('iso-8859-1')
  return decoder.decode(bytes)
}

export function FileViewer({ open, onOpenChange, blob, fileName, onDownload }: FileViewerProps) {
  const [textContent, setTextContent] = useState<string | null>(null)
  const [objectUrl, setObjectUrl] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const revokeRef = useRef<string | null>(null)

  const mimeType = blob?.type ?? 'application/octet-stream'
  const category = categorize(mimeType, fileName)

  const revokeUrl = useCallback(() => {
    if (revokeRef.current) {
      URL.revokeObjectURL(revokeRef.current)
      revokeRef.current = null
    }
  }, [])

  const loadContent = useCallback(async () => {
    if (!blob) return
    setLoading(true)
    revokeUrl()
    try {
      if (category === 'text' || category === 'markdown' || category === 'code') {
        const text = await decodeText(blob)
        setTextContent(text)
        setObjectUrl(null)
      } else if (category === 'image' || category === 'pdf') {
        const url = URL.createObjectURL(blob)
        revokeRef.current = url
        setObjectUrl(url)
        setTextContent(null)
      } else {
        setTextContent(null)
        setObjectUrl(null)
      }
    } finally {
      setLoading(false)
    }
  }, [blob, category, revokeUrl])

  useEffect(() => {
    if (open && blob) {
      void loadContent()
    }
    if (!open) {
      revokeUrl()
      setTextContent(null)
      setObjectUrl(null)
    }
  }, [open, blob, loadContent, revokeUrl])

  useEffect(() => revokeUrl, [revokeUrl])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="w-[calc(100vw-4rem)] sm:max-w-[calc(100vw-4rem)] max-h-[90vh] flex flex-col">
        <div className="flex items-center gap-3">
          <Eye className="size-4 shrink-0 text-muted-foreground" />
          <DialogTitle className="truncate min-w-0 leading-normal">{fileName}</DialogTitle>
          <span className="ml-auto shrink-0 text-xs text-muted-foreground">{mimeType}</span>
          {onDownload && (
            <Button type="button" variant="outline" size="sm" className="shrink-0" onClick={onDownload}>
              <Download className="size-3.5" />
              下载
            </Button>
          )}
          <DialogClose render={<Button variant="ghost" size="icon-sm" className="shrink-0" />}>
            <X className="size-4" />
          </DialogClose>
        </div>

        <div className="overflow-y-auto flex-1 min-h-0">
          {loading ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="size-6 animate-spin text-muted-foreground" />
            </div>
          ) : category === 'text' || category === 'code' ? (
            <pre className="rounded-md bg-muted p-4 text-xs leading-relaxed overflow-x-auto whitespace-pre-wrap wrap-break-word font-mono">
              {textContent}
            </pre>
          ) : category === 'markdown' ? (
            <div
              className={cn(
                'px-2',
                'prose prose-sm max-w-none text-foreground dark:prose-invert',
                'prose-p:my-2 prose-headings:my-3',
                'prose-ul:my-2 prose-ol:my-2 prose-li:my-0.5',
                'prose-pre:overflow-x-auto prose-pre:text-xs',
                'prose-code:text-xs prose-table:text-xs'
              )}
            >
              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                {normalizeEscapedText(textContent ?? '', { preserveMarkdownCode: true })}
              </ReactMarkdown>
            </div>
          ) : category === 'image' && objectUrl ? (
            <div className="flex items-center justify-center p-4">
              <img src={objectUrl} alt={fileName} className="max-w-full rounded-md" />
            </div>
          ) : category === 'pdf' && objectUrl ? (
            <iframe
              src={objectUrl}
              title={fileName}
              className="w-full rounded-md border"
              style={{ height: 'calc(90vh - 8rem)' }}
            />
          ) : (
            <div className="flex flex-col items-center justify-center gap-3 py-12 text-muted-foreground">
              <FileText className="size-10" />
              <p className="text-sm">该文件类型不支持预览</p>
              {onDownload && (
                <Button type="button" variant="outline" size="sm" onClick={onDownload}>
                  <Download className="size-3.5" />
                  下载文件
                </Button>
              )}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
