// CDP screenshot: project files page + task deliverables with kind badges.
// Usage: node cdp_shot_badges.js
const { spawn } = require('child_process');
const fs = require('fs');
const http = require('http');

const EDGE = 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
const PORT = 9335;
const PROFILE = process.env.TEMP + '/edge-cdp-badge';
const PROJECT = 'c20a28793123895de89c08ec';
const TASK = '76677124dfc67c29e37b4533';
const EMAIL = 'test001@163.com';
const PASS = 'qwer1234';

function req(method, path, body, headers = {}) {
  return new Promise((resolve, reject) => {
    const data = body ? JSON.stringify(body) : null;
    const r = http.request({ host: '127.0.0.1', port: PORT, path, method,
      headers: { 'Content-Type': 'application/json', ...(data ? { 'Content-Length': Buffer.byteLength(data) } : {}), ...headers } },
      (res) => { let buf = ''; res.on('data', c => buf += c); res.on('end', () => resolve(buf)); });
    r.on('error', reject);
    if (data) r.write(data);
    r.end();
  });
}

async function cmd(id, method, params = {}, sessionId) {
  return req('/', null, {}, {}) && JSON.parse(await new Promise((resolve, reject) => {
    const data = JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) });
    const r = http.request({ host: '127.0.0.1', port: PORT, path: '/json/version', method: 'GET' }, () => {});
    r.on('error', () => {});
    r.end();
    // WebSocket would be needed; fall back to /json list for target ws
    resolve('{}');
  }));
}

(async () => {
  // Real CDP over WebSocket (zero-dep, Node 22 global WebSocket)
  const login = await fetch('http://localhost:8080/api/v1/auth/login', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: EMAIL, password: PASS }) }).then(r => r.json());
  const token = (login.data && login.data.access_token) || login.access_token || '';
  console.log('LOGIN token_len =', token.length);
  const auth = { state: { token, user: { email: EMAIL } }, version: 0 };
  fs.mkdirSync(PROFILE, { recursive: true });

  const edge = spawn(EDGE, [
    '--headless=new', `--remote-debugging-port=${PORT}`, `--user-data-dir=${PROFILE}`,
    '--window-size=1500,950', '--no-first-run', '--disable-gpu', 'about:blank',
  ], { stdio: 'ignore' });

  let wsUrl = null;
  for (let i = 0; i < 40 && !wsUrl; i++) {
    await new Promise(r => setTimeout(r, 250));
    try {
      const list = JSON.parse(await new Promise((resolve, reject) => {
        http.get(`http://127.0.0.1:${PORT}/json`, res => { let b = ''; res.on('data', c => b += c); res.on('end', () => resolve(b)); }).on('error', reject);
      }));
      const page = list.find(t => t.type === 'page');
      if (page) wsUrl = page.webSocketDebuggerUrl;
    } catch { /* retry */ }
  }
  if (!wsUrl) { console.log('FAIL no CDP'); edge.kill(); process.exit(1); }
  const ws = new WebSocket(wsUrl);
  await new Promise(r => { ws.onopen = r; });
  let mid = 0;
  const send = (method, params = {}, sessionId) => new Promise((resolve) => {
    const id = ++mid;
    ws.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }));
    const on = (ev) => { const m = JSON.parse(ev.data); if (m.id === id) { ws.removeEventListener('message', on); resolve(m.result); } };
    ws.addEventListener('message', on);
  });
  const evalJs = async (expr) => {
    const r = await send('Runtime.evaluate', { expression: expr, returnByValue: true, awaitPromise: true });
    if (r && r.exceptionDetails) return 'EVAL-ERR:' + (r.exceptionDetails.text || '') + ' ' + ((r.exceptionDetails.exception && r.exceptionDetails.exception.description) || '').slice(0, 200);
    return r && r.result && r.result.value;
  };
  const sleep = (ms) => new Promise(r => setTimeout(r, ms));

  // inject auth via real login form
  await send('Page.enable');
  await send('Page.navigate', { url: `http://localhost:5174/login` });
  await sleep(3500);
  const loginRes = await evalJs(`(function(){
    const setV = (el, v) => {
      const s = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
      s.call(el, v); el.dispatchEvent(new Event('input', { bubbles: true }));
    };
    const inputs = [...document.querySelectorAll('input')];
    const em = inputs.find(i => i.type === 'email' || i.type === 'text' || i.autocomplete === 'email');
    const pw = inputs.find(i => i.type === 'password');
    if (!em || !pw) return 'no-inputs';
    setV(em, ${JSON.stringify(EMAIL)});
    setV(pw, ${JSON.stringify(PASS)});
    const btn = document.querySelector('button[type="submit"]') || [...document.querySelectorAll('button')].find(b => /登录/.test(b.innerText));
    if (!btn) return 'no-button';
    btn.click();
    return 'submitted';
  })()`);
  console.log('LOGIN FORM =', loginRes);
  await sleep(4000);
  await send('Page.navigate', { url: `http://localhost:5174/projects/${PROJECT}?tab=files` });
  await sleep(5000);

  // workflow progress panel: click step0 node, assert only deliverable shows
  const wfClick = await evalJs(`(function(){
    const btns = [...document.querySelectorAll('button')];
    const n = btns.find(b => b.innerText && b.innerText.includes('剧本创作'));
    if (!n) return 'no-node';
    n.click(); return 'clicked';
  })()`);
  console.log('WORKFLOW NODE CLICK =', wfClick);
  await sleep(1500);
  const text0 = await evalJs('document.body.innerText') || '';
  const wfOk = text0.includes('最终产物') && text0.includes('定稿交付版_20260902.md')
    && !text0.includes('01-intake.md') && !text0.includes('08-final-revised-draft.md');
  console.log('WORKFLOW ONLY-DELIVERABLE =', wfOk ? 'PASS' : 'FAIL');
  await send('Page.captureScreenshot').then(r => {
    if (r && r.data) fs.writeFileSync('D:/AIWorkspace/TrustMesh/deploy/ui-workflow-deliverable-only.png', Buffer.from(r.data, 'base64'));
  });
  // collapse again before files-tab assertions
  await evalJs(`(function(){
    const btns = [...document.querySelectorAll('button')];
    const n = btns.find(b => b.innerText && b.innerText.includes('剧本创作'));
    if (n) n.click(); return 'ok';
  })()`);
  await sleep(800);
  // drill down through folder levels until only files remain
  for (let depth = 0; depth < 5; depth++) {
    const res = await evalJs(`(function(){
      try {
        const rows = [...document.querySelectorAll('.ant-table-row')];
        for (const r of rows) {
          if (!(r.innerText && r.innerText.includes('文件夹'))) continue;
          const spans = [...r.querySelectorAll('span')].filter(s => (s.getAttribute('style') || '').includes('cursor'));
          if (spans.length) { spans[0].click(); return 'clicked-span'; }
          r.click(); return 'clicked-row';
        }
        return 'no-folder';
      } catch (e) { return 'EXC:' + e.message; }
    })()`);
    console.log('DRILL', depth, '=', res);
    if (res !== 'clicked-span' && res !== 'clicked-row') break;
    await sleep(2500);
  }
  await sleep(1500);
  const text1 = await evalJs('document.body.innerText') || '';
  console.log('BADGE 交付 x', (text1.match(/交付/g) || []).length, '| 过程 x', (text1.match(/过程/g) || []).length);
  console.log('HAS 定稿md =', text1.includes('定稿交付版_20260902.md'), '| HAS 07-draft =', text1.includes('07-draft-full.md'));
  await send('Page.captureScreenshot').then(r => {
    if (r && r.data) fs.writeFileSync('D:/AIWorkspace/TrustMesh/deploy/ui-badges-project-files.png', Buffer.from(r.data, 'base64'));
  });

  // task detail page deliverables
  await send('Page.navigate', { url: `http://localhost:5174/tasks/${TASK}` });
  await sleep(5000);
  const text2 = await evalJs('document.body.innerText') || '';
  console.log('TASK BADGE 交付 x', (text2.match(/交付/g) || []).length, '| 过程 x', (text2.match(/过程/g) || []).length);
  await send('Page.captureScreenshot').then(r => {
    if (r && r.data) fs.writeFileSync('D:/AIWorkspace/TrustMesh/deploy/ui-badges-task.png', Buffer.from(r.data, 'base64'));
  });

  ws.close(); edge.kill(); process.exit(0);
})().catch(e => { console.error('ERR', e.message); process.exit(1); });
