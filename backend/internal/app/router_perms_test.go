package app

import (
	"testing"
	"time"

	"trustmesh/backend/internal/authz"
	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/logger"
)

// 本文件是设计文档 §9「权限点漏标路由 → 默认拒绝」的落地机制：
//
//	routePermManifest 收录每一条 authed 路由的权限点（或 "base" = 登录即可用的
//	基础权限路由）。新增/变更路由时若不在此显式声明，测试立即失败 ——
//	逼开发在加路由的那一刻就做出权限决策，而不是靠自觉。
//
// 清单值只能是两类：
//   - authz 包定义的权限点常量（如 authz.PermAgentManage）
//   - permBase（基础权限：登录即可用，如仪表盘浏览、任务日常协作）
//
// 注意：本测试校验「每条路由都有显式声明」，声明与路由中间件的一致性靠
// code review 保证（gin 无法在 Routes() 里反查中间件参数）。

const permBase = "base"

// routePermManifest key = "METHOD /full/path"（gin Routes() 的原样格式）。
var routePermManifest = map[string]string{
	// ── 数字员工 ──
	"POST /api/v1/agents":                             authz.PermAgentManage,
	"GET /api/v1/agents":                              authz.PermAgentView,
	"GET /api/v1/agents/invite-prompt":                permBase, // 邀请提示词，任何登录用户可为自己的 agent 生成
	"GET /api/v1/agents/join-requests":                authz.PermJoinRequestApprove,
	"POST /api/v1/agents/join-requests/:id/approve":   authz.PermJoinRequestApprove,
	"POST /api/v1/agents/join-requests/:id/reject":    authz.PermJoinRequestApprove,
	"GET /api/v1/agents/:id":                          authz.PermAgentView,
	"PATCH /api/v1/agents/:id":                        authz.PermAgentManage,
	"DELETE /api/v1/agents/:id":                       authz.PermAgentManage,
	"GET /api/v1/agents/:id/stats":                    authz.PermAgentView,
	"GET /api/v1/agents/:id/insights":                 authz.PermAgentView,
	"GET /api/v1/agents/:id/tasks":                    authz.PermAgentView,
	"GET /api/v1/agents/:id/capabilities":             authz.PermAgentView,
	"POST /api/v1/agents/:id/capabilities":            authz.PermAgentManage,
	"GET /api/v1/agents/:id/cron/executions":          authz.PermAgentView,
	"POST /api/v1/agents/:id/skills/upload":           authz.PermAgentManage,
	"GET /api/v1/agents/:id/chat":                     permBase, // AI办公室日常交互
	"GET /api/v1/agents/:id/chat/sessions":            permBase,
	"GET /api/v1/agents/:id/chat/sessions/:sessionId": permBase,
	"POST /api/v1/agents/:id/chat/messages":           permBase,
	"POST /api/v1/agents/:id/chat/reset":              permBase,

	// ── 项目 ──
	"POST /api/v1/projects":                                                   authz.PermProjectCreate,
	"GET /api/v1/projects":                                                    permBase,
	"GET /api/v1/projects/:projectId":                                         permBase,
	"PATCH /api/v1/projects/:projectId":                                       authz.PermProjectManage,
	"DELETE /api/v1/projects/:projectId":                                      authz.PermProjectManage,
	"GET /api/v1/projects/:projectId/workflow-progress":                       permBase,
	"POST /api/v1/projects/:projectId/workflow/steps/:stepIndex/outputs/bind": permBase,

	// ── 工作流模板（只读浏览属基础权限） ──
	"POST /api/v1/workflow-templates":                                 authz.PermWorkflowTemplateMgr,
	"GET /api/v1/workflow-templates":                                  permBase,
	"GET /api/v1/workflow-templates/:templateId":                      permBase,
	"PATCH /api/v1/workflow-templates/:templateId":                    authz.PermWorkflowTemplateMgr,
	"POST /api/v1/workflow-templates/:templateId/copy":                authz.PermWorkflowTemplateMgr,
	"POST /api/v1/workflow-templates/:templateId/curate":              authz.PermWorkflowTemplateMgr,
	"DELETE /api/v1/workflow-templates/:templateId":                   authz.PermWorkflowTemplateMgr,
	"POST /api/v1/tasks/:id/distill-template":                         authz.PermWorkflowTemplateMgr,
	"POST /api/v1/projects/:projectId/workflows/inherit":              authz.PermWorkflowTemplateMgr,
	"GET /api/v1/projects/:projectId/workflows/:workflowId/sync-diff": permBase, // 只读 diff 预览
	"POST /api/v1/projects/:projectId/workflows/:workflowId/sync":     authz.PermWorkflowTemplateMgr,
	"POST /api/v1/projects/:projectId/workflows/:workflowId/detach":   authz.PermWorkflowTemplateMgr,

	// ── 项目文件（日常协作，全部基础权限） ──
	"POST /api/v1/projects/:projectId/files":                 permBase,
	"POST /api/v1/projects/:projectId/folders":               permBase,
	"GET /api/v1/projects/:projectId/files/browse":           permBase,
	"GET /api/v1/projects/:projectId/files/artifacts":        permBase,
	"GET /api/v1/projects/:projectId/files/tree":             permBase,
	"GET /api/v1/projects/:projectId/files":                  permBase,
	"GET /api/v1/projects/:projectId/files/:fileId/content":  permBase,
	"DELETE /api/v1/projects/:projectId/files/:fileId":       permBase,
	"PATCH /api/v1/projects/:projectId/files/:fileId/rename": permBase,
	"PATCH /api/v1/projects/:projectId/files/:fileId/move":   permBase,
	"POST /api/v1/projects/:projectId/files/batch-delete":    permBase,

	// ── 会议（参与属基础权限；开始/结束是管理动作） ──
	"POST /api/v1/projects/:projectId/meetings": permBase,
	"GET /api/v1/projects/:projectId/meetings":  permBase,
	"GET /api/v1/meetings/:id":                  permBase,
	"POST /api/v1/meetings/:id/messages":        permBase,
	"GET /api/v1/meetings/:id/messages":         permBase,
	"POST /api/v1/meetings/:id/start":           authz.PermMeetingManage,
	"POST /api/v1/meetings/:id/end":             authz.PermMeetingManage,
	"POST /api/v1/meetings/:id/todos":           permBase,

	// ── 任务（日常协作大部分属基础权限；创建与派发显式管控） ──
	"POST /api/v1/projects/:projectId/tasks":              authz.PermTaskCreate,
	"POST /api/v1/projects/:projectId/tasks/planning":     authz.PermTaskCreate,
	"POST /api/v1/projects/:projectId/tasks/from-text":    authz.PermTaskCreate,
	"GET /api/v1/projects/:projectId/tasks":               permBase,
	"GET /api/v1/tasks/:id":                               permBase,
	"GET /api/v1/tasks/:id/events":                        permBase,
	"POST /api/v1/tasks/:id/messages":                     permBase,
	"POST /api/v1/tasks/:id/approve":                      permBase,
	"POST /api/v1/tasks/:id/reject":                       permBase,
	"POST /api/v1/tasks/:id/cancel":                       permBase,
	"POST /api/v1/tasks/:id/todos":                        permBase,
	"POST /api/v1/tasks/:id/todos/:todoId/insert":         permBase,
	"PATCH /api/v1/tasks/:id/todos/:todoId":               permBase,
	"DELETE /api/v1/tasks/:id/todos/:todoId":              permBase,
	"PUT /api/v1/tasks/:id/todos/reorder":                 permBase,
	"POST /api/v1/tasks/:id/todos/:todoId/dispatch":       authz.PermTaskDispatch,
	"POST /api/v1/tasks/:id/todos/:todoId/outputs/bind":   permBase,
	"POST /api/v1/tasks/:id/todos/:todoId/review":         permBase,
	"POST /api/v1/tasks/:id/todos/:todoId/reopen":         permBase,
	"POST /api/v1/tasks/:id/todos/:todoId/answer":         permBase,
	"GET /api/v1/tasks/:id/comments":                      permBase,
	"POST /api/v1/tasks/:id/comments":                     permBase,
	"GET /api/v1/action-items":                            permBase,
	"POST /api/v1/action-items/convert":                   authz.PermTaskCreate,
	"GET /api/v1/tasks/:id/artifacts/:artifactId/content": permBase,

	// ── 仪表盘 / 通知 / 实时流 / 节点健康 ──
	"GET /api/v1/dashboard/stats":              permBase,
	"GET /api/v1/dashboard/events":             permBase,
	"GET /api/v1/dashboard/tasks":              permBase,
	"GET /api/v1/agents/:id/events":            permBase,
	"GET /api/v1/clawsynapse/health":           permBase,
	"GET /api/v1/notifications":                permBase,
	"GET /api/v1/notifications/unread-count":   permBase,
	"PATCH /api/v1/notifications/:id/read":     permBase,
	"POST /api/v1/notifications/mark-all-read": permBase,
	"GET /api/v1/events/stream":                permBase,
	"POST /api/v1/chats/attachments":           permBase,

	// ── 外部应用（浏览与日常使用属基础权限） ──
	// 三级作用域后「能否管理」取决于应用层级（数据相关），仍由 handler + store 裁决，
	// 路由层刻意保持 permBase（全局级走上面的平台命名空间）。
	"POST /api/v1/external-apps":            permBase,
	"GET /api/v1/external-apps":             permBase,
	"GET /api/v1/external-apps/:id":         permBase,
	"PATCH /api/v1/external-apps/:id":       permBase,
	"DELETE /api/v1/external-apps/:id":      permBase,
	"POST /api/v1/external-apps/:id/launch": permBase,

	// ── 运维工单（member 不可见） ──
	"GET /api/v1/ops/incidents":             authz.PermOpsView,
	"GET /api/v1/ops/incidents/:id":         authz.PermOpsView,
	"POST /api/v1/ops/incidents/:id/ignore": authz.PermOpsManage,
	"POST /api/v1/ops/incidents/:id/close":  authz.PermOpsManage,
	"GET /api/v1/ops/metrics":               authz.PermOpsView, // handler 另保留企业租户上下文门禁（跨租户聚合）

	// ── 账号自助 ──
	"GET /api/v1/users/me":           permBase,
	"PATCH /api/v1/users/me":         permBase,
	"POST /api/v1/users/me/password": permBase,

	// ── 组织（浏览/创建属基础权限；成员管理显式管控） ──
	"GET /api/v1/organizations":                        permBase,
	"POST /api/v1/organizations":                       permBase,
	"GET /api/v1/organizations/:id":                    permBase,
	"GET /api/v1/organizations/:id/members":            permBase,
	"POST /api/v1/organizations/:id/members":           authz.PermOrgMemberMgr,
	"PATCH /api/v1/organizations/:id/members/:userId":  authz.PermOrgMemberMgr,
	"DELETE /api/v1/organizations/:id/members/:userId": authz.PermOrgMemberMgr,
	"PATCH /api/v1/organizations/:id/menu-overrides":   authz.PermOrgSettings, // 菜单覆盖=企业设置项（owner）
	// 组织资料（名称/简称）与 logo：**owner 或内置 admin** 可改（handler 内
	// ownerOrAdminGuard），logo 读取仅成员可见。★ 刻意不标 PermOrgSettings：
	// 该权限点是 owner 独有（authz/role.go ownerPerms，authz_test.go 断言 admin
	// 不得拥有），标上去会把 admin 挡在路由层、与 handler 的裁决自相矛盾。
	// 与 /organizations/:id/llm-config 三条同款：路由层 base，细粒度守卫在 handler。
	"PATCH /api/v1/organizations/:id":     permBase,
	"POST /api/v1/organizations/:id/logo": permBase,
	"GET /api/v1/organizations/:id/logo":  permBase,

	// ── 企业角色（设计文档 §5：列表供改角色下拉使用，增删改仅 owner） ──
	"GET /api/v1/organizations/:id/roles":            authz.PermOrgMemberMgr,
	"POST /api/v1/organizations/:id/roles":           authz.PermOrgRoleMgr,
	"PATCH /api/v1/organizations/:id/roles/:roleId":  authz.PermOrgRoleMgr,
	"DELETE /api/v1/organizations/:id/roles/:roleId": authz.PermOrgRoleMgr,

	// ── 知识库（浏览/检索属基础权限） ──
	"POST /api/v1/knowledge/documents":               authz.PermKnowledgeManage,
	"GET /api/v1/knowledge/documents":                permBase,
	"GET /api/v1/knowledge/documents/:id":            permBase,
	"PATCH /api/v1/knowledge/documents/:id":          authz.PermKnowledgeManage,
	"DELETE /api/v1/knowledge/documents/:id":         authz.PermKnowledgeManage,
	"GET /api/v1/knowledge/documents/:id/chunks":     permBase,
	"POST /api/v1/knowledge/documents/:id/reprocess": authz.PermKnowledgeManage,
	"POST /api/v1/knowledge/search":                  permBase,

	// ── 岗位市场（条件注册：依赖 roles_index.json） ──
	"GET /api/v1/market/departments": authz.PermMarketBrowse,
	"GET /api/v1/market/roles":       authz.PermMarketBrowse,
	"GET /api/v1/market/roles/:id":   authz.PermMarketBrowse,

	// ── 平台命名空间（只认平台管理员标记；企业角色一律 403） ──
	"GET /api/v1/platform/orgs":              authz.PermPlatformOrgLifecycle,
	"POST /api/v1/platform/orgs":             authz.PermPlatformOrgLifecycle,
	"GET /api/v1/platform/orgs/:id":          authz.PermPlatformOrgLifecycle,
	"POST /api/v1/platform/orgs/:id/disable": authz.PermPlatformOrgLifecycle,
	"POST /api/v1/platform/orgs/:id/restore": authz.PermPlatformOrgLifecycle,
	"GET /api/v1/platform/config":            authz.PermPlatformConfigRead,
	"PUT /api/v1/platform/config":            authz.PermPlatformConfigWrite,
	"GET /api/v1/platform/audit-logs":        authz.PermPlatformAuditView,
	"GET /api/v1/platform/usage":             authz.PermPlatformUsageView,
	"GET /api/v1/platform/llm-config":        authz.PermPlatformConfigRead,
	"PUT /api/v1/platform/llm-config":        authz.PermPlatformConfigWrite,
	"DELETE /api/v1/platform/llm-config":     authz.PermPlatformConfigWrite,
	// 全局级外部应用：平台管理员独有的产品挂载配置（三级作用域 2026-09-17）。
	"GET /api/v1/platform/external-apps":        authz.PermPlatformExtAppMgr,
	"POST /api/v1/platform/external-apps":       authz.PermPlatformExtAppMgr,
	"PATCH /api/v1/platform/external-apps/:id":  authz.PermPlatformExtAppMgr,
	"DELETE /api/v1/platform/external-apps/:id": authz.PermPlatformExtAppMgr,
	// 平台用户管理：查看（列表/详情/企业成员）与账号运维动作分两个权限点。
	"GET /api/v1/platform/orgs/:id/members":          authz.PermPlatformOrgLifecycle,
	"GET /api/v1/platform/users":                     authz.PermPlatformUserRead,
	"GET /api/v1/platform/users/:id":                 authz.PermPlatformUserRead,
	"POST /api/v1/platform/users/:id/reset-password": authz.PermPlatformUserManage,
	"POST /api/v1/platform/users/:id/disable":        authz.PermPlatformUserManage,
	"POST /api/v1/platform/users/:id/enable":         authz.PermPlatformUserManage,
	// 桌面端发行版（自建更新源，docs/desktop-app-update-plan-2026-09-18.md）。
	// 四个动作共用一个权限点：它们都能改变「全员拿到哪个版本」，同档破坏力。
	"GET /api/v1/platform/desktop-releases":               authz.PermPlatformDesktopRelease,
	"POST /api/v1/platform/desktop-releases":              authz.PermPlatformDesktopRelease,
	"POST /api/v1/platform/desktop-releases/:id/publish":  authz.PermPlatformDesktopRelease,
	"POST /api/v1/platform/desktop-releases/:id/rollback": authz.PermPlatformDesktopRelease,
	"DELETE /api/v1/platform/desktop-releases/:id":        authz.PermPlatformDesktopRelease,

	// ── LLM 助手与配置 ──
	// 租户层 LLM 配置在 handler 层已有细粒度守卫（租户层 owner-admin / 个人层本人），
	// 路由层记 base；平台层的三条已收敛到上面的平台命名空间。
	"POST /api/v1/assistant/chat":                 permBase,
	"GET /api/v1/organizations/:id/llm-config":    permBase,
	"PUT /api/v1/organizations/:id/llm-config":    permBase,
	"DELETE /api/v1/organizations/:id/llm-config": permBase,
	"POST /api/v1/llm-config/test":                permBase,
	"POST /api/v1/llm-config/models":              permBase,
	"GET /api/v1/llm-config/personal":             permBase,
	"PUT /api/v1/llm-config/personal":             permBase,
	"DELETE /api/v1/llm-config/personal":          permBase,
}

// publicRoutes 无需登录（不走 RequireAuth），不参与权限点声明。
var publicRoutes = map[string]bool{
	"GET /healthz":               true,
	"GET /webhook/clawsynapse":   true,
	"POST /webhook/clawsynapse":  true,
	"POST /api/v1/auth/register": true,
	"POST /api/v1/auth/login":    true,
	"POST /api/v1/auth/refresh":  true,
	"GET /api/v1/platform/info":  true,
	// 短时效下载 token 鉴权（非 JWT），语义上公开。
	"GET /api/v1/files/agent/:fileId/token/:token":       true,
	"GET /api/v1/files/agent/:fileId":                    true,
	"GET /api/v1/debug/gen-token/:fileId":                true,
	"GET /api/v1/chats/attachments/:fileId/token/:token": true,
	// 条件注册（market）。
	"GET /api/v1/market/roles/:id/download": true,
	// 桌面端更新 feed（docs/desktop-app-update-plan-2026-09-18.md §4）：
	// **刻意公开**。electron-updater 跑在独立 session 分区，不携带登录 cookie，
	// 要求鉴权会让「登出已久」的机器静默地永远升不了级（方案 §1 决策 6）。
	"GET /api/v1/desktop/releases/feed/latest.yml": true,
	"GET /api/v1/desktop/releases/feed/:filename":  true,
}

// conditionalRoutes 按需注册（当前仅 market 系列依赖 roles_index.json 存在），
// 测试环境未注册时不要求出现在路由表中。
var conditionalRoutes = map[string]bool{
	"GET /api/v1/market/departments":        true,
	"GET /api/v1/market/roles":              true,
	"GET /api/v1/market/roles/:id":          true,
	"GET /api/v1/market/roles/:id/download": true,
}

// TestRoutePermManifestCoversAllRoutes 双向核对路由表与权限清单：
//  1. 路由表中每条非公开路由必须在清单中有显式声明（漏标即失败）；
//  2. 清单中的每条路由必须真实注册（或属条件注册），防止清单与路由表漂移。
func TestRoutePermManifestCoversAllRoutes(t *testing.T) {
	log, err := logger.New("error")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer func() { _ = log.Sync() }()

	application, err := New(config.Config{
		Port:            "0",
		JWTSecret:       "test-secret",
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: 168 * time.Hour,
		LogLevel:        "error",
		AllowAllCORS:    true,
		ReadTimeout:     3 * time.Second,
		ShutdownGrace:   3 * time.Second,
	}, log)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			t.Fatalf("close app: %v", closeErr)
		}
	}()

	registered := map[string]bool{}
	for _, r := range application.Engine.Routes() {
		key := r.Method + " " + r.Path
		registered[key] = true
		if publicRoutes[key] {
			continue
		}
		if _, ok := routePermManifest[key]; !ok {
			t.Errorf("路由 %s 未在 routePermManifest 声明权限点（或 base）", key)
		}
	}
	for key := range routePermManifest {
		if !registered[key] && !conditionalRoutes[key] {
			t.Errorf("清单声明了 %s 但路由表中不存在（清单与路由表漂移）", key)
		}
	}
	for key := range publicRoutes {
		if !registered[key] && !conditionalRoutes[key] {
			t.Errorf("公开路由 %s 在路由表中不存在（清单与路由表漂移）", key)
		}
	}
}

// TestRoutePermManifestValuesValid 校验清单值的合法性：
// 权限点必须是 authz 包已定义的常量，防止手写字符串 typo 静默失效。
func TestRoutePermManifestValuesValid(t *testing.T) {
	valid := map[string]bool{permBase: true}
	for _, p := range authz.AllOrgPermissions() {
		valid[p] = true
	}
	for _, p := range authz.PlatformPermissions() {
		valid[p] = true
	}
	for key, perm := range routePermManifest {
		if !valid[perm] {
			t.Errorf("路由 %s 声明了未知权限点 %q", key, perm)
		}
	}
}
