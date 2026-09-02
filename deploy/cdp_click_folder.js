// 点开项目文件页的「数字员工产物」虚拟目录，截图。
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

const PORT = 9333;
const TOKEN = fs.readFileSync(path.join(__dirname, '.tmp/tm_token.txt'), 'utf8').trim();
const REFRESH = fs.readFileSync(path.join(__dirname, '.tmp/tm_refresh.txt'), 'utf8').trim();
const USER = JSON.parse(fs.readFileSync(path.join(__dirname, '.tmp/tm_user.json'), 'utf8'));
const TAG = process.argv[2] || 'ui-files-click';
const NAV = process.argv[3] || '/projects/c20a28793123895de89c08ec?tab=files';

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function main() {
  const ms = spawn(
    EDGE,
    [
      '--headless=new',
      '--disable-gpu',
      '--no-sandbox',
      `--remote-debugging-port=${PORT}`,
      '--user-data-dir=C:\\tmp\\edge-profile-' + Date.now(),
    ],
    { stdio: 'ignore' }
  );

  await sleep(1500);
  const ver = await fetch(`http://127.0.0.1:${PORT}/json/version`).then((r) => r.json());
  const opened = await fetch(`http://127.0.0.1:${PORT}/json/new?about:blank`, { method: 'PUT' }).then((r) => r.json());
  const wsUrl = opened.webSocketDebuggerUrl;
  const W = require('node:url').URL;
  const WebSocket = globalThis.WebSocket;
  const sock = new WebSocket(wsUrl);

  let nextId = 1;
  const pending = new Map();
  const events = [];

  await new Promise((resolve, reject) => {
    sock.onopen = resolve;
    sock.onerror = reject;
  });

  sock.onmessage = (ev) => {
    const m = JSON.parse(ev.data);
    if (m.id != null && pending.has(m.id)) {
      pending.get(m.id)(m);
      pending.delete(m.id);
    } else if (m.method) {
      events.push(m);
    }
  };

  const cmd = (method, params = {}) =>
    new Promise((resolve) => {
      const id = nextId++;
      pending.set(id, resolve);
      sock.send(JSON.stringify({ id, method, params }));
    });

  await cmd('Runtime.enable');
  await cmd('Log.enable');
  await cmd('Page.enable');
  await cmd('Network.enable');

  // 注入 zustand 登录态
  await cmd('Page.addScriptToEvaluateOnNewDocument', {
    source: `
      try {
        localStorage.setItem('trustmesh-v2:auth', JSON.stringify({
          state: { accessToken: ${JSON.stringify(TOKEN)}, refreshToken: ${JSON.stringify(REFRESH)}, user: ${JSON.stringify(USER)} },
          version: 0
        }));
      } catch (e) {}
    `,
  });

  await cmd('Page.navigate', { url: 'http://localhost:5174' + NAV });
  await sleep(3500);

  // 列出当前页文字
  const r1 = await cmd('Runtime.evaluate', {
    expression: 'document.body.innerText',
    returnByValue: true,
  });

  // 点开「数字员工产物」文件夹 — 用 XPath 找到表格里那一行的点击目标
  const r2 = await cmd('Runtime.evaluate', {
    expression: `(function(){
      const rows = document.querySelectorAll('.ant-table-row, [role="row"]');
      for (const r of rows) {
        if (r.innerText && r.innerText.includes('数字员工产物')) {
          const cell = r.querySelector('.ant-table-cell, [role="cell"]') || r;
          cell.click();
          return 'clicked';
        }
      }
      // 备用：直接点 .ant-table-row > 第一格
      return 'not-found';
    })()`,
    returnByValue: true,
  });
  console.log('CLICK =', r2.result?.result?.value);

  await sleep(2000);

  const r3 = await cmd('Runtime.evaluate', {
    expression: 'document.body.innerText',
    returnByValue: true,
  });

  const ss = await cmd('Page.captureScreenshot', { format: 'png' });
  fs.writeFileSync(path.join(__dirname, `ui-check-${TAG}.png`), Buffer.from(ss.result.data, 'base64'));

  const exps = events.filter((e) => e.method === 'Runtime.exceptionThrown' || e.method === 'Log.entryAdded');
  console.log('===== 点击后页面 =====');
  console.log((r3.result?.result?.value || '').slice(0, 1500));
  console.log('===== 异常数 =====', exps.length);
  if (exps.length) console.log(JSON.stringify(exps[0], null, 2).slice(0, 600));

  sock.close();
  ms.kill();
  await sleep(200);
}

main().catch((e) => { console.error('FAIL', e); process.exit(1); });
