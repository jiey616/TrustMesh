import { useState } from 'react'
import { Button, Modal, Input, Typography, Tag, App } from 'antd'
import { RedoOutlined, ExclamationCircleOutlined } from '@ant-design/icons'
import { useResumeTaskTodo } from '@/hooks/useTasks'
import type { Todo } from '@/types'

const { Text } = Typography

/** 与后端 store.maxReopens 保持一致：同一 todo 最多被重开 3 次。 */
const MAX_REOPENS = 3

interface Props {
  taskId: string
  /** 当前任务里处于 failed 的 todo（至少一个，否则不该渲染本组件）。 */
  failedTodos: Todo[]
  /** 成功发起「重试/继续」后的回调（用于关闭外层或滚动）。 */
  onResumed?: () => void
}

/**
 * 失败任务的「重试/继续」入口，占据 Composer 的位置。
 *
 * 与「重开 todo」的区别：重开只把状态拉回 in_progress、不通知执行者，用户还得自己去
 * 评论区 @ 数字员工（而 @ 并不是执行通道）。这里走的是平台同一条请求内
 * 「重开 + 带 resume 标记的 remind」，执行侧会据此真的接着干活。
 *
 * 理由必填：它是执行侧唯一能知道「上次错在哪、这次要改什么」的输入。
 */
export function TaskResumeEntry({ taskId, failedTodos, onResumed }: Props) {
  const { message } = App.useApp()
  const resumeTodo = useResumeTaskTodo()

  const [open, setOpen] = useState(false)
  const [reason, setReason] = useState('')
  const [selectedId, setSelectedId] = useState<string>(() => failedTodos[0]?.id ?? '')
  const [error, setError] = useState('')

  // 选中的 todo 可能因刷新而消失/换状态；回退到第一个仍失败的 todo。
  const selected = failedTodos.find((t) => t.id === selectedId) ?? failedTodos[0]
  const reopenCount = selected?.reopen_count ?? 0
  const exhausted = reopenCount >= MAX_REOPENS

  const close = () => {
    setOpen(false)
    setReason('')
    setError('')
  }

  const handleSubmit = async () => {
    if (!selected) {
      setError('没有可重试的步骤')
      return
    }
    if (reason.trim().length < 2) {
      setError('请填写重试理由（至少 2 个字），说明上次失败的原因或这次要改什么')
      return
    }
    if (exhausted) {
      setError(`该步骤已重试 ${MAX_REOPENS} 次，已达上限，请新建任务`)
      return
    }
    try {
      await resumeTodo.mutateAsync({ taskId, todoId: selected.id, reason: reason.trim() })
      message.success('已重新开始执行，数字员工会按你的理由继续')
      close()
      onResumed?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : '重试失败')
    }
  }

  return (
    <>
      <div
        style={{
          maxWidth: 560,
          margin: '0 auto',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 12,
          background: 'var(--surface-sunken)',
          border: '1px solid var(--line)',
          borderRadius: 'var(--radius-control)',
          padding: '10px 12px',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
          <ExclamationCircleOutlined style={{ color: 'var(--error)', fontSize: 14, flexShrink: 0 }} />
          <Text style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
            任务已失败，需先重试/继续才能再次执行
            {failedTodos.length > 1 && (
              <Text type="secondary" style={{ fontSize: 12 }}>（{failedTodos.length} 个步骤失败）</Text>
            )}
          </Text>
        </div>
        <Button
          type="primary"
          icon={<RedoOutlined />}
          onClick={() => setOpen(true)}
          style={{ flexShrink: 0 }}
        >
          重试/继续
        </Button>
      </div>

      <Modal
        open={open}
        title="重试/继续执行"
        okText="重新开始执行"
        cancelText="取消"
        confirmLoading={resumeTodo.isPending}
        onOk={() => void handleSubmit()}
        onCancel={close}
        destroyOnHidden
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {failedTodos.length > 1 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <Text type="secondary" style={{ fontSize: 12 }}>选择要重试的步骤</Text>
              {failedTodos.map((t) => (
                <Button
                  key={t.id}
                  size="small"
                  type={t.id === selected?.id ? 'primary' : 'default'}
                  onClick={() => setSelectedId(t.id)}
                  style={{ textAlign: 'left', height: 'auto', padding: '6px 10px', whiteSpace: 'normal' }}
                >
                  {t.title}
                  {(t.reopen_count ?? 0) > 0 && (
                    <Tag style={{ marginLeft: 6, fontSize: 11 }} color={(t.reopen_count ?? 0) >= MAX_REOPENS ? 'error' : 'warning'}>
                      已重试 {t.reopen_count}/{MAX_REOPENS}
                    </Tag>
                  )}
                </Button>
              ))}
            </div>
          )}

          {selected?.error && (
            <div
              style={{
                fontSize: 12,
                color: 'var(--error)',
                whiteSpace: 'pre-wrap',
                background: 'rgba(244,63,94,0.06)',
                borderRadius: 'var(--radius-control)',
                padding: '6px 8px',
                maxHeight: 120,
                overflowY: 'auto',
              }}
            >
              上次失败原因：{selected.error}
            </div>
          )}

          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <Text type="secondary" style={{ fontSize: 12 }}>
              重试理由（必填）
              {failedTodos.length === 1 && reopenCount > 0 && (
                <Tag style={{ marginLeft: 8, fontSize: 11 }} color={exhausted ? 'error' : 'warning'}>
                  已重试 {reopenCount}/{MAX_REOPENS}
                </Tag>
              )}
            </Text>
            <Input.TextArea
              value={reason}
              onChange={(e) => {
                setReason(e.target.value)
                if (error) setError('')
              }}
              placeholder="说明上次失败的原因，或这次希望怎么改（数字员工会以此为最高优先级依据）"
              rows={3}
              autoFocus
              maxLength={500}
              showCount
            />
          </div>

          <Text type="secondary" style={{ fontSize: 12 }}>
            提交后该步骤会重新开始执行，并消耗执行额度。
          </Text>

          {error && (
            <Text style={{ fontSize: 12, color: 'var(--error)' }}>{error}</Text>
          )}
        </div>
      </Modal>
    </>
  )
}
