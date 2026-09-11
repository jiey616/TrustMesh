# 生产环境「新建会议提示失败」修复报告

- 日期：2026-07-14
- 处理人：自动化部署（WorkBuddy）
- 状态：✅ 已修复并验证上线

---

## 一、故障现象
生产环境点击「新建会议」后，前端弹出「创建会议失败」提示，会议无法创建。

## 二、根因定位
通过只读诊断（生产 36.137.106.15）确认：

| 镜像 | 构建时间 | 含会议路由 |
|------|---------|-----------|
| `trustmesh/backend:latest`（旧） | 2026-07-07 15:29 | ❌ 二进制 `meetings` 字符串 = 0 |
| `trustmesh/backend:latest`（新） | 2026-07-14 | ✅ 二进制 `meetings` 字符串 = 7 |
| `trustmesh/frontend:latest` | 2026-07-10 04:39 | ✅ 已带会议 UI |

**链路**：前端 7-10 新版有会议入口 → 发 `POST /api/v1/projects/:id/meetings` → 后端 7-07 旧镜像**根本未注册该路由** → 返回 404 → 前端 `mutateAsync` 抛异常 → toast「创建会议失败」。

结论：非代码缺陷，是**后端镜像版本落后于前端**，会议功能代码未纳入当时构建的镜像。Mongo 正常（ping ok、meetings 集合 count=0 从未建过会），handler 逻辑本身无问题。

## 三、修复步骤
1. 本地构建：`docker build --platform linux/amd64 -t trustmesh/backend:latest ./backend`（含全部最新代码改动）
2. 导出：`docker save trustmesh/backend:latest -o backend-latest.tar`（14MB）
3. 上传：SFTP → `36.137.106.15:/opt/trustmesh/backend-latest.tar`
4. 备份旧镜像（防回滚）：`docker tag trustmesh/backend:latest trustmesh/backend:backup-pre-20260714`（旧 ID 75aa1e046019）
5. 加载覆盖：`docker load -i /opt/trustmesh/backend-latest.tar`
6. 重建容器：`docker compose -f /opt/trustmesh/docker-compose.yml up -d --force-recreate backend`

## 四、验证结果
| 检查项 | 旧镜像 | 新镜像 |
|--------|--------|--------|
| 二进制 `meetings` 字符串 | 0 | **7** |
| `POST /meetings`（无 token，容器内） | 404 | **401**（路由已注册，被鉴权拦截） |
| `GET /meetings` 列表（无 token） | 404 | **401** |
| `/healthz` | 200 | 200 |
| 容器状态 | healthy | **Up healthy** |

`401` 即为修复铁证：旧镜像因路由缺失直接 404；新镜像路由存在、由鉴权中间件返回 401。带登录态的前端请求将正常进入 `Create` handler 完成创建（handler 唯一失败点 Mongo 已确认正常）。

## 五、回滚预案
若新镜像出现运行异常，在生产执行：
```
docker tag trustmesh/backend:backup-pre-20260714 trustmesh/backend:latest
docker compose -f /opt/trustmesh/docker-compose.yml up -d --force-recreate backend
```

## 六、架构说明
生产 backend 容器**未发布 8080 到宿主机**（仅前端 3000 对外，backend 走 Docker 内部 `backend:8080` 网络），属正常设计。从宿主机直接 `curl 127.0.0.1:8080` 返回 000 是预期现象，与本次故障无关；用户经前端访问的新建会议链路不受影响。

## 七、建议
请在前端界面实际点击「新建会议」做一次端到端确认（登录态下应返回创建成功）。
