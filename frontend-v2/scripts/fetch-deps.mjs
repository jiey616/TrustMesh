// 递归解析 antd 依赖树并用 node fetch 下载 tarball（绕过慢速 npm 客户端）
// 用法: node fetch-deps.mjs <输出目录>
import { writeFile, mkdir } from 'node:fs/promises'
import path from 'node:path'

const OUT = process.argv[2] || 'C:/Users/Jiey/AppData/Local/Temp/antd-tarballs'
const REGISTRY = 'https://registry.npmjs.org/'
const cache = new Map() // name -> packument

async function getPackument(name) {
  if (cache.has(name)) return cache.get(name)
  const url = REGISTRY + name.replace('/', '%2F')
  const res = await fetch(url, { headers: { accept: 'application/json' } })
  if (!res.ok) throw new Error(`fetch ${name}: ${res.status}`)
  const data = await res.json()
  cache.set(name, data)
  return data
}

// 简化版 semver：把 "1.2.3" 转数字数组
function parseVer(v) {
  const m = /^(\d+)\.(\d+)\.(\d+)/.exec(v)
  return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : null
}
function cmp(a, b) {
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] - b[i]
  }
  return 0
}

// 判断版本是否满足 semver 范围（简化，支持 ^ ~ x.y.z >=x.y.z 和 * 常见形式）
function satisfies(verStr, range) {
  const v = parseVer(verStr)
  if (!v) return false
  const r = String(range).trim()
  if (!r || r === '*' || r === 'x' || r === 'latest') return true
  const m = r.match(/^(\^|~|>=|<=|>|<|=)?\s*v?(\d+)\.(\d+)\.(\d+)/)
  if (!m) {
    // 处理 "5.x" / "^5.1.x" 形式
    if (/x/i.test(r)) {
      const mx = r.match(/^(\^|~)?\s*(\d+)\.(\d+)?\.?x?/i)
      if (mx) {
        const major = Number(mx[2])
        const minor = mx[3] ? Number(mx[3]) : -1
        if (v[0] !== major) return false
        if (minor >= 0 && v[1] !== minor) return false
        return true
      }
      return false
    }
    return true // 未知范围，宽松通过
  }
  const op = m[1] || '='
  const target = [Number(m[2]), Number(m[3]), Number(m[4])]
  const c = cmp(v, target)
  switch (op) {
    case '=': return c === 0
    case '>': return c > 0
    case '>=': return c >= 0
    case '<': return c < 0
    case '<=': return c <= 0
    case '^':
      return v[0] === target[0] && cmp(v, target) >= 0
    case '~':
      return v[0] === target[0] && v[1] === target[1] && cmp(v, target) >= 0
    default: return c === 0
  }
}

// 从 packument 中选满足范围的最新稳定版本
function pickVersion(packument, range) {
  const versions = Object.keys(packument.versions)
    .filter((v) => !/[-+]/.test(v)) // 排除 pre-release
    .map(parseVer)
    .filter(Boolean)
    .sort(cmp)
  for (let i = versions.length - 1; i >= 0; i--) {
    const v = versions[i].join('.')
    if (satisfies(v, range)) return v
  }
  // 回退：用 dist-tags.latest
  return packument['dist-tags']?.latest
}

// BFS 遍历依赖树
const results = new Map() // name -> { version, tarball, dependencies }
const queue = [
  ['antd', '^5.24.0'],
  ['@ant-design/icons', '^5.6.1'],
  ['dayjs', '^1.11.13'],
]
const visited = new Set()

while (queue.length) {
  const [name, range] = queue.shift()
  if (visited.has(name)) continue
  visited.add(name)

  const packument = await getPackument(name)
  const version = pickVersion(packument, range)
  if (!version) {
    console.warn(`WARN: no version for ${name}@${range}`)
    continue
  }
  const meta = packument.versions[version]
  if (!meta || !meta.dist?.tarball) {
    console.warn(`WARN: no tarball for ${name}@${version}`)
    continue
  }

  results.set(name, {
    version,
    tarball: meta.dist.tarball,
    dependencies: { ...meta.dependencies, ...meta.peerDependencies },
  })

  for (const [depName, depRange] of Object.entries(results.get(name).dependencies)) {
    if (!visited.has(depName)) queue.push([depName, depRange])
  }
}

// 下载所有 tarball
await mkdir(OUT, { recursive: true })
console.log(`Total packages to download: ${results.size}`)
let ok = 0
const failed = []
for (const [name, meta] of results) {
  const filename = name.replace('/', '__') + '@' + meta.version + '.tgz'
  const filepath = path.join(OUT, filename)
  try {
    const res = await fetch(meta.tarball)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const buf = Buffer.from(await res.arrayBuffer())
    await writeFile(filepath, buf)
    ok++
    if (ok % 20 === 0) console.log(`downloaded ${ok}/${results.size}`)
  } catch (e) {
    failed.push([name, meta.tarball, e.message])
    console.warn(`FAIL ${name}: ${e.message}`)
  }
}

console.log(`\nDone: ${ok}/${results.size} downloaded`)
if (failed.length) {
  console.log('Failed list:')
  for (const [name, url, msg] of failed) console.log(`  ${name}: ${url} (${msg})`)
}

// 保存版本信息清单
await writeFile(
  path.join(OUT, '_versions.json'),
  JSON.stringify(Object.fromEntries([...results.entries()].map(([n, m]) => [n, m.version])), null, 2),
)
console.log('versions saved to', path.join(OUT, '_versions.json'))
