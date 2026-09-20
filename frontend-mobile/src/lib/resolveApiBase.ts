/**
 * API 根地址裁决（纯函数，便于穷举测试）。
 *
 * 优先级：**用户手动配置 > 环境变量 > 原生壳默认 > 同源相对地址**。
 *
 * - 手动配置最高：现场换服务器时不可能重新打包（环境变量会被烘焙进产物）。
 * - 原生壳默认：Capacitor webview 的 origin 是 `https://localhost`，同源相对地址
 *   会打到壳自己身上 ⇒ 原生环境兜底指向生产服务器（MePage 仍可手改覆盖）。
 * - 默认同源 `/api/v1/`：生产把移动端产物挂在后端同域 nginx 下即可零配置；
 *   本机开发由 Vite 代理转发（见 vite.config.ts，代理侧跳过自签证书校验）。
 *
 * 返回值**必须以 `/` 结尾**（ky 的 prefixUrl 要求），且调用路径不能以 `/` 开头。
 */
export interface ApiBaseInput {
  /** 用户在「我的 → 服务器地址」里手填的值 */
  override?: string | null
  /** 构建期注入的环境变量 */
  envBase?: string | null
  /** 原生壳兜底默认值（web/PWA 传 null，走同源） */
  nativeDefault?: string | null
}

export const DEFAULT_API_BASE = '/api/v1/'
export const DEFAULT_NATIVE_API_BASE = 'https://175.27.135.91/api/v1/'

export function resolveApiBase(input: ApiBaseInput = {}): string {
  const override = normalize(input.override)
  if (override) return override
  const env = normalize(input.envBase)
  if (env) return env
  const native = normalize(input.nativeDefault)
  if (native) return native
  return DEFAULT_API_BASE
}

function normalize(raw: string | null | undefined): string {
  if (!raw) return ''
  const trimmed = raw.trim().replace(/\/+$/, '')
  return trimmed ? `${trimmed}/` : ''
}
