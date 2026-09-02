import { ArrowRight, ChevronRight, Plus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Agent, Workflow, WorkflowStep } from '@/types'
import { StepIOEditor } from './StepIOEditor'

interface WorkflowPipelineEditorProps {
  value: Workflow
  onChange: (wf: Workflow) => void
  agents: Agent[]
}

const STEP_PLACEHOLDERS = ['编剧产出剧本', '导演拆解分镜', '测试质量验收']

export function WorkflowPipelineEditor({ value, onChange, agents }: WorkflowPipelineEditorProps) {
  const steps = value.steps ?? []
  const set = (next: Workflow) => onChange(next)

  const updateStep = (idx: number, patch: Partial<WorkflowStep>) => {
    const next = steps.map((s, i) => (i === idx ? { ...s, ...patch } : s))
    set({ ...value, steps: next })
  }

  const addStep = (at?: number) => {
    const newStep: WorkflowStep = { name: '', role: '', need_review: false }
    const next = [...steps]
    next.splice(at ?? next.length, 0, newStep)
    set({ ...value, steps: next })
  }

  const removeStep = (idx: number) => {
    const next = steps.filter((_, i) => i !== idx)
    set({ ...value, steps: next })
  }

  return (
    <div className="flex flex-col gap-3">
      <Input
        value={value.name}
        onChange={(e) => set({ ...value, name: e.target.value })}
        placeholder="工作流名称（如：剧本制作流水线）"
        className="font-medium"
      />

      <div className="rounded-xl border bg-muted/20 p-4">
        {steps.length === 0 ? (
          <div className="flex flex-col items-center gap-3 py-8 text-center">
            <p className="text-sm text-muted-foreground">
              还没有步骤。点击下方按钮开始搭建流水线。
            </p>
            <Button size="sm" onClick={() => addStep()}>
              <Plus className="size-3.5 mr-1" />
              添加第一个步骤
            </Button>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
              <span className="rounded bg-background border px-2 py-0.5">开始</span>
              <ChevronRight className="size-3" />
            </div>

            {steps.map((step, idx) => (
              <div key={idx} className="flex items-stretch gap-1.5">
                <div className="min-w-0 flex-1 rounded-lg border bg-background px-3 py-2.5">
                  <div className="flex items-center gap-2">
                    <span className="shrink-0 rounded-md bg-primary/10 px-1.5 py-0.5 text-[11px] font-medium text-primary">
                      步骤 {idx + 1}
                    </span>
                    <Input
                      value={step.name}
                      onChange={(e) => updateStep(idx, { name: e.target.value })}
                      placeholder={STEP_PLACEHOLDERS[idx % STEP_PLACEHOLDERS.length]}
                      className="h-7 text-sm"
                    />
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-6 shrink-0 text-muted-foreground hover:text-destructive"
                      onClick={() => removeStep(idx)}
                    >
                      <X className="size-3.5" />
                    </Button>
                  </div>
                  <div className="mt-2 flex flex-wrap items-center gap-3">
                    <Select
                      value={step.agent_id ?? step.role ?? ''}
                      onValueChange={(v: string | null) => {
                        const agent = agents?.find((a) => a.id === v)
                        if (agent) {
                          updateStep(idx, { role: agent.role || agent.name, agent_id: agent.id })
                        } else {
                          updateStep(idx, { role: '', agent_id: '' })
                        }
                      }}
                    >
                      <SelectTrigger className="h-8 w-56 text-xs">
                        <SelectValue placeholder="绑定执行智能体" />
                      </SelectTrigger>
                      <SelectContent>
                        {(agents ?? [])
                          .filter((a) => !a.archived)
                          .map((a) => (
                            <SelectItem key={a.id} value={a.id}>
                              {a.name}（{a.role || '无角色'}）
                            </SelectItem>
                          ))}
                      </SelectContent>
                    </Select>
                    <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
                      <Checkbox
                        checked={!!step.need_review}
                        onCheckedChange={(chk) => updateStep(idx, { need_review: !!chk })}
                      />
                      需人工确认
                    </label>
                  </div>
                  <StepIOEditor
                    step={step}
                    steps={steps}
                    stepIndex={idx}
                    onChange={(patch) => updateStep(idx, patch)}
                  />
                </div>

                {/* connector arrow to next step */}
                <div className="flex items-center">
                  <ArrowRight className="size-4 shrink-0 text-muted-foreground/60" />
                </div>
              </div>
            ))}

            <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
              <ChevronRight className="size-3" />
              <span className="rounded bg-background border px-2 py-0.5">完成</span>
            </div>

            <Button variant="outline" size="sm" onClick={() => addStep()} className="self-start mt-1">
              <Plus className="size-3.5 mr-1" />
              在末尾添加步骤
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
