import { cn } from '@/lib/utils'

interface PageContainerProps {
  children: React.ReactNode
  className?: string
  ref?: React.Ref<HTMLDivElement>
}

export function PageContainer({ children, className, ref }: PageContainerProps) {
  return (
    <div ref={ref} className={cn('p-6', className)}>
      {children}
    </div>
  )
}
