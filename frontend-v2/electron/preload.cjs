'use strict'

// 暴露最小信息给渲染进程：用于判断是否运行在桌面壳内（HashRouter 切换等）
const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('process', {
  versions: {
    electron: process.versions.electron,
  },
})

/**
 * 桌面端专属能力（浏览器环境为 undefined）：
 * - 读写主进程持久化配置（自签名证书信任开关）
 * - 走主进程 net.request 做连接探测，可拿回 Chromium 错误码
 *   （渲染进程 fetch 只会得到笼统的 "Failed to fetch"）
 */
contextBridge.exposeInMainWorld('desktop', {
  getConfig: () => ipcRenderer.invoke('tm:desktop-config-get'),
  setTrustInsecureTls: (enabled) =>
    ipcRenderer.invoke('tm:desktop-config-set', { trustInsecureTls: !!enabled }),
  probeServer: (baseUrl, timeoutMs) => ipcRenderer.invoke('tm:probe-server', baseUrl, timeoutMs),
  showNotification: (options) => ipcRenderer.invoke('tm:show-notification', options),
  onNotificationClicked: (callback) => {
    const handler = (_event, tag) => callback(tag)
    ipcRenderer.on('tm:notification-clicked', handler)
    return () => ipcRenderer.removeListener('tm:notification-clicked', handler)
  },

  /**
   * 自动更新（方案 docs/desktop-app-update-plan-2026-09-18.md §6）。
   *
   * ⚠️ 改本文件（含新增通道）**必须重新发一次桌面包**：preload 打进 asar，
   *    老版本的壳不认识新通道，会静默拿到 undefined。
   */
  getAppVersion: () => ipcRenderer.invoke('tm:app-version'),
  getUpdateState: () => ipcRenderer.invoke('tm:update-get-state'),
  updateCheck: () => ipcRenderer.invoke('tm:update-check'),
  updateInstall: () => ipcRenderer.invoke('tm:update-install'),
  onUpdateState: (callback) => {
    const handler = (_event, state) => callback(state)
    ipcRenderer.on('tm:update-state', handler)
    return () => ipcRenderer.removeListener('tm:update-state', handler)
  },
})
