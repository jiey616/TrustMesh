'use strict'

const { app, BrowserWindow, shell, Menu, Tray, ipcMain, net, session, Notification, nativeImage } = require('electron')
const fs = require('node:fs')
const path = require('node:path')
const { autoUpdater } = require('electron-updater')

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

let tray = null
let forceQuit = false

// ---------------------------------------------------------------------------
// 自动更新（electron-updater）
//
// 更新源地址**烘焙固定**为生产地址（方案 §1 决策 11）：不跟随用户在设置页填的
// serverConfig —— 只有一个生产环境，跟随用户输入只会让填错地址的机器跑去别处更新。
// 地址同时写进 package.json 的 build.publish，打包时落成 app-update.yml。
// ---------------------------------------------------------------------------
const UPDATE_FEED_URL = 'https://175.27.135.91/api/v1/desktop/releases/feed'
const UPDATE_FEED_HOST = '175.27.135.91'
// electron-updater 的网络请求走 electron.net（Chromium 栈）且挂在这个**独立分区**上
// （electronHttpExecutor.js 的 NET_SESSION_NAME）。分区名是它的私有约定，改版本时要复核。
const UPDATE_NET_PARTITION = 'electron-updater'
// 开发态 / 未打包（没有 app-update.yml）时彻底关掉，否则 checkForUpdates 必抛异常。
const updaterEnabled = !isDev && app.isPackaged

/**
 * 给更新通道装证书信任钩子。
 *
 * 🔴 必须用**分区上的 setCertificateVerifyProc**，不能靠 app.on('certificate-error')：
 *    后者只覆盖 webContents，对 electron.net.request **完全不触发**（实测日志一次
 *    都没打印），所以设置页那个「信任自签名证书」开关对更新请求一点用都没有。
 *    生产更新源是自签证书（CN=175.27.135.91），不装这个钩子 ⇒ 所有机器永远升不了级。
 *
 * 🔴 只对烘焙的更新主机 cb(0)，其余一律 cb(-3) 落回 Chromium 默认校验。
 *    图省事全放行等于给更新通道开一个可被中间人任意投毒的洞（未签名包的
 *    完整性只剩 HTTPS + sha512，见方案 §3.3）。
 */
function installUpdaterCertTrust() {
  const ses = session.fromPartition(UPDATE_NET_PARTITION)
  ses.setCertificateVerifyProc((request, callback) => {
    if (request.hostname === UPDATE_FEED_HOST) {
      callback(0) // 0 = 信任
      return
    }
    callback(-3) // -3 = 落回 Chromium 默认校验（不是 -2「拒绝」）
  })
}

/** 更新状态机：主进程是唯一真相，渲染进程通过 tm:update-state 订阅快照。 */
let updateState = { status: 'idle', version: null, percent: 0, message: null }

function setUpdateState(patch) {
  updateState = { ...updateState, ...patch }
  for (const win of BrowserWindow.getAllWindows()) {
    win.webContents.send('tm:update-state', updateState)
  }
}

function setupAutoUpdater() {
  installUpdaterCertTrust()

  autoUpdater.autoDownload = true // 静默后台下载，用户点「重启更新」时无需等待
  autoUpdater.autoInstallOnAppQuit = true // 「稍后」= 真正退出时再装，绝不打断工作
  // ⚠️ 不调 setFeedURL：地址由 app-update.yml 烘焙（决策 11）。

  autoUpdater.on('checking-for-update', () => setUpdateState({ status: 'checking', message: null }))
  autoUpdater.on('update-available', (info) =>
    setUpdateState({ status: 'available', version: info?.version ?? null, message: null }),
  )
  autoUpdater.on('update-not-available', () =>
    setUpdateState({ status: 'up-to-date', percent: 0, message: null }),
  )
  autoUpdater.on('download-progress', (p) =>
    setUpdateState({ status: 'downloading', percent: Math.round(p?.percent ?? 0) }),
  )
  autoUpdater.on('update-downloaded', (info) =>
    setUpdateState({ status: 'downloaded', version: info?.version ?? null, percent: 100, message: null }),
  )
  autoUpdater.on('error', (err) => {
    const message = String((err && err.message) || err || 'unknown error')
    // 服务端没有已发布版本时 feed 返回 404。这不是故障，是「本次没有更新」——
    // 按错误弹窗只会让还没发过版的机器每次启动都报一次假警。
    if (/404/.test(message)) {
      setUpdateState({ status: 'up-to-date', percent: 0, message: null })
      return
    }
    console.warn('[updater] 检查/下载更新失败:', message)
    setUpdateState({ status: 'error', percent: 0, message })
  })

  // 启动后延迟静默检查：Electron 冷启动时抢网络只会拖慢首屏，且此刻渲染进程
  // 还没订阅状态（事件会丢），延迟同时解决这两个问题。
  setTimeout(() => {
    void autoUpdater.checkForUpdates().catch((err) => {
      console.warn('[updater] 启动检查失败:', String((err && err.message) || err))
    })
  }, 15000)
}

/**
 * 查找托盘/通知通用图标路径：
 *   1. exe 旁的 resources/icon.ico（安装包/便携版）
 *   2. asar 内 build/icon.ico（开发模式）
 * 找不到返回 undefined。
 */
function findIconPath() {
  const exeDir = path.dirname(process.execPath)
  const candidates = [
    path.join(exeDir, 'resources', 'icon.ico'),
    path.join(__dirname, '..', 'build', 'icon.ico'),
  ]
  for (const p of candidates) {
    if (fs.existsSync(p)) return p
  }
  return undefined
}

function createTrayIcon(win) {
  // 托盘图标：优先用找到的 ico 文件；找不到则用 exe 内嵌图标兜底
  const iconPath = findIconPath()
  let trayIcon
  if (iconPath) {
    trayIcon = new Tray(iconPath)
  } else {
    const exeIcon = nativeImage.createFromPath(process.execPath)
    trayIcon = new Tray(exeIcon.isEmpty() ? nativeImage.createEmpty() : exeIcon)
  }
  tray = trayIcon
  tray.setToolTip('TrustMesh')

  const updateContextMenu = () => {
    const template = [
      {
        label: win.isVisible() ? '隐藏窗口' : '显示窗口',
        click: () => {
          if (win.isVisible()) {
            win.hide()
          } else {
            win.show()
            win.focus()
          }
          updateContextMenu()
        },
      },
      { type: 'separator' },
      {
        label: '退出',
        click: () => {
          forceQuit = true
          app.quit()
        },
      },
    ]
    tray.setContextMenu(Menu.buildFromTemplate(template))
  }

  tray.on('click', () => {
    if (win.isVisible()) {
      win.focus()
    } else {
      win.show()
      win.focus()
    }
    updateContextMenu()
  })

  updateContextMenu()
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

  // 点击关闭按钮时隐藏到托盘，除非从托盘菜单显式退出（若托盘创建失败则正常关闭）
  win.on('close', (event) => {
    if (!forceQuit && tray) {
      event.preventDefault()
      win.hide()
    }
  })

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
// 设置 AppUserModelID 和 app.name，确保 Windows 系统通知和任务栏显示 "TrustMesh"
app.setAppUserModelId('com.trustmesh.desktop')
app.setName('TrustMesh')
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

    // 渲染进程请求显示系统通知
    ipcMain.handle('tm:show-notification', (_event, options) => {
      const { title, body, tag } = options || {}
      if (!title) return false
      if (!Notification.isSupported()) return false
      const n = new Notification({
        title: String(title),
        body: body ? String(body) : undefined,
        icon: findIconPath() || '',
        silent: false,
      })
      n.on('click', () => {
        const [w] = BrowserWindow.getAllWindows()
        if (w) {
          if (w.isMinimized()) w.restore()
          w.show()
          w.focus()
        }
        // 通知渲染进程某条通知被点击
        w?.webContents.send('tm:notification-clicked', tag)
      })
      n.show()
      return true
    })

    // ── 自动更新（方案 docs/desktop-app-update-plan-2026-09-18.md §6）──
    // ⚠️ 必须在 app.whenReady 之后才注册/触发：早于此刻，渲染进程还没建立 IPC 通道，
    //    更新事件发出去没人收（静默丢失）。
    ipcMain.handle('tm:app-version', () => app.getVersion())
    ipcMain.handle('tm:update-get-state', () => ({
      ...updateState,
      enabled: updaterEnabled,
      feedUrl: UPDATE_FEED_URL,
    }))
    ipcMain.handle('tm:update-check', async () => {
      if (!updaterEnabled) {
        return { ...updateState, enabled: false, feedUrl: UPDATE_FEED_URL, message: '开发模式不检查更新' }
      }
      try {
        await autoUpdater.checkForUpdates()
      } catch (err) {
        // 交给 'error' 事件统一落状态；这里吞掉异常只为不把 IPC 调用打成 rejected。
        console.warn('[updater] 手动检查失败:', String((err && err.message) || err))
      }
      return { ...updateState, enabled: true, feedUrl: UPDATE_FEED_URL }
    })
    ipcMain.handle('tm:update-install', () => {
      if (!updaterEnabled || updateState.status !== 'downloaded') return false
      // 🔴 必须先置 forceQuit：本壳「关闭 → 隐藏到托盘」拦下了 window close，
      //    quitAndInstall 内部走 app.quit()，不置位会被拦在托盘里 → 安装永远不启动。
      forceQuit = true
      setImmediate(() => autoUpdater.quitAndInstall(false, true))
      return true
    })

    if (updaterEnabled) {
      setupAutoUpdater()
    } else {
      console.log('[updater] 已禁用（开发模式或未打包）')
    }

    const win = createMainWindow()
    try {
      createTrayIcon(win)
    } catch (err) {
      console.error('[tray] 创建托盘图标失败:', err)
    }

    app.on('activate', () => {
      if (BrowserWindow.getAllWindows().length === 0) {
        createMainWindow()
      } else {
        win.show()
        win.focus()
      }
    })
  })

  // 关闭到托盘：窗口关闭时不退出应用，托盘图标保持运行
  app.on('window-all-closed', (event) => {
    event.preventDefault()
  })
}
