import { apiClient } from './client'
import type { ApiResponse } from '@/types'

// ─── 平台「操作手册」（结构化单篇文档）───
//
// 与 platform_guides（整篇 HTML，沙箱 iframe 渲染）并存但定位不同：
// platform_manual 存结构化章节树，前端用平台自身的 antd 组件渲染，
// 视觉与平台一致，内容由管理员在界面内**表单化编辑**（无需懂 HTML）。
//
// 阅读（GET /manual）是登录基础权限；管理三动作（GET/PUT /platform/manual、
// POST /platform/manual/images）复用权限点 platform.guide.mgr。
//
// 🔴 配图不内联：image block 只存 URL（后端 /api/v1/manual/images/{id} 提供），
// 避免单文档逼近 BSON 16MiB 硬顶。上传走 uploadManualImage 拿 URL 再写入 block。

/** 一个内容块的类型。未知类型前端一律跳过渲染、不报错（向前兼容）。 */
export type ManualBlockType =
  | 'paragraph'
  | 'heading'
  | 'steps'
  | 'tip'
  | 'warn'
  | 'image'
  | 'table'
  | 'flow'
  | 'qa'

export interface ManualQA {
  q: string
  a: string
}

/** 内容块的统一数据体。各 type 只用其中若干字段（与后端 ManualBlockData 一一对应）。 */
export interface ManualBlockData {
  text?: string
  items?: string[]
  qa_pairs?: ManualQA[]
  url?: string
  alt?: string
  caption?: string
  head?: string[]
  rows?: string[][]
}

export interface ManualBlock {
  type: ManualBlockType | string
  data: ManualBlockData
}

export interface ManualSection {
  /** 稳定锚点标识（章节重排不变，滚动定位与编辑定位都用它）。 */
  id: string
  title: string
  /** 章节导语（可选，一句话概述）。 */
  lead?: string
  blocks: ManualBlock[]
}

export interface PlatformManual {
  id: string
  title: string
  subtitle?: string
  footer?: string
  sections: ManualSection[]
  updated_by?: string
  updated_at?: string
}

export interface PlatformManualMeta {
  title: string
  subtitle?: string
  sections: number
  updated_by?: string
  updated_at?: string
}

/** 手册保存载荷（无 id / 审计字段，由后端补）。 */
export interface PlatformManualInput {
  title: string
  subtitle?: string
  footer?: string
  sections: ManualSection[]
}

export interface ManualImageAsset {
  id: string
  file_name: string
  size: number
  content_type: string
  url: string
  uploaded_by?: string
  uploaded_at?: string
}

export async function getManual() {
  return apiClient.get('manual').json<ApiResponse<{ manual: PlatformManual | null }>>()
}

export async function getPlatformManual() {
  return apiClient
    .get('platform/manual')
    .json<ApiResponse<{ manual: PlatformManual | null; meta?: PlatformManualMeta | null }>>()
}

export async function savePlatformManual(input: PlatformManualInput) {
  return apiClient
    .put('platform/manual', { json: input })
    .json<ApiResponse<{ manual: PlatformManual; meta?: PlatformManualMeta }>>()
}

/** 上传一张配图，返回可写入 image block 的 URL。 */
export async function uploadManualImage(file: File) {
  const form = new FormData()
  form.append('file', file)
  return apiClient
    .post('platform/manual/images', { body: form })
    .json<ApiResponse<{ image: ManualImageAsset }>>()
}
