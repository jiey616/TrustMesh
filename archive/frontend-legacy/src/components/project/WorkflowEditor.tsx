import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
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

interface WorkflowEditorProps {
  value: Workflow | undefined
  onChange: (wf: Workflow | undefined) => void
  agents: Agent[]
}

const STEP_PLACEHOLDERS = ['编剧产出剧本', '导演拆解分镜', '测试质量验收']

export function WorkflowEditor({ value, onChange, agents }: WorkflowEditorProps) {
  const steps = value?.steps ?? []

  const set = (next: Workflow | undefined) => onChange(next)

  const updateStep = (idx: number, patch: Partial<WorkflowStep>) => {
    const next = steps.map((s, i) => (i === idx ? { ...s, ...patch } : s))
    set({ name: value?.name ?? '', steps: next })
  }

  const addStep = () => {
    const next = [
      ...steps,
      { name: '', role: '', need_review: false },
    ]
    set({ name: value?.name ?? '', steps: next })
  }

  const removeStep = (idx: number) => {
    const next = steps.filter((_, i) => i !== idx)
    if (next.length === 0) {
      set(undefined)
    } else {
      set({ name: value?.name ?? '', steps: next })
    }
  }

  const moveStep = (idx: number, dir: -1 | 1) => {
    const target = idx + dir
    if (target < 0 || target >= steps.length) return
    const next = [...steps]
    ;[next[idx], next[target]] = [next[target], next[idx]]
    set({ name: value?.name ?? '', steps: next })
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <label className="text-sm font-medium">工作流（可选）</label>
        {value && (
          <Button
            variant="ghost"
            size="sm"
            className="text-xs text-destructive"
            onClick={() => set(undefined)}
          >
            清除工作流
          </Button>
        )}
      </div>

      {!value ? (
        <Button variant="outline" size="sm" onClick={addStep} className="self-start">
          <Plus className="size-3.5 mr-1" />
          配置工作流
        </Button>
      ) : (
        <>
          <Input
            value={value.name}
            onChange={(e) => set({ name: e.target.value, steps })}
            placeholder="工作流名称（如：剧本制作流水线）"
          />
          <div className="flex flex-col gap-2">
            {steps.map((step, idx) => (
              <div key={idx} className="flex items-center gap-2 rounded-lg border bg-background px-3 py-2">
                <div className="flex flex-col">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-6"
                    disabled={idx === 0}
                    onClick={() => moveStep(idx, -1)}
                  >
                    <ArrowUp className="size-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-6"
                    disabled={idx === steps.length - 1}
                    onClick={() => moveStep(idx, 1)}
                  >
                    <ArrowDown className="size-3.5" />
                  </Button>
                </div>
                <span className="w-4 text-xs text-muted-foreground">{idx + 1}.</span>
                <div className="flex min-w-0 flex-1 flex-col gap-1.5">
                  <Input
                    value={step.name}
                    onChange={(e) => updateStep(idx, { name: e.target.value })}
                    placeholder={STEP_PLACEHOLDERS[idx % STEP_PLACEHOLDERS.length]}
                  />
                  <div className="flex items-center gap-2">
                    <Select
                      value={step.role}
                      onValueChange={(v: string | null) => updateStep(idx, { role: v ?? '' })}
                    >
                      <SelectTrigger className="h-8 w-40 text-xs">
                        <SelectValue placeholder="绑定角色" />
                      </SelectTrigger>
                      <SelectContent>
                        {(agents ?? [])
                          .filter((a) => !a.archived)
                          .map((a) => (
                            <SelectItem key={a.id} value={a.role || a.name}>
                              {a.name}（{a.role || '无角色'}）
                            </SelectItem>
                          ))}
                      </SelectContent>
                    </Select>
                    <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
                      <Checkbox
                        checked={!!step.need_review}
                        onCheckedChange={(c) => updateStep(idx, { need_review: !!c })}
                      />
                      需人工确认
                    </label>
                  </div>
                </div>
                <StepIOEditor
                  step={step}
                  steps={steps}
                  stepIndex={idx}
                  onChange={(patch) => updateStep(idx, patch)}
                />
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-7 shrink-0 text-muted-foreground hover:text-destructive"
                  onClick={() => removeStep(idx)}
                >
                  <Trash2 className="size-3.5" />
                </Button>
              </div>
            ))}
          </div>
          <Button variant="outline" size="sm" onClick={addStep} className="self-start">
            <Plus className="size-3.5 mr-1" />
            添加步骤
          </Button>
        </>
      )}
    </div>
  )
}
