import { api } from './client'
import type { ApiResponse, ChatAttachment } from '@/types'

// Upload a local file as a chat attachment (数字员工对话 / 任务对话共用).
// Returns the attachment metadata the UI echoes back when sending a message.
export async function uploadChatAttachment(file: File) {
  const form = new FormData()
  form.append('file', file, file.name)
  return api
    .post('chats/attachments', {
      body: form,
      timeout: 120_000,
    })
    .json<ApiResponse<ChatAttachment>>()
}

// Upload multiple files, tolerating partial failure (returns { ok, failed }).
export async function uploadChatAttachments(files: File[]): Promise<{ ok: ChatAttachment[]; failed: string[] }> {
  const ok: ChatAttachment[] = []
  const failed: string[] = []
  await Promise.all(
    files.map(async (file) => {
      try {
        const res = await uploadChatAttachment(file)
        ok.push(res.data)
      } catch {
        failed.push(file.name)
      }
    }),
  )
  return { ok, failed }
}
