/**
 * 验证「已归档项目」收起到右上角抽屉的行为（项目列表页）。
 * 零依赖：Node 22 自带 global WebSocket + 本机 Edge 无头（CDP）。
 *
 * 用法： node cdp_archive_check.js
 *
 * 检查项：
 *   1. 主列表只出现活跃项目（归档的不能垫在下方）
 *   2. 右上角存在「已归档项目 (N)」按钮，N == 归档数
 *   3. 点击后 Drawer 打开，行数 == 归档数
 *   4. 抽屉内搜索框按名称过滤生效
 *   5. 全程无 console error / 未捕获异常
 */
const { spawn } = require('child_process')
const fs = require('fs')
const os = require('os')
const path = require('path')

const EDGE = 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'
const PORT = 9334
const BASE = 'http://localhost:5174'
const AUTH_KEY = 'trustmesh-v2:auth'

const tmp = (f) => path.join(__dirname, '.tmp', f)
const accessToken = fs.readFileSync(tmp('tm_token.txt'), 'utf8').trim()
const refreshToken = fs.readFileSync(tmp('tm_refresh.txt'), 'utf8').trim()
const user = JSON.parse(fs.readFileSync(tmp('tm_user.json'), 'utf8').trim())

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const httpJson = async (url, options) => (await fetch(url, options)).json()

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
      if (msg.method === 'Runtime.exceptionThrown') this.exceptions.push(msg.params.exceptionDetails)
      if (msg.method === 'Log.entryAdded' && msg.params.entry.level === 'error') {
        this.logs.push(`[${msg.params.entry.source}] ${msg.params.entry.text}`)
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
    const r = await this.send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true })
    if (r.exceptionDetails) throw new Error('eval error: ' + JSON.stringify(r.exceptionDetails))
    return r.result.value
  }
  async shot(name) {
    const s = await this.send('Page.captureScreenshot', { format: 'png' })
    const out = path.join(__dirname, name)
    fs.writeFileSync(out, Buffer.from(s.data, 'base64'))
    return out
  }
}

// 读取「主列表」里的项目名（排除抽屉，抽屉在 .ant-drawer 内）
const READ_LIST = `(() => {
  const rows = Array.from(document.querySelectorAll('.ant-table-tbody tr.ant-table-row'))
    .filter(r => !r.closest('.ant-drawer'));
  return rows.map(r => (r.querySelector('td a') || r.querySelector('td')).innerText.trim());
})()`

const READ_DRAWER = `(() => {
  const d = document.querySelector('.ant-drawer');
  if (!d) return null;
  const rows = Array.from(d.querySelectorAll('.ant-table-tbody tr.ant-table-row'));
  return {
    title: (d.querySelector('.ant-drawer-title') || {}).innerText || '',
    open: d.classList.contains('ant-drawer-open') || !!d.querySelector('.ant-drawer-content'),
    count: rows.length,
    names: rows.map(r => (r.querySelector('td a') || r.querySelector('td')).innerText.trim()),
    pager: (d.querySelector('.ant-pagination') || {}).innerText || '',
  };
})()`

async function main() {
  const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'edge-archive-'))
  const proc = spawn(EDGE, [
    '--headless=new', `--remote-debugging-port=${PORT}`, `--user-data-dir=${profile}`,
    '--no-first-run', '--no-default-browser-check', '--disable-gpu',
    '--window-size=1600,1000', 'about:blank',
  ], { stdio: 'ignore' })

  const checks = []
  const check = (name, pass, detail) => {
    checks.push({ name, pass, detail })
    console.log(`${pass ? '✅' : '❌'} ${name}${detail ? ' — ' + detail : ''}`)
  }

  try {
    let version = null
    for (let i = 0; i < 40; i++) {
      try { version = await httpJson(`http://127.0.0.1:${PORT}/json/version`); break } catch { await sleep(500) }
    }
    if (!version) throw new Error('Edge 调试端口未就绪')
    console.log('Edge:', version.Browser, '\n')

    const page = await httpJson(`http://127.0.0.1:${PORT}/json/new?about:blank`, { method: 'PUT' })
    const ws = new WebSocket(page.webSocketDebuggerUrl)
    await new Promise((res, rej) => {
      ws.addEventListener('open', res, { once: true })
      ws.addEventListener('error', rej, { once: true })
    })
    const cdp = new CDP(ws)
    await cdp.send('Runtime.enable')
    await cdp.send('Log.enable')
    await cdp.send('Page.enable')

    await cdp.send('Page.navigate', { url: BASE + '/' })
    await sleep(2000)
    await cdp.evaluate(`(() => {
      localStorage.setItem(${JSON.stringify(AUTH_KEY)}, JSON.stringify({
        state: { accessToken: ${JSON.stringify(accessToken)}, refreshToken: ${JSON.stringify(refreshToken)}, user: ${JSON.stringify(user)} },
        version: 0,
      }));
      return 'ok';
    })()`)

    await cdp.send('Page.navigate', { url: BASE + '/projects' })
    await sleep(5000)

    // 1. 主列表只应有活跃项目
    const mainList = await cdp.evaluate(READ_LIST)
    const hasArchivedInMain = mainList.some((n) => /测试项目（0830）|2060|测试123|测试项目2|测试任务2|全局工作流验证项目/.test(n))
    check('主列表不含归档项目', !hasArchivedInMain, `主列表 ${mainList.length} 行: ${JSON.stringify(mainList)}`)

    // 2. 右上角归档入口
    const btn = await cdp.evaluate(`(() => {
      const b = Array.from(document.querySelectorAll('button')).find(x => x.innerText.includes('已归档项目'));
      return b ? b.innerText.trim() : null;
    })()`)
    check('右上角有「已归档项目」入口', !!btn, btn || '未找到')
    const expected = Number((btn || '').match(/\((\d+)\)/)?.[1] ?? -1)
    check('入口带归档数量', expected >= 0, `解析到 N=${expected}`)

    let png = await cdp.shot('ui-archive-1-list.png')

    // 3. 点击打开抽屉
    await cdp.evaluate(`(() => {
      const b = Array.from(document.querySelectorAll('button')).find(x => x.innerText.includes('已归档项目'));
      b.click(); return 'clicked';
    })()`)
    await sleep(1200)
    let drawer = await cdp.evaluate(READ_DRAWER)
    check('点击后抽屉打开', !!drawer, drawer ? drawer.title : '未渲染')
    check('抽屉内行数 == 归档数', drawer && drawer.count === expected,
      drawer ? `抽屉 ${drawer.count} 行 / 入口标 ${expected}` : '-')
    png = await cdp.shot('ui-archive-2-drawer.png')

    // 4. 搜索过滤
    if (drawer && drawer.count > 0) {
      const target = drawer.names[0]
      await cdp.evaluate(`(() => {
        const input = document.querySelector('.ant-drawer input');
        input.focus(); return !!input;
      })()`)
      await cdp.send('Input.insertText', { text: target.slice(0, 3) })
      await sleep(900)
      const after = await cdp.evaluate(READ_DRAWER)
      check('抽屉搜索过滤生效', after && after.count > 0 && after.count < drawer.count,
        `输入"${target.slice(0, 3)}" → ${after?.count} 行（原 ${drawer.count}）`)
      png = await cdp.shot('ui-archive-3-search.png')
    }

    // 5. 关闭抽屉后主列表不受影响
    await cdp.evaluate(`(() => {
      const c = document.querySelector('.ant-drawer-close');
      if (c) { c.click(); return 'closed' }
      return 'no-close-btn';
    })()`)
    await sleep(900)
    const mainAfter = await cdp.evaluate(READ_LIST)
    check('关闭抽屉后主列表不变', JSON.stringify(mainAfter) === JSON.stringify(mainList),
      `${mainAfter.length} 行`)

    console.log('\n===== 运行时错误 =====')
    if (cdp.exceptions.length === 0 && cdp.logs.length === 0) {
      console.log('✅ 无 console error / 未捕获异常')
    } else {
      cdp.exceptions.forEach((e) => console.log('❌ EXCEPTION:', e.exception?.description || e.text))
      ;[...new Set(cdp.logs)].forEach((l) => console.log('❌ LOG:', l))
    }
    const ok = checks.every((c) => c.pass)
    console.log(`\n结果：${checks.filter((c) => c.pass).length}/${checks.length} 通过，截图目录 ${path.dirname(png)}`)
    ws.close()
    if (!ok) process.exitCode = 1
  } finally {
    proc.kill()
  }
}

main().catch((e) => {
  console.error('❌ 检查失败:', e.message)
  process.exit(1)
})
