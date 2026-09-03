"""端到端验证：重复上传静默 + 双发只写一条。

用真实任务 31319dd8419fb49d075acc33（status=done）：
  A) 已入库的同名文件 -> 409，不新增评论
  B) 未入库的新文件名 -> 409，新增 1 条评论
  C) B 的完全重复投递（同 transferId）-> 409，不新增
"""
import json, subprocess, urllib.request, urllib.error

WEBHOOK = "http://localhost:8080/webhook/clawsynapse"
TASK_ID = "31319dd8419fb49d075acc33"
TODO_ID = "TD_01"
PLATFORM_NODE = "n1-cb979d6ad5fb2df289ecb50d3cfc18bc"
FROM_NODE = "n1-0b7fb13b2eafa5c515386fbaecd11568"

def count_warnings():
    out = subprocess.run(
        ["docker", "exec", "mongo", "mongosh", "--quiet", "trustmesh", "--eval",
         f'db.comments.countDocuments({{task_id:"{TASK_ID}", content: /未入库/}})'],
        capture_output=True, text=True, encoding="utf-8").stdout
    return int(out.strip().splitlines()[-1])

def post(tid, name):
    payload = {
        "nodeId": PLATFORM_NODE, "type": "transfer.received", "from": FROM_NODE,
        "sessionKey": TASK_ID,
        "message": json.dumps({
            "transferId": tid, "fileName": name, "fileSize": 1024,
            "localPath": f"/var/lib/trustmesh-transfers/{tid}-{name}",
            "mimeType": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        }, ensure_ascii=False),
        "metadata": {"taskId": TASK_ID, "todoId": TODO_ID},
    }
    req = urllib.request.Request(WEBHOOK, data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
                                 headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            return r.status, r.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace")

print(f"基线未入库告警数: {count_warnings()}")
before = count_warnings()

# A) 同名文件已在库（角色清单 03:30 已入库）-> 应静默
code, body = post("VERIFYdup0000000000001", "生死靶心_角色清单_v01_20260903.xlsx")
after_a = count_warnings()
print(f"\nA) 重复上传（同名已入库）: HTTP {code}")
print(f"   告警数 {before} -> {after_a}  {'PASS 静默' if after_a == before else 'FAIL 仍在误报'}")

# B) 新文件名（库中不存在）-> 应新增 1 条
code, body = post("VERIFYnew0000000000001", "验证_不存在的文件_20260903.xlsx")
after_b = count_warnings()
print(f"\nB) 真实缺失文件: HTTP {code}")
print(f"   告警数 {after_a} -> {after_b}  {'PASS 新增1条' if after_b == after_a + 1 else 'FAIL 期望+1'}")

# C) B 的完全重复投递 -> 不应新增
code, body = post("VERIFYnew0000000000001", "验证_不存在的文件_20260903.xlsx")
after_c = count_warnings()
print(f"\nC) 同一 transfer 双发: HTTP {code}")
print(f"   告警数 {after_b} -> {after_c}  {'PASS 去重' if after_c == after_b else 'FAIL 仍重复写'}")

print(f"\n总计: {before} -> {after_c}（期望 +1，即只有真实缺失那一条）")
