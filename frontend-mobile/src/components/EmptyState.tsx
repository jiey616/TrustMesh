interface Props {
  title: string
  description?: string
}

export function EmptyState({ title, description }: Props) {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <div className="mb-3 h-12 w-12 rounded-full bg-[var(--tm-brand-soft)]" />
      <p className="text-[15px] font-medium">{title}</p>
      {description ? (
        <p className="mt-1 max-w-[240px] text-[13px] leading-relaxed text-[var(--tm-text-2)]">
          {description}
        </p>
      ) : null}
    </div>
  )
}
