import { forwardRef, type ComponentPropsWithoutRef } from 'react'
import { cn } from '@/lib/utils'

type ScrollAreaProps = ComponentPropsWithoutRef<'div'>

const ScrollArea = forwardRef<HTMLDivElement, ScrollAreaProps>(({ className, children, ...props }, ref) => {
  return (
    <div ref={ref} className={cn('overflow-auto', className)} {...props}>
      {children}
    </div>
  )
})

ScrollArea.displayName = 'ScrollArea'

function ScrollBar() {
  return null
}

export { ScrollArea, ScrollBar }
