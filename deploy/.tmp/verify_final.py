import json, subprocess, urllib.request

BASE = "http://localhost:8080/api/v1"
def sh(cmd):
    return subprocess.run(cmd, shell=True, capture_output=True, text=True, encoding="utf-8").stdout

def api(path, token=None, method="GET", data=None):
    req = urllib.request.Request(BASE + path, method=method,
        data=json.dumps(data).encode() if data else None,
        headers={"Content-Type": "application/json", **({"Authorization": "Bearer " + token} if token else {})})
    with urllib.request.urlopen(req, timeout=20) as r:
        return json.loads(r.read().decode())

out = sh('docker exec mongo mongosh --quiet trustmesh --eval \'var u=db.users.findOne({email:"test001@163.com"}); print(u._id)\'')
print("user:", out.strip()[:40])

# 直接用 API 登录
with urllib.request.urlopen(urllib.request.Request(BASE + "/auth/login",
        data=json.dumps({"email": "test001@163.com", "password": "qwer1234"}).encode(),
        headers={"Content-Type": "application/json"}, method="POST"), timeout=20) as r:
    d = json.loads(r.read().decode())
tok = (d.get("data") or {}).get("access_token") or d.get("access_token")
if not tok:
    print("登录失败:", json.dumps(d)[:300]); raise SystemExit(1)

task = api("/tasks/31319dd8419fb49d075acc33", tok)["data"]
print("\n任务:", task["title"], "| status:", task["status"])
arts = task.get("artifacts") or []
print("交付文件:", len(arts))
for a in arts:
    print("  -", a["file_name"], "| kind:", a.get("kind"), "| output:", a.get("output_name") or "-")

cms = api("/tasks/31319dd8419fb49d075acc33/comments", tok)["data"]
warn = [c for c in (cms if isinstance(cms, list) else cms.get("items", [])) if "未入库" in (c.get("content") or "")]
print("\n时间线评论总数:", len(cms), "| 其中「未入库」告警:", len(warn))
for c in warn:
    print("  -", (c.get("content") or "")[:60])
