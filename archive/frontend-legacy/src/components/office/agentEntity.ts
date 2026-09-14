import { Container, Graphics, Sprite, Text, type Renderer, type Texture } from 'pixi.js'
import { COLORS, SCENE_WIDTH, type DeskSlot } from './officeLayout'
import { getCharacterTexture } from './agentArt'
import type { AgentVisual } from './simulation'

// 每个 Agent 一个实体：椅子 → 像素小人 → 桌子+显示器（前景遮挡坐姿下半身）→
// 名牌 → 状态角标 → 气泡。容器整体 zIndex = 座位 y，保证多工位前后遮挡正确。

const CHAR_SCALE = 4
const TYPING_INTERVAL = 0.32

const STICKER_EMOJI: Record<string, string> = {
  celebrate: '🎉',
  failed: '😵',
}

const STATE_EMOJI: Record<string, string> = {
  thinking: '💭',
  asking: '❓',
}

const FONT_FAMILY = '"Segoe UI", "Microsoft YaHei", "PingFang SC", sans-serif'

export interface AgentEntityOptions {
  onTap: (agentId: string) => void
}

// 打字帧纹理需要渲染器；scene init 时注入，所有实体共享
let sharedRenderer: Renderer | null = null

export function setOfficeRenderer(renderer: Renderer | null) {
  sharedRenderer = renderer
}

export class AgentEntity extends Container {
  readonly agentId: string
  readonly paletteIndex: number
  readonly slotId: string

  private slot: DeskSlot

  private chair: Graphics
  private desk: Graphics
  private character: Sprite
  private namePlate: Text
  private sticker: Text
  private stateBadge: Text
  private bubbleContainer: Container
  private bubbleBg: Graphics
  private bubbleText: Text

  private textureReady = false
  private typingPhase = 0
  private typingTimer = 0
  private animateTyping = false

  constructor(agentId: string, slot: DeskSlot, paletteIndex: number, options: AgentEntityOptions) {
    super()
    this.agentId = agentId
    this.slot = slot
    this.slotId = slot.id
    this.paletteIndex = paletteIndex
    this.sortableChildren = true
    this.zIndex = slot.seatY
    this.eventMode = 'static'
    this.cursor = 'pointer'
    this.on('pointertap', () => options.onTap(agentId))

    // 椅子（人物身后）
    this.chair = new Graphics()
    this.chair.zIndex = -2
    this.addChild(this.chair)

    // 像素小人（纹理在渲染器注入后生成）
    this.character = new Sprite()
    this.character.anchor.set(0.5, 0)
    this.character.scale.set(CHAR_SCALE)
    this.character.zIndex = -1
    this.addChild(this.character)

    // 桌子 + 显示器（前景，遮挡下半身）
    this.desk = new Graphics()
    this.desk.zIndex = 1
    this.addChild(this.desk)

    // 名牌
    this.namePlate = new Text({
      text: '',
      style: {
        fontFamily: FONT_FAMILY,
        fontSize: 11,
        fill: 0x5a534b,
        fontWeight: '600',
      },
    })
    this.namePlate.anchor.set(0.5, 0)
    this.namePlate.zIndex = 2
    this.addChild(this.namePlate)

    // 瞬时贴纸（🎉 / 😵）
    this.sticker = new Text({ text: '', style: { fontSize: 18 } })
    this.sticker.anchor.set(0.5, 1)
    this.sticker.zIndex = 10
    this.addChild(this.sticker)

    // 状态角标（💭 / ❓ / 💤）
    this.stateBadge = new Text({ text: '', style: { fontSize: 15 } })
    this.stateBadge.anchor.set(0.5, 1)
    this.stateBadge.zIndex = 9
    this.addChild(this.stateBadge)

    // 气泡
    this.bubbleContainer = new Container()
    this.bubbleContainer.zIndex = 20
    this.bubbleBg = new Graphics()
    this.bubbleText = new Text({
      text: '',
      style: {
        fontFamily: FONT_FAMILY,
        fontSize: 12,
        fill: 0x2f2a26,
        wordWrap: true,
        wordWrapWidth: 190,
        breakWords: true,
        lineHeight: 16,
      },
    })
    this.bubbleContainer.addChild(this.bubbleBg, this.bubbleText)
    this.bubbleContainer.visible = false
    this.addChild(this.bubbleContainer)

    this.drawFurniture()
    this.layout()
  }

  private drawFurniture() {
    const { deskX, deskY, kind } = this.slot
    const deskW = kind === 'pm' ? 150 : 124

    // 椅子：靠背 + 座面
    this.chair
      .roundRect(this.slot.seatX - 20, this.slot.seatY + 30, 40, 34, 6)
      .fill({ color: COLORS.chair })
    this.chair
      .roundRect(this.slot.seatX - 20, this.slot.seatY + 8, 40, 14, 5)
      .fill({ color: 0x5c626e })

    // 桌面（顶面 + 前沿 + 桌腿）
    this.desk.rect(deskX, deskY + 30, deskW, 12).fill({ color: COLORS.deskTop })
    this.desk.rect(deskX, deskY + 42, deskW, 8).fill({ color: COLORS.deskFront })
    this.desk.rect(deskX + 4, deskY + 50, 6, 22).fill({ color: COLORS.deskSide })
    this.desk.rect(deskX + deskW - 10, deskY + 50, 6, 22).fill({ color: COLORS.deskSide })

    // 显示器（背对观众，立在桌面上，遮住人物胸部以下）
    const monW = 46
    const monH = 30
    const monX = this.slot.seatX - monW / 2
    this.desk.roundRect(monX, deskY + 10, monW, monH, 3).fill({ color: COLORS.monitorFrame })
    this.desk.rect(monX + 3, deskY + 13, monW - 6, monH - 10).fill({ color: COLORS.monitorScreen })
    this.desk.rect(this.slot.seatX - 8, deskY + 40, 16, 4).fill({ color: COLORS.monitorFrame })
    // 键盘
    this.desk.roundRect(this.slot.seatX - 18, deskY + 22, 36, 6, 2).fill({ color: 0x8b909a })
  }

  private layout() {
    const { seatX, seatY } = this.slot
    this.character.position.set(seatX, seatY)
    this.namePlate.position.set(seatX, seatY + 76)
    this.sticker.position.set(seatX + 26, seatY + 6)
    this.stateBadge.position.set(seatX - 26, seatY + 6)
    this.bubbleContainer.position.set(seatX, seatY - 14)
  }

  apply(visual: AgentVisual, now: number) {
    const offline = visual.presence === 'offline'
    this.character.visible = !offline
    this.chair.alpha = offline ? 0.4 : 1
    this.desk.alpha = offline ? 0.55 : 1

    if (this.namePlate.text !== visual.name) {
      this.namePlate.text = visual.name
    }

    if (!this.textureReady && sharedRenderer) {
      const texture = this.typingTexture(0)
      if (texture) {
        this.character.texture = texture
        this.textureReady = true
      }
    }

    // 瞬时贴纸（到点消失）
    const stickerActive = visual.sticker !== null && now < visual.stickerUntil
    this.sticker.text = stickerActive && visual.sticker ? STICKER_EMOJI[visual.sticker] ?? '' : ''
    this.sticker.visible = stickerActive

    // 状态角标
    const badge = offline ? '💤' : STATE_EMOJI[visual.state] ?? ''
    this.stateBadge.text = badge
    this.stateBadge.visible = badge !== ''

    // 气泡（bubbleUntil === 0 表示常驻，如等待用户回答）
    const bubbleVisible = !offline && !!visual.bubble && (visual.bubbleUntil === 0 || now < visual.bubbleUntil)
    if (bubbleVisible) {
      this.renderBubble(visual.bubble ?? '', visual.bubbleKind)
    }
    this.bubbleContainer.visible = bubbleVisible

    // 打字动画：working 或（busy 且无任务状态）
    this.animateTyping = !offline && (visual.state === 'working' || (visual.state === 'idle' && visual.presence === 'busy'))
  }

  private typingTexture(frame: number): Texture | null {
    if (!sharedRenderer) return null
    return getCharacterTexture(sharedRenderer, this.paletteIndex, frame)
  }

  /** 每帧推进：打字起伏 + 贴纸脉冲 */
  update(dt: number) {
    if (this.animateTyping) {
      this.typingTimer += dt
      if (this.typingTimer > TYPING_INTERVAL) {
        this.typingTimer = 0
        this.typingPhase = this.typingPhase === 0 ? 1 : 0
        const texture = this.typingTexture(this.typingPhase)
        if (texture) {
          this.character.texture = texture
          this.textureReady = true
          this.character.y = this.slot.seatY + (this.typingPhase === 1 ? 1.5 : 0)
        }
      }
    } else if (this.typingPhase !== 0) {
      this.typingPhase = 0
      this.character.y = this.slot.seatY
      const texture = this.typingTexture(0)
      if (texture) this.character.texture = texture
    }

    if (this.sticker.visible) {
      const pulse = 1 + Math.sin(performance.now() / 180) * 0.08
      this.sticker.scale.set(pulse)
    }
  }

  private renderBubble(text: string, kind: 'normal' | 'question') {
    this.bubbleText.text = text
    const w = Math.min(this.bubbleText.width + 20, 210)
    const h = this.bubbleText.height + 14
    const borderColor = kind === 'question' ? 0xd97706 : 0x8a8378
    const fillColor = kind === 'question' ? 0xfff7e6 : 0xfdfcfa

    this.bubbleBg.clear()
    this.bubbleBg.roundRect(0, -h - 10, w, h, 8).fill({ color: fillColor, alpha: 0.97 })
    this.bubbleBg.roundRect(0, -h - 10, w, h, 8).stroke({ color: borderColor, width: 1.5 })
    // 气泡尾巴
    this.bubbleBg.moveTo(-5, -10).lineTo(5, -10).lineTo(0, -2).fill({ color: fillColor })
    this.bubbleBg.moveTo(-5, -10).lineTo(0, -2).stroke({ color: borderColor, width: 1.5 })
    this.bubbleBg.moveTo(5, -10).lineTo(0, -2).stroke({ color: borderColor, width: 1.5 })

    this.bubbleText.position.set(10, -h - 3)

    // 气泡横向居中于座位，且不越出画布
    const { seatX, seatY } = this.slot
    const left = seatX - w / 2
    const clampedLeft = Math.max(8, Math.min(left, SCENE_WIDTH - w - 8))
    this.bubbleContainer.position.set(clampedLeft, seatY - 14)
  }
}
