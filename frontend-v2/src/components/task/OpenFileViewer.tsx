import { useMemo } from 'react'
import {
  imagePlugin,
  textPlugin,
  pdfPlugin,
  officePlugin,
  archivePlugin,
  audioPlugin,
  videoPlugin,
  fallbackPlugin,
} from '@open-file-viewer/core'
import type { PreviewPlugin } from '@open-file-viewer/core'
import { FileViewer as OFVFileViewer } from '@open-file-viewer/react'
import '@open-file-viewer/core/style.css'
import pdfWorkerSrc from 'pdfjs-dist/build/pdf.worker.mjs?url'
import { useTheme } from '@/theme/ThemeProvider'

interface OpenFileViewerProps {
  blob: Blob
  fileName: string
}

/**
 * Open File Viewer 懒加载预览组件。
 * 本文件（含 @open-file-viewer/core/react、pdfjs-dist）由外层 React.lazy 动态引入，
 * 只在需要预览 PDF / Office / 音视频 / 压缩包等格式时才加载对应 chunk。
 */
export function OpenFileViewer({ blob, fileName }: OpenFileViewerProps) {
  const { theme } = useTheme()
  const plugins = useMemo<PreviewPlugin[]>(
    () => [
      imagePlugin(),
      textPlugin(),
      pdfPlugin({ workerSrc: pdfWorkerSrc }),
      officePlugin(),
      archivePlugin(),
      audioPlugin(),
      videoPlugin(),
      fallbackPlugin(),
    ],
    [],
  )

  return (
    <OFVFileViewer
      file={blob}
      fileName={fileName}
      mimeType={blob.type || undefined}
      width="100%"
      height="62vh"
      fit="contain"
      toolbar
      theme={theme === 'dark' ? 'dark' : 'light'}
      plugins={plugins}
    />
  )
}
