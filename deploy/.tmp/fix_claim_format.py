# -*- coding: utf-8 -*-
"""修复 store_planning.go 中 ClaimPlanRejectNotify 的格式（471-501 行整体重写）。"""
import sys

p = r"D:/AIWorkspace/TrustMesh/backend/internal/store/store_planning.go"
d = open(p, "rb").read().decode("utf-8")
nl = "\r\n" if "\r\n" in d else "\n"
lines = d.split(nl)

# 边界自检：471 行应为注释开头，502 行应为 GetTaskPMPublishTarget
if "ClaimPlanRejectNotify" not in lines[470] or "GetTaskPMPublishTarget" not in lines[501]:
    print("[FAIL] 边界不符，终止"); print(repr(lines[470])); print(repr(lines[501])); sys.exit(1)

clean = [
    "// ClaimPlanRejectNotify 对「任务 + 规划被拒指纹」做进程内节流：同一指纹自动催 PM",
    "// 的次数未超过 max 时，计数 +1 并返回 true（允许推送）；否则返回 false。",
    "// fingerprint 用 mismatch 文本本身（同一原因重提只催有限次，换原因则重新计）。",
    "func (s *Store) ClaimPlanRejectNotify(taskID, fingerprint string, max int) bool {",
    "\ts.mu.Lock()",
    "\tdefer s.mu.Unlock()",
    "\tif s.planRejectNotify == nil {",
    "\t\ts.planRejectNotify = make(map[string]int)",
    "\t}",
    "\tkey := taskID + \"\\x00\" + fingerprint",
    "\tif s.planRejectNotify[key] >= max {",
    "\t\treturn false",
    "\t}",
    "\ts.planRejectNotify[key]++",
    "\treturn true",
    "}",
    "",
]
lines[470:501] = clean
open(p, "wb").write(nl.join(lines).encode("utf-8"))
print("[OK] ClaimPlanRejectNotify 已重写为规范格式, nl=", repr(nl))
