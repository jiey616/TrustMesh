import { Plus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { StepInput, StepOutput, WorkflowStep } from '@/types'

const PREV = 'prev' // 直接对应后端 StepIOLink.Step = "prev"（上一步）

interface StepIOEditorProps {
  step: WorkflowStep
  steps: WorkflowStep[]
  stepIndex: number
  onChange: (patch: { inputs?: StepInput[]; outputs?: StepOutput[] }) => void
}

/** 单个步骤的「输入 / 输出」定义编辑器。输入可关联上游步骤的输出文件。 */
export function StepIOEditor({ step, steps, stepIndex, onChange }: StepIOEditorProps) {
  const outputs = step.outputs ?? []
  const inputs = step.inputs ?? []

  // 上游步骤：仅允许引用当前步骤之前的步骤（含「上一步」）。
  const upstreamSteps = steps.slice(0, stepIndex)

  const setOutputs = (next: StepOutput[]) => onChange({ outputs: next })
  const setInputs = (next: StepInput[]) => onChange({ inputs: next })

  const updateOutput = (i: number, patch: Partial<StepOutput>) =>
    setOutputs(outputs.map((o, k) => (k === i ? { ...o, ...patch } : o)))
  const addOutput = () => setOutputs([...outputs, { name: '', description: '', mime_type: '' }])
  const removeOutput = (i: number) => setOutputs(outputs.filter((_, k) => k !== i))

  const updateInput = (i: number, patch: Partial<StepInput>) =>
    setInputs(inputs.map((o, k) => (k === i ? { ...o, ...patch } : o)))
  const addInput = () =>
    setInputs([...inputs, { name: '', description: '', mime_type: '', source: { step: PREV, output: '' } }])
  const removeInput = (i: number) => setInputs(inputs.filter((_, k) => k !== i))

  // 根据来源步骤解析可选的输出名列表。
  const outputsForSource = (srcStep: string): StepOutput[] => {
    if (srcStep === PREV) {
      const prev = upstreamSteps[upstreamSteps.length - 1]
      return prev?.outputs ?? []
    }
    const src = upstreamSteps.find((s) => s.name === srcStep)
    return src?.outputs ?? []
  }

  return (
    <div className="mt-2 flex flex-col gap-2 rounded-md border border-dashed bg-muted/30 p-2">
      {/* ===== 输出 ===== */}
      <div className="flex flex-col gap-1.5">
        <div className="flex items-center justify-between">
          <span className="text-[11px] font-medium text-muted-foreground">输出文件</span>
          <Button variant="ghost" size="sm" className="h-6 text-[11px]" onClick={addOutput}>
            <Plus className="size-3 mr-0.5" />
            添加输出
          </Button>
        </div>
        {outputs.length === 0 && (
          <p className="text-[11px] text-muted-foreground/70">该步骤产出的文件（供后续步骤作为输入引用）。</p>
        )}
        {outputs.map((o, i) => (
          <div key={i} className="flex flex-wrap items-center gap-1.5">
            <Input
              value={o.name}
              onChange={(e) => updateOutput(i, { name: e.target.value })}
              placeholder="输出名（如：剧本正文）"
              className="h-7 w-32 text-xs"
            />
            <Input
              value={o.mime_type ?? ''}
              onChange={(e) => updateOutput(i, { mime_type: e.target.value })}
              placeholder="类型 text/markdown"
              className="h-7 w-32 text-xs"
            />
            <Input
              value={o.description ?? ''}
              onChange={(e) => updateOutput(i, { description: e.target.value })}
              placeholder="说明（可选）"
              className="h-7 min-w-0 flex-1 text-xs"
            />
            <Button
              variant="ghost"
              size="icon"
              className="size-6 shrink-0 text-muted-foreground hover:text-destructive"
              onClick={() => removeOutput(i)}
            >
              <Trash2 className="size-3" />
            </Button>
          </div>
        ))}
      </div>

      {/* ===== 输入 ===== */}
      <div className="flex flex-col gap-1.5">
        <div className="flex items-center justify-between">
          <span className="text-[11px] font-medium text-muted-foreground">输入文件（关联上游输出）</span>
          <Button
            variant="ghost"
            size="sm"
            className="h-6 text-[11px]"
            onClick={addInput}
            disabled={upstreamSteps.length === 0}
            title={upstreamSteps.length === 0 ? '前面没有步骤可引用' : undefined}
          >
            <Plus className="size-3 mr-0.5" />
            添加输入
          </Button>
        </div>
        {upstreamSteps.length === 0 && (
          <p className="text-[11px] text-muted-foreground/70">第一个步骤没有上游，无法引用输入。</p>
        )}
        {inputs.map((inp, i) => {
          const avail = outputsForSource(inp.source.step)
          return (
            <div key={i} className="flex flex-wrap items-center gap-1.5">
              <Input
                value={inp.name}
                onChange={(e) => updateInput(i, { name: e.target.value })}
                placeholder="输入名（如：上游剧本）"
                className="h-7 w-32 text-xs"
              />
              <Select
                value={inp.source.step || PREV}
                onValueChange={(v: string | null) => {
                  // 来源切换后，原 output 可能失效，重置为空。
                  updateInput(i, { source: { step: v ?? PREV, output: '' } })
                }}
              >
                <SelectTrigger className="h-7 w-28 text-xs">
                  <SelectValue placeholder="来源步骤" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={PREV}>上一步</SelectItem>
                  {upstreamSteps
                    .filter((s) => s.name.trim() !== '')
                    .map((s) => (
                      <SelectItem key={s.name} value={s.name}>
                        {s.name}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
              <Select
                value={inp.source.output}
                onValueChange={(v: string | null) =>
                  updateInput(i, { source: { ...inp.source, output: v ?? '' } })
                }
              >
                <SelectTrigger className="h-7 w-32 text-xs">
                  <SelectValue placeholder={avail.length ? '选择输出' : '无可用输出'} />
                </SelectTrigger>
                <SelectContent>
                  {avail.map((o) => (
                    <SelectItem key={o.name} value={o.name}>
                      {o.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                variant="ghost"
                size="icon"
                className="size-6 shrink-0 text-muted-foreground hover:text-destructive"
                onClick={() => removeInput(i)}
              >
                <Trash2 className="size-3" />
              </Button>
            </div>
          )
        })}
      </div>
    </div>
  )
}
