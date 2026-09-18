import type { DesktopReleaseMetadata } from '@/types'

/**
 * 桌面端发行版上传前的**配对校验**（纯函数，便于单测）。
 *
 * 方案：docs/desktop-app-update-plan-2026-09-18.md §5。
 *
 * 为什么值得单独抽出来：「拿 0.3.0 的安装包配 0.4.0 的 release.json」是这套流程里
 * 最容易犯、后果最重的错 —— 一旦发布，所有客户端会照最新版的版本号去下一个版本号
 * 对不上的包，按 sha512 校验失败后**静默装不上**（electron-updater 不会把这种错误
 * 显式报给用户）。服务端也会挡（见 store.CreateDesktopRelease 的文件名/版本一致性
 * 与 sha512 重算），但那时 87MB 已经白传完了。
 */

export interface UploadPairValidation {
  ok: boolean
  /** 失败原因，直接展示给用户 —— 必须可行动（说清是哪一个文件不对）。 */
  reason?: string
}

export type DesktopReleaseMetadataLike = Pick<
  DesktopReleaseMetadata,
  'version' | 'file' | 'sha512'
>

export function validateUploadPair(
  installerName: string,
  meta: DesktopReleaseMetadataLike,
): UploadPairValidation {
  if (!meta.version) {
    return { ok: false, reason: 'release.json 缺少 version' }
  }
  if (!meta.sha512) {
    return { ok: false, reason: 'release.json 缺少 sha512' }
  }
  if (meta.file && meta.file !== installerName) {
    return {
      ok: false,
      reason: `release.json 的 file（${meta.file}）与所选安装包（${installerName}）不一致`,
    }
  }
  if (!installerName.includes(meta.version)) {
    return {
      ok: false,
      reason: `安装包文件名里不含版本号 ${meta.version}，请确认没有选错包`,
    }
  }
  return { ok: true }
}
