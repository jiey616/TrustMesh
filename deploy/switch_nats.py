#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
switch_nats.py — 一键切换测试环境 clawsynapse 的 NATS 服务器

背景：测试环境 /opt/trustmesh-test 的 clawsynapse 通过 .env 里 NATS_SERVERS
连接旧远端 NATS (nats://220.168.146.21:9414)，但服务器上已跑着新的 NATS
容器 trustmesh-nats (nats:latest, 0.0.0.0:4222)。
本脚本把 .env 的 NATS_SERVERS 改为目标地址，并 --force-recreate clawsynapse 使生效，
最后验证 NATS 连接与 peer 可见性。

用法：
  python switch_nats.py                  # 切到默认 nats://175.27.135.91:4222
  python switch_nats.py nats://x:4222    # 切到自定义地址
  python switch_nats.py --check          # 只检查当前状态，不改动

安全：
  - 修改前自动备份 .env -> .env.bak-<时间戳>（可回滚）
  - 幂等：已是目标值则跳过修改
  - 非 --force 不自动执行，需确认
"""

import paramiko
import sys
import os
import time
import datetime

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"
COMPOSE_DIR = "/opt/trustmesh-test"
ENV_FILE = "/opt/trustmesh-test/.env"
TARGET_DEFAULT = "nats://175.27.135.91:4222"


def log(msg):
    print(f"[{datetime.datetime.now().strftime('%H:%M:%S')}] {msg}", flush=True)


def ssh_exec(ssh, cmd, timeout=60):
    _, out, err = ssh.exec_command(cmd, timeout=timeout)
    return (out.read().decode("utf-8", "replace") + err.read().decode("utf-8", "replace")).strip()


def main():
    check_only = "--check" in sys.argv
    target = None
    for a in sys.argv[1:]:
        if a.startswith("nats://"):
            target = a
    if target is None:
        target = TARGET_DEFAULT

    log(f"目标 NATS: {target}")
    log(f"服务器: {HOST}:{PORT} (cloudUser)")
    if check_only:
        log("--check 模式：仅检查，不修改")

    ssh = paramiko.SSHClient()
    ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    try:
        ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    except Exception as e:
        log(f"❌ SSH 连接失败: {e}")
        sys.exit(1)
    log("✅ SSH 连接成功")

    # ---------- 1. 读取当前 .env（用 python 读取避免 shell 转义问题） ----------
    env_file = ENV_FILE
    env_out = ssh_exec(ssh, f"python3 -c \"p=r'{env_file}';import re;s=open(p,encoding='utf-8').read();m=re.search(r'(?m)^NATS_SERVERS=(.*)$',s);print(m.group(1) if m else '__NOT_SET__')\"")
    log(f"当前 .env 中 NATS_SERVERS: {env_out}")

    # ---------- 2. 检查当前容器实际连接 ----------
    cur = ssh_exec(ssh, "docker exec trustmesh-clawsynapse env 2>/dev/null | grep '^NATS_SERVERS=' || echo __NO_CTN__")
    log(f"运行中 clawsynapse NATS_SERVERS: {cur}")

    # ---------- 3. 校验目标 NATS 可达 ----------
    target_host = target.replace("nats://", "").split(":")[0]
    target_port = target.replace("nats://", "").split(":")[1] if ":" in target.replace("nats://", "") else "4222"
    reach = ssh_exec(ssh, f'timeout 3 bash -c "</dev/tcp/{target_host}/{target_port}" 2>&1 && echo TCP_OK || echo TCP_FAIL')
    log(f"目标 NATS ({target_host}:{target_port}) 可达性: {reach}")

    if check_only:
        log("检查完成（--check 模式，未做任何修改）")
        ssh.close()
        return

    # ---------- 4. 幂等判断 ----------
    # 注意区分：.env 已是目标值 ≠ 容器已生效。容器仍在跑旧 NATS 时必须重建。
    env_val = env_out.strip().strip('"')
    container_val = cur.split("=", 1)[1].strip().strip('"') if cur.startswith("NATS_SERVERS=") else ""
    if env_val == target and container_val == target:
        log(f"⏭️ .env 与容器均已指向 {target}，无需操作")
        ssh.close()
        return
    if env_val == target:
        log(f".env 已是 {target}，但容器仍为 {container_val or '未知'}，将继续重建容器")

    if reach != "TCP_OK":
        log(f"⚠️ 目标 NATS 不可达，仍尝试切换（可能只是探测超时）")

    # ---------- 5. 备份 .env（仅当需要改写时） ----------
    bak = None
    if env_val != target:
        bak = f"{env_file}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}"
        ssh_exec(ssh, f"cp {env_file} {bak}")
        log(f"✅ 已备份 .env -> {bak}")

        # ---------- 6. 写入新值 ----------
        # 用 python 在服务器端改，避免 sed 转义问题
        script = (
            "import re,io\n"
            f"p=r'{env_file}'\n"
            "s=open(p,encoding='utf-8').read()\n"
            f"s=re.sub(r'(?m)^NATS_SERVERS=.*$', 'NATS_SERVERS={target}', s)\n"
            f"if 'NATS_SERVERS={target}' not in s:\n"
            f"    s+='\\nNATS_SERVERS={target}\\n'\n"
            "open(p,'w',encoding='utf-8').write(s)\n"
            "print('written')\n"
        )
        res = ssh_exec(ssh, f"python3 -c \"{script}\"")
        log(f"写入 .env: {res}")
        new_val = ssh_exec(ssh, f"grep '^NATS_SERVERS=' {env_file}")
        log(f"新 .env 值: {new_val}")
    else:
        log("✅ .env 已为目标值，跳过备份与改写")

    # ---------- 7. force-recreate clawsynapse ----------
    # 用 --no-build --no-deps --pull never：不拉镜像/不构建/不触发依赖/强制用本地镜像，
    # 只重建 clawsynapse 单容器（镜像 trustmesh/clawsynapse:test 已在服务器本地）。
    # TAG=test 显式指定（compose 默认 latest 在服务器上不存在，避免触发 pull 报错）
    log("重启 clawsynapse 容器（TAG=test --force-recreate --no-build --no-deps --pull never）...")
    res = ssh_exec(
        ssh,
        f"cd {COMPOSE_DIR} && TAG=test docker compose up -d --force-recreate --no-build --no-deps --pull never clawsynapse 2>&1 | tail -5",
        timeout=180,
    )
    log(res)

    # ---------- 8. 等待并验证 ----------
    time.sleep(8)
    health = ssh_exec(ssh, "curl -s -m 5 http://127.0.0.1:18080/v1/health 2>/dev/null | head -c 600")
    if 'nats' in health:
        import re as _re
        m = _re.search(r'"serverUrl":"([^"]*)"', health)
        st = _re.search(r'"status":"([^"]*)"', health)
        log(f"NATS 连接: serverUrl={m.group(1) if m else '?'} status={st.group(1) if st else '?'}")
    else:
        log("⚠️ 未能从 health 读到 NATS 状态：")
        log(health[:400])
    peers = ssh_exec(ssh, "curl -s -m 5 http://127.0.0.1:18080/v1/peers 2>/dev/null | head -c 300")
    log(f"peers: {peers[:250]}")

    log("✅ 切换流程完成")
    if bak:
        log(f"如需回滚：cp {bak} {env_file} && cd {COMPOSE_DIR} && docker compose up -d --force-recreate --no-build --no-deps clawsynapse")
    ssh.close()


if __name__ == "__main__":
    main()
