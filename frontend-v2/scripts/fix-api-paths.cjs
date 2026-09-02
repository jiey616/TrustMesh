const fs = require('fs')
const path = require('path')

function walk(dir) {
  let results = []
  for (const f of fs.readdirSync(dir)) {
    const p = path.join(dir, f)
    const st = fs.statSync(p)
    if (st.isDirectory()) {
      if (['node_modules', 'dist-v2', '.vite-cache', 'build-output'].includes(f)) continue
      results = results.concat(walk(p))
    } else if (/\.(ts|tsx)$/.test(f)) results.push(p)
  }
  return results
}

const files = walk('src')
let count = 0
for (const f of files) {
  const buf = fs.readFileSync(f)
  const text = buf.toString('utf8')
  // apiClient.get(`api/v1/...`) -> apiClient.get(`/api/v1/...`)
  const newText = text.replace(/(apiClient\.(?:get|post|patch|delete|put)\(`)(api\/v1\/)/g, '$1/$2')
  if (newText !== text) {
    fs.writeFileSync(f, newText, 'utf8')
    count++
    console.log('fixed:', f)
  }
}
console.log('修复文件数:', count)
