import { apiClient } from './client'
import type { ApiResponse, ChatAttachment } from '@/types'

/**
 * 上传一个聊天附件（字节流走统一 /api/v1/chats/attachments 端点）。
 * 返回后端的 ChatAttachment 元数据（无 url，读取时后端补签名链接）。
 */
export async function uploadChatAttachment(file: File): Promise<ChatAttachment> {
  const form = new FormData()
  form.append('file', file, file.name)
  const res = await apiClient
    .post('chats/attachments', { body: form, timeout: 120_000 })
    .json<ApiResponse<ChatAttachment>>()
  return res.data
}

/**
 * 并行上传多个附件，容忍部分失败：
 * 成功项收集在 ok，失败项逐条留在 failed（附文件名与原因），不整体抛错。
 */
export async function uploadChatAttachments(
  files: File[],
): Promise<{ ok: ChatAttachment[]; failed: { file: File; error: string }[] }> {
  const results = await Promise.allSettled(files.map((file) => uploadChatAttachment(file)))
  const ok: ChatAttachment[] = []
  const failed: { file: File; error: string }[] = []
  results.forEach((result, index) => {
    if (result.status === 'fulfilled') {
      ok.push(result.value)
    } else {
      failed.push({
        file: files[index],
        error: result.reason instanceof Error ? result.reason.message : '上传失败',
      })
    }
  })
  return { ok, failed }
}
