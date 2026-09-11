# 研发设计文档：任务结果待办转新任务（Action Items → Tasks）

> 状态：待确认
> 作者：WorkBuddy
> 日期：2026-08-07
> 关联：Todo 人工确认与退回重做（已完成，2026-08-07）

---

## 1. 背景与目标

### 1.1 背景

多智能体任务执行后，执行 Agent 的结果中常常包含**后续待办事项**（如"概念设计完成后需导演审核""台词修改需要美术配合"）。当前这些待办散落在 `todo.result.output` 文本中，**无法结构化流转**：PM 智能体不会主动将其转为新任务，用户需要手动复制创建，链路断裂。

### 1.2 目标

- 执行 Agent 完成任务时，可在结果中**结构化声明后续待办**（`action_items`，单次 ≤ 5 条，同类/同智能体整合）
- PM 智能体收到各智能体的待办后**按目标智能体整合**：同一智能体的待办合并为 1 个新任务（多 todo），**自动将确定可执行的转为任务**，指派给对应智能体
- PM 拿不准的待办进入**项目内「待办」Tab**，用户确认（可调整执行者）后创建
- 全程可溯源（新任务记录来源任务）、防重复转换

### 1.3 非目标

- 不做自动派发策略优化（同一 agent 并发限制等）
- 不引入新的消息传输总线（复用现有 ClawSynapse 协议）
- 不改动 Todo 状态机（沿用现有 pending/in_progress/done/failed/canceled + review 旁路字段）

---

## 2. 现状分析（已核实）

### 2.1 现有能力

| 能力 | 位置 | 说明 |
|---|---|---|
| `task.create` 协议 | `protocol/clawsynapse.go` `TaskCreatePayload` | PM 发 `task.create` 创建任务，todos 用 `assignee_node_id` 指定执行者；**已实现** |
| `task.created` 确认 | `webhook.go` `publishTaskCreated` | 创建后回执 PM |
| 用户创建任务 | `CreateTaskByUser` | 用户 API `POST /projects/:id/tasks` |
| `todo.complete` | `TodoCompletePayload` | 已支持 `need_review` / `return_previous` |
| 结果存储 | `TodoResult{Summary, Output, Metadata}` | **无结构化待办字段**（本次核心缺口） |
| PM 通知 | `task.status_changed` 等 | 任务状态变化通知 PM |

### 2.2 关键缺口

1. **`TodoResult` 无 `action_items` 结构**——待办无法结构化传递
2. **PM 无感知通道**——任务完成后 backend 未主动把"结果中的待办"推给 PM
3. **无防重机制**——同一待办可能被重复转换
4. **无确认 UI**——PM 拿不准的待办无处展示/确认
5. **无溯源字段**——新任务与来源任务无关联

---

## 3. 需求设计（grill 结论）

| # | 决策点 | 结论 |
|---|---|---|
| 1 | 待办来源 | 执行 Agent 在 `todo.complete` 的 result 带**结构化 `action_items`**（可含 assignee 建议） |
| 2 | 触发方式 | **默认自动转 + 拿不准才确认**（PM 判断） |
| 3 | 转换语义 | **PM 整合：按目标智能体分组，同一智能体的待办整合为 1 个新任务（每个待办 = 1 个 todo）**；不同智能体 → 不同任务 |
| 4 | 智能体指派 | **待办自带 assignee 建议优先 + PM 兜底判断**（整合时以该组的执行者作为任务 assignee） |
| 5 | 任务归属 | **来源任务同 project + `source_task_id` 溯源 + 来源任务动态通知** |
| 6 | 传递与防重 | action_items 放 `result.metadata` + backend 发 `task.result` 通知 PM + **`converted_task_id` 标记防重复** |
| 7 | 确认通道 | **前端「待办」Tab**（项目内，用户逐个勾选确认） |
| 8 | 确认后创建 | **前端直接调 `task.create`（用户身份）**，不依赖 PM 在线；**同样按目标智能体整合**（与 PM 自动转一致） |

---

## 4. 技术设计

### 4.1 数据模型

#### `TodoResult` 新增 `action_items`（model/task.go）

```go
type ActionItem struct {
    Title            string `json:"title"`                        // 待办标题（必填）
    Description      string `json:"description,omitempty"`        // 待办描述
    AssigneeNodeID   string `json:"assignee_node_id,omitempty"`   // 建议执行者节点（可选）
    AssigneeRole     string `json:"assignee_role,omitempty"`      // 建议角色名（可选，PM 可据此匹配）
    Status           string `json:"status"`                       // "pending" | "awaiting_confirmation" | "converted"
    ConvertedTaskID  string `json:"converted_task_id,omitempty"`  // 转换后生成的任务 ID（防重）
    ConfirmedBy      string `json:"confirmed_by,omitempty"`       // 确认人（"pm" | 用户ID）
    CreatedAt        time.Time `json:"created_at"`
}
```

- 存于 `TodoResult.Metadata["action_items"]`（结构化数组），或直接加 `ActionItems []ActionItem` 字段（**推荐直接加字段**，类型安全、前端友好）

#### `TaskDetail` 新增 `source_task_id`（溯源）

```go
SourceTaskID string `json:"source_task_id,omitempty" bson:"source_task_id,omitempty"`
```

### 4.2 协议扩展

#### 4.2.1 `todo.complete` — 执行 Agent 声明待办

执行 Agent 在 `todo.complete` 的 `result` 里携带 `action_items`：

```json
{
  "task_id": "task_123",
  "todo_id": "todo_5",
  "result": {
    "summary": "概念设计完成",
    "output": "...",
    "action_items": [
      {
        "title": "导演质量门审核概念设计",
        "description": "按 S1-S4 视觉规范审核角色一致性",
        "assignee_role": "导演"
      },
      {
        "title": "分镜师基于概念设计出分镜",
        "assignee_node_id": "n1-xxx"
      }
    ]
  }
}
```

#### 4.2.2 新消息 `task.result`（backend → PM）

todo.complete 处理完成后，若 result 含未转换的 `action_items`，backend 向 PM 推送：

```json
{
  "type": "task.result",
  "sessionKey": "<task_id>",
  "message": {
    "task_id": "task_123",
    "todo_id": "todo_5",
    "todo_title": "概念设计",
    "action_items": [
      { "title": "导演质量门审核概念设计", "assignee_role": "导演", "status": "pending" }
    ],
    "project_id": "proj_1"
  }
}
```

> PM 收到后：**按目标智能体分组整合**——同一智能体的待办合成 1 个新任务（每个待办 = 1 个 todo，`task.create` 带 `source_task_id` 回填），不同智能体分别建任务；能直接执行的 → 自动转；拿不准的 → 将待办置 `awaiting_confirmation`（通过新消息或用户确认 API）。

### 4.3 后端改动

| 文件 | 改动 |
|---|---|
| `model/task.go` | `TodoResult` 加 `ActionItems []ActionItem`；`TaskDetail` 加 `SourceTaskID` |
| `store/workflow.go` | `CreateTaskByUser` / `CreateTaskByPMNodeWithMessageID` 支持 `SourceTaskID`；todo.complete 存 action_items；新增标记 `ConvertActionItem`（置 converted + 记录 task_id） |
| `clawsynapse/webhook.go` | 新增 `handleTaskResult`（发 PM 通知）；`handleTodoComplete` 完成后检测 action_items 触发通知；`handleTaskCreate` 透传 source_task_id |
| `protocol/clawsynapse.go` | `TaskResultPayload` 新结构；`TaskCreatePayload` 加 `source_task_id` |
| `handler/task.go` | 新增确认 API（见 4.5） |
| `app/router.go` | 新路由 |

### 4.4 确认 API（前端面板）

```http
# 列出当前用户所有「待确认待办」（跨任务聚合）
GET /api/v1/action-items?status=awaiting_confirmation
→ { items: [{ task_id, todo_id, title, description, assignee_role, assignee_node_id, created_at, source_task_title }] }

# 用户确认并创建任务（前端直接调，用户身份；同一 assignee 的多条待办合并为 1 个任务的多 todo）
POST /api/v1/action-items/convert
body: { item_ids: [...], assignee_agent_id?: string, project_id?: string }
→ 按 assignee_agent_id 分组创建新任务（每组 1 个任务，每待办 1 个 todo；source_task_id 回填 + converted_task_id 标记）
```

### 4.5 前端改动

| 文件 | 改动 |
|---|---|
| `types/index.ts` | `ActionItem` 类型；`TodoResult` 加 `action_items`；`TaskDetail` 加 `source_task_id` |
| 新组件 `ActionItemsPanel` | 「待确认待办」面板：列表 + 勾选 + 调整执行者 + 确认创建 |
| 入口 | 侧边栏或任务详情入口（建议先做任务详情 Tab + 全局入口二选一，见 4.6） |
| 任务动态 | 来源任务动态追加"已根据结果创建任务 X" |
| 新任务 | 显示"来源任务"引用 |

### 4.6 UI 入口

- **方案（已定）：项目内新增「待办」Tab 页，与「任务」Tab 页平级**
  - 项目详情页（ProjectBoardPage）增加 Tab：「任务」/「待办」（+ 现有其他 Tab）
  - 「待办」Tab 展示**该项目下所有 `awaiting_confirmation` 待办**（跨任务的聚合视图）：标题、来源任务、建议执行者、时间
  - 操作：勾选 → 调整执行者（下拉选择该项目可用的智能体）→ 确认创建 → 调确认 API
  - 不做全局侧边栏入口（待办归属项目，项目内管理更清晰）

### 4.7 Skill 更新

| Skill | 改动 |
|---|---|
| `tm-task-exec` | 完成回报时：若结果产生后续待办，在 `todo.complete` 的 result 带 `action_items`（含建议 assignee）；**必须将同类问题、同智能体的待办整合合并，单次最多 5 条**，优先保留高优先级/独立执行者的事项；说明何时声明 |
| `tm-task-plan` | 收到 `task.result` 后：**按目标智能体整合待办**（同一智能体的待办 → 1 个任务的多个 todo；不同智能体 → 不同任务），可执行的 → `task.create` 自动转（`source_task_id` 回填）；拿不准 → 通过确认 API 置 `awaiting_confirmation`（或发消息让用户在前端确认） |

---

## 5. 防重与一致性

1. **防重**：`action_items[i].converted_task_id` 非空即视为已转换，PM/确认 API 跳过；`task.create` 已按 messageID 幂等
2. **并发**：确认 API 转换时校验 `converted_task_id` 为空（乐观锁），避免双击重复创建
3. **待办状态机**：`pending`（可转）→ `converted`（已转）| `awaiting_confirmation`（需用户确认）→ `converted`
4. **过期清理**：`awaiting_confirmation` 超 7 天未处理，动态提示用户（暂不做自动清理，仅提示）

---

## 6. 验证方案

### 6.1 后端单测
- `TodoResult` 序列化含 action_items 往返一致
- `task.result` 通知触发条件（有未转换 action_items）
- 确认 API：转换成功 / 重复转换 409 / assignee 归属校验
- `source_task_id` 透传

### 6.2 端到端（webhook 模拟）
1. 创建任务 → 导演完成 todo（带 2 个 action_items：1 个带 assignee_node_id、1 个只带 role）
2. 断言：backend 发 `task.result` 给 PM；PM（模拟）对第 1 个发 `task.create`（source_task_id）→ 新任务创建成功 + converted 标记
3. 断言：第 2 个待办置 awaiting_confirmation → 前端 API 列出 → 用户确认（选 assignee）→ 新任务创建 + converted
4. 断言：重复转换被拒；来源任务动态出现"已创建任务"通知

### 6.3 回归
- 现有 todo.complete（无 action_items）行为不变
- review/退回重做流程不受影响

---

## 7. 改动清单与规模

| 层 | 文件数 | 规模 |
|---|---|---|
| 后端 | ~7（model/protocol/store/webhook/handler/router） | 中 |
| 前端 | ~5（types/新组件/入口/动态） | 中 |
| Skill | 2（tm-task-exec / tm-task-plan） | 小 |
| 文档 | message-protocol.md 补 task.result | 小 |

**预估**：比"人工确认+退回重做"略小（无复杂状态机，主要是数据流打通）。

---

## 8. 风险与兼容性

| 风险 | 等级 | 缓解 |
|---|---|---|
| PM 自主转换可能误判 | 中 | 拿不准走确认面板；转换后可取消 |
| action_items 数据量膨胀 | 中 | **单次完成回报上限 5 条**；skill 要求智能体**将同类问题/同智能体的待办整合**（尽量合并为 1 条），超限时按优先级保留前 5 条并提示 |
| 老数据无 action_items | 无 | 字段可选，缺省不触发通知 |
| 前端面板入口位置 | 低 | 项目内「待办」Tab（已定） |

---

## 9. 实施顺序（建议）

1. **阶段 0**：模型字段 + 协议结构（ActionItem / TaskResultPayload / source_task_id）
2. **阶段 1**：backend 数据流（todo.complete 存 action_items → task.result 通知 → 确认 API + convert 逻辑）
3. **阶段 2**：前端（types + ActionItemsPanel + 入口 + 动态/溯源展示）
4. **阶段 3**：Skill 更新（tm-task-exec 声明 / tm-task-plan 转换）+ 部署
5. **验证**：单测 + webhook 端到端 + 回归

---

*待确认项：① UI 入口（**已定**：项目内「待办」Tab，与任务 Tab 平级）；② action_items 上限（**已定**：单次 ≤ 5 条，同类/同智能体整合）；③ awaiting_confirmation 过期策略（按推荐：超 7 天仅动态提示，不自动清理）。*
