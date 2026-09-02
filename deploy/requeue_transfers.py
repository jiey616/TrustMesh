"""补登「已传到平台节点但后端 422 拒收」的文件。

背景
----
agent 用 `clawsynapse transfer send` 上传文件时若漏掉 `--metadata taskId=`，
后端 `handleTransferReceived` 会返回 422 missing metadata：文件真实落盘在共享卷
`trustmesh-transfer-data`（平台节点与 backend 各挂一份）上，但 artifacts /
project_files 一条都不会写，平台文件页看不到。

修好 skill 之后，历史丢掉的文件可以用本脚本补登 —— 直接重放一条带正确 metadata
的 `transfer.received` webhook，让后端走它自己的落库 + 复制逻辑，不手动写 Mongo。

用法
----
    # 先看看会补哪些（不改动任何东西）
    python requeue_transfers.py --task-id <taskId> --todo-id <todoId> --pattern ".md" --dry-run

    # 实际补登（默认只处理最近 24 小时的文件）
    python requeue_transfers.py --task-id <taskId> --todo-id <todoId> --pattern ".md"

    # 指定时间窗（分钟）
    python requeue_transfers.py --task-id <taskId> --todo-id <todoId> --since-minutes 180

文件名格式：<22位 transferId>-<原始文件名>
"""
import argparse
import json
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request

BACKEND_CONTAINER = "trustmesh-backend"
TRANSFER_DIR = "/var/lib/trustmesh-transfers"
WEBHOOK_URL = "http://localhost:8080/webhook/clawsynapse"

# 平台节点（webhook 转发方）与来源 agent 节点，仅用于补登记录的 from/nodeId 字段。
PLATFORM_NODE = "n1-cb979d6ad5fb2df289ecb50d3cfc18bc"

# 文件名前缀里的 transferId 是 22 位 base64url（NanoID 风格）
TRANSFER_RE = re.compile(r"^(?P<tid>[A-Za-z0-9_-]{22})-(?P<name>.+)$")

MIME_BY_EXT = {
    ".md": "text/markdown",
    ".txt": "text/plain",
    ".json": "application/json",
    ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    ".pdf": "application/pdf",
}


def list_transfer_files():
    """在 backend 容器里列出共享卷上的文件 → [(tid, name, size, mtime_epoch)]."""
    # backend 是 Alpine/busybox：ls 没有 --time-style，改用 stat。
    # 必须写成 "./$f"：transferId 可能以 '-' 开头（如 -1l5AXMTeefxrkphYP_xAw），
    # 裸 "$f" 会被 stat 当成命令行选项，导致该文件被静默跳过。
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


def post_transfer_received(tid, name, size, from_node, task_id, todo_id, mtime, output_name=""):
    local_path = f"{TRANSFER_DIR}/{tid}-{name}"
    ext = "." + name.rsplit(".", 1)[-1].lower() if "." in name else ""
    payload = {
        "nodeId": PLATFORM_NODE,
        "type": "transfer.received",
        "from": from_node,
        "sessionKey": task_id,
        "message": json.dumps({
            "transferId": tid,
            "fileName": name,
            "fileSize": size,
            "localPath": local_path,
            "mimeType": MIME_BY_EXT.get(ext, ""),
        }, ensure_ascii=False),
        "metadata": {"taskId": task_id, "todoId": todo_id, **({"outputName": output_name} if output_name else {})},
    }
    req = urllib.request.Request(
        WEBHOOK_URL,
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            return resp.status, resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace")
    except Exception as e:  # noqa: BLE001
        return 0, str(e)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--task-id", required=True)
    ap.add_argument("--todo-id", default="")
    ap.add_argument("--from-node", required=True, help="来源 agent 的 node id（n1-...）")
    ap.add_argument("--output-name", default="", help="绑定到工作流步骤输出的名字（原样使用任务 workflow 里定义的输出名），留空则按过程文件补登")
    ap.add_argument("--pattern", default="", help="只处理文件名包含该子串的文件")
    ap.add_argument("--name-regex", default="", help="只处理原始文件名匹配该正则的文件")
    ap.add_argument("--since-minutes", type=int, default=1440)
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    cutoff = time.time() - args.since_minutes * 60
    name_re = re.compile(args.name_regex) if args.name_regex else None
    all_files = list_transfer_files()
    picked = [
        f for f in all_files
        if f[3] >= cutoff
        and (not args.pattern or args.pattern in f[1])
        and (name_re is None or name_re.search(f[1]))
    ]
    picked.sort(key=lambda f: f[3])

    # Same-name collapse: backend dedups by (todo, file_name) only in memory,
    # but persistArtifactUnsafe upserts by _id=TransferID. Replaying the same
    # file_name under a different transfer id (e.g. agent retry) would create
    # a duplicate artifact row in Mongo. Keep only the freshest entry per
    # (todo_id, file_name) here, so we requeue once.
    seen = {}
    for f in picked:
        key = (args.todo_id, f[1])
        seen[key] = f  # last write wins because picked is sorted by mtime asc
    picked = list(seen.values())
    picked.sort(key=lambda f: f[3])

    if not picked:
        print(f"未找到匹配文件（最近 {args.since_minutes} 分钟，pattern={args.pattern!r}）")
        return 0

    print(f"匹配到 {len(picked)} 个文件（最近 {args.since_minutes} 分钟）\n")
    ok = fail = 0
    for tid, name, size, mtime in picked:
        stamp = time.strftime("%m-%d %H:%M", time.localtime(mtime))
        if args.dry_run:
            print(f"  [dry-run] {stamp}  {name}  ({size} B)  tid={tid}")
            continue
        code, body = post_transfer_received(
            tid, name, size, args.from_node, args.task_id, args.todo_id, mtime,
            output_name=args.output_name,
        )
        if code == 200:
            ok += 1
            print(f"  [ OK ] {stamp}  {name}  ({size} B)")
        else:
            fail += 1
            print(f"  [FAIL] {stamp}  {name}  HTTP {code}  {body[:200]}")
        time.sleep(0.3)

    if not args.dry_run:
        print(f"\n完成：成功 {ok} / 失败 {fail}")
    return 1 if fail else 0


if __name__ == "__main__":
    sys.exit(main())
