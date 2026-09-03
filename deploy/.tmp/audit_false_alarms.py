"""全库扫描：区分「未入库」告警里的真缺失与误报。

判定：告警里的文件名若已存在于该任务的 db.artifacts，说明文件其实入库了，
这次拒绝只是重复上传 -> 误报。否则是真缺失（文件留在传输卷，平台看不到）。
只读，不修改任何数据。
"""
import json, re, subprocess

def mongosh(js):
    out = subprocess.run(["docker", "exec", "mongo", "mongosh", "--quiet", "trustmesh", "--eval", js],
                         capture_output=True, text=True, encoding="utf-8").stdout
    return out

js = r'''
var rows = [];
db.comments.find({content: /文件上传未入库/}).sort({created_at: 1}).forEach(function(c){
  var m = c.content.match(/^⚠️ 文件上传未入库：(.+?)（/);
  var tid = c.content.match(/transferId=([A-Za-z0-9_-]+)/);
  rows.push({
    task_id: c.task_id || "",
    file_name: m ? m[1] : "",
    transfer_id: tid ? tid[1] : "",
    created_at: c.created_at.toISOString(),
    comment_id: String(c._id)
  });
});
print(JSON.stringify(rows));
'''
raw = mongosh(js).strip()
start = raw.find("[")
rows = json.loads(raw[start:]) if start >= 0 else []

# 任务标题
titles = {}
tjs = r'''
var t = {};
db.tasks.find({}, {title: 1}).forEach(function(d){ t[String(d._id)] = d.title; });
print(JSON.stringify(t));
'''
traw = mongosh(tjs).strip()
tstart = traw.find("{")
if tstart >= 0:
    titles = json.loads(traw[tstart:])

# 每个任务已入库的文件名
filed = {}
fjs = r'''
var f = {};
db.artifacts.find({}, {task_id: 1, file_name: 1}).forEach(function(a){
  var k = String(a.task_id);
  if (!f[k]) f[k] = [];
  f[k].push(a.file_name);
});
print(JSON.stringify(f));
'''
fraw = mongosh(fjs).strip()
fstart = fraw.find("{")
if fstart >= 0:
    filed = json.loads(fraw[fstart:])

false_alarm, real_missing = [], []
seen = set()
for r in rows:
    names = set(filed.get(r["task_id"], []))
    key = (r["task_id"], r["file_name"], r["transfer_id"])
    dup = key in seen   # 同一 transfer 的重复告警（双发）
    seen.add(key)
    entry = dict(r, dup=dup, task_title=titles.get(r["task_id"], "(未知任务)"))
    (false_alarm if r["file_name"] in names else real_missing).append(entry)

print("扫描到「文件上传未入库」评论:", len(rows), "条\n")

print("=== A. 误报（文件其实已入库，属重复上传） ===")
for e in false_alarm:
    flag = "  [双发重复]" if e["dup"] else ""
    print(f"  {e['created_at'][:19]} | {e['task_title'][:28]} | {e['file_name']}{flag}")

print(f"\n小计 {len(false_alarm)} 条，涉及 {len({e['task_id'] for e in false_alarm})} 个任务")

print("\n=== B. 真缺失（文件确实不在库） ===")
for e in real_missing:
    flag = "  [双发重复]" if e["dup"] else ""
    print(f"  {e['created_at'][:19]} | {e['task_title'][:28]} | {e['file_name']}{flag}")

print(f"\n小计 {len(real_missing)} 条，涉及 {len({e['task_id'] for e in real_missing})} 个任务")
