import { useState, useMemo } from 'react'
import { Button } from 'antd'
import { LeftOutlined, RightOutlined, SendOutlined } from '@ant-design/icons'
import type { UIBlock, UIBlockResponse, UIResponse } from '@/types'

export interface UIResponseDraft {
  step: number
  responses: Record<string, UIBlockResponse>
}

interface UIResponsePanelProps {
  blocks: UIBlock[]
  onSubmit: (content: string, uiResponse: UIResponse) => void
  disabled?: boolean
  /**
   * 传入后由外部托管步骤与回答（受控），否则退回组件内部 state。
   * 待确认抽屉靠它把填写进度存进全局草稿——抽屉关掉组件就卸载了，
   * 不留外部托管的话用户收起再打开，填了一半的内容会全部蒸发。
   */
  draft?: UIResponseDraft
  onDraftChange?: (draft: UIResponseDraft) => void
}

/** 逐步交互面板：逐个呈现 ui_blocks，用户逐步回答，最后确认提交。 */
export function UIResponsePanel({ blocks, onSubmit, disabled, draft, onDraftChange }: UIResponsePanelProps) {
  const [innerStep, setInnerStep] = useState(0)
  const [innerResponses, setInnerResponses] = useState<Record<string, UIBlockResponse>>({})
  const currentStep = draft?.step ?? innerStep
  const responses = draft?.responses ?? innerResponses

  const goStep = (step: number) => {
    if (onDraftChange) onDraftChange({ step, responses })
    else setInnerStep(step)
  }

  const interactiveBlocks = useMemo(() => blocks.filter((b) => b.type !== 'info'), [blocks])
  const totalSteps = interactiveBlocks.length
  const isReviewStep = currentStep >= totalSteps
  const currentBlock = interactiveBlocks[currentStep]

  const updateResponse = (blockId: string, response: UIBlockResponse) => {
    const next = { ...responses, [blockId]: response }
    if (onDraftChange) onDraftChange({ step: currentStep, responses: next })
    else setInnerResponses(next)
  }

  const canProceed = (): boolean => {
    if (isReviewStep) return true
    if (!currentBlock) return false
    const resp = responses[currentBlock.id]
    switch (currentBlock.type) {
      case 'single_select':
        return (resp?.selected?.length ?? 0) > 0
      case 'text_input':
        return currentBlock.required !== true || (resp?.text?.trim().length ?? 0) > 0
      case 'confirm':
        return resp?.confirmed != null
      default:
        return true
    }
  }

  const handleNext = () => {
    if (canProceed() && currentStep < totalSteps) goStep(currentStep + 1)
  }

  const handleSubmit = () => {
    const content = generateSummary(blocks, responses)
    onSubmit(content, { blocks: responses })
  }

  const steps = interactiveBlocks.map((_, i) => i)
  const allSteps = [...steps, steps.length] // +1 确认步骤

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 12,
        borderRadius: 'var(--radius-structure)',
        border: '1px solid rgba(109,95,245,0.2)',
        background: 'rgba(109,95,245,0.04)',
        padding: 14,
      }}
    >
      {/* 步骤指示器 */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        {allSteps.map((i) => {
          const active = i === currentStep
          const done = i < currentStep
          const isConfirmDot = i === steps.length
          return (
            <span
              key={i}
              onClick={() => i < currentStep && goStep(i)}
              style={{
                height: 6,
                width: active ? 22 : 16,
                borderRadius: 'var(--radius-pill)',
                background: active ? 'var(--signal)' : done || isConfirmDot ? 'rgba(109,95,245,0.5)' : 'var(--surface-raised)',
                transition: 'all 0.2s',
                cursor: i < currentStep ? 'pointer' : 'default',
              }}
            />
          )
        })}
      </div>

      {isReviewStep ? (
        <ReviewStep blocks={blocks} responses={responses} onEdit={goStep} interactiveBlocks={interactiveBlocks} />
      ) : currentBlock ? (
        <StepContent
          block={currentBlock}
          response={responses[currentBlock.id]}
          onUpdate={(resp) => updateResponse(currentBlock.id, resp)}
          stepIndex={currentStep}
          totalSteps={totalSteps}
        />
      ) : null}

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Button
          size="small"
          type="text"
          icon={<LeftOutlined />}
          disabled={currentStep === 0}
          onClick={() => currentStep > 0 && goStep(currentStep - 1)}
        >
          上一步
        </Button>
        {isReviewStep ? (
          <Button
            size="small"
            type="primary"
            icon={<SendOutlined />}
            disabled={disabled}
            onClick={handleSubmit}
          >
            提交
          </Button>
        ) : (
          <Button size="small" type="primary" disabled={!canProceed()} onClick={handleNext}>
            下一步
            <RightOutlined />
          </Button>
        )}
      </div>
    </div>
  )
}

function StepContent({
  block,
  response,
  onUpdate,
  stepIndex,
  totalSteps,
}: {
  block: UIBlock
  response?: UIBlockResponse
  onUpdate: (resp: UIBlockResponse) => void
  stepIndex: number
  totalSteps: number
}) {
  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10 }}>
        <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-primary)' }}>{block.label}</span>
        <span style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>{stepIndex + 1} / {totalSteps}</span>
      </div>
      {block.type === 'single_select' && <SelectBlockInteractive block={block} response={response} onUpdate={onUpdate} />}
      {block.type === 'text_input' && <TextInputBlockInteractive block={block} response={response} onUpdate={onUpdate} />}
      {block.type === 'confirm' && <ConfirmBlockInteractive block={block} response={response} onUpdate={onUpdate} />}
    </div>
  )
}

function SelectBlockInteractive({
  block,
  response,
  onUpdate,
}: {
  block: UIBlock
  response?: UIBlockResponse
  onUpdate: (resp: UIBlockResponse) => void
}) {
  const selected = response?.selected ?? block.default ?? []
  const toggle = (value: string) => {
    if (block.multiple) {
      onUpdate({
        selected: selected.includes(value) ? selected.filter((v) => v !== value) : [...selected, value],
      })
    } else {
      onUpdate({ selected: [value] })
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      {block.multiple && <span style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>可多选</span>}
      {block.options?.map((opt) => {
        const isSelected = selected.includes(opt.value)
        return (
          <button
            key={opt.value}
            type="button"
            onClick={() => toggle(opt.value)}
            style={{
              display: 'flex',
              alignItems: 'flex-start',
              gap: 10,
              borderRadius: 'var(--radius-control)',
              border: `1px solid ${isSelected ? 'rgba(109,95,245,0.6)' : 'var(--line-strong)'}`,
              background: isSelected ? 'rgba(109,95,245,0.1)' : 'var(--surface-sunken)',
              padding: '8px 12px',
              textAlign: 'left',
              cursor: 'pointer',
              transition: 'all 0.15s',
              fontFamily: 'inherit',
            }}
          >
            <span
              style={{
                flexShrink: 0,
                width: 16,
                height: 16,
                marginTop: 2,
                borderRadius: 'var(--radius-avatar)',
                border: `2px solid ${isSelected ? 'var(--signal)' : 'var(--line-strong)'}`,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                background: isSelected ? 'var(--signal)' : 'transparent',
              }}
            >
              {isSelected && <span style={{ color: 'var(--text-primary)', fontSize: 10, lineHeight: 1 }}>✓</span>}
            </span>
            <span style={{ flex: 1, minWidth: 0 }}>
              <span style={{ display: 'block', fontSize: 13, color: isSelected ? '#fff' : 'var(--text-primary)', fontWeight: isSelected ? 500 : 400 }}>
                {opt.label}
              </span>
              {opt.description && (
                <span style={{ display: 'block', fontSize: 12, color: 'var(--text-tertiary)', marginTop: 2 }}>
                  {opt.description}
                </span>
              )}
            </span>
          </button>
        )
      })}
    </div>
  )
}

function TextInputBlockInteractive({
  block,
  response,
  onUpdate,
}: {
  block: UIBlock
  response?: UIBlockResponse
  onUpdate: (resp: UIBlockResponse) => void
}) {
  return (
    <div>
      {block.required === false && (
        <span style={{ fontSize: 11, color: 'var(--text-quaternary)', marginBottom: 6, display: 'block' }}>选填</span>
      )}
      <textarea
        value={response?.text ?? ''}
        onChange={(e) => onUpdate({ text: e.target.value })}
        placeholder={block.placeholder ?? '请输入...'}
        rows={3}
        style={{
          width: '100%',
          background: 'var(--surface-inset)',
          border: '1px solid var(--line-strong)',
          borderRadius: 'var(--radius-control)',
          padding: '8px 12px',
          color: 'var(--text-primary)',
          fontSize: 13,
          lineHeight: 1.6,
          resize: 'none',
          outline: 'none',
          fontFamily: 'inherit',
        }}
      />
    </div>
  )
}

function ConfirmBlockInteractive({
  block,
  response,
  onUpdate,
}: {
  block: UIBlock
  response?: UIBlockResponse
  onUpdate: (resp: UIBlockResponse) => void
}) {
  const confirmed = response?.confirmed
  return (
    <div style={{ display: 'flex', gap: 10 }}>
      <button
        type="button"
        onClick={() => onUpdate({ confirmed: true })}
        style={{
          flex: 1,
          borderRadius: 'var(--radius-control)',
          border: `2px solid ${confirmed === true ? 'var(--success)' : 'var(--line-strong)'}`,
          background: confirmed === true ? 'rgba(16,185,129,0.1)' : 'transparent',
          padding: '10px 12px',
          color: confirmed === true ? 'var(--success)' : 'var(--text-primary)',
          fontSize: 13,
          fontWeight: 500,
          cursor: 'pointer',
          fontFamily: 'inherit',
        }}
      >
        {block.confirm_label ?? '确认'}
      </button>
      <button
        type="button"
        onClick={() => onUpdate({ confirmed: false })}
        style={{
          flex: 1,
          borderRadius: 'var(--radius-control)',
          border: `2px solid ${confirmed === false ? 'var(--warning)' : 'var(--line-strong)'}`,
          background: confirmed === false ? 'rgba(245,158,11,0.1)' : 'transparent',
          padding: '10px 12px',
          color: confirmed === false ? 'var(--warning)' : 'var(--text-primary)',
          fontSize: 13,
          fontWeight: 500,
          cursor: 'pointer',
          fontFamily: 'inherit',
        }}
      >
        {block.cancel_label ?? '取消'}
      </button>
    </div>
  )
}

function ReviewStep({
  blocks,
  responses,
  onEdit,
  interactiveBlocks,
}: {
  blocks: UIBlock[]
  responses: Record<string, UIBlockResponse>
  onEdit: (step: number) => void
  interactiveBlocks: UIBlock[]
}) {
  return (
    <div>
      <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 10 }}>确认你的选择</div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        {blocks.map((block) => {
          const resp = responses[block.id]
          const editIndex = interactiveBlocks.findIndex((b) => b.id === block.id)
          return (
            <div key={block.id} style={{ display: 'flex', alignItems: 'center', gap: 8, background: 'var(--surface)', borderRadius: 'var(--radius-control)', padding: '6px 10px' }}>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>{block.label}</div>
                <div style={{ fontSize: 12, color: 'var(--text-primary)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {formatBlockResponse(block, resp)}
                </div>
              </div>
              {editIndex >= 0 && (
                <button
                  type="button"
                  onClick={() => onEdit(editIndex)}
                  style={{ fontSize: 11, color: 'var(--signal)', background: 'none', border: 'none', cursor: 'pointer', flexShrink: 0 }}
                >
                  修改
                </button>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function formatBlockResponse(block: UIBlock, response?: UIBlockResponse): string {
  if (!response) return '未填写'
  switch (block.type) {
    case 'single_select': {
      const labels = (response.selected ?? [])
        .map((v) => block.options?.find((o) => o.value === v)?.label ?? v)
      return labels.length > 0 ? labels.join('、') : '未选择'
    }
    case 'text_input':
      return response.text?.trim() || '未填写'
    case 'confirm':
      if (response.confirmed === true) return block.confirm_label ?? '已确认'
      if (response.confirmed === false) return block.cancel_label ?? '已取消'
      return '未确认'
    default:
      return ''
  }
}

function generateSummary(blocks: UIBlock[], responses: Record<string, UIBlockResponse>): string {
  const parts: string[] = []
  for (const block of blocks) {
    if (block.type === 'info') continue
    const text = formatBlockResponse(block, responses[block.id])
    if (text && text !== '未填写' && text !== '未选择') {
      parts.push(`${block.label}：${text}`)
    }
  }
  return parts.join('；')
}
