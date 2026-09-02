"""只读：确认生产机上是否有标准测试账号、users 规模。

⚠️ 已停用（2026-09-01 用户确认）：生产机已切换为 175.27.135.91:62000
   （cloudUser@/opt/trustmesh-test，见 ssh_helper.py），36.137.106.15 不再承载
   TrustMesh 生产部署。复用前请改连接参数并删除护栏。
"""
import sys

sys.exit(
    "⛔ 本脚本已停用：生产机不再是 36.137.106.15。\n"
    "   现在生产 = 175.27.135.91:62000 (cloudUser, /opt/trustmesh-test)，见 deploy/ssh_helper.py。\n"
    "   确认后请更新连接参数并删除本护栏。"
)

import paramiko

HOST = "36.137.106.15"
USER = "root"
PASS = "Lh1804@1806"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect(HOST, username=USER, password=PASS, timeout=20)


def run(cmd, timeout=90):
    print(f"$ {cmd[:160]}")
    _, out, err = ssh.exec_command(cmd, timeout=timeout)
    o = out.read().decode(errors="replace").strip()
    e = err.read().decode(errors="replace").strip()
    if o:
        print(o[:1500])
    if e:
        print(f"[stderr] {e[:500]}")
    print()


try:
    print("=== 生产机是否有 test001@163.com ===")
    run(
        "docker exec trustmesh-mongo mongosh --quiet --eval "
        "'db.getSiblingDB(\"trustmesh\").users.find({email:/test001/}, {email:1, created_at:1}).toArray()'"
    )
    print("=== 生产机用户总数 ===")
    run("docker exec trustmesh-mongo mongosh --quiet --eval 'print(db.getSiblingDB(\"trustmesh\").users.countDocuments({}))'")
finally:
    ssh.close()
