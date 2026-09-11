# TrustMesh 技能广场（Skill Marketplace）实现方案

> 调研日期：2026-08-19｜基于 TrustMesh 当前代码（`backend/`、`frontend/`）实地核查
> 目标：在 TrustMesh 上提供「技能广场」——用户浏览/选择平台托管技能，配置到智能体，运行时智能体通过「技能分发协议」调用平台侧的技能完成执行。

---

## 0. 目标与关键架构决策

你的诉求里有一句关键话：**"智能体运行的时候直接通过 skill 分发协议调用平台上的 skill 去执行"**。

这句话定义了两种实现路线，必须先在方案上分清：

| 路线 | 含义 | skill 内容是否落到 Agent 节点 | 内容对用户是否可见 | 复用现有代码 |
|---|---|---|---|---|
| **A. 本地安装型**（沿用现状） | 技能包 zip 通过 `capability.set` 下发到 hermes 节点，Agent 在本地读 SKILL.md 执行 | 是（节点本地有明文） | 可见 | 大量复用 |
| **B. 远程执行型**（skill-as-API） | skill 正文始终留在平台/执行节点，Agent 运行时发 `skill.invoke` 回平台，平台执行后回传结果 | 否（只在平台侧） | **不可见** | 需扩建 |

结合上一轮你关心的「skill 包内容对用户不可见」，以及本句「调用**平台上**的 skill 去执行」，**路线 B 才是真正符合意图的形态**：skill 永不离开平台 → 天然防逆向 + 集中治理 + 可撤销/可计费。

**推荐落地策略**：分阶段。先用路线 A 把「广场目录 + 一键分发到节点」跑通（低成本、复用现状、立刻可用），再上路线 B 的远程执行运行时（实现内容不可见这一核心价值）。两阶段共用同一套「技能目录数据层」，避免返工。

---

## 1. 现状盘点（已核实）

### 1.1 平台定位
TrustMesh 是**多智能体编排枢纽，本身不跑 LLM 推理**。真正的智能在外部 Agent 节点（hermes / clawsynapse 节点，经 NATS + HTTP 接入）。平台通过 `clawsynapse.Client.Publish` 派发消息、通过 `POST /webhook/clawsynapse` 接收 Agent 回调；唯一的 LLM 循环是可选的 `internal/assistant`（平台 copilot，非任务执行路径）。

### 1.2 现有「技能」三层（地基已具备）
1. **技能包目录**：`skills/` 下有 4 个 `SKILL.md`（`tm-task-exec`、`tm-task-plan`、`tm-meeting-host`、`tm-meeting-participant`），格式即 Anthropic Agent Skills 标准（YAML frontmatter + Markdown + 可选 scripts）。
2. **运行时技能下发链路**：`clawsynapse.Client.UploadSkillFile`（zip→节点）+ `SetCapabilities({target:"skill", action:"add", fileIds})` → hermes adapter 安装到 managed skill 目录。**目前仅对 `product=="hermes"` 节点开放**（`handler/agent.go`）。
3. **运行时触发**：派发 `todo.assigned` 时带 `ExecBrief.MustUseSkill`（默认 `tm-task-exec`），Agent 收到后本地读取同名 SKILL.md 执行（`protocol/clawsynapse.go:174`）。

### 1.3 现有「市场」模板（直接克隆对象）
- `store/store_market.go`（`MarketStore`：读 `roles_index.json` + 部门/角色过滤/详情/打 zip）
- `model/market.go`（RolesIndex / MarketRole / 响应类型）
- `handler/market.go`（`GET /market/departments|roles|roles/:id|roles/:id/download`）
- `cmd/gen-roles-index`（扫描目录生成索引 JSON）
- 前端：`MarketPage`、`RoleCard`、`RoleDetailSheet`、`api/market.ts`、`hooks/useMarket.ts`

### 1.4 现有 Agent 配置 / 能力 UI
- 后端 `model/agent.go`：`Agent{ ..., Capabilities []string, NodeID, Product, Status }`；`handler/agent.go` 已有 `GetCapabilities / SetCapabilities / UploadSkillFile` 三个端点。
- 前端：`AgentConfigDialog`（创建/编辑 Agent）、`HermesCapabilityTab` + `SkillAddDialog`（**手动上传技能包 zip → 部署到节点**）、`AgentChatPanel`。

### 1.5 关键缺口
- **无技能目录**（可浏览/搜索/选择的 skill 列表）——只有可下载的角色市场。
- **无 skill 与 Agent 的显式绑定**（今天 `Capabilities` 只是自由标签，`MustUseSkill` 是写死的默认 skill）。
- **无远程执行**（skill 必须落节点本地才能跑）。
- **无多租户**（隔离边界只有 `UserID`，无 tenant/workspace）。
- **无 MCP 集成**（当前分发走自定义 clawsynapse 协议，非 MCP）。

---

## 2. 总体架构（路线 B 为目标态）

```
┌─────────────┐  浏览/选择/配置   ┌──────────────────────────┐
│  前端        │ ───────────────► │  技能广场目录层 (平台)      │
│ SkillMarket  │                  │  SkillStore + skills_index │
└─────────────┘ ◄─────────────── └──────────────────────────┘
       │ 配置 Agent.Skills=[...]           │ 绑定关系入库
       ▼                                   ▼
┌─────────────┐  发布 todo.assigned         ┌──────────────────────────┐
│ Agent 节点   │ ───(clawsynapse publish)──► │  TrustMesh 编排中枢        │
│ (hermes)     │                            │  - 写入 ExecBrief.skills   │
│              │ ◄── skill.invoke ──────────│  - 运行时分发协议           │
│              │ ─── skill.result ─────────►│  └─ 远程执行 Runtime ──────┘
└─────────────┘   (skill 正文不离开平台)     │     (沙箱 + LLM 编排，
                                            │      加载 SKILL.md/scripts)
```

**运行时分发协议（新增消息类型）**：
- 平台 → Agent：`skill.invoke { skill, version, input(JSON), context_refs, session_key }`
- Agent → 平台：`skill.result { status, output, output_files(refs), error, logs_preview }`
- 平台侧执行 Runtime 加载 skill 包（SKILL.md + scripts），在**隔离沙箱**内运行，结果回传。Agent 全程看不到 skill 正文。

> 与现有 `todo.assigned` 的关系：Agent 在执行 todo 时，若 `ExecBrief.skills` 包含远程技能，则改为向平台发 `skill.invoke`；平台执行后把 `skill.result.output` 注入 Agent 上下文继续推理。本地安装型技能仍走原有「节点本地读 SKILL.md」路径，二者可并存。

---

## 3. 数据模型与协议设计

### 3.1 技能目录模型（克隆 `model/market.go`）
新增 `model/skill.go`：
```go
type SkillsIndex struct {
    GeneratedAt string           `json:"generated_at"`
    Categories  []SkillCategory `json:"categories"`
}
type SkillCategory struct {
    ID       string      `json:"id"`
    Name     string      `json:"name"`
    Skills   []SkillItem `json:"skills"`
}
type SkillItem struct {
    ID          string   `json:"id"`            // 目录名，全局唯一
    Name        string   `json:"name"`          // 来自 SKILL.md frontmatter
    Description  string   `json:"description"`
    Category    string   `json:"category"`
    Author      string   `json:"author"`
    Version     string   `json:"version"`       // SemVer
    Tags        []string `json:"tags"`
    Visibility  string   `json:"visibility"`    // global | private
    Pricing     string   `json:"pricing"`       // free | one_time | subscription
    Files       SkillFiles `json:"files"`       // SKILL.md / scripts / references / assets 路径
}
type SkillDetail struct {
    SkillItem
    SkillMarkdown string `json:"skill_markdown"`    // SKILL.md 正文（仅平台/授权调用可见）
    Readme       string `json:"readme,omitempty"`
}
```
> **隐私边界**：`SkillMarkdown`（正文）只在平台内部执行/授权详情时读取；目录列表与广场只暴露元数据（name/description/category/version/tags），符合「渐进式披露」。

### 3.2 技能包格式（沿用并扩充）
沿用 `skills/<id>/SKILL.md` + 可选 `scripts/ references/ assets/`。frontmatter 扩充：
```yaml
name: pdf-report
description: 生成结构化 PDF 财报
version: "1.0.0"            # 提为一级字段，SemVer
category: finance
author: acme
visibility: global
pricing: free
tags: [pdf, report]
```
新增 `cmd/gen-skills-index`（克隆 `cmd/gen-roles-index`）扫描 `skills/` 生成 `skills_index.json`。

### 3.3 运行时分发协议（加入 `protocol/clawsynapse.go`）
```go
type SkillInvokePayload struct {
    Skill       string         `json:"skill"`                 // skill id / name
    Version     string         `json:"version,omitempty"`
    Input       map[string]any `json:"input"`                 // 结构化参数(JSON)
    ContextRefs []string       `json:"context_refs,omitempty"`// 关联 task/todo/文件
    SessionKey  string         `json:"session_key"`
}
type SkillResultPayload struct {
    Skill     string `json:"skill"`
    Status    string `json:"status"` // ok | error | need_review
    Output    string `json:"output"` // ≤4KB 摘要；大结果走 files
    Files     []TaskAttachedFileRef `json:"files,omitempty"`
    Error     string `json:"error,omitempty"`
    LogsPreview string `json:"logs_preview,omitempty"`
}
```

### 3.4 新增后端 API（挂在 `/api/v1`）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/skills/categories` | 分类列表（含数量） |
| GET | `/skills?category=&q=` | 技能列表（仅元数据） |
| GET | `/skills/:id` | 技能详情（授权用户可见正文摘要） |
| GET | `/skills/:id/download` | 下载技能包 zip（本地安装型用） |
| GET | `/agents/:id/skills` | 该 Agent 已绑定技能 |
| POST | `/agents/:id/skills` | 绑定/解绑技能（`{skill_id, action: add|remove}`） |
| POST | `/agents/:id/skills/install` | 一键从广场安装到节点（复用 UploadSkillFile + SetCapabilities） |
| POST | `/skills/:id/execute` | **远程执行入口**（Runtime 调用，路线 B 核心） |

---

## 4. 后端改造清单

| # | 改造点 | 涉及文件（新建/改） | 难度 |
|---|---|---|---|
| 1 | 技能目录数据层 `SkillStore`（克隆 `MarketStore`，改扫描/索引字段） | `store/store_skill.go`（新） | 低 |
| 2 | 技能模型与响应类型 | `model/skill.go`（新） | 低 |
| 3 | 索引生成脚本 | `cmd/gen-skills-index`（新，克隆 gen-roles-index） | 低 |
| 4 | 广场目录 API（categories/列表/详情/download） | `handler/skill.go`（新）+ `router.go` 注册 | 低 |
| 5 | Agent ↔ Skill 绑定模型与存储 | `model/agent.go` 加 `Skills []string`；`store/store_agent.go` 加绑定方法 | 中 |
| 6 | Agent 技能端点（绑定/解绑/一键安装到节点） | `handler/agent.go` 加 3 个方法 + `router.go` | 中 |
| 7 | 派发时下发技能列表 | `webhook.go` / `dispatchNextTodo`：把 Agent.Skills 写入 `ExecBrief.skills`（新增字段） | 中 |
| 8 | 运行时分发协议类型 | `protocol/clawsynapse.go` 加 `SkillInvokePayload`/`SkillResultPayload` + webhook 分发分支 | 中 |
| 9 | 远程执行 Runtime（加载 SKILL.md + 沙箱跑 scripts + LLM 编排） | `internal/skillexec`（新，可复用 `internal/assistant` 的 LLM 循环） | **高** |
| 10 | 沙箱隔离（网络隔离/只读根/资源与超时限制/屏蔽 LD_PRELOAD） | `internal/skillexec` + 部署（gVisor/Kata 可选） | **高** |
| 11 | 权限/计费/版本/撤销 | `handler/skill.go` + `store`（谁可调、版本 pin、下架即失效） | 中 |
| 12 | 发布准入与静态扫描 | 发布流水线 + 脚本扫描（参考 SkillSpector 类） | 中 |

---

## 5. 前端改造清单

| # | 改造点 | 涉及文件（新建/改） | 难度 |
|---|---|---|---|
| 1 | 技能广场页（分类侧栏 + 搜索 + 卡片网格） | `pages/SkillMarketPage.tsx`（新，克隆 `MarketPage`） | 低 |
| 2 | 技能卡片 / 详情抽屉 | `components/skill/SkillCard.tsx`、`SkillDetailSheet.tsx`（新，克隆 `RoleCard`/`RoleDetailSheet`） | 低 |
| 3 | 技能 API 客户端 / hooks | `api/skills.ts`、`hooks/useSkills.ts`（新，克隆 `market.ts`/`useMarket.ts`） | 低 |
| 4 | Agent 配置增加「技能」步骤 | `AgentConfigDialog.tsx` 增加 skill 多选（复用 checkbox/Select） | 中 |
| 5 | 已部署技能 Tab 接入广场 | `HermesCapabilityTab` 的 `SkillAddDialog` 增加「从广场选择安装」（调 `/agents/:id/skills/install`） | 中 |
| 6 | 路由 + 侧边栏导航 | `App.tsx` 加 `/skills`、`/skills/:id`；`Sidebar.tsx` 加入口 | 低 |
| 7 | 远程执行可视化（可选） | 复用 `AgentChatPanel` / assistant 工具调用卡片展示 skill 执行进度 | 中 |

---

## 6. 安全与防护（来自上一轮调研）

- **内容不可见（核心）**：路线 B 下 skill 正文只在平台执行 Runtime 内加载，Agent 节点与终端用户均拿不到明文 → 天然防逆向，优于本地加密包（无白盒密钥难题、无上下文泄露）。
- **准入层**：发布者身份认证 + 2FA、发布前静态扫描（恶意 skill 占比 Snyk 实测 13.4%）、`capabilities` manifest 声明危险操作。
- **运行时沙箱**：强制网络隔离、只读根文件系统、非 root、PID/CPU/内存/超时限制、屏蔽 `LD_PRELOAD/PYTHONPATH`；不可信代码上 gVisor/Kata。
- **权限模型**：`Authenticate → ResolveTenant → Authorize → Filter → Execute → Audit`，模型只做相关性决策，权限在服务端。
- **撤销**：因可执行体在服务端，下架/吊销授权是瞬时动作（隐藏暴露 / 下线回退 / 吊销授权三档）。
- **计费**：参考 Anthropic Skills Marketplace（15% 分成，免费/买断/订阅），TrustMesh 可在 `/skills/:id/execute` 入口做计量。

---

## 7. 改造难度总评与优先级

**总体结论**：路线 A（目录 + 本地一键安装）改造量小、复用度高，**约 70% 可克隆现有市场/能力代码**，难度集中在「绑定关系 + 派发下发」。路线 B（远程执行）的主要难点是**新建执行 Runtime + 沙箱**，这是唯一的高难度块，也是「内容不可见」价值所在。

**难度评级**：
- 🟢 低：目录数据层、前端页面/卡片、路由、索引脚本（纯克隆 + 改名）。
- 🟡 中：Agent↔Skill 绑定、派发下发 `ExecBrief.skills`、技能端点、权限/版本/撤销、前端配置接入。
- 🔴 高：远程执行 Runtime、沙箱隔离（需基础设施与编排改动）。

---

## 8. 推荐落地路线（分阶段）

- **阶段 1（P0，🟢 低）— 技能广场目录 + 一键安装到节点**
  建 `SkillStore`/`model/skill.go`/`handler/skill.go` + 索引脚本 + 前端 `SkillMarketPage`/`SkillCard`/`SkillDetailSheet` + 一键 `install` 到 hermes 节点（复用 `UploadSkillFile`+`SetCapabilities`）。立刻让用户「浏览 → 选 → 装到 Agent 节点」。
- **阶段 2（P1，🟡 中）— 显式技能选择与派发**
  `Agent.Skills` 字段 + 绑定端点 + `todo.assigned` 下发 `ExecBrief.skills`；前端 `AgentConfigDialog` 增加技能多选。让「配置选择技能广场的 skill」成为显式能力，Agent 运行时按所选技能行事。
- **阶段 3（P2，🔴 高）— 远程执行分发协议**
  新增 `skill.invoke`/`skill.result` 协议 + `internal/skillexec` Runtime + 沙箱；`/skills/:id/execute` 入口。实现「调用平台上的 skill 去执行」且内容不落客户端。**这是目标态**。
- **阶段 4（持续，🟡 中）— 治理**
  发布准入/扫描、权限/计费/版本 pin/撤销、多租户（若需跨用户共享，先补 tenant 维度；否则沿用「全局目录 + 按 UserID 私有」现有市场模式）。

---

## 9. 关键风险与待决策点（需你拍板）

1. **hermes-only 限制**：今日能力下发仅对 `product=="hermes"` 开放。阶段 1 的「一键安装」需限定 hermes 节点或对其它节点做降级说明。
2. **外部依赖**：`capability.set` 落地依赖外部 clawsynapse daemon + hermes adapter（不在本仓库），阶段 1 验证前需确认目标节点侧 skill 目录机制就绪。
3. **多租户缺失**：跨用户发布/订阅需补 tenant/workspace；若先做「平台官方技能库」，沿用全局只读目录即可，零成本。
4. **远程执行延迟/流式**：`skill.invoke` 同步调用平台 Runtime 会拉长 Agent 推理步；需设计超时与可选 SSE 流式回传。
5. **本地 vs 远程并存**：建议阶段 3 保留「节点本地已装 skill」走原路径，远程 skill 走 `skill.invoke`，由 `ExecBrief.skills` 区分，避免破坏现有任务流。

---

### 附：可直接复用的现有资产（减少返工）
- 数据层：`store/store_market.go` → `store/store_skill.go`
- 模型：`model/market.go` → `model/skill.go`
- Handler：`handler/market.go` → `handler/skill.go`
- 下发：`clawsynapse.Client.UploadSkillFile` / `SetCapabilities`（已成熟）
- 前端：`MarketPage`、`RoleCard`、`RoleDetailSheet`、`api/market.ts`、`hooks/useMarket.ts`、`Sidebar.tsx`、`HermesCapabilityTab`/`SkillAddDialog`
- 触发：`protocol/clawsynapse.go` 的 `ExecBrief`/`TodoAssignedPayload`（扩展字段即可）
