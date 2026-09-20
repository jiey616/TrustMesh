// 键为 TaskStatus ∪ WorkflowStepStatus（流水线步骤状态复用同一套标签）
const MAP: Record<string, { label: string; color: string; bg: string }> = {
  planning: { label: '规划中', color: '#6d5ff5', bg: '#eeedfe' },
  review: { label: '待确认', color: '#ba7517', bg: '#faeeda' },
  pending: { label: '待开始', color: '#5f5e5a', bg: '#f1efe8' },
  in_progress: { label: '进行中', color: '#185fa5', bg: '#e6f1fb' },
  awaiting_review: { label: '待审核', color: '#ba7517', bg: '#faeeda' },
  waiting_user: { label: '等你处理', color: '#a32d2d', bg: '#fcebeb' },
  done: { label: '已完成', color: '#0f6e56', bg: '#e1f5ee' },
  failed: { label: '失败', color: '#a32d2d', bg: '#fcebeb' },
  canceled: { label: '已取消', color: '#5f5e5a', bg: '#f1efe8' },
  unassigned: { label: '待指派', color: '#5f5e5a', bg: '#f1efe8' },
}

export function StatusTag({ status }: { status: string }) {
  const t = MAP[status] ?? { label: status, color: '#5f5e5a', bg: '#f1efe8' }
  return (
    <span
      className="rounded-full px-2 py-[2px] text-[11px] leading-[16px]"
      style={{ color: t.color, background: t.bg }}
    >
      {t.label}
    </span>
  )
}
