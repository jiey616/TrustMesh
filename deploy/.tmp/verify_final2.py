import json, urllib.request
BASE = "http://localhost:8080/api/v1"
with urllib.request.urlopen(urllib.request.Request(BASE + "/auth/login",
        data=json.dumps({"email": "test001@163.com", "password": "qwer1234"}).encode(),
        headers={"Content-Type": "application/json"}, method="POST"), timeout=20) as r:
    d = json.loads(r.read().decode())
tok = (d.get("data") or {}).get("access_token")

def api(p):
    req = urllib.request.Request(BASE + p, headers={"Authorization": "Bearer " + tok})
    with urllib.request.urlopen(req, timeout=20) as r:
        return json.loads(r.read().decode()).get("data")

for tid, label in [("31319dd8419fb49d075acc33", "资产提取"), ("dc1f49153710fad78ca18a3a", "分镜拆解")]:
    t = api(f"/tasks/{tid}")
    arts = t.get("artifacts") or []
    deliverables = [a for a in arts if a.get("kind") == "deliverable"]
    print(f"\n=== {label} | {t['title']} | status={t['status']} ===")
    print(f"文件 {len(arts)} 个，其中交付物 {len(deliverables)} 个")
    for a in deliverables:
        print(f"   [交付] {a['file_name']}  ->  {a.get('output_name') or '(未绑定)'}")
