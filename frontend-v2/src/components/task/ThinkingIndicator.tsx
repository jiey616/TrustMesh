import { RobotOutlined } from '@ant-design/icons'

/** PM 思考中指示器 */
export function ThinkingIndicator() {
  return (
    <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
      <div
        style={{
          width: 28,
          height: 28,
          borderRadius: 'var(--radius-avatar)',
          flexShrink: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontSize: 13,
          background: 'linear-gradient(135deg, #f59e0b, #f43f5e)',
          color: 'var(--text-primary)',
        }}
      >
        <RobotOutlined />
      </div>
      <div style={{ display: 'flex', gap: 4, padding: '10px 14px', borderRadius: 'var(--radius-structure)', background: 'var(--surface-raised)', border: '1px solid var(--line)' }}>
        {[0, 1, 2].map((i) => (
          <span
            key={i}
            style={{
              width: 5,
              height: 5,
              borderRadius: 'var(--radius-avatar)',
              background: 'rgba(255,255,255,0.4)',
              animation: `tm-blink 1.2s infinite ${i * 0.2}s`,
            }}
          />
        ))}
        <style>{`@keyframes tm-blink { 0%, 100% { opacity: 0.3 } 50% { opacity: 1 } }`}</style>
      </div>
    </div>
  )
}
