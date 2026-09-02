/**
 * 用本机 Edge 无头模式（CDP）验证 V2 页面运行时是否报错。
 * 零依赖：Node 22 自带 global WebSocket，不需要安装 Playwright/Chromium。
 *
 * 用法：
 *   node cdp_ui_check.js <path> [label]
 * 例：
 *   node cdp_ui_check.js /projects/c20a28793123895de89c08ec project
 */
const { spawn } = require('child_process')
const fs = require('fs')
const os = require('os')
const path = require('path')

const EDGE = 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'
const PORT = 9333
const BASE = 'http://localhost:5174'
const AUTH_KEY = 'trustmesh-v2:auth'

const target = process.argv[2] || '/projects'
const label = process.argv[3] || 'page'

// 注意：Node 不认 Git Bash 的 /tmp（Windows 原生程序会当成 C:\tmp），
// 统一放到脚本同级的 .tmp 目录下，bash 与 node 两侧都能对上。
const tmp = (f) => path.join(__dirname, '.tmp', f)
const accessToken = fs.readFileSync(tmp('tm_token.txt'), 'utf8').trim()
const refreshToken = fs.readFileSync(tmp('tm_refresh.txt'), 'utf8').trim()
const user = JSON.parse(fs.readFileSync(tmp('tm_user.json'), 'utf8').trim())

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

async function httpJson(url, options) {
  const res = await fetch(url, options)
  return res.json()
}

class CDP {
  constructor(ws) {
    this.ws = ws
    this.id = 0
    this.pending = new Map()
    this.logs = []
    this.exceptions = []
    ws.addEventListener('message', (ev) => {
      const msg = JSON.parse(ev.data)
      if (msg.id && this.pending.has(msg.id)) {
        const { resolve, reject } = this.pending.get(msg.id)
        this.pending.delete(msg.id)
        msg.error ? reject(new Error(JSON.stringify(msg.error))) : resolve(msg.result)
        return
      }
      if (msg.method === 'Runtime.exceptionThrown') {
        this.exceptions.push(msg.params.exceptionDetails)
      }
      if (msg.method === 'Log.entryAdded') {
        const e = msg.params.entry
        if (e.level === 'error') this.logs.push(`[${e.source}] ${e.text}`)
      }
      if (msg.method === 'Runtime.consoleAPICalled' && msg.params.type === 'error') {
        this.logs.push('[console] ' + msg.params.args.map((a) => a.value ?? a.description ?? '').join(' '))
      }
    })
  }

  send(method, params = {}) {
    const id = ++this.id
    this.ws.send(JSON.stringify({ id, method, params }))
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject })
      setTimeout(() => reject(new Error(`timeout: ${method}`)), 30000)
    })
  }

  async evaluate(expression) {
    const r = await this.send('Runtime.evaluate', {
      expression,
      returnByValue: true,
      awaitPromise: true,
    })
    if (r.exceptionDetails) throw new Error('eval error: ' + JSON.stringify(r.exceptionDetails))
    return r.result.value
  }
}

async function main() {
  const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'edge-cdp-'))
  const proc = spawn(EDGE, [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    `--user-data-dir=${profile}`,
    '--no-first-run',
    '--no-default-browser-check',
    '--disable-gpu',
    '--window-size=1600,1000',
    'about:blank',
  ], { detached: false, stdio: 'ignore' })

  try {
    // 等待调试端口就绪
    let version = null
    for (let i = 0; i < 40; i++) {
      try {
        version = await httpJson(`http://127.0.0.1:${PORT}/json/version`)
        break
      } catch {
        await sleep(500)
      }
    }
    if (!version) throw new Error('Edge 调试端口未就绪')
    console.log('✅ Edge 已启动:', version.Browser)

    // 开一个标签页
    const page = await httpJson(`http://127.0.0.1:${PORT}/json/new?about:blank`, { method: 'PUT' })
    const ws = new WebSocket(page.webSocketDebuggerUrl)
    await new Promise((resolve, reject) => {
      ws.addEventListener('open', resolve, { once: true })
      ws.addEventListener('error', reject, { once: true })
    })
    const cdp = new CDP(ws)
    await cdp.send('Runtime.enable')
    await cdp.send('Log.enable')
    await cdp.send('Page.enable')

    // 先落到同源页面，注入登录态（zustand persist 格式）
    await cdp.send('Page.navigate', { url: BASE + '/' })
    await sleep(2500)
    await cdp.evaluate(
      `(() => {
        localStorage.setItem(${JSON.stringify(AUTH_KEY)}, JSON.stringify({
          state: {
            accessToken: ${JSON.stringify(accessToken)},
            refreshToken: ${JSON.stringify(refreshToken)},
            user: ${JSON.stringify(user)},
          },
          version: 0,
        }));
        return 'ok';
      })()`
    )
    console.log('✅ 登录态已注入')

    // 真正打开目标页面
    await cdp.send('Page.navigate', { url: BASE + target })
    await sleep(6000)

    const info = await cdp.evaluate(
      `(() => {
        const text = (document.body.innerText || '').replace(/\\s+/g, ' ').trim();
        const tabs = Array.from(document.querySelectorAll('.ant-tabs-tab'))
          .map(t => t.innerText.trim()).filter(Boolean);
        const menu = Array.from(document.querySelectorAll('.ant-menu-item'))
          .map(m => m.innerText.trim()).filter(Boolean);
        return {
          url: location.href,
          title: document.title,
          textHead: text.slice(0, 400),
          tabs,
          menu,
          hasErrorBoundary: text.includes('Something went wrong') || text.includes('出错'),
        };
      })()`
    )

    const shot = await cdp.send('Page.captureScreenshot', { format: 'png' })
    const outPng = path.join(__dirname, `ui-check-${label}.png`)
    fs.writeFileSync(outPng, Buffer.from(shot.data, 'base64'))

    console.log('\n===== 页面状态 =====')
    console.log('URL        :', info.url)
    console.log('标题       :', info.title)
    console.log('Tab 列表   :', JSON.stringify(info.tabs))
    console.log('侧边栏菜单 :', JSON.stringify(info.menu))
    console.log('正文摘要   :', info.textHead.slice(0, 200))
    console.log('错误边界   :', info.hasErrorBoundary)

    console.log('\n===== 运行时错误 =====')
    if (cdp.exceptions.length === 0 && cdp.logs.length === 0) {
      console.log('✅ 无 console error / 未捕获异常')
    } else {
      cdp.exceptions.forEach((e) => {
        console.log('❌ EXCEPTION:', e.exception?.description || e.text)
      })
      ;[...new Set(cdp.logs)].forEach((l) => console.log('❌ LOG:', l))
    }
    console.log('\n截图:', outPng)
    ws.close()
  } finally {
    proc.kill()
  }
}

main().catch((e) => {
  console.error('❌ 检查失败:', e.message)
  process.exit(1)
})
