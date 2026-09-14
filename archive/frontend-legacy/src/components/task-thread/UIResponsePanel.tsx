import { useState, useMemo } from 'react'
import { Check, ChevronLeft, ChevronRight, Send } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { UIBlock, UIBlockResponse, UIResponse } from '@/types'

interface UIResponsePanelProps {
  blocks: UIBlock[]
  onSubmit: (content: string, uiResponse: UIResponse) => void
  disabled?: boolean
}

/**
 * 逐步交互面板：逐个呈现 ui_blocks，用户逐步回答，最后确认提交。
 */
export function UIResponsePanel({ blocks, onSubmit, disabled }: UIResponsePanelProps) {
  const [currentStep, setCurrentStep] = useState(0)
  const [responses, setResponses] = useState<Record<string, UIBlockResponse>>({})

  // 过滤掉 info 类型（只展示，不需要回答）
  const interactiveBlocks = useMemo(() => blocks.filter((b) => b.type !== 'info'), [blocks])
  const totalSteps = interactiveBlocks.length
  const isReviewStep = currentStep >= totalSteps
  const currentBlock = interactiveBlocks[currentStep]

  const updateResponse = (blockId: string, response: UIBlockResponse) => {
    setResponses((prev) => ({ ...prev, [blockId]: response }))
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
    if (canProceed() && currentStep < totalSteps) {
      setCurrentStep(currentStep + 1)
    }
  }

  const handlePrev = () => {
    if (currentStep > 0) {
      setCurrentStep(currentStep - 1)
    }
  }

  const handleSubmit = () => {
    const content = generateSummary(blocks, responses)
    const uiResponse: UIResponse = { blocks: responses }
    onSubmit(content, uiResponse)
  }

  return (
    <div className="flex flex-col gap-3 rounded-2xl border bg-card p-4 shadow-sm">
      {/* 步骤指示器 */}
      <div className="flex items-center gap-1.5">
        {interactiveBlocks.map((_, i) => (
          <button
            key={i}
            onClick={() => i < currentStep && setCurrentStep(i)}
            className={cn(
              'h-1.5 rounded-full transition-all',
              i < currentStep
                ? 'bg-primary w-4 cursor-pointer'
                : i === currentStep
                  ? 'bg-primary w-6'
                  : 'bg-muted w-4',
              isReviewStep && 'bg-primary w-4 cursor-pointer'
            )}
          />
        ))}
        {/* 确认步骤点 */}
        <div className={cn(
          'h-1.5 rounded-full transition-all',
          isReviewStep ? 'bg-primary w-6' : 'bg-muted w-4'
        )} />
      </div>

      {/* 当前步骤内容 */}
      {isReviewStep ? (
        <ReviewStep blocks={blocks} responses={responses} onEdit={setCurrentStep} interactiveBlocks={interactiveBlocks} />
      ) : currentBlock ? (
        <StepContent
          block={currentBlock}
          response={responses[currentBlock.id]}
          onUpdate={(resp) => updateResponse(currentBlock.id, resp)}
          stepIndex={currentStep}
          totalSteps={totalSteps}
        />
      ) : null}

      {/* 导航按钮 */}
      <div className="flex items-center justify-between pt-1">
        <Button
          variant="ghost"
          size="sm"
          onClick={handlePrev}
          disabled={currentStep === 0}
          className="gap-1 text-xs"
        >
          <ChevronLeft className="size-3.5" />
          上一步
        </Button>

        {isReviewStep ? (
          <Button
            size="sm"
            onClick={handleSubmit}
            disabled={disabled}
            className="gap-1.5 text-xs"
          >
            <Send className="size-3.5" />
            提交
          </Button>
        ) : (
          <Button
            size="sm"
            onClick={handleNext}
            disabled={!canProceed()}
            className="gap-1 text-xs"
          >
            下一步
            <ChevronRight className="size-3.5" />
          </Button>
        )}
      </div>
    </div>
  )
}

// ─── 步骤内容 ───

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
    <div className="min-h-[80px]">
      <div className="flex items-center justify-between mb-3">
        <p className="text-sm font-medium">{block.label}</p>
        <span className="text-[10px] text-muted-foreground">
          {stepIndex + 1} / {totalSteps}
        </span>
      </div>
      {block.type === 'single_select' && (
        <SelectBlockInteractive block={block} response={response} onUpdate={onUpdate} />
      )}
      {block.type === 'text_input' && (
        <TextInputBlockInteractive block={block} response={response} onUpdate={onUpdate} />
      )}
      {block.type === 'confirm' && (
        <ConfirmBlockInteractive block={block} response={response} onUpdate={onUpdate} />
      )}
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
      const next = selected.includes(value)
        ? selected.filter((v) => v !== value)
        : [...selected, value]
      onUpdate({ selected: next })
    } else {
      onUpdate({ selected: [value] })
    }
  }

  return (
    <div className="flex flex-col gap-2">
      {block.multiple && (
        <p className="text-[10px] text-muted-foreground">可多选</p>
      )}
      {block.options?.map((opt) => {
        const isSelected = selected.includes(opt.value)
        return (
          <button
            key={opt.value}
            onClick={() => toggle(opt.value)}
            className={cn(
              'flex items-center gap-3 rounded-xl border px-4 py-3 text-left text-sm transition-all cursor-pointer',
              isSelected
                ? 'border-primary bg-primary/5 shadow-sm'
                : 'border-border hover:border-primary/30 hover:bg-muted/30'
            )}
          >
            <div className={cn(
              'flex size-5 shrink-0 items-center justify-center rounded-full border-2 transition-colors',
              isSelected ? 'border-primary bg-primary' : 'border-muted-foreground/30'
            )}>
              {isSelected && <Check className="size-3 text-primary-foreground" />}
            </div>
            <div className="flex-1">
              <span className={cn(isSelected && 'font-medium')}>{opt.label}</span>
              {opt.description && (
                <p className="text-xs text-muted-foreground mt-0.5">{opt.description}</p>
              )}
            </div>
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
        <p className="text-[10px] text-muted-foreground mb-1.5">选填</p>
      )}
      <textarea
        value={response?.text ?? ''}
        onChange={(e) => onUpdate({ text: e.target.value })}
        placeholder={block.placeholder ?? '请输入...'}
        rows={3}
        className="w-full resize-none rounded-xl border bg-background px-4 py-3 text-sm leading-relaxed outline-none placeholder:text-muted-foreground focus:border-primary/50 focus:ring-1 focus:ring-primary/20 transition-all"
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
    <div className="flex gap-3">
      <button
        onClick={() => onUpdate({ confirmed: true })}
        className={cn(
          'flex-1 rounded-xl border-2 px-4 py-3 text-sm font-medium transition-all cursor-pointer',
          confirmed === true
            ? 'border-green-500 bg-green-500/10 text-green-600'
            : 'border-border hover:border-green-500/50'
        )}
      >
        {block.confirm_label ?? '确认'}
      </button>
      <button
        onClick={() => onUpdate({ confirmed: false })}
        className={cn(
          'flex-1 rounded-xl border-2 px-4 py-3 text-sm font-medium transition-all cursor-pointer',
          confirmed === false
            ? 'border-orange-500 bg-orange-500/10 text-orange-600'
            : 'border-border hover:border-orange-500/50'
        )}
      >
        {block.cancel_label ?? '取消'}
      </button>
    </div>
  )
}

// ─── 确认步骤 ───

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
    <div className="min-h-[80px]">
      <p className="text-sm font-medium mb-3">确认你的选择</p>
      <div className="flex flex-col gap-2">
        {blocks.map((block) => {
          const resp = responses[block.id]
          const editIndex = interactiveBlocks.findIndex((b) => b.id === block.id)
          return (
            <div key={block.id} className="flex items-start gap-2 rounded-lg bg-muted/30 px-3 py-2">
              <div className="flex-1 min-w-0">
                <p className="text-[10px] text-muted-foreground">{block.label}</p>
                <p className="text-xs mt-0.5 truncate">{formatBlockResponse(block, resp)}</p>
              </div>
              {editIndex >= 0 && (
                <button
                  onClick={() => onEdit(editIndex)}
                  className="text-[10px] text-primary hover:text-primary/80 shrink-0 cursor-pointer"
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

// ─── 工具函数 ───

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
    case 'info':
      return block.content ?? ''
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
