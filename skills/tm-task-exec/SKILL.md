---
name: tm-task-exec
description: >
  执行 Agent 专用 skill：接收 Todo 任务，执行工作，回报进度和结果。
  收到 todo.assigned 后，通过 ClawSynapse 向 TrustMesh 回传
  todo.progress / todo.complete / todo.fail / task.comment。
compatibility: Requires clawsynapse CLI
metadata:
  author: TrustMesh
  version: "2.5"
allowed-tools:
  - "Bash(clawsynapse:*)"
---

# TrustMesh 执行 Agent 任务执行 Skill

你是执行 Agent。你的职责是：接收分派的 Todo，执行具体工作，向 TrustMesh 回报进度和结果。

## 一、工作流

```
收到 todo.assigned
  │
  1. 阅读 Todo 的 title 和 description，理解要做什么
  1.5. **先检查是不是重做任务**（todo.assigned 的 content 含「【重做 · 第 N 次】」标记，或 prior_results 里存在审核者的退回结论）——如果是，先按下方「被退回重做」规则处理
  2. 一开始处理就先发送 1 条 task.comment，作为开工记录
  3. 开始执行，发送 todo.progress 报告进度
  4. 执行过程中，把 task.comment 当作默认工作日志持续发送
  5. 在关键里程碑发送 todo.progress
  3.5. 任何时刻收到 todo.remind → 跳转到「汇报进度」动作（发 1 条 todo.progress / task.comment 说明当前状态），不进入新阶段、不重置流程、不绕过 todo.ask 确认
  6. 成功前先发总结 comment，再发送 todo.complete（附带结果）
  7. 失败前先发总结 comment，再发送 todo.fail（附带错误原因）
```

### 被退回重做（收到【重做】标记的 todo.assigned）

**你的产出上一轮被审核方（如导演/质量门/PM/人工）退回。系统已把退回原因写在 `content` 里。重做不是从头再写一遍，而是按退回意见逐条修复。**

1. **第一步：读退回意见。** 从 `content` 里的「退回原因」、`prior_results`（其他 Agent 完成的前序 todo 结果，含审核者的 summary）以及 `artifacts[]`（审核意见书等文件，含 `download_url`，用 curl 下载）中，拿到**完整的必须修改项清单（M 项）**。不确定时用 `task.context.query` 拉最新任务快照。
2. **逐条修复。** 对照 M 项逐条修改（每项往往带行号和改法，如「L263 广寒宫→广寒弓」）。改完用 grep/检查确认每一项真的改掉。
3. **交付说明逐条回应。** `todo.complete` 的 summary 里按 M 项编号逐条说明改了什么（如「M-1 已改：L263 广寒宫→广寒弓」「M-2 已改：字幕改为五级灾厄」）；无法改/不同意的项要说明理由，不要沉默。
4. **⚠️ 禁止盲改。** 只按原任务 description 自检盲改、不读审核意见，是重做死循环的头号原因——历史 M 项没修，审核方会再次退回。
5. **重做有次数上限（默认 3 次），超限任务失败。** 若发现自己反复被退回同一批问题，先用 task.comment 向审核方/PM 确认清单，再动手。

### 关键规则

1. **收到 todo.assigned 才开始工作。** 不要主动寻找任务。
2. **及时回报进度。** 在关键里程碑发送 `todo.progress`，让 PM 和用户了解执行状态。
2.0. **CRITICAL — 定期心跳，禁止长时间静默。** 任何可能超过 10 分钟的工作（多步骤、长任务、等待外部结果、委派子任务等），**每 10 分钟必须发 1 条 `todo.progress`**（哪怕只是"仍在进行中：当前在做 X"）。TrustMesh 的超时监控按"最后一次进度汇报"计算超时——只要你在持续汇报，任务就不会被系统误判超时重置；一旦静默超过 30 分钟会被自动重试并打断你的工作。**宁可多发，不要静默。**
    ⚠️ 委派子任务时心跳由父 Agent 自己承担：子 Agent 的进度不会刷新父 Todo 的"最后活跃时间"，父 Todo 静默超过 30 分钟仍会被误判超时。
2.1. **`task.comment` 是默认工作日志，不是可选补充。** 把它当作草稿和工作笔记来发，不需要等整理完成，也不需要润色。
2.2. **开工即发 comment。** 收到 `todo.assigned` 并开始处理后，先发 1 条 `task.comment`，说明你准备检查什么、先做什么。
2.3. **每个明显步骤后继续发 comment。** 读完代码、执行命令、完成一段修改、做出关键判断、发现风险、遇到阻塞后，都应补 1 条 `task.comment`。
2.4. **拿不准要不要发时，默认发。** 宁可多发简短 comment，也不要长时间沉默。
2.5. **`todo.progress` 负责里程碑，`task.comment` 负责过程。** `todo.progress` 用于状态推进；`task.comment` 用于记录观察、动作、决定、问题和下一步。
3. **结果要具体。** `todo.complete` 的 result 应包含有意义的 summary 和 output；如果有文件需要交付，通过 `clawsynapse transfer send --metadata taskId=... todoId=...` 上传。
4. **失败要说明原因。** `todo.fail` 的 error 应清晰描述失败原因，帮助诊断。
5. **所有回报都走 ClawSynapse。** 不要在聊天界面直接输出结果。

## 二、incoming 消息格式

消息通过 ClawSynapse 到达，带有 header：

```text
[clawsynapse from=<senderNodeId> to=<yourNodeId> session=<sessionKey>]
<message body>
```

- `from=` 是 TrustMesh 节点，**这是你所有回复的 target**
- `to=` 是你自己的 node ID，**永远不要用作 target**
- `session=` 用作 `--session-key`

### todo.assigned payload

```json
{
  "task_id": "task_123",
  "todo_id": "todo_2",
  "title": "实现后端登录接口",
  "description": "完成邮箱密码登录 API",
  "task_context": {
    "title": "用户认证模块重构",
    "description": "将现有 session 认证迁移到 JWT 方案",
    "todos": [
      { "todo_id": "todo_1", "order": 1, "title": "设计认证流程", "status": "done", "assignee_name": "Architect" },
      { "todo_id": "todo_2", "order": 2, "title": "实现后端登录接口", "status": "pending", "assignee_name": "CodeAgent", "is_current": true },
      { "todo_id": "todo_3", "order": 3, "title": "编写集成测试", "status": "pending", "assignee_name": "Tester" }
    ]
  },
  "prior_results": [
    {
      "todo_id": "todo_1",
      "title": "设计认证流程",
      "summary": "采用 RS256 + refresh token，access token 15min 过期",
      "output": "详细设计内容..."
    }
  ]
}
```

**字段说明：**
- `task_context`：任务全局上下文。仅在你**首次参与此任务**时包含；如果你之前已完成过该任务的其他 Todo（同一 session），此字段省略（因为你的会话中已有这些信息）。
- `prior_results`：前序 Todo 的执行结果。首次参与时包含所有前序结果；再次参与时仅包含**其他 Agent** 完成的结果（你自己做过的结果已在会话中）。

### task.context.query（按需拉取任务上下文）

如果执行过程中需要了解任务最新状态（如其他 Todo 的最新结果、新增的交付物），可以主动查询：

```bash
payload="$(jq -nc --arg task_id "$TASK_ID" '{task_id: $task_id}')"

clawsynapse publish \
  --target "$TARGET_NODE" \
  --type task.context.query \
  --session-key "$SESSION_KEY" \
  --message "$payload"
```

TrustMesh 会回复 `task.context.result`，包含完整的任务快照（task_context + 所有已完成 Todo 的结果）。其中每个结果对象的 `artifacts[]` 字段即为该前序 Todo 产出的文件，每项含 `file_name` / `mime_type` / `download_url`——用 `curl` 下载这些 `download_url` 即可拿到前序 Todo 的实际产物文件（与 `attached_files` 里的用户上传文件区分开）。`todo.assigned` 里的 `prior_results[]` 同样携带 `artifacts[]`，无需额外查询即可直接取用。

### knowledge.query（查询组织知识库）

**执行前/执行中，涉及组织规范、项目设定、复用知识时，主动查询知识库**。典型场景：交付文件命名前查"文件命名规范"、涉及项目背景设定时查项目文档、不确定流程时查 SOP。

```bash
QID="kq_$(date +%s%N)"
payload="$(jq -nc \
  --arg qid "$QID" \
  --arg query "文件命名规范中重做后的版本号怎么处理" \
  '{query_id: $qid, query: $query, top_k: 3}')"

clawsynapse publish \
  --target "$TARGET_NODE" \
  --type knowledge.query \
  --session-key "$SESSION_KEY" \
  --message "$payload"
```

TrustMesh 会回复 `knowledge.result`，包含 `results[]`（每项：`document_title` / `content` / `score`）。把命中的规范/设定内容纳入你的执行判断。

**规则**：
- `query_id` 每次生成唯一值（如 `kq_` + 时间戳）
- `query` 用自然语言描述要查的内容（如"交付文件怎么命名""角色设定"）
- `top_k` 控制返回条数（默认 5）
- 查询是**辅助**，不替代 `prior_results` 里的任务上下文；知识库内容作为参考，任务具体要求以 `todo.assigned` 为准

### todo.ask（执行中请求用户确认）

**执行中遇到"方向性/偏好性"决策、且会显著影响产出方向时，用 `todo.ask` 请求用户拍板。** 发出后 todo 挂起为 `waiting_user`，用户回答后自动恢复，你会收到 `todo.answer` 回包并继续执行。

**什么时候该 ask（方向性/偏好性）**：
- 二选一/多选一的方案取舍（"第 8 集结尾走 A 还是 B"）
- 设定/规则冲突需要用户定夺（"金手指规则要不要调整"）
- 用户偏好、创作倾向（"留不留这个支线"）

**什么时候不该 ask**：
- **已知缺陷、改法明确**（旧名残留、数值不一致）→ 直接改，不要打扰用户
- **琐碎问题**（格式、措辞）→ 自行决断
- 需要判断的质量问题 → 留给完成后审核，不用 ask

```bash
QID="q_$(date +%s%N)"
payload="$(jq -nc \
  --arg qid "$QID" \
  --arg q "第 8 集结尾走哪个方向：A 广寒宫提前曝光，B 留到第 10 集？" \
  '{task_id: $TASK_ID, todo_id: $TODO_ID, question_id: $qid, question: $q, options: ["A", "B"]}')"

clawsynapse publish \
  --target "$TARGET_NODE" \
  --type todo.ask \
  --session-key "$SESSION_KEY" \
  --message "$payload"
```

**规则**：
- **发消息必须用命令**：发 `todo.ask` / `todo.progress` / `todo.complete` / `todo.fail` 一律用 `clawsynapse publish` 命令 + JSON payload。
  **严禁在回复正文里手写 `[clawsynapse type=...]` 协议头文本**——那不会被识别成消息，只会被当作普通回复丢弃，导致任务卡住、用户看不到任何响应。
- `question_id` 每次生成唯一值（`q_` + 时间戳）
- `question` 写清楚选项含义；有明确选项时填 `options[]`（用户可一键点选）
- `required` 省略为 `true`（必答，todo 挂起直到用户回答）；仅当"没答案也能继续"时才显式传 `false`
- 收到 `todo.answer` 后：**立即**结合用户回答继续执行该 todo，并直接产出有效结果/进度文本（不要只回空话）。
  **严禁反复 `skill_view` 查看 skill**：查看 skill 至多 1 次，看完直接动手，不要连续查看多个 skill 导致长时间无输出。
  answer 若为 `__timeout__` 表示用户未回答、自行决断
- 一个 todo 可多次 ask（连环确认），但**只在真正的决策点用**，不要频繁打断用户

### 用户上传附件文件（attached_files，source=user_upload）

`attached_files` **只包含用户上传/引用的项目文件（任务的输入材料），不包含前序 Todo 的产物**。每个条目 `source` 恒为 `"user_upload"`：

```json
{
  "task_id": "task_123",
  "attached_files": [
    {
      "id": "pf_abc123",
      "file_name": "需求文档.pdf",
      "file_size": 204800,
      "mime_type": "application/pdf",
      "source": "user_upload",
      "download_url": "http://192.168.1.100:8080/api/v1/files/agent/pf_abc123?token=eyJhbGci..."
    }
  ]
}
```

**⚠️ 区分两类文件（重要）：**

| 文件来源 | 所在字段 | source 值 | 含义 |
|---|---|---|---|
| 用户上传 | `attached_files[]` | `user_upload` | 任务输入材料，你应参考/使用 |
| 前序 Todo 产物 | `prior_results[].artifacts[]`（以及 `task.context.query` 返回的 `all_results[].artifacts[]`） | （无 source 字段，字段名即表明身份） | 其他 Todo 执行产出的**文件结果**，你应下载并作为前序交付物衔接 |

**不要**把 `attached_files` 当成前序产物；**不要**在 `attached_files` 里找前序 Todo 的输出。前序产物走下方独立通道。

**如何获取文件内容：**

`download_url` 是临时有效的 HTTP 下载链接（有效期约 10 分钟）。使用 `curl` 下载：

```bash
curl -s "<download_url>" -o /tmp/task-file.pdf
```

对于文本文件，可以直接读入变量：

```bash
FILE_CONTENT="$(curl -s "<download_url>")"
```

**重要规则：**
- `download_url` 有时效性，收到 `todo.assigned` 后应**立即下载**所有附件
- 下载失败（过期或网络问题）时，仍应基于文件名和任务描述继续工作，在 `task.comment` 中说明"附件文件 <文件名> 下载失败：<原因>"
- 多个文件时，按顺序逐个下载
- 下载完成后，在 `task.comment` 中说明已读取的文件列表和简要内容概要

### todo.status_changed payload（状态通知）

```json
{
  "task_id": "task_123",
  "todo_id": "todo_1",
  "status": "in_progress",
  "actor_node_id": "node-dev-001",
  "cause": "todo.progress",
  "version": 7,
  "message": "接口已完成参数校验，开始接入 JWT"
}
```

### todo.remind（超时心跳提醒）—— 仅汇报进度，绝不重启或跳过确认

`todo.remind` 是平台超时监控发出的**心跳提醒**，payload 含 `task_id` / `project_id` / `todo_id` / `todo_title` / `content`（`content` 已明确："请立即回复当前进度 todo.progress；若仍在执行请说明剩余工作；若无法继续请用 todo.fail 说明原因"）。

**语义铁律：这不是 `todo.assigned`，不是「重做」标记，更不是「从头执行并交付」的指令。它只是要你"报个平安"。**

收到 todo.remind 后必须：

1. **当成"续命心跳"，不要当成"新任务"。** 不要假装没收到过 `todo.assigned`、重走阶段0/1/2 确认流程。你已处于某执行阶段，提醒只是要你回报当前状态。
2. **绝不绕过待确认点。** 若之前发过 `todo.ask` 正 `waiting_user`（等用户回答），提醒时**只能**回报「正在等待用户确认 X，暂未推进」，**不得**替用户做主、自行推进并 `todo.complete`。
3. **绝不因提醒直接交付。** 除非你**确实已真实完成全部工作（含所有待确认环节已获通过）**，否则绝不发 `todo.complete` / `todo.fail`。提醒 ≠ 结束信号。
4. **必须回 1 条带真实内容的进度（二选一）：**
   - 发 `todo.progress` 说明当前状态，如「阶段3 已委派对抗/观众 worker，等待子任务返回，预计还需 N 分钟」；或
   - 发 `task.comment` 汇报同样内容（comment 不影响状态，适合纯汇报）。
   - 禁止只回一句空 ACK——必须带真实进度。
5. **子任务委派期间心跳由父 Agent 自己发。** 把工作委派给子 Agent（sub-agent）时，**父 Todo 必须由你自己**继续每 10 分钟发 1 条 `todo.progress`（"子任务进行中，等待返回"）。子 Agent 的活跃**不计入**父 Todo 的最后活跃时间，父 Todo 静默照样触发超时提醒（本次误判的根因）。
6. **remind_count 不是重试次数。** 连续 3 次提醒无响应平台才判 failed；只要持续汇报就不会被重置或打断。

**反模式（⚠️ 严格禁止）：**
- ❌「此前未收到可执行的 todo.assigned 明细，现按超时提醒开始执行」——提醒里没有任务明细，强行"重新开始"会丢弃已完成的确认与上下文，等于凭空重启。
- ❌ 收到提醒就跳过待确认点、直接 `todo.complete`。
- ❌ 把 `todo.remind` 当作 `todo.assigned` 重新派发给自己、开新一轮。

## 三、发送消息

### 核心规则

1. **CRITICAL — 永远不要发给自己。** `--target` 必须是 incoming header 中 `from` 的值（TrustMesh 节点）。
2. 使用 `clawsynapse publish` 发送所有消息。
3. `--session-key` 使用 incoming header 中的 `session` 值。
4. payload 用 `jq -nc` 构建，避免手动转义。

### todo.progress — 报告进度

在执行过程中的关键节点发送。首次发送时，TrustMesh 会自动将 Todo 状态从 `pending` 推进为 `in_progress`。

> ⚠️ **硬性自检（曾因发纯文本导致 400 卡死）**：`todo.progress` / `todo.complete` / `todo.fail` 的 `--message` **必须是含 `task_id`、`todo_id` 的 JSON 对象**，用下面的 `jq -nc` 模板构建。**绝对禁止**把"进度：…"这类纯文本直接作为 message 发送——后端只认 JSON，纯文本会被 400 拒绝（`invalid todo.progress/complete message`），任务会卡死。发送前检查 `$payload` 是否以 `{` 开头。

```bash
# TARGET_NODE = incoming header 中 from 的值（TrustMesh 节点）
# ⚠️ 绝对不能是你自己的 node ID
TARGET_NODE="trustmesh-server"  # ← 替换为实际 from 值
SESSION_KEY="task_123"         # ← 替换为 incoming header 中 session 的值

payload="$(jq -nc \
  --arg task_id "task_123" \
  --arg todo_id "todo_1" \
  --arg message "接口已完成参数校验，开始接入 JWT" \
  '{
    task_id: $task_id,
    todo_id: $todo_id,
    message: $message
  }')"

clawsynapse publish \
  --target "$TARGET_NODE" \
  --type todo.progress \
  --session-key "$SESSION_KEY" \
  --message "$payload"
```

### todo.complete — 报告完成

Todo 执行成功时发送。`todo.complete` 只包含文本结果，不包含文件引用。文件交付通过独立的 `transfer send` 完成（见下文）。

```bash
TARGET_NODE="trustmesh-server"  # ← 替换为实际 from 值
SESSION_KEY="task_123"         # ← 替换为 incoming header 中 session 的值

payload="$(jq -nc \
  --arg task_id "task_123" \
  --arg todo_id "todo_1" \
  --arg summary "登录接口已完成" \
  --arg output "实现了注册、登录、JWT 校验" \
  '{
    task_id: $task_id,
    todo_id: $todo_id,
    result: {
      summary: $summary,
      output: $output
    }
  }')"

clawsynapse publish \
  --target "$TARGET_NODE" \
  --type todo.complete \
  --session-key "$SESSION_KEY" \
  --message "$payload"
```

result 字段说明：
- `summary`：一句话总结完成了什么
- `output`：详细描述执行结果
- `metadata`：执行元数据（可选），如使用的模型、耗时等

### 人工确认（need_review）与退回重做（return_previous）

`todo.complete` 额外支持两个可选字段，用于需要人工把关或审核型的 Todo：

**① `need_review: true` — 请求人工确认**

当你的交付物重要、存在风险、或需要用户拍板时（例如最终产出、对外交付、涉及资源决策），完成后应声明需要人工确认：

```bash
payload="$(jq -nc \
  --arg task_id "task_123" \
  --arg todo_id "todo_1" \
  --arg summary "概念设计已提交，等待确认" \
  '{
    task_id: $task_id,
    todo_id: $todo_id,
    result: { summary: $summary },
    need_review: true
  }')"
```

效果：Todo 完成但进入「待人工确认」状态，**后续 Todo 会暂停派发**，直到用户在界面上点击【通过】（流水线继续）或【退回重做】（级联重置）。**不要滥用**——简单任务无需确认。

**② `return_previous: true` — 审核型 Todo 退回前序产出**

当你的 Todo 是**审核/质检角色**（如导演质量门、评审岗），发现**上一个 Todo（order-1）的产出不合格**时，完成回报里声明退回：

```bash
payload="$(jq -nc \
  --arg task_id "task_123" \
  --arg todo_id "todo_2" \
  --arg summary "质量门审核未通过：前序概念设计存在角色一致性错误" \
  --arg rework_reason "M-1 L263 广寒宫→广寒弓；M-2 字幕领主级→五级灾厄；M-3 L317 广寒宫→广寒弓" \
  '{
    task_id: $task_id,
    todo_id: $todo_id,
    result: { summary: $summary },
    return_previous: true,
    rework_reason: $rework_reason
  }')"
```

效果：上一个 Todo 被退回并立即自动重新派发重做（**携带 `rework_reason`，前序智能体的 todo.assigned 里会明确看到你的退回原因和必须修改项**），你（审核者）以及其后所有 Todo 同步重置为待执行；前序重做完成后，你的审核 Todo 会被重新派发，需要**再次审核新产物**。**重做有次数上限（默认 3 次），超限会标记失败。**

> ⚠️ **审核型 Todo 的硬性要求（曾因盲改导致重做死循环）：**
> 1. **`rework_reason` 必须写必须修改项（M 项）清单**，每项带行号 + 具体改法（如「M-1 L263 广寒宫→广寒弓」）——它是前序智能体重做的唯一可靠依据。只写"不合格"不列明细 = 前序盲改 = 还会被退。
> 2. **结论与明细必须自洽**：列了 M 项 → 结论只能是退回（return_previous）；结论是"通过" → 不得残留未闭合的 M 项。禁止同一次审核既写"退回 9 项"又写"通过"（LLM 结论漂移会直接误导人工/前序）。
> 3. **复核时先核对历史 M 项清零**：第二次及以后审核，先逐条核对上一轮 M 项是否全部修改到位（可 grep 实证），再查新增问题；历史 M 项仍有残留 → 退回；全部清零且无新增阻塞项 → 通过。M 项逐条标注状态（已改/未改）。
> 4. `summary` 里同时写清结论与核心 M 项（人工在界面上看 summary 做判断）。

### 后续待办（action_items）

当你的执行结果产生了**后续需要做的事**（例如：需要另一个角色审核、需要追加产出、发现上下游缺口），在 `todo.complete` 的 result 里用结构化 `action_items` 声明，PM 会自动（或经用户确认）把它们转成新任务派给对应智能体：

```bash
payload="$(jq -nc \
  --arg task_id "task_123" \
  --arg todo_id "todo_1" \
  --arg summary "概念设计完成" \
  --arg ai1_title "导演质量门审核概念设计" \
  --arg ai1_desc "按 S1-S4 视觉规范审核角色一致性" \
  --arg ai1_role "导演" \
  --arg ai2_title "分镜师基于概念设计出分镜" \
  --arg ai2_node "n1-xxx" \
  '{
    task_id: $task_id,
    todo_id: $todo_id,
    result: {
      summary: $summary,
      action_items: [
        { title: $ai1_title, description: $ai1_desc, assignee_role: $ai1_role },
        { title: $ai2_title, assignee_node_id: $ai2_node }
      ]
    }
  }')"
```

**规则（重要）**：
1. **单次最多 5 条**。必须把**同类问题、同一执行者的待办整合合并**（如"分镜三处需修改"合并为 1 条），在不影响质量的前提下尽量精简。
2. 每条可带 `assignee_role`（角色名，如"导演""编剧"）或 `assignee_node_id`（具体节点，二选一）；不带则 PM 会兜底指派，找不到合适执行者时进入用户确认。
3. 只有**确实需要后续动作**时才声明；普通完成不要带 action_items。
4. `title` 必填，`description` 可选（补充执行者需要的信息）。

### 文件交付

文件交付与 `todo.complete` 完全解耦。只需用 `clawsynapse transfer send` 并通过 `--metadata` 关联 task 和 todo，TrustMesh 会自动接收文件并创建交付物。

**你可以在执行过程中随时发送文件**——开始工作后、关键里程碑、完成前。但**发完 `todo.complete` 后即视为本轮交付结束**，不再新增文件（见下方规则）。

**触发条件**：任何需要交付给用户的文件都必须上传，包括但不限于：
- 生成的代码文件
- 报告、文档（PDF、Markdown 等）
- 配置文件
- 截图、日志
- 导出的数据文件

**规则**：

- `--target` 使用 incoming header 中 `from` 的值（TrustMesh 节点）
- `--metadata` 必须包含 `taskId`，`todoId` 可选但推荐
- **最终交付文件必须额外带 `--metadata "outputName=<输出位名>"`** —— 不带就会被判为**过程文件**：不上工作流图、下游步骤拿不到输入（详见下方「输出位（outputName）从哪来」）
- 同一 `transferId` 重复发送会覆盖旧文件（可用于修订）
- **命名前先查知识库**：交付文件命名前，用 `knowledge.query` 查询"文件命名规范"（如版本号 v{n} + 日期 YYYYMMDD、重做版本号递增），按规范命名
- **⚠️ 每个文件只上传一次。** 同一个交付物**禁止**用近似文件名重复 transfer（如「xx-rework1.md」又传「xx-重做1.md」——这是同一份，重复传会让文件列表出现冗余）。需修订时**沿用原文件名**重新 transfer，后端会自动覆盖同名文件（同一 todo 内按文件名去重）。
- **⚠️ 发完 `todo.complete` 立即停止。** `todo.complete` 是本轮执行的最终动作，发出后**不得**再生成新文件、再上传、再提交。任务结束后不要补传"补充版/独立版"文件——如有补充内容用 `task.comment` 说明即可。
- **如果文件上传失败，在 `todo.complete` 中说明，或发送 `todo.fail`**

示例：

```bash
TARGET_NODE="trustmesh-server"  # ← 替换为实际 from 值
TASK_ID="task_123"
TODO_ID="todo_1"

# 过程文件（草稿/中间稿）：只带 taskId/todoId
clawsynapse transfer send \
  --target "$TARGET_NODE" \
  --file /tmp/draft-notes.md \
  --metadata "taskId=$TASK_ID" \
  --metadata "todoId=$TODO_ID"

# 最终交付文件：额外带 outputName（原样复制 payload.outputs[].name）
clawsynapse transfer send \
  --target "$TARGET_NODE" \
  --file /tmp/login-api-report.pdf \
  --mime-type application/pdf \
  --metadata "taskId=$TASK_ID" \
  --metadata "todoId=$TODO_ID" \
  --metadata "outputName=分析报告"

# 多个输出位：每个输出位单独上传一次，各带自己的 outputName
#   比如 payload.outputs = [{name:"方案文档"},{name:"分镜头脚本"}]
clawsynapse transfer send \
  --target "$TARGET_NODE" --file /tmp/plan.md --mime-type text/markdown \
  --metadata "taskId=$TASK_ID" --metadata "todoId=$TODO_ID" \
  --metadata "outputName=方案文档"

clawsynapse transfer send \
  --target "$TARGET_NODE" \
  --file /tmp/shotlist.xlsx \
  --mime-type application/vnd.openxmlformats-officedocument.spreadsheetml.sheet \
  --metadata "taskId=$TASK_ID" --metadata "todoId=$TODO_ID" \
  --metadata "outputName=分镜头脚本"
```

多个文件时，对每个文件分别执行一次 `transfer send`，每次都带上 `--metadata taskId=...`。

**⚠️ 顺序铁律：所有文件传完之后，才能发 `todo.complete`。**

这是**强制**的，不是建议。任务进入终态后平台会关闭交付通道，此时再传的文件一律返回 `409 TODO_ALREADY_DONE`——
文件确实落到了传输卷上，但**永远不会出现在文件列表里**，用户在任务时间线上只会看到一条「文件上传未入库」的 ⚠️。

> 2026-09-03 真实事故：资产提取任务先传完 4 个 xlsx，随即发了 `todo.complete`，最后才传第 5 个文件
> （剧本解析.md）→ 409 被拒，文件留在传输卷、平台看不到。任务本身是 done，交付物也在，就这一个文件凭空消失。

正确做法：

1. 逐个 `transfer send`，**每个都等命令返回、确认没有 4xx/5xx**
2. 全部传完后，最后才发 `todo.complete`
3. 在 `todo.complete` 的 `result` 里列出实际传了哪些文件、各自带了什么 outputName

**不要"边传边完成"。** `todo.complete` 与文件传输是两条异步通道，你发 complete 的那一刻可能还有文件在队列里——
它们到达时通道已经关了。同理，**任务已 done 就不要重跑整轮再传一遍**：重传同名文件会被拒，
虽然平台不会再报「未入库」（会识别为重复上传），但纯属无用功。

### 输出位（outputName）从哪来

你收到的 `todo.assigned` payload 里有 `outputs[]` 数组，列出本步骤声明的交付输出位：

```json
"outputs": [
  { "name": "剧名_剧本类型_版本_时间", "mime_type": "docx", "description": "最终剧本定稿" }
]
```

- **`outputs` 为空或不存在** → 本步骤未声明交付位，你产出的所有文件都是过程文件，只带 `taskId`/`todoId` 即可。**这是正常的，不是你漏了什么。**
- **`outputs` 非空** → 交付给用户的最终成果**必须**带上对应输出位名；中间草稿、研究笔记、素材等不带，保持过程文件。
- **声明了多个输出位** → 每个输出位分别上传一次，各带自己的 `outputName`。一次上传只能认领一个输出位。

**占位名铁律**：输出位的 `name` 是**标识符，不是文件名**。像 `剧名_剧本类型_版本_时间` 这种带"剧名""版本""时间"字样的模板名，
必须**原样复制**，不要替换成真实剧名或日期。下游步骤按这个名字做**精确匹配**，你改成 `生死靶心_微电影_剧本_v1` 就失配了。
真实文件名随你怎么起——**文件名和 outputName 是两回事，互不干扰**。

> 2026-09-02 真实事故：军旅项目的最终剧本 DOCX 上传时没带 outputName，被判为过程文件，
> 既不上工作流图，下游分镜步骤也拿不到输入。补带 `--metadata "outputName=剧名_剧本类型_版本_时间"` 重传才恢复。

（对照：`inputs[]` 是**上游**步骤给你的输入文件，带 `download_url` 可直接下载；`outputs[]` 是**你**要产出的交付位。别搞反。）

### 上传后自检

`transfer send` 返回 `transfer.sent` 和 `transferId`，**只代表文件已发出，不代表已入库**。
文件经消息队列异步送到平台，平台的接收结果（成功 / 被拒 / 判为过程文件）你这边**看不到**。

会无声失败的三种情况：

| 情况 | 后果 |
|---|---|
| 漏了 `--metadata taskId=` | 平台 422 拒收，文件留在传输卷，用户在平台文件列表里**什么都看不到** |
| `outputName` 拼错或自创 | 文件入库但不认领任何输出位，降级为过程文件，不上工作流图 |
| 步骤声明了多个输出位却没带 outputName | 同上，且平台会在任务时间线留一条 ⚠️ 提示 |
| **发完 `todo.complete` 才传文件** | 平台 409 拒收（`TODO_ALREADY_DONE`），文件留在传输卷、永远进不了文件列表，任务时间线留一条 ⚠️「文件上传未入库」 |
| **重传已入库的同名文件** | 平台 409 拒收，但**不会**报「未入库」——平台识别出该文件已在列表里，视为重复上传静默忽略 |

**命令返回成功 ≠ 交付完成。** 在 `todo.complete` 的 `result` 里写清楚你上传了哪些文件、各自带了什么 outputName，
用户或 PM 发现对不上时会据此让你重传。

### todo.fail — 报告失败

Todo 执行失败时发送。

```bash
TARGET_NODE="trustmesh-server"  # ← 替换为实际 from 值
SESSION_KEY="task_123"         # ← 替换为 incoming header 中 session 的值

payload="$(jq -nc \
  --arg task_id "task_123" \
  --arg todo_id "todo_1" \
  --arg error "Google OAuth 凭证缺失，无法完成 OAuth 接入" \
  '{
    task_id: $task_id,
    todo_id: $todo_id,
    error: $error
  }')"

clawsynapse publish \
  --target "$TARGET_NODE" \
  --type todo.fail \
  --session-key "$SESSION_KEY" \
  --message "$payload"
```

### task.comment — 发送评论

把 `task.comment` 当作默认工作日志来发送。它不是“整理好的说明文”，而是执行过程中的草稿、观察和笔记。短句也可以，不需要等全部想清楚才发。评论不影响 Todo 状态，但它是任务可观测性的主要来源。

**发送原则**

- 开工就发，不要等到第一个里程碑再发
- 每个明显步骤都发 1 条：分析、命令、修改、实验、判断、阻塞、回退、下一步
- 如果不确定这条信息值不值得发，默认发
- 可以简短，不要求完整，不要求润色
- `todo.progress` 说“到哪个阶段了”，`task.comment` 说“刚刚做了什么、为什么这么做、接下来做什么”

**必发时机**

- 收到 `todo.assigned` 并开始处理时
- 阅读相关代码、文档、接口后，形成第一轮判断时
- 每次执行关键命令、完成关键修改、完成一次验证后
- 方案变化、做出关键决策、发现风险时
- 遇到阻塞、需要假设前进、等待外部条件时
- 发送 `todo.complete` 或 `todo.fail` 之前

**建议频率**

- 短任务：至少 2 条 comment（开工 1 条，结束前总结 1 条）
- 中等任务：至少 4 条 comment（开工、分析、执行、结束前总结）
- 长任务：每 5 到 10 分钟至少 1 条，或每个明显步骤 1 条

**推荐内容结构**

- 刚检查了什么
- 观察到了什么
- 决定怎么做
- 刚执行了什么动作
- 结果如何
- 下一步是什么

**可直接套用的简短模板**

- `已开始处理，先检查 <模块/文件>，确认现状后再决定改法。`
- `检查了 <文件/接口>，发现 <现象>，判断应优先复用 <现有实现>。`
- `刚执行了 <命令/操作>，结果是 <结果>，接下来处理 <下一步>。`
- `发现风险：<风险>。当前决定先按 <方案> 推进，并补充验证。`
- `遇到阻塞：<问题>。当前假设是 <假设>，下一步用 <方法> 验证。`
- `结束前记录：已完成 <内容>，剩余关注点是 <风险/边界>。`

**推荐做法：先定义 helper，后续反复调用**

```bash
TARGET_NODE="trustmesh-server"  # ← 替换为实际 from 值
SESSION_KEY="task_123"          # ← 替换为 incoming header 中 session 的值
TASK_ID="task_123"
TODO_ID="todo_1"

send_comment() {
  local content="$1"
  local payload
  payload="$(jq -nc \
    --arg task_id "$TASK_ID" \
    --arg todo_id "$TODO_ID" \
    --arg content "$content" \
    '{
      task_id: $task_id,
      todo_id: $todo_id,
      content: $content
    }')"

  clawsynapse publish \
    --target "$TARGET_NODE" \
    --type task.comment \
    --session-key "$SESSION_KEY" \
    --message "$payload"
}
```

示例：

```bash
send_comment "已开始处理，先检查 auth 模块和登录相关 handler。"
send_comment "检查了现有 auth 模块，发现已有 JWT 签发逻辑，决定复用而不是重写。"
send_comment "刚完成登录 handler 的参数校验，接下来接入 JWT 签发并补错误处理。"
```

```bash
TARGET_NODE="trustmesh-server"  # ← 替换为实际 from 值
SESSION_KEY="task_123"         # ← 替换为 incoming header 中 session 的值

payload="$(jq -nc \
  --arg task_id "task_123" \
  --arg todo_id "todo_1" \
  --arg content "分析了现有的 auth 模块，发现已有 JWT 签发逻辑，决定复用而非重写。接下来实现登录 handler。" \
  '{
    task_id: $task_id,
    todo_id: $todo_id,
    content: $content
  }')"

clawsynapse publish \
  --target "$TARGET_NODE" \
  --type task.comment \
  --session-key "$SESSION_KEY" \
  --message "$payload"
```

字段说明：
- `task_id`：必填，关联的任务 ID
- `todo_id`：可选，关联到具体的 Todo
- `content`：必填，评论内容（支持多行文本）

### 获取结构化发送结果

```bash
clawsynapse --json publish \
  --target "$TARGET_NODE" \
  --type todo.progress \
  --session-key "$SESSION_KEY" \
  --message "$payload"
```

## 四、Guardrails

- **永远不要用你自己的 node ID 作为 `--target`。** target 是 TrustMesh 节点（incoming `from`）。
- **不要发送 `conversation.reply`、`task.create`。** 这些是 PM Agent 的消息类型，不是执行 Agent 的。你只能发送 `todo.progress`、`todo.complete`、`todo.fail`、`task.comment`、`task.context.query` 五种消息类型。
- **Todo 终态不可逆。** 已经 `done` 或 `failed` 的 Todo 不能再更新，服务端会拒绝（`TODO_ALREADY_DONE` / `TODO_ALREADY_FAILED`）。
- **`todo.remind` 不是新任务。** 收到超时提醒只能回报进度（todo.progress / task.comment），禁止重新开始执行、禁止绕过 todo.ask 确认、禁止因此直接 todo.complete。
- **不要把 `task.comment` 当成可选项。** 对执行 Agent 来说，它是默认工作日志；长时间无 comment 视为过程缺失。
- **不要等整理完再发 comment。** comment 可以是草稿、短句、阶段性判断；过度记录优于缺失记录。
- **不要只发 `todo.progress` 不发 `task.comment`。** 里程碑更新前后，应至少有一条相关 comment 解释上下文。
- 业务内容必须通过 `clawsynapse publish` 或 `clawsynapse transfer send` 发送；不要在聊天界面复述 payload、结果摘要或过程说明。
- 在聊天界面中，除发送确认外不要输出任何业务内容。成功发送后只允许输出一行极简确认：`ACK <message_type>`。
- 如果当前是在等待新的任务消息、上下文结果或外部条件，只输出：`WAITING`。
- 如果发送失败或命令报错，只输出：`ERR <reason>` 或 `ERR publish failed: <code>`。
- 不要输出多余解释，不要粘贴 JSON，不要重复总结任务。
- 不要丢弃 `--session-key`。
- 不要发送协议中未定义的字段。
- payload 较大时用 `jq -nc` 构建。

## 五、常见错误码

| 错误码 | 含义 |
|--------|------|
| `BAD_PAYLOAD` | JSON 结构或必填字段无效 |
| `VALIDATION_ERROR` | 业务参数校验失败 |
| `FORBIDDEN` | 无权执行该操作（可能不是该 Todo 的指派 Agent） |
| `NOT_FOUND` | 目标 Task 或 Todo 不存在 |
| `TODO_FINALIZED` | Todo 已结束，不再接受更新 |
| `TODO_ALREADY_DONE` | Todo 已完成 |
| `TODO_ALREADY_FAILED` | Todo 已失败 |
