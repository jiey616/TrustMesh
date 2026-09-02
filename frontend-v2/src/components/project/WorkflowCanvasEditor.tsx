import { useState } from 'react'
import { Button, Input, Select, Switch, Tooltip } from 'antd'
import {
  PlusOutlined,
  DeleteOutlined,
  HolderOutlined,
  AimOutlined,
} from '@ant-design/icons'
import type { Agent, Workflow, WorkflowStep } from '@/types'
import { AgentAvatar } from '@/components/shared/AgentAvatar'
import { StepIOEditor } from './StepIOEditor'

const STEP_PLACEHOLDERS = ['编剧产出剧本', '导演拆解分镜', '测试质量验收', '人工复核']

/**
 * 工作流画布编辑器（借鉴 n8n 节点视觉 + Zapier 线性步骤流）：
 * 开始 → 步骤节点（数字员工头像 + 名称 + 绑定数字员工 + 人工确认）→ 完成
 * 支持：步骤间 + 插入、拖拽排序、内联配置、无效步骤标红
 */
export function WorkflowCanvasEditor({
  workflow,
  onChange,
  agents,
}: {
  workflow: Workflow
  onChange: (wf: Workflow) => void
  agents: Agent[]
}) {
  const steps = workflow.steps ?? []
  const set = (next: Workflow) => onChange(next)

  const updateStep = (idx: number, patch: Partial<WorkflowStep>) => {
    const next = steps.map((s, i) => (i === idx ? { ...s, ...patch } : s))
    set({ ...workflow, steps: next })
  }

  const addStep = (at: number) => {
    const newStep: WorkflowStep = { name: '', need_review: false }
    const next = [...steps]
    next.splice(at, 0, newStep)
    set({ ...workflow, steps: next })
  }

  const removeStep = (idx: number) => {
    const next = steps.filter((_, i) => i !== idx)
    set({ ...workflow, steps: next })
  }

  const [dragIdx, setDragIdx] = useState<number | null>(null)
  const [dragOver, setDragOver] = useState<number | null>(null)

  const handleDrop = (targetIdx: number) => {
    if (dragIdx === null || dragIdx === targetIdx) {
      setDragIdx(null)
      setDragOver(null)
      return
    }
    const next = [...steps]
    const [moved] = next.splice(dragIdx, 1)
    next.splice(targetIdx, 0, moved)
    set({ ...workflow, steps: next })
    setDragIdx(null)
    setDragOver(null)
  }

  const findAgent = (s: WorkflowStep) =>
    s.agent_id ? agents.find((a) => a.id === s.agent_id) : undefined

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
      <Input
        value={workflow.name}
        onChange={(e) => set({ ...workflow, name: e.target.value })}
        placeholder="工作流名称（如：剧本制作流水线）"
        style={{
          fontWeight: 600,
          background: 'rgba(255,255,255,0.04)',
          borderColor: 'rgba(255,255,255,0.1)',
        }}
      />

      <div style={{ marginTop: 16, display: 'flex', flexDirection: 'column', alignItems: 'stretch' }}>
        {/* 开始端点 */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <div style={{ width: 36, display: 'flex', justifyContent: 'center' }}>
            <span
              style={{
                width: 10,
                height: 10,
                borderRadius: '50%',
                background: '#27a644',
                boxShadow: '0 0 8px rgba(39,166,68,0.8)',
              }}
            />
          </div>
          <span style={{ fontSize: 12, fontWeight: 600, color: 'rgba(255,255,255,0.5)' }}>开始</span>
        </div>

        {/* 步骤节点 */}
        {steps.map((step, idx) => {
          const agent = findAgent(step)
          const invalid = !step.name.trim() || (!step.role?.trim() && !step.agent_id)
          const isDragging = dragIdx === idx
          return (
            <div key={idx} style={{ display: 'flex', flexDirection: 'column' }}>
              {/* 连线 + 插入按钮 */}
              <div style={{ width: 36, display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
                <span style={{ width: 1, height: 14, background: 'rgba(255,255,255,0.12)' }} />
                <Tooltip title="在此处插入步骤">
                  <button
                    type="button"
                    onClick={() => addStep(idx)}
                    style={{
                      width: 20,
                      height: 20,
                      borderRadius: '50%',
                      border: '1px solid rgba(109,95,245,0.5)',
                      background: 'rgba(109,95,245,0.12)',
                      color: '#8b7ff8',
                      fontSize: 11,
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      cursor: 'pointer',
                      lineHeight: 1,
                    }}
                  >
                    <PlusOutlined />
                  </button>
                </Tooltip>
                <span style={{ width: 1, height: 14, background: 'rgba(255,255,255,0.12)' }} />
              </div>

              {/* 节点卡片 */}
              <div
                draggable
                onDragStart={() => setDragIdx(idx)}
                onDragOver={(e) => {
                  e.preventDefault()
                  setDragOver(idx)
                }}
                onDrop={(e) => {
                  e.preventDefault()
                  handleDrop(idx)
                }}
                onDragEnd={() => {
                  setDragIdx(null)
                  setDragOver(null)
                }}
                style={{
                  display: 'flex',
                  gap: 10,
                  alignItems: 'stretch',
                  opacity: isDragging ? 0.4 : 1,
                  transform: dragOver === idx && dragIdx !== null && dragIdx !== idx ? 'translateY(4px)' : 'none',
                  transition: 'opacity 0.15s, transform 0.15s',
                }}
              >
                {/* 拖拽手柄 */}
                <div
                  style={{
                    width: 12,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    cursor: 'grab',
                    color: 'rgba(255,255,255,0.25)',
                    fontSize: 12,
                  }}
                  title="拖拽排序"
                >
                  <HolderOutlined />
                </div>

                {/* 节点图标（数字员工头像 / 序号） */}
                <div style={{ display: 'flex', alignItems: 'center', flexShrink: 0 }}>
                  {agent ? (
                    <AgentAvatar name={agent.name} role={agent.role} seed={agent.node_id} size={36} />
                  ) : (
                    <div
                      style={{
                        width: 36,
                        height: 36,
                        borderRadius: '50%',
                        border: `1.5px dashed ${invalid ? 'rgba(239,68,68,0.5)' : 'rgba(109,95,245,0.5)'}`,
                        background: 'rgba(109,95,245,0.08)',
                        color: invalid ? '#f87171' : '#8b7ff8',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        fontSize: 15,
                        flexShrink: 0,
                      }}
                    >
                      {idx + 1}
                    </div>
                  )}
                </div>

                {/* 节点卡片主体 */}
                <div
                  style={{
                    flex: 1,
                    minWidth: 0,
                    borderRadius: 12,
                    border: `1px solid ${invalid ? 'rgba(239,68,68,0.4)' : 'rgba(255,255,255,0.1)'}`,
                    background: invalid ? 'rgba(239,68,68,0.04)' : 'rgba(255,255,255,0.03)',
                    padding: '10px 12px',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 8,
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <Input
                      value={step.name}
                      onChange={(e) => updateStep(idx, { name: e.target.value })}
                      placeholder={`步骤 ${idx + 1} · ${STEP_PLACEHOLDERS[idx % STEP_PLACEHOLDERS.length]}`}
                      style={{
                        flex: 1,
                        minWidth: 0,
                        fontSize: 13,
                        fontWeight: 500,
                        background: 'rgba(255,255,255,0.04)',
                        borderColor: 'rgba(255,255,255,0.1)',
                      }}
                    />
                    <Tooltip title="删除步骤">
                      <Button
                        type="text"
                        size="small"
                        icon={<DeleteOutlined />}
                        style={{ color: 'rgba(255,255,255,0.4)' }}
                        onClick={() => removeStep(idx)}
                      />
                    </Tooltip>
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
                    <Select
                      style={{ width: 220 }}
                      size="small"
                      value={step.agent_id ?? undefined}
                      onChange={(v) => {
                        const a = agents.find((x) => x.id === v)
                        if (a) updateStep(idx, { agent_id: a.id, role: a.role || a.name })
                        else updateStep(idx, { agent_id: '', role: '' })
                      }}
                      placeholder="绑定执行数字员工"
                      allowClear
                      options={(agents ?? [])
                        .filter((a) => !a.archived)
                        .map((a) => ({
                          value: a.id,
                          label: `${a.name}（${a.role || '无角色'}）`,
                        }))}
                    />
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12, color: 'rgba(255,255,255,0.55)' }}>
                      <Switch
                        size="small"
                        checked={!!step.need_review}
                        onChange={(chk) => updateStep(idx, { need_review: chk })}
                      />
                      需人工确认
                    </span>
                  </div>
                  {/* 文件输入 / 输出定义（可关联上游步骤的输出文件） */}
                  <StepIOEditor
                    step={step}
                    steps={steps}
                    stepIndex={idx}
                    onChange={(patch) => updateStep(idx, patch)}
                  />
                </div>
              </div>
            </div>
          )
        })}

        {/* 末尾连线 + 添加按钮 */}
        <div style={{ width: 36, display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
          <span style={{ width: 1, height: 14, background: 'rgba(255,255,255,0.12)' }} />
          {steps.length === 0 ? (
            <div style={{ width: 20, height: 20, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <span style={{ width: 1, height: 20, background: 'rgba(255,255,255,0.12)' }} />
            </div>
          ) : (
            <Tooltip title="在末尾添加步骤">
              <button
                type="button"
                onClick={() => addStep(steps.length)}
                style={{
                  width: 20,
                  height: 20,
                  borderRadius: '50%',
                  border: '1px solid rgba(109,95,245,0.5)',
                  background: 'rgba(109,95,245,0.12)',
                  color: '#8b7ff8',
                  fontSize: 11,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  cursor: 'pointer',
                  lineHeight: 1,
                }}
              >
                <PlusOutlined />
              </button>
            </Tooltip>
          )}
          <span style={{ width: 1, flex: 1, minHeight: 14, background: 'rgba(255,255,255,0.12)' }} />
        </div>

        {/* 完成端点 */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <div style={{ width: 36, display: 'flex', justifyContent: 'center' }}>
            <span
              style={{
                width: 10,
                height: 10,
                borderRadius: '50%',
                background: '#6dc67f',
                boxShadow: '0 0 8px rgba(109,198,127,0.8)',
              }}
            />
          </div>
          <span style={{ fontSize: 12, fontWeight: 600, color: 'rgba(255,255,255,0.5)' }}>
            完成
          </span>
        </div>
      </div>

      {/* 空态引导 */}
      {steps.length === 0 && (
        <div
          style={{
            marginTop: 16,
            padding: '24px 16px',
            textAlign: 'center',
            borderRadius: 12,
            border: '1px dashed rgba(109,95,245,0.35)',
            background: 'rgba(109,95,245,0.04)',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: 10,
          }}
        >
          <AimOutlined style={{ fontSize: 26, color: '#6d5ff5' }} />
          <div style={{ fontSize: 13, color: 'rgba(255,255,255,0.6)' }}>
            还没有步骤。点击上方 <PlusOutlined style={{ fontSize: 10, color: '#8b7ff8' }} /> 或下方按钮开始搭建流水线
          </div>
          <Button size="small" type="primary" icon={<PlusOutlined />} onClick={() => addStep(0)}>
            添加第一个步骤
          </Button>
        </div>
      )}
    </div>
  )
}
