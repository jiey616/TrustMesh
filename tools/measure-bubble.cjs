// 解析 PNG 找到橙色气泡的像素包围盒（用于验证气泡在屏幕上的真实尺寸）
// 用法: node tools/measure-bubble.cjs <png路径>
const fs = require('fs')
const zlib = require('zlib')

function decodePNG(path) {
  const buf = fs.readFileSync(path)
  let pos = 8
  let w = 0, h = 0, colorType = 6
  const idat = []
  while (pos < buf.length) {
    const len = buf.readUInt32BE(pos)
    const type = buf.toString('ascii', pos + 4, pos + 8)
    const data = buf.slice(pos + 8, pos + 8 + len)
    if (type === 'IHDR') {
      w = data.readUInt32BE(0)
      h = data.readUInt32BE(4)
      colorType = data[9]
    } else if (type === 'IDAT') idat.push(data)
    else if (type === 'IEND') break
    pos += 12 + len
  }
  const raw = zlib.inflateSync(Buffer.concat(idat))
  const channels = colorType === 6 ? 4 : colorType === 2 ? 3 : 1
  const stride = w * channels
  const out = Buffer.alloc(h * stride)
  let rp = 0
  for (let y = 0; y < h; y++) {
    const ft = raw[rp++]
    const line = raw.slice(rp, rp + stride)
    rp += stride
    const prev = y > 0 ? out.slice((y - 1) * stride, y * stride) : Buffer.alloc(stride)
    const cur = out.slice(y * stride, (y + 1) * stride)
    for (let x = 0; x < stride; x++) {
      const a = x >= channels ? cur[x - channels] : 0
      const b = prev[x]
      const c = x >= channels ? prev[x - channels] : 0
      let v = line[x]
      if (ft === 1) v = (v + a) & 255
      else if (ft === 2) v = (v + b) & 255
      else if (ft === 3) v = (v + ((a + b) >> 1)) & 255
      else if (ft === 4) {
        const p = a + b - c
        const pa = Math.abs(p - a), pb = Math.abs(p - b), pc = Math.abs(p - c)
        v = (v + (pa <= pb && pa <= pc ? a : pb <= pc ? b : c)) & 255
      }
      cur[x] = v
    }
  }
  return { w, h, channels, data: out }
}

const { w, h, channels, data } = decodePNG(process.argv[2])

// 可传入裁剪范围 "x0,y0,x1,y1"，把统计限制在 3D 画布内，排除页面上的橙色 UI
const clip = process.argv[3]
  ? process.argv[3].split(',').map(Number)
  : null
const X0 = clip ? clip[0] : 0
const Y0 = clip ? clip[1] : 0
const X1 = clip ? clip[2] : w - 1
const Y1 = clip ? clip[3] : h - 1

// 橙色 #f59e0b(245,158,11)；半透明叠加会偏暗，放宽范围
let minX = 1e9, maxX = -1, minY = 1e9, maxY = -1, count = 0
// 每行橙色像素数 → 估算文字行
for (let y = Math.max(0, Y0); y <= Math.min(h - 1, Y1); y++) {
  for (let x = Math.max(0, X0); x <= Math.min(w - 1, X1); x++) {
    const i = (y * w + x) * channels
    const r = data[i], g = data[i + 1], b = data[i + 2]
    if (r > 210 && r < 256 && g > 120 && g < 185 && b < 60) {
      count++
      if (x < minX) minX = x
      if (x > maxX) maxX = x
      if (y < minY) minY = y
      if (y > maxY) maxY = y
    }
  }
}

console.log(JSON.stringify({
  image: `${w}x${h}`,
  clip: clip || null,
  orangePixels: count,
  box: count ? { x: minX, y: minY, w: maxX - minX + 1, h: maxY - minY + 1 } : null,
}, null, 2))

// 橙色像素按 y 的直方图（每 25px 一档），用于判断橙色到底来自页面 UI 还是 3D 画布
if (count) {
  const bucket = 25
  const hist = []
  for (let y = 0; y < h; y++) {
    let n = 0
    for (let x = 0; x < w; x++) {
      const i = (y * w + x) * channels
      const r = data[i], g = data[i + 1], b = data[i + 2]
      if (r > 210 && r < 256 && g > 120 && g < 185 && b < 60) n++
    }
    const bi = Math.floor(y / bucket)
    hist[bi] = (hist[bi] || 0) + n
  }
  const lines = []
  hist.forEach((n, i) => {
    if (n > 0) lines.push(`y ${i * bucket}-${i * bucket + bucket - 1}: ${n}`)
  })
  console.log('orangeByY:\n' + lines.join('\n'))
}

// 文字行估算：气泡内深色（#0a0a12 文字）像素按行聚合
if (count) {
  const bands = []
  let start = -1
  for (let y = minY; y <= maxY; y++) {
    let dark = 0
    for (let x = minX; x <= maxX; x++) {
      const i = (y * w + x) * channels
      const r = data[i], g = data[i + 1], b = data[i + 2]
      if (r < 70 && g < 70 && b < 80) dark++
    }
    const isText = dark > 3
    if (isText && start < 0) start = y
    if (!isText && start >= 0) { bands.push([start, y - 1]); start = -1 }
  }
  if (start >= 0) bands.push([start, maxY])
  console.log('textBands:', JSON.stringify(bands.map(([a, b]) => ({ top: a, height: b - a + 1 }))))
}
