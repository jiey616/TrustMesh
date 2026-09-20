import { Capacitor } from '@capacitor/core'
import { LocalNotifications } from '@capacitor/local-notifications'
import { Directory, Filesystem } from '@capacitor/filesystem'
import { Share } from '@capacitor/share'

/**
 * 原生壳（Android/iOS）专属能力桥：所有调用都先判 isNativePlatform()，
 * 普通浏览器（5175 dev / PWA）一律 no-op，绝不影响 Web 使用。
 *
 * 🔴 刻意集中在此文件：将来接 iOS 时只改这里。
 */

/** 是否运行在原生壳内（区别于浏览器/PWA） */
export function isNativeApp(): boolean {
  return Capacitor.isNativePlatform()
}

const CHANNEL_TASK_EVENTS = 'task-events'
let primed = false
let notifSeq = 1

/** 原生壳内请求通知权限、建 Android 通知渠道、注册点击跳转。非原生环境 no-op。 */
export async function primeLocalNotifications(): Promise<void> {
  if (!isNativeApp() || primed) return
  primed = true
  try {
    const status = await LocalNotifications.requestPermissions()
    if (status.display !== 'granted') return
    // Android 8+ 必须先建渠道；importance HIGH 才会横幅+震动
    await LocalNotifications.createChannel({
      id: CHANNEL_TASK_EVENTS,
      name: '任务动态',
      description: '待办确认、步骤完成、任务成败',
      importance: 4, // Importance.High
      visibility: 1, // Visibility.Public
      vibration: true,
    })
    await LocalNotifications.addListener('localNotificationActionPerformed', (action) => {
      const taskId = (action.notification.extra as { taskId?: string } | undefined)?.taskId
      if (taskId) window.location.hash = `#/tasks/detail/${taskId}`
    })
  } catch {
    // 通知初始化失败不影响主流程
  }
}

/**
 * 弹一条本地通知（仅原生壳；Web 上 no-op——Web 端的等价物是收件箱红点）。
 * fire-and-forget：调用方不 await。
 */
export function notifyTaskEvent(n: { title: string; body: string; taskId?: string }): void {
  if (!isNativeApp()) return
  // Android 要求 int id；用递增序号取模避开 32 位上限
  const id = notifSeq++ & 0x7fffffff
  void LocalNotifications.schedule({
    notifications: [
      {
        id,
        title: n.title,
        body: n.body,
        extra: { taskId: n.taskId },
        channelId: CHANNEL_TASK_EVENTS,
      },
    ],
  }).catch(() => {
    /* 通知失败不影响主流程 */
  })
}

/** blob → base64（去掉 dataURL 前缀，Filesystem.writeFile 只吃裸 base64） */
function blobToBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onloadend = () => {
      const r = String(reader.result ?? '')
      const i = r.indexOf(',')
      resolve(i >= 0 ? r.slice(i + 1) : r)
    }
    reader.onerror = () => reject(new Error('读取文件失败'))
    reader.readAsDataURL(blob)
  })
}

/**
 * 原生壳：把 blob 写入应用缓存并拉起系统分享/打开面板（用户可选 WPS/浏览器打开或保存）。
 * 🔴 WebView 里 `<a download>` 与 `window.open(blob:)` 均无效，非原生返回 false 由调用方走 Web 分支。
 */
export async function shareBlobAsFile(blob: Blob, fileName: string): Promise<boolean> {
  if (!isNativeApp()) return false
  const base64 = await blobToBase64(blob)
  const path = `deliverables/${Date.now()}-${fileName}`
  await Filesystem.writeFile({
    path,
    data: base64,
    directory: Directory.Cache,
    recursive: true,
  })
  const { uri } = await Filesystem.getUri({ path, directory: Directory.Cache })
  await Share.share({ title: fileName, url: uri, dialogTitle: '打开或保存文件' })
  return true
}
