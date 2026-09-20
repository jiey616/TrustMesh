package app

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"trustmesh/backend/internal/agentfile"
	"trustmesh/backend/internal/assistant"
	"trustmesh/backend/internal/auth"
	"trustmesh/backend/internal/authz"
	"trustmesh/backend/internal/clawsynapse"
	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/embedding"
	"trustmesh/backend/internal/handler"
	"trustmesh/backend/internal/knowledge"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/project"
	"trustmesh/backend/internal/store"
)

type App struct {
	Engine             *gin.Engine
	Store              *store.Store
	PeerSyncer         *clawsynapse.PeerSyncer
	TrustRequestSyncer *clawsynapse.TrustRequestSyncer
}

func New(cfg config.Config, log *zap.Logger) (*App, error) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(middleware.Recovery(log))
	engine.Use(middleware.Logging(log))
	engine.Use(middleware.CORS(cfg.AllowAllCORS))
	engine.Use(middleware.RateLimit(log))

	s, err := store.NewWithConfig(cfg, log)
	if err != nil {
		return nil, err
	}
	jwtManager := auth.NewJWTManager(cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	clawClient := clawsynapse.NewClient(cfg.ClawSynapseAPIURL, cfg.ClawSynapseTimeout, cfg.ClawSynapseAPIToken)
	// LLM 配置（A1+B2+C1）：env 兜底注入 + 平台/租户两级 UI 配置，热生效。
	s.SetLLMEnvDefaults(cfg.AssistantAPIURL, cfg.AssistantAPIKey, cfg.AssistantModel)
	// 平台管理员账号集合（设计文档 §6.3）：PLATFORM_ADMIN_EMAILS 非空 = 种子模式
	// （账号集合以 env 为唯一权威，未命中者撤销标记）；未配置时回落历史行为
	// （首个注册用户自动提升），避免锁死存量环境。
	s.SeedPlatformAdmins(cfg.PlatformAdminEmails)
	// 企业角色（设计文档 §5）：内置角色权限模板从 authz 矩阵注入（store 不反向依赖
	// authz），随后跑一次幂等迁移 —— 给存量租户补种内置角色 + 回填 membership.role_id。
	// 必须早于任何请求：解析链依赖 role_id 与 org_roles 的存在性，晚于此窗口的请求
	// 会走「内置矩阵回落」分支（结果一致，但没必要让它发生）。
	s.SetBuiltinRoleTemplates(map[string][]string{
		model.OrgRoleOwner:  authz.BuiltinPermissions(model.OrgRoleOwner, cfg.PermLegacyMember),
		model.OrgRoleAdmin:  authz.BuiltinPermissions(model.OrgRoleAdmin, cfg.PermLegacyMember),
		model.OrgRoleMember: authz.BuiltinPermissions(model.OrgRoleMember, cfg.PermLegacyMember),
	})
	s.MigrateOrgRoles()
	llmProvider := assistant.NewLLMProvider(func(orgID, userID string) (assistant.LLMParams, bool) {
		url, key, _, opsModel, source := s.ResolveLLMParams(orgID, userID)
		if url == "" || key == "" {
			return assistant.LLMParams{}, false
		}
		return assistant.LLMParams{
			APIURL: url, APIKey: key, Model: opsModel, OpsModel: opsModel, Source: source,
		}, true
	})
	// LLM 归因（F）：按工单归属租户解析配置（B2）；不可用时优雅降级为
	// 模板指引并标记需人工复核，规则引擎与工单不受影响。
	s.SetOpsAttributionHook(assistant.OpsAttributor(llmProvider))
	peerSyncer := clawsynapse.NewPeerSyncer(clawClient, s, cfg.ClawSynapsePeerSync, log)
	if peerSyncer != nil {
		peerSyncer.Start()
	}
	trustRequestSyncer := clawsynapse.NewTrustRequestSyncer(clawClient, s, cfg.ClawSynapsePeerSync, log)
	if trustRequestSyncer != nil {
		trustRequestSyncer.Start()
	}

	// 权限体系（docs/permission-system-design-2026-09-16.md）：功能级鉴权入口。
	// 权限集每请求实时解析（membership.role_id → org_roles → 内置矩阵回落），
	// 禁止跨请求进程内缓存 —— T3.1 多实例下角色变更必须即时生效。
	az := authz.NewAuthorizer(authz.NewStoreResolver(s, cfg.PermLegacyMember))
	// platformAdminChecker 供平台命名空间门禁（/api/v1/platform/*）与业务 API 反向
	// 门禁（authed 组）共用 —— authz 不反向依赖 store，故由 app 层注入。
	platformAdminChecker := func(userID string) bool { return s.UserIsPlatformAdmin(userID) }
	// 种子模式下（PLATFORM_ADMIN_EMAILS 已配置）平台管理员与业务账号严格分离：
	// 调业务 API 一律 403（账号/会话类白名单除外）。未配置 env 的环境不做反向拒绝，
	// 避免把历史上自动提升的管理员锁在自己的业务之外。
	platformSeparationStrict := len(cfg.PlatformAdminEmails) > 0

	authHandler := handler.NewAuthHandler(s, jwtManager)
	userHandler := handler.NewUserHandler(s, az)
	agentHandler := handler.NewAgentHandler(s, clawClient)
	agentChatHandler := handler.NewAgentChatHandler(s, clawClient, cfg.ExternalURL, []byte(cfg.JWTSecret), cfg.DownloadTokenTTL, log)
	projectHandler := handler.NewProjectHandler(s)

	actionItemsHandler := handler.NewActionItemsHandler(s, log)
	transferHandler := handler.NewTransferHandler(s)
	dashboardHandler := handler.NewDashboardHandler(s)
	clawSynapseHandler := handler.NewClawSynapseHandler(clawClient)
	notificationHandler := handler.NewNotificationHandler(s)
	joinRequestHandler := handler.NewJoinRequestHandler(s, clawClient, cfg)
	realtimeHandler := handler.NewRealtimeHandler(s)
	platformHandler := handler.NewPlatformHandler(cfg.PlatformName)
	externalAppHandler := handler.NewExternalAppHandler(s, az, auth.ExternalTokenIssuer, cfg.ExternalAppTokenTTL)
	workflowTemplateHandler := handler.NewWorkflowTemplateHandler(s)
	// orgHandler 需注入文件存储（组织 logo 上传/直出），在 projectFileStorage 就绪后赋值（见下文）。
	var orgHandler *handler.OrgHandler
	orgRoleHandler := handler.NewOrgRoleHandler(s, cfg.PermLegacyMember)

	// Knowledge base components (optional - requires EMBEDDING_API_KEY)
	var knowledgeHandler *handler.KnowledgeHandler
	var qdrantClient *knowledge.QdrantClient
	var embeddingClient embedding.Client
	fileStorage := knowledge.NewLocalFileStorage(cfg.KnowledgeStorePath)

	if cfg.EmbeddingAPIKey != "" {
		var err error
		embeddingClient, err = embedding.NewClient(cfg)
		if err != nil {
			log.Warn("embedding client init failed, knowledge search disabled", zap.Error(err))
		}
		qdrantClient = knowledge.NewQdrantClient(cfg.QdrantURL, cfg.EmbeddingDimension)
		if err := qdrantClient.EnsureCollection(context.Background()); err != nil {
			log.Warn("qdrant collection init failed, knowledge search disabled", zap.Error(err))
			qdrantClient = nil
		}
	}

	// Knowledge base components. Processor is created regardless of embedding
	// availability: without an embedding key documents are still chunked and
	// stored, so text search works; vector search activates once a key is set.
	var processor *knowledge.Processor
	processor = knowledge.NewProcessor(fileStorage, embeddingClient, qdrantClient, s, log)
	knowledgeHandler = handler.NewKnowledgeHandler(s, fileStorage, processor, embeddingClient, qdrantClient, log)

	engine.GET("/healthz", handler.Health)
	engine.GET("/webhook/clawsynapse", func(c *gin.Context) { c.Status(200) })

	v1 := engine.Group("/api/v1")
	v1.POST("/auth/register", authHandler.Register)
	v1.POST("/auth/login", authHandler.Login)
	v1.POST("/auth/refresh", authHandler.Refresh)

	// Market public download endpoint for external installers like OpenClaw.
	// Listing/detail APIs remain authenticated.
	if marketStore, err := store.NewMarketStore(cfg.MarketDataPath); err != nil {
		log.Warn("market store init failed, market disabled", zap.Error(err))
	} else {
		marketHandler := handler.NewMarketHandler(marketStore)
		v1.GET("/market/roles/:id/download", marketHandler.DownloadRole)
	}

	v1.GET("/platform/info", platformHandler.Info)

	// 桌面端更新 feed：**公开只读，不走鉴权**（方案 §1 决策 6）。
	// electron-updater 跑在独立 session 分区 `electron-updater`，不共享登录 cookie；
	// 一旦要求登录，「登出已久 / 从未登录」的机器就永远升不了级，而且这是静默的
	// （用户只会看到「没有更新」）。latest.yml 由当前 published 记录**动态生成**，
	// 不接受上传者提供的版本，保证「发布指针」是唯一真相来源。
	// handler 与下面平台命名空间的 /platform/desktop-releases 共用同一个实例。
	platformDesktopReleaseHandler := handler.NewPlatformDesktopReleaseHandler(s)
	desktopFeed := v1.Group("/desktop/releases/feed")
	desktopFeed.GET("/latest.yml", platformDesktopReleaseHandler.LatestYML)
	desktopFeed.GET("/:filename", platformDesktopReleaseHandler.DownloadReleaseFile)

	// 平台「使用引导」：单篇全局 HTML（登录用户读 /guide；管理在 /platform/guide）。
	platformGuideHandler := handler.NewPlatformGuideHandler(s)

	// 移动端安装包（Android APK）：公开 meta/下载（扫码即下，未登录场景）；
	// 管理在 /platform/mobile-app（上传即覆盖）。
	mobileAppReleaseHandler := handler.NewMobileAppReleaseHandler(s)
	v1.GET("/mobile/app/latest", mobileAppReleaseHandler.Latest)
	v1.GET("/mobile/app/download", mobileAppReleaseHandler.Download)

	authed := v1.Group("")
	authed.Use(middleware.RequireAuth(jwtManager))
	// 多租户阶段 0：解析 X-Org-Id 并注入 Scope。存量客户端不带该头，
	// 中间件直接放行（只带 UserID），行为与改造前完全一致。
	authed.Use(middleware.OrgScope(s))
	// 平台管理员的业务 API 反向门禁（设计文档 §6.3）：种子模式下平台管理员只能走
	// /api/v1/platform/*，业务路由一律 403；账号/会话类外壳能力（/users、/organizations、
	// /notifications、/events、/llm-config）仍放行。未配置 env 时恒放行（见上文）。
	authed.Use(authz.RequireBusinessAccount(platformSeparationStrict, platformAdminChecker))

	// 平台「使用引导」：全员基础权限（登录即可读），无角色权限点。
	authed.GET("/guide", platformGuideHandler.Get)

	authed.POST("/agents", az.RequirePerm(authz.PermAgentManage), agentHandler.Create)
	authed.GET("/agents", az.RequirePerm(authz.PermAgentView), agentHandler.List)
	authed.GET("/agents/invite-prompt", joinRequestHandler.GetInvitePrompt)
	authed.GET("/agents/join-requests", az.RequirePerm(authz.PermJoinRequestApprove), joinRequestHandler.List)
	authed.POST("/agents/join-requests/:id/approve", az.RequirePerm(authz.PermJoinRequestApprove), joinRequestHandler.Approve)
	authed.POST("/agents/join-requests/:id/reject", az.RequirePerm(authz.PermJoinRequestApprove), joinRequestHandler.Reject)
	authed.GET("/agents/:id", az.RequirePerm(authz.PermAgentView), agentHandler.Get)
	authed.PATCH("/agents/:id", az.RequirePerm(authz.PermAgentManage), agentHandler.Update)
	authed.DELETE("/agents/:id", az.RequirePerm(authz.PermAgentManage), agentHandler.Delete)
	authed.GET("/agents/:id/stats", az.RequirePerm(authz.PermAgentView), agentHandler.Stats)
	authed.GET("/agents/:id/insights", az.RequirePerm(authz.PermAgentView), agentHandler.Insights)
	authed.GET("/agents/:id/tasks", az.RequirePerm(authz.PermAgentView), agentHandler.Tasks)
	authed.GET("/agents/:id/capabilities", az.RequirePerm(authz.PermAgentView), agentHandler.GetCapabilities)
	authed.POST("/agents/:id/capabilities", az.RequirePerm(authz.PermAgentManage), agentHandler.SetCapabilities)
	authed.GET("/agents/:id/cron/executions", az.RequirePerm(authz.PermAgentView), agentHandler.ListCronExecutions)
	authed.POST("/agents/:id/skills/upload", az.RequirePerm(authz.PermAgentManage), agentHandler.UploadSkillFile)
	// agent 对话是 AI办公室日常基础交互，属基础权限（登录即可用）。
	authed.GET("/agents/:id/chat", agentChatHandler.Get)
	authed.GET("/agents/:id/chat/sessions", agentChatHandler.ListSessions)
	authed.GET("/agents/:id/chat/sessions/:sessionId", agentChatHandler.GetSession)
	authed.POST("/agents/:id/chat/messages", agentChatHandler.SendMessage)
	authed.POST("/agents/:id/chat/reset", agentChatHandler.Reset)

	authed.POST("/projects", az.RequirePerm(authz.PermProjectCreate), projectHandler.Create)
	authed.GET("/projects", projectHandler.List)
	authed.GET("/projects/:projectId", projectHandler.Get)
	authed.GET("/projects/:projectId/workflow-progress", projectHandler.WorkflowProgress)
	// 项目流程 · 手工绑定交付物：任意文件（含用户手工上传的）→ 任意步骤输出位。
	authed.POST("/projects/:projectId/workflow/steps/:stepIndex/outputs/bind", projectHandler.BindStepOutput)
	authed.PATCH("/projects/:projectId", az.RequirePerm(authz.PermProjectManage), projectHandler.Update)
	authed.DELETE("/projects/:projectId", az.RequirePerm(authz.PermProjectManage), projectHandler.Archive)

	// Global workflow templates (user-scoped) + project inherit/sync.
	// 模板的浏览（List/Get/sync-diff 只读预览）属基础权限；增删改与同步动作走
	// workflow.template.mgr（owner/admin）。
	authed.POST("/workflow-templates", az.RequirePerm(authz.PermWorkflowTemplateMgr), workflowTemplateHandler.Create)
	authed.GET("/workflow-templates", workflowTemplateHandler.List)
	authed.GET("/workflow-templates/:templateId", workflowTemplateHandler.Get)
	authed.PATCH("/workflow-templates/:templateId", az.RequirePerm(authz.PermWorkflowTemplateMgr), workflowTemplateHandler.Update)
	authed.POST("/workflow-templates/:templateId/copy", az.RequirePerm(authz.PermWorkflowTemplateMgr), workflowTemplateHandler.Copy)
	authed.POST("/workflow-templates/:templateId/curate", az.RequirePerm(authz.PermWorkflowTemplateMgr), workflowTemplateHandler.Curate)
	authed.DELETE("/workflow-templates/:templateId", az.RequirePerm(authz.PermWorkflowTemplateMgr), workflowTemplateHandler.Delete)
	// T1.9: 从成功任务一键沉淀工作流模板（挂在 /tasks/:id 下，避开
	// /workflow-templates/:templateId 通配段的兄弟节点冲突）。
	authed.POST("/tasks/:id/distill-template", az.RequirePerm(authz.PermWorkflowTemplateMgr), workflowTemplateHandler.Distill)
	authed.POST("/projects/:projectId/workflows/inherit", az.RequirePerm(authz.PermWorkflowTemplateMgr), workflowTemplateHandler.Inherit)
	authed.GET("/projects/:projectId/workflows/:workflowId/sync-diff", workflowTemplateHandler.SyncDiff)
	authed.POST("/projects/:projectId/workflows/:workflowId/sync", az.RequirePerm(authz.PermWorkflowTemplateMgr), workflowTemplateHandler.ApplySync)
	authed.POST("/projects/:projectId/workflows/:workflowId/detach", az.RequirePerm(authz.PermWorkflowTemplateMgr), workflowTemplateHandler.Detach)

	// Project files
	projectFileStorage := project.NewLocalFileStorage(cfg.FilesStoragePath)
	// 组织 handler 注入文件存储 / 基础路径 / 外部地址，供 logo 上传与成员鉴权直出使用。
	orgHandler = handler.NewOrgHandler(s, projectFileStorage, cfg.FilesStoragePath, cfg.ExternalURL)
	s.SetFileStorage(projectFileStorage) // used by timeout monitor to auto-generate meeting minutes
	projectFileHandler := handler.NewProjectFileHandler(s, projectFileStorage, log)

	// Agent file download endpoint (authenticated by short-lived download token, not JWT).
	// New format: /api/v1/files/agent/:fileId/token/:token (token in path, not query).
	// Old format: /api/v1/files/agent/:fileId?token=xxx (kept for compatibility).
	agentFileHandler := agentfile.NewHandler(s, projectFileStorage, jwtManager, log)
	v1.GET("/files/agent/:fileId/token/:token", agentFileHandler.Download)
	v1.GET("/files/agent/:fileId", agentFileHandler.Download)
	v1.GET("/debug/gen-token/:fileId", agentFileHandler.DebugGenToken)

	// Chat attachment upload/download (数字员工 + 任务对话共用).
	chatAttachmentHandler := handler.NewChatAttachmentHandler(projectFileStorage, cfg.FilesStoragePath, jwtManager, []byte(cfg.JWTSecret), cfg.DownloadTokenTTL, cfg.ExternalURL, log)
	authed.POST("/chats/attachments", chatAttachmentHandler.Upload)
	v1.GET("/chats/attachments/:fileId/token/:token", chatAttachmentHandler.Download)

	meetingHandler := handler.NewMeetingHandler(s, clawClient, projectFileStorage, cfg.ExternalURL, []byte(cfg.JWTSecret), cfg.DownloadTokenTTL)

	// Resume the inactivity watchdog for any meeting left in_progress by a
	// previous backend instance (timers are in-memory only).
	go meetingHandler.RecoverTimeouts(context.Background())

	// Webhook handler: every dependency is injected at construction so its
	// configuration is immutable once the server starts serving. Previously
	// these fields were assigned via post-construction setters
	// (SetKnowledgeComponents / SetProjectFileStorage / SetAgentFileConfig /
	// SetMeetingActivityNotifier), which raced with concurrent request handlers
	// reading the same fields (T0.10). Construction therefore happens only after
	// every dependency below is ready: knowledge (embeddingClient, qdrantClient),
	// project file storage, and the meeting handler.
	webhookHandler := clawsynapse.NewWebhookHandler(clawsynapse.WebhookDeps{
		Store:              s,
		Client:             clawClient,
		Log:                log,
		Embedder:           embeddingClient,
		Qdrant:             qdrantClient,
		ProjectFileStorage: projectFileStorage,
		ExternalURL:        cfg.ExternalURL,
		JWTSecret:          []byte(cfg.JWTSecret),
		DownloadTTL:        cfg.DownloadTokenTTL,
		OnMeetingActivity:  meetingHandler.OnMeetingActivity,
	})
	// Redispatch hook: actually re-publish a todo to its assignee. Consumed by
	// the dispatch reconciler below to heal pipelines broken by a silently
	// failed dispatch (P-01). Kept as a hook because the store must not import
	// the clawsynapse package.
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {
		if err := webhookHandler.RetryDispatch(ctx, taskID, todoID); err != nil {
			log.Warn("dispatch reconcile failed",
				zap.String("task_id", taskID), zap.String("todo_id", todoID), zap.Error(err))
		}
	})
	// Timeout reminders nudge the assignee without re-dispatching the todo.
	// C.2 统一干预编排：remind 先经编排器留痕（进运维工单时间线），再走
	// 原有下发；计时与判死仍在 timeout_monitor，编排器不改变其语义。
	s.SetRemindHook(func(ctx context.Context, taskID, todoID string) {
		s.RecordTimeoutRemind(taskID, todoID)
		webhookHandler.RemindTodo(ctx, taskID, todoID)
	})
	// Planning-stall nudges wake the PM via task.message when a task has been
	// stuck in planning without a finalized plan.
	s.SetPlanningStallHook(webhookHandler.NudgePlanningPM)
	// Adapter lifecycle cancel chain: after CancelTask, notify each canceled
	// todo's assignee node so it stops the in-flight run (spec §6).
	s.SetCancelNotifyHook(webhookHandler.NotifyTaskCanceled)
	// 统一干预编排器：运维修复指引的唯一下发出口（C.2）。
	s.SetOpsPublishHook(webhookHandler.PublishOpsMention)

	// 启动顺序不变量（T0.10b）：所有 Set*Hook 必须在下面这些后台 ticker 启动
	// 之前完成。hook 字段（dispatchHook / remindHook / planningStallHook /
	// cancelNotifyHook / opsPublishHook / opsAttributionHook）的读写都是无锁的，
	// 其安全性依赖 Go 的「goroutine 创建 happens-before」语义：只要写在启动
	// 读线程的 `go` 语句之前完成，就无需给 hook 加锁（cancelNotifyHook 在请求
	// 路径读取，请求天然 happens-after New() 返回，同样无锁安全）。切勿把任何
	// Set*Hook 下移到这些 Start* 之后，否则会重新引入读写竞态。
	//
	// Start background cleanup ticker to prevent unbounded memory growth (OOM).
	// 无外部副作用（只清本进程内存 map），故**不受 leader 门禁**：每个实例各自回收
	// 自己的内存才是正确的。
	go s.StartCleanupTicker(context.Background())
	// T3.1：后台循环 leader 选举。仅当 LEADER_ELECTION_ENABLED=1 且 Mongo 可用时启动
	// （s.LeaderElectionArmed() 判定）。必须在下面三个受门禁 ticker 首轮触发之前启动，
	// 使其 isLeader 尽早确立。单实例默认关闭 → 不启动，三 ticker 恒跑（现状不变）。
	if s.LeaderElectionArmed() {
		go s.StartLeaderElection(context.Background())
	}
	// T3.1 W2：SSE 跨实例广播（Mongo outbox + tailer）。仅当 SSE_BROADCAST_ENABLED=1 且
	// Mongo 可用时启动（StartSSEBroadcast 内部自判 Armed）。关闭时事件只投本实例订阅者，
	// 行为与改造前逐字节一致。必须在请求开始流入前启动，让 tailer 游标尽早对齐。
	if s.SSEBroadcastArmed() {
		go s.StartSSEBroadcast(context.Background())
	}
	// Start timeout monitor to detect and retry/fail stuck in_progress todos.
	// Reads remindHook / planningStallHook — 必须在 SetRemindHook /
	// SetPlanningStallHook 之后启动。多实例下仅 leader 执行（见 StartTimeoutMonitor 内门禁）。
	go s.StartTimeoutMonitor(context.Background())
	// Ops scanner: rule-based anomaly discovery feeding ops_incident tickets.
	// Off by default; flip OPS_ENABLED=1 to enable. Reads opsPublishHook /
	// opsAttributionHook（经 attributeAndGuide）— 必须在对应 Set*Hook 之后启动。
	if cfg.OpsEnabled {
		go s.StartOpsScanner(context.Background())
	}
	// Dispatch reconciler: periodic safety net that re-dispatches pending todos
	// which should have been dispatched but were not (transient publish failure,
	// process restart, network partition). Runs outside the store lock. Reads
	// dispatchHook — 必须在 SetDispatchHook 之后启动。
	go s.StartDispatchReconciler(context.Background())

	taskHandler := handler.NewTaskHandler(s, clawClient, webhookHandler, cfg.ExternalURL, []byte(cfg.JWTSecret), cfg.DownloadTokenTTL, log)

	engine.POST("/webhook/clawsynapse", webhookHandler.HandleWebhook)

	authed.POST("/projects/:projectId/files", projectFileHandler.Upload)
	authed.POST("/projects/:projectId/folders", projectFileHandler.CreateFolder)
	authed.GET("/projects/:projectId/files/browse", projectFileHandler.Browse)
	authed.GET("/projects/:projectId/files/artifacts", projectFileHandler.ListArtifacts)
	authed.GET("/projects/:projectId/files/tree", projectFileHandler.GetTree)
	authed.GET("/projects/:projectId/files", projectFileHandler.List)
	authed.GET("/projects/:projectId/files/:fileId/content", projectFileHandler.GetContent)
	authed.DELETE("/projects/:projectId/files/:fileId", projectFileHandler.Delete)
	authed.PATCH("/projects/:projectId/files/:fileId/rename", projectFileHandler.Rename)
	authed.PATCH("/projects/:projectId/files/:fileId/move", projectFileHandler.Move)
	authed.POST("/projects/:projectId/files/batch-delete", projectFileHandler.BatchDelete)

	// Meeting Room routes。参与/发言属基础权限（日常协作）；
	// 会议生命周期（创建/开始/结束）统一走 meeting.manage：只读旁听仍对所有人开放，
	// 否则「能建会但开不了会」会留下永远 waiting 的僵尸会议（设计 §4 语义自洽）。
	authed.POST("/projects/:projectId/meetings", az.RequirePerm(authz.PermMeetingManage), meetingHandler.Create)
	authed.GET("/projects/:projectId/meetings", meetingHandler.List)
	authed.GET("/meetings/:id", meetingHandler.Get)
	authed.POST("/meetings/:id/messages", meetingHandler.SendMessage)
	authed.GET("/meetings/:id/messages", meetingHandler.ListMessages)
	authed.POST("/meetings/:id/start", az.RequirePerm(authz.PermMeetingManage), meetingHandler.Start)
	authed.POST("/meetings/:id/end", az.RequirePerm(authz.PermMeetingManage), meetingHandler.End)
	authed.POST("/meetings/:id/todos", meetingHandler.AddTodo)

	authed.POST("/projects/:projectId/tasks", az.RequirePerm(authz.PermTaskCreate), taskHandler.Create)
	authed.POST("/projects/:projectId/tasks/planning", az.RequirePerm(authz.PermTaskCreate), taskHandler.CreatePlanning)
	authed.POST("/projects/:projectId/tasks/from-text", az.RequirePerm(authz.PermTaskCreate), taskHandler.CreateFromText)
	authed.GET("/projects/:projectId/tasks", taskHandler.ListByProject)
	authed.GET("/tasks/:id", taskHandler.Get)
	authed.GET("/tasks/:id/events", taskHandler.ListEvents)
	authed.POST("/tasks/:id/messages", taskHandler.AppendTaskMessage)
	authed.POST("/tasks/:id/approve", taskHandler.ApprovePlan)
	authed.POST("/tasks/:id/reject", taskHandler.RejectPlan)
	authed.POST("/tasks/:id/cancel", taskHandler.Cancel)
	authed.POST("/tasks/:id/todos", taskHandler.AddTodo)
	authed.POST("/tasks/:id/todos/:todoId/insert", taskHandler.InsertTodo)
	authed.PATCH("/tasks/:id/todos/:todoId", taskHandler.UpdateTodo)
	authed.DELETE("/tasks/:id/todos/:todoId", taskHandler.RemoveTodo)
	authed.PUT("/tasks/:id/todos/reorder", taskHandler.ReorderTodos)
	authed.POST("/tasks/:id/todos/:todoId/dispatch", az.RequirePerm(authz.PermTaskDispatch), taskHandler.DispatchTodo)
	authed.POST("/tasks/:id/todos/:todoId/outputs/bind", taskHandler.BindTodoOutput)
	authed.POST("/tasks/:id/todos/:todoId/review", taskHandler.ReviewTodo)
	authed.POST("/tasks/:id/todos/:todoId/reopen", taskHandler.ReopenTodo)
	authed.POST("/tasks/:id/todos/:todoId/answer", taskHandler.AnswerTodo)
	authed.GET("/tasks/:id/comments", taskHandler.ListComments)
	authed.POST("/tasks/:id/comments", taskHandler.AddComment)
	authed.GET("/action-items", actionItemsHandler.List)
	// 待办转任务 = 建任务的一种入口，与 task.create 同级。
	authed.POST("/action-items/convert", az.RequirePerm(authz.PermTaskCreate), actionItemsHandler.Convert)
	authed.GET("/tasks/:id/artifacts/:artifactId/content", transferHandler.GetTaskArtifactContent)

	authed.GET("/dashboard/stats", dashboardHandler.Stats)
	authed.GET("/dashboard/events", dashboardHandler.RecentEvents)
	authed.GET("/dashboard/tasks", dashboardHandler.RecentTasks)
	authed.GET("/agents/:id/events", dashboardHandler.AgentEvents)

	authed.GET("/clawsynapse/health", clawSynapseHandler.Health)

	authed.GET("/notifications", notificationHandler.List)
	authed.GET("/notifications/unread-count", notificationHandler.UnreadCount)
	authed.PATCH("/notifications/:id/read", notificationHandler.MarkRead)
	authed.POST("/notifications/mark-all-read", notificationHandler.MarkAllRead)
	authed.GET("/events/stream", realtimeHandler.Stream)

	// External platform SSO "connect" registry + launch.
	// 六个路由的权限点保持基础权限：能否**管理**取决于应用的作用域（数据相关），
	// 路由级权限点表达不了 —— 由 handler（org.app.mgr 前置校验）+ store
	// （canManageExternalAppUnsafe）裁决；可见性由 store 按 global/org/personal 三级过滤。
	ext := authed.Group("/external-apps")
	ext.POST("", externalAppHandler.Create)
	ext.GET("", externalAppHandler.List)
	ext.GET("/:id", externalAppHandler.Get)
	ext.PATCH("/:id", externalAppHandler.Update)
	ext.DELETE("/:id", externalAppHandler.Delete)
	ext.POST("/:id/launch", externalAppHandler.Launch)

	// Ops incidents: rule-based anomaly tickets + manual intervention (ignore/close).
	// 权限：查看 ops.view / 人工干预 ops.manage（owner/admin；member 不可见）。
	opsHandler := handler.NewOpsHandler(s)
	ops := authed.Group("/ops")
	ops.GET("/incidents", az.RequirePerm(authz.PermOpsView), opsHandler.List)
	ops.GET("/incidents/:id", az.RequirePerm(authz.PermOpsView), opsHandler.Get)
	ops.POST("/incidents/:id/ignore", az.RequirePerm(authz.PermOpsManage), opsHandler.Ignore)
	ops.POST("/incidents/:id/close", az.RequirePerm(authz.PermOpsManage), opsHandler.Close)
	// Phase 0 observability export (T0.11): process-wide counters, histograms
	// (step-advance latency, stall duration) and the alert rule set.
	// 路由层 ops.view 之外，handler 仍保留「必须带企业租户上下文」的数据级门禁
	// （指标是跨租户进程级聚合，个人空间语境下不暴露）。
	ops.GET("/metrics", az.RequirePerm(authz.PermOpsView), opsHandler.Metrics)

	// Current user account: rename + password change.
	// 账号是 user 维度资源，不参与租户裁决（不受 X-Org-Id 影响）。
	authed.GET("/users/me", userHandler.Me)
	authed.PATCH("/users/me", userHandler.UpdateProfile)
	authed.POST("/users/me/password", userHandler.ChangePassword)

	// Multi-tenant organizations (stage 4-A): org CRUD + member management.
	// 组织浏览/创建属基础权限；成员管理走 org.member.mgr（owner/admin）。
	orgs := authed.Group("/organizations")
	orgs.GET("", orgHandler.List)
	orgs.POST("", orgHandler.Create)
	orgs.GET("/:id", orgHandler.Get)
	orgs.GET("/:id/members", orgHandler.ListMembers)
	orgs.POST("/:id/members", az.RequirePerm(authz.PermOrgMemberMgr), orgHandler.AddMember)
	orgs.PATCH("/:id/members/:userId", az.RequirePerm(authz.PermOrgMemberMgr), orgHandler.UpdateMemberRole)
	orgs.DELETE("/:id/members/:userId", az.RequirePerm(authz.PermOrgMemberMgr), orgHandler.RemoveMember)
	// 企业级菜单覆盖（只能缩小可见菜单）：属企业设置项，走 org.settings（owner）。
	orgs.PATCH("/:id/menu-overrides", az.RequirePerm(authz.PermOrgSettings), orgHandler.SetMenuOverrides)
	// 组织资料（名称/简称）与 logo：owner/admin 改动，handler 层裁决；logo 读取仅成员可见。
	orgs.PATCH("/:id", orgHandler.UpdateProfile)
	orgs.POST("/:id/logo", orgHandler.UploadLogo)
	orgs.GET("/:id/logo", orgHandler.GetLogo)

	// 企业角色（设计文档 §5）：列表供成员改角色的下拉使用（org.member.mgr），
	// 增删改仅 owner（org.role.mgr）。参数名与上面的 orgs 组保持一致（同为 :id）。
	roles := authed.Group("/organizations/:id/roles")
	roles.GET("", az.RequirePerm(authz.PermOrgMemberMgr), orgRoleHandler.List)
	roles.POST("", az.RequirePerm(authz.PermOrgRoleMgr), orgRoleHandler.Create)
	roles.PATCH("/:roleId", az.RequirePerm(authz.PermOrgRoleMgr), orgRoleHandler.Update)
	roles.DELETE("/:roleId", az.RequirePerm(authz.PermOrgRoleMgr), orgRoleHandler.Delete)

	// 知识库：浏览/检索属基础权限；条目增删改与重建走 knowledge.manage。
	kb := authed.Group("/knowledge")
	kb.POST("/documents", az.RequirePerm(authz.PermKnowledgeManage), knowledgeHandler.Upload)
	kb.GET("/documents", knowledgeHandler.List)
	kb.GET("/documents/:id", knowledgeHandler.Get)
	kb.PATCH("/documents/:id", az.RequirePerm(authz.PermKnowledgeManage), knowledgeHandler.Update)
	kb.DELETE("/documents/:id", az.RequirePerm(authz.PermKnowledgeManage), knowledgeHandler.Delete)
	kb.GET("/documents/:id/chunks", knowledgeHandler.ListChunks)
	kb.POST("/documents/:id/reprocess", az.RequirePerm(authz.PermKnowledgeManage), knowledgeHandler.Reprocess)
	kb.POST("/search", knowledgeHandler.Search)

	// Market (job role marketplace)
	// roles_index.json 需要先运行 go run ./cmd/gen-roles-index 生成
	if marketStore, err := store.NewMarketStore(cfg.MarketDataPath); err != nil {
		log.Warn("market store init failed, market disabled", zap.Error(err))
	} else {
		marketHandler := handler.NewMarketHandler(marketStore)
		mkt := authed.Group("/market")
		// 市场浏览走 market.browse（member 可看）；安装角色包落地为建 agent
		// （POST /agents → agent.manage），member 天然被拦住，无需独立端点。
		mkt.GET("/departments", az.RequirePerm(authz.PermMarketBrowse), marketHandler.ListDepts)
		mkt.GET("/roles", az.RequirePerm(authz.PermMarketBrowse), marketHandler.ListRoles)
		mkt.GET("/roles/:id", az.RequirePerm(authz.PermMarketBrowse), marketHandler.GetRole)
	}

	// Assistant (LLM-powered): 配置解析已热生效化（平台/租户 UI 配置 > env），
	// 路由常驻注册；当前租户无可用配置时 Chat 返回友好 SSE 错误。
	{
		toolExecutor := assistant.NewToolExecutor(s, embeddingClient, qdrantClient)
		hasKnowledge := embeddingClient != nil && qdrantClient != nil
		assistantHandler := handler.NewAssistantHandler(llmProvider, toolExecutor, hasKnowledge, log)
		authed.POST("/assistant/chat", assistantHandler.Chat)
		log.Info("assistant enabled (per-tenant llm config)",
			zap.String("env_model", cfg.AssistantModel), zap.Bool("env_key_set", cfg.AssistantAPIKey != ""))
	}

	// Platform/org LLM configuration UI endpoints (A1+B2+D1).
	llmConfigHandler := handler.NewLLMConfigHandler(s)

	// ── 平台命名空间（设计文档 §3.2 / §6.3）────────────────────────────────
	// 独立命名空间，只认平台管理员标记（env 种子），**不看企业角色**：企业角色
	// （含 owner）调此处一律 403。反向门禁见上面 authed 组的 RequireBusinessAccount。
	//
	// 与公开的 GET /api/v1/platform/info 共存：同一静态前缀下的兄弟静态段，
	// 无通配冲突（plat 组只新增 orgs/config/audit-logs/usage/llm-config/users 静态段）。
	platformAdminHandler := handler.NewPlatformAdminHandler(s)
	platformExternalAppHandler := handler.NewPlatformExternalAppHandler(s)
	plat := v1.Group("/platform")
	plat.Use(middleware.RequireAuth(jwtManager))
	plat.GET("/orgs", authz.RequirePlatformPerm(authz.PermPlatformOrgLifecycle, platformAdminChecker), platformAdminHandler.ListOrgs)
	plat.POST("/orgs", authz.RequirePlatformPerm(authz.PermPlatformOrgLifecycle, platformAdminChecker), platformAdminHandler.CreateOrg)
	plat.GET("/orgs/:id", authz.RequirePlatformPerm(authz.PermPlatformOrgLifecycle, platformAdminChecker), platformAdminHandler.GetOrg)
	plat.POST("/orgs/:id/disable", authz.RequirePlatformPerm(authz.PermPlatformOrgLifecycle, platformAdminChecker), platformAdminHandler.DisableOrg)
	plat.POST("/orgs/:id/restore", authz.RequirePlatformPerm(authz.PermPlatformOrgLifecycle, platformAdminChecker), platformAdminHandler.RestoreOrg)
	// 平台视角的企业成员（设计文档 §3.2）：只读账号元数据与角色，不读企业业务内容。
	plat.GET("/orgs/:id/members", authz.RequirePlatformPerm(authz.PermPlatformOrgLifecycle, platformAdminChecker), platformAdminHandler.ListOrgMembers)
	// 平台用户管理：账号列表/详情（读）+ 重置密码/禁用/启用（写）。
	// 禁用语义见 handler/platform_user.go（登录与 refresh 一律拒绝，token 自然过期）。
	plat.GET("/users", authz.RequirePlatformPerm(authz.PermPlatformUserRead, platformAdminChecker), platformAdminHandler.ListUsers)
	plat.GET("/users/:id", authz.RequirePlatformPerm(authz.PermPlatformUserRead, platformAdminChecker), platformAdminHandler.GetUser)
	plat.POST("/users/:id/reset-password", authz.RequirePlatformPerm(authz.PermPlatformUserManage, platformAdminChecker), platformAdminHandler.ResetUserPassword)
	plat.POST("/users/:id/disable", authz.RequirePlatformPerm(authz.PermPlatformUserManage, platformAdminChecker), platformAdminHandler.DisableUser)
	plat.POST("/users/:id/enable", authz.RequirePlatformPerm(authz.PermPlatformUserManage, platformAdminChecker), platformAdminHandler.EnableUser)
	plat.GET("/config", authz.RequirePlatformPerm(authz.PermPlatformConfigRead, platformAdminChecker), platformAdminHandler.GetConfig)
	plat.PUT("/config", authz.RequirePlatformPerm(authz.PermPlatformConfigWrite, platformAdminChecker), platformAdminHandler.PutConfig)
	plat.GET("/audit-logs", authz.RequirePlatformPerm(authz.PermPlatformAuditView, platformAdminChecker), platformAdminHandler.ListAuditLogs)
	plat.GET("/usage", authz.RequirePlatformPerm(authz.PermPlatformUsageView, platformAdminChecker), platformAdminHandler.Usage)
	// 平台默认 LLM 配置：从 authed 组（企业角色可及）收敛进平台命名空间。
	plat.GET("/llm-config", authz.RequirePlatformPerm(authz.PermPlatformConfigRead, platformAdminChecker), llmConfigHandler.GetPlatform)
	plat.PUT("/llm-config", authz.RequirePlatformPerm(authz.PermPlatformConfigWrite, platformAdminChecker), llmConfigHandler.PutPlatform)
	plat.DELETE("/llm-config", authz.RequirePlatformPerm(authz.PermPlatformConfigWrite, platformAdminChecker), llmConfigHandler.DeletePlatform)
	// 全局级外部应用（2026-09-17 三级作用域）：平台管理员独有的产品挂载配置，
	// 与企业角色正交 —— 企业 owner 调此处一律 403（平台命名空间门禁）。
	plat.GET("/external-apps", authz.RequirePlatformPerm(authz.PermPlatformExtAppMgr, platformAdminChecker), platformExternalAppHandler.List)
	plat.POST("/external-apps", authz.RequirePlatformPerm(authz.PermPlatformExtAppMgr, platformAdminChecker), platformExternalAppHandler.Create)
	plat.PATCH("/external-apps/:id", authz.RequirePlatformPerm(authz.PermPlatformExtAppMgr, platformAdminChecker), platformExternalAppHandler.Update)
	plat.DELETE("/external-apps/:id", authz.RequirePlatformPerm(authz.PermPlatformExtAppMgr, platformAdminChecker), platformExternalAppHandler.Delete)
	// 桌面端发行版（自建更新源，docs/desktop-app-update-plan-2026-09-18.md）：
	// 上传 / 发布 / 回滚 / 删除安装包。四个动作共用一个平台权限点
	// platform.desktop.release —— 它们的破坏力同档（都能改变「全员拿到哪个版本」），
	// 再拆权限点只会增加配置负担而无实质隔离。
	plat.GET("/desktop-releases", authz.RequirePlatformPerm(authz.PermPlatformDesktopRelease, platformAdminChecker), platformDesktopReleaseHandler.List)
	plat.POST("/desktop-releases", authz.RequirePlatformPerm(authz.PermPlatformDesktopRelease, platformAdminChecker), platformDesktopReleaseHandler.Upload)
	plat.POST("/desktop-releases/:id/publish", authz.RequirePlatformPerm(authz.PermPlatformDesktopRelease, platformAdminChecker), platformDesktopReleaseHandler.Publish)
	plat.POST("/desktop-releases/:id/rollback", authz.RequirePlatformPerm(authz.PermPlatformDesktopRelease, platformAdminChecker), platformDesktopReleaseHandler.Rollback)
	plat.DELETE("/desktop-releases/:id", authz.RequirePlatformPerm(authz.PermPlatformDesktopRelease, platformAdminChecker), platformDesktopReleaseHandler.Delete)

	// 平台「使用引导」（单篇全局 HTML）：上传即覆盖（PUT），删除幂等。
	// 阅读端点在 authed 组的 GET /guide —— 引导是给全员看的，登录即可读。
	plat.GET("/guide", authz.RequirePlatformPerm(authz.PermPlatformGuideMgr, platformAdminChecker), platformGuideHandler.PlatformGet)
	plat.PUT("/guide", authz.RequirePlatformPerm(authz.PermPlatformGuideMgr, platformAdminChecker), platformGuideHandler.PlatformUpload)
	plat.DELETE("/guide", authz.RequirePlatformPerm(authz.PermPlatformGuideMgr, platformAdminChecker), platformGuideHandler.PlatformDelete)

	// 移动端安装包：上传即覆盖（POST），删除幂等。三个动作共用 platform.mobileapp.mgr。
	// 公开 meta/下载在 v1 组的 /mobile/app/latest 与 /mobile/app/download。
	plat.GET("/mobile-app", authz.RequirePlatformPerm(authz.PermPlatformMobileAppMgr, platformAdminChecker), mobileAppReleaseHandler.PlatformGet)
	plat.POST("/mobile-app", authz.RequirePlatformPerm(authz.PermPlatformMobileAppMgr, platformAdminChecker), mobileAppReleaseHandler.PlatformUpload)
	plat.DELETE("/mobile-app", authz.RequirePlatformPerm(authz.PermPlatformMobileAppMgr, platformAdminChecker), mobileAppReleaseHandler.PlatformDelete)

	// 企业层 LLM 配置：handler 层已有 owner/admin 守卫，路由层维持基础权限。
	orgLLM := authed.Group("/organizations/:id/llm-config")
	orgLLM.GET("", llmConfigHandler.GetOrg)
	orgLLM.PUT("", llmConfigHandler.PutOrg)
	orgLLM.DELETE("", llmConfigHandler.DeleteOrg)
	authed.POST("/llm-config/test", llmConfigHandler.Test)
	authed.POST("/llm-config/models", llmConfigHandler.Models)
	// 个人空间层：配置挂在本人个人租户键上，任何登录用户可配自己的。
	personalLLM := authed.Group("/llm-config/personal")
	personalLLM.GET("", llmConfigHandler.GetPersonal)
	personalLLM.PUT("", llmConfigHandler.PutPersonal)
	personalLLM.DELETE("", llmConfigHandler.DeletePersonal)

	// Question timeout supervisor: periodically auto-resumes waiting_user todos
	// whose non-required question went unanswered past the timeout.
	go superviseQuestionTimeouts(s, taskHandler, cfg.QuestionTimeout, log)

	return &App{Engine: engine, Store: s, PeerSyncer: peerSyncer, TrustRequestSyncer: trustRequestSyncer}, nil
}

// superviseQuestionTimeouts scans for timed-out non-required todo.ask
// questions and auto-resumes them, forwarding todo.answer (__timeout__) so the
// assignee agent can proceed on its own judgement.
func superviseQuestionTimeouts(s *store.Store, taskHandler *handler.TaskHandler, timeout time.Duration, log *zap.Logger) {
	if s == nil || taskHandler == nil || timeout <= 0 {
		return
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now().UTC()
		for _, it := range s.ListTimedOutQuestions(now, timeout) {
			task, q, appErr := s.AnswerTodo(store.SystemScope(), it.TaskID, it.TodoID, it.QuestionID, "__timeout__", "system", true)
			if appErr != nil {
				if log != nil {
					log.Warn("question timeout auto-answer failed",
						zap.String("task_id", it.TaskID), zap.String("todo_id", it.TodoID),
						zap.String("question_id", it.QuestionID), zap.Error(appErr))
				}
				continue
			}
			taskHandler.PublishTodoAnswer(context.Background(), task, it.TodoID, q)
			if log != nil {
				log.Info("question timed out, todo auto-resumed",
					zap.String("task_id", it.TaskID), zap.String("todo_id", it.TodoID), zap.String("question_id", it.QuestionID))
			}
		}
	}
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	var errs []error
	if a.PeerSyncer != nil {
		a.PeerSyncer.Close()
	}
	if a.TrustRequestSyncer != nil {
		a.TrustRequestSyncer.Close()
	}
	if a.Store != nil {
		if err := a.Store.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// FlushAll 在停机时把全内存状态有界地回写 Mongo。nil-safe：a 或 a.Store 为 nil
// 时返回 nil，方便在初始化失败等边界场景安全调用。
//
// 调用顺序约定：必须在 Close() 之前调用（Close 会断开 Mongo，之后落盘无从谈起）。
// ctx 由调用方控制时间预算；到期时 Store 侧会 fail-fast 返回 ctx 错误。
func (a *App) FlushAll(ctx context.Context) error {
	if a == nil || a.Store == nil {
		return nil
	}
	return a.Store.FlushPersistAll(ctx)
}
