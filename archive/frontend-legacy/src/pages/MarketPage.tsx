import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Search } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { useMarketDepts, useMarketRoles } from '@/hooks/useMarket'
import { RoleCard } from '@/components/market/RoleCard'
import { useDebounce } from '@/hooks/useDebounce'

export function MarketPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [inputValue, setInputValue] = useState(searchParams.get('q') ?? '')
  const debouncedQuery = useDebounce(inputValue, 300)

  const activeDept = searchParams.get('dept') ?? ''

  const { data: depts, isLoading: deptsLoading } = useMarketDepts()
  const { data: roles, isLoading: rolesLoading } = useMarketRoles({
    dept: activeDept || undefined,
    q: debouncedQuery || undefined,
  })

  function handleDeptSelect(deptId: string) {
    setSearchParams(prev => {
      const next = new URLSearchParams(prev)
      if (deptId) {
        next.set('dept', deptId)
      } else {
        next.delete('dept')
      }
      return next
    })
  }

  function handleSearch(value: string) {
    setInputValue(value)
    setSearchParams(prev => {
      const next = new URLSearchParams(prev)
      if (value) {
        next.set('q', value)
      } else {
        next.delete('q')
      }
      return next
    })
  }

  const totalCount = roles?.length ?? 0
  const activeDeptName = depts?.find(d => d.id === activeDept)?.name

  return (
    <div className="flex h-full flex-col">
      {/* 页头 */}
      <div className="border-b px-6 py-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold">岗位市场</h1>
            <p className="mt-0.5 text-sm text-muted-foreground">
              191 个预置 AI 智能体角色，下载到本地即可接入团队
            </p>
          </div>
          <div className="relative w-64">
            <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="搜索角色名称或描述..."
              className="pl-9"
              value={inputValue}
              onChange={e => handleSearch(e.target.value)}
            />
          </div>
        </div>
      </div>

      {/* 主体：左侧分类 + 右侧列表 */}
      <div className="flex flex-1 min-h-0">
        {/* 左侧部门导航 */}
        <aside className="w-48 shrink-0 border-r">
          <ScrollArea className="h-full py-3">
            <nav className="space-y-0.5 px-2">
              <button
                onClick={() => handleDeptSelect('')}
                className={cn(
                  'flex w-full items-center justify-between rounded-md px-3 py-2 text-sm transition-colors hover:bg-muted',
                  !activeDept && 'bg-muted font-medium'
                )}
              >
                <span>全部</span>
                {deptsLoading ? (
                  <Skeleton className="h-4 w-8" />
                ) : (
                  <span className="text-xs text-muted-foreground">
                    {depts?.reduce((s, d) => s + d.count, 0) ?? 0}
                  </span>
                )}
              </button>
              {deptsLoading
                ? Array.from({ length: 8 }).map((_, i) => (
                    <Skeleton key={i} className="mx-1 my-0.5 h-8" />
                  ))
                : depts?.map(dept => (
                    <button
                      key={dept.id}
                      onClick={() => handleDeptSelect(dept.id)}
                      className={cn(
                        'flex w-full items-center justify-between rounded-md px-3 py-2 text-sm transition-colors hover:bg-muted',
                        activeDept === dept.id && 'bg-muted font-medium'
                      )}
                    >
                      <span className="truncate">{dept.name}</span>
                      <span className="ml-2 shrink-0 text-xs text-muted-foreground">{dept.count}</span>
                    </button>
                  ))}
            </nav>
          </ScrollArea>
        </aside>

        {/* 右侧角色列表 */}
        <div className="flex-1 min-w-0 flex flex-col">
          {/* 状态条 */}
          <div className="border-b px-5 py-2.5 text-sm text-muted-foreground">
            {activeDeptName ? (
              <span>
                <span className="font-medium text-foreground">{activeDeptName}</span>
                {debouncedQuery && (
                  <>
                    {' · 搜索 "'}
                    <span className="font-medium text-foreground">{debouncedQuery}</span>
                    {'"'}
                  </>
                )}
              </span>
            ) : debouncedQuery ? (
              <span>
                搜索 "
                <span className="font-medium text-foreground">{debouncedQuery}</span>
                "
              </span>
            ) : (
              <span>全部角色</span>
            )}
            {!rolesLoading && (
              <span className="ml-2">· {totalCount} 个结果</span>
            )}
          </div>

          <ScrollArea className="flex-1">
            {rolesLoading ? (
              <div className="grid grid-cols-2 gap-4 p-5 xl:grid-cols-3 2xl:grid-cols-4">
                {Array.from({ length: 12 }).map((_, i) => (
                  <Skeleton key={i} className="h-36 rounded-lg" />
                ))}
              </div>
            ) : roles && roles.length > 0 ? (
              <div className="grid grid-cols-2 gap-4 p-5 xl:grid-cols-3 2xl:grid-cols-4">
                {roles.map(role => (
                  <RoleCard key={role.id} role={role} />
                ))}
              </div>
            ) : (
              <div className="flex flex-col items-center justify-center py-20 text-center">
                <p className="text-muted-foreground">未找到匹配的角色</p>
                {(debouncedQuery || activeDept) && (
                  <button
                    onClick={() => {
                      handleDeptSelect('')
                      handleSearch('')
                    }}
                    className="mt-2 text-sm text-primary hover:underline"
                  >
                    清除筛选
                  </button>
                )}
              </div>
            )}
          </ScrollArea>
        </div>
      </div>
    </div>
  )
}
