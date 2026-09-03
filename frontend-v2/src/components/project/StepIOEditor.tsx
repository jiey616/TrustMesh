import { Button, Input, Select, Tooltip } from 'antd'
import { PlusOutlined, DeleteOutlined, FileTextOutlined } from '@ant-design/icons'
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

  const inputStyle = {
    background: 'var(--surface)',
    borderColor: 'var(--line-strong)',
  } as const

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 10,
        borderRadius: 'var(--radius-control)',
        border: '1px dashed var(--line-strong)',
        background: 'var(--surface-sunken)',
        padding: '8px 10px',
      }}
    >
      {/* ===== 输出 ===== */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-tertiary)' }}>
            <FileTextOutlined style={{ marginRight: 4 }} />
            输出文件
          </span>
          <Tooltip title="声明本步骤会产出的文件，供后续步骤作为输入引用">
            <Button type="text" size="small" icon={<PlusOutlined />} onClick={addOutput} style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>
              添加输出
            </Button>
          </Tooltip>
        </div>
        {outputs.length === 0 && (
          <p style={{ margin: 0, fontSize: 11, color: 'var(--text-quaternary)' }}>
            该步骤产出的文件（供后续步骤作为输入引用）。
          </p>
        )}
        {outputs.map((o, i) => (
          <div key={i} style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 6 }}>
            <Input
              size="small"
              value={o.name}
              onChange={(e) => updateOutput(i, { name: e.target.value })}
              placeholder="输出名（如：剧本正文）"
              style={{ ...inputStyle, width: 132, fontSize: 12 }}
            />
            <Input
              size="small"
              value={o.mime_type ?? ''}
              onChange={(e) => updateOutput(i, { mime_type: e.target.value })}
              placeholder="类型 text/markdown"
              style={{ ...inputStyle, width: 132, fontSize: 12 }}
            />
            <Input
              size="small"
              value={o.description ?? ''}
              onChange={(e) => updateOutput(i, { description: e.target.value })}
              placeholder="说明（可选）"
              style={{ ...inputStyle, flex: 1, minWidth: 80, fontSize: 12 }}
            />
            <Tooltip title="删除输出">
              <Button
                type="text"
                size="small"
                icon={<DeleteOutlined />}
                style={{ color: 'var(--text-quaternary)' }}
                onClick={() => removeOutput(i)}
              />
            </Tooltip>
          </div>
        ))}
      </div>

      {/* ===== 输入 ===== */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-tertiary)' }}>
            输入文件（关联上游输出）
          </span>
          <Tooltip
            title={upstreamSteps.length === 0 ? '前面没有步骤可引用' : '引用之前步骤的输出文件'}
          >
            <Button
              type="text"
              size="small"
              icon={<PlusOutlined />}
              onClick={addInput}
              disabled={upstreamSteps.length === 0}
              style={{ fontSize: 11, color: 'var(--text-tertiary)' }}
            >
              添加输入
            </Button>
          </Tooltip>
        </div>
        {upstreamSteps.length === 0 && (
          <p style={{ margin: 0, fontSize: 11, color: 'var(--text-quaternary)' }}>
            第一个步骤没有上游，无法引用输入。
          </p>
        )}
        {inputs.map((inp, i) => {
          const avail = outputsForSource(inp.source.step)
          return (
            <div key={i} style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 6 }}>
              <Input
                size="small"
                value={inp.name}
                onChange={(e) => updateInput(i, { name: e.target.value })}
                placeholder="输入名（如：上游剧本）"
                style={{ ...inputStyle, width: 128, fontSize: 12 }}
              />
              <Select
                size="small"
                style={{ width: 108 }}
                value={inp.source.step || PREV}
                onChange={(v: string) => {
                  // 来源切换后，原 output 可能失效，重置为空。
                  updateInput(i, { source: { step: v ?? PREV, output: '' } })
                }}
                options={[
                  { value: PREV, label: '上一步' },
                  ...upstreamSteps
                    .filter((s) => s.name.trim() !== '')
                    .map((s) => ({ value: s.name, label: s.name })),
                ]}
                placeholder="来源步骤"
              />
              <Select
                size="small"
                style={{ width: 118 }}
                value={inp.source.output}
                onChange={(v: string) => updateInput(i, { source: { ...inp.source, output: v ?? '' } })}
                placeholder={avail.length ? '选择输出' : '无可用输出'}
                options={avail.map((o) => ({ value: o.name, label: o.name }))}
              />
              <Tooltip title="删除输入">
                <Button
                  type="text"
                  size="small"
                  icon={<DeleteOutlined />}
                  style={{ color: 'var(--text-quaternary)' }}
                  onClick={() => removeInput(i)}
                />
              </Tooltip>
            </div>
          )
        })}
      </div>
    </div>
  )
}
