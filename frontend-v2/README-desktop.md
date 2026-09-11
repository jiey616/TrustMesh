# TrustMesh 桌面版（Windows）

基于 Electron 的 Windows 桌面客户端，复用 `frontend-v2` 的全部前端代码。
**服务端地址在应用内配置**（默认 `http://localhost:8080`），不需要随客户端打包后端。

## 功能特性

- 完整复用 Web 端全部功能（项目 / 任务 / 数字员工 / 会议 / 知识库 / 运维工单…）
- 启动后可在应用内修改服务端地址：
  - **登录页右下角**「服务器地址」浮动按钮（未登录即可配置）
  - 登录后：侧边栏左下角「工作区菜单 → 服务器设置」，或「个人信息」页（`/profile`）中的「服务器连接」卡片
  - 支持「测试连接」，保存后应用自动刷新并连接新服务端
- 单实例运行；外链自动用系统浏览器打开

## 开发调试

```bash
cd frontend-v2
npm install
npm run desktop:dev   # Vite 热更新 + Electron 壳，自动开 DevTools
```

## 打包发布

```bash
cd frontend-v2
npm run desktop:pack
```

产物在 `frontend-v2/release/`：

| 文件 | 说明 |
|---|---|
| `TrustMesh-Setup-x.y.z.exe` | NSIS 安装程序（可选安装目录、桌面快捷方式） |
| `TrustMesh-Portable-x.y.z.exe` | 绿色便携版，双击即用 |

## 服务端地址说明

- 地址保存在系统用户目录的 localStorage（安装版/便携版互不影响系统其他数据）
- 支持 `http://` 与 `https://`，可带端口与路径前缀
- 留空保存 = 恢复默认 `http://localhost:8080`
- 也可通过构建时环境变量 `VITE_API_BASE_URL` 固化地址（优先级最高）

## 技术要点

- 前端请求统一走 `ky` 的 `prefixUrl = <服务端>/api/v1/`，SSE（`/events/stream`、`assistant/chat`）与文件下载链接同样基于该地址
- Electron 内用 `HashRouter`（`file://` 协议下 history 路由刷新会 404），浏览器内仍用 `BrowserRouter`
- 图标：`build/icon.ico`（品牌紫色闪电）
