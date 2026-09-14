import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatRelativeTime(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffSec = Math.floor(diffMs / 1000)
  const diffMin = Math.floor(diffSec / 60)
  const diffHour = Math.floor(diffMin / 60)
  const diffDay = Math.floor(diffHour / 24)

  if (diffSec < 60) return '刚刚'
  if (diffMin < 60) return `${diffMin} 分钟前`
  if (diffHour < 24) return `${diffHour} 小时前`
  if (diffDay < 7) return `${diffDay} 天前`
  return date.toLocaleDateString('zh-CN')
}

export function truncateNodeId(str: string, front = 6, back = 4) {
  if (str.length <= front + back + 3) return str
  return str.slice(0, front) + '...' + str.slice(-back)
}

export function formatDateTime(dateStr: string): string {
  return new Date(dateStr).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

interface NormalizeEscapedTextOptions {
  preserveMarkdownCode?: boolean
  /** 单换行（非段落分隔）转 GFM 软换行（行尾两空格），渲染为 <br> */
  softBreak?: boolean
}

function decodeEscapedControlChars(value: string) {
  return value
    .replace(/\\r\\n/g, '\n')
    .replace(/\\n/g, '\n')
    .replace(/\\r/g, '\r')
    .replace(/\\t/g, '\t')
}

// 单换行（前后都不是换行）转 GFM 软换行（行尾两空格），渲染为 <br>；
// 连续的 \n\n 视为段落分隔，保持不变。
function toSoftBreaks(value: string): string {
  return value.replace(/(^|[^\n])\n(?=[^\n])/g, '$1  \n')
}

export function normalizeEscapedText(value: string | null | undefined, options: NormalizeEscapedTextOptions = {}) {
  if (!value) {
    return ''
  }

  const proc = (s: string): string => {
    let r = decodeEscapedControlChars(s)
    if (options.softBreak) r = toSoftBreaks(r)
    return r
  }

  if (!options.preserveMarkdownCode) {
    return proc(value)
  }

  const markdownCodePattern = /```[\s\S]*?```|`[^`\n]*`/g
  let result = ''
  let lastIndex = 0

  for (const match of value.matchAll(markdownCodePattern)) {
    const index = match.index ?? 0
    result += proc(value.slice(lastIndex, index))
    result += match[0]
    lastIndex = index + match[0].length
  }

  result += proc(value.slice(lastIndex))
  return result
}

export function stripMessagePrefix(content: string): string {
  return content.replace(/^(?:\s*\[[^[\]]+\]\s*)+/u, '').trimStart()
}

export function normalizeChatMessageContent(content: string): string {
  const stripped = stripMessagePrefix(content)

  if (!(stripped.startsWith('"') && stripped.endsWith('"'))) {
    return stripped
  }

  try {
    const parsed = JSON.parse(stripped)
    return typeof parsed === 'string' ? parsed : stripped
  } catch {
    return stripped
  }
}
