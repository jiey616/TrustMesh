# 岗位市场数据

> 本目录为岗位市场的全部静态数据；`roles/` 由脚本生成，勿手工编辑（`--clean` 会清空重建）。

本目录存放 TrustMesh 岗位市场的静态数据。`roles/` 下每个子目录即一个岗位，固定包含三件套：

| 文件 | 作用 |
|------|------|
| `IDENTITY.md` | 岗位名 + 一句话简介（索引解析只取前两行） |
| `SOUL.md` | 身份人设、记忆、沟通风格、关键规则 |
| `AGENTS.md` | 核心使命、工作流程、交付物 |

## 数据来源

内容取自开源项目 [agency-agents-zh](https://github.com/jnMetaCode/agency-agents-zh)
（上游 [agency-agents](https://github.com/msitarzewski/agency-agents)，**MIT License**），
按其 `scripts/convert.sh --tool openclaw` 的章节划分规则转换为上述三件套。

- 导入规模：20 个部门 / 264 个岗位
- 许可：MIT，每个岗位的 `IDENTITY.md` 内均已标注来源
- 上游 `convert.sh` 的 `AGENT_DIRS` 漏了 `strategy` 目录，本仓库的导入脚本已覆盖

## 重新生成

```bash
# 1. 拉取上游仓库（任意目录）
git clone --depth 1 git@github.com:jnMetaCode/agency-agents-zh.git tmp/agency/repo

# 2. 转换为岗位包
python backend/scripts/build_market_roles.py --src tmp/agency/repo --out data/roles --clean

# 3. 重建索引（勿手改 roles_index.json）
cd backend && go run ./cmd/gen-roles-index -roles ../data/roles -output ../data/roles_index.json -prefix data/roles
```

## 部署

容器通过只读挂载取得数据（`/app/data`），来源于仓库根的 `data/`：

- 本地 `docker-compose.yml`：`./data:/app/data:ro`
- 生产 `docker-compose.prod.yml`：`./data/roles_index.json` 与 `./data/roles` 分别只读挂载

索引在 backend 启动时一次性载入内存，改动数据后需重启 backend 生效。
