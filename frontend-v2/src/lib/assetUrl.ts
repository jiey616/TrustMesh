import { getApiBase } from '@/stores/serverConfigStore'

/** 已带协议（http:/https:/data:/blob:/…）或协议相对（`//cdn…`）的地址。 */
const ABSOLUTE = /^(?:[a-z][a-z0-9+.-]*:|\/\/)/i

/**
 * 把「服务端返回的相对地址」解析成当前运行环境真正可加载的地址。
 *
 * 🔴 为什么必须有这个函数（桌面版手册图片全裂的根因）：
 *   手册配图入库时，后端存的是**根相对路径** `/api/v1/manual/images/{id}`
 *   （`backend/internal/store/store_platform_manual.go` 的 `URL: "/api/v1/manual/images/" + id`）。
 *   - Web 端：页面 origin 就是服务端，`<img src="/api/v1/...">` 正确解析 ⇒ 正常显示；
 *   - **桌面端：页面 origin 是 `file://`**（主进程 `win.loadFile(dist-v2/index.html)`），
 *     根相对路径被解析成 `file:///api/v1/manual/images/{id}` ⇒ 一律裂图。
 *
 * 规则：
 *   - 已是绝对/协议相对地址（含管理员手填的外链、`data:`、`blob:`）⇒ 原样返回；
 *   - 其余（`/xxx` 或 `xxx`）⇒ 前面补 API 的 **origin**。
 *     origin 由 `getApiBase()` 去掉结尾的 `/api/v1` 得到：
 *     · Web（容器 build 注入了 `VITE_API_BASE_URL=/api/v1/`）⇒ origin 为空串
 *       ⇒ 结果仍是根相对路径，**同源行为完全不变**；
 *     · 桌面端 ⇒ `getApiBase()` 是 `https://host/api/v1/` ⇒ 补成 `https://host/api/v1/...`。
 *
 * ⚠️ 在**调用时**读 `getApiBase()`（不是模块加载时），这样设置页改服务端地址后不必等刷新。
 *    `apiBase` 参数只为单测注入（默认就是 `getApiBase()`），生产调用一律不传第二参。
 */
export function resolveAssetUrl(url: string | null | undefined, apiBase: string = getApiBase()): string {
  const raw = (url ?? '').trim()
  if (!raw) return ''
  if (ABSOLUTE.test(raw)) return raw
  const origin = apiBase.replace(/\/api\/v1\/?$/, '').replace(/\/+$/, '')
  return `${origin}/${raw.replace(/^\/+/, '')}`
}
