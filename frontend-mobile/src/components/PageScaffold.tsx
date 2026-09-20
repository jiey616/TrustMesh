import type { ReactNode } from 'react'

interface Props {
  title: string
  /** 顶栏右侧动作区 */
  extra?: ReactNode
  children: ReactNode
}

/**
 * 页面骨架：吸顶标题栏 + 可滚动内容区。
 * 底部 TabBar 由 TabLayout 提供（不再每页自己留白），避免重复占位与滚动区高度错乱。
 */
export function PageScaffold({ title, extra, children }: Props) {
  return (
    <div className="flex min-h-full flex-col">
      <header className="tm-safe-top sticky top-0 z-10 border-b border-[var(--tm-line)] bg-white/95 backdrop-blur">
        <div className="flex h-[44px] items-center justify-between px-4">
          <h1 className="text-[17px] font-medium">{title}</h1>
          <div className="flex items-center gap-3">{extra}</div>
        </div>
      </header>
      <main className="flex-1 px-4 py-3">{children}</main>
    </div>
  )
}
