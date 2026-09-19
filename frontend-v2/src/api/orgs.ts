import { apiClient } from './client'
import type {
  ApiResponse,
  ApiListResponse,
  OrgView,
  OrgMemberView,
  CreateOrgRequest,
  AddOrgMemberRequest,
} from '@/types'

// ─── 多租户组织 API（阶段 4-A 后端：organizations） ───

export async function listOrganizations() {
  return apiClient.get('organizations').json<ApiListResponse<OrgView>>()
}

export async function createOrganization(input: CreateOrgRequest) {
  return apiClient.post('organizations', { json: input }).json<ApiResponse<OrgView>>()
}

export async function getOrganization(id: string) {
  return apiClient.get(`organizations/${id}`).json<ApiResponse<OrgView>>()
}

export async function listOrgMembers(id: string) {
  return apiClient
    .get(`organizations/${id}/members`)
    .json<ApiListResponse<OrgMemberView>>()
}

export async function addOrgMember(id: string, input: AddOrgMemberRequest) {
  return apiClient
    .post(`organizations/${id}/members`, { json: input })
    .json<ApiResponse<OrgMemberView>>()
}

/** 改成员角色：传 role_id（自定义角色优先）或内置角色串 role。 */
export async function updateOrgMemberRole(
  id: string,
  userId: string,
  ref: { role?: string; role_id?: string },
) {
  return apiClient
    .patch(`organizations/${id}/members/${userId}`, { json: ref })
    .json<ApiResponse<OrgMemberView>>()
}

export async function removeOrgMember(id: string, userId: string) {
  return apiClient
    .delete(`organizations/${id}/members/${userId}`)
    .json<ApiResponse<{ removed: string }>>()
}

/** 企业级菜单覆盖（只能缩小）：被点名的菜单键对全员隐藏（需 org.settings）。 */
export async function updateOrgMenuOverrides(id: string, menuOverrides: string[]) {
  return apiClient
    .patch(`organizations/${id}/menu-overrides`, { json: { menu_overrides: menuOverrides } })
    .json<ApiResponse<OrgView>>()
}

/** 更新组织资料（名称 / 简称），需 owner 或 admin。 */
export async function updateOrganizationProfile(id: string, input: { name: string; short_name?: string }) {
  return apiClient.patch(`organizations/${id}`, { json: input }).json<ApiResponse<OrgView>>()
}

/** 上传组织 logo（multipart 字段 file），需 owner 或 admin。返回含 logo_url 的最新 OrgView。 */
export async function uploadOrganizationLogo(id: string, file: File) {
  const form = new FormData()
  form.append('file', file)
  return apiClient
    .post(`organizations/${id}/logo`, { body: form })
    .json<ApiResponse<OrgView>>()
}

/**
 * 取组织 logo 字节（带鉴权），供 `<img>` 通过 object URL 显示。
 *
 * 🔴 这条路径的存在本身就是必须的：`GET /organizations/:id/logo` 注册在**鉴权路由组**上
 * （router.go 的 `orgs` 组挂了 middleware.RequireAuth），而 `<img src>` / antd `<Avatar src>`
 * 走浏览器原生图片加载，**无法附加 Authorization 头** ⇒ 把 `logo_url` 直接交给 `src`
 * 必然 401，表现为「提示已更新但图标不显示」。所以只能经 apiClient（自动带
 * Authorization + X-Org-Id，并复用 401 刷新重放）取回 blob，再转 object URL。
 * 显示侧统一走 `hooks/useOrgLogo.ts` 的 useOrgLogoObjectUrl，不要在组件里自己拼。
 *
 * `cache: 'no-store'`：后端该响应带 `Cache-Control: private, max-age=3600`，而 logo 的 URL
 * 在重复上传后**不变**（不含版本号）⇒ 若走浏览器 HTTP 缓存，重新上传后最长 1 小时仍显示旧图，
 * 又是一次「上传了却没生效」。故显式禁用缓存（图片很小，代价可忽略）。
 */
export async function fetchOrganizationLogo(id: string): Promise<Blob> {
  return apiClient.get(`organizations/${id}/logo`, { cache: 'no-store' }).blob()
}
