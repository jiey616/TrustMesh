# 输入位可满足性「可见化」方案（2026-09-29）

**状态**：待评审，**未实施**
**关联**：本次已上线的修复 —— 提交 `3976779`，生产 tag `20260929-stepinputs`（跨任务输入解析支持 canceled 前序任务）
**前置结论**：L1（管道）已修并在生产实证；本方案处理 **L2（可见性）**；L3（数据质量）**明确不在平台侧修**，理由见 §7。

---

## 0. 结论先行

**只修一件事**：把「输入位解析结果」从**只发给执行侧的出站 payload**，变成**同时落在 todo 上、并让前端能读到**。

- **主体改动**：`model.Todo` 新增 `Inputs []TodoInput`（**对称于已存在的 `Outputs`**），在 **7 个**解析点写回（见 §3.3）。
- **明确不做**：① 通知（与「通知收窄」原则冲突，见 §7.1）；② 资产 `@码` 校验（平台不解析文件内容，见 §7.2）。
- **依赖的先例**：`model.Todo` 上的 `DispatchAttempts` / `LastDispatchAt` / `LastDispatchErr` —— 它们的存在理由是同一句话：「让静默失败可观测」（09-10 TD_04 卡 56 分钟事故）。本方案是**同一模式的第二个实例**，不是新范式。

---

## 1. 问题定性：三层，只修第二层

| 层 | 内容 | 证据 | 状态 |
|---|---|---|---|
| **L1 管道** | 跨任务解析**跳过 `status=="canceled"` 的前序任务** ⇒ `resolved=false` ⇒ agent 无 `download_url` | 执行侧三次提问原文「本步输入位在平台侧未解析」；`workflow_ref={step_from:4,step_to:6}` 证明该输入位必然跨任务 | ✅ **已修**（两档优先级）并**生产实证** |
| **L2 可见性** | 解析失败**对用户零可见** —— 只随出站 payload 下发 + 打一行 Warn 日志 | `handler/task.go` 的 5 处 `BuildTodoInputs` **全是出站 payload**；`Get` 直接返回 `*model.TaskDetail`，而 `model.Todo` **没有输入侧字段** | ⛔ **本方案** |
| **L3 数据质量** | 上游产物与下游需求的资产 `@码` 口径不一致（跳过出图的资产仍被引用；`JS002` vs `JS004` 编号口径） | TD_02 拿到 51382 B 输入后 **7 组里 4 组（57%）受阻** | ⛔ 属执行侧 skill（**平台无法校验**，§7.2） |

**关键不对称**：`model.Todo` 有 `Outputs []TodoOutput`（**输出**侧留痕：哪个产物绑到哪个输出位），却**没有任何输入侧对应物**。于是「这一步的输入到底从哪来、拿到了没有」在数据模型里**根本没有位置** —— 只能每次现算现发。

---

## 2. 为什么选这个落点（而不是别的）

1. **对称性**：`Outputs` 已有，补 `Inputs` 是补全而非新增概念。
2. **有先例**：`DispatchAttempts`/`LastDispatchAt`/`LastDispatchErr` 已经确立了「把静默失败写回 todo 供人看」的范式，本方案沿用同一套写法（`mutateTaskUnsafe` + `publishTaskUnsafe`）。
3. **直击本次事故的元问题**：用户之所以要找人排查，是因为「输入拿不到」这件事**只存在于执行侧 agent 的脑子里**（它发 comment 才知道）。落库后，用户在任务页**在 agent 开口之前**就能看到。
4. **零成本覆盖所有场景**：不管是创建时、派发时、重试时还是回答时解析失败，只要走过一次 `BuildTodoInputs`，状态就留在 todo 上，之后一直可见。

---

## 3. 改动清单

### 3.1 `backend/internal/model/task.go`

新增**给前端看**的输入位结构（**注意：与出站的 `protocol.TodoInputRef` 是两个不同的东西**）：

```go
// TodoInput mirrors TodoOutput for the *input* side: it records how this
// todo's declared StepInputs resolved at dispatch time, so a missing
// upstream file is observable in the task page instead of only reaching the
// assignee agent (2026-09-29).
//
// Deliberately narrower than protocol.TodoInputRef: DownloadUrl carries a
// signed JWT and FileRef is an internal id — neither may reach the browser.
//
// State is deliberately three-valued rather than a bool: "the input is
// missing" and "the input is not ready YET" call for opposite user actions
// (intervene vs. wait), and a bare Resolved=false cannot tell them apart —
// which would leave the user asking a human, defeating the point.
type TodoInput struct {
    Name       string     `json:"name" bson:"name"`
    SourceStep string     `json:"source_step,omitempty" bson:"source_step,omitempty"`
    OutputName string     `json:"output_name,omitempty" bson:"output_name,omitempty"`
    FileName   string     `json:"file_name,omitempty" bson:"file_name,omitempty"`
    FileSize   int64      `json:"file_size,omitempty" bson:"file_size,omitempty"`
    State      string     `json:"state" bson:"state"` // resolved | pending | missing
    CheckedAt  *time.Time `json:"checked_at,omitempty" bson:"checked_at,omitempty"`
}
```

**`State` 的判定规则（这是本方案的语义核心）**

| State | 含义 | 用户该做什么 |
|---|---|---|
| `resolved` | 已解析到 `download_url` | 无需动作 |
| `pending` | **上游任务存在且仍活跃**，只是尚未产出该输出位 | **等**（前序跑完会自动就绪） |
| `missing` | ① 上游任务已终态（完成/取消/失败）却无该输出位的产物，或 ② 项目里**根本没有任何任务持有该来源步骤** | **必须人工介入**：补文件 / 重新绑定 / 授权降级 |

判定实现：在 `resolveStepInput` 落到 `Resolved:false` 之前，**加一次探测** —— 复用 `FindTaskForWorkflowStep` 那套候选遍历（不引入新的全量扫锁），拿到「是否存在持有该步骤的任务」+「该任务是否终态」，据此二分。**跨任务解析已经遍历过一遍，这里是复用同一批候选，不是二次遍历。**

⚠️ 若实施时发现复用成本高于预期，**退路**是 P0 先只出 `resolved` / `unresolved` 两态，把三态挪到 P1 —— 但要在方案里显式记为「已知的价值缺口」，而不是默默降级。

并在 `Todo` 上追加（放在既有 `Outputs` 之后，形成对称）：

```go
    // Inputs records how this todo's declared StepInputs resolved the last
    // time the platform built a dispatch/answer payload. Unlike Outputs
    // (agent-declared), this is platform-observed. An entry with
    // Resolved=false is the machine-readable form of "this step cannot get
    // its upstream file" — the condition that used to be visible only to the
    // assignee agent.
    Inputs []TodoInput `json:"inputs,omitempty" bson:"inputs,omitempty"`
```

### 3.2 `backend/internal/store/workflow.go`

新增写回方法，**写法照抄 `RecordTodoDispatch`（`workflow.go:105`）**：

```go
// RecordTodoInputRefs stores the platform-observed input resolution state on a
// todo. Internal (no Scope) — callers are handlers that already passed scope
// checks on the surrounding operation.
func (s *Store) RecordTodoInputRefs(taskID, todoID string, inputs []model.TodoInput) *transport.AppError
```

要点：
- `s.mu.Lock()` → `findTodoIndex` → `mutateTaskUnsafe(task.ID, func(...))` 内**整体替换** `td.Inputs`（不是 append —— 每次都是**最新快照**）→ `s.publishTaskUnsafe(task.ID)`。
- **不**做 `ensureTodoAcceptingUpdates` 之类的终态校验：解析状态是**观测**不是状态机迁移，终态 todo 也应能刷新（用户重试失败后仍要看得到原因）。
- `inputs` 为空切片时**写入 nil 还是保持原值**？→ **保持原值**（避免「没解析过」把已观测到的失败抹掉）。

### 3.3 `backend/internal/clawsynapse/webhook.go` —— **单一收口，handler 侧零改动**

**核心决策：落库收口在 `BuildTodoInputs` 内部，而不是散落在 7 个调用点。**

`BuildTodoInputs`（`webhook.go:2459`）是**全部 7 个解析点唯一的共同入口** ⇒ 在它尾部写回，**一次改动覆盖所有路径**：

```go
// todoInputsView strips everything the browser must not see (notably the
// JWT-bearing DownloadUrl and the internal FileRef) before the refs are
// persisted for display. This is the ONLY place allowed to cross that
// boundary — keeping it next to the parse site means no caller can
// accidentally persist raw refs.
func todoInputsView(refs []protocol.TodoInputRef, now time.Time) []model.TodoInput

func (h *WebhookHandler) BuildTodoInputs(task *model.TaskDetail, todo *model.Todo) []protocol.TodoInputRef {
    refs := /* …… 既有解析逻辑完全不动 …… */

    // Persist the observed resolution state so a missing upstream file is
    // visible on the task page, not only to the assignee agent (2026-09-29).
    // Guarded exactly like the warn above: the package's tests build
    // zero-value handlers, and a read path must never fail on a nil store.
    if h.store != nil && task != nil && todo != nil {
        h.store.RecordTodoInputRefs(task.ID, todo.ID, todoInputsView(refs, time.Now().UTC()))
    }
    return refs
}
```

**为什么不在 7 个调用点各加一行**：
- 7 处散点 = 7 次漏改机会。**本方案初稿就漏了两处**（`remindTodo` 与 `buildTodoAssignedPayload`），是靠读代码才发现的；
- 这 7 处**横跨两个包**（`handler` / `clawsynapse`），未来新增第 8 个出口时**没有任何机制**提醒开发者同步。

**为什么这个副作用可接受**：`BuildTodoInputs` **已经不是纯函数** —— 本轮修复刚在它内部加了 `h.log.Warn`（`webhook_input_unresolved_log_test.go` 已为它建立了「nil logger 不 panic」的守卫范式）。两者同构：都是**观测副作用**，沿用同一个 `nil` 守卫。

🔴 **实施第一步（前置条件；不满足则换方案）**
确认 **7 个调用点全部不在 `s.mu` 持锁区内**。若某处是在 `mutateTaskUnsafe` / `GetTaskUnsafe` 的闭包内调用 `BuildTodoInputs`，则 `RecordTodoInputRefs` 再次取锁会 **死锁**（全内存 Store 单锁）。此时退路＝仅在那一个调用点改为「函数返回后再落库」，其余仍走收口。

**7 个调用点**（供核对覆盖完整性；实施时先 `grep -c 'BuildTodoInputs'` 确认没有第 8 处）

| # | 位置 | 宿主函数 | payload / 场景 |
|---|---|---|---|
| 1 | `handler/task.go:207` | `autoDispatchFirstTodo` | `todo.assigned`（user_created） |
| 2 | `handler/task.go:323` | 手动派发 handler | `todo.assigned`（manual_dispatch） |
| 3 | `handler/task.go:560` | `PublishTodoAnswer` | `todo.answer`（无 `sc`，见 §5.3） |
| 4 | `handler/task.go:584` | `publishReworkDispatch` | `todo.assigned`（rework） |
| 5 | `handler/task.go:616` | `dispatchNextTodo` | `todo.assigned`（review_approved） |
| 6 | `clawsynapse/webhook.go:1942` | **`remindTodo`**（user_resume） | `todo.remind` ← **本次事故通道** |
| 7 | `clawsynapse/webhook.go:2427` | **`buildTodoAssignedPayload`** | 被其 **4 个**调用者共用：`webhook.go:1820` / `1851` / `2223` / `2381` |

⚠️ 第 7 项是**公共构造函数** —— 只它一个就覆盖 4 个派发出口。这正是「收口在 `BuildTodoInputs`」的最大收益：**不必逐一追踪它的调用者**。

📌 **可选重构（P1，不在 P0）**：`handler` 侧那 4 处是**自己手搓** `TodoAssignedPayload`，与 `buildTodoAssignedPayload` 功能重复。若把后者导出让 handler 复用，payload 构造就能收敛为一条路径（届时落库点自然只剩 2 处）。

### 3.4 测试

- `store`：`RecordTodoInputRefs` 契约测试（整片替换、终态 todo 也能写、空切片不覆盖）。
- `handler`：
  - 正向：派发时 `resolved=false` 的输入位被写进 todo；
  - **反向守卫（必做）**：断言 API 响应里的 `inputs[]` **不含** `download_url` / `file_ref` 键 —— 这是防泄漏的钉子，不能只靠 code review。
- 前端契约：`frontend-v2` 的任务详情类型定义补 `inputs`（`zod`/TS 类型），并加一条「缺失输入位要被渲染」的组件测试。

### 3.5 `frontend-v2`

- 任务页 todo 卡片展示：`输入位 3/4 已就绪`，未就绪的展开列出 `name`（+ `source_step`）。
- 需要先探索现有 todo 卡片组件与 `pendingItems` 的渲染路径（实施第一步）。

---

## 4. 关键设计决策

| # | 决策 | 理由 |
|---|---|---|
| D1 | **view 结构剔除 `DownloadUrl`/`FileRef`** | `DownloadUrl` 是带 JWT 的签名直链，一旦进浏览器就等于把取件凭证发给终端用户（§3.1 注释已写明）。**这是本方案唯一的硬安全约束。** |
| D2 | **不发通知** | ① agent 本来就会用 `todo.ask` 提问，那条已有 `todo_ask_received` 通知 ⇒ 再加一条是重复噪音；② §〇bis 确立的原则是「通知只负责要人做决定」，而「某个输入位没解析」单独出现时**不需要用户立刻做决定**（agent 可能自己兜住，如 TD_01 那样）。**只做可见性。** |
| D3 | **落库时机 = 解析发生的时刻**（派发/回答），不是每次读接口时现算 | 现算要遍历全项目任务、且要拿锁，放进 `GET /tasks/:id` 会**放大读放大**（正是 §〇ter 那类事故的成因）。写一次、读多次。 |
| D4 | **整片覆盖而非 append** | 它是「最近一次观测的快照」，不是历史流水（历史在事件流里）。append 会让前端要做归并。 |
| D5 | **`omitempty` + 存量无该字段 = 零影响** | 向后兼容：老 todo 的 `inputs` 为空 ⇒ 前端不渲染该区块。**无需数据迁移**。 |

---

## 5. 风险与坑

### 5.1 `model.Todo` / view 两套结构（§一.6）
`Get` 是 `transport.WriteData(c, http.StatusOK, task)` —— **直接序列化 model**，所以加 `model.Todo` 字段会自动进 API，**这一点是便利也是陷阱**：它意味着**任何加到 model 上的字段都会自动暴露**。因此：
- `TodoInput` 必须**天生**不含敏感字段（不能说「反正 view 层会剥」）；
- 若 `handler` 另有独立的 `todoView` 用于别的接口（如任务列表），需一并核对（实施时 grep `TodoView`）。

### 5.2 Mongo 镜像
`Store` 全内存、Mongo 仅镜像 ⇒ `bson` tag 必须与 `json` tag 同时加，否则重启回读后字段丢失（表现为「重启后前端就看不到输入位了」这种极难定位的现象）。

### 5.3 `PublishTodoAnswer` 没有 `sc`
它的签名是 `(ctx, task, todoID, question)`。落库有两种写法：
- **(a)** 用不带 scope 的内部方法（即 §3.2 的设计，`task.OrgID` 已在 `task` 里）；
- (b) 从 `task` 构造 scope 再走公开方法。
**选 (a)**：该函数是 handler 内部、调用前 `AnswerTodo` 已做过权限校验，再校验一次是冗余。

### 5.4 写放大
每次派发多一次 Mongo 写。与既有 `RecordTodoDispatch` **同一量级**（它已经在每次派发时写），可接受。

### 5.5 并发
必须走 `mutateTaskUnsafe`（内部持 `s.mu`），**不要**自己取 `s.tasks[taskID]` 改 —— 全内存 Store 的既定纪律。

---

## 6. 测试与验收

**闸门**：`python _be_gate.py fmt build test race`（`-race` 必须 `golang:1.25.1` debian）＋ 前端 `node _fe_gate.mjs lint tsc t2 t3 test build`。

**生产验收（可复现的构造法）**：
1. 取一个「输入位必然跨任务、且源任务无产物」的任务 —— 即**档 1 守卫会跳过**的情形（canceled **且** 该步骤无交付物）；
2. 派发该 todo，`GET /api/v1/tasks/:id` ⇒ 断言 `todos[].inputs[]` 里存在 `resolved=false` 且 `name` 正确；
3. **反向**：正常任务（本次的 TD_02）应显示 `resolved=true` —— 它现在就是现成的正例。
4. **泄漏守卫**：对同一响应 grep `download_url` / `file_ref` ⇒ **必须为 0**。

**⚠️ 观测窗口纪律**（本次事故的教训）：日志/字段「没有出现」只有在「确实发生过会触发它的路径」时才叫证据 ⇒ 验收前必须先证明**机会存在**（真的派发过、真的有区间外输入位）。

---

## 7. 明确不做（附理由）

### 7.1 不做「输入位缺失」的独立通知
见 D2。与 09-24 确立的**通知收窄**原则一致：通知只服务于「要人做决定」。若将来发现 agent 兜住失败时用户确实需要被告知，**也应该先改 agent 的 `todo.ask` 行为（执行侧），而不是加一条平台通知**。

### 7.2 不做资产 `@码` 一致性校验
本次 TD_02 的 4/7 组受阻（缺 `@DJ001`、`@CJ007`、整卡 `@DJ007`、`@JS002` 编号口径）**全部发生在文件内容里**（`分组视频生成提示词` 是一份 markdown）。平台侧：
- **不解析交付物内容**（设计如此，也不该为此破例）；
- 因此**无法**知道「提示词引用了画布上不存在的资产」。

⇒ 这类问题的正确落点是**执行侧 skill**（让「分镜分组」产出的提示词只引用已确认出图的资产，或让「资产制作」的跳过决策被下游感知）。用户已明确搁置 skill 线，**建议单独立项，不在本方案范围内**。

### 7.3 不做「创建时静态依赖标注」（列为 P1）
「本任务哪些输入位来自区间外 ⇒ 依赖前序任务」是**纯静态信息**（读 `workflow_ref` + `steps[].inputs` 即可），零运行时风险，但：
- 它的边际价值在 P0 完成后下降（P0 已经能显示**当前**是否拿得到）；
- 它引入「事前预警的分级语义」（硬伤 / 待产出）需要额外设计。

⇒ 列 P1，等 P0 落地并观察真实使用后再定。

---

## 8. 分期

| 期 | 内容 | 预估改动面 |
|---|---|---|
| **P0** | §3.1–3.5：落库 + API 暴露 + 前端展示 + 泄漏守卫测试 | 后端 3 文件 + 前端 2–3 文件 |
| P1 | 创建时的静态依赖标注（§7.3） | 后端 2 文件 + 前端 1 文件 |
| P2 | 把「输入位缺失」纳入任务健康度/待办清单（`pendingItems`） | 前端为主 |

---

## 9. 我的建议（原三个「待确认点」的结论）

### 9.1 `TodoInput` 应加 `State` 三态，不能只有布尔 —— **已采纳，见 §3.1**
`resolved=false` 是**无差别**的：用户看到「未就绪」分不清是
**「上游还在跑（等就行）」** 还是 **「上游已终态、产物不会再有（必须人工介入）」** —— 两者处置**相反**。
只给布尔值，用户还得来问人，**方案的核心价值就落空了**。判定复用 `FindTaskForWorkflowStep` 已有的候选遍历，**不引入二次扫描**。

### 9.2 落库点必须覆盖 `remindTodo`，且应**收口在 `BuildTodoInputs`** —— **已采纳，见 §3.3**
原方案只列了 handler 的 4 处，**漏了 `remindTodo`（本次事故通道！）和 `buildTodoAssignedPayload`（它还覆盖 4 个出口）**。
- `todo.resume` **必须覆盖**：本次事故就是用户在 `failed` 状态下点「重试/继续」。若那次仍解析不到、而用户看不到任何提示，**等于白点**；
- `todo.reopen` 与 `todo.resume` **是同一流程**（`ResumeTodo → ReopenTodo → todo.remind(user_resume)`），覆盖 `remindTodo` 即自动覆盖；
- 收口比散点更安全（§3.3 的「漏改论证」）。

### 9.3 只在任务详情页展示，**不上列表** —— **建议照此实施**
列表接口的读放大**正是 §〇ter 那次事故的直接成因**（542 KB / 无分页 / 3–4 s 全量重拉）。
为「输入位缺失」这种**罕见事件**让**每个列表响应**变重，是拿常见路径的性能换罕见路径的可读性。若将来确需在列表提示，
正确做法是加**标量** `input_issue_count int`（**不**把 `inputs` 数组塞进列表），归 P2。

### 9.4 一处取舍：要不要去重写库
`RecordTodoInputRefs` 每次 `BuildTodoInputs` 都会写一次 Mongo（即使结果没变）。
**建议 P0 不做去重** —— 调用频率＝每次派发/回答一次，与既有 `RecordTodoDispatch` 同量级。
若加「结果相等则跳过」，必须排除每次都变的 `CheckedAt`，**判定复杂度大于收益**。

## 10. 实施记录（2026-09-29 中午；**代码已写、闸门已跑、未提交**）

### 10.1 落点与 §3.3 完全一致
`BuildTodoInputs`（`clawsynapse/webhook.go`）是 **7 个解析点的唯一共同入口**
（`handler/task.go` :207 / :323 / :560 / :584 / :616；`clawsynapse/webhook.go` :1942 `remindTodo`、
:2427 `buildTodoAssignedPayload` 被 4 个出口共用）⇒ 收口在此，**handler 一行未改**。

| 文件 | 改动 |
|---|---|
| `model/task.go` | `Todo.Inputs []TodoInput`（输入侧对称 `Outputs`）＋ `TodoInput` 结构（**刻意比 `protocol.TodoInputRef` 窄**：无 `DownloadUrl`/`FileRef`）＋ `InputStateResolved/Pending/Missing` |
| `store/workflow.go` | 新增 `RecordTodoInputRefs(taskID, todoID, inputs)`：**空切片 no-op**、**跳过终态守卫** |
| `clawsynapse/webhook.go` | `resolveStepInput` 改为返回 `(*protocol.TodoInputRef, string)`；`BuildTodoInputs` 尾部落库（`h.store != nil` 守卫）；warn 增 `state` 字段 |

### 10.2 🔴 实施中修掉一处**契约与实现不一致**（方案 §3.1 已定义、初版实现未做到）
`TodoInput.State` 的注释与 §3.1 的判定表都写明：`pending` = 上游**仍活跃**（等就行），
`missing` = 无 holder **或 holder 已终态且无该产物**（必须人工介入）。但初版 `resolveStepInput`
只按「`candidates` 是否为空」分流 ⇒ **上游 todo 已终态但没产出该文件时会报 `pending`** ——
等于指示用户去等一件永远不会发生的事，而**这正是本次事故的形态**（前序任务被取消）。

修法：遍历完 candidates 未命中后，再遍历一次 ——
**只要存在一个非终态 holder 就 `pending`，全部终态才 `missing`**；
新增 `todoStatusTerminal`（`done|failed|canceled`，与 `store_artifact.go` 归档路径的终态集保持一致，
避免第二套口径）。
📌 **通律：字段语义写在注释里，就必须在实现里逐支对上**（与 §四元教训 1 同源）——
否则前端会据此给出反向的行动指引，而这类偏差编译、类型、静态检查**全部抓不到**。

### 10.3 测试（4 个文件 / 14 条新用例，逐条 verbose 证明「确实执行」）
| 文件 | 覆盖 |
|---|---|
| `store/workflow_input_refs_test.go` | 落库 / 整片替换（非追加）/ 空切片 no-op / 终态 todo 可写 / 未知目标不污染 |
| `clawsynapse/webhook_input_state_test.go` | **三态表**：resolved、live-holder→pending、**terminal-holder→missing**、no-holder→missing；＋ 落库断言（State/Name/SourceStep/OutputName/CheckedAt）＋ resolved 带 `file_name`+`file_size` ＋ 无输入位不落库 ＋ warn 带 `state` ＋ 终态前序**严格名匹配仍不解析** |
| `model/task_todo_input_json_test.go` | **允许键白名单**泄漏守卫（多一个字段就红，失败信息即 review）＋ `state` 永不被 `omitempty` 吃掉 ＋ `Outputs`/`Inputs` 省略策略一致 |
| `handler/task_detail_inputs_test.go` | 真 `TaskHandler.Get` + 真 gin context：**正向**（API 现在能返回 `inputs` —— 此前完全没有，这是 L2 的核心）＋ **反向**（`download_url`/`file_ref` **逐键断言**）＋ 未派发过的 todo 不带 `inputs` |

### 10.4 闸门
- `_be_gate.py` 的 `COPY` 已更新为本轮 7 个文件；`race` 覆盖面扩到 **4 包**（加 `./internal/model/`）；
  新增 `vrefs` / `vstate` / `vdetail` / `vmodel` 四个定向 verbose 步骤。
- `fmt build vrefs vstate vdetail vmodel` → **rc 全 0**（gofmt 干净、build 30.8s、14 条用例全 PASS）。

### 10.5 未做（明确留白）
- **§3.5 前端渲染**未做：任务详情页的「输入位 N/M 已就绪 + 未就绪明细」。后端契约已就绪
  （`todos[].inputs[]` 带 `state`），随时可接。
- **未提交、未推送、未部署**（本地提交是常态，`push`/部署是单独的刻意动作）。
- 测试只到「handler 层真 HTTP 响应」为止，**未经生产 E2E**（本轮改动不涉及出站 payload，
  因此不适用上一轮的 daemon `contentLength` 字节级判据；后续如需生产实证，看任务详情接口的
  `inputs[]` 即可）。
