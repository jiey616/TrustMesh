import { useState } from 'react'
import { toast } from 'sonner'
import { Pencil, Plus, Trash2, Workflow as WorkflowIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { WorkflowPipelineEditor } from '@/components/project/WorkflowPipelineEditor'
import { useUpdateProject } from '@/hooks/useProjects'
import { useAgents } from '@/hooks/useAgents'
import { ApiRequestError } from '@/api/client'
import type { Project, Workflow } from '@/types'

interface WorkflowListPanelProps {
  project: Project | undefined
}

function cloneWorkflows(list: Workflow[] | undefined): Workflow[] {
  return (list ?? []).map((w) => ({ name: w.name, steps: w.steps.map((s) => ({ ...s })) }))
}

interface DraftState {
  mode: 'create' | 'edit'
  index: number
  wf: Workflow
}

export function WorkflowListPanel({ project }: WorkflowListPanelProps) {
  const updateProject = useUpdateProject()
  const { data: agents } = useAgents()
  const [draft, setDraft] = useState<DraftState | null>(null)
  const [deleting, setDeleting] = useState<number | null>(null)
  const [pending, setPending] = useState(false)

  const workflows = project?.workflows ?? []

  const saveAll = async (next: Workflow[]) => {
    if (!project) return
    setPending(true)
    try {
      await updateProject.mutateAsync({ id: project.id, input: { workflows: next } })
      toast.success('工作流已保存')
      setDraft(null)
    } catch (err) {
      toast.error(err instanceof ApiRequestError ? err.message : '保存失败')
    } finally {
      setPending(false)
    }
  }

  const validate = (wf: Workflow): string | null => {
    if (!wf.name.trim()) return '请填写工作流名称'
    if (wf.steps.length === 0) return '请至少添加一个步骤'
    if (wf.steps.some((s) => !s.name.trim() || (!s.role?.trim() && !s.agent_id))) return '每个步骤都需要填写名称并绑定智能体'
    return null
  }

  const handleSave = async () => {
    if (!draft) return
    const errMsg = validate(draft.wf)
    if (errMsg) {
      toast.error(errMsg)
      return
    }
    if (draft.mode === 'create') {
      await saveAll([...workflows, draft.wf])
    } else {
      const next = workflows.map((w, i) => (i === draft.index ? draft.wf : w))
      await saveAll(next)
    }
  }

  const handleDelete = async () => {
    if (deleting === null) return
    const next = workflows.filter((_, i) => i !== deleting)
    await saveAll(next)
    setDeleting(null)
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          预定义工作流，新建任务时可选择（不选则 PM 自由规划）
        </p>
        <Button size="sm" onClick={() => setDraft({ mode: 'create', index: -1, wf: { name: '', steps: [] } })} disabled={!project}>
          <Plus className="size-3.5 mr-1" />
          新建工作流
        </Button>
      </div>

      {workflows.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed py-10 text-center">
          <WorkflowIcon className="size-8 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground">还没有工作流</p>
          <p className="text-xs text-muted-foreground/70">创建一条流水线，让 PM 按固定步骤规划任务</p>
        </div>
      ) : (
        <div className="grid gap-3 md:grid-cols-2">
          {workflows.map((wf, idx) => (
            <div key={idx} className="rounded-xl border bg-background p-4 flex flex-col gap-2">
              <div className="flex items-center justify-between gap-2">
                <p className="font-medium truncate">{wf.name || '未命名工作流'}</p>
                <div className="flex shrink-0 gap-1">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-7"
                    onClick={() => setDraft({ mode: 'edit', index: idx, wf: cloneWorkflows([wf])[0] })}
                  >
                    <Pencil className="size-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-7 text-muted-foreground hover:text-destructive"
                    onClick={() => setDeleting(idx)}
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-1.5 text-xs">
                {wf.steps.map((s, si) => {
                  const boundAgent = s.agent_id ? (agents ?? []).find((a) => a.id === s.agent_id) : undefined
                  return (
                    <span key={si} className="flex items-center gap-1.5">
                      <span className="rounded-md bg-muted px-1.5 py-0.5">
                        {s.name || '未命名'}
                        {boundAgent ? ` @${boundAgent.name}` : s.role ? ` (${s.role})` : ''}
                        {s.need_review ? ' ⏳' : ''}
                      </span>
                      {si < wf.steps.length - 1 && <span className="text-muted-foreground/50">→</span>}
                    </span>
                  )
                })}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* 新建 / 编辑 dialog */}
      <Dialog
        open={!!draft}
        onOpenChange={(open) => {
          if (!open && !pending) setDraft(null)
        }}
      >
        <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{draft?.mode === 'edit' ? '编辑工作流' : '新建工作流'}</DialogTitle>
          </DialogHeader>
          {draft && (
            <WorkflowPipelineEditor
              value={draft.wf}
              onChange={(wf) => setDraft({ ...draft, wf })}
              agents={agents ?? []}
            />
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDraft(null)} disabled={pending}>
              取消
            </Button>
            <Button onClick={handleSave} disabled={pending}>
              {pending ? '保存中…' : '保存工作流'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>删除工作流？</DialogTitle>
            <p className="text-sm text-muted-foreground">
              已创建的任务不受影响（任务保存的是工作流快照）。此操作不可撤销。
            </p>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleting(null)}>取消</Button>
            <Button variant="destructive" onClick={handleDelete}>删除</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
