// 自包含 UI 验证：登录 → 项目文件页 → 点开「数字员工产物」→ 断言最终剧本文件可见 → 截图
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

const EDGE = 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe';
const PORT = 9341;
const EMAIL = 'test001@163.com';
const PASSWORD = 'qwer1234';
const PROJECT = 'c20a28793123895de89c08ec';
const TASK = '76677124dfc67c29e37b4533';
const WANT = ['红色安全灯下的第十二张底片_单集短剧剧本_定稿交付版_20260902.md',
  '《红色安全灯下的第十二张底片》_影视剧本_完整稿_v01_20260902.docx'];

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function main() {
  const browser = spawn(EDGE, [
    '--headless=new', '--disable-gpu', '--no-sandbox',
    `--remote-debugging-port=${PORT}`,
    '--user-data-dir=C:\\tmp\\edge-profile-ff-' + Date.now(),
  ], { stdio: 'ignore' });

  await sleep(1800);
  await fetch(`http://127.0.0.1:${PORT}/json/version`).then((r) => r.json());
  const opened = await fetch(`http://127.0.0.1:${PORT}/json/new?about:blank`, { method: 'PUT' }).then((r) => r.json());
  const sock = new WebSocket(opened.webSocketDebuggerUrl);

  let nextId = 1;
  const pending = new Map();
  await new Promise((res, rej) => { sock.onopen = res; sock.onerror = rej; });
  sock.onmessage = (ev) => {
    const m = JSON.parse(ev.data);
    if (m.id != null && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  };
  const cmd = (method, params = {}) => new Promise((resolve) => {
    const id = nextId++; pending.set(id, resolve);
    sock.send(JSON.stringify({ id, method, params }));
  });
  const evalJs = async (expr) => {
    const r = await cmd('Runtime.evaluate', { expression: expr, returnByValue: true, awaitPromise: true });
    return r.result?.result?.value;
  };

  await cmd('Runtime.enable');
  await cmd('Page.enable');

  // 1) 登录
  await cmd('Page.navigate', { url: 'http://localhost:5174/login' });
  await sleep(3000);
  await evalJs(`(function(){
    const setV = (el, v) => {
      const s = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
      s.call(el, v); el.dispatchEvent(new Event('input', { bubbles: true }));
    };
    const inputs = [...document.querySelectorAll('input')];
    const em = inputs.find(i => i.type === 'email' || i.type === 'text' || i.autocomplete === 'email');
    const pw = inputs.find(i => i.type === 'password');
    if (!em || !pw) return 'no-inputs: ' + inputs.map(i=>i.type).join(',');
    setV(em, ${JSON.stringify(EMAIL)});
    setV(pw, ${JSON.stringify(PASSWORD)});
    const btn = document.querySelector('button[type="submit"]') || [...document.querySelectorAll('button')].find(b => /登录|登 录|Sign in|Login/.test(b.innerText));
    if (!btn) return 'no-button';
    btn.click();
    return 'submitted';
  })()`);
  await sleep(4000);

  const auth = await evalJs(`localStorage.getItem('trustmesh-v2:auth')`);
  if (!auth) { console.log('LOGIN FAILED, page =', (await evalJs('document.body.innerText') || '').slice(0, 300)); sock.close(); browser.kill(); return; }
  console.log('LOGIN OK');

  // 2) 项目文件页
  await cmd('Page.navigate', { url: `http://localhost:5174/projects/${PROJECT}?tab=files` });
  await sleep(5000);
  // 直接点 file_name 文字节点（不是图标 cell）
  const clicked = await evalJs(`(function(){
    const all = [...document.querySelectorAll('span, a, div')];
    const t = all.find(el => el.children.length === 0 && el.innerText && el.innerText.trim() === '数字员工产物');
    if (t) { t.click(); return 'clicked-text'; }
    return 'not-found';
  })()`);
  console.log('FOLDER CLICK =', clicked);
  const want1 = WANT[0];
  for (let i = 0; i < 25; i++) {
    const has = await evalJs(`(document.body.innerText || '').includes(${JSON.stringify(want1)})`);
    if (has) break;
    await sleep(400);
  }

  let text = (await evalJs('document.body.innerText') || '');
  const ss1 = await cmd('Page.captureScreenshot', { format: 'png' });
  fs.writeFileSync(path.join(__dirname, 'ui-check-final-project-files.png'), Buffer.from(ss1.result.data, 'base64'));

  // 3) 任务详情页产物 tab
  await cmd('Page.navigate', { url: `http://localhost:5174/tasks/${TASK}?tab=files` });
  await sleep(5000);
  let text2 = (await evalJs('document.body.innerText') || '');
  if (!/数字员工产物|红色安全灯/.test(text2)) {
    const c2 = await evalJs(`(function(){
      const all = [...document.querySelectorAll('span, a, div')];
      const t = all.find(el => el.children.length === 0 && el.innerText && el.innerText.trim() === '数字员工产物');
      if (t) { t.click(); return 'clicked-text'; }
      return 'not-found';
    })()`);
    console.log('TASK FOLDER CLICK =', c2);
    for (let i = 0; i < 25; i++) {
      const has = await evalJs(`(document.body.innerText || '').includes(${JSON.stringify(want1)})`);
      if (has) break;
      await sleep(400);
    }
    text2 = (await evalJs('document.body.innerText') || '');
  }
  const ss2 = await cmd('Page.captureScreenshot', { format: 'png' });
  fs.writeFileSync(path.join(__dirname, 'ui-check-final-task-files.png'), Buffer.from(ss2.result.data, 'base64'));

  const all = text + '\n---\n' + text2;
  console.log('\n===== 断言 =====');
  for (const w of WANT) {
    console.log((all.includes(w) ? '  [PASS] ' : '  [FAIL] ') + w);
  }
  console.log('\n===== 项目文件页（前 2500 字）=====');
  console.log(text.slice(0, 2500));
  console.log('\n===== 任务文件页（前 2500 字）=====');
  console.log(text2.slice(0, 2500));

  sock.close(); browser.kill();
  await sleep(300);
}

main().catch((e) => { console.error('FAIL', e); process.exit(1); });
