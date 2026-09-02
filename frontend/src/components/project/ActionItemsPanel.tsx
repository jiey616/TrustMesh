import { useCallback, useEffect, useState } from 'react'
import { CheckCircle2, ListChecks, Loader2, RefreshCw, UserRound } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from 'sonner'
import { listActionItems, convertActionItems } from '@/api/actionItems'
import { useAgents } from '@/hooks/useAgents'
import type { ActionItemRefDTO } from '@/types'

interface ActionItemsPanelProps {
  projectId: string
}

interface ItemRow extends ActionItemRefDTO {
  key: string
  assigneeAgentId?: string // resolved agent id chosen by the user
}

export function ActionItemsPanel({ projectId }: ActionItemsPanelProps) {
  const [items, setItems] = useState<ItemRow[]>([])
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [converting, setConverting] = useState(false)
  const { data: agents } = useAgents()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listActionItems(projectId)
      const list: ItemRow[] = (res.data?.items ?? []).map((it: ActionItemRefDTO) => ({
        ...it,
        key: `${it.task_id}:${it.todo_id}:${it.item_index}`,
      }))
      setItems(list)
      setSelected(new Set())
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '加载待办失败')
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    load()
  }, [load])

  const toggle = (key: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  const toggleAll = () => {
    setSelected((prev) => (prev.size === items.length ? new Set() : new Set(items.map((i) => i.key))))
  }

  const setAssigneeFor = (key: string, agentId: string) => {
    setItems((prev) => prev.map((i) => (i.key === key ? { ...i, assigneeAgentId: agentId } : i)))
  }

  const handleConvert = async () => {
    const chosen = items.filter((i) => selected.has(i.key))
    if (chosen.length === 0) return
    setConverting(true)
    try {
      // Per-item assignee override: key → agent id (backend resolves to node).
      const assignees: Record<string, string> = {}
      for (const it of chosen) {
        if (it.assigneeAgentId) {
          assignees[it.key] = it.assigneeAgentId
        }
      }
      const res = await convertActionItems(projectId, chosen.map((i) => i.key), assignees)
      toast.success(`已创建 ${res.data?.count ?? 0} 个任务`)
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '转换失败')
    } finally {
      setConverting(false)
    }
  }

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="size-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b px-6 py-3">
        <div className="flex items-center gap-2 text-sm font-medium">
          <ListChecks className="size-4" />
          待确认待办
          <Badge variant="secondary" className="ml-1">{items.length}</Badge>
        </div>
        <div className="flex items-center gap-2">
          {items.length > 0 && (
            <Button variant="ghost" size="sm" onClick={toggleAll} className="text-xs">
              <CheckCircle2 className="size-3.5 mr-1" />
              {selected.size === items.length ? '取消全选' : '全选'}
            </Button>
          )}
          <Button variant="ghost" size="sm" onClick={load} className="text-xs">
            <RefreshCw className="size-3.5 mr-1" />
            刷新
          </Button>
          <Button size="sm" disabled={selected.size === 0 || converting} onClick={handleConvert}>
            {converting ? '创建中…' : `确认创建（${selected.size}）`}
          </Button>
        </div>
      </div>

      {items.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 text-muted-foreground">
          <ListChecks className="size-10" />
          <p className="text-sm">暂无待确认待办</p>
          <p className="text-xs">智能体完成任务并声明后续待办后，会出现在这里</p>
        </div>
      ) : (
        <div className="flex-1 overflow-auto px-6 py-4">
          <div className="space-y-2">
            {items.map((it) => (
              <div
                key={it.key}
                className={`flex items-start gap-3 rounded-lg border p-3 transition-colors ${
                  selected.has(it.key) ? 'border-primary/50 bg-primary/5' : 'bg-card'
                }`}
              >
                <Checkbox
                  checked={selected.has(it.key)}
                  onCheckedChange={() => toggle(it.key)}
                />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">{it.title}</span>
                    <Badge variant="outline" className="shrink-0 text-[11px]">
                      {it.assignee_role || '待指派'}
                    </Badge>
                  </div>
                  {it.description && (
                    <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">
                      {it.description}
                    </p>
                  )}
                  <p className="mt-1 text-[11px] text-muted-foreground">
                    来源任务：{it.task_title} · {it.todo_title}
                  </p>
                </div>
                <div className="w-44 shrink-0">
                  <Select
                    value={it.assigneeAgentId ?? ''}
                    onValueChange={(v: string | null) => v && setAssigneeFor(it.key, v)}
                  >
                    <SelectTrigger className="h-8 text-xs">
                      <SelectValue placeholder="选择执行者" />
                    </SelectTrigger>
                    <SelectContent>
                      {(agents ?? []).map((a) => (
                        <SelectItem key={a.id} value={a.id}>
                          <span className="flex items-center gap-1.5">
                            <UserRound className="size-3" />
                            {a.name}
                          </span>
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
