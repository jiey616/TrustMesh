import { Link, useSearchParams } from 'react-router-dom'
import { cn } from '@/lib/utils'
import type { MarketRoleListItem } from '@/types'

const deptColorMap: Record<string, string> = {
  engineering:          'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  marketing:            'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
  design:               'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-400',
  product:              'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
  'project-management': 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400',
  testing:              'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
  support:              'bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-400',
  specialized:          'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-400',
  'creative-tech':      'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-400',
  finance:              'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
  hr:                   'bg-lime-100 text-lime-700 dark:bg-lime-900/30 dark:text-lime-400',
  legal:                'bg-stone-100 text-stone-700 dark:bg-stone-900/30 dark:text-stone-400',
  'sales-marketing':    'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400',
  'supply-chain':       'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
  academic:             'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-400',
}

interface RoleCardProps {
  role: MarketRoleListItem
}

export function RoleCard({ role }: RoleCardProps) {
  const [searchParams] = useSearchParams()
  const deptColor = deptColorMap[role.dept_id] ?? 'bg-muted text-muted-foreground'

  return (
    <Link
      to={`/market/roles/${role.id}`}
      state={{ fromSearch: searchParams.toString() }}
      className="flex flex-col gap-3 rounded-lg border bg-card p-4 transition-shadow hover:shadow-md hover:border-border/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <div className="flex items-start justify-between gap-2">
        <h3 className="font-semibold leading-tight text-foreground">{role.name}</h3>
        <span className={cn('shrink-0 rounded-full px-2 py-0.5 text-xs font-medium', deptColor)}>
          {role.dept_name}
        </span>
      </div>
      <p className="line-clamp-2 text-sm text-muted-foreground">{role.description}</p>
      <span className="mt-auto text-xs text-primary font-medium">查看详情 →</span>
    </Link>
  )
}
