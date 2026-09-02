"""审计传输卷上「已落盘但未入库」的孤儿文件。

背景
----
agent 上传漏 `--metadata taskId=` 时后端会 422 拒收：文件真实落在共享卷
`trustmesh-transfer-data` 上，但 artifacts 集合里没有对应记录，平台完全看不到
（2026-09-02 军旅任务一次丢了 7 个过程文件）。2026-09-02 已加后端兜底推断 +
失败可见化，但**历史上已经丢的文件**不会被自动找回，需要本脚本扫出来再补登。

用法
----
    python audit_orphan_transfers.py            # 列出所有孤儿文件
    python audit_orphan_transfers.py --json     # JSON 输出，便于后续脚本消费

文件名格式：<22位 transferId>-<原始文件名>
"""
import argparse
import json
import re
import subprocess
import sys
import time

BACKEND_CONTAINER = "trustmesh-backend"
TRANSFER_DIR = "/var/lib/trustmesh-transfers"
MONGO_CONTAINER = "mongo"
DB_NAME = "trustmesh"

TRANSFER_RE = re.compile(r"^(?P<tid>[A-Za-z0-9_-]{22})-(?P<name>.+)$")


def list_disk_files():
    """列出传输卷上的文件 → [(tid, name, size, mtime_epoch)]."""
    out = subprocess.run(
        ["docker", "exec", BACKEND_CONTAINER, "sh", "-c",
         f'cd {TRANSFER_DIR} && for f in *; do stat -c "%n|%s|%Y" "./$f"; done'],
        capture_output=True, text=True, encoding="utf-8", check=True,
    ).stdout
    files = []
    for line in out.splitlines():
        parts = line.rsplit("|", 2)
        if len(parts) != 3:
            continue
        name, size_s, mtime_s = parts
        if name.startswith("./"):
            name = name[2:]
        try:
            size, mtime = int(size_s), int(mtime_s)
        except ValueError:
            continue
        m = TRANSFER_RE.match(name)
        if m:
            files.append((m.group("tid"), m.group("name"), size, mtime))
    return files


def list_db_transfer_ids():
    """取 artifacts 集合里所有已入库的 transferId（_id 就是 transferId）。"""
    out = subprocess.run(
        ["docker", "exec", MONGO_CONTAINER, "mongosh", DB_NAME, "--quiet", "--eval",
         "db.artifacts.find({}, {_id: 1}).forEach(a => print(a._id))"],
        capture_output=True, text=True, encoding="utf-8", check=True,
    ).stdout
    return {line.strip() for line in out.splitlines() if line.strip()}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--json", action="store_true", help="以 JSON 输出")
    args = ap.parse_args()

    disk = list_disk_files()
    in_db = list_db_transfer_ids()
    orphans = [f for f in disk if f[0] not in in_db]
    orphans.sort(key=lambda f: f[3])

    if args.json:
        print(json.dumps([{
            "transfer_id": t, "file_name": n, "size": s,
            "mtime": time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(m)),
        } for t, n, s, m in orphans], ensure_ascii=False, indent=2))
        return 0

    print(f"传输卷文件 {len(disk)} 个 | 已入库 {len(disk) - len(orphans)} 个 | 孤儿 {len(orphans)} 个\n")
    if not orphans:
        print("✅ 没有落盘未入库的文件")
        return 0
    print("落盘未入库（需补登或确认是否本就无需入库）：\n")
    for tid, name, size, mtime in orphans:
        stamp = time.strftime("%m-%d %H:%M", time.localtime(mtime))
        print(f"  {stamp}  {size:>8} B  {name}   tid={tid}")
    print("\n补登：requeue_transfers.py --task-id <任务ID> --todo-id <TD_x> "
          "--from-node <n1-...> --since-minutes N --name-regex <正则>")
    return 0


if __name__ == "__main__":
    sys.exit(main())
