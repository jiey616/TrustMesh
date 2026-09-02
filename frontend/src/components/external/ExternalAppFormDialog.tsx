import { useState, type ReactNode } from 'react'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { toast } from 'sonner'
import { ApiRequestError } from '@/api/client'
import { useCreateExternalApp, useUpdateExternalApp } from '@/hooks/useExternalApps'
import type { ExternalAppView } from '@/types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  // 传入则为编辑模式，否则为新增模式
  app?: ExternalAppView | null
}

const SSO_TYPE_OPTIONS: { value: 'trustmesh_jwt'; label: string }[] = [
  { value: 'trustmesh_jwt', label: 'TrustMesh 签发 JWT（推荐）' },
]

const FRAME_MODE_OPTIONS: { value: 'newtab' | 'iframe'; label: string }[] = [
  { value: 'newtab', label: '新标签页打开' },
  { value: 'iframe', label: '内嵌 iframe（需外部平台支持）' },
]

export function ExternalAppFormDialog({ open, onOpenChange, app }: Props) {
  const isEdit = !!app
  const createApp = useCreateExternalApp()
  const updateApp = useUpdateExternalApp()

  const [name, setName] = useState(app?.name ?? '')
  const [baseUrl, setBaseUrl] = useState(app?.base_url ?? '')
  const [clientId, setClientId] = useState(app?.client_id ?? '')
  const [ssoType, setSsoType] = useState<'trustmesh_jwt'>(app?.sso_type ?? 'trustmesh_jwt')
  const [frameMode, setFrameMode] = useState<'newtab' | 'iframe'>(app?.frame_mode ?? 'newtab')
  const [scopes, setScopes] = useState(app?.scopes ?? '')
  const [error, setError] = useState('')
  const [revealedSecret, setRevealedSecret] = useState<string | null>(null)

  // 当对话框打开/切换目标时重置表单
  const resetForm = (nextApp?: ExternalAppView | null) => {
    setName(nextApp?.name ?? '')
    setBaseUrl(nextApp?.base_url ?? '')
    setClientId(nextApp?.client_id ?? '')
    setSsoType(nextApp?.sso_type ?? 'trustmesh_jwt')
    setFrameMode(nextApp?.frame_mode ?? 'newtab')
    setScopes(nextApp?.scopes ?? '')
    setError('')
    setRevealedSecret(null)
  }

  const handleOpenChange = (next: boolean) => {
    if (!next) resetForm(app)
    onOpenChange(next)
  }

  const copySecret = async (secret: string) => {
    try {
      await navigator.clipboard.writeText(secret)
      toast.success('client_secret 已复制')
    } catch {
      toast.error('复制失败，请手动选择复制')
    }
  }

  const handleSubmit = async () => {
    setError('')
    const payload = {
      name: name.trim(),
      base_url: baseUrl.trim(),
      client_id: clientId.trim(),
      sso_type: ssoType,
      frame_mode: frameMode,
      scopes: scopes.trim(),
    }
    if (!payload.name || !payload.base_url || !payload.client_id) {
      setError('名称、基础 URL、Client ID 均为必填')
      return
    }
    try {
      if (isEdit && app) {
        await updateApp.mutateAsync({ id: app.id, input: payload })
        toast.success('外部平台已更新')
        handleOpenChange(false)
      } else {
        const res = await createApp.mutateAsync(payload)
        // client_secret 仅返回一次，立即展示并要求用户保存
        setRevealedSecret(res.data.client_secret)
        toast.success('外部平台已创建')
      }
    } catch (err) {
      const message = err instanceof ApiRequestError ? err.message : '保存失败'
      setError(message)
    }
  }

  const pending = createApp.isPending || updateApp.isPending

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? '编辑外部平台' : '新增外部平台'}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? '修改该平台的连接配置。'
              : '登记一个可通过 SSO 免密打开的外部平台。创建后会生成 client_secret，请妥善保存。'}
          </DialogDescription>
        </DialogHeader>

        {revealedSecret ? (
          <SecretReveal
            secret={revealedSecret}
            appName={name}
            onCopy={() => copySecret(revealedSecret)}
            onDone={() => handleOpenChange(false)}
          />
        ) : (
          <div className="flex flex-col gap-4">
            <Field label="平台名称" required>
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="例如：画宗 AIGC 工厂" />
            </Field>
            <Field label="基础 URL" required>
              <Input
                value={baseUrl}
                onChange={(e) => setBaseUrl(e.target.value)}
                placeholder="https://external.example.com/sso/entry"
              />
            </Field>
            <Field label="Client ID" required>
              <Input
                value={clientId}
                onChange={(e) => setClientId(e.target.value)}
                placeholder="外部平台分配给 TrustMesh 的标识"
              />
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field label="SSO 类型">
                <Select value={ssoType} onValueChange={(v) => setSsoType(v as 'trustmesh_jwt')}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {SSO_TYPE_OPTIONS.map((o) => (
                      <SelectItem key={o.value} value={o.value}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field label="打开方式">
                <Select value={frameMode} onValueChange={(v) => setFrameMode(v as 'newtab' | 'iframe')}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {FRAME_MODE_OPTIONS.map((o) => (
                      <SelectItem key={o.value} value={o.value}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </div>
            <Field label="Scopes（可选）">
              <Textarea
                value={scopes}
                onChange={(e) => setScopes(e.target.value)}
                placeholder="逗号分隔的权限范围，如：read,write"
                rows={2}
              />
            </Field>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
                取消
              </Button>
              <Button type="button" onClick={handleSubmit} disabled={pending}>
                {pending ? '保存中...' : isEdit ? '保存' : '创建'}
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

function Field({ label, required, children }: { label: string; required?: boolean; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2">
      <Label className="text-sm font-medium">
        {label}
        {required && <span className="text-destructive ml-0.5">*</span>}
      </Label>
      {children}
    </div>
  )
}

function SecretReveal({
  secret,
  appName,
  onCopy,
  onDone,
}: {
  secret: string
  appName: string
  onCopy: () => void
  onDone: () => void
}): ReactNode {
  return (
    <div className="flex flex-col gap-4">
      <div className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-400">
        请立即复制并妥善保存 <b>client_secret</b>。该密钥<b>仅显示这一次</b>，关闭后将无法再次查看，需重新创建平台。
      </div>
      <Field label={`${appName} 的 client_secret`}>
        <div className="flex items-center gap-2">
          <Input readOnly value={secret} className="font-mono text-xs" />
          <Button type="button" variant="secondary" onClick={onCopy} className="shrink-0">
            复制
          </Button>
        </div>
      </Field>
      <DialogFooter>
        <Button type="button" onClick={onDone}>
          我已保存，完成
        </Button>
      </DialogFooter>
    </div>
  )
}
