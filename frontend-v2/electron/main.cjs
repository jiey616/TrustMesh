'use strict'

const { app, BrowserWindow, shell, Menu, ipcMain, net } = require('electron')
const fs = require('node:fs')
const path = require('node:path')

// 便携版 / 安装版都把用户数据（含 localStorage，服务端地址配置就存在这里）
// 放到可预期目录。Electron 默认 userData 在 %APPDATA%\<productName>，够用，无需改。
const isDev = !!process.env.VITE_DEV_SERVER_URL

// ---------------------------------------------------------------------------
// 桌面端持久化配置（独立于 localStorage，主进程/渲染进程都能读）
//   trustInsecureTls: 信任自签名 / 私有 CA 证书（默认 false）
// ---------------------------------------------------------------------------
let desktopConfig = { trustInsecureTls: false }

function desktopConfigPath() {
  return path.join(app.getPath('userData'), 'trustmesh-desktop.json')
}

function loadDesktopConfig() {
  try {
    const raw = fs.readFileSync(desktopConfigPath(), 'utf8')
    desktopConfig = { trustInsecureTls: false, ...JSON.parse(raw) }
  } catch {
    // 首次运行或配置文件缺失：保持默认值
  }
}

function saveDesktopConfig(patch) {
  desktopConfig = { ...desktopConfig, ...patch }
  try {
    fs.mkdirSync(path.dirname(desktopConfigPath()), { recursive: true })
    fs.writeFileSync(desktopConfigPath(), JSON.stringify(desktopConfig, null, 2), 'utf8')
  } catch (err) {
    console.error('[desktop-config] 持久化失败:', err)
  }
  return desktopConfig
}

/**
 * 证书校验：默认严格校验（Chromium 行为）。
 * 用户在设置页开启「信任自签名证书」后放行内网自签名 / 私有 CA。
 */
app.on('certificate-error', (event, _webContents, url, error, _certificate, callback) => {
  if (desktopConfig.trustInsecureTls) {
    event.preventDefault()
    callback(true)
    return
  }
  console.warn('[tls] 证书被拒绝（如需信任请在设置页开启信任自签名证书）:', error, url)
  callback(false)
})

/**
 * 连接探测：走主进程 net.request，能拿到 Chromium 的错误码
 * （渲染进程 fetch 只会得到笼统的 "Failed to fetch"）。
 */
function probeServer(baseUrl, timeoutMs = 6000) {
  return new Promise((resolve) => {
    let target
    try {
      target = `${String(baseUrl).replace(/\/+$/, '')}/api/v1/platform/info`
      // 非 http(s) 一律拒绝，避免协议注入
      if (!/^https?:\/\//i.test(target)) throw new Error('unsupported protocol')
    } catch {
      resolve({ ok: false, status: 0, error: 'ERR_INVALID_URL' })
      return
    }

    let settled = false
    let clearTimer = () => {}
    const done = (result) => {
      if (settled) return
      settled = true
      clearTimer()
      resolve(result)
    }

    const timer = setTimeout(() => {
      try {
        req.abort()
      } catch {
        // 请求已结束
      }
      done({ ok: false, status: 0, error: 'ERR_TIMEOUT' })
    }, timeoutMs)
    clearTimer = () => clearTimeout(timer)

    let req
    try {
      req = net.request({ method: 'GET', url: target })
    } catch (err) {
      window_clear()
      resolve({ ok: false, status: 0, error: String((err && err.message) || err) })
      return
    }

    req.on('response', (res) => {
      res.resume() // 丢弃响应体
      done({ ok: res.statusCode >= 200 && res.statusCode < 300, status: res.statusCode, error: null })
    })
    req.on('error', (err) => {
      let code = String((err && err.message) || err || 'ERR_FAILED')
      // Chromium 原文示例：net::ERR_CERT_AUTHORITY_INVALID
      const matched = /net::([A-Z0-9_]+)/.exec(code)
      done({ ok: false, status: 0, error: matched ? matched[1] : code })
    })
    req.on('abort', () => done({ ok: false, status: 0, error: 'ERR_ABORTED' }))
    req.end()
  })
}

function createMainWindow() {
  const win = new BrowserWindow({
    width: 1440,
    height: 900,
    minWidth: 1024,
    minHeight: 700,
    title: 'TrustMesh',
    backgroundColor: '#0b0d12',
    autoHideMenuBar: true,
    show: false,
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
      spellcheck: false,
    },
  })

  win.once('ready-to-show', () => win.show())

  // 外链一律交给系统浏览器，不在壳内打开
  win.webContents.setWindowOpenHandler(({ url }) => {
    if (/^https?:\/\//i.test(url)) {
      void shell.openExternal(url)
      return { action: 'deny' }
    }
    return { action: 'allow' }
  })

  // 只允许同源（file://）导航；其余跳转（如下载完成页等）走系统浏览器
  win.webContents.on('will-navigate', (event, url) => {
    if (!url.startsWith('file://') && !url.startsWith('http://localhost')) {
      event.preventDefault()
      void shell.openExternal(url)
    }
  })

  if (isDev) {
    void win.loadURL(process.env.VITE_DEV_SERVER_URL)
    win.webContents.openDevTools({ mode: 'detach' })
  } else {
    void win.loadFile(path.join(__dirname, '..', 'dist-v2', 'index.html'))
  }

  return win
}

// 单实例：双击第二次只激活已有窗口
const gotLock = app.requestSingleInstanceLock()
if (!gotLock) {
  app.quit()
} else {
  app.on('second-instance', () => {
    const [win] = BrowserWindow.getAllWindows()
    if (win) {
      if (win.isMinimized()) win.restore()
      win.focus()
    }
  })

  app.whenReady().then(() => {
    loadDesktopConfig()
    Menu.setApplicationMenu(null)

    ipcMain.handle('tm:desktop-config-get', () => desktopConfig)
    ipcMain.handle('tm:desktop-config-set', (_event, patch) => saveDesktopConfig(patch || {}))
    ipcMain.handle('tm:probe-server', (_event, baseUrl, timeoutMs) =>
      probeServer(baseUrl, typeof timeoutMs === 'number' ? timeoutMs : 6000),
    )

    createMainWindow()

    app.on('activate', () => {
      if (BrowserWindow.getAllWindows().length === 0) createMainWindow()
    })
  })

  app.on('window-all-closed', () => {
    if (process.platform !== 'darwin') app.quit()
  })
}
