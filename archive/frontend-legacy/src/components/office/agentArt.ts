import { Graphics, Texture } from 'pixi.js'
import type { Renderer } from 'pixi.js'
import { CHARACTER_PALETTES, type CharacterPalette } from './officeLayout'

// ─── 像素小人（代码生成，零外部资产） ───
// 16×15 像素矩阵的「背面坐姿」（面向显示器），通过 renderer.generateTexture
// 生成纹理并关闭平滑（nearest），放大后保持像素锐利。
// frame 0/1 为打字两帧（头部下沉 1px 产生敲击起伏），frame 2 为静止坐姿。

// 图例：H=头发 S=上衣 A=上衣暗部 K=皮肤 O=描边 .=透明
const BASE_ROWS = [
  '................',
  '......OOOO......',
  '.....OHHHHO.....',
  '....OHHHHHHO....',
  '....OHHHHHHO....',
  '....OHHHHHHO....',
  '...KOHHHHHHOK...',
  '....OHHHHHHO....',
  '....OSSSSSSO....',
  '..OSSSSSSSSSSO..',
  '..OSASSSSSASO...',
  '..OSSSSSSSSSSO..',
  '..OKSSSSSSSSKO..',
  '...OSSSSSSSSO...',
  '...OOOOOOOOOO...',
]

const TYPING_ROWS = [
  '................',
  '................',
  '......OOOO......',
  '.....OHHHHO.....',
  '....OHHHHHHO....',
  '....OHHHHHHO....',
  '....OHHHHHHO....',
  '...KOHHHHHHOK...',
  '....OHHHHHHO....',
  '....OSSSSSSO....',
  '..OSSSSSSSSSSO..',
  '..OSASSSSSASO...',
  '..OSSSSSSSSSSO..',
  '..OKSSSSSSSSKO..',
  '...OOOOOOOOOO...',
]

const FRAMES: string[][] = [BASE_ROWS, TYPING_ROWS]

const CELL = 1
export const CHARACTER_TEX_W = 16 * CELL
export const CHARACTER_TEX_H = 15 * CELL

const textureCache = new Map<string, Texture>()

// 像素字母码 → 调色板属性映射
const COLOR_KEYS: Record<string, keyof CharacterPalette> = {
  H: 'hair',
  S: 'shirt',
  A: 'accent',
  K: 'skin',
}

function colorFor(palette: CharacterPalette, key: string): number {
  if (key === 'O') return 0x2a2622
  const prop = COLOR_KEYS[key]
  return prop ? palette[prop] : 0xff00ff
}

function buildTexture(renderer: Renderer, paletteIndex: number, frameIndex: number): Texture {
  const palette = CHARACTER_PALETTES[paletteIndex % CHARACTER_PALETTES.length]
  const rows = FRAMES[frameIndex % FRAMES.length]

  const gfx = new Graphics()
  for (let y = 0; y < rows.length; y++) {
    const row = rows[y]
    for (let x = 0; x < row.length; x++) {
      const key = row[x]
      if (key === '.' || key === undefined) continue
      gfx.rect(x * CELL, y * CELL, CELL, CELL)
      gfx.fill({ color: colorFor(palette, key) })
    }
  }
  const texture = renderer.generateTexture({ target: gfx, resolution: 1 })
  texture.source.scaleMode = 'nearest'
  gfx.destroy()
  return texture
}

export function getCharacterTexture(renderer: Renderer, paletteIndex: number, frameIndex: number): Texture {
  const cacheKey = `${paletteIndex}:${frameIndex}`
  let texture = textureCache.get(cacheKey)
  if (!texture) {
    texture = buildTexture(renderer, paletteIndex, frameIndex)
    textureCache.set(cacheKey, texture)
  }
  return texture
}
