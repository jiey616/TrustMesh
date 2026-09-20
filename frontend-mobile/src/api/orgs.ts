import { apiClient } from './client'
import type { ApiListResponse, OrgView } from '@/types'

/**
 * 列举当前用户所属的全部组织。
 *
 * 🔴 该接口本身也是"当前空间"相关的：排查"任务看不到"时，必须先枚举组织、
 * 再逐个带 X-Org-Id 重查（不带该头的列表接口只返回个人空间，实测 9 vs 11 条）。
 */
export async function listOrganizations(): Promise<OrgView[]> {
  const res = await apiClient.get('organizations').json<ApiListResponse<OrgView>>()
  return res.data.items ?? []
}
