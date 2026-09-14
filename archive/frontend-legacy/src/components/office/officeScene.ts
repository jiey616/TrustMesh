import { Application, Container, Graphics, Text } from 'pixi.js'
import { assignDesks, COLORS, DESK_SLOTS, MEETING_TABLE, SCENE_HEIGHT, SCENE_WIDTH, seatIndexOf } from './officeLayout'
import { AgentEntity, setOfficeRenderer } from './agentEntity'
import { officeSimulation, type AgentVisual } from './simulation'

// ─── 办公室场景 ───
// PixiJS 渲染层：地面/家具/会议室静态装饰 + 每个 Agent 一个 AgentEntity。
// 数据只来自 officeSimulation（真实 SSE 驱动），场景自身不含任何业务判断。

const FONT_FAMILY = '"Segoe UI", "Microsoft YaHei", "PingFang SC", sans-serif'

export interface OfficeSceneOptions {
  onAgentClick: (agentId: string) => void
}

export class OfficeScene {
  private app: Application | null = null
  private world: Container | null = null
  private officeLayer: Container | null = null
  private entities = new Map<string, AgentEntity>()
  private unsubscribe: (() => void) | null = null
  private options: OfficeSceneOptions

  constructor(options: OfficeSceneOptions) {
    this.options = options
  }

  async init(host: HTMLElement, width: number, height: number) {
    const app = new Application()
    await app.init({
      width,
      height,
      backgroundColor: COLORS.floor,
      antialias: true,
      resolution: window.devicePixelRatio || 1,
      autoDensity: true,
    })
    this.app = app
    host.appendChild(app.canvas)

    this.world = new Container()
    app.stage.addChild(this.world)

    setOfficeRenderer(app.renderer)
    this.drawMap(this.world)

    this.officeLayer = new Container()
    this.officeLayer.sortableChildren = true
    this.world.addChild(this.officeLayer)

    this.syncAgents(officeSimulation.getState())
    this.unsubscribe = officeSimulation.subscribe(() => {
      this.syncAgents(officeSimulation.getState())
    })

    app.ticker.add(this.onTick)
    this.resize(width, height)
  }

  resize(width: number, height: number) {
    if (!this.app || !this.world) return
    this.app.renderer.resize(width, height)
    const scale = Math.min(width / SCENE_WIDTH, height / SCENE_HEIGHT)
    this.world.scale.set(scale)
    this.world.position.set(
      (width - SCENE_WIDTH * scale) / 2,
      (height - SCENE_HEIGHT * scale) / 2,
    )
  }

  setPaused(paused: boolean) {
    if (!this.app) return
    if (paused) {
      this.app.ticker.stop()
    } else {
      this.app.ticker.start()
    }
  }

  destroy() {
    this.unsubscribe?.()
    this.unsubscribe = null
    if (this.app) {
      this.app.ticker.remove(this.onTick)
      this.app.destroy(true, { children: true })
    }
    this.app = null
    this.world = null
    this.officeLayer = null
    this.entities.clear()
    setOfficeRenderer(null)
  }

  private onTick = (ticker: { deltaMS: number }) => {
    const dt = Math.min(ticker.deltaMS / 1000, 0.05)
    for (const entity of this.entities.values()) {
      entity.update(dt)
    }
  }

  /** 同步模拟状态 → 实体集合（新增/删除/刷新） */
  private syncAgents(visuals: AgentVisual[]) {
    if (!this.officeLayer) return
    const seen = new Set<string>()
    const assignment = assignDesks(visuals)

    const now = Date.now()
    for (const visual of visuals) {
      seen.add(visual.id)
      try {
        let entity = this.entities.get(visual.id)
        const slot = assignment.get(visual.id) ?? DESK_SLOTS[DESK_SLOTS.length - 1]
        if (!entity || entity.slotId !== slot.id) {
          // 新成员，或花名册变化导致座位重排 → 重建实体
          if (entity) {
            entity.destroy({ children: true })
          }
          entity = new AgentEntity(visual.id, slot, seatIndexOf(visual.id), {
            onTap: this.options.onAgentClick,
          })
          this.officeLayer.addChild(entity)
          this.entities.set(visual.id, entity)
        }
        entity.apply(visual, now)
      } catch (err) {
        // 单个实体异常不拖垮整个办公室
        console.error('[office] entity sync failed', visual.id, err)
      }
    }

    for (const [id, entity] of this.entities) {
      if (!seen.has(id)) {
        entity.destroy({ children: true })
        this.entities.delete(id)
      }
    }
  }

  private drawMap(parent: Container) {
    const map = new Container()

    // 地板 + 细网格
    const floor = new Graphics()
    floor.rect(0, 0, SCENE_WIDTH, SCENE_HEIGHT).fill({ color: COLORS.floor })
    for (let x = 0; x <= SCENE_WIDTH; x += 48) {
      floor.moveTo(x, 0).lineTo(x, SCENE_HEIGHT).stroke({ color: COLORS.floorLine, width: 1 })
    }
    for (let y = 0; y <= SCENE_HEIGHT; y += 48) {
      floor.moveTo(0, y).lineTo(SCENE_WIDTH, y).stroke({ color: COLORS.floorLine, width: 1 })
    }
    map.addChild(floor)

    // 顶部墙沿
    const wall = new Graphics()
    wall.rect(0, 0, SCENE_WIDTH, 46).fill({ color: COLORS.wallTop })
    wall.rect(0, 46, SCENE_WIDTH, 8).fill({ color: COLORS.wallSide })
    map.addChild(wall)

    // 会议室区域（M2 空间化占位：桌 + 椅 + 铭牌）
    const meeting = new Graphics()
    meeting.roundRect(MEETING_TABLE.x, MEETING_TABLE.y, MEETING_TABLE.w, MEETING_TABLE.h, 14).fill({ color: COLORS.rug })
    meeting.roundRect(MEETING_TABLE.x + 34, MEETING_TABLE.y + 34, MEETING_TABLE.w - 68, 62, 10).fill({ color: COLORS.deskTop })
    meeting.roundRect(MEETING_TABLE.x + 34, MEETING_TABLE.y + 34, MEETING_TABLE.w - 68, 62, 10).stroke({ color: COLORS.deskFront, width: 3 })
    for (let i = 0; i < 4; i++) {
      const cx = MEETING_TABLE.x + 44 + i * ((MEETING_TABLE.w - 88) / 3)
      const cy = MEETING_TABLE.y + (i % 2 === 0 ? 18 : 100)
      meeting.roundRect(cx - 12, cy, 24, 12, 5).fill({ color: COLORS.chair })
    }
    map.addChild(meeting)
    const meetingLabel = new Text({
      text: '会议室（M2）',
      style: { fontFamily: FONT_FAMILY, fontSize: 12, fill: 0x8a9a8f, fontWeight: '600' },
    })
    meetingLabel.position.set(MEETING_TABLE.x + MEETING_TABLE.w / 2 - 44, MEETING_TABLE.y + 8)
    map.addChild(meetingLabel)

    // PM 独立办公区地毯
    const pmRug = new Graphics()
    pmRug.roundRect(76, 86, 190, 110, 12).fill({ color: 0xe7ddd2 })
    map.addChild(pmRug)
    const pmLabel = new Text({
      text: 'PM 办公室',
      style: { fontFamily: FONT_FAMILY, fontSize: 12, fill: 0xa3927e, fontWeight: '600' },
    })
    pmLabel.position.set(112, 60)
    map.addChild(pmLabel)

    // 绿植点缀
    for (const [px, py] of [[40, 560], [916, 560], [916, 240]] as const) {
      const plant = new Graphics()
      plant.circle(px, py, 14).fill({ color: COLORS.plant })
      plant.circle(px - 8, py + 6, 9).fill({ color: 0x4f7a4b })
      plant.circle(px + 8, py + 6, 9).fill({ color: 0x6d9a66 })
      plant.roundRect(px - 9, py + 10, 18, 12, 3).fill({ color: COLORS.pot })
      map.addChild(plant)
    }

    parent.addChild(map)
  }
}
