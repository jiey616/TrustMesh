# QM 开源项目深度学习 × TrustMesh 对比分析

> 调研对象：https://github.com/yc-software/qm（Multiplayer agent harness for work，MIT 开源，67 commits，2026-07-29 历史重建后活跃迭代）

---

## 一、QM 是什么

**一句话**：把编码 agent 的能力（沙箱执行、工具调用）产品化为一个**组织级、多租户、带作用域隔离与治理能力的协作平台**——以 Slack 和 Web 为前端、Postgres 为持久层、可插拔 harness/模型为引擎，通过 CLI + deployment 仓库的模式交付到客户自有云账号。

**README 核心论点**：大多数 agent 被设计成"个人助理"，强行给全公司用会很快变复杂。QM 专为创业公司设计：每位员工拥有**隔离的工作空间**，同时可以在频道、群组消息和项目中协作。

### 1.1 核心架构

```
Postgres (sessions · memory · queue)
     ↕
Headless Core (API · identity · policy · scheduler)
     ↕
Agent loop (Pi, OpenCode, Claude Code 驱动同一核心)
     ↕
Per-scope sandbox (files · tools · logged-in services)
```

### 1.2 核心设计原则

| 原则 | QM 实现 |
|---|---|
| **Scope 隔离** | 每个人、每个房间有**作用域隔离的**记忆、文件、密钥链视图、权限、定时任务、Web 应用和持久沙箱 |
| **厂商中立** | harness 和 model 可替换（Pi / OpenCode / Codex / Claude Code 驱动同一核心） |
| **小而固定工具面** | 一个 `execute` 工具在 scope 自己的隔离沙箱执行命令——沙箱是"持久计算机"，安装的工具永久保留 |
| **插件化表面** | Web UI / admin panel / portal 都是 core HTTP API 之上的**可选插件**；Slack 是可选进程内插件 |
| **core 通用化** | 公司特定的一切（组织配置、自定义工具与技能、沙箱镜像、基础设施）放在 **deployment directory**，由 `qm` CLI 校验和部署 |
| **可替换基座** | 每个底层组件（harness、会话存储、沙箱、记忆）都在接口之后，生产实现通过 **wiring 文件**注入 |

### 1.3 安全模型（三姿态 + 硬约束）

| 姿态 | 行为 |
|---|---|
| **Strict** | 每个 harness 工具调用暂停等待人工批准（两个无副作用回合结束器除外） |
| **Auto**（默认） | 分类器在带来源标注的外部数据和工具结果送达模型前进行筛查 |
| **Dangerous** | 无内容筛查、工具调用间无暂停 |

**硬约束**：预声明命令策略（审批规则 + 硬性拒绝递归删除/破坏性 SQL）**在所有姿态下生效**，包括 Dangerous。

### 1.4 刻意只放门户的 3 个动作（Deliberately portal-only）

| 动作 | 理由 |
|---|---|
| **Admin grant changes** | 管理员授权变更若开放给 agent，被注入的 agent 可自行提权或降权 |
| **Impersonation** | 身份切换会让一次混乱回合变成另一个人的权限 |
| **Command-approval decisions** | 命令审批是人的判断，若 agent 可审批则人机门控坍缩为单一模型决策 |

**共同逻辑**：这些是"授权未来 agent 行为"的决定，**决定本身必须来自 agent 之外**。

### 1.5 其他值得注意的工程实践

- **npm 供应链防护**：`min-release-age=7`（新发布包必须老化 7 天才能进 lockfile），CI 用 `npm ci` 锁定
- **不可变 pin**：sandbox 镜像用 digest pin、deployment layer 用 SHA-256 内容哈希版本化、任务定义 digest-pinned，**不用可变 tag**
- **URL 派生**：`publicUrl` 是唯一公共坐标，CLI 派生 core/Slack/web/admin/portal 全部 URL
- **贡献方式**：只接受 human-written text（`.txt`/`.md` 描述变更），不收代码 PR
- **私有 fork 安全**：明确禁止 GitHub Fork 按钮（fork 无法私有、与上游共享对象网络导致提交可按 SHA 泄漏），用 clone + mirror push
- **Cron 设计**：路径可寻址 `/crons/<id>`、运行历史、每次 run 链接到对应 worklog 会话 `/?session=<id>`
- **Skills 设计**：按 scope 遮蔽（shadowing）、管理员审批后晋升全组织、支持 git 仓库导入、(scopeId, name) 索引单次快照读
- **Delivery 设计**：outbox + 幂等键去重、回退投递路径（避免重复发帖）、withThread/withReact/withDelete 辅助器
- **镜像瘦身**：core 镜像从 4.44 GB → 3.12 GB（清除签名 URL 泄漏、machine-id 固定、关闭遥测）

---

## 二、与 TrustMesh 的共通理念

| 维度 | QM | TrustMesh | 共通点 |
|---|---|---|---|
| **平台定位** | 多人协作 agent harness（组织级协作平台） | AI 多智能体协作平台 | 都是把 agent 能力产品化为**组织级协作平台**，而非个人助理 |
| **中央 core** | Headless core（API·identity·policy·scheduler） | TrustMesh backend（Go+Gin，统一编排） | 中央调度+策略+身份 |
| **可插拔引擎** | Pi/OpenCode/Codex/Claude Code 驱动同一核心 | ClawSynapse 网格连接多个 Agent 节点（hermes/webhook） | 引擎/节点可插拔 |
| **作用域隔离** | scope 隔离（记忆/文件/密钥/权限） | agent 隔离（nodeId/信任/能力） | 细粒度数据与权限隔离 |
| **技能系统** | skills 按 scope 遮蔽+共享+审批晋升 | capability skill + managed dir + external_dirs | 技能管理+运行时加载 |
| **定时任务** | crons（路径可寻址+运行历史+worklog） | capability cron + executions 执行记录 | 定时任务+执行记录 |
| **消息投递** | delivery outbox+幂等+回退 | webhook + ACK 过滤 + 会议广播 | 消息投递+回执管理 |
| **记忆系统** | memory 按 scope 隔离+历史+恢复 | 项目文件/会议纪要/任务记录 | 持久化记忆与上下文 |
| **安全姿态** | Strict/Auto/Dangerous 三级 | trust 信任模式（open/tofu/explicit） | 分级安全策略 |
| **文件沙箱** | per-scope sandbox 持久计算机 | hermes 节点 skills/cron 输出目录 | 沙箱/执行环境 |
| **CLI 部署** | `qm` CLI + deployment directory 契约 | deploy 脚本（paramiko）+ compose 文件 | 部署自动化 |

**核心理念高度重合**：两者都认为"agent 应该作为组织级基础设施"，而非个人助理；都强调**中央编排 + 节点隔离 + 可插拔引擎 + 技能/任务/记忆三大支柱**。

---

## 三、TrustMesh 可借鉴的点（按价值/可行性排序）

### 🔴 P0：立即能落地的高价值改进

| # | 借鉴点 | QM 做法 | TrustMesh 现状 | 落地建议 |
|---|---|---|---|---|
| 1 | **capability.set 超时分级** | 查询 5s / 写回 30s（gateway 重启耗时） | 已完成（2026-08-05 修复） | ✅ 已借鉴完成 |
| 2 | **Cron 路径可寻址 + worklog 深链** | `/crons/<id>` 深链接、每次 run 链接到 worklog 会话 `/?session=<id>` | executions 只有列表+弹窗，无深链接 | 前端加 `/agents/<id>/crons/<jobId>` 路由 + 每次执行链接到对应会话页 |
| 3 | **不可变 pin 替代可变 tag** | sandbox 镜像 digest pin、deployment layer SHA-256 版本化 | 镜像用可变 tag（test/latest），部署漂移 | 部署时记录镜像 digest，回滚用 digest 而非 tag；compose 用 `image@sha256:...` |
| 4 | **deployment directory 契约** | 可提交可移植目录 + `qm` CLI 校验（contract v1） | 手工 compose + .env，无统一契约 | 定义 `trustmesh-deploy/` 目录契约（compose 文件 + .env 模板 + 技能包 + 版本锁定 + 校验脚本） |
| 5 | **URL 派生（单一 publicUrl）** | publicUrl 派生所有服务 URL | URL 分散在多个环境变量（EXTERNAL_URL/API_URL/WEBHOOK_URL） | 统一为单一 `PUBLIC_URL`，其他 URL 由 CLI/entrypoint 派生 |

### 🟡 P1：中等价值，需要设计讨论

| # | 借鉴点 | QM 做法 | TrustMesh 现状 | 落地建议 |
|---|---|---|---|---|
| 6 | **scope 模型（比 agent 更细粒度）** | scope = 人 + 房间，隔离记忆/文件/密钥/权限 | 只有 agent 级隔离（nodeId/信任） | 引入 scope 概念：scope = agent + 项目 + 会话，实现更细粒度的数据隔离（项目文件/会议记录/密钥按 scope 隔离） |
| 7 | **命令审批策略（工具级门控）** | tool.json 的 approvals 只能收紧不能放松、硬性拒绝递归删除/破坏性 SQL | capability.set 直接改技能/模型，无门控 | 高危操作（技能部署、模型切换、cron 删除）加命令审批策略（审批规则 + 硬性拒绝危险命令） |
| 8 | **Deliberately portal-only actions** | 3 个高危动作只放门户：管理员授权/身份切换/命令审批 | 无区分"agent 可做"和"只有人能做" | 信任审批、密钥管理、管理员授权限定为只有人类门户能做，不开放 agent self-API |
| 9 | **Delivery outbox + 幂等键** | outbox 用幂等键按 visitor+app+day 去重 | webhook 推送 + ACK 过滤，无幂等键 | 消息投递加幂等键（deliveryId + turnId），防重复发帖 |
| 10 | **Skills 按 scope 遮蔽** | 同名技能按 scope 遮蔽（visibleFor），(scopeId, name) 索引 | 技能只有 enable/disable，无遮蔽 | 按 agent 角色遮蔽：PM 技能 vs Executor 技能，同名不同实现 |
| 11 | **分类器筛查（内容安全）** | Auto 姿态用分类器筛查带来源标注的外部数据 | 无内容筛查（Tirith 是本地扫描，不在消息链路） | 消息送达模型前加分类器筛查（可复用 Tirith 或独立筛查代理） |
| 12 | **审计记录统一化** | 所有安全相关动作记录审计事件 | capability.set 有 logAudit，但不完整 | 统一审计：所有高危操作（信任变更、密钥操作、技能部署、cron 变更）记录审计事件到独立表 |

### 🟢 P2：长期价值，架构级参考

| # | 借鉴点 | QM 做法 | TrustMesh 现状 | 落地建议 |
|---|---|---|---|---|
| 13 | **Wiring 文件注入** | wiring.ts 统一依赖组装/注入入口 | 依赖注入分散在 main.go 和各 handler | 引入 wiring.go 统一依赖组装（自定义 provider 接入统一解析 choke point） |
| 14 | **Deployment layer（部署层）** | deployment layer = 技能树 + 描述符 + SHA-256 内容哈希，up 同步到 core 表 | 技能/配置直接写 hermes config.yaml，无部署层概念 | 引入 deployment layer：技能包 + 描述符 + 内容哈希，部署时同步到 core，core 按哈希版本化 |
| 15 | **CLI 门控顺序** | check → doctor → build → plan → up → check --live | 手动 docker compose build + up，无门控 | 部署脚本加门控：check 静态检查 → doctor 外部检查 → plan 渲染 → up 执行 → check --live 活漂移检查 |
| 16 | **npm 供应链防护** | min-release-age=7（新包老化 7 天） | 无供应链防护 | frontend 的 .npmrc 加 `min-release-age=7`，CI 用 `npm ci` 锁定 |
| 17 | **贡献方式（human-written text）** | 只收 `.txt`/`.md` 变更描述，不收代码 PR | 无此机制 | 可选：docs/adrs/ 目录存放变更描述和设计理念，与代码分离 |
| 18 | **私有 fork 安全实践** | 禁止 GitHub Fork 按钮（泄漏风险），用 clone + mirror push | 无 fork 机制 | 若需 fork，用 clone + mirror push，禁止 GitHub Fork 按钮 |
| 19 | **镜像瘦身** | core 4.44 GB → 3.12 GB（清除签名 URL 泄漏、machine-id 固定、关闭遥测） | backend 36MB / frontend 49.8MB（已很轻） | 可借鉴其"清除签名 URL 泄漏"思路（检查镜像是否含敏感元数据） |
| 20 | **会话 fork 原点持久化** | sessions 支持 fork 原点持久化 | 无会话 fork 机制 | 可选：会话支持 fork（分支点记录），便于多分支对话 |

### 🔵 P3：理念级参考（不一定实现，但值得理解）

| # | 借鉴点 | QM 做法 | TrustMesh 可思考的点 |
|---|---|---|---|
| 21 | **Egress 强制（出站代理）** | 出站流量强制走 egress proxy（VALIDATED-ONLY，未完全强制） | 目前无出站代理，可考虑敏感数据出站管控 |
| 22 | **Keychain 视图** | 每个 scope 自己的密钥链视图，凭据以 acting-as 身份行事 | 目前密钥在 .env 文件，无按 scope 隔离的密钥链 |
| 23 | **可替换沙箱后端** | sandbox.backend 可替换（sprites/aws） | hermes 节点是固定沙箱，可考虑可替换后端（本地/云/微 VM） |
| 24 | **Web apps（快速内部应用）** | 快速搭建内部定制应用并发布给指定人员 | 目前无此能力，可考虑内部应用市场 |
| 25 | **Onboarding（新用户引导）** | onboarding 模块 | 目前无引导，可考虑新用户/新 agent 引导流程 |

---

## 四、QM 特别值得学习的工程哲学

1. **"scope 隔离"是产品级设计，不是技术细节**：QM 把"每个人的记忆/文件/密钥/权限都隔离"作为核心卖点，不是可选配置。TrustMesh 目前 agent 级隔离比较粗，scope 模型（agent + 项目 + 会话）能支撑更细粒度场景（比如同一 agent 在不同项目里数据完全隔离）。

2. **"可替换"是架构红线**：harness/model/sandbox/memory 全部在接口之后，生产实现通过 wiring 注入。TrustMesh 的 ClawSynapse 网格已经有这个雏形（webhook/hermes 适配器），但 model/沙箱还没有接口抽象。

3. **"不可变"是部署红线**：镜像 digest pin、deployment layer SHA-256、任务定义 digest-pinned——**一切可变 tag 都被视为漂移风险**。TrustMesh 目前 test/latest tag 每次部署都可能漂移，digest pin 是最低成本的安全提升。

4. **"门控"是安全红线**：Strict/Auto/Dangerous 三姿态 + 硬性拒绝命令在所有姿态下生效 + 3 个动作刻意只放门户——**安全不是可选配置，是产品形态**。TrustMesh 目前 capability.set 无门控，高危操作（技能部署/模型切换）应该加审批策略。

5. **"审计"是合规红线**：所有安全相关动作记录审计事件。TrustMesh 目前审计不完整（只有 capability.set 的 logAudit），统一审计是合规和故障排查的基础。

6. **"契约"是交付红线**：deployment directory contract v1 + qm CLI 校验 + contract 主版本号 fail closed——**部署是可提交可移植的目录，不是手工操作**。TrustMesh 目前部署是手工 compose + .env，deployment directory 契约是最值得借鉴的落地形态。

---

## 五、建议的借鉴路线

| 阶段 | 动作 | 预期收益 |
|---|---|---|
| **阶段 0（本周）** | ① 镜像 digest pin 替代可变 tag<br>② 前端 cron 路径可寻址 + worklog 深链<br>③ 统一 publicUrl 派生 | 部署漂移减少 90%，cron 体验提升，环境变量简化 |
| **阶段 1（两周内）** | ④ deployment directory 契约（trustmesh-deploy/）<br>⑤ capability.set 高危操作加命令审批策略<br>⑥ delivery 幂等键去重 | 部署可提交可移植，高危操作有门控，消息不重复 |
| **阶段 2（一个月内）** | ⑦ scope 模型（agent + 项目 + 会话）<br>⑧ skills 按 scope 遮蔽<br>⑨ 统一审计记录 | 细粒度数据隔离，技能分角色，合规可审计 |
| **阶段 3（按需）** | ⑩ wiring 文件注入<br>⑪ deployment layer 概念<br>⑫ CLI 门控顺序 | 架构可插拔，部署层版本化，部署门控化 |

---

*调研完成：2026-08-05 | QM 仓库：yc-software/qm | TrustMesh：本地开发中*
