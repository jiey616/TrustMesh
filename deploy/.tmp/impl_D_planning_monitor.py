# -*- coding: utf-8 -*-
"""D 方案：planning 阶段兜底监控（停滞检测 + 自动催 PM + 待人工标记）。"""
import sys

ROOT = r"D:/AIWorkspace/TrustMesh/backend"
T = "\t"


def load(rel):
    p = f"{ROOT}/{rel}"
    d = open(p, "rb").read().decode("utf-8")
    nl = "\r\n" if "\r\n" in d else "\n"
    return p, d.split(nl), nl


def save(p, lines, nl):
    open(p, "wb").write(nl.join(lines).encode("utf-8"))


def find(lines, pred, label):
    hits = [i for i, l in enumerate(lines) if pred(l)]
    if len(hits) != 1:
        print(f"[FAIL] {label} hits={len(hits)}")
        sys.exit(1)
    print(f"[OK] {label} @line {hits[0]+1}")
    return hits[0]


# ---------- 1) store.go：钩子 + 计数字段 + New() 初始化 + Set 方法 ----------
p, lines, nl = load("internal/store/store.go")
i = find(lines, lambda l: l.strip().startswith("planRejectNotify map[string]int"), "1a. 字段锚点")
lines[i + 1:i + 1] = [
    "\t// planningStallHook is set by the app layer to nudge the PM when a task",
    "\t// has been stuck in planning (no finalized plan, PM owes a response).",
    "\t// Runs outside the store lock.",
    "\tplanningStallHook func(ctx context.Context, taskID string)",
    "\t// planningStallCount / planningStallLastRemind are in-memory per-task",
    "\t// throttle state for the planning-stall monitor.",
    "\tplanningStallCount      map[string]int",
    "\tplanningStallLastRemind map[string]time.Time",
]
j = find(lines, lambda l: l.strip().startswith("planRejectNotify:   make(map[string]int),"), "1b. New() 锚点")
lines[j + 1:j + 1] = [
    "\t\tplanningStallCount:      make(map[string]int),",
    "\t\tplanningStallLastRemind: make(map[string]time.Time),",
]
k = find(lines, lambda l: l.startswith("func (s *Store) SetRemindHook"), "1c. SetRemindHook 锚点")
# 找该函数结束（下一个 ^} ）
e = next(x for x in range(k, len(lines)) if lines[x].strip() == "}")
setter = [
    "",
    "// SetPlanningStallHook registers a callback used to nudge the PM when a task",
    "// has been stuck in planning without a finalized plan. The hook runs outside",
    "// the store lock.",
    "func (s *Store) SetPlanningStallHook(hook func(ctx context.Context, taskID string)) {",
    "\ts.planningStallHook = hook",
    "}",
]
lines[e + 1:e + 1] = setter
save(p, lines, nl)
print("[OK] store.go 完成")

# ---------- 2) timeout_monitor.go：常量 + checkPlanningTimeouts + ticker 调用 ----------
p, lines, nl = load("internal/store/timeout_monitor.go")
# 2a. import 加 fmt
i = find(lines, lambda l: l.strip() == '"context"', "2a. import context")
lines.insert(i, '\t"fmt"')
# 2b. 常量
i = find(lines, lambda l: l.strip().startswith("defaultMaxReworks"), "2b. 常量锚点")
lines[i + 1:i + 1] = [
    "",
    "\tplanningStallTimeout = 15 * time.Minute // planning 无进展（PM 欠回复）判定",
    "\tplanningMaxReminders = 2                // planning 停滞最多自动催 PM 次数",
]
# 2c. ticker 调用
i = find(lines, lambda l: l.strip() == "s.checkMeetingTimeouts()", "2c. ticker 调用锚点")
lines.insert(i + 1, "\t\t\ts.checkPlanningTimeouts()")
# 2d. 新函数：插在 checkMeetingTimeouts 注释块之前
i = find(lines, lambda l: l.startswith("// checkMeetingTimeouts force-concludes"), "2d. checkMeetingTimeouts 锚点")
fn = [
    "// checkPlanningTimeouts scans tasks stuck in planning with no finalized plan",
    "// (todos == 0) where the PM owes a response — i.e. the LAST task message is a",
    "// user message (or there are no messages at all). A task whose last message",
    "// came from the PM is a clarification questionnaire waiting for the USER,",
    "// which is normal idle and must NOT be nudged. Stalled tasks get up to",
    "// planningMaxReminders automatic nudges (via planningStallHook, one per",
    "// defaultRemindInterval); after the last one a system comment flags the task",
    "// for human attention. We deliberately do NOT auto-fail planning tasks: a",
    "// failed task here would most likely just be recreated by the user.",
    "func (s *Store) checkPlanningTimeouts() {",
    "\tnow := time.Now().UTC()",
    "\tcutoff := now.Add(-planningStallTimeout)",
    "",
    "\tvar nudges []string",
    "\tvar flagged []string",
    "",
    "\ts.mu.Lock()",
    "\t// Lazy cleanup: drop throttle state for tasks that left planning.",
    "\tfor id := range s.planningStallCount {",
    "\t\tt, ok := s.tasks[id]",
    "\t\tif !ok || t.Status != \"planning\" {",
    "\t\t\tdelete(s.planningStallCount, id)",
    "\t\t\tdelete(s.planningStallLastRemind, id)",
    "\t\t}",
    "\t}",
    "\tfor _, task := range s.tasks {",
    "\t\tif task.Status != \"planning\" || len(task.Todos) > 0 {",
    "\t\t\tcontinue",
    "\t\t}",
    "\t\tlast := task.UpdatedAt",
    "\t\tif last.IsZero() {",
    "\t\t\tlast = task.CreatedAt",
    "\t\t}",
    "\t\tif last.After(cutoff) {",
    "\t\t\tcontinue // recent activity, not stalled",
    "\t\t}",
    "\t\t// Trap guard: PM's last message is a reply/question → waiting for the",
    "\t\t// user to answer. That is normal idle; nudging the PM here would just",
    "\t\t// make it re-ask the same questionnaire.",
    "\t\tif n := len(task.Messages); n > 0 && task.Messages[n-1].Role == \"pm_agent\" {",
    "\t\t\tcontinue",
    "\t\t}",
    "\t\tcount := s.planningStallCount[task.ID]",
    "\t\tif count >= planningMaxReminders {",
    "\t\t\tcontinue // already nudged max times and flagged",
    "\t\t}",
    "\t\tif lr, seen := s.planningStallLastRemind[task.ID]; seen && lr.Add(defaultRemindInterval).After(now) {",
    "\t\t\tcontinue // remind interval not yet elapsed",
    "\t\t}",
    "\t\ts.planningStallCount[task.ID] = count + 1",
    "\t\ts.planningStallLastRemind[task.ID] = now",
    "\t\tnudges = append(nudges, task.ID)",
    "\t\tif count+1 >= planningMaxReminders {",
    "\t\t\tflagged = append(flagged, task.ID)",
    "\t\t}",
    "\t\tif s.log != nil {",
    "\t\t\ts.log.Warn(\"planning stalled, sending nudge\",",
    "\t\t\t\tzap.String(\"task_id\", task.ID),",
    "\t\t\t\tzap.Int(\"nudge_count\", count+1),",
    "\t\t\t\tzap.Int(\"max_reminders\", planningMaxReminders),",
    "\t\t\t)",
    "\t\t}",
    "\t}",
    "\ts.mu.Unlock()",
    "",
    "\tfor _, id := range nudges {",
    "\t\tif s.planningStallHook != nil {",
    "\t\t\ts.planningStallHook(context.Background(), id)",
    "\t\t}",
    "\t}",
    "\tfor _, id := range flagged {",
    "\t\tcomment := fmt.Sprintf(\"⚠️ 规划阶段停滞：PM 长时间未提交有效规划，平台已自动催办 %d 次仍无响应，已标记待人工介入。\", planningMaxReminders)",
    "\t\tif _, err := s.AppendSystemTaskComment(id, comment); err != nil && s.log != nil {",
    "\t\t\ts.log.Warn(\"append planning-stall comment failed\", zap.String(\"task_id\", id), zap.Error(err))",
    "\t\t}",
    "\t}",
    "}",
    "",
]
lines[i:i] = fn
save(p, lines, nl)
print("[OK] timeout_monitor.go 完成")

# ---------- 3) webhook.go：NudgePlanningPM 方法 ----------
p, lines, nl = load("internal/clawsynapse/webhook.go")
i = find(lines, lambda l: l.startswith("func (h *WebhookHandler) RemindTodo"), "3a. RemindTodo 锚点")
e = next(x for x in range(i, len(lines)) if lines[x].strip() == "}")
method = [
    "",
    "// NudgePlanningPM is the planning-stall hook: the store timeout monitor calls",
    "// it when a task has been stuck in planning (no finalized plan, PM owes a",
    "// response) for planningStallTimeout. It publishes a task.message to the PM",
    "// (the only channel proven to wake the PM into a new LLM turn) so it resumes",
    "// planning.",
    "func (h *WebhookHandler) NudgePlanningPM(ctx context.Context, taskID string) {",
    "\tif h == nil || h.client == nil {",
    "\t\treturn",
    "\t}",
    "\ttask := h.store.GetTaskInternal(taskID)",
    "\tif task == nil {",
    "\t\treturn",
    "\t}",
    "\tpmNodeID, appErr := h.store.GetTaskPMPublishTarget(task.UserID, task.ID)",
    "\tif appErr != nil {",
    "\t\tif h.log != nil {",
    "\t\t\th.log.Warn(\"skip planning-stall nudge\", zap.String(\"task_id\", taskID), zap.String(\"code\", appErr.Code))",
    "\t\t}",
    "\t\treturn",
    "\t}",
    "\tinstruction := \"【规划停滞提醒】这是对既有任务《\" + task.Title + \"》的规划催办，不是新任务指派。任务已较长时间停留在规划阶段且尚未产生任何 Todo。\" +",
    "\t\t\"请检查你上一轮的 task.plan_ready 是否被拒绝或尚未提交：若被拒绝，请按拒绝原因修正后重新发送 task.plan_ready；若尚未提交，请立即提交（需求已经确认过，不要回到澄清流程）。\" +",
    "\t\t\"若用户明确只要交付到某个步骤为止，请在 task.plan_ready 中声明 deliver_scope.up_to_step。多次提醒无响应平台将标记该任务待人工介入。\"",
    "\tpayload := protocol.PMTaskMessage{",
    "\t\tSchemaVersion: \"1.0\",",
    "\t\tTaskID:        task.ID,",
    "\t\tProjectID:     task.ProjectID,",
    "\t\tContent:       instruction,",
    "\t\tUserContent:   instruction,",
    "\t\tIsInitial:     false,",
    "\t}",
    "\tif _, err := h.client.Publish(ctx, pmNodeID, \"task.message\", payload, task.ID, map[string]any{\"source\": \"planning_stall\"}); err != nil {",
    "\t\tif h.log != nil {",
    "\t\t\th.log.Warn(\"planning-stall nudge publish failed\", zap.String(\"task_id\", taskID), zap.String(\"target_node\", pmNodeID), zap.Error(err))",
    "\t\t}",
    "\t}",
    "}",
]
lines[e + 1:e + 1] = method
save(p, lines, nl)
print("[OK] webhook.go 完成")

# ---------- 4) router.go：装配钩子 ----------
p, lines, nl = load("internal/app/router.go")
i = find(lines, lambda l: l.strip().startswith("s.SetRemindHook(webhookHandler.RemindTodo)"), "4a. SetRemindHook 锚点")
lines[i + 1:i + 1] = [
    "\t// Planning-stall nudges wake the PM via task.message when a task has been",
    "\t// stuck in planning without a finalized plan.",
    "\ts.SetPlanningStallHook(webhookHandler.NudgePlanningPM)",
]
save(p, lines, nl)
print("[OK] router.go 完成")
print("\nDONE D")
