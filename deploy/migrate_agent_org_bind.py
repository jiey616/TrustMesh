#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
migrate_agent_org_bind.py — T1.1 存量 agent 租户绑定回填

背景
----
T1.1 在 backend 侧新增「派发时租户强校验」(`strictDispatchOrgGate`)：
`dispatchNextTodo` 只向 `agent.OrgID == task.OrgID`（或 task 无 org）的节点派发，
未绑 org 的存量节点禁止接收生产派发。新注册的 agent 在 ApproveJoinRequest 时已落
`org_id`，但**存量 agent** 的 `org_id` 可能为空，直接开强校验会让它们接不到活。

本脚本把存量 `org_id` 为空的 agent 回填为「其 owner 用户的个人租户 (personal org)」，
与 backend `store.personalOrgOfUnsafe` 的语义一致。回填后重启 backend（内存态从 Mongo
重新加载），即可安全地把 `TRUSTMESH_STRICT_DISPATCH_ORG_GATE=true` 翻成强校验。

安全设计
--------
- 默认 **dry-run**：只统计 + 打印将如何绑定，不写 Mongo。必须显式 `--apply` 才落库。
- 只改 `agents` 集合里 `org_id` 缺失/为空的文档；其它字段一律不动。
- 若 owner 没有个人租户：默认 **跳过并报告**（不臆造 org）；如需补齐可加 `--create-personal-org`。
- 幂等：重复运行只会影响「仍为空」的 agent。

用法
----
    # 1) 先干跑，看影响面
    python migrate_agent_org_bind.py --dry-run

    # 2) 确认无误后真正回填
    python migrate_agent_org_bind.py --apply

    # 3) （可选）owner 没有个人租户时，连带创建 personal org + membership
    python migrate_agent_org_bind.py --apply --create-personal-org

环境变量
--------
    MONGO_URI      默认 mongodb://127.0.0.1:27017
    MONGO_DATABASE 默认 trustmesh
    （与 backend internal/config/config.go 的默认值一致）

运行位置
--------
需能直连目标 MongoDB。生产通常在 175.27.135.91，可在该宿主上
`docker exec` 进 mongo 容器旁执行，或从可达网络直连。
依赖 pymongo：pip install pymongo
"""
import os
import sys
import uuid

try:
    from pymongo import MongoClient
    from pymongo.errors import PyMongoError
except ImportError:
    sys.stderr.write("❌ 需要 pymongo：pip install pymongo\n")
    sys.exit(2)


def main() -> int:
    args = sys.argv[1:]
    apply = "--apply" in args
    dry_run = "--dry-run" in args or not apply
    create_personal = "--create-personal-org" in args

    uri = os.getenv("MONGO_URI", "mongodb://127.0.0.1:27017")
    db_name = os.getenv("MONGO_DATABASE", "trustmesh")

    if dry_run and not apply:
        print("🔍 DRY-RUN 模式：不会写入任何数据（加 --apply 才落库）")

    client = MongoClient(uri, serverSelectionTimeoutMS=10000)
    try:
        client.admin.command("ping")
    except PyMongoError as e:
        sys.stderr.write(f"❌ 无法连接 MongoDB ({uri}): {e}\n")
        return 1

    db = client[db_name]
    agents = db["agents"]
    orgs = db["organizations"]
    memberships = db["org_memberships"]

    # 存量未绑 org 的 agent：org_id 缺失，或为空串
    unbound_filter = {
        "$or": [
            {"org_id": {"$exists": False}},
            {"org_id": ""},
        ]
    }

    total = agents.count_documents({})
    unbound = list(agents.find(unbound_filter))
    print(f"📊 agents 总数={total}, 未绑 org 的存量 agent 数={len(unbound)}")

    if not unbound:
        print("✅ 没有需要回填的 agent，可直接开强校验。")
        return 0

    bound = 0
    skipped = 0
    created_orgs = 0
    plan_lines = []

    for a in unbound:
        aid = a.get("_id")
        uid = a.get("user_id", "")
        if not uid:
            plan_lines.append(f"  - SKIP  agent={aid} 无 user_id，无法定位个人租户")
            skipped += 1
            continue

        personal = orgs.find_one({"kind": "personal", "owner_id": uid})
        if personal is None:
            if create_personal:
                # 与 backend ensurePersonalOrgUnsafe 对齐：slug=u-<userID>, kind=personal, owner=userID
                org_id = "org_" + uuid.uuid4().hex[:16]
                now = None  # MongoDB 写入时自动用当前时间，不强依赖客户端时钟
                orgs.insert_one({
                    "_id": org_id,
                    "name": uid,
                    "slug": "u-" + uid,
                    "kind": "personal",
                    "owner_id": uid,
                })
                memberships.insert_one({
                    "_id": "om_" + uuid.uuid4().hex[:16],
                    "org_id": org_id,
                    "user_id": uid,
                    "role": "owner",
                })
                personal = {"_id": org_id}
                created_orgs += 1
                print(f"  + 为新用户 {uid} 创建个人租户 {org_id}")
            else:
                plan_lines.append(f"  - SKIP  agent={aid} user={uid} 无个人租户（加 --create-personal-org 可补齐）")
                skipped += 1
                continue

        target_org = personal["_id"]
        if dry_run:
            plan_lines.append(f"  - BIND  agent={aid} user={uid} -> org_id={target_org}")
        else:
            agents.update_one({"_id": aid}, {"$set": {"org_id": target_org}})
            bound += 1

    for line in plan_lines:
        print(line)

    if dry_run:
        print(f"🔍 DRY-RUN 完成：将绑定 {len(unbound) - skipped} 个，跳过 {skipped} 个"
              + (f"，将新建 {created_orgs} 个个人租户" if create_personal else ""))
        print("   确认无误后执行：python migrate_agent_org_bind.py --apply"
              + (" --create-personal-org" if create_personal else ""))
    else:
        print(f"✅ 回填完成：绑定 {bound} 个，跳过 {skipped} 个，新建个人租户 {created_orgs} 个")
        print("   下一步：重启 backend 使内存态从 Mongo 重新加载，再设 "
              "TRUSTMESH_STRICT_DISPATCH_ORG_GATE=true 开启强校验。")

    return 0


if __name__ == "__main__":
    sys.exit(main())
