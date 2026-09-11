import { useEffect, useMemo, useState } from 'react'
import { Alert, Input, Modal, Select, Tag } from 'antd'
import { useBindStepOutput, useWorkflowProgress } from '@/hooks/useProjects'
import { useProjectFiles } from '@/hooks/useProjectFiles'

/** 待绑定的文件：优先 file_id（项目文件，含用户手工上传的），否则 artifact_id。 */
export interface BindOutputFile {
  fileId?: string
  artifactId?: string
  fileName: string
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  projectId: string
  /** 已确定的待绑定文件；为 null 且 pickFile 时可在这里现选。 */
  file: BindOutputFile | null
  /** 从流程条某个步骤发起时锁定目标步骤，用户不必再选。 */
  lockedStepIndex?: number
  /** 未指定文件时允许在弹窗内挑选项目文件（从步骤视角发起时用）。 */
  pickFile?: boolean
  onBound?: () => void
}

/**
 * 项目流程 · 手工绑定交付物。
 *
 * 把项目里任意一个文件（用户在文件区手工上传的、或别的任务产出的）绑定为总流程
 * 某个步骤的输出位交付物。地址用 project + stepIndex，后端自己解析承载任务与
 * todo，所以任意入口都能直接复用这个弹窗，不必先知道该步骤对应哪个 todoId。
 *
 * 输出位约束与后端一致：步骤声明了输出位就只能在声明范围内选（避免拼出下游步骤
 * 取不到的名字），没声明才允许自由命名。
 */
export function BindOutputModal({
  open,
  onOpenChange,
  projectId,
  file,
  lockedStepIndex,
  pickFile = false,
  onBound,
}: Props) {
  const { data: progress } = useWorkflowProgress(open ? projectId : undefined)
  const bindMutation = useBindStepOutput(projectId)
  const [stepIndex, setStepIndex] = useState<number | undefined>(lockedStepIndex)
  const [outputName, setOutputName] = useState('')
  const [pickedFileId, setPickedFileId] = useState<string | undefined>(undefined)

  // 只有「没给文件 + 允许现选」时才拉项目文件列表。
  const needPicker = pickFile && !file
  const { data: projectFiles } = useProjectFiles(open && needPicker ? projectId : undefined)
  const fileOptions = useMemo(
    () => (projectFiles ?? []).filter((f) => !f.is_folder).map((f) => ({ value: f.id, label: f.file_name })),
    [projectFiles],
  )
  // 优先用外部指定的文件；否则用弹窗内选中的。
  const effectiveFile: BindOutputFile | null = useMemo(() => {
    if (file) return file
    if (!pickedFileId) return null
    const hit = (projectFiles ?? []).find((f) => f.id === pickedFileId)
    return { fileId: pickedFileId, fileName: hit?.file_name ?? '' }
  }, [file, pickedFileId, projectFiles])

  const steps = progress?.steps ?? []
  const step = stepIndex !== undefined ? steps[stepIndex] : undefined
  const declared = useMemo(() => step?.declared_outputs ?? [], [step])
  const occupied = useMemo(
    () => new Set((step?.outputs ?? []).map((o) => o.output_name).filter(Boolean) as string[]),
    [step],
  )
  const notDispatched = !!step && (step.status === 'unassigned' || !step.task_id)

  // 每次打开重置：锁定步骤时直接选中，否则清空让用户自己选。
  useEffect(() => {
    if (!open) return
    setStepIndex(lockedStepIndex)
    setOutputName('')
    setPickedFileId(undefined)
  }, [open, lockedStepIndex])

  // 步骤只有一个声明输出位时直接预选，少一次点击。
  useEffect(() => {
    if (stepIndex === undefined) return
    const d = steps[stepIndex]?.declared_outputs ?? []
    setOutputName(d.length === 1 ? d[0] : '')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stepIndex, steps.length])

  const trimmed = outputName.trim()
  const willReplace = !!trimmed && occupied.has(trimmed)
  const canSubmit = !!effectiveFile && stepIndex !== undefined && !!trimmed && !notDispatched && !bindMutation.isPending

  const submit = async () => {
    if (!effectiveFile || stepIndex === undefined || !trimmed) return
    try {
      await bindMutation.mutateAsync({
        stepIndex,
        input: {
          file_id: effectiveFile.fileId,
          artifact_id: effectiveFile.artifactId,
          output_name: trimmed,
          replace: willReplace,
        },
      })
      onBound?.()
      onOpenChange(false)
    } catch (err) {
      // 后端的 AppError message 已带可读文案（输出位未声明 / 步骤未派发 / 位已被占用…）
      Modal.error({
        title: '绑定失败',
        content: err instanceof Error ? err.message : '绑定失败，请稍后重试',
      })
    }
  }

  return (
    <Modal
      title="绑定为交付物"
      open={open}
      onCancel={() => !bindMutation.isPending && onOpenChange(false)}
      onOk={() => void submit()}
      okText="绑定"
      cancelText="取消"
      confirmLoading={bindMutation.isPending}
      okButtonProps={{ disabled: !canSubmit }}
      destroyOnClose
      width={480}
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: 14, paddingTop: 4 }}>
        <div style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          把该文件绑定为项目总流程某个步骤的最终交付物，绑定后该步骤的产出区会展示它。
        </div>

        {needPicker ? (
          <div>
            <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginBottom: 6 }}>要绑定的文件</div>
            <Select
              style={{ width: '100%' }}
              showSearch
              optionFilterProp="label"
              placeholder="选择项目里的文件"
              value={pickedFileId}
              onChange={(v) => setPickedFileId(v)}
              options={fileOptions}
              notFoundContent={projectFiles ? '项目里还没有文件，请先在文件区上传' : '加载中…'}
            />
          </div>
        ) : (
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 8,
              padding: '8px 10px',
              borderRadius: 'var(--radius-control)',
              border: '1px solid var(--line)',
              background: 'var(--surface-raised)',
            }}
          >
            <span style={{ fontSize: 12, color: 'var(--text-quaternary)', flexShrink: 0 }}>文件</span>
            <span
              style={{
                fontSize: 13,
                color: 'var(--text-primary)',
                fontWeight: 500,
                minWidth: 0,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {effectiveFile?.fileName ?? '-'}
            </span>
          </div>
        )}

        <div>
          <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginBottom: 6 }}>目标步骤</div>
          <Select
            style={{ width: '100%' }}
            placeholder="选择要绑定的流程步骤"
            value={stepIndex}
            onChange={(v) => setStepIndex(v)}
            disabled={lockedStepIndex !== undefined}
            options={steps.map((s, i) => ({
              value: i,
              label: `${i + 1}. ${s.name}`,
              disabled: s.status === 'unassigned' || !s.task_id,
            }))}
            notFoundContent={progress ? '该项目尚未配置总流程' : '加载中…'}
          />
        </div>

        {step && (
          <div>
            <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginBottom: 6 }}>
              输出位{declared.length > 0 ? '（该步骤声明的可选范围）' : '（该步骤未声明输出位，可自由命名）'}
            </div>
            {declared.length > 0 ? (
              <Select
                style={{ width: '100%' }}
                placeholder="选择输出位"
                value={trimmed || undefined}
                onChange={(v) => setOutputName(v)}
                options={declared.map((name) => ({
                  value: name,
                  label: occupied.has(name) ? `${name}（将覆盖）` : name,
                }))}
              />
            ) : (
              <Input
                placeholder="输入输出位名称，例如：分镜脚本"
                value={outputName}
                onChange={(e) => setOutputName(e.target.value)}
                maxLength={80}
              />
            )}
          </div>
        )}

        {notDispatched && (
          <Alert
            type="warning"
            showIcon
            message="该步骤尚未派发任务"
            description="流程步骤必须已经派发任务才能绑定交付物，请先让 PM 启动该步骤。"
          />
        )}

        {willReplace && !notDispatched && (
          <Alert
            type="info"
            showIcon
            message={
              <span>
                输出位 <Tag color="purple">{trimmed}</Tag> 已被占用，绑定将覆盖原有交付物。
              </span>
            }
          />
        )}
      </div>
    </Modal>
  )
}
