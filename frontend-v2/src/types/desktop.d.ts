/** 主进程连接探测结果（含 Chromium net 错误码） */
export interface ProbeOutcome {
  ok: boolean
  status: number
  error: string | null
}

export {}

declare global {
  interface Window {
    /** Electron 渲染进程中由 preload 注入；浏览器环境为 undefined */
    process?: {
      versions?: {
        electron?: string
      }
    }
    /** 桌面端专属桥接；浏览器 / Web 部署环境为 undefined */
    desktop?: {
      getConfig: () => Promise<{ trustInsecureTls?: boolean }>
      setTrustInsecureTls: (enabled: boolean) => Promise<{ trustInsecureTls?: boolean }>
      probeServer: (baseUrl: string, timeoutMs?: number) => Promise<ProbeOutcome>
    }
  }
}
