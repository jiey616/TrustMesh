import type { KnowledgeSearchResult } from '@/types'
import { FileText } from 'lucide-react'

interface Props {
  items: KnowledgeSearchResult[]
}

export function KnowledgeResultCard({ items }: Props) {
  if (items.length === 0) return null

  return (
    <div className="rounded-xl border bg-card p-3 space-y-2 text-sm">
      <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        <FileText className="size-3.5" />
        知识库结果 ({items.length})
      </div>
      <div className="space-y-2">
        {items.slice(0, 3).map((item, i) => (
          <div key={i} className="rounded-lg bg-muted/50 p-2.5 space-y-1">
            <div className="flex items-center justify-between">
              <span className="font-medium text-xs">{item.document_title}</span>
              {item.score < 1 && (
                <span className="text-[10px] text-muted-foreground">
                  {(item.score * 100).toFixed(0)}%
                </span>
              )}
            </div>
            <p className="text-xs text-muted-foreground line-clamp-3">
              {item.content}
            </p>
          </div>
        ))}
        {items.length > 3 && (
          <p className="text-xs text-muted-foreground text-center">
            还有 {items.length - 3} 条结果
          </p>
        )}
      </div>
    </div>
  )
}
