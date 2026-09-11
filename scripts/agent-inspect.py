#!/usr/bin/env python3
"""从 `docker inspect <容器>` 的完整 JSON 输出，生成 docker run 启动参数片段。

用法:
  docker inspect <容器> | python3 scripts/agent-inspect.py

输出为逐行参数，供 agent-recreate.sh 拼装 docker run 命令。
"""
import json
import sys


def main() -> None:
    data = json.load(sys.stdin)
    if isinstance(data, list):
        data = data[0]  # docker inspect <name> 返回单元素数组
    cfg = data.get("Config", {})
    host = data.get("HostConfig", {})

    # --restart
    rp = host.get("RestartPolicy", {}).get("Name", "no") or "no"
    if rp != "no":
        print(f"--restart={rp}")

    # --hostname（跳过自动生成的容器 ID，避免新机固定旧 ID）
    hn = cfg.get("Hostname", "")
    if hn and not (len(hn) == 12 and hn.isalnum() and hn.islower()):
        print(f"--hostname={hn}")

    # -p 端口映射
    for cport, binds in host.get("PortBindings", {}).items():
        cnum = cport.split("/")[0]
        for b in binds or []:
            hp = b.get("HostPort", "")
            if hp:
                print(f"-p {hp}:{cnum}")

    # -v 卷挂载（仅 named volume）
    for m in data.get("Mounts", []):
        if m.get("Type") == "volume":
            print(f"-v {m['Name']}:{m['Destination']}")

    # --network（跳过默认 bridge/host）
    net = host.get("NetworkMode", "bridge") or "bridge"
    if net not in ("bridge", "host"):
        print(f"--network {net}")

    # -e 环境变量
    # 排除: PATH 自动注入、镜像构建期默认变量（LANG/GPG_KEY/PYTHON_* 等）
    IMAGE_BUILD_ENV = {
        "LANG", "GPG_KEY", "PYTHON_VERSION", "PYTHON_SHA256",
        "PYTHON_PIP_VERSION", "PYTHON_SETUPTOOLS_VERSION", "PYTHON_GET_PIP_URL",
        "PYTHON_GET_PIP_SHA256", "PIP_NO_INDEX", "PIP_DISABLE_PIP_VERSION_CHECK",
    }
    for e in cfg.get("Env", []) or []:
        key = e.split("=", 1)[0]
        if key == "PATH" or key in IMAGE_BUILD_ENV:
            continue
        print(f"-e {e}")

    # 镜像
    print(cfg.get("Image", ""))


if __name__ == "__main__":
    main()
