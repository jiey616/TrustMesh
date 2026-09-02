const fs = require('fs');
const path = require('path');

const srcNm = path.resolve('D:/AiWorkspace/TrustMesh/frontend/node_modules');
const dstNm = path.resolve('D:/AiWorkspace/TrustMesh/frontend-v2/node_modules');

function copyDir(src, dst) {
  fs.mkdirSync(dst, { recursive: true });
  for (const entry of fs.readdirSync(src, { withFileTypes: true })) {
    const s = path.join(src, entry.name);
    const d = path.join(dst, entry.name);
    if (entry.isDirectory()) {
      // skip nested node_modules to avoid cycles; top-level flat installs only
      if (entry.name === 'node_modules') {
        copyDir(s, d);
      } else {
        copyDir(s, d);
      }
    } else {
      fs.copyFileSync(s, d);
    }
  }
}

function collect(name) {
  const seen = new Set();
  function walk(n) {
    if (seen.has(n)) return;
    const pkgPath = path.join(srcNm, n);
    if (!fs.existsSync(path.join(pkgPath, 'package.json'))) return;
    seen.add(n);
    const pj = JSON.parse(fs.readFileSync(path.join(pkgPath, 'package.json'), 'utf8'));
    const all = { ...(pj.dependencies || {}), ...(pj.optionalDependencies || {}) };
    for (const d of Object.keys(all)) walk(d);
  }
  walk(name);
  return [...seen];
}

const roots = ['react-markdown', 'remark-gfm'];
const all = collect(roots[0]).concat(collect(roots[1])).filter((v, i, a) => a.indexOf(v) === i);

let copied = 0;
let skipped = 0;
const failed = [];
for (const pkg of all) {
  const src = path.join(srcNm, pkg);
  const dst = path.join(dstNm, pkg);
  if (!fs.existsSync(src)) {
    failed.push(pkg);
    continue;
  }
  try {
    if (fs.existsSync(dst)) {
      // package already present, still refresh for correctness
      fs.rmSync(dst, { recursive: true, force: true });
    }
    copyDir(src, dst);
    copied++;
  } catch (e) {
    failed.push(`${pkg}: ${e.message}`);
  }
}

console.log(`total=${all.length} copied=${copied} skipped=${skipped} failed=${failed.length}`);
if (failed.length) console.log('FAILED:\n' + failed.join('\n'));
