import { CanvasTexture, LinearFilter, SRGBColorSpace, type Texture } from 'three'
import { roleTintFor } from './officeLayout'

// ─── Canvas 贴图生成 ───
// 为什么不用 drei 的 <Text>：
//   1. 它默认使用 Roboto 字体（从 CDN 加载），离线环境会失败
//   2. 默认字体不含中文字形，Agent 名字（中文）会渲染成豆腐块
// 改用 Canvas 2D 绘制文字再转贴图：支持中文、零网络依赖、可缓存。

const FONT_FAMILY = '"PingFang SC", "Microsoft YaHei", "Segoe UI", sans-serif'

/** 文字贴图缓存：同文本+同样式只生成一次 */
const labelCache = new Map<string, CanvasTexture>()

export interface LabelStyle {
  fontSize?: number
  color?: string
  background?: string
  borderColor?: string
  paddingX?: number
  paddingY?: number
  radius?: number
  bold?: boolean
  maxWidth?: number
}

function roundRect(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number,
) {
  const rr = Math.min(r, w / 2, h / 2)
  ctx.beginPath()
  ctx.moveTo(x + rr, y)
  ctx.arcTo(x + w, y, x + w, y + h, rr)
  ctx.arcTo(x + w, y + h, x, y + h, rr)
  ctx.arcTo(x, y + h, x, y, rr)
  ctx.arcTo(x, y, x + w, y, rr)
  ctx.closePath()
}

/**
 * 生成一张居中的文字贴图（用于名牌、标牌）。
 * 返回的贴图尺寸为 2 的幂次对齐，便于 GPU 采样。
 */
export function makeLabelTexture(text: string, style: LabelStyle = {}): CanvasTexture {
  const {
    fontSize = 48,
    color = '#ffffff',
    background = 'rgba(10,10,18,0.72)',
    borderColor = 'rgba(255,255,255,0.18)',
    paddingX = 24,
    paddingY = 14,
    radius = 16,
    bold = false,
    maxWidth = 512,
  } = style

  // maxWidth 参与缓存 key：同一段文本在不同最大宽度下裁剪结果不同
  const cacheKey = `${text}|${fontSize}|${color}|${background}|${borderColor}|${bold}|${maxWidth}`
  const cached = labelCache.get(cacheKey)
  if (cached) return cached

  const canvas = document.createElement('canvas')
  const ctx = canvas.getContext('2d')!

  // 先用大画布量文字宽度，再裁剪
  const measureCanvas = document.createElement('canvas')
  const mctx = measureCanvas.getContext('2d')!
  mctx.font = `${bold ? '600 ' : ''}${fontSize}px ${FONT_FAMILY}`

  // 超宽文本：逐字裁剪并加省略号。
  // 此前只是把宽度 clamp 到 maxWidth，但绘制时仍画完整文本 —— 超出画布的部分
  // 直接被裁掉且没有省略号，看起来就是「消息展示不全」。这里保证永不裁切。
  const limit = maxWidth - paddingX * 2
  let display = text
  if (mctx.measureText(text).width > limit) {
    display = text
    while (display.length > 1 && mctx.measureText(`${display}…`).width > limit) {
      display = display.slice(0, -1)
    }
    display = `${display}…`
  }
  const textWidth = Math.min(mctx.measureText(display).width, limit)

  const w = Math.ceil(textWidth + paddingX * 2)
  const h = Math.ceil(fontSize + paddingY * 2)
  canvas.width = w
  canvas.height = h

  if (background !== 'transparent') {
    ctx.fillStyle = background
    roundRect(ctx, 0, 0, w, h, radius)
    ctx.fill()
  }
  if (borderColor !== 'transparent') {
    ctx.strokeStyle = borderColor
    ctx.lineWidth = 2
    roundRect(ctx, 1, 1, w - 2, h - 2, radius)
    ctx.stroke()
  }

  ctx.font = `${bold ? '600 ' : ''}${fontSize}px ${FONT_FAMILY}`
  ctx.fillStyle = color
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText(display, w / 2, h / 2 + 1)

  const texture = new CanvasTexture(canvas)
  texture.colorSpace = SRGBColorSpace
  texture.minFilter = LinearFilter
  texture.magFilter = LinearFilter
  texture.needsUpdate = true
  // 供 billboard 按比例还原尺寸
  texture.userData = { aspect: w / h }

  labelCache.set(cacheKey, texture)
  return texture
}

/**
 * 多行气泡贴图。
 * 单行版（makeLabelTexture）在长消息下会被压成细条：宽度上限会反过来压缩高度，
 * 字高只剩几个像素根本看不清。这里改为按字符换行 + 限制行数，气泡更方、字号更大。
 */
export interface BubbleStyle {
  fontSize?: number
  lineHeight?: number
  /** 最多显示几行，超出以省略号收尾 */
  maxLines?: number
  /** 单行文本宽度上限（px）：决定每行字数，进而决定场景里字号的实际大小 */
  maxLineWidth?: number
  color?: string
  background?: string
  borderColor?: string
  paddingX?: number
  paddingY?: number
  radius?: number
}

export function makeBubbleTexture(text: string, style: BubbleStyle = {}): CanvasTexture {
  const {
    fontSize = 40,
    lineHeight = 50,
    maxLines = 4,
    maxLineWidth = 470,
    color = '#e8e8f4',
    background = 'rgba(20,20,34,0.92)',
    borderColor = 'rgba(255,255,255,0.22)',
    paddingX = 20,
    paddingY = 15,
    radius = 16,
  } = style

  const cacheKey = `bubble|${text}|${fontSize}|${lineHeight}|${maxLines}|${maxLineWidth}|${color}|${background}|${borderColor}`
  const cached = labelCache.get(cacheKey)
  if (cached) return cached

  const measureCtx = document.createElement('canvas').getContext('2d')!
  const font = `${fontSize}px ${FONT_FAMILY}`
  measureCtx.font = font

  // 逐字换行（中文无空格按字符断行；英文/数字同样按字符，够用）
  const chars = [...text]
  const lines: string[] = []
  let cur = ''
  for (let i = 0; i < chars.length; i++) {
    const next = cur + chars[i]
    if (measureCtx.measureText(next).width > maxLineWidth && cur) {
      lines.push(cur)
      cur = chars[i]!
      if (lines.length === maxLines) break
    } else {
      cur = next
    }
  }
  // 已到行数上限但仍有剩余字符 → 省略号收尾（完整文本由页面「最新动态」卡片呈现）
  const consumed = lines.join('').length + cur.length
  if (cur && lines.length < maxLines) lines.push(cur)
  if (consumed < chars.length && lines.length > 0) {
    let last = lines[lines.length - 1]!
    while (last.length > 1 && measureCtx.measureText(`${last}…`).width > maxLineWidth) {
      last = last.slice(0, -1)
    }
    lines[lines.length - 1] = `${last}…`
  }

  const textW = Math.max(...lines.map((l) => measureCtx.measureText(l).width))
  const w = Math.ceil(textW + paddingX * 2)
  const h = Math.ceil(lines.length * lineHeight + paddingY * 2)

  const canvas = document.createElement('canvas')
  canvas.width = w
  canvas.height = h
  const ctx = canvas.getContext('2d')!

  if (background !== 'transparent') {
    ctx.fillStyle = background
    roundRect(ctx, 0, 0, w, h, radius)
    ctx.fill()
  }
  if (borderColor !== 'transparent') {
    ctx.strokeStyle = borderColor
    ctx.lineWidth = 3
    roundRect(ctx, 1.5, 1.5, w - 3, h - 3, radius)
    ctx.stroke()
  }

  ctx.font = font
  ctx.fillStyle = color
  ctx.textAlign = 'left'
  ctx.textBaseline = 'middle'
  lines.forEach((line, i) => {
    ctx.fillText(line, paddingX, paddingY + i * lineHeight + lineHeight / 2)
  })

  const texture = new CanvasTexture(canvas)
  texture.colorSpace = SRGBColorSpace
  texture.minFilter = LinearFilter
  texture.magFilter = LinearFilter
  texture.needsUpdate = true
  // aspect 用于按比例还原；width/height 用于按「像素→世界」固定比例还原，
  // 让不同长度的消息在场景里保持同一字号（否则行越多字越小）。
  texture.userData = { aspect: w / h, width: w, height: h }

  labelCache.set(cacheKey, texture)
  return texture
}

/** 贴图原始像素宽高（用于按固定比例还原世界尺寸） */
export interface TextureSize {
  aspect: number
  width: number
  height: number
}

export function textureSize(texture: Texture): TextureSize {
  const ud = texture.userData as Partial<TextureSize> | undefined
  return {
    aspect: typeof ud?.aspect === 'number' ? ud.aspect : 1,
    width: typeof ud?.width === 'number' ? ud.width : 0,
    height: typeof ud?.height === 'number' ? ud.height : 0,
  }
}

/** 贴图宽高比（用于还原 billboard 尺寸） */
export function textureAspect(texture: Texture): number {
  const aspect = (texture.userData as { aspect?: number } | undefined)?.aspect
  return typeof aspect === 'number' ? aspect : 1
}

/** 页面卸载时释放全部缓存贴图，避免内存泄漏 */
export function disposeLabelCache() {
  for (const texture of labelCache.values()) texture.dispose()
  labelCache.clear()
}

// ─── 角色精灵贴图 ───
// 阶段 2 先用程序化生成的人形占位图跑通全链路，
// 后续可整体替换为 AI 生成的插画，接口不变。
// 注意：不使用 ai-office-react 的 Spine 素材（MIT 不覆盖素材，商用有版权风险）。

const characterCache = new Map<string, CanvasTexture>()

const SKIN = '#f2c9a1'
const HAIR = '#2f2a28'
const TEX_W = 256
const TEX_H = 512

function drawCharacter(ctx: CanvasRenderingContext2D, tint: string, kind: string) {
  ctx.clearRect(0, 0, TEX_W, TEX_H)

  // 腿
  ctx.fillStyle = '#3a3f4c'
  ctx.beginPath()
  ctx.roundRect(96, 360, 26, 120, 10)
  ctx.roundRect(134, 360, 26, 120, 10)
  ctx.fill()

  // 身体（角色主色）
  ctx.fillStyle = tint
  ctx.beginPath()
  ctx.roundRect(78, 190, 100, 180, 28)
  ctx.fill()

  // 手臂
  ctx.fillStyle = tint
  ctx.beginPath()
  ctx.roundRect(56, 205, 26, 130, 13)
  ctx.roundRect(174, 205, 26, 130, 13)
  ctx.fill()

  // 手
  ctx.fillStyle = SKIN
  ctx.beginPath()
  ctx.arc(69, 345, 14, 0, Math.PI * 2)
  ctx.arc(187, 345, 14, 0, Math.PI * 2)
  ctx.fill()

  // 头
  ctx.fillStyle = SKIN
  ctx.beginPath()
  ctx.arc(128, 130, 54, 0, Math.PI * 2)
  ctx.fill()

  // 头发（上半圆）
  ctx.fillStyle = HAIR
  ctx.beginPath()
  ctx.arc(128, 124, 54, Math.PI, Math.PI * 2)
  ctx.fill()
  ctx.fillRect(74, 118, 108, 12)

  // 角色区分标识：PM 加肩章，reviewer 加眼镜
  if (kind === 'pm') {
    ctx.fillStyle = '#ffffff'
    ctx.globalAlpha = 0.85
    ctx.beginPath()
    ctx.roundRect(88, 196, 34, 10, 5)
    ctx.roundRect(134, 196, 34, 10, 5)
    ctx.fill()
    ctx.globalAlpha = 1
  } else if (kind === 'reviewer') {
    ctx.strokeStyle = '#2f2a28'
    ctx.lineWidth = 5
    ctx.beginPath()
    ctx.arc(110, 132, 17, 0, Math.PI * 2)
    ctx.arc(146, 132, 17, 0, Math.PI * 2)
    ctx.moveTo(127, 132)
    ctx.lineTo(129, 132)
    ctx.stroke()
  } else if (kind === 'developer') {
    // 胸口一道浅色横纹，像工装反光条
    ctx.fillStyle = '#ffffff'
    ctx.globalAlpha = 0.3
    ctx.fillRect(78, 250, 100, 9)
    ctx.globalAlpha = 1
  }
}

/**
 * 生成角色精灵贴图（按 role 分类，同 role 靠 tint 色相偏移区分个体）。
 * 返回带透明背景的竖版贴图。
 */
export function makeCharacterTexture(role: string, agentId: string): CanvasTexture {
  const cacheKey = `${role}|${agentId}`
  const cached = characterCache.get(cacheKey)
  if (cached) return cached

  const canvas = document.createElement('canvas')
  canvas.width = TEX_W
  canvas.height = TEX_H
  const ctx = canvas.getContext('2d')!

  drawCharacter(ctx, roleTintFor(role, agentId), role)

  const texture = new CanvasTexture(canvas)
  texture.colorSpace = SRGBColorSpace
  texture.minFilter = LinearFilter
  texture.magFilter = LinearFilter
  texture.needsUpdate = true
  texture.userData = { aspect: TEX_W / TEX_H }

  characterCache.set(cacheKey, texture)
  return texture
}

/** 释放全部角色贴图 */
export function disposeCharacterCache() {
  for (const texture of characterCache.values()) texture.dispose()
  characterCache.clear()
}

// ─── 显示器代码屏幕 ───
// 屏幕画的是真实代码片段：远景看是彩色条纹（有代码感），
// 聚焦拉近后能看清具体字符 —— 给懂的人一点小惊喜。
// 屏幕在场景里只有约 0.85×0.55 单位，故字号要够大、行数要少。

const codeCache = new Map<string, CanvasTexture>()

const CODE_SAMPLES: string[][] = [
  [
    'async function dispatch(todo) {',
    '  const agent = await pick(todo.role)',
    '  const res = await agent.run({',
    '    task: todo.title,',
    '    ctx: todo.context,',
    '  })',
    '  return res.output',
    '}',
  ],
  [
    'def plan_tasks(goal):',
    '    steps = decompose(goal)',
    '    for step in steps:',
    '        agent = match(step.role)',
    '        yield assign(agent, step)',
    '    return workflow(steps)',
  ],
  [
    '{',
    '  "role": "executor",',
    '  "model": "deepseek-v4",',
    '  "skills": ["tm-task-exec"],',
    '  "max_retries": 3,',
    '  "timeout": "30m"',
    '}',
  ],
  [
    '$ tm task create --project p1 \\',
    '    --agent code-agent \\',
    '    --title "整理情报"',
    '-> dispatching ...',
    '-> todo_1 assigned',
    '-> waiting progress',
  ],
]

/** 语法高亮配色（取 V2 多巴胺色板） */
const CODE_COLORS = {
  bg: '#0b0d16',
  bar: 'rgba(109,95,245,0.20)',
  dot: '#6d5ff5',
  lineNo: 'rgba(255,255,255,0.20)',
  keyword: '#c084fc',
  string: '#34d399',
  number: '#fbbf24',
  punct: 'rgba(255,255,255,0.42)',
  plain: '#a5b4fc',
}

// 全部用非全局正则：全局正则的 lastIndex 会在多次 test 之间串味，导致着色错乱
const KEYWORD_RE =
  /\b(async|await|function|const|return|def|for|in|yield|if|else|class|import|from|new|let|var)\b/
const STRING_RE = /^["'`]/
const NUMBER_RE = /^\d+$/
const PUNCT_RE = /^[{}()[\];,.:=><+\-*/|&\\]+$/

function hashString(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0
  return h
}

function tokenColor(token: string): string {
  if (KEYWORD_RE.test(token)) return CODE_COLORS.keyword
  if (STRING_RE.test(token)) return CODE_COLORS.string
  if (NUMBER_RE.test(token)) return CODE_COLORS.number
  if (PUNCT_RE.test(token)) return CODE_COLORS.punct
  return CODE_COLORS.plain
}

/**
 * 生成显示器代码屏贴图（同一 seed 只生成一次）。
 * 用 slot.id 作 seed，让每个工位的屏幕显示不同代码。
 */
export function makeCodeScreenTexture(seed: string): CanvasTexture {
  const cacheKey = `code|${seed}`
  const cached = codeCache.get(cacheKey)
  if (cached) return cached

  const lines = CODE_SAMPLES[hashString(seed) % CODE_SAMPLES.length]!
  const W = 512
  const H = 330
  const canvas = document.createElement('canvas')
  canvas.width = W
  canvas.height = H
  const ctx = canvas.getContext('2d')!

  ctx.fillStyle = CODE_COLORS.bg
  ctx.fillRect(0, 0, W, H)

  // 顶部窗口栏 + 三个圆点
  ctx.fillStyle = CODE_COLORS.bar
  ctx.fillRect(0, 0, W, 28)
  ctx.fillStyle = CODE_COLORS.dot
  for (let i = 0; i < 3; i++) {
    ctx.beginPath()
    ctx.arc(20 + i * 20, 14, 4.5, 0, Math.PI * 2)
    ctx.fill()
  }

  const fontSize = 21
  const lineHeight = 30
  ctx.font = `${fontSize}px "JetBrains Mono", ui-monospace, Menlo, Consolas, monospace`
  ctx.textBaseline = 'top'

  lines.forEach((line, i) => {
    const y = 40 + i * lineHeight
    if (y > H - fontSize) return
    ctx.fillStyle = CODE_COLORS.lineNo
    ctx.fillText(String(i + 1).padStart(2, ' '), 14, y)

    let x = 56
    for (const token of line.split(/(\s+)/)) {
      if (!token) continue
      const width = ctx.measureText(token).width
      if (!/^\s+$/.test(token)) {
        ctx.fillStyle = tokenColor(token)
        ctx.fillText(token, x, y)
      }
      x += width
    }
  })

  const texture = new CanvasTexture(canvas)
  texture.colorSpace = SRGBColorSpace
  texture.minFilter = LinearFilter
  texture.magFilter = LinearFilter
  texture.needsUpdate = true
  texture.userData = { aspect: W / H }

  codeCache.set(cacheKey, texture)
  return texture
}

/** 释放全部代码屏贴图 */
export function disposeCodeCache() {
  for (const texture of codeCache.values()) texture.dispose()
  codeCache.clear()
}
