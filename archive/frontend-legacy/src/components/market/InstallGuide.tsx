import { useMemo } from 'react'
import { Check, Copy, Terminal } from 'lucide-react'
import { toast } from 'sonner'
import type { MarketRoleDetail } from '@/types'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'

interface InstallGuideProps {
  role: MarketRoleDetail
}

function buildDownloadUrl(roleId: string) {
  const origin = window.location.origin.replace(/\/$/, '')
  return `${origin}/api/v1/market/roles/${roleId}/download`
}

function buildLocalZipPath(roleId: string) {
  return `~/Downloads/${roleId}.zip`
}


function buildOpenClawPrompt(role: MarketRoleDetail) {
  const agentId = role.id
  const zipUrl = buildDownloadUrl(role.id)
  const localZipPath = buildLocalZipPath(role.id)
  const workspacePath = `~/.openclaw/workspace`

  return `## 安装角色包

我需要你帮我安装一个角色包到 Openclaw workspace。

### 角色包信息

- Agent ID: \`${agentId}\`
- URL: \`${zipUrl}\`
- 本地路径（如果我已经下载完成）: \`${localZipPath}\`

### 安装步骤

\`\`\`bash
# 下载角色包（如果需要）
curl -L -o /tmp/${agentId}.zip "${zipUrl}"

# 解压并覆盖到 workspace 目录（-j 忽略 zip 内子目录，直接平铺）
unzip -o -j /tmp/${agentId}.zip -d ${workspacePath}
\`\`\`

解压完成后直接生效，无需额外配置。

如果 URL 无法直接访问，请改用我本地已下载的 zip 文件路径继续安装。`
}

export function InstallGuide({ role }: InstallGuideProps) {
  const { copiedKey, copy } = useCopyToClipboard(2000)

  const prompt = useMemo(() => buildOpenClawPrompt(role), [role])

  async function handleCopyPrompt() {
    const ok = await copy(prompt, role.id)
    if (ok) {
      toast.success('Openclaw 提示词已复制')
    } else {
      toast.error('复制失败，请手动选择复制')
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col rounded-lg border">
      <div className="flex items-center gap-2 border-b px-4 py-3 text-sm font-medium">
        <span className="flex items-center gap-2">
          <Terminal className="size-4 text-muted-foreground" />
          安装说明
        </span>
      </div>
      <div className="flex min-h-0 flex-1 flex-col px-4 py-4">
        <div className="space-y-2">
          <p className="text-sm text-foreground">复制下面这段提示词到 Openclaw。</p>
          <p className="text-xs text-muted-foreground">
            提示词里已经带上当前角色的 ID、名称、部门、描述和角色包地址，Openclaw 可直接继续安装与注册。
          </p>
        </div>

        <div className="mt-4 flex items-center justify-between gap-3 rounded-md border bg-muted/30 px-3 py-2">
          <div className="min-w-0 text-xs text-muted-foreground">
            当前角色：<span className="font-mono text-foreground">{role.id}</span>
          </div>
          <Button onClick={handleCopyPrompt} size="sm" variant="outline">
            {copiedKey === role.id ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
            {copiedKey === role.id ? '已复制' : '复制提示词'}
          </Button>
        </div>

        <div className="mt-4 min-h-0 flex-1 overflow-auto rounded-lg bg-muted">
          <pre className="px-3 py-3 text-xs leading-5 text-foreground whitespace-pre-wrap wrap-break-word">
            {prompt}
          </pre>
        </div>

        <p className="mt-4 text-xs text-muted-foreground">
          如果你已经在当前页面下载了角色包，可以把提示词中的本地路径改成实际下载位置后再发给 Openclaw。
        </p>
      </div>
    </div>
  )
}
