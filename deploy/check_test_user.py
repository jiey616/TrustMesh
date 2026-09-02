"""只读：查测试机 Mongo 里有哪些用户，确认测试账号是否存在。"""
from ssh_helper import connect, run

ssh = connect()
try:
    print("=== 用户列表（email / 创建时间） ===")
    run(
        ssh,
        "export PATH=$HOME/.local/bin:$PATH; docker exec trustmesh-mongo mongosh --quiet "
        "--eval 'db.getSiblingDB(\"trustmesh\").users.find({}, {email:1, created_at:1}).limit(20).toArray()'",
        timeout=90,
    )
    print("\n=== 用户总数 ===")
    run(
        ssh,
        "export PATH=$HOME/.local/bin:$PATH; docker exec trustmesh-mongo mongosh --quiet "
        "--eval 'print(db.getSiblingDB(\"trustmesh\").users.countDocuments({}))'",
        timeout=90,
    )
    print("\n=== external_apps 集合现状 ===")
    run(
        ssh,
        "export PATH=$HOME/.local/bin:$PATH; docker exec trustmesh-mongo mongosh --quiet "
        "--eval 'db.getSiblingDB(\"trustmesh\").external_apps.find({}, {name:1, placement:1, visibility:1, status:1}).toArray()'",
        timeout=90,
    )
finally:
    ssh.close()
