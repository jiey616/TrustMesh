# 研发设计文档：项目工作流（PM 按指定工作流规划任务）

> 状态：已确认（2026-08-10）
> 作者：WorkBuddy
> 日期：2026-08-10
> 关联：Todo 人工确认与退回重做（已完成，2026-08-07）、Action Items → Tasks（已完成，2026-08-07）

---

## 1. 背景与目标

### 1.1 背景

当前 PM 智能体的规划是**自由发挥**的：收到需求后自行理解、澄清、拆解 todos、指派执行者。同一项目的不同类型任务可能产出差异很大的流程，且无法保证"必须经过某几个环节、按固定顺序、由指定角色执行"。用户需要**预定义工作流**，让 PM 严格按流程规划任务。

### 1.2 目标

- 项目可配置**有序工作流**：步骤列表，每步绑定角色（或具体智能体）、可选"需人工确认"
- 创建任务时**默认按项目工作流**规划，可临时覆盖
- PM 规划（`task.plan_ready`）时**必须遵守工作流骨架**：步骤全覆盖、顺序一致、assignee 角色匹配
- **backend 硬校验**规划结果：不符合 → 拒绝并返回明确错误，PM 修正重发
- 人工确认点**显式固化**在工作流中（强制），PM 仍可额外追加（可选）
- 执行阶段**零新增逻辑**：复用现有 todo 顺序派发 + review + rework

### 1.3 非目标

- 不做可视化工作流拖拽编辑器（用表单式步骤编辑即可）
- 不改 Todo 状态机、不改 ClawSynapse 消息总线
- 不做工作流版本化/历史（后续可扩展）
- 不做执行阶段强制约束（执行顺序由 todo 顺序机制天然保证）

---

## 2. 现状分析（已核实）

| 能力 | 位置 | 说明 |
|---|---|---|
| 项目模型 | `model/project.go` | 有 `PMAgent`（每项目一个 PM）、`WorkStatus`；**无 workflow 字段** |
| Agent 角色 | `model/agent.go` | Agent 有 `Role`（如"导演"）、`NodeID`；角色是字符串，非受控枚举 |
| PM 规划 | `tm-task-plan` skill | `task.message` → 澄清 → `task.plan_ready`（title/description/todos）→ 用户确认 → 派发 |
| 规划校验 | `webhook.go` `handleTaskPlanReady` | 有基本校验（title/todos 非空等），**无工作流校验** |
| 任务消息 | `task.message` payload | backend 发 PM 的消息，可扩展字段 |
| 人工确认 | `TodoResult.NeedReview` | 已实现 `pending_approval` → 用户通过/退回 |
| 顺序派发 | `NextDispatchableTodo` | 前序完成才派发下一个，天然保证执行顺序 |

**关键架构约束**：PM/执行 Agent 是**外部 ClawSynapse 节点**，看不到 backend 内部数据。工作流必须通过**消息 payload** 传给 PM（`task.message`），否则 PM 无从知晓。

---

## 3. 决策点与结论

| # | 决策点 | 结论 |
|---|---|---|
| 1 | 工作流形态 | **有序步骤骨架**：`[{name, role, need_review?}]`；角色绑定强约束，步骤可拆多个 todo、PM 可追加步骤（软约束） |
| 2 | 定义位置 | **项目级配置为主**（`project.workflow`），创建任务时可**覆盖**（任务存 workflow 快照） |
| 3 | 传递机制 | backend 发 `task.message` 时把 workflow 放 payload 字段；`tm-task-plan` skill 增加"按 workflow 规划"规则 |
| 4 | 约束强度 | **backend 硬校验 `task.plan_ready`**：骨架步骤全覆盖 + 顺序一致 + assignee 角色匹配；不满足返回明确错误（含差异说明），PM 修正重发 |
| 5 | 人工确认 | **工作流步骤显式标注** `need_review`（强制确认点）；PM 规划时可**额外追加** need_review（可选确认点） |
| 6 | 执行阶段 | 复用现有 todo 顺序派发 + review + rework，零新增执行逻辑 |

---

## 4. 数据模型

### 4.1 工作流结构

```go
// model/workflow.go（新文件）

// WorkflowStep 是工作流中的一个有序步骤。
type WorkflowStep struct {
	Name       string `json:"name" bson:"name"`                 // 步骤名，如"编剧产出剧本"
	Role       string `json:"role" bson:"role"`                 // 绑定角色（Agent.Role 或 Agent.Name 匹配）
	NeedReview bool   `json:"need_review,omitempty" bson:"need_review,omitempty"` // 该步骤产出需人工确认
}

// Workflow 是项目预定义的有序工作流。
type Workflow struct {
	Name   string          `json:"name" bson:"name"`             // 工作流名，如"剧本制作流水线"
	Steps  []WorkflowStep  `json:"steps" bson:"steps"`           // 有序步骤（顺序即执行顺序）
}
```

### 4.2 挂载位置

```go
// Project 增加（model/project.go）
Workflow *Workflow `json:"workflow,omitempty" bson:"workflow,omitempty"` // 项目默认工作流

// TaskDetail 增加（model/task.go）
Workflow *Workflow `json:"workflow,omitempty" bson:"workflow,omitempty"` // 任务级工作流快照（创建时从项目复制或覆盖）
```

> 任务创建时：若用户指定 workflow → 用指定值；否则复制 `project.workflow`；都不存在 → 无工作流（PM 自由规划，兼容现有行为）。

---

## 5. 协议与消息

### 5.1 `task.message`（backend → PM）增加 workflow

```json
{
  "schema_version": "1.0",
  "task_id": "task_123",
  "project_id": "proj_1",
  "content": "请使用 /tm-task-plan skill 处理本次需求。",
  "user_content": "做一个 XXX",
  "workflow": {
    "name": "剧本制作流水线",
    "steps": [
      { "name": "编剧产出剧本", "role": "编剧" },
      { "name": "导演拆解分镜", "role": "导演", "need_review": true },
      { "name": "测试质量验收", "role": "测试" }
    ]
  }
}
```

- `workflow` 为**可选**字段；无工作流时 PM 自由规划（向后兼容）
- 流程：任务创建 → `publishTaskCreated` 附近，backend 从 task.Workflow 读取并塞入 `task.message` payload

### 5.2 `task.plan_ready` 校验（backend）

`handleTaskPlanReady` 在现有校验后增加 `validatePlanAgainstWorkflow(task, todos)`：

```
若 task.Workflow == nil → 跳过校验（兼容）
否则：
  1. 每个工作流步骤，在 todos 中按 assignee 角色匹配"至少一个 todo"（角色匹配：todo.Assignee.Name 或 Agent.Role 命中步骤.Role）
  2. 这些代表步骤的 todos 的先后顺序必须与工作流步骤顺序一致（用 todos 中最小的匹配 order 做锚点，单调递增）
  3. 失败 → 返回 422 WORKFLOW_MISMATCH，错误信息列出差异（缺哪个步骤/顺序哪里错/哪步角色不匹配），PM 按 tm-task-plan skill 修正重发
```

> 允许 PM 追加额外 todo（如澄清产物、中间检查），只要骨架步骤被覆盖且顺序不违背。**允许一个工作流步骤拆成多个 todo**（拆分步骤的多个 todo 都在该步骤锚点位置附近）。

### 5.3 任务创建 API 覆盖

`POST /projects/:id/tasks` body 增加可选 `workflow`（覆盖项目默认）：

```json
{ "title": "...", "assignee_agent_id": "...", "workflow": { ... } }
```

---

## 6. 前端

### 6.1 项目设置：工作流编辑

- 项目「设置」区（或项目页新增「工作流」Tab）：
  - 步骤列表：每步 = 步骤名输入 + 角色下拉（从项目内 Agent 列表/角色选取）+「需人工确认」开关
  - 可增删步骤、上下移排序、保存 → `PUT /projects/:id` 存 `project.workflow`
  - 未配置时显示"未配置工作流（PM 自由规划）"

### 6.2 创建任务

- 「新任务」弹窗：可选「按项目工作流」下拉（默认项目工作流 / 覆盖编辑 / 无）
- 覆盖编辑 = 内联步骤列表（复用 6.1 的编辑器组件）

### 6.3 规划结果展示

- 任务详情规划确认面板（PlanReviewPanel）：若任务有工作流，显示"按工作流：剧本制作流水线"角标；校验失败的规划会显示 backend 返回的差异原因

---

## 7. Skill 更新（tm-task-plan）

- 新增章节「按工作流规划（workflow）」：
  - 收到 `task.message` 的 `workflow` 字段时，**必须按步骤顺序规划 todos**，每步指派绑定角色的智能体
  - 步骤可拆多个 todo；可在骨架基础上追加额外 todo（如澄清、中间检查），但**不得遗漏/调换骨架步骤**
  - 步骤标注 `need_review: true` 时，对应 todo 完成必须声明 `need_review: true`
  - 若 `task.plan_ready` 被 backend 以 `WORKFLOW_MISMATCH` 拒绝，按错误信息修正（补步骤/调顺序/改角色）后重发
- 无 `workflow` 字段时保持现有自由规划

---

## 8. 实现清单

### 阶段 0：模型 + 协议
- [ ] `model/workflow.go`：`WorkflowStep` / `Workflow`
- [ ] `Project.Workflow`、`TaskDetail.Workflow` 字段
- [ ] `TaskCreateByUser` / `TaskCreateByPM` 复制 workflow 快照（项目默认或覆盖）
- [ ] `task.message` payload 增加 workflow（`TaskMessagePayload` 或新建 `TaskPlanningPayload`）

### 阶段 1：backend 校验
- [ ] `validatePlanAgainstWorkflow(task, todos)`：步骤全覆盖 + 顺序 + 角色匹配
- [ ] `handleTaskPlanReady` 接入校验，失败返回 `WORKFLOW_MISMATCH`（含差异详情）
- [ ] 任务创建 API 支持 `workflow` 覆盖
- [ ] 单测：工作流校验（缺步骤/乱序/角色错/正常/无工作流兼容）

### 阶段 2：前端
- [ ] 工作流编辑器组件（步骤表单 + 排序 + 角色选择 + need_review 开关）
- [ ] 项目设置接入（保存 `project.workflow`）
- [ ] 创建任务支持选择/覆盖工作流
- [ ] 规划确认面板显示工作流角标 + 校验错误提示

### 阶段 3：Skill + 部署验证
- [ ] `tm-task-plan` 更新「按工作流规划」章节，部署测试PM容器
- [ ] 端到端验证：配置工作流 → 创建任务 → PM 按工作流规划 → 校验通过 → 执行 → 人工确认点生效
- [ ] 回归：无工作流项目 PM 自由规划不受影响

---

## 9. 风险与注意

- **角色匹配口径**：`role` 匹配用 `Agent.Role` 或 `Agent.Name` 模糊匹配（现有 convert 的 assignee 解析同款逻辑）；需在文档中说明避免歧义
- **PM 不遵守**：硬校验兜底，但校验失败会多一轮 PM 修正往返；skill 强化 + 校验错误信息要足够清晰
- **向后兼容**：workflow 全程可选字段，无工作流项目行为不变（重要：现有测试项目不受影响）
- **外部节点**：workflow 必须走消息 payload，不能假设 PM 能读到 backend 数据（关键架构约束）
