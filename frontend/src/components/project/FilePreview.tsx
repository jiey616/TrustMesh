import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { X, Download, ChevronLeft, ChevronRight, Loader2, FileText, Image, FileCode, FileSpreadsheet } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { downloadProjectFile } from '@/api/projectFiles'
import type { ProjectFile } from '@/types'

interface Props {
  file: ProjectFile | null
  projectId: string
  files: ProjectFile[]
  onClose: () => void
  onNavigate: (file: ProjectFile) => void
}

function mimeCategory(mime: string): 'text' | 'image' | 'pdf' | 'csv' | 'code' | 'other' {
  if (!mime) return 'other'
  if (mime.startsWith('image/')) return 'image'
  if (mime === 'application/pdf') return 'pdf'
  if (mime === 'text/csv' || mime === 'text/tab-separated-values') return 'csv'
  const codeMimes = [
    'application/json', 'application/javascript', 'application/xml',
    'text/html', 'text/css', 'application/x-yaml', 'text/yaml',
    'application/x-sh', 'text/x-python', 'text/x-go', 'text/x-java',
    'text/x-c', 'text/x-c++', 'text/x-rust', 'text/x-typescript',
    'application/typescript', 'text/x-sql',
  ]
  if (codeMimes.some(c => mime.includes(c.replace('x-', '')))) return 'code'
  if (mime.startsWith('text/')) return 'text'
  return 'other'
}

function guessLang(mime: string, fileName: string): string {
  if (mime === 'application/json') return 'json'
  if (mime === 'application/javascript' || mime === 'text/javascript') return 'javascript'
  if (mime === 'application/typescript') return 'typescript'
  if (mime === 'text/html') return 'html'
  if (mime === 'text/css') return 'css'
  if (mime === 'application/xml' || mime === 'text/xml') return 'xml'
  if (mime === 'text/x-python' || (fileName && fileName.endsWith('.py'))) return 'python'
  if (mime === 'text/x-go' || (fileName && fileName.endsWith('.go'))) return 'go'
  if (mime === 'text/x-java' || (fileName && fileName.endsWith('.java'))) return 'java'
  if (mime === 'text/x-c' || (fileName && fileName.endsWith('.c'))) return 'c'
  if (mime === 'text/x-c++' || (fileName && fileName.endsWith('.cpp'))) return 'cpp'
  if (mime === 'text/x-rust' || (fileName && fileName.endsWith('.rs'))) return 'rust'
  if (mime === 'text/x-sql' || (fileName && fileName.endsWith('.sql'))) return 'sql'
  if (mime === 'application/x-yaml' || mime === 'text/yaml' || (fileName && (fileName.endsWith('.yml') || fileName.endsWith('.yaml')))) return 'yaml'
  if (mime === 'application/x-sh' || (fileName && fileName.endsWith('.sh'))) return 'bash'
  if (mime === 'text/x-typescript' || (fileName && fileName.endsWith('.tsx'))) return 'tsx'
  if (fileName && fileName.endsWith('.jsx')) return 'jsx'
  if (fileName && fileName.endsWith('.ts')) return 'typescript'
  if (fileName && fileName.endsWith('.js')) return 'javascript'
  return ''
}

const LANG_LABELS: Record<string, string> = {
  json: 'JSON', javascript: 'JavaScript', typescript: 'TypeScript',
  html: 'HTML', css: 'CSS', xml: 'XML', python: 'Python', go: 'Go',
  java: 'Java', c: 'C', cpp: 'C++', rust: 'Rust', sql: 'SQL',
  yaml: 'YAML', bash: 'Shell', tsx: 'TSX', jsx: 'JSX',
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function CodeView({ text, lang, fileName }: { text: string; lang: string; fileName?: string }) {
  const lines = text.split('\n')
  const lineCount = lines.length
  const label = lang ? (LANG_LABELS[lang] || lang) : (fileName ? 'Code' : 'Text')

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 px-3 py-1.5 border-b text-xs text-muted-foreground bg-muted/30 shrink-0">
        <FileCode className="size-3.5" />
        <span className="font-medium">{label}</span>
        <span className="tabular-nums">{lineCount} lines</span>
      </div>
      <div className="overflow-auto flex-1">
        <table className="w-full border-collapse font-mono text-xs leading-relaxed">
          <tbody>
            {lines.map((line, i) => (
              <tr key={i} className="hover:bg-muted/30">
                <td className="select-none text-right pr-3 pl-3 py-0.5 text-muted-foreground border-r w-12 align-top">
                  {i + 1}
                </td>
                <td className="pl-3 py-0.5 whitespace-pre-wrap break-all">
                  {line || '\u00A0'}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function ImageView({ url, fileName }: { url: string; fileName: string }) {
  return (
    <div className="flex flex-col items-center justify-center h-full p-4">
      <img
        src={url}
        alt={fileName}
        className="max-w-full max-h-full object-contain rounded"
        style={{ maxHeight: 'calc(80vh - 120px)' }}
      />
    </div>
  )
}

function PdfView({ url, fileName }: { url: string; fileName: string }) {
  return (
    <iframe
      src={url}
      title={fileName}
      className="w-full h-full border-0 rounded"
      style={{ minHeight: 'calc(80vh - 120px)' }}
    />
  )
}

function CsvView({ text }: { text: string; fileName: string }) {
  const lines = text.trim().split('\n')
  if (lines.length === 0) return <p className="p-4 text-sm text-muted-foreground">Empty file</p>
  const headers = lines[0].split(',')
  const rows = lines.slice(1).map(l => l.split(','))

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 px-3 py-1.5 border-b text-xs text-muted-foreground bg-muted/30 shrink-0">
        <FileSpreadsheet className="size-3.5" />
        <span>CSV</span>
        <span className="tabular-nums">{headers.length} cols, {rows.length} rows</span>
      </div>
      <div className="overflow-auto flex-1">
        <table className="w-full border-collapse text-xs">
          <thead className="sticky top-0 bg-muted/50">
            <tr>
              {headers.map((h, i) => (
                <th key={i} className="border px-2 py-1 text-left font-medium whitespace-nowrap">
                  {h.trim()}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.slice(0, 1000).map((row, ri) => (
              <tr key={ri} className="hover:bg-muted/20">
                {row.map((cell, ci) => (
                  <td key={ci} className="border px-2 py-0.5 whitespace-nowrap">
                    {cell.trim()}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
        {rows.length > 1000 && (
          <p className="text-xs text-muted-foreground p-2 text-center">
            Showing 1,000 of {rows.length} rows
          </p>
        )}
      </div>
    </div>
  )
}

function OtherView({ file, onDownload }: { file: ProjectFile; onDownload: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center h-full gap-4 p-8">
      <FileText className="size-16 text-muted-foreground" />
      <div className="text-center space-y-1">
        <p className="font-medium">{file.file_name}</p>
        <p className="text-sm text-muted-foreground">{file.mime_type || 'Unknown type'}</p>
        <p className="text-sm text-muted-foreground">{formatSize(file.file_size)}</p>
      </div>
      <p className="text-sm text-muted-foreground">此文件类型不支持在线预览</p>
      <Button onClick={onDownload} variant="outline" size="sm">
        <Download className="size-4 mr-1.5" />
        下载文件
      </Button>
    </div>
  )
}

// Main Component
export function FilePreview({ file, projectId, files, onClose, onNavigate }: Props) {
  const [content, setContent] = useState<string | null>(null)
  const [objectUrl, setObjectUrl] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const prevFileRef = useRef<string | null>(null)

  const category = useMemo(() => file ? mimeCategory(file.mime_type) : 'other', [file])
  const lang = useMemo(() => {
    if (!file) return ''
    if (category === 'code' || category === 'text') return guessLang(file.mime_type, file.file_name)
    return ''
  }, [file, category])

  const fileIndex = useMemo(() => {
    if (!file) return -1
    return files.findIndex(f => f.id === file.id)
  }, [file, files])

  const prevFile = fileIndex > 0 ? files[fileIndex - 1] : null
  const nextFile = fileIndex < files.length - 1 ? files[fileIndex + 1] : null

  const loadContent = useCallback(async () => {
    if (!file) return
    if (prevFileRef.current === file.id) return
    prevFileRef.current = file.id

    if (objectUrl) {
      URL.revokeObjectURL(objectUrl)
      setObjectUrl(null)
    }
    setContent(null)
    setError(null)

    if (category === 'other') return

    setLoading(true)
    try {
      const blob = await downloadProjectFile(projectId, file.id)
      if (category === 'image' || category === 'pdf') {
        const url = URL.createObjectURL(blob)
        setObjectUrl(url)
      } else {
        const text = await blob.text()
        setContent(text)
      }
    } catch (err: unknown) {
      console.error('Preview error:', err)
      setError('无法加载文件内容')
    } finally {
      setLoading(false)
    }
  }, [file, projectId, category])

  useEffect(() => {
    loadContent()
  }, [loadContent])

  useEffect(() => {
    return () => {
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [])

  if (!file) return null

  const handleDownload = () => {
    window.open(`/api/v1/projects/${projectId}/files/${file.id}/content`, '_blank')
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowLeft' && prevFile) {
      onNavigate(prevFile)
    } else if (e.key === 'ArrowRight' && nextFile) {
      onNavigate(nextFile)
    } else if (e.key === 'Escape') {
      onClose()
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 bg-black/50 flex items-center justify-center"
      onClick={(e) => { if (e.target === e.currentTarget) onClose() }}
      onKeyDown={handleKeyDown}
      tabIndex={0}
    >
      <div className="bg-background rounded-lg border shadow-xl flex flex-col" style={{ width: '80vw', maxWidth: '960px', height: '85vh', maxHeight: '700px' }}>
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-2.5 border-b shrink-0">
          <div className="flex items-center gap-2 min-w-0">
            {category === 'image' && <Image className="size-4 text-blue-500 shrink-0" />}
            {category === 'code' && <FileCode className="size-4 text-purple-500 shrink-0" />}
            {category === 'text' && <FileText className="size-4 text-amber-500 shrink-0" />}
            {category === 'csv' && <FileSpreadsheet className="size-4 text-green-500 shrink-0" />}
            {category === 'pdf' && <FileText className="size-4 text-red-500 shrink-0" />}
            <span className="font-medium text-sm truncate">{file.file_name}</span>
            <span className="text-xs text-muted-foreground shrink-0">{formatSize(file.file_size)}</span>
          </div>

          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              className="size-7"
              disabled={!prevFile}
              onClick={() => prevFile && onNavigate(prevFile)}
              title="上一个"
            >
              <ChevronLeft className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="size-7"
              disabled={!nextFile}
              onClick={() => nextFile && onNavigate(nextFile)}
              title="下一个"
            >
              <ChevronRight className="size-4" />
            </Button>
            <Button variant="ghost" size="sm" className="h-7 px-2 text-xs" onClick={handleDownload}>
              <Download className="size-3.5 mr-1" />
              下载
            </Button>
            <Button variant="ghost" size="icon" className="size-7" onClick={onClose}>
              <X className="size-4" />
            </Button>
          </div>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-hidden">
          {loading && (
            <div className="flex items-center justify-center h-full">
              <Loader2 className="size-6 animate-spin text-muted-foreground" />
            </div>
          )}

          {error && (
            <div className="flex flex-col items-center justify-center h-full gap-3 text-muted-foreground">
              <p className="text-sm">{error}</p>
              <Button variant="outline" size="sm" onClick={handleDownload}>
                <Download className="size-4 mr-1.5" />
                下载文件
              </Button>
            </div>
          )}

          {!loading && !error && category === 'image' && objectUrl && (
            <ImageView url={objectUrl} fileName={file.file_name} />
          )}

          {!loading && !error && category === 'pdf' && objectUrl && (
            <PdfView url={objectUrl} fileName={file.file_name} />
          )}

          {!loading && !error && (category === 'text' || category === 'code') && content !== null && (
            <CodeView text={content} lang={lang} fileName={file.file_name} />
          )}

          {!loading && !error && category === 'csv' && content !== null && (
            <CsvView text={content} fileName={file.file_name} />
          )}

          {!loading && !error && category === 'other' && (
            <OtherView file={file} onDownload={handleDownload} />
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-4 py-1.5 border-t text-xs text-muted-foreground shrink-0">
          <span>{file.file_name} · {file.mime_type || 'Unknown'} · {file.source === 'agent_artifact' ? '智能体产物' : '用户上传'}</span>
          <span>{fileIndex + 1} / {files.length}</span>
        </div>
      </div>
    </div>
  )
}
