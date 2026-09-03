import { Tooltip } from 'antd'
import { CheckOutlined, CopyOutlined } from '@ant-design/icons'
import { useClawSynapseHealth } from '@/hooks/useClawSynapse'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'

function truncateMiddle(str: string, maxLen: number) {
  if (!str || str.length <= maxLen) return str
  const side = Math.floor((maxLen - 3) / 2)
  return str.slice(0, side) + '...' + str.slice(-side)
}

function CopyIcon({ value }: { value: string }) {
  const { copied, copy } = useCopyToClipboard()
  return (
    <Tooltip title={copied ? '已复制' : '复制'}>
      <span
        role="button"
        onClick={(e) => {
          e.stopPropagation()
          copy(value)
        }}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          cursor: 'pointer',
          color: copied ? '#22d3ee' : 'rgba(255,255,255,0.4)',
          fontSize: 12,
          marginLeft: 4,
        }}
      >
        {copied ? <CheckOutlined /> : <CopyOutlined />}
      </span>
    </Tooltip>
  )
}

export function NodeStatusIndicator() {
  const { data, isLoading } = useClawSynapseHealth()

  if (isLoading) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: 'var(--text-tertiary)' }}>
        <span style={{ width: 8, height: 8, borderRadius: 'var(--radius-avatar)', background: 'rgba(255,255,255,0.2)' }} />
        <span>检测中...</span>
      </div>
    )
  }

  const online = data?.online ?? false
  const nodeId = data?.node_id
  const did = data?.did
  const trustMode = data?.trust_mode

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: 'var(--text-tertiary)', flexWrap: 'wrap' }}>
      <span style={{ position: 'relative', display: 'inline-block', width: 8, height: 8, flexShrink: 0 }}>
        {online && (
          <span
            style={{
              position: 'absolute',
              inset: 0,
              borderRadius: 'var(--radius-avatar)',
              background: 'var(--success)',
              opacity: 0.6,
              animation: 'nodePing 1.5s ease-out infinite',
            }}
          />
        )}
        <span
          style={{
            position: 'absolute',
            inset: 0,
            borderRadius: 'var(--radius-avatar)',
            background: online ? '#22c55e' : '#ef4444',
          }}
        />
      </span>

      {online && data ? (
        <>
          {nodeId ? (
            <span style={{ display: 'inline-flex', alignItems: 'center', fontFamily: 'JetBrains Mono, monospace' }}>
              <span title={nodeId}>{truncateMiddle(nodeId, 20)}</span>
              <CopyIcon value={nodeId} />
            </span>
          ) : (
            <span style={{ fontWeight: 500 }}>节点在线</span>
          )}

          {did && (
            <>
              <span style={{ color: 'var(--text-quaternary)' }}>|</span>
              <span style={{ display: 'inline-flex', alignItems: 'center', fontFamily: 'JetBrains Mono, monospace' }} title={did}>
                {truncateMiddle(did, 24)}
                <CopyIcon value={did} />
              </span>
            </>
          )}

          {trustMode && (
            <>
              <span style={{ color: 'var(--text-quaternary)' }}>|</span>
              <span style={{ fontWeight: 500, textTransform: 'uppercase' }}>{trustMode}</span>
            </>
          )}
        </>
      ) : (
        <span style={{ color: 'var(--error)' }}>节点离线</span>
      )}

      <style>{`
        @keyframes nodePing {
          0% { transform: scale(1); opacity: 0.6; }
          75%, 100% { transform: scale(2.4); opacity: 0; }
        }
      `}</style>
    </div>
  )
}
