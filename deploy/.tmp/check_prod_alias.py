# -*- coding: utf-8 -*-
"""只读检查生产环境的 compose / .env 是否存在与本地相同的 DNS 别名问题。

本地已于 2026-09-03 修复（commit bc792eb）：clawsynapse service 指定了
container_name 后，service 名不再注册为 DNS 别名，导致 backend 按
CLAWSYNAPSE_API_URL=http://clawsynapse:18080 解析失败。
本脚本只读取，不做任何修改。
"""
import sys
import os

_here = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, _here)
sys.path.insert(0, os.path.dirname(_here))  # ssh_helper.py 在上级 deploy/ 目录

from ssh_helper import connect, run  # noqa: E402

CHECK = (
    "cd /opt/trustmesh-test && "
    "echo '=== compose 里 clawsynapse service 定义 ==='; "
    "grep -n -A5 '^  clawsynapse:' docker-compose.yml; "
    "echo; "
    "echo '=== 是否已配 aliases ==='; "
    "grep -n 'aliases' docker-compose.yml || echo '(无 aliases 配置 -> 存在同样问题)'; "
    "echo; "
    "echo '=== .env 的 CLAWSYNAPSE_API_URL ==='; "
    "grep -i 'CLAWSYNAPSE_API_URL' .env || echo '(未显式配置，走 compose 默认值 http://clawsynapse:18080)'; "
    "echo; "
    "echo '=== backend 近 3 分钟 peer sync 报错次数 ==='; "
    "docker logs --since 3m trustmesh-backend 2>&1 | grep -c 'peer sync failed' || echo 0"
)


def main() -> None:
    ssh = connect()
    try:
        print(run(ssh, CHECK, quiet=True))
    finally:
        ssh.close()


if __name__ == "__main__":
    main()
