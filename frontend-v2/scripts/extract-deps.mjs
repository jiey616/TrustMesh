// 纯 node 解压 .tgz（不依赖 tar.exe / 沙箱兼容）
import { readFileSync, writeFileSync, mkdirSync, rmSync, existsSync } from 'node:fs'
import { gunzipSync } from 'node:zlib'
import path from 'node:path'

const DIR = 'C:/Users/Jiey/AppData/Local/Temp/antd-tarballs'
const NM = 'D:/AiWorkspace/TrustMesh/frontend-v2/node_modules'

// 解析 tar 归档（UStar 格式），返回 [{ name, content }]
function parseTar(buf) {
  const entries = []
  let offset = 0
  while (offset + 512 <= buf.length) {
    const header = buf.subarray(offset, offset + 512)
    // 全零 block = 结束
    if (header.every((b) => b === 0)) break
    let name = header.subarray(0, 100).toString('utf8').replace(/\0/g, '')
    const prefix = header.subarray(345, 500).toString('utf8').replace(/\0/g, '')
    if (prefix) name = prefix + '/' + name
    const sizeStr = header.subarray(124, 136).toString('utf8').replace(/\0/g, '').trim()
    const size = parseInt(sizeStr, 8) || 0
    const type = header.subarray(156, 157).toString('utf8')
    const dataStart = offset + 512
    const data = buf.subarray(dataStart, dataStart + size)
    // 跳过目录类型，只保留普通文件（type '0' 或空）
    if (type !== '5' && name && !name.endsWith('/')) {
      entries.push({ name, content: Buffer.from(data) })
    }
    offset = dataStart + Math.ceil(size / 512) * 512
  }
  return entries
}

const versions = JSON.parse(readFileSync(path.join(DIR, '_versions.json'), 'utf8'))
let count = 0
let fail = 0

for (const [name, version] of Object.entries(versions)) {
  const tgz = path.join(DIR, name.replace('/', '__') + '@' + version + '.tgz')
  const dest = path.join(NM, name)
  try {
    const raw = readFileSync(tgz)
    const gunzipped = gunzipSync(raw)
    const entries = parseTar(gunzipped)
    // 移除 package/ 前缀
    for (const entry of entries) {
      const rel = entry.name.startsWith('package/') ? entry.name.slice('package/'.length) : entry.name
      if (!rel) continue
      const filePath = path.join(dest, rel)
      mkdirSync(path.dirname(filePath), { recursive: true })
      writeFileSync(filePath, entry.content)
    }
    count++
  } catch (e) {
    fail++
    console.log('FAIL', name, e.message.slice(0, 60))
  }
}
console.log('extracted:', count, '/ failed:', fail, '/ total:', Object.keys(versions).length)
