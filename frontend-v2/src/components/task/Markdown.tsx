import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

interface MarkdownProps {
  content: string
  /** small 用于事件流/描述等次要文本，normal 用于评论/正文 */
  size?: 'small' | 'normal'
}

const styles: Record<string, React.CSSProperties> = {
  container: {
    lineHeight: 1.7,
    color: 'var(--text-primary)',
    wordBreak: 'break-word',
  },
  containerSmall: {
    lineHeight: 1.6,
    color: 'var(--text-secondary)',
    wordBreak: 'break-word',
    fontSize: 12,
  },
  p: { margin: '0.3em 0' },
  h1: { fontSize: 16, margin: '0.6em 0 0.3em' },
  h2: { fontSize: 15, margin: '0.6em 0 0.3em' },
  h3: { fontSize: 14, margin: '0.5em 0 0.3em' },
  h4: { fontSize: 13, margin: '0.5em 0 0.3em' },
  ul: { margin: '0.3em 0', paddingLeft: 20 },
  ol: { margin: '0.3em 0', paddingLeft: 20 },
  li: { margin: '0.15em 0' },
  pre: {
    background: 'var(--surface-inset)',
    border: '1px solid var(--line)',
    borderRadius: 'var(--radius-control)',
    padding: '10px 12px',
    overflowX: 'auto',
    fontSize: 12,
    lineHeight: 1.5,
    margin: '0.5em 0',
  },
  code: {
    background: 'var(--surface-raised)',
    borderRadius: 'var(--radius-control)',
    padding: '1px 5px',
    fontSize: '0.92em',
    fontFamily: 'Consolas, "Courier New", monospace',
  },
  blockquote: {
    borderLeft: '3px solid rgba(109,95,245,0.4)',
    margin: '0.4em 0',
    paddingLeft: 12,
    color: 'var(--text-secondary)',
  },
  a: { color: 'var(--cyan)' },
  hr: { border: 'none', borderTop: '1px solid var(--line-strong)', margin: '0.6em 0' },
  table: { borderCollapse: 'collapse', margin: '0.5em 0', fontSize: 12 },
  th: { border: '1px solid var(--line-strong)', padding: '4px 10px', textAlign: 'left' },
  td: { border: '1px solid var(--line-strong)', padding: '4px 10px' },
  img: { maxWidth: '100%', borderRadius: 'var(--radius-control)' },
  input: { marginRight: 4 },
}

function CodeBlock({ className, children }: { className?: string; children?: React.ReactNode }) {
  const match = /language-(\w+)/.exec(className || '')
  if (!match) {
    return <code style={styles.code}>{children}</code>
  }
  return (
    <pre style={styles.pre}>
      <code>{children}</code>
    </pre>
  )
}

export function Markdown({ content, size = 'normal' }: MarkdownProps) {
  return (
    <div style={size === 'small' ? styles.containerSmall : styles.container}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          p: ({ children }) => <p style={styles.p}>{children}</p>,
          h1: ({ children }) => <h1 style={styles.h1}>{children}</h1>,
          h2: ({ children }) => <h2 style={styles.h2}>{children}</h2>,
          h3: ({ children }) => <h3 style={styles.h3}>{children}</h3>,
          h4: ({ children }) => <h4 style={styles.h4}>{children}</h4>,
          ul: ({ children }) => <ul style={styles.ul}>{children}</ul>,
          ol: ({ children }) => <ol style={styles.ol}>{children}</ol>,
          li: ({ children }) => <li style={styles.li}>{children}</li>,
          pre: ({ children }) => <pre style={styles.pre}>{children}</pre>,
          code: ({ className, children }) => (
            <CodeBlock className={className}>{children}</CodeBlock>
          ),
          blockquote: ({ children }) => <blockquote style={styles.blockquote}>{children}</blockquote>,
          a: ({ children, href }) => <a style={styles.a} href={href} target="_blank" rel="noreferrer">{children}</a>,
          hr: () => <hr style={styles.hr} />,
          table: ({ children }) => <table style={styles.table}>{children}</table>,
          th: ({ children }) => <th style={styles.th}>{children}</th>,
          td: ({ children }) => <td style={styles.td}>{children}</td>,
          img: ({ src, alt }) => <img src={src} alt={alt} style={styles.img} />,
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  )
}
