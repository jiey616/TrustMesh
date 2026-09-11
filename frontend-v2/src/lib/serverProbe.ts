import type { ProbeOutcome } from '@/types/desktop'
import { isDesktopShell } from '@/stores/serverConfigStore'

export type ProbeLevel = 'ok' | 'warn' | 'error'

export interface ProbeSummary {
  level: ProbeLevel
  /** 面向用户的结论文案 */
  message: string
}

const DEFAULT_TIMEOUT = 6000

/**
 * 把底层错误（Chromium net 错误码 / fetch 失败）翻译成人话。
 * 证书类问题要给出可操作指引，否则用户只会看到「无法连接」。
 */
function describeError(error: string | null): ProbeSummary {
  if (!error) return { level: 'error', message: '连接失败，原因未知' }

  const code = error.toUpperCase()
  if (code.includes('CERT') || code.includes('SSL') || code.includes('TLS')) {
    return {
      level: 'error',
      message: `证书不受信任（${error}）：证书为自签名、过期或域名不匹配。若是内网自签名，请勾选下方「信任自签名证书」后重试。`,
    }
  }
  if (code.includes('ENOTFOUND') || code.includes('EAI_AGAIN') || code.includes('NAME_NOT_RESOLVED')) {
    return { level: 'error', message: `域名解析失败（${error}）：请检查主机名是否正确、DNS 是否可达。` }
  }
  if (code.includes('CONNECTION_REFUSED') || code.includes('ERR_CONNECTION_REFUSED')) {
    return { level: 'error', message: `连接被拒绝（${error}）：主机可达但端口未监听，请检查后端是否启动、端口是否正确。` }
  }
  if (code.includes('TIMEOUT') || code.includes('TIMEDOUT')) {
    return { level: 'error', message: `连接超时（${error}）：请检查网络连通性与防火墙策略。` }
  }
  if (code.includes('INVALID_URL') || code.includes('ERR_INVALID_URL')) {
    return { level: 'error', message: '地址格式不正确，请输入 http(s)://主机[:端口] 形式。' }
  }
  return { level: 'error', message: `无法连接（${error}）：请检查地址、网络与证书。` }
}

/** 浏览器环境回退实现（拿不到细粒度错误，只区分超时/失败） */
async function probeViaFetch(baseUrl: string): Promise<ProbeOutcome> {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), DEFAULT_TIMEOUT)
  try {
    const res = await fetch(`${baseUrl.replace(/\/+$/, '')}/api/v1/platform/info`, {
      signal: controller.signal,
    })
    return { ok: res.ok, status: res.status, error: null }
  } catch {
    return { ok: false, status: 0, error: 'ERR_CONNECTION_FAILED' }
  } finally {
    window.clearTimeout(timer)
  }
}

/**
 * 探测服务端连通性并给出可读结论。
 * 桌面环境走主进程 net.request（能拿回 Chromium 错误码），浏览器退化为 fetch。
 */
export async function probeServer(baseUrl: string): Promise<ProbeSummary> {
  let outcome: ProbeOutcome
  if (isDesktopShell()) {
    try {
      outcome = await window.desktop!.probeServer(baseUrl, DEFAULT_TIMEOUT)
    } catch {
      outcome = { ok: false, status: 0, error: 'ERR_IPC_FAILED' }
    }
  } else {
    outcome = await probeViaFetch(baseUrl)
  }

  if (outcome.ok) {
    return { level: 'ok', message: `连接成功：${baseUrl.replace(/\/+$/, '')}` }
  }
  if (outcome.status > 0) {
    return {
      level: 'warn',
      message: `服务器有响应，但状态码为 ${outcome.status}（可能路径不对或未授权）。`,
    }
  }
  return describeError(outcome.error)
}
