import { Copy, Check } from 'lucide-react'
import { useClawSynapseHealth } from '@/hooks/useClawSynapse'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'

function truncateMiddle(str: string, maxLen: number) {
  if (str.length <= maxLen) return str
  const side = Math.floor((maxLen - 3) / 2)
  return str.slice(0, side) + '...' + str.slice(-side)
}

function CopyIcon({ value }: { value: string }) {
  const { copied, copy } = useCopyToClipboard()

  return (
    <button
      type="button"
      onClick={() => copy(value)}
      className="inline-flex items-center text-muted-foreground/50 hover:text-foreground transition-colors cursor-pointer"
      title="复制"
    >
      {copied ? <Check className="size-3" /> : <Copy className="size-3" />}
    </button>
  )
}

export function NodeStatusIndicator() {
  const { data, isLoading } = useClawSynapseHealth()

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <span className="h-2 w-2 rounded-full bg-muted" />
        <span>检测中...</span>
      </div>
    )
  }

  const online = data?.online ?? false
  const nodeId = data?.node_id
  const did = data?.did
  const trustMode = data?.trust_mode

  return (
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <span className="relative shrink-0" style={{ width: 8, height: 8 }}>
        {online && (
          <span className="absolute inset-0 animate-ping rounded-full bg-green-400 opacity-75" />
        )}
        <span
          className={`absolute inset-0 rounded-full ${online ? 'bg-green-500' : 'bg-red-500'}`}
        />
      </span>

      {online && data ? (
        <>
          {nodeId ? (
            <>
              <span className="font-mono" title={nodeId}>
                {truncateMiddle(nodeId, 20)}
              </span>
              <CopyIcon value={nodeId} />
            </>
          ) : (
            <span className="font-medium">节点在线</span>
          )}

          {did && (
            <>
              <span className="text-border">|</span>
              <span className="font-mono hidden sm:inline" title={did}>
                {truncateMiddle(did, 24)}
              </span>
              <span className="hidden sm:inline">
                <CopyIcon value={did} />
              </span>
            </>
          )}

          {trustMode && (
            <>
              <span className="text-border hidden sm:inline">|</span>
              <span className="font-medium uppercase">{trustMode}</span>
            </>
          )}
        </>
      ) : (
        <span className="text-destructive">节点离线</span>
      )}
    </div>
  )
}
