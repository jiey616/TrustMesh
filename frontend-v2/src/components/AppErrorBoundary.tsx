import { Component, type ErrorInfo, type ReactNode } from 'react'
import { Button, Result } from 'antd'

interface Props {
  children: ReactNode
}

interface State {
  error: Error | null
}

export class AppErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[ErrorBoundary]', error, info)
  }

  handleReload = () => {
    this.setState({ error: null })
    window.location.reload()
  }

  render() {
    if (this.state.error) {
      const err = this.state.error
      return (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: '100vh', padding: 24 }}>
          <Result
            status="error"
            title="页面加载出错"
            subTitle={err.message}
            extra={
              <Button type="primary" onClick={this.handleReload}>
                刷新页面
              </Button>
            }
          >
            {err.stack && (
              <pre
                style={{
                  marginTop: 12,
                  maxWidth: 720,
                  maxHeight: 240,
                  overflow: 'auto',
                  textAlign: 'left',
                  fontSize: 12,
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word',
                  color: 'rgba(255,255,255,0.6)',
                  background: 'rgba(255,255,255,0.05)',
                  borderRadius: 8,
                  padding: 12,
                }}
              >
                {err.stack}
              </pre>
            )}
          </Result>
        </div>
      )
    }
    return this.props.children
  }
}