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
	"trustmesh/backend/internal/clawsynapse"
	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/embedding"
	"trustmesh/backend/internal/handler"
	"trustmesh/backend/internal/knowledge"
	"trustmesh/backend/internal/middleware"
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
	s.EnsurePlatformAdminExists()
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

	authHandler := handler.NewAuthHandler(s, jwtManager)
	userHandler := handler.NewUserHandler(s)
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
	externalAppHandler := handler.NewExternalAppHandler(s, auth.ExternalTokenIssuer, cfg.ExternalAppTokenTTL)
	workflowTemplateHandler := handler.NewWorkflowTemplateHandler(s)
	orgHandler := handler.NewOrgHandler(s)

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

	authed := v1.Group("")
	authed.Use(middleware.RequireAuth(jwtManager))
	// 多租户阶段 0：解析 X-Org-Id 并注入 Scope。存量客户端不带该头，
	// 中间件直接放行（只带 UserID），行为与改造前完全一致。
	authed.Use(middleware.OrgScope(s))

	authed.POST("/agents", agentHandler.Create)
	authed.GET("/agents", agentHandler.List)
	authed.GET("/agents/invite-prompt", joinRequestHandler.GetInvitePrompt)
	authed.GET("/agents/join-requests", joinRequestHandler.List)
	authed.POST("/agents/join-requests/:id/approve", joinRequestHandler.Approve)
	authed.POST("/agents/join-requests/:id/reject", joinRequestHandler.Reject)
	authed.GET("/agents/:id", agentHandler.Get)
	authed.PATCH("/agents/:id", agentHandler.Update)
	authed.DELETE("/agents/:id", agentHandler.Delete)
	authed.GET("/agents/:id/stats", agentHandler.Stats)
	authed.GET("/agents/:id/insights", agentHandler.Insights)
	authed.GET("/agents/:id/tasks", agentHandler.Tasks)
	authed.GET("/agents/:id/capabilities", agentHandler.GetCapabilities)
	authed.POST("/agents/:id/capabilities", agentHandler.SetCapabilities)
	authed.GET("/agents/:id/cron/executions", agentHandler.ListCronExecutions)
	authed.POST("/agents/:id/skills/upload", agentHandler.UploadSkillFile)
	authed.GET("/agents/:id/chat", agentChatHandler.Get)
	authed.GET("/agents/:id/chat/sessions", agentChatHandler.ListSessions)
	authed.GET("/agents/:id/chat/sessions/:sessionId", agentChatHandler.GetSession)
	authed.POST("/agents/:id/chat/messages", agentChatHandler.SendMessage)
	authed.POST("/agents/:id/chat/reset", agentChatHandler.Reset)

	authed.POST("/projects", projectHandler.Create)
	authed.GET("/projects", projectHandler.List)
	authed.GET("/projects/:projectId", projectHandler.Get)
	authed.GET("/projects/:projectId/workflow-progress", projectHandler.WorkflowProgress)
	// 项目流程 · 手工绑定交付物：任意文件（含用户手工上传的）→ 任意步骤输出位。
	authed.POST("/projects/:projectId/workflow/steps/:stepIndex/outputs/bind", projectHandler.BindStepOutput)
	authed.PATCH("/projects/:projectId", projectHandler.Update)
	authed.DELETE("/projects/:projectId", projectHandler.Archive)

	// Global workflow templates (user-scoped) + project inherit/sync.
	authed.POST("/workflow-templates", workflowTemplateHandler.Create)
	authed.GET("/workflow-templates", workflowTemplateHandler.List)
	authed.GET("/workflow-templates/:templateId", workflowTemplateHandler.Get)
	authed.PATCH("/workflow-templates/:templateId", workflowTemplateHandler.Update)
	authed.POST("/workflow-templates/:templateId/copy", workflowTemplateHandler.Copy)
	authed.DELETE("/workflow-templates/:templateId", workflowTemplateHandler.Delete)
	// T1.9: 从成功任务一键沉淀工作流模板（挂在 /tasks/:id 下，避开
	// /workflow-templates/:templateId 通配段的兄弟节点冲突）。
	authed.POST("/tasks/:id/distill-template", workflowTemplateHandler.Distill)
	authed.POST("/projects/:projectId/workflows/inherit", workflowTemplateHandler.Inherit)
	authed.GET("/projects/:projectId/workflows/:workflowId/sync-diff", workflowTemplateHandler.SyncDiff)
	authed.POST("/projects/:projectId/workflows/:workflowId/sync", workflowTemplateHandler.ApplySync)
	authed.POST("/projects/:projectId/workflows/:workflowId/detach", workflowTemplateHandler.Detach)

	// Project files
	projectFileStorage := project.NewLocalFileStorage(cfg.FilesStoragePath)
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
	go s.StartCleanupTicker(context.Background())
	// Start timeout monitor to detect and retry/fail stuck in_progress todos.
	// Reads remindHook / planningStallHook — 必须在 SetRemindHook /
	// SetPlanningStallHook 之后启动。
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

	// Meeting Room routes
	authed.POST("/projects/:projectId/meetings", meetingHandler.Create)
	authed.GET("/projects/:projectId/meetings", meetingHandler.List)
	authed.GET("/meetings/:id", meetingHandler.Get)
	authed.POST("/meetings/:id/messages", meetingHandler.SendMessage)
	authed.GET("/meetings/:id/messages", meetingHandler.ListMessages)
	authed.POST("/meetings/:id/start", meetingHandler.Start)
	authed.POST("/meetings/:id/end", meetingHandler.End)
	authed.POST("/meetings/:id/todos", meetingHandler.AddTodo)

	authed.POST("/projects/:projectId/tasks", taskHandler.Create)
	authed.POST("/projects/:projectId/tasks/planning", taskHandler.CreatePlanning)
	authed.POST("/projects/:projectId/tasks/from-text", taskHandler.CreateFromText)
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
	authed.POST("/tasks/:id/todos/:todoId/dispatch", taskHandler.DispatchTodo)
	authed.POST("/tasks/:id/todos/:todoId/outputs/bind", taskHandler.BindTodoOutput)
	authed.POST("/tasks/:id/todos/:todoId/review", taskHandler.ReviewTodo)
	authed.POST("/tasks/:id/todos/:todoId/reopen", taskHandler.ReopenTodo)
	authed.POST("/tasks/:id/todos/:todoId/answer", taskHandler.AnswerTodo)
	authed.GET("/tasks/:id/comments", taskHandler.ListComments)
	authed.POST("/tasks/:id/comments", taskHandler.AddComment)
	authed.GET("/action-items", actionItemsHandler.List)
	authed.POST("/action-items/convert", actionItemsHandler.Convert)
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
	ext := authed.Group("/external-apps")
	ext.POST("", externalAppHandler.Create)
	ext.GET("", externalAppHandler.List)
	ext.GET("/:id", externalAppHandler.Get)
	ext.PATCH("/:id", externalAppHandler.Update)
	ext.DELETE("/:id", externalAppHandler.Delete)
	ext.POST("/:id/launch", externalAppHandler.Launch)

	// Ops incidents: rule-based anomaly tickets + manual intervention (ignore/close).
	opsHandler := handler.NewOpsHandler(s)
	ops := authed.Group("/ops")
	ops.GET("/incidents", opsHandler.List)
	ops.GET("/incidents/:id", opsHandler.Get)
	ops.POST("/incidents/:id/ignore", opsHandler.Ignore)
	ops.POST("/incidents/:id/close", opsHandler.Close)
	// Phase 0 observability export (T0.11): process-wide counters, histograms
	// (step-advance latency, stall duration) and the alert rule set. Gated to
	// org owner/admin inside the handler.
	ops.GET("/metrics", opsHandler.Metrics)

	// Current user account: rename + password change.
	// 账号是 user 维度资源，不参与租户裁决（不受 X-Org-Id 影响）。
	authed.GET("/users/me", userHandler.Me)
	authed.PATCH("/users/me", userHandler.UpdateProfile)
	authed.POST("/users/me/password", userHandler.ChangePassword)

	// Multi-tenant organizations (stage 4-A): org CRUD + member management.
	orgs := authed.Group("/organizations")
	orgs.GET("", orgHandler.List)
	orgs.POST("", orgHandler.Create)
	orgs.GET("/:id", orgHandler.Get)
	orgs.GET("/:id/members", orgHandler.ListMembers)
	orgs.POST("/:id/members", orgHandler.AddMember)
	orgs.PATCH("/:id/members/:userId", orgHandler.UpdateMemberRole)
	orgs.DELETE("/:id/members/:userId", orgHandler.RemoveMember)

	kb := authed.Group("/knowledge")
	kb.POST("/documents", knowledgeHandler.Upload)
	kb.GET("/documents", knowledgeHandler.List)
	kb.GET("/documents/:id", knowledgeHandler.Get)
	kb.PATCH("/documents/:id", knowledgeHandler.Update)
	kb.DELETE("/documents/:id", knowledgeHandler.Delete)
	kb.GET("/documents/:id/chunks", knowledgeHandler.ListChunks)
	kb.POST("/documents/:id/reprocess", knowledgeHandler.Reprocess)
	kb.POST("/search", knowledgeHandler.Search)

	// Market (job role marketplace)
	// roles_index.json 需要先运行 go run ./cmd/gen-roles-index 生成
	if marketStore, err := store.NewMarketStore(cfg.MarketDataPath); err != nil {
		log.Warn("market store init failed, market disabled", zap.Error(err))
	} else {
		marketHandler := handler.NewMarketHandler(marketStore)
		mkt := authed.Group("/market")
		mkt.GET("/departments", marketHandler.ListDepts)
		mkt.GET("/roles", marketHandler.ListRoles)
		mkt.GET("/roles/:id", marketHandler.GetRole)
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
	platLLM := authed.Group("/platform/llm-config")
	platLLM.GET("", llmConfigHandler.GetPlatform)
	platLLM.PUT("", llmConfigHandler.PutPlatform)
	platLLM.DELETE("", llmConfigHandler.DeletePlatform)
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
