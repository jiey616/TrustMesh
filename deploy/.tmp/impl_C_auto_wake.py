# -*- coding: utf-8 -*-
"""C 方案：422 WORKFLOW_MISMATCH 后平台主动推 task.message 唤醒 PM（含节流）。"""
import io, sys

ROOT = r"D:/AIWorkspace/TrustMesh/backend"
T = "    "


def load(rel):
    p = f"{ROOT}/{rel}"
    d = open(p, "rb").read().decode("utf-8")
    nl = "\r\n" if "\r\n" in d else "\n"
    lines = d.split(nl)
    return p, lines, nl


def save(p, lines, nl):
    open(p, "wb").write(nl.join(lines).encode("utf-8"))


def find(lines, pred, label, must=True):
    hits = [i for i, l in enumerate(lines) if pred(l)]
    if not hits:
        print(f"[FAIL] {label}")
        if must:
            sys.exit(1)
        return None
    print(f"[OK] {label} @line {hits[0]+1} (hits={len(hits)})")
    return hits[0]


fails = []

# ---------- 1) store.go：struct 字段 + New() 初始化 ----------
p, lines, nl = load("internal/store/store.go")
i = find(lines, lambda l: l.strip().startswith("agentChatBySession map[string]string"), "1a. struct 字段锚点")
if i is not None:
    lines.insert(i + 1, "\t// planRejectNotify 记录「任务+规划被拒指纹」已自动催 PM 的次数（进程内节流）。")
    lines.insert(i + 2, "\tplanRejectNotify map[string]int")
    print("[OK] 1a. struct 字段已插入")
j = find(lines, lambda l: l.strip().startswith("agentChatBySession: make(map[string]string),"), "1b. New() 初始化锚点")
if j is not None:
    lines.insert(j + 1, "\t\tplanRejectNotify:   make(map[string]int),")
    print("[OK] 1b. New() 已初始化")
save(p, lines, nl)

# ---------- 2) store_planning.go：ClaimPlanRejectNotify 方法 ----------
p, lines, nl = load("internal/store/store_planning.go")
i = find(lines, lambda l: "func (s *Store) GetTaskPMPublishTarget" in l, "2a. 方法插入锚点")
if i is not None:
    method = [
        "\t// ClaimPlanRejectNotify 对「任务 + 规划被拒指纹」做进程内节流：同一指纹自动催 PM",
        "\t// 的次数未超过 max 时，计数 +1 并返回 true（允许推送）；否则返回 false。",
        "\t// fingerprint 用 mismatch 文本本身（同一原因重提只催有限次，换原因则重新计）。",
        "\tfunc (s *Store) ClaimPlanRejectNotify(taskID, fingerprint string, max int) bool {",
        "\t\ts.mu.Lock()",
        "\t\tdefer s.mu.Unlock()",
        "\t\tif s.planRejectNotify == nil {",
        "\t\t\ts.planRejectNotify = make(map[string]int)",
        "\t\t}",
        "\t\tkey := taskID + \"\\x00\" + fingerprint",
        "\t\tif s.planRejectNotify[key] >= max {",
        "\t\t\treturn false",
        "\t\t}",
        "\t\ts.planRejectNotify[key]++",
        "\t\treturn true",
        "\t}",
    ]
    # 插到 GetTaskPMPublishTarget 前一个空行处
    k = i
    while k > 0 and lines[k - 1].strip() == "":
        k -= 1
    lines[k:k] = [l for pair in zip(method, [""] * len(method)) for l in pair][:-1]
    print("[OK] 2. ClaimPlanRejectNotify 已插入")
    save(p, lines, nl)

# ---------- 3) webhook.go：const + 方法 + 调用点 ----------
p, lines, nl = load("internal/clawsynapse/webhook.go")

# 3a. 调用点：在 transport.WriteError(c, transport.Validation(...WORKFLOW_MISMATCH...)) 之前
i = find(lines, lambda l: "if mismatch := validatePlanAgainstWorkflowWithScope" in l, "3a-1. 校验调用行")
call_anchor = find(lines, lambda l: l.strip().startswith('transport.WriteError(c, transport.Validation("规划不符合项目工作流'), "3a-2. WriteError 锚点", must=False)
if call_anchor is not None:
    call = [
        "\t\t// 422 回执在 ClawSynapse 侧不会触发 PM 的新一轮 LLM 推理（它只回一句 ACK 就停在",
        "\t\t// WAITING），任务会永久卡在 planning。这里平台主动推一条 task.message 把修正指令",
        "\t\t// 送进 PM 会话，让它自行重发 plan_ready（用户消息可唤醒 PM，已实证）。",
        "\t\th.notifyPMPlanRejected(c, taskWithWF, mismatch, string(submittedJSON))",
        "",
    ]
    lines[call_anchor:call_anchor] = call
    print("[OK] 3a. 调用点已插入")

# 3b. 方法体：插在 handleTaskPlanReady 函数结束之后（即下一个 func 之前）
i = find(lines, lambda l: l.startswith("func (h *WebhookHandler) handleTodoProgress"), "3b-1. handleTodoProgress 锚点")
if i is not None:
    method = [
        "// planRejectNotifyLimit 是同一个任务、同一个 mismatch 指纹最多自动催 PM 的次数，",
        "// 防止 PM 反复提交同一份错误规划形成推送风暴。",
        "const planRejectNotifyLimit = 2",
        "",
        "// notifyPMPlanRejected 在规划校验失败后主动给 PM 推一条修正指令。",
        "//",
        "// 背景：task.plan_ready 的 422 在 ClawSynapse 侧以 task.error 形式回投，不会触发",
        "// PM 的新一轮 LLM 推理 —— PM 停在 WAITING，任务永久卡在 planning 且 todos 为 0。",
        "// 而 task.message（用户消息通道）能唤醒 PM，因此由平台代发修正指令。",
        "// 节流：同一 (task, mismatch 指纹) 最多自动催 planRejectNotifyLimit 次。",
        "func (h *WebhookHandler) notifyPMPlanRejected(c *gin.Context, task *model.TaskDetail, mismatch, submittedTodos string) {",
        "\tif h.client == nil || task == nil {",
        "\t\treturn",
        "\t}",
        "\tif !h.store.ClaimPlanRejectNotify(task.ID, mismatch, planRejectNotifyLimit) {",
        "\t\tif h.log != nil {",
        '\t\t\th.log.Warn("plan-reject auto-notify throttled", zap.String("task_id", task.ID))',
        "\t\t}",
        "\t\treturn",
        "\t}",
        "\tpmNodeID, appErr := h.store.GetTaskPMPublishTarget(task.UserID, task.ID)",
        "\tif appErr != nil {",
        "\t\tif h.log != nil {",
        '\t\t\th.log.Warn("skip plan-reject notify", zap.String("task_id", task.ID), zap.String("code", appErr.Code))',
        "\t\t}",
        "\t\treturn",
        "\t}",
        "\tinstruction := fmt.Sprintf(",
        '\t\t"你刚才提交的 task.plan_ready 被平台校验拒绝，规划未生效，任务仍停留在 planning（todo 数为 0）。\\n\\n"+',
        '\t\t"拒绝原因：%s\\n\\n"+',
        '\t\t"你提交的 todos：%s\\n\\n"+',
        '\t\t"请按拒绝原因修正后，立即重新发送 task.plan_ready（不要回到澄清流程，需求已经确认过）。\\n"+',
        '\t\t"若缺少的步骤是用户明确表示不需要的尾部步骤，请在 task.plan_ready 中声明 deliver_scope.up_to_step 为你打算止步的那一步（该步骤名必须真实存在于工作流中）。",',
        "\t\tmismatch, submittedTodos)",
        "\tpayload := protocol.PMTaskMessage{",
        '\t\tSchemaVersion: "1.0",',
        "\t\tTaskID:        task.ID,",
        "\t\tProjectID:     task.ProjectID,",
        "\t\tContent:       instruction,",
        "\t\tUserContent:   instruction,",
        "\t\tIsInitial:     false,",
        "\t\tWorkflow:      task.Workflow,",
        "\t}",
        "\tif _, err := h.client.Publish(c.Request.Context(), pmNodeID, \"task.message\", payload, task.ID, nil); err != nil {",
        "\t\tif h.log != nil {",
        '\t\t\th.log.Warn("plan-reject notify publish failed", zap.String("task_id", task.ID), zap.Error(err))',
        "\t\t}",
        "\t\treturn",
        "\t}",
        "\tif h.log != nil {",
        '\t\th.log.Info("plan-reject auto-notify sent", zap.String("task_id", task.ID), zap.String("pm_node", pmNodeID))',
        "\t}",
        "}",
        "",
    ]
    lines[i:i] = method
    print("[OK] 3b. 方法已插入")
save(p, lines, nl)

print("\nDONE C")
