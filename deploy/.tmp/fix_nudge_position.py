# -*- coding: utf-8 -*-
"""修复 webhook.go：NudgePlanningPM 误插进 RemindTodo 函数体，搬到其真正结尾后。"""
import sys

p = r"D:/AIWorkspace/TrustMesh/backend/internal/clawsynapse/webhook.go"
d = open(p, "rb").read().decode("utf-8")
nl = "\r\n" if "\r\n" in d else "\n"
lines = d.split(nl)


def idx_of(pred, label):
    hits = [i for i, l in enumerate(lines) if pred(l)]
    if len(hits) != 1:
        print(f"[FAIL] {label} hits={len(hits)}")
        sys.exit(1)
    return hits[0]


# 1) 定位误插块：从 RemindTodo 后第一个空行开始（1454，0-based 1453）到方法闭合 }
i_func = idx_of(lambda l: l.startswith("func (h *WebhookHandler) RemindTodo"), "RemindTodo")
i_start = idx_of(lambda l: l.startswith("// NudgePlanningPM is the planning-stall hook"), "误插块注释首行") - 1  # 前面的空行
i_method_close = idx_of(lambda l: l == "}" and lines[l and 0 or 0] == "}" and False or (l == "}"), "占位") if False else None
# 方法闭合 } 是误插块中 strip=="}" 且下一行以 \ttask := 开头的那一行
for x in range(i_start, min(i_start + 60, len(lines))):
    if lines[x] == "}" and x + 1 < len(lines) and lines[x + 1].strip().startswith("task := h.store.GetTaskInternal"):
        i_end = x
        break
else:
    print("[FAIL] 未找到误插块闭合 }")
    sys.exit(1)
block = lines[i_start:i_end + 1]
if not any("func (h *WebhookHandler) NudgePlanningPM" in l for l in block):
    print("[FAIL] 块内容不符")
    sys.exit(1)
del lines[i_start:i_end + 1]
print(f"[OK] 已摘除误插块 lines {i_start+1}-{i_end+1} ({len(block)} 行)")

# 2) 定位 RemindTodo 真正结尾：timeout_remind Publish 行 + 5 行
i_pub = idx_of(lambda l: '"timeout_remind"' in l, "RemindTodo Publish 行")
# A=pub, B=if log, C=warn, D=}, E=}, F=}（函数闭合）
if lines[i_pub + 5].strip() != "}" or lines[i_pub + 4].strip() != "}" or lines[i_pub + 3].strip() != "}":
    print("[FAIL] RemindTodo 结尾结构不符")
    for x in range(i_pub, i_pub + 7):
        print(x + 1, repr(lines[x]))
    sys.exit(1)
lines[i_pub + 6:i_pub + 6] = block
print(f"[OK] 块已插入 RemindTodo 结尾后 @line {i_pub+7}")
open(p, "wb").write(nl.join(lines).encode("utf-8"))
print("DONE")
