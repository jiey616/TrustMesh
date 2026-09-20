import { useEffect, useState } from 'react'
import { Popover } from 'antd'
import { DownloadOutlined, MobileOutlined } from '@ant-design/icons'
import { QRCodeCanvas } from 'qrcode.react'
import { fetchLatestDesktopRelease, fetchLatestMobileRelease } from '@/lib/appDownload'
import type { DesktopReleaseInfo, MobileAppReleaseInfo } from '@/lib/appDownload'
import { isElectronRuntime } from '@/stores/serverConfigStore'

/**
 * 登录页底部「获取客户端」入口：
 * - Web 登录页：桌面端下载（公开 feed，无发布记录时自动隐藏）+ 移动端扫码；
 * - 桌面端登录页：只保留移动端扫码（用户已经在桌面端里）。
 * 二维码内容是 **APK 安装包的直接下载地址**（公开端点，扫码即下即装），
 * 平台管理员尚未上传安装包时入口自动隐藏。
 * 可选项失败一律静默降级，不阻塞登录主流程。
 */
export function ClientDownloadLinks() {
  const onWeb = !isElectronRuntime()
  const [release, setRelease] = useState<DesktopReleaseInfo | null>(null)
  const [mobileApp, setMobileApp] = useState<MobileAppReleaseInfo | null>(null)

  useEffect(() => {
    const ctl = new AbortController()
    if (onWeb) {
      fetchLatestDesktopRelease(ctl.signal)
        .then(setRelease)
        .catch(() => {})
    }
    fetchLatestMobileRelease(ctl.signal)
      .then(setMobileApp)
      .catch(() => {})
    return () => ctl.abort()
  }, [onWeb])

  return (
    <div className="tm-login-clients">
      {onWeb && release && (
        <a className="tm-login-link tm-login-client-entry" href={release.downloadUrl} download>
          <DownloadOutlined aria-hidden />
          <span>下载桌面端 v{release.version}</span>
        </a>
      )}
      {mobileApp && (
        <Popover
          placement="topRight"
          trigger="click"
          styles={{ body: { padding: 12 } }}
          content={
            <div className="tm-login-qr-panel">
              {/* 二维码固定白底黑码，保证任何主题下都可扫 */}
              <div className="tm-login-qr-box">
                <QRCodeCanvas value={mobileApp.downloadUrl} size={160} marginSize={0} />
              </div>
              <div className="tm-login-qr-caption">
                手机扫码下载移动端 v{mobileApp.version}
                <br />
                （Android 安装包，下载后直接安装）
              </div>
            </div>
          }
        >
          <span className="tm-login-link tm-login-client-entry" role="button" tabIndex={0}>
            <MobileOutlined aria-hidden />
            <span>移动端扫码下载</span>
          </span>
        </Popover>
      )}
    </div>
  )
}
