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
	// Start background cleanup ticker to prevent unbounded memory growth (OOM).
	go s.StartCleanupTicker(context.Background())
	// Start timeout monitor to detect and retry/fail stuck in_progress todos.
	go s.StartTimeoutMonitor(context.Background())
	jwtManager := auth.NewJWTManager(cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	clawClient := clawsynapse.NewClient(cfg.ClawSynapseAPIURL, cfg.ClawSynapseTimeout)
	webhookHandler := clawsynapse.NewWebhookHandler(s, clawClient, log)
	// Timeout retries must actually re-dispatch the todo to its assignee.
	s.SetDispatchHook(webhookHandler.RedispatchTodo)
	// Timeout reminders nudge the assignee without re-dispatching the todo.
	s.SetRemindHook(webhookHandler.RemindTodo)
	// Planning-stall nudges wake the PM via task.message when a task has been
	// stuck in planning without a finalized plan.
	s.SetPlanningStallHook(webhookHandler.NudgePlanningPM)
	peerSyncer := clawsynapse.NewPeerSyncer(clawClient, s, cfg.ClawSynapsePeerSync, log)
	if peerSyncer != nil {
		peerSyncer.Start()
	}
	trustRequestSyncer := clawsynapse.NewTrustRequestSyncer(clawClient, s, cfg.ClawSynapsePeerSync, log)
	if trustRequestSyncer != nil {
		trustRequestSyncer.Start()
	}

	authHandler := handler.NewAuthHandler(s, jwtManager)
	agentHandler := handler.NewAgentHandler(s, clawClient)
	agentChatHandler := handler.NewAgentChatHandler(s, clawClient, log)
	projectHandler := handler.NewProjectHandler(s)

	taskHandler := handler.NewTaskHandler(s, clawClient, webhookHandler, cfg.ExternalURL, []byte(cfg.JWTSecret), cfg.DownloadTokenTTL, log)
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

	// Inject knowledge components into webhook handler for knowledge.query
	// support (embedder/qdrant may be nil → text-only search).
	webhookHandler.SetKnowledgeComponents(embeddingClient, qdrantClient)

	engine.GET("/healthz", handler.Health)
	engine.GET("/webhook/clawsynapse", func(c *gin.Context) { c.Status(200) })
	engine.POST("/webhook/clawsynapse", webhookHandler.HandleWebhook)

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
	authed.PATCH("/projects/:projectId", projectHandler.Update)
	authed.DELETE("/projects/:projectId", projectHandler.Archive)

	// Global workflow templates (user-scoped) + project inherit/sync.
	authed.POST("/workflow-templates", workflowTemplateHandler.Create)
	authed.GET("/workflow-templates", workflowTemplateHandler.List)
	authed.GET("/workflow-templates/:templateId", workflowTemplateHandler.Get)
	authed.PATCH("/workflow-templates/:templateId", workflowTemplateHandler.Update)
	authed.POST("/workflow-templates/:templateId/copy", workflowTemplateHandler.Copy)
	authed.DELETE("/workflow-templates/:templateId", workflowTemplateHandler.Delete)
	authed.POST("/projects/:projectId/workflows/inherit", workflowTemplateHandler.Inherit)
	authed.GET("/projects/:projectId/workflows/:workflowId/sync-diff", workflowTemplateHandler.SyncDiff)
	authed.POST("/projects/:projectId/workflows/:workflowId/sync", workflowTemplateHandler.ApplySync)
	authed.POST("/projects/:projectId/workflows/:workflowId/detach", workflowTemplateHandler.Detach)

	// Project files
	projectFileStorage := project.NewLocalFileStorage(cfg.FilesStoragePath)
	s.SetFileStorage(projectFileStorage) // used by timeout monitor to auto-generate meeting minutes
	projectFileHandler := handler.NewProjectFileHandler(s, projectFileStorage, log)
	webhookHandler.SetProjectFileStorage(projectFileStorage)
	webhookHandler.SetAgentFileConfig(cfg.ExternalURL, []byte(cfg.JWTSecret), cfg.DownloadTokenTTL)

	// Agent file download endpoint (authenticated by short-lived download token, not JWT).
	// New format: /api/v1/files/agent/:fileId/token/:token (token in path, not query).
	// Old format: /api/v1/files/agent/:fileId?token=xxx (kept for compatibility).
	agentFileHandler := agentfile.NewHandler(s, projectFileStorage, jwtManager, log)
	v1.GET("/files/agent/:fileId/token/:token", agentFileHandler.Download)
	v1.GET("/files/agent/:fileId", agentFileHandler.Download)
	v1.GET("/debug/gen-token/:fileId", agentFileHandler.DebugGenToken)

	meetingHandler := handler.NewMeetingHandler(s, clawClient, projectFileStorage, cfg.ExternalURL, []byte(cfg.JWTSecret), cfg.DownloadTokenTTL)
	webhookHandler.SetMeetingActivityNotifier(meetingHandler.OnMeetingActivity)

	// Resume the inactivity watchdog for any meeting left in_progress by a
	// previous backend instance (timers are in-memory only).
	go meetingHandler.RecoverTimeouts(context.Background())

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

	// Assistant (LLM-powered, optional)
	if cfg.AssistantAPIKey != "" {
		llmClient := assistant.NewLLMClient(cfg.AssistantAPIURL, cfg.AssistantAPIKey, cfg.AssistantModel)
		toolExecutor := assistant.NewToolExecutor(s, embeddingClient, qdrantClient)
		hasKnowledge := embeddingClient != nil && qdrantClient != nil
		assistantHandler := handler.NewAssistantHandler(llmClient, toolExecutor, hasKnowledge, log)
		authed.POST("/assistant/chat", assistantHandler.Chat)
		log.Info("assistant enabled", zap.String("model", cfg.AssistantModel))
	}

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
