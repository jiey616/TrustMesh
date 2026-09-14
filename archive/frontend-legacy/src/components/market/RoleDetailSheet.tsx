import { Download } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from '@/components/ui/sheet'
import { InstallGuide } from './InstallGuide'
import { useMarketRole } from '@/hooks/useMarket'
import { downloadRole } from '@/api/market'
import { cn } from '@/lib/utils'
import { useState } from 'react'
import { toast } from 'sonner'

const deptColorMap: Record<string, string> = {
  engineering: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  marketing: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
  design: 'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-400',
  product: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
  'project-management': 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400',
  testing: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
  support: 'bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-400',
  specialized: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-400',
  'creative-tech': 'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-400',
  finance: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
  hr: 'bg-lime-100 text-lime-700 dark:bg-lime-900/30 dark:text-lime-400',
  legal: 'bg-stone-100 text-stone-700 dark:bg-stone-900/30 dark:text-stone-400',
  'sales-marketing': 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400',
  'supply-chain': 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
  academic: 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-400',
}

interface RoleDetailSheetProps {
  roleId: string | undefined
  open: boolean
  onOpenChange: (open: boolean) => void
}

function MarkdownContent({ content }: { content: string }) {
  return (
    <div className="prose prose-sm dark:prose-invert max-w-none [&>*:first-child]:mt-0">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
    </div>
  )
}

export function RoleDetailSheet({ roleId, open, onOpenChange }: RoleDetailSheetProps) {
  const { data: role, isLoading } = useMarketRole(roleId)
  const [downloading, setDownloading] = useState(false)

  async function handleDownload() {
    if (!roleId) return
    setDownloading(true)
    try {
      await downloadRole(roleId)
      toast.success('角色包下载成功')
    } catch {
      toast.error('下载失败，请重试')
    } finally {
      setDownloading(false)
    }
  }

  const deptColor = role ? (deptColorMap[role.dept_id] ?? 'bg-muted text-muted-foreground') : ''

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-xl flex flex-col gap-0 p-0" showCloseButton>
        {isLoading || !role ? (
          <div className="p-6 space-y-4">
            <Skeleton className="h-6 w-2/3" />
            <Skeleton className="h-4 w-1/3" />
            <Skeleton className="h-32 w-full" />
          </div>
        ) : (
          <>
            {/* 头部 */}
            <SheetHeader className="px-6 pt-6 pb-4 border-b">
              <div className="flex items-start justify-between gap-3 pr-8">
                <div className="min-w-0">
                  <SheetTitle className="text-lg font-semibold">{role.name}</SheetTitle>
                  <SheetDescription className="mt-1 text-sm">{role.description}</SheetDescription>
                </div>
                <span className={cn('shrink-0 rounded-full px-2.5 py-1 text-xs font-medium', deptColor)}>
                  {role.dept_name}
                </span>
              </div>
              <Button
                onClick={handleDownload}
                disabled={downloading}
                size="sm"
                className="mt-3 w-full"
              >
                <Download className="size-4 mr-1.5" />
                {downloading ? '下载中...' : '下载角色包'}
              </Button>
            </SheetHeader>

            {/* Tabs 内容 */}
            <Tabs defaultValue="identity" className="flex-1 flex flex-col min-h-0 px-6 pt-4">
              <TabsList variant="line" className="w-full justify-start">
                <TabsTrigger value="identity">简介</TabsTrigger>
                <TabsTrigger value="soul">人格</TabsTrigger>
                <TabsTrigger value="agents">规范</TabsTrigger>
              </TabsList>
              <TabsContent value="identity" className="flex-1 min-h-0 mt-3">
                <ScrollArea className="h-full">
                  <MarkdownContent content={role.identity_content} />
                </ScrollArea>
              </TabsContent>
              <TabsContent value="soul" className="flex-1 min-h-0 mt-3">
                <ScrollArea className="h-full">
                  <MarkdownContent content={role.soul_content} />
                </ScrollArea>
              </TabsContent>
              <TabsContent value="agents" className="flex-1 min-h-0 mt-3">
                <ScrollArea className="h-full">
                  <MarkdownContent content={role.agents_content} />
                </ScrollArea>
              </TabsContent>
            </Tabs>

            {/* 安装说明 */}
            <div className="px-6 pb-6 pt-3 border-t mt-auto">
              <InstallGuide role={role} />
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  )
}
