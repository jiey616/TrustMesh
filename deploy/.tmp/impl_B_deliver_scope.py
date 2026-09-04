#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""B. 交付范围裁剪（deliver_scope）—— 自适应换行符版

踩坑记录（重要）：
1. grep -c $'\\r$' 检测 CRLF **不可靠**（返回了完全错误的结果）。
   正确做法：Python 读 bytes，比较 count(b'\\r\\n') 与 count(b'\\n')。
2. backend 换行符**不统一**：protocol/webhook.go 是纯 LF，store_planning.go 是纯 CRLF。
   故每个文件独立检测主导换行符并先规范化，再操作。
3. bytes 字面量不能含非 ASCII -> 用 str + encode('utf-8')。
4. 整块精确匹配对不齐 gofmt 对齐空格会静默失败 -> 用特征子串/strip 定位。

改动：
1. protocol/clawsynapse.go  TaskPlanReadyPayload 加 DeliverScope + 新增 PlanDeliverScope
2. model/task.go            TaskDetail 加 DeliverScope
3. clawsynapse/webhook.go   调用点改 WithScope + 转换函数 + 透传落库
（store/store_planning.go、plan_validation.go 已改好，本脚本仅校验存在性）
"""
import sys

ROOT = r"D:/AIWorkspace/TrustMesh/backend"
T = "\t"


def load(path):
    with open(path, "rb") as f:
        return f.read()


def save(path, data):
    with open(path, "wb") as f:
        f.write(data)


def detect_nl(data):
    """返回该文件的主导换行符：CRLF 仅在「所有 \\n 都是 \\r\\n」时才认定。"""
    total_lf = data.count(b"\n")
    crlf = data.count(b"\r\n")
    if total_lf > 0 and crlf == total_lf:
        return b"\r\n"
    return b"\n"


def normalize(data, nl):
    """统一为 nl（先把所有 CRLF 折成 LF，再按需展开）。"""
    data = data.replace(b"\r\n", b"\n")
    if nl == b"\r\n":
        data = data.replace(b"\n", b"\r\n")
    return data


def enc(lines):
    return [s.encode("utf-8") if isinstance(s, str) else s for s in lines]


def find_once(lines, pred, label):
    hits = [i for i, l in enumerate(lines) if pred(l)]
    if len(hits) != 1:
        print(f"[FAIL] {label}: 命中 {len(hits)} 处（期望 1）")
        return None
    return hits[0]


def already(data, needle, label):
    if needle.encode("utf-8") in data:
        print(f"[SKIP] {label}（已存在）")
        return True
    return False


fails = []


def edit(path, label):
    """载入 -> 检测并规范化换行符 -> 返回 (lines, nl, path)。"""
    data = load(path)
    nl = detect_nl(data)
    data = normalize(data, nl)
    return data.split(nl), nl, data


def commit(path, lines, nl):
    save(path, nl.join(lines))


# ── 1. protocol: TaskPlanReadyPayload ─────────────────────────────────────
p = f"{ROOT}/internal/protocol/clawsynapse.go"
data = load(p)
if already(data, "DeliverScope *PlanDeliverScope", "1. protocol TaskPlanReadyPayload"):
    pass
else:
    lines, nl, data = edit(p, "protocol")
    # 注意：文件里另有结构体也带 `Todos []TaskCreateTodoPayload`，
    # 必须先锁定 TaskPlanReadyPayload 结构体，再在其内部定位 todos 行。
    s = find_once(
        lines,
        lambda l: l.strip().startswith(b"type TaskPlanReadyPayload struct {"),
        "1a. 定位 TaskPlanReadyPayload 结构体",
    )
    if s is None:
        fails.append("1a")
    else:
        e = next((k for k in range(s, len(lines)) if lines[k].strip() == b"}"), None)
        i = next((k for k in range(s, e if e else s) if b"TaskCreateTodoPayload" in lines[k]), None)
        if e is None or i is None:
            fails.append("1b")
        else:
            ins = [
                T + "// DeliverScope 允许 PM 声明本次交付到工作流的哪一步为止（用户只要求部分交付时）。",
                T + "// 为空表示必须覆盖工作流的全部步骤（历史行为）。",
                T + 'DeliverScope *PlanDeliverScope `json:"deliver_scope,omitempty"`',
            ]
            lines[i + 1:i + 1] = enc(ins)
            decl = [
                "",
                "// PlanDeliverScope 声明任务规划的交付范围。",
                "// UpToStep 是工作流中的步骤名（如「资产制作」）：校验时只要求覆盖到该步（含）",
                "// 为止，其后的步骤允许不规划。UpToStep 必须真实存在于工作流中，否则为无效声明。",
                "type PlanDeliverScope struct {",
                T + 'UpToStep string `json:"up_to_step,omitempty"`',
                "}",
            ]
            after_e = e + 1 + len(ins)  # ins 插在 e 之前，e 已后移
            lines[after_e:after_e] = enc(decl)
            commit(p, lines, nl)
            print("[OK]   1. protocol: TaskPlanReadyPayload + PlanDeliverScope")

# ── 2. model: TaskDetail 加 DeliverScope 字段 ─────────────────────────────
p = f"{ROOT}/internal/model/task.go"
data = load(p)
if already(data, "DeliverScope  *TaskDeliverScope", "2. model TaskDetail DeliverScope"):
    pass
else:
    lines, nl, data = edit(p, "model")
    i = find_once(
        lines,
        lambda l: l.strip().startswith(b"WorkflowRef") and b"*WorkflowRef" in l,
        "2. 定位 WorkflowRef 字段行",
    )
    if i is None:
        fails.append("2")
    else:
        ins = [
            T + "// DeliverScope 记录 PM 声明的交付终点步骤（部分交付场景，如「只要到资产图」）。",
            T + "// 为空表示全量交付。仅作记录与审计，不参与派发。",
            T + 'DeliverScope  *TaskDeliverScope  `json:"deliver_scope,omitempty" bson:"deliver_scope,omitempty"`',
        ]
        lines[i + 1:i + 1] = enc(ins)
        commit(p, lines, nl)
        print("[OK]   2. model: TaskDetail 加 DeliverScope 字段（并清理被污染的 CRLF 行）")

# ── 3. webhook.go ─────────────────────────────────────────────────────────
p = f"{ROOT}/internal/clawsynapse/webhook.go"
lines, nl, data = edit(p, "webhook")
changed = False

old_call = b"validatePlanAgainstWorkflow(taskWithWF.Workflow, payload.Todos, h.roleOfAgentNode)"
new_call = (
    b"validatePlanAgainstWorkflowWithScope(taskWithWF.Workflow, payload.Todos, "
    b"h.roleOfAgentNode, deliverScopeFromPayload(payload.DeliverScope))"
)
joined = nl.join(lines)
if b"validatePlanAgainstWorkflowWithScope(taskWithWF.Workflow" in joined:
    print("[SKIP] 3a. webhook 调用点（已改造）")
elif joined.count(old_call) == 1:
    lines = nl.join(lines).replace(old_call, new_call, 1).split(nl)
    changed = True
    print("[OK]   3a. webhook: 调用点改 WithScope")
else:
    print(f"[FAIL] 3a. webhook 调用点：命中 {joined.count(old_call)} 次（期望 1）")
    fails.append("3a")

joined = nl.join(lines)
if b"func deliverScopeFromPayload(" in joined:
    print("[SKIP] 3b. 转换函数（已存在）")
else:
    i = find_once(
        lines,
        lambda l: l.strip().startswith(b"func (h *WebhookHandler) handleTaskPlanReady("),
        "3b. 定位 handleTaskPlanReady",
    )
    if i is None:
        fails.append("3b")
    else:
        fn = [
            "// deliverScopeFromPayload converts the PM-declared delivery scope (wire",
            "// format) into the stored model. Returns nil when the PM declared nothing,",
            '// which keeps the historical "all steps required" behaviour.',
            "func deliverScopeFromPayload(scope *protocol.PlanDeliverScope) *model.TaskDeliverScope {",
            T + "if scope == nil {",
            T + T + "return nil",
            T + "}",
            T + "upTo := strings.TrimSpace(scope.UpToStep)",
            T + 'if upTo == "" {',
            T + T + "return nil",
            T + "}",
            T + "return &model.TaskDeliverScope{UpToStep: upTo}",
            "}",
            "",
        ]
        lines[i:i] = enc(fn)
        changed = True
        print("[OK]   3b. webhook: 新增 deliverScopeFromPayload")

joined = nl.join(lines)
if b"DeliverScope: deliverScopeFromPayload(payload.DeliverScope)," in joined:
    print("[SKIP] 3c. 透传 DeliverScope（已存在）")
else:
    s = find_once(
        lines,
        lambda l: l.strip().startswith(b"in := store.TaskPlanReadyInput{"),
        "3c. 定位 TaskPlanReadyInput 字面量",
    )
    if s is None:
        fails.append("3c")
    else:
        e = next((k for k in range(s + 1, len(lines)) if lines[k].strip() == b"}"), None)
        if e is None:
            fails.append("3c-end")
        else:
            block = [
                T + T + "TaskID:       payload.TaskID,",
                T + T + "Title:        payload.Title,",
                T + T + "Description:  payload.Description,",
                T + T + "Todos:        make([]store.TaskCreateTodoInput, 0, len(payload.Todos)),",
                T + T + "DeliverScope: deliverScopeFromPayload(payload.DeliverScope),",
            ]
            lines[s + 1:e] = enc(block)
            changed = True
            print("[OK]   3c. webhook: 透传 DeliverScope 到落库")

if changed:
    commit(p, lines, nl)

print("")
if fails:
    print("=== 存在失败项: " + ", ".join(fails) + " ===")
    sys.exit(1)
print("=== B 改动完成，待编译验证 ===")
