package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/metrics"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

type processedMessageRecord struct {
	ID         string `bson:"_id"`
	Action     string `bson:"action"`
	ResourceID string `bson:"resource_id"`
}

func (s *Store) enableMongo(cfg config.Config, log *zap.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MongoTimeout)
	defer cancel()

	opts := options.Client().
		ApplyURI(cfg.MongoURI).
		SetMaxPoolSize(20).
		SetMinPoolSize(2).
		SetMaxConnIdleTime(30 * time.Second).
		SetServerSelectionTimeout(5 * time.Second).
		SetConnectTimeout(10 * time.Second).
		SetTimeout(10 * time.Second)

	client, err := mongo.Connect(opts)
	if err != nil {
		return err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return err
	}

	db := client.Database(cfg.MongoDatabase)
	s.mongoEnabled = true
	s.mongoClient = client
	s.mongoUsers = db.Collection("users")
	s.mongoAgents = db.Collection("agents")
	s.mongoJoinRequests = db.Collection("join_requests")
	s.mongoProjects = db.Collection("projects")
	s.mongoAgentChats = db.Collection("agent_chats")
	s.mongoTasks = db.Collection("tasks")
	s.mongoEvents = db.Collection("events")
	s.mongoComments = db.Collection("comments")
	s.mongoProcessedMessages = db.Collection("processed_messages")
	s.mongoNotifications = db.Collection("notifications")
	s.mongoArtifacts = db.Collection("artifacts")
	s.mongoExternalApps = db.Collection("external_apps")
	s.mongoKnowledgeDocs = db.Collection("knowledge_documents")
	s.mongoKnowledgeChunks = db.Collection("knowledge_chunks")
	s.mongoProjectFiles = db.Collection("project_files")
	s.mongoMeetings = db.Collection("meetings")
	s.mongoMeetingMessages = db.Collection("meeting_messages")
	s.mongoWorkflowTemplates = db.Collection("workflow_templates")
	s.mongoOrganizations = db.Collection("organizations")
	s.mongoOrgMemberships = db.Collection("org_memberships")
	s.mongoProjectMembers = db.Collection("project_members")
	s.mongoOpsIncidents = db.Collection("ops_incidents")
	s.mongoLLMSettings = db.Collection("platform_settings")
	// T2.6 通用幂等键集合：_id 唯一由 Mongo 隐式保证（E11000 = 命中），
	// expire_at 上的 TTL 索引（expireAfterSeconds=0）由 ensureMongoIndexes 创建。
	s.mongoIdempotencyKeys = db.Collection("idempotency_keys")
	s.mongoTimeout = cfg.MongoTimeout
	if log != nil {
		s.log = log
	}

	if err := s.ensureMongoIndexes(); err != nil {
		_ = client.Disconnect(context.Background())
		s.clearMongoCollections()
		return err
	}
	if err := s.loadMongoState(); err != nil {
		_ = client.Disconnect(context.Background())
		s.clearMongoCollections()
		return err
	}

	// 启动快照自检：仅观测（记日志），不影响启动流程。
	s.validateLoadedSnapshot()

	if s.log != nil {
		s.log.Info("mongo repository store enabled", zap.String("database", cfg.MongoDatabase))
	}
	return nil
}

// validateLoadedSnapshot 在 loadMongoState 之后做一致性自检，仅记日志（WARN），
// 不影响启动。
//
// 目的：TrustMesh 是全内存状态机，若镜像加载实际失败或镜像为空，进程会以「空库」
// 启动并在一段时间内静默地用新请求覆盖旧数据。这里把关键集合的计数打出来，并在
// 关键集合全为空时告警，给运维一个可观测的信号；绝不可因自检失败而阻断启动。
func (s *Store) validateLoadedSnapshot() {
	s.mu.RLock()
	users := len(s.users)
	agents := len(s.agents)
	projects := len(s.projects)
	tasks := len(s.tasks)
	s.mu.RUnlock()

	if s.log == nil {
		return
	}
	s.log.Info("mongo state loaded snapshot",
		zap.Int("users", users),
		zap.Int("agents", agents),
		zap.Int("projects", projects),
		zap.Int("tasks", tasks),
	)
	if users == 0 && agents == 0 && projects == 0 && tasks == 0 {
		s.log.Warn("mongo state loaded but principal collections are empty",
			zap.String("hint", "mirror may be empty or load failed silently; verify MONGO_DATABASE/MONGO_URI before trusting this node"))
	}
}

func (s *Store) Close() error {
	if !s.mongoEnabled || s.mongoClient == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.mongoTimeout)
	defer cancel()
	return s.mongoClient.Disconnect(ctx)
}

func (s *Store) clearMongoCollections() {
	s.mongoEnabled = false
	s.mongoClient = nil
	s.mongoUsers = nil
	s.mongoAgents = nil
	s.mongoJoinRequests = nil
	s.mongoProjects = nil
	s.mongoAgentChats = nil
	s.mongoTasks = nil
	s.mongoEvents = nil
	s.mongoComments = nil
	s.mongoProcessedMessages = nil
	s.mongoNotifications = nil
	s.mongoArtifacts = nil
	s.mongoExternalApps = nil
	s.mongoKnowledgeDocs = nil
	s.mongoKnowledgeChunks = nil
	s.mongoProjectFiles = nil
	s.mongoMeetings = nil
	s.mongoMeetingMessages = nil
	s.mongoWorkflowTemplates = nil
	s.mongoOrganizations = nil
	s.mongoOrgMemberships = nil
	s.mongoProjectMembers = nil
	s.mongoOpsIncidents = nil
	s.mongoLLMSettings = nil
	s.mongoIdempotencyKeys = nil
}

func (s *Store) mongoContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), s.mongoTimeout)
}

func (s *Store) ensureMongoIndexes() error {
	indexes := map[*mongo.Collection][]mongo.IndexModel{
		s.mongoUsers: {
			{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		s.mongoAgents: {
			{Keys: bson.D{{Key: "node_id", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "user_id", Value: 1}}},
			{Keys: bson.D{{Key: "status", Value: 1}}},
			{Keys: bson.D{{Key: "role", Value: 1}}},
			{Keys: bson.D{{Key: "capabilities", Value: 1}}},
		},
		s.mongoJoinRequests: {
			{Keys: bson.D{{Key: "trust_request_id", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "status", Value: 1}, {Key: "created_at", Value: -1}}},
			{Keys: bson.D{{Key: "node_id", Value: 1}, {Key: "status", Value: 1}}},
		},
		s.mongoProjects: {
			{Keys: bson.D{{Key: "user_id", Value: 1}}},
			{Keys: bson.D{{Key: "pm_agent_id", Value: 1}}},
		},
		s.mongoTasks: {
			{Keys: bson.D{{Key: "project_id", Value: 1}, {Key: "status", Value: 1}}},
			{Keys: bson.D{{Key: "pm_agent_id", Value: 1}}},
			{Keys: bson.D{{Key: "todos.assignee.agent_id", Value: 1}, {Key: "todos.status", Value: 1}}},
			{Keys: bson.D{{Key: "status", Value: 1}}},
		},
		s.mongoEvents: {
			{Keys: bson.D{{Key: "task_id", Value: 1}, {Key: "created_at", Value: 1}}},
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: 1}}},
			{Keys: bson.D{{Key: "actor_id", Value: 1}, {Key: "created_at", Value: 1}}},
		},
		s.mongoComments: {
			{Keys: bson.D{{Key: "task_id", Value: 1}, {Key: "created_at", Value: 1}}},
		},
		s.mongoNotifications: {
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "is_read", Value: 1}, {Key: "created_at", Value: -1}}},
		},
		s.mongoAgentChats: {
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "agent_id", Value: 1}, {Key: "status", Value: 1}}},
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "agent_id", Value: 1}, {Key: "updated_at", Value: -1}}},
			{Keys: bson.D{{Key: "session_key", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		s.mongoArtifacts: {
			{Keys: bson.D{{Key: "task_id", Value: 1}}},
			{Keys: bson.D{{Key: "todo_id", Value: 1}}},
		},
		s.mongoKnowledgeDocs: {
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "status", Value: 1}}},
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "project_id", Value: 1}}},
			{Keys: bson.D{{Key: "tags", Value: 1}}},
		},
		s.mongoKnowledgeChunks: {
			{Keys: bson.D{{Key: "document_id", Value: 1}, {Key: "chunk_index", Value: 1}}},
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "project_id", Value: 1}}},
		},
		s.mongoProjectFiles: {
			{Keys: bson.D{{Key: "project_id", Value: 1}}},
			{Keys: bson.D{{Key: "project_id", Value: 1}, {Key: "source", Value: 1}}},
			{Keys: bson.D{{Key: "project_id", Value: 1}, {Key: "task_id", Value: 1}}},
			{Keys: bson.D{{Key: "transfer_id", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"transfer_id": bson.M{"$exists": true}})},
		},
		s.mongoMeetings: {
			{Keys: bson.D{{Key: "project_id", Value: 1}}},
		},
		s.mongoMeetingMessages: {
			{Keys: bson.D{{Key: "meeting_id", Value: 1}}},
		},
		s.mongoOrganizations: {
			{Keys: bson.D{{Key: "slug", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "owner_id", Value: 1}}},
		},
		s.mongoOrgMemberships: {
			{Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "user_id", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "user_id", Value: 1}}},
		},
		s.mongoProjectMembers: {
			{Keys: bson.D{{Key: "project_id", Value: 1}, {Key: "user_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		s.mongoWorkflowTemplates: {
			{Keys: bson.D{{Key: "user_id", Value: 1}}},
		},
		s.mongoLLMSettings: {
			{Keys: bson.D{{Key: "org_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		s.mongoOpsIncidents: {
			// 去重硬保证：同一主体+规则在活跃期只允许一个工单。
			// 只靠应用层判断会在并发巡检下漏判，必须用唯一索引兜底。
			// 部分唯一索引：仅活跃工单（active=true）对 dedupe_key 唯一。
			// 全局唯一会让终态历史工单挡住同类新工单（问题复发开不了单）。
			{Keys: bson.D{{Key: "dedupe_key", Value: 1}},
				Options: options.Index().SetUnique(true).
					SetPartialFilterExpression(bson.M{"active": true})},
			{Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "status", Value: 1}, {Key: "created_at", Value: -1}}},
			{Keys: bson.D{{Key: "task_id", Value: 1}, {Key: "status", Value: 1}}},
			{Keys: bson.D{{Key: "rule_id", Value: 1}, {Key: "status", Value: 1}}},
		},
		s.mongoIdempotencyKeys: {
			// T2.6 通用幂等键集合：_id 唯一（Mongo 隐式保证，E11000 = 命中），
			// expire_at 上的 TTL 索引（expireAfterSeconds=0）到点按字段值过期，
			// 避免集合无限增长。
			{Keys: bson.D{{Key: "expire_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		},
	}

	if s.mongoTasks != nil {
		ctx, cancel := s.mongoContext()
		_ = s.mongoTasks.Indexes().DropOne(ctx, "conversation_id_1")
		cancel()
	}

	for collection, models := range indexes {
		if collection == nil || len(models) == 0 {
			continue
		}
		ctx, cancel := s.mongoContext()
		_, err := collection.Indexes().CreateMany(ctx, models)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) loadMongoState() error {
	users, err := s.loadUsers()
	if err != nil {
		return err
	}
	agents, err := s.loadAgents()
	if err != nil {
		return err
	}
	joinRequests, userJoinRequests, trustRequestIndex, err := s.loadJoinRequests(users)
	if err != nil {
		return err
	}
	// T2.3 存量兼容：先把「无 version 字段」的存量项目文档补成 1，再载入 ——
	// 否则带 {_id, version} 乐观锁 filter 的更新永远匹配不上 → 生产全量写失败。
	s.backfillProjectVersions()
	projects, err := s.loadProjects()
	if err != nil {
		return err
	}
	agentChats, activeAgentChats, agentChatBySession, err := s.loadAgentChats()
	if err != nil {
		return err
	}
	// T2.2 存量兼容：先把「无 version 字段」的存量任务文档补成 1，再载入 ——
	// 否则带 {_id, version} 乐观锁 filter 的更新永远匹配不上 → 生产全量写失败。
	s.backfillTaskVersions()
	tasks, projectTasks, err := s.loadTasks()
	if err != nil {
		return err
	}
	taskEvents, userEvents, agentEvents, err := s.loadEvents()
	if err != nil {
		return err
	}
	taskComments, err := s.loadComments()
	if err != nil {
		return err
	}
	taskArtifacts, err := s.loadArtifacts()
	if err != nil {
		return err
	}
	externalApps, err := s.loadExternalApps()
	if err != nil {
		return err
	}
	processedMessages, err := s.loadProcessedMessages()
	if err != nil {
		return err
	}
	notifications, userNotifications, err := s.loadNotifications()
	if err != nil {
		return err
	}
	knowledgeDocs, userKnowledgeDocs, err := s.loadKnowledgeDocs()
	if err != nil {
		return err
	}
	projectFiles, projectFileIndex, transferFileIndex, err := s.loadProjectFiles()
	if err != nil {
		return err
	}
	// T2.1 存量兼容：先把「无 version 字段」的存量会议文档补成 1，再载入 ——
	// 否则带 {_id, version} 乐观锁 filter 的更新永远匹配不上 → 生产全量写失败。
	s.backfillMeetingVersions()
	meetings, projectMeetings, err := s.loadMeetings()
	if err != nil {
		return err
	}
	meetingMessages, meetingMessageIndex, err := s.loadMeetingMessages()
	if err != nil {
		return err
	}
	workflowTemplates, userWorkflowTemplates, err := s.loadWorkflowTemplates()
	if err != nil {
		return err
	}
	organizations, err := s.loadOrganizations()
	if err != nil {
		return err
	}
	orgMemberships, orgMemberIndex, userOrgIndex, err := s.loadOrgMemberships()
	if err != nil {
		return err
	}
	projectMembers, err := s.loadProjectMembers()
	if err != nil {
		return err
	}
	opsIncidents, opsByDedupeKey, opsByTask, err := s.loadOpsIncidents()
	if err != nil {
		return err
	}
	llmConfigs, err := s.loadLLMConfigs()
	if err != nil {
		return err
	}
	usersByMail := make(map[string]string, len(users))
	for id, user := range users {
		usersByMail[user.Email] = id
	}
	agentByNode := make(map[string]string, len(agents))
	for id, agent := range agents {
		if !agent.Archived {
			agentByNode[agent.NodeID] = id
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.users = users
	s.usersByMail = usersByMail
	s.agents = agents
	s.agentByNode = agentByNode
	s.joinRequests = joinRequests
	s.userJoinRequests = userJoinRequests
	s.trustRequestIndex = trustRequestIndex
	s.projects = projects
	s.agentChats = agentChats
	s.activeAgentChats = activeAgentChats
	s.agentChatBySession = agentChatBySession
	s.tasks = tasks
	s.projectTasks = projectTasks
	s.taskEvents = taskEvents
	s.userEvents = userEvents
	s.agentEvents = agentEvents
	s.taskComments = taskComments
	s.taskArtifacts = taskArtifacts
	s.externalApps = externalApps
	s.processedMessages = processedMessages
	s.notifications = notifications
	s.userNotifications = userNotifications
	s.knowledgeDocs = knowledgeDocs
	s.userKnowledgeDocs = userKnowledgeDocs
	s.projectFiles = projectFiles
	s.projectFileIndex = projectFileIndex
	s.transferFileIndex = transferFileIndex
	s.meetings = meetings
	s.projectMeetings = projectMeetings
	s.meetingMessages = meetingMessages
	s.meetingMessageIndex = meetingMessageIndex
	s.workflowTemplates = workflowTemplates
	s.userWorkflowTemplates = userWorkflowTemplates
	s.organizations = organizations
	s.orgMemberships = orgMemberships
	s.orgMemberIndex = orgMemberIndex
	s.userOrgIndex = userOrgIndex
	s.projectMembers = projectMembers
	s.llmConfigs = llmConfigs
	s.opsIncidents = opsIncidents
	s.opsByDedupeKey = opsByDedupeKey
	s.opsByTask = opsByTask
	// 多租户：事件索引按资源归属重建（含存量事件归属校正），幂等。
	// 必须在 tasks/projects/agents 全部赋值之后执行。
	s.reindexEventOrgsUnsafe()
	// T1.2：会议会话态回填——存量会议文档缺少会话字段，重启时按历史消息派生一次
	// （phase/target/speaker_turns/last_activity_at）。幂等、只读内存，不回写 Mongo。
	// 必须在 meetings / meetingMessages / meetingMessageIndex 全部赋值之后执行。
	s.backfillMeetingSessionUnsafe()
	return nil
}

// backfillMeetingSessionUnsafe 为「尚无语会话态」的存量会议按历史消息派生会话字段
// （T1.2）。调用方必须持有写锁，且 s.meetings / s.meetingMessages /
// s.meetingMessageIndex 已全部赋值。纯派生、幂等：已有会话态的会议
// （LastActivityAt 非零，即新写入路径已镜像过）直接跳过。
func (s *Store) backfillMeetingSessionUnsafe() {
	for id, m := range s.meetings {
		if m == nil || !m.LastActivityAt.IsZero() {
			continue
		}
		ids := s.meetingMessageIndex[id]
		if len(ids) == 0 {
			continue
		}
		msgs := make([]*model.MeetingMessage, 0, len(ids))
		for _, mid := range ids {
			if msg, ok := s.meetingMessages[mid]; ok && msg != nil {
				msgs = append(msgs, msg)
			}
		}
		// 索引顺序不保证按时间（Mongo 载入顺序），按 CreatedAt 排序后再派生，
		// 保证 LastPhase/LastTarget/LastActivityAt 取的是最后一条消息。
		sort.Slice(msgs, func(i, j int) bool { return msgs[i].CreatedAt.Before(msgs[j].CreatedAt) })
		for _, msg := range msgs {
			if msg.Phase != "" {
				m.LastPhase = msg.Phase
			}
			if msg.Target != "" {
				m.LastTarget = msg.Target
			}
			if msg.SenderType == "agent" {
				m.SpeakerTurns++
			}
			m.LastActivityAt = msg.CreatedAt
		}
	}
}

func (s *Store) loadUsers() (map[string]*model.User, error) {
	items := make(map[string]*model.User)
	if s.mongoUsers == nil {
		return items, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoUsers.Find(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []model.User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, err
	}
	for i := range users {
		user := users[i]
		items[user.ID] = copyUser(&user)
	}
	return items, nil
}

func (s *Store) loadAgents() (map[string]*model.Agent, error) {
	items := make(map[string]*model.Agent)
	if s.mongoAgents == nil {
		return items, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoAgents.Find(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var agents []model.Agent
	if err := cursor.All(ctx, &agents); err != nil {
		return nil, err
	}
	for i := range agents {
		agent := agents[i]
		items[agent.ID] = copyAgent(&agent)
	}
	return items, nil
}

func (s *Store) loadJoinRequests(users map[string]*model.User) (map[string]*model.JoinRequest, map[string][]string, map[string]string, error) {
	items := make(map[string]*model.JoinRequest)
	userJoinRequests := make(map[string][]string)
	trustRequestIndex := make(map[string]string)
	if s.mongoJoinRequests == nil {
		return items, userJoinRequests, trustRequestIndex, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoJoinRequests.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, nil, nil, err
	}
	defer cursor.Close(ctx)

	var joinRequests []model.JoinRequest
	if err := cursor.All(ctx, &joinRequests); err != nil {
		return nil, nil, nil, err
	}
	for i := range joinRequests {
		jr := joinRequests[i]
		items[jr.ID] = copyJoinRequest(&jr)
		trustRequestIndex[jr.TrustRequestID] = jr.ID
		if strings.TrimSpace(jr.UserID) != "" {
			if _, exists := users[jr.UserID]; exists {
				userJoinRequests[jr.UserID] = append(userJoinRequests[jr.UserID], jr.ID)
			}
			continue
		}
		for _, user := range users {
			userJoinRequests[user.ID] = append(userJoinRequests[user.ID], jr.ID)
		}
	}
	return items, userJoinRequests, trustRequestIndex, nil
}

func (s *Store) loadProjects() (map[string]*model.Project, error) {
	items := make(map[string]*model.Project)
	if s.mongoProjects == nil {
		return items, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoProjects.Find(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var projects []model.Project
	if err := cursor.All(ctx, &projects); err != nil {
		return nil, err
	}
	for i := range projects {
		project := projects[i]
		// T2.3：存量文档没有 version 字段，解码后为 0 → 归一化为 1，
		// 与库内（启动时已 backfill）保持一致，避免乐观锁 filter 失配。
		normalizeProjectVersion(&project)
		items[project.ID] = copyProject(&project)
	}
	return items, nil
}

func (s *Store) loadAgentChats() (map[string]*model.AgentChat, map[string]string, map[string]string, error) {
	items := make(map[string]*model.AgentChat)
	activeChats := make(map[string]string)
	bySession := make(map[string]string)
	if s.mongoAgentChats == nil {
		return items, activeChats, bySession, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoAgentChats.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, nil, nil, err
	}
	defer cursor.Close(ctx)

	var chats []model.AgentChat
	if err := cursor.All(ctx, &chats); err != nil {
		return nil, nil, nil, err
	}
	for i := range chats {
		chat := chats[i]
		chatCopy := chat
		chatCopy.Messages = copyAgentChatMessages(chat.Messages)
		items[chat.ID] = &chatCopy
		bySession[chat.SessionKey] = chat.ID
		if chat.Status == "active" {
			activeChats[activeAgentChatKey(chat.UserID, chat.AgentID)] = chat.ID
		}
	}
	return items, activeChats, bySession, nil
}

func (s *Store) loadTasks() (map[string]*model.TaskDetail, map[string][]string, error) {
	items := make(map[string]*model.TaskDetail)
	projectTasks := make(map[string][]string)
	if s.mongoTasks == nil {
		return items, projectTasks, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoTasks.Find(ctx, bson.D{})
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	var tasks []model.TaskDetail
	if err := cursor.All(ctx, &tasks); err != nil {
		return nil, nil, err
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
	for i := range tasks {
		task := tasks[i]
		// T2.2：存量文档没有 version 字段，解码后为 0 → 归一化为 1，
		// 与库内（启动时已 backfill）保持一致，避免乐观锁 filter 失配。
		normalizeTaskVersion(&task)
		items[task.ID] = copyTask(&task)
		projectTasks[task.ProjectID] = append(projectTasks[task.ProjectID], task.ID)
	}
	return items, projectTasks, nil
}

func (s *Store) loadEvents() (map[string][]model.Event, map[string][]*model.Event, map[string][]*model.Event, error) {
	taskEvents := make(map[string][]model.Event)
	userEvents := make(map[string][]*model.Event)
	agentEvents := make(map[string][]*model.Event)
	if s.mongoEvents == nil {
		return taskEvents, userEvents, agentEvents, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoEvents.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, nil, nil, err
	}
	defer cursor.Close(ctx)

	var events []model.Event
	if err := cursor.All(ctx, &events); err != nil {
		return nil, nil, nil, err
	}
	for i := range events {
		event := events[i]
		if event.TaskID != "" {
			taskEvents[event.TaskID] = append(taskEvents[event.TaskID], event)
		}
		if event.UserID != "" {
			userEvents[event.UserID] = append(userEvents[event.UserID], &events[i])
		}
		if event.ActorType == "agent" && event.ActorID != "" {
			agentEvents[event.ActorID] = append(agentEvents[event.ActorID], &events[i])
		}
	}
	return taskEvents, userEvents, agentEvents, nil
}

func (s *Store) loadNotifications() (map[string]*model.Notification, map[string][]string, error) {
	items := make(map[string]*model.Notification)
	userNotifications := make(map[string][]string)
	if s.mongoNotifications == nil {
		return items, userNotifications, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoNotifications.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	var notifications []model.Notification
	if err := cursor.All(ctx, &notifications); err != nil {
		return nil, nil, err
	}
	for i := range notifications {
		n := notifications[i]
		items[n.ID] = &n
		userNotifications[n.UserID] = append(userNotifications[n.UserID], n.ID)
	}
	return items, userNotifications, nil
}

func (s *Store) loadProcessedMessages() (map[string]processedMessage, error) {
	items := make(map[string]processedMessage)
	if s.mongoProcessedMessages == nil {
		return items, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoProcessedMessages.Find(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var records []processedMessageRecord
	if err := cursor.All(ctx, &records); err != nil {
		return nil, err
	}
	for _, record := range records {
		items[record.ID] = processedMessage{
			Action:     record.Action,
			ResourceID: record.ResourceID,
		}
	}
	return items, nil
}

// loadOpsIncidents 载入运维工单并建立三个内存索引。
// dedupe 索引只收活跃工单（终态历史不参与去重）；
// task 反查索引收全量（历史工单也要能按任务追溯）。
// 🔴 load 函数在锁外执行，只允许返回值，不允许直接写 s 字段。
func (s *Store) loadOpsIncidents() (map[string]*model.OpsIncident, map[string]string, map[string][]string, error) {
	items := make(map[string]*model.OpsIncident)
	byDedupe := make(map[string]string)
	byTask := make(map[string][]string)
	if s.mongoOpsIncidents == nil {
		return items, byDedupe, byTask, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoOpsIncidents.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, nil, nil, err
	}
	defer cursor.Close(ctx)

	var docs []model.OpsIncident
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, nil, nil, err
	}
	for i := range docs {
		doc := &docs[i]
		items[doc.ID] = doc
		if doc.IsActive() {
			// 理论上活跃 dedupe_key 唯一（部分唯一索引兜底）；
			// 若存量数据冲突，保留最新一条，避免启动失败。
			if prev, ok := byDedupe[doc.DedupeKey]; !ok || items[prev].CreatedAt.Before(doc.CreatedAt) {
				byDedupe[doc.DedupeKey] = doc.ID
			}
		}
		if doc.TaskID != "" {
			byTask[doc.TaskID] = append(byTask[doc.TaskID], doc.ID)
		}
	}
	return items, byDedupe, byTask, nil
}

// persistOpsIncidentUnsafe 持久化单条运维工单（全量替换，含 actions 数组）。
// 🔴 仅能在持锁的 *Unsafe 路径内调用。
func (s *Store) persistOpsIncidentUnsafe(inc *model.OpsIncident) error {
	if !s.mongoEnabled || s.mongoOpsIncidents == nil || inc == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoOpsIncidents.ReplaceOne(ctx, bson.M{"_id": inc.ID}, *inc, options.Replace().SetUpsert(true))
	return err
}

func (s *Store) persistUserUnsafe(user *model.User) error {
	if !s.mongoEnabled || s.mongoUsers == nil || user == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoUsers.ReplaceOne(ctx, bson.M{"_id": user.ID}, copyUser(user), options.Replace().SetUpsert(true))
	return err
}

func (s *Store) persistAgentUnsafe(agent *model.Agent) error {
	if !s.mongoEnabled || s.mongoAgents == nil || agent == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoAgents.ReplaceOne(ctx, bson.M{"_id": agent.ID}, copyAgent(agent), options.Replace().SetUpsert(true))
	return err
}

func (s *Store) persistJoinRequestUnsafe(jr *model.JoinRequest) error {
	if !s.mongoEnabled || s.mongoJoinRequests == nil || jr == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoJoinRequests.ReplaceOne(ctx, bson.M{"_id": jr.ID}, copyJoinRequest(jr), options.Replace().SetUpsert(true))
	return err
}

func (s *Store) deleteAgentUnsafe(agentID string) error {
	if !s.mongoEnabled || s.mongoAgents == nil || strings.TrimSpace(agentID) == "" {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoAgents.DeleteOne(ctx, bson.M{"_id": agentID})
	return err
}

// ─── T2.3：项目域「Mongo 权威」写序与乐观锁 ───
//
// 项目域自 T2.3 起改为 **Mongo 权威**：persistProjectUnsafe 升级为**权威提交原语**
// —— 先做带版本化乐观锁的 Mongo 写并确认，失败则**不产生内存副作用**（由调用方回滚）。
// 与会议域 T2.1 / 任务域 T2.2 同构：Mongo 写失败或版本冲突时**直接返错、内存零副作用**。
//
// 已知代价同 T2.1/T2.2：Mongo I/O 在 s.mu 持锁内执行，临界区被拉长（MONGO_TIMEOUT 5s）。

// projectVersionFloor 项目版本号起始值：新建项目从 1 开始；
// 「存量 version <= 0」的文档在载入 / 回填时统一归一化为 1，
// 保证乐观锁 filter（{_id, version}）永远有确定的基准值。
const projectVersionFloor = 1

// projectVersionConflictCode / projectVersionConflictMessage 是版本冲突出口共用的业务码
// （HTTP 409）与文案；调用方按 code 判定「要不要重试」，文案不参与判定。
const (
	projectVersionConflictCode    = "PROJECT_VERSION_CONFLICT"
	projectVersionConflictMessage = "project was modified concurrently, please retry"
)

// projectVersionConflict 构造统一形态的版本冲突错误。
func projectVersionConflict() *transport.AppError {
	return transport.Conflict(projectVersionConflictCode, projectVersionConflictMessage)
}

// isProjectVersionConflict 判定一个错误是否为版本冲突（调用方据此决定要不要重试）。
func isProjectVersionConflict(appErr *transport.AppError) bool {
	return appErr != nil && appErr.Code == projectVersionConflictCode
}

// normalizeProjectVersion 把缺失 / 非法的 version 归一化为 projectVersionFloor。
//
// 存量项目文档（T2.3 之前写入）没有 version 字段，解码后为 0；若不归一化，
// 后续带 {_id, version} filter 的更新永远匹配不上 —— 该项目会被永久锁死。
// 归一化只在内存解码 / 载入路径上做，库内真值由 backfillProjectVersions 补齐。
func normalizeProjectVersion(project *model.Project) {
	if project == nil {
		return
	}
	if project.Version < 1 {
		project.Version = projectVersionFloor
	}
}

// applyProjectVersionedReplaceLocked 在持锁内对项目执行一次带乐观锁的 Mongo 全量替换。
//
// 契约（调用方必须遵守）：
//   - filter = {_id: project.ID, version: cur}，全量 ReplaceOne 且 Upsert(true)；
//   - Mongo 报错         → mongoWriteError(500)，调用方须立即返回，**不得改任何内存字段**；
//   - 未命中（ModifiedCount==0 且 UpsertedCount==0）→ 见下方 healZeroVersionProjectLocked：
//     先排除「存量 version<=0 文档」这种可自愈形态，仍然不行才 409 PROJECT_VERSION_CONFLICT；
//   - 成功               → 内存对象的 Version 推进到 cur+1，调用方再写其余内存字段。
//
// 为什么 ModifiedCount==0 就能判定「未命中」：$set 里 version 恒从 cur 变成 cur+1，
// 因此「命中」必然「修改」，ModifiedCount==0 ⟺ filter 未命中。
func (s *Store) applyProjectVersionedReplaceLocked(project *model.Project) *transport.AppError {
	normalizeProjectVersion(project)
	cur := project.Version
	next := cur + 1

	// 纯内存模式（Mongo 未启用）：没有权威库可比，仍然推进版本以保持内存自洽。
	if !s.mongoEnabled || s.mongoProjects == nil {
		project.Version = next
		return nil
	}

	doc := copyProject(project)
	doc.Version = next

	// Mongo 写在持锁内，会拉长临界区 —— 本批接受，留待 T05 原子性整批迁移再拆细粒度锁。
	ctx, cancel := s.mongoContext()
	defer cancel()
	res, err := s.mongoProjects.ReplaceOne(ctx, bson.M{"_id": project.ID, "version": cur}, doc, options.Replace().SetUpsert(true))
	// Upsert 的 filter 含 _id：当文档已存在但 version 与 cur 不匹配时（真实并发落后，
	// 或滚动发布期间由旧镜像（无 Version 字段）写出的存量 version<=0 文档），服务端
	// **不会**返回 ModifiedCount==0，而是尝试插入一个同 _id 的新文档 → E11000 duplicate
	// key。必须把这种错误并入「未命中」分支（heal / 409 conflict），否则版本冲突会被
	// 误报成 500，且 heal 分支永远不可达。
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return mongoWriteError(err)
	}
	if err == nil && res != nil && (res.ModifiedCount > 0 || res.UpsertedCount > 0) {
		project.Version = next
		return nil
	}

	// 未命中除了「真实并发落后」，还有一种**一旦发生就永久锁死**的存量形态
	// （库内 version<=0 而内存已被归一化成 1），必须自愈 —— 见下面的注释。
	if appErr := s.healZeroVersionProjectLocked(project, cur); appErr != nil {
		return appErr
	}
	project.Version = next
	return nil
}

// healZeroVersionProjectLocked 处理 filter 未命中的**存量兼容**分支（对齐 T2.2）。
//
// 未命中（ModifiedCount == 0）有两种成因，必须区分：
//  1. 真实并发落后：库内 version 是合法的（> 0）但已被别的写方推进 → 维持 409，
//     不允许掩盖，否则就退化成「后写静默覆盖先写」，正是乐观锁要消灭的问题；
//  2. 存量非法文档：version 缺失 / 显式 0 / null / 负数。这类文档一旦被命中，
//     **每次**更新都会 409，而且**重启也救不回来** —— 启动时的一次性回填覆盖不到
//     滚动发布期间由旧镜像（无 Version 字段）新建出来的文档，等于该项目永久锁死。
//
// 对 ② 做一次性自愈：先把库内 version 修成内存认定的 cur（此刻库内的值本来就是非法的，
// 因此这次修不用带 version filter），再重试一次正常的版本化替换。
//
// 这是纯存量兼容修复，**不改变并发语义**：唯一被放宽的是「version <= 0」这种明确
// 非法的历史取值，正常写路径永远不会在库里留下 <= 0 的版本。
func (s *Store) healZeroVersionProjectLocked(project *model.Project, cur int) *transport.AppError {
	ctx, cancel := s.mongoContext()
	defer cancel()

	var doc struct {
		Version int `bson:"version"`
	}
	if err := s.mongoProjects.FindOne(ctx, bson.M{"_id": project.ID}).Decode(&doc); err != nil {
		// 文档不存在（或读取失败）→ 不是版本问题，维持冲突语义。
		return projectVersionConflict()
	}
	if doc.Version > 0 {
		// 合法但已落后 → 真实并发冲突，如实上报。
		return projectVersionConflict()
	}

	// 存量非法值：先修成 cur，再按正常路径重试一次。
	if _, err := s.mongoProjects.UpdateOne(ctx, bson.M{"_id": project.ID}, bson.M{"$set": bson.M{"version": cur}}); err != nil {
		return mongoWriteError(err)
	}
	retryDoc := copyProject(project)
	retryDoc.Version = cur + 1
	retry, err := s.mongoProjects.ReplaceOne(ctx, bson.M{"_id": project.ID, "version": cur}, retryDoc, options.Replace().SetUpsert(true))
	if err != nil {
		return mongoWriteError(err)
	}
	if retry == nil || (retry.ModifiedCount == 0 && retry.UpsertedCount == 0) {
		return projectVersionConflict()
	}
	return nil
}

// backfillProjectVersions 为「存量无合法 version」的项目文档补 version=1（T2.3）。
//
// 为什么要补：T2.3 起所有项目更新都带 {_id, version} 乐观锁 filter；存量文档解码后
// version 为 0（库里根本没这个字段），filter 永远匹配不上 → 该项目任何更新都判冲突，
// 等于生产全量写失败。
//
// filter 用 {"version": {"$not": {"$gt": 0}}} 而非 {"version": {"$exists": false}}：
// 后者漏掉了「显式 0 / null / 负数」以及**滚动发布期间由旧镜像（无 Version 字段）新建、
// 但本实例回填启动时点已过**的文档 —— 这类文档对当前进程同样永久不可写。一次性把
// 缺失 / null / 0 / 负数全部归一，避免永久锁死。
//
// 幂等：重复执行无副作用。失败只告警不阻断启动 —— 载入路径上的 normalizeProjectVersion
// 会把内存侧归一化，最坏退化成「更新报冲突」，而 applyProjectVersionedReplaceLocked 的
// healZeroVersionProjectLocked 还能在写入时再自愈一次，不会静默丢数据。
func (s *Store) backfillProjectVersions() {
	if !s.mongoEnabled || s.mongoProjects == nil {
		return
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	res, err := s.mongoProjects.UpdateMany(ctx,
		bson.M{"version": bson.M{"$not": bson.M{"$gt": 0}}},
		bson.M{"$set": bson.M{"version": projectVersionFloor}},
	)
	if err != nil {
		if s.log != nil {
			s.log.Warn("failed to backfill project version field", zap.Error(err))
		}
		return
	}
	if s.log != nil && res != nil && res.ModifiedCount > 0 {
		s.log.Info("backfilled project version field", zap.Int64("projects", res.ModifiedCount))
	}
}

// ─── T2.3b：项目文件域「Mongo 权威」写序与乐观锁 ───
//
// 项目文件域自 T2.3b 起与 Project 聚合（T2.3）逐行同构：persistProjectFileUnsafe 升级为
// **权威提交原语** —— 先做带版本化乐观锁的 Mongo 写并确认，失败则**不产生内存副作用**。
// 与会议域 T2.1 / 任务域 T2.2 / 项目域 T2.3 同构：Mongo 写失败或版本冲突时直接返错、
// 内存零副作用（由调用方回滚）。
//
// 为什么本批必须**全量**（含删除路径），而不是只改原语：
//  1. 只要存在任一不走版本化原语的写方，它就会绕过 filter 冲突检测 —— 乐观锁在多实例 /
//     滚动发布窗口下防的正是「另一个进程已推进 version」，半量实现会让这个防护静默失效；
//  2. 真正在丢数据的是「吞错」：rename/move/delete 失败即静默丢弃 → 重启丢失，delete 失败
//     则文件「复活」。要修这个必须致命化 + 零副作用回滚，而致命化与版本化必须是同一批。

// projectFileVersionFloor 项目文件版本号起始值：新建记录从 1 开始；
// 「存量 version <= 0」的文档在载入 / 回填时统一归一化为 1，
// 保证乐观锁 filter（{_id, version}）永远有确定的基准值。
const projectFileVersionFloor = 1

// projectFileVersionConflictCode / projectFileVersionConflictMessage 是版本冲突出口共用的
// 业务码（HTTP 409）与文案；调用方按 code 判定「要不要重试」，文案不参与判定。
const (
	projectFileVersionConflictCode    = "PROJECT_FILE_VERSION_CONFLICT"
	projectFileVersionConflictMessage = "project file was modified concurrently, please retry"
)

// projectFileConflict 构造统一形态的版本冲突错误。
func projectFileConflict() *transport.AppError {
	return transport.Conflict(projectFileVersionConflictCode, projectFileVersionConflictMessage)
}

// isProjectFileVersionConflict 判定一个错误是否为版本冲突（调用方据此决定要不要重试）。
func isProjectFileVersionConflict(appErr *transport.AppError) bool {
	return appErr != nil && appErr.Code == projectFileVersionConflictCode
}

// normalizeProjectFileVersion 把缺失 / 非法的 version 归一化为 projectFileVersionFloor。
//
// 存量项目文件文档（T2.3b 之前写入）没有 version 字段，解码后为 0；若不归一化，
// 后续带 {_id, version} filter 的更新永远匹配不上 —— 该记录会被永久锁死。
// 归一化只在内存解码 / 载入路径上做，库内真值由 backfillProjectFileVersions 补齐。
func normalizeProjectFileVersion(pf *model.ProjectFile) {
	if pf == nil {
		return
	}
	if pf.Version < 1 {
		pf.Version = projectFileVersionFloor
	}
}

// applyProjectFileVersionedReplaceLocked 在持锁内对项目文件执行一次带乐观锁的 Mongo 全量替换。
//
// 契约（调用方必须遵守）：
//   - filter = {_id: pf.ID, version: cur}，全量 ReplaceOne 且 Upsert(true)；
//   - Mongo 报错         → mongoWriteError(500)，调用方须立即返回，**不得改任何内存字段**；
//   - 未命中（ModifiedCount==0 且 UpsertedCount==0）→ 见 healZeroVersionProjectFileLocked：
//     先排除「存量 version<=0 文档」这种可自愈形态，仍然不行才 409 PROJECT_FILE_VERSION_CONFLICT；
//   - 成功               → 内存对象的 Version 推进到 cur+1，调用方再写其余内存字段。
//
// 为什么不写「非锁路径会把 version 改小」这种论断（对初稿论证的更正）：本原语写入的
// doc.Version 恒为 cur+1（从内存取当前值推导），且只在成功时才把内存 version 推进 ——
// 内存 version 因此单调不减，不存在「拿陈旧副本覆盖」的写方。所以非锁路径至多写出
// 「与 Mongo 相同或更大」的 version，真正的问题是「吞错」造成的短期分叉 / 删除复活，
// 以及半量实现会让乐观锁 filter 静默失效（见本节顶部注释）。
func (s *Store) applyProjectFileVersionedReplaceLocked(pf *model.ProjectFile) *transport.AppError {
	normalizeProjectFileVersion(pf)
	cur := pf.Version
	next := cur + 1

	// 纯内存模式（Mongo 未启用）：没有权威库可比，仍然推进版本以保持内存自洽。
	if !s.mongoEnabled || s.mongoProjectFiles == nil {
		pf.Version = next
		return nil
	}

	doc := copyProjectFile(pf)
	doc.Version = next

	// Mongo 写在持锁内，会拉长临界区 —— 本批接受（同 T2.1/T2.2/T2.3），
	// 留待 T05 原子性整批迁移再拆细粒度锁。
	ctx, cancel := s.mongoContext()
	defer cancel()
	res, err := s.mongoProjectFiles.ReplaceOne(ctx, bson.M{"_id": pf.ID, "version": cur}, doc, options.Replace().SetUpsert(true))
	// Upsert 的 filter 含 _id：当文档已存在但 version 与 cur 不匹配时（真实并发落后，
	// 或滚动发布期间由旧镜像（无 Version 字段）写出的存量 version<=0 文档），服务端
	// **不会**返回 ModifiedCount==0，而是尝试插入一个同 _id 的新文档 → E11000 duplicate
	// key。必须把这种错误并入「未命中」分支（heal / 409 conflict），否则版本冲突会被
	// 误报成 500，且 heal 分支永远不可达。
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return mongoWriteError(err)
	}
	if err == nil && res != nil && (res.ModifiedCount > 0 || res.UpsertedCount > 0) {
		pf.Version = next
		return nil
	}

	// 未命中除了「真实并发落后」，还有一种**一旦发生就永久锁死**的存量形态
	// （库内 version<=0 而内存已被归一化成 1），必须自愈 —— 见 healZeroVersionProjectFileLocked。
	if appErr := s.healZeroVersionProjectFileLocked(pf, cur); appErr != nil {
		return appErr
	}
	pf.Version = next
	return nil
}

// healZeroVersionProjectFileLocked 处理 filter 未命中的**存量兼容**分支（对齐 T2.2/T2.3）。
//
// 未命中（ModifiedCount == 0）有两种成因，必须区分：
//  1. 真实并发落后：库内 version 是合法的（> 0）但已被别的写方推进 → 维持 409，
//     不允许掩盖，否则就退化成「后写静默覆盖先写」，正是乐观锁要消灭的问题；
//  2. 存量非法文档：version 缺失 / 显式 0 / null / 负数。这类文档一旦被命中，
//     **每次**更新都会 409，而且**重启也救不回来** —— 启动时的一次性回填覆盖不到
//     滚动发布期间由旧镜像（无 Version 字段）新建出来的文档，等于该记录永久锁死。
//
// 对 ② 做一次性自愈：先把库内 version 修成内存认定的 cur（此刻库内的值本来就是非法的，
// 因此这次修不用带 version filter），再重试一次正常的版本化替换。
//
// 这是纯存量兼容修复，**不改变并发语义**：唯一被放宽的是「version <= 0」这种明确
// 非法的历史取值，正常写路径永远不会在库里留下 <= 0 的版本。
func (s *Store) healZeroVersionProjectFileLocked(pf *model.ProjectFile, cur int) *transport.AppError {
	ctx, cancel := s.mongoContext()
	defer cancel()

	var doc struct {
		Version int `bson:"version"`
	}
	if err := s.mongoProjectFiles.FindOne(ctx, bson.M{"_id": pf.ID}).Decode(&doc); err != nil {
		// 文档不存在（或读取失败）→ 不是版本问题，维持冲突语义。
		return projectFileConflict()
	}
	if doc.Version > 0 {
		// 合法但已落后 → 真实并发冲突，如实上报。
		return projectFileConflict()
	}

	// 存量非法值：先修成 cur，再按正常路径重试一次。
	if _, err := s.mongoProjectFiles.UpdateOne(ctx, bson.M{"_id": pf.ID}, bson.M{"$set": bson.M{"version": cur}}); err != nil {
		return mongoWriteError(err)
	}
	retryDoc := copyProjectFile(pf)
	retryDoc.Version = cur + 1
	retry, err := s.mongoProjectFiles.ReplaceOne(ctx, bson.M{"_id": pf.ID, "version": cur}, retryDoc, options.Replace().SetUpsert(true))
	if err != nil {
		return mongoWriteError(err)
	}
	if retry == nil || (retry.ModifiedCount == 0 && retry.UpsertedCount == 0) {
		return projectFileConflict()
	}
	return nil
}

// backfillProjectFileVersions 为「存量无合法 version」的项目文件文档补 version=1（T2.3b）。
//
// 为什么要补：T2.3b 起所有项目文件更新都带 {_id, version} 乐观锁 filter；存量文档解码后
// version 为 0（库里根本没这个字段），filter 永远匹配不上 → 该记录任何更新都判冲突，
// 等于生产全量写失败。
//
// filter 用 {"version": {"$not": {"$gt": 0}}} 而非 {"version": {"$exists": false}}：
// 后者漏掉了「显式 0 / null / 负数」以及**滚动发布期间由旧镜像（无 Version 字段）新建、
// 但本实例回填启动时点已过**的文档 —— 这类文档对当前进程同样永久不可写。一次性把
// 缺失 / null / 0 / 负数全部归一，避免永久锁死。
//
// 幂等：重复执行无副作用。失败只告警不阻断启动 —— 载入路径上的 normalizeProjectFileVersion
// 会把内存侧归一化，最坏退化成「更新报冲突」，而 applyProjectFileVersionedReplaceLocked 的
// healZeroVersionProjectFileLocked 还能在写入时再自愈一次，不会静默丢数据。
func (s *Store) backfillProjectFileVersions() {
	if !s.mongoEnabled || s.mongoProjectFiles == nil {
		return
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	res, err := s.mongoProjectFiles.UpdateMany(ctx,
		bson.M{"version": bson.M{"$not": bson.M{"$gt": 0}}},
		bson.M{"$set": bson.M{"version": projectFileVersionFloor}},
	)
	if err != nil {
		if s.log != nil {
			s.log.Warn("failed to backfill project file version field", zap.Error(err))
		}
		return
	}
	if s.log != nil && res != nil && res.ModifiedCount > 0 {
		s.log.Info("backfilled project file version field", zap.Int64("project_files", res.ModifiedCount))
	}
}

// VerifyProjectFileConsistency 校验内存项目文件缓存与 Mongo 文档是否一致（T2.3b 双写过渡期）。
//
// 返回 (检查条数, 不一致条数, 错误)。生产可调用、**无副作用**：只在 RLock 下取一份内存
// 快照，然后逐条 FindOne 比对，不写任何集合。过渡期应当恒为 mismatched == 0。
// Mongo 未启用时返回 (0, 0, nil) —— 没有第二份数据可比。
//
// len(children) 的比对放在本函数（而非 projectFileSnapshotMatches）：子节点不存在于单条
// ProjectFile 结构内，只能靠集合内 parent_id 计数，故对每个文件夹记录额外比对两侧子节点数。
func (s *Store) VerifyProjectFileConsistency() (int, int, error) {
	if !s.mongoEnabled || s.mongoProjectFiles == nil {
		return 0, 0, nil
	}

	s.mu.RLock()
	snapshot := make([]model.ProjectFile, 0, len(s.projectFiles))
	memoryChildCount := make(map[string]int)
	for _, pf := range s.projectFiles {
		if pf == nil {
			continue
		}
		snapshot = append(snapshot, *pf)
		if pf.ParentID != "" {
			memoryChildCount[pf.ParentID]++
		}
	}
	s.mu.RUnlock()

	ctx, cancel := s.mongoContext()
	defer cancel()

	mismatched := 0
	for i := range snapshot {
		want := snapshot[i]
		var got model.ProjectFile
		err := s.mongoProjectFiles.FindOne(ctx, bson.M{"_id": want.ID}).Decode(&got)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				// 内存有、库里没有 —— 正是改造前「重启即丢」的分叉形态。
				mismatched++
				continue
			}
			return len(snapshot), mismatched, err
		}
		if !projectFileSnapshotMatches(want, got) {
			mismatched++
			continue
		}
		// 文件夹额外比对子节点数（仅在两侧都存在该文件夹文档时，避免与「文档缺失」重复计数）。
		if want.IsFolder {
			mongoChildren, err := s.mongoProjectFiles.CountDocuments(ctx, bson.M{"parent_id": want.ID})
			if err != nil {
				return len(snapshot), mismatched, err
			}
			if int(mongoChildren) != memoryChildCount[want.ID] {
				mismatched++
			}
		}
	}
	return len(snapshot), mismatched, nil
}

// projectFileSnapshotMatches 判定一份内存快照与 Mongo 文档的关键字段是否一致。
//
// 只比写路径会改动的**稳定**字段。刻意**不比 LocalPath**：它在字节写盘之后由
// SetProjectFileLocalPath 单独一步设置，落库与内存之间可能短暂不同步 → 比了会制造假 mismatch。
// 刻意**不比 CreatedAt**：BSON Date 只有毫秒精度，而内存 time.Now() 带纳秒，序列化落库
// 再读回必然截断到毫秒 → `Equal` 恒判不等（T2.1/T2.2/T2.3 同款结论）。
// len(children) 不是单条结构内可得的字段，改由 VerifyProjectFileConsistency 依 parent_id 计数比对。
func projectFileSnapshotMatches(want, got model.ProjectFile) bool {
	return want.ProjectID == got.ProjectID &&
		want.ParentID == got.ParentID &&
		want.FileName == got.FileName &&
		want.Version == got.Version &&
		want.IsFolder == got.IsFolder
}

// ─── 双写校验（T2.3 过渡期） ───

// VerifyProjectConsistency 校验内存项目缓存与 Mongo 文档是否一致（T2.3 双写过渡期）。
//
// 返回 (检查条数, 不一致条数, 错误)。生产可调用、**无副作用**：只在 RLock 下取一份
// 内存快照，然后逐条 FindOne 比对，不写任何集合。
// 过渡期应当恒为 mismatched == 0；非 0 表示内存与 Mongo 已经分叉，需要人工介入。
// Mongo 未启用时返回 (0, 0, nil) —— 没有第二份数据可比。
func (s *Store) VerifyProjectConsistency() (int, int, error) {
	if !s.mongoEnabled || s.mongoProjects == nil {
		return 0, 0, nil
	}

	s.mu.RLock()
	snapshot := make([]model.Project, 0, len(s.projects))
	for _, p := range s.projects {
		if p != nil {
			snapshot = append(snapshot, *p)
		}
	}
	s.mu.RUnlock()

	ctx, cancel := s.mongoContext()
	defer cancel()

	mismatched := 0
	for i := range snapshot {
		want := snapshot[i]
		var got model.Project
		err := s.mongoProjects.FindOne(ctx, bson.M{"_id": want.ID}).Decode(&got)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				// 内存有、库里没有 —— 正是改造前「重启即丢」的分叉形态。
				mismatched++
				continue
			}
			return len(snapshot), mismatched, err
		}
		if !projectSnapshotMatches(want, got) {
			mismatched++
		}
	}
	return len(snapshot), mismatched, nil
}

// projectSnapshotMatches 判定一份内存快照与 Mongo 文档的关键字段是否一致。
//
// 只比写路径会改动的**稳定**字段；workflows 只比条数（数组深比会引入排序 / 时间的噪声）。
// 刻意**不比 TaskSummary**：库内该字段恒为零值（只在 buildProjectViewUnsafe 的 view 克隆上
// 重算，从不写回存储对象）→ 比了必然全量假 mismatch。
// 刻意**不比 UpdatedAt**：BSON Date 只有毫秒精度，而内存 time.Now() 带纳秒，序列化落库
// 再读回必然截断到毫秒 → `Equal` 恒判不等，会制造假 mismatch（T2.1/T2.2 同款结论）。
func projectSnapshotMatches(want, got model.Project) bool {
	return want.Status == got.Status &&
		want.Version == got.Version &&
		want.Name == got.Name &&
		want.PMAgentID == got.PMAgentID &&
		len(want.Workflows) == len(got.Workflows)
}

// persistProjectUnsafe 是项目域的**权威提交原语**（T2.3）：
//
//	① 先对项目文档做带乐观锁的版本化 Mongo 全量替换并确认（applyProjectVersionedReplaceLocked）；
//	② 成功后才由调用方（mutateProjectUnsafe / 各写路径）落实内存；失败则**不产生内存副作用**。
//
// 🔴 签名是 *transport.AppError（而非 error）：调用方必须以 `if appErr := ...; appErr != nil`
// 显式判空。若把「nil *transport.AppError」直接装进 error 接口会得到**非 nil**（Go typed-nil
// 陷阱，T2.2 曾因此让 CreateTaskByPMNode 返回 (nil, nil) 并 panic 3 个测试）。
//
// 注：persistFailForTest 注入检查放在 Mongo 启用判定**之前**，这样纯内存用例
// （New() + persistFailForTest=true）也能触发失败，用于验证「失败零副作用」。
func (s *Store) persistProjectUnsafe(project *model.Project) *transport.AppError {
	if s.persistFailForTest {
		return mongoWriteError(fmt.Errorf("injected project persist failure"))
	}
	if !s.mongoEnabled || s.mongoProjects == nil || project == nil {
		return nil
	}
	return s.applyProjectVersionedReplaceLocked(project)
}

func (s *Store) persistAgentChatUnsafe(chat *model.AgentChat) error {
	if !s.mongoEnabled || s.mongoAgentChats == nil || chat == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	clone := *chat
	clone.Messages = copyAgentChatMessages(chat.Messages)
	_, err := s.mongoAgentChats.ReplaceOne(ctx, bson.M{"_id": chat.ID}, clone, options.Replace().SetUpsert(true))
	return err
}
func (s *Store) persistTaskUnsafe(task *model.TaskDetail) error {
	if task == nil {
		return nil
	}
	// T2.2：委托给带乐观锁的版本化替换（权威提交原语）。persistAgentGraphUnsafe 等
	// 直接调用点也会因此自动获得版本锁保护。
	//
	// 注意：本函数返回 error（interface），而 applyTaskVersionedReplaceLocked 返回
	// *transport.AppError。**不能**直接 `return s.applyTaskVersionedReplaceLocked(task)`——
	// 成功时它返回的是「nil 指针」，装进 error 接口后 **非 nil**（Go typed-nil 陷阱），
	// 调用方会误判为失败（曾导致 CreateTaskByPMNode 返回 (nil, nil)）。必须显式判空。
	if s.persistFailForTest {
		return fmt.Errorf("injected task persist failure")
	}
	if appErr := s.applyTaskVersionedReplaceLocked(task); appErr != nil {
		return appErr
	}
	return nil
}

func (s *Store) persistTaskEventsUnsafe(taskID string) error {
	if !s.mongoEnabled || s.mongoEvents == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	if _, err := s.mongoEvents.DeleteMany(ctx, bson.M{"task_id": taskID}); err != nil {
		return err
	}
	events := s.taskEvents[taskID]
	if len(events) == 0 {
		return nil
	}
	docs := make([]any, 0, len(events))
	for _, event := range events {
		docs = append(docs, event)
	}
	_, err := s.mongoEvents.InsertMany(ctx, docs)
	return err
}

func (s *Store) persistNotificationUnsafe(n *model.Notification) {
	if !s.mongoEnabled || s.mongoNotifications == nil || n == nil {
		return
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	clone := *n
	if _, err := s.mongoNotifications.ReplaceOne(ctx, bson.M{"_id": n.ID}, clone, options.Replace().SetUpsert(true)); err != nil {
		if s.log != nil {
			s.log.Warn("failed to persist notification", zap.String("id", n.ID), zap.Error(err))
		}
	}
}

func (s *Store) persistProcessedMessageUnsafe(key string) error {
	if !s.mongoEnabled || s.mongoProcessedMessages == nil || key == "" {
		return nil
	}
	record, ok := s.processedMessages[key]
	if !ok {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoProcessedMessages.ReplaceOne(ctx, bson.M{"_id": key}, processedMessageRecord{
		ID:         key,
		Action:     record.Action,
		ResourceID: record.ResourceID,
	}, options.Replace().SetUpsert(true))
	return err
}

func (s *Store) persistAgentGraphUnsafe(agentID string) error {
	agent, ok := s.agents[agentID]
	if !ok {
		return nil
	}
	if err := s.persistAgentUnsafe(agent); err != nil {
		return err
	}
	for _, project := range s.projects {
		if project.PMAgentID == agentID {
			if appErr := s.persistProjectUnsafe(project); appErr != nil {
				return appErr
			}
		}
	}
	for _, task := range s.tasks {
		needsPersist := task.PMAgentID == agentID
		if !needsPersist {
			for _, todo := range task.Todos {
				if todo.Assignee.AgentID == agentID {
					needsPersist = true
					break
				}
			}
		}
		if needsPersist {
			if err := s.persistTaskUnsafe(task); err != nil {
				return err
			}
		}
	}
	return nil
}

// persistTaskBundleUnsafe 是任务域的**权威提交原语**（T2.2）：
//
//	① 先对任务文档做带乐观锁的版本化 Mongo 全量替换并确认（applyTaskVersionedReplaceLocked）；
//	② 成功后才由调用方（mutateTaskUnsafe / 各写路径）落实内存；失败则**不产生内存副作用**。
//	③ 任务文档落库成功后，再持久化事件流（persistTaskEventsUnsafe）。事件是派生量、
//	   可经 history 重算，单条失败仅告警、不阻断本次提交（与会议域 T2.1 同理）。
//
// 注意：本函数不在持锁语义之外；调用方必须已在 s.mu 持锁内（*Unsafe 约定）。
func (s *Store) persistTaskBundleUnsafe(taskID string) error {
	if s.persistFailForTest {
		return fmt.Errorf("injected task persist failure")
	}
	task, ok := s.tasks[taskID]
	if !ok {
		return nil
	}
	if appErr := s.applyTaskVersionedReplaceLocked(task); appErr != nil {
		return appErr
	}
	if err := s.persistTaskEventsUnsafe(taskID); err != nil {
		if s.log != nil {
			s.log.Warn("failed to persist task events (derived, re-syncable)",
				zap.String("task_id", taskID), zap.Error(err))
		}
	}
	return nil
}

// ─── T2.2：任务域「Mongo 权威」写序与乐观锁 ───
//
// 任务域自 T2.2 起改为 **Mongo 权威**：所有写路径一律「先落库成功，再改内存」——
// 通过 persistTaskBundleUnsafe 统一提交原语实现。与会议域 T2.1 同理，Mongo 写
// 失败或版本冲突时**直接返错、内存零副作用**。
//
// 已知代价同 T2.1：Mongo I/O 在 s.mu 持锁内执行，临界区被拉长（MONGO_TIMEOUT 5s）。

// taskVersionFloor 任务版本号起始值：新建任务从 1 开始；
// 「存量 version <= 0」的文档在载入 / 回填时统一归一化为 1，
// 保证乐观锁 filter（{_id, version}）永远有确定的基准值。
const taskVersionFloor = 1

// taskVersionConflictCode / taskVersionConflictMessage 是版本冲突出口共用的业务码
// （HTTP 409）与文案；调用方按 code 判定「要不要重试」，文案不参与判定。
const (
	taskVersionConflictCode    = "TASK_VERSION_CONFLICT"
	taskVersionConflictMessage = "task was modified concurrently, please retry"
)

// taskVersionConflict 构造统一形态的版本冲突错误。
func taskVersionConflict() *transport.AppError {
	return transport.Conflict(taskVersionConflictCode, taskVersionConflictMessage)
}

// isTaskVersionConflict 判定一个错误是否为版本冲突（调用方据此决定要不要重试）。
func isTaskVersionConflict(appErr *transport.AppError) bool {
	return appErr != nil && appErr.Code == taskVersionConflictCode
}

// normalizeTaskVersion 把缺失 / 非法的 version 归一化为 taskVersionFloor。
//
// 存量任务文档（T2.2 之前写入）没有 version 字段，解码后为 0；若不归一化，
// 后续带 {_id, version} filter 的更新永远匹配不上 —— 该任务会被永久锁死。
// 归一化只在内存解码 / 载入路径上做，库内真值由 backfillTaskVersions 补齐。
func normalizeTaskVersion(task *model.TaskDetail) {
	if task == nil {
		return
	}
	if task.Version < 1 {
		task.Version = taskVersionFloor
	}
}

// applyTaskVersionedReplaceLocked 在持锁内对任务执行一次带乐观锁的 Mongo 全量替换。
//
// 契约（调用方必须遵守）：
//   - filter = {_id: task.ID, version: cur}，全量 ReplaceOne 且 Upsert(true)；
//   - Mongo 报错         → mongoWriteError(500)，调用方须立即返回，**不得改任何内存字段**；
//   - 未命中（ModifiedCount==0 且 UpsertedCount==0）→ 见下方 healZeroVersionTaskLocked：
//     先排除「存量 version<=0 文档」这种可自愈形态，仍然不行才 409 TASK_VERSION_CONFLICT；
//   - 成功               → 内存对象的 Version 推进到 cur+1，调用方再写其余内存字段。
//
// 为什么 ModifiedCount==0 就能判定「未命中」：$set 里 version 恒从 cur 变成 cur+1，
// 因此「命中」必然「修改」，ModifiedCount==0 ⟺ filter 未命中。
func (s *Store) applyTaskVersionedReplaceLocked(task *model.TaskDetail) *transport.AppError {
	normalizeTaskVersion(task)
	cur := task.Version
	next := cur + 1

	// 纯内存模式（Mongo 未启用）：没有权威库可比，仍然推进版本以保持内存自洽。
	if !s.mongoEnabled || s.mongoTasks == nil {
		task.Version = next
		return nil
	}

	doc := copyTask(task)
	doc.Version = next

	// Mongo 写在持锁内，会拉长临界区 —— 本批接受，留待 T05 原子性整批迁移再拆细粒度锁。
	ctx, cancel := s.mongoContext()
	defer cancel()
	res, err := s.mongoTasks.ReplaceOne(ctx, bson.M{"_id": task.ID, "version": cur}, doc, options.Replace().SetUpsert(true))
	// Upsert 的 filter 含 _id：当文档已存在但 version 与 cur 不匹配时（真实并发落后，
	// 或滚动发布期间由旧镜像（无 Version 字段）写出的存量 version<=0 文档），服务端
	// **不会**返回 ModifiedCount==0，而是尝试插入一个同 _id 的新文档 → E11000 duplicate
	// key。必须把这种错误并入「未命中」分支（heal / 409 conflict），否则版本冲突会被
	// 误报成 500，且 heal 分支永远不可达。
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return mongoWriteError(err)
	}
	if err == nil && res != nil && (res.ModifiedCount > 0 || res.UpsertedCount > 0) {
		task.Version = next
		return nil
	}

	// 未命中除了「真实并发落后」，还有一种**一旦发生就永久锁死**的存量形态
	// （库内 version<=0 而内存已被归一化成 1），必须自愈 —— 见下面的注释。
	if appErr := s.healZeroVersionTaskLocked(task, cur); appErr != nil {
		return appErr
	}
	task.Version = next
	return nil
}

// healZeroVersionTaskLocked 处理 filter 未命中的**存量兼容**分支（P1）。
//
// 未命中（ModifiedCount == 0）有两种成因，必须区分：
//  1. 真实并发落后：库内 version 是合法的（> 0）但已被别的写方推进 → 维持 409，
//     不允许掩盖，否则就退化成「后写静默覆盖先写」，正是乐观锁要消灭的问题；
//  2. 存量非法文档：version 缺失 / 显式 0 / null / 负数。这类文档一旦被命中，
//     **每次**更新都会 409，而且**重启也救不回来** —— 启动时的一次性回填覆盖不到
//     滚动发布期间由旧镜像（无 Version 字段）新建出来的文档，等于该任务永久锁死。
//
// 对 ② 做一次性自愈：先把库内 version 修成内存认定的 cur（此刻库内的值本来就是非法的，
// 因此这次修不用带 version filter），再重试一次正常的版本化替换。
//
// 这是纯存量兼容修复，**不改变并发语义**：唯一被放宽的是「version <= 0」这种明确
// 非法的历史取值，正常写路径永远不会在库里留下 <= 0 的版本。
func (s *Store) healZeroVersionTaskLocked(task *model.TaskDetail, cur int) *transport.AppError {
	ctx, cancel := s.mongoContext()
	defer cancel()

	var doc struct {
		Version int `bson:"version"`
	}
	if err := s.mongoTasks.FindOne(ctx, bson.M{"_id": task.ID}).Decode(&doc); err != nil {
		// 文档不存在（或读取失败）→ 不是版本问题，维持冲突语义。
		return taskVersionConflict()
	}
	if doc.Version > 0 {
		// 合法但已落后 → 真实并发冲突，如实上报。
		return taskVersionConflict()
	}

	// 存量非法值：先修成 cur，再按正常路径重试一次。
	if _, err := s.mongoTasks.UpdateOne(ctx, bson.M{"_id": task.ID}, bson.M{"$set": bson.M{"version": cur}}); err != nil {
		return mongoWriteError(err)
	}
	retryDoc := copyTask(task)
	retryDoc.Version = cur + 1
	retry, err := s.mongoTasks.ReplaceOne(ctx, bson.M{"_id": task.ID, "version": cur}, retryDoc, options.Replace().SetUpsert(true))
	if err != nil {
		return mongoWriteError(err)
	}
	if retry == nil || (retry.ModifiedCount == 0 && retry.UpsertedCount == 0) {
		return taskVersionConflict()
	}
	return nil
}

// backfillTaskVersions 为「存量无合法 version」的任务文档补 version=1（T2.2）。
//
// 为什么要补：T2.2 起所有任务更新都带 {_id, version} 乐观锁 filter；存量文档解码后
// version 为 0（库里根本没这个字段），filter 永远匹配不上 → 该任务任何更新都判冲突，
// 等于生产全量写失败。
//
// filter 用 {"version": {"$not": {"$gt": 0}}} 而非 {"version": {"$exists": false}}：
// 后者漏掉了「显式 0 / null / 负数」以及**滚动发布期间由旧镜像（无 Version 字段）新建、
// 但本实例回填启动时点已过**的文档 —— 这类文档对当前进程同样永久不可写。一次性把
// 缺失 / null / 0 / 负数全部归一，避免永久锁死。
//
// 幂等：重复执行无副作用。失败只告警不阻断启动 —— 载入路径上的 normalizeTaskVersion
// 会把内存侧归一化，最坏退化成「更新报冲突」，而 applyTaskVersionedReplaceLocked 的
// healZeroVersionTaskLocked 还能在写入时再自愈一次，不会静默丢数据。
func (s *Store) backfillTaskVersions() {
	if !s.mongoEnabled || s.mongoTasks == nil {
		return
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	res, err := s.mongoTasks.UpdateMany(ctx,
		bson.M{"version": bson.M{"$not": bson.M{"$gt": 0}}},
		bson.M{"$set": bson.M{"version": taskVersionFloor}},
	)
	if err != nil {
		if s.log != nil {
			s.log.Warn("failed to backfill task version field", zap.Error(err))
		}
		return
	}
	if s.log != nil && res != nil && res.ModifiedCount > 0 {
		s.log.Info("backfilled task version field", zap.Int64("tasks", res.ModifiedCount))
	}
}

// ─── 双写校验（T2.2 过渡期） ───

// VerifyTaskConsistency 校验内存任务缓存与 Mongo 文档是否一致（T2.2 双写过渡期）。
//
// 返回 (检查条数, 不一致条数, 错误)。生产可调用、**无副作用**：只在 RLock 下取一份
// 内存快照，然后逐条 FindOne 比对，不写任何集合。
// 过渡期应当恒为 mismatched == 0；非 0 表示内存与 Mongo 已经分叉，需要人工介入。
// Mongo 未启用时返回 (0, 0, nil) —— 没有第二份数据可比。
func (s *Store) VerifyTaskConsistency() (int, int, error) {
	if !s.mongoEnabled || s.mongoTasks == nil {
		return 0, 0, nil
	}

	s.mu.RLock()
	snapshot := make([]model.TaskDetail, 0, len(s.tasks))
	for _, t := range s.tasks {
		if t != nil {
			snapshot = append(snapshot, *t)
		}
	}
	s.mu.RUnlock()

	ctx, cancel := s.mongoContext()
	defer cancel()

	mismatched := 0
	for i := range snapshot {
		want := snapshot[i]
		var got model.TaskDetail
		err := s.mongoTasks.FindOne(ctx, bson.M{"_id": want.ID}).Decode(&got)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				// 内存有、库里没有 —— 正是改造前「重启即丢」的分叉形态。
				mismatched++
				continue
			}
			return len(snapshot), mismatched, err
		}
		if !taskSnapshotMatches(want, got) {
			mismatched++
		}
	}
	return len(snapshot), mismatched, nil
}

// taskSnapshotMatches 判定一份内存快照与 Mongo 文档的关键字段是否一致。
//
// 只比写路径会改动的**稳定**字段；todos 只比条数（数组深比会引入排序 / 时间的噪声）。
// 刻意**不比 UpdatedAt**：BSON Date 只有毫秒精度，而内存 time.Now() 带纳秒，序列化落库
// 再读回必然截断到毫秒 → `Equal` 恒判不等，会制造假 mismatch（会议域 T2.1 的
// meetingSnapshotMatches 同样刻意避开时间字段）。
func taskSnapshotMatches(want, got model.TaskDetail) bool {
	return want.Status == got.Status &&
		want.Version == got.Version &&
		want.Title == got.Title &&
		want.ProjectID == got.ProjectID &&
		len(want.Todos) == len(got.Todos)
}

func (s *Store) loadComments() (map[string][]model.Comment, error) {
	taskComments := make(map[string][]model.Comment)
	if s.mongoComments == nil {
		return taskComments, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoComments.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var comments []model.Comment
	if err := cursor.All(ctx, &comments); err != nil {
		return nil, err
	}
	for _, c := range comments {
		taskComments[c.TaskID] = append(taskComments[c.TaskID], c)
	}
	return taskComments, nil
}

func (s *Store) persistCommentUnsafe(c *model.Comment) error {
	if !s.mongoEnabled || s.mongoComments == nil || c == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoComments.ReplaceOne(ctx, bson.M{"_id": c.ID}, c, options.Replace().SetUpsert(true))
	return err
}

func (s *Store) loadKnowledgeDocs() (map[string]*model.KnowledgeDocument, map[string][]string, error) {
	items := make(map[string]*model.KnowledgeDocument)
	userDocs := make(map[string][]string)
	if s.mongoKnowledgeDocs == nil {
		return items, userDocs, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoKnowledgeDocs.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	var docs []model.KnowledgeDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, nil, err
	}
	for i := range docs {
		doc := docs[i]
		items[doc.ID] = &doc
		userDocs[doc.UserID] = append(userDocs[doc.UserID], doc.ID)
	}
	return items, userDocs, nil
}

func (s *Store) persistKnowledgeDocUnsafe(doc *model.KnowledgeDocument) error {
	if !s.mongoEnabled || s.mongoKnowledgeDocs == nil || doc == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoKnowledgeDocs.ReplaceOne(ctx, bson.M{"_id": doc.ID}, doc, options.Replace().SetUpsert(true))
	return err
}

func (s *Store) deleteKnowledgeDocUnsafe(docID string) error {
	if !s.mongoEnabled || s.mongoKnowledgeDocs == nil || docID == "" {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoKnowledgeDocs.DeleteOne(ctx, bson.M{"_id": docID})
	return err
}

func (s *Store) loadWorkflowTemplates() (map[string]*model.WorkflowTemplate, map[string][]string, error) {
	items := make(map[string]*model.WorkflowTemplate)
	userTemplates := make(map[string][]string)
	if s.mongoWorkflowTemplates == nil {
		return items, userTemplates, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoWorkflowTemplates.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	var docs []model.WorkflowTemplate
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, nil, err
	}
	for i := range docs {
		doc := docs[i]
		items[doc.ID] = &doc
		userTemplates[doc.UserID] = append(userTemplates[doc.UserID], doc.ID)
	}
	return items, userTemplates, nil
}

func (s *Store) persistWorkflowTemplateUnsafe(doc *model.WorkflowTemplate) error {
	if !s.mongoEnabled || s.mongoWorkflowTemplates == nil || doc == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoWorkflowTemplates.ReplaceOne(ctx, bson.M{"_id": doc.ID}, doc, options.Replace().SetUpsert(true))
	return err
}

func (s *Store) deleteWorkflowTemplateUnsafe(templateID string) error {
	if !s.mongoEnabled || s.mongoWorkflowTemplates == nil || templateID == "" {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoWorkflowTemplates.DeleteOne(ctx, bson.M{"_id": templateID})
	return err
}

func (s *Store) loadMeetings() (map[string]*model.Meeting, map[string][]string, error) {
	items := make(map[string]*model.Meeting)
	projectIdx := make(map[string][]string)
	if s.mongoMeetings == nil {
		return items, projectIdx, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoMeetings.Find(ctx, bson.D{})
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	var meetings []model.Meeting
	if err := cursor.All(ctx, &meetings); err != nil {
		return nil, nil, err
	}
	for i := range meetings {
		m := &meetings[i]
		// T2.1：存量文档没有 version 字段，解码后为 0 → 归一化为 1，
		// 与库内（启动时已 backfill）保持一致，避免乐观锁 filter 失配。
		normalizeMeetingVersion(m)
		items[m.ID] = m
		projectIdx[m.ProjectID] = append(projectIdx[m.ProjectID], m.ID)
	}
	return items, projectIdx, nil
}

// backfillMeetingVersions 为「存量无合法 version」的会议文档补 version=1（T2.1）。
//
// 为什么要补：T2.1 起所有会议更新都带 {_id, version} 乐观锁 filter；存量文档解码后
// version 为 0（库里根本没这个字段），filter 永远匹配不上 → 该会议任何更新都判冲突，
// 等于生产全量写失败。
//
// filter 用 {"version": {"$not": {"$gt": 0}}} 而非 {"version": {"$exists": false}}：
// 后者漏掉了「显式 0 / null / 负数」以及**滚动发布期间由旧镜像（无 Version 字段）新建、
// 但本实例回填启动时点已过**的文档 —— 这类文档对当前进程同样永久不可写。一次性把
// 缺失 / null / 0 / 负数全部归一，避免永久锁死。
//
// 幂等：重复执行无副作用。失败只告警不阻断启动 —— 载入路径上的 normalizeMeetingVersion
// 会把内存侧归一化，最坏退化成「更新报冲突」，而 applyMeetingVersionedUpdateLocked 的
// healZeroVersionMeetingLocked 还能在写入时再自愈一次，不会静默丢数据。
func (s *Store) backfillMeetingVersions() {
	if !s.mongoEnabled || s.mongoMeetings == nil {
		return
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	res, err := s.mongoMeetings.UpdateMany(ctx,
		bson.M{"version": bson.M{"$not": bson.M{"$gt": 0}}},
		bson.M{"$set": bson.M{"version": meetingVersionFloor}},
	)
	if err != nil {
		if s.log != nil {
			s.log.Warn("failed to backfill meeting version field", zap.Error(err))
		}
		return
	}
	if s.log != nil && res != nil && res.ModifiedCount > 0 {
		s.log.Info("backfilled meeting version field", zap.Int64("meetings", res.ModifiedCount))
	}
}

func (s *Store) loadMeetingMessages() (map[string]*model.MeetingMessage, map[string][]string, error) {
	items := make(map[string]*model.MeetingMessage)
	meetingIdx := make(map[string][]string)
	if s.mongoMeetingMessages == nil {
		return items, meetingIdx, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoMeetingMessages.Find(ctx, bson.D{})
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	var msgs []model.MeetingMessage
	if err := cursor.All(ctx, &msgs); err != nil {
		return nil, nil, err
	}
	for i := range msgs {
		msg := &msgs[i]
		items[msg.ID] = msg
		meetingIdx[msg.MeetingID] = append(meetingIdx[msg.MeetingID], msg.ID)
	}
	return items, meetingIdx, nil
}

func (s *Store) deleteKnowledgeChunksUnsafe(docID string) error {
	if !s.mongoEnabled || s.mongoKnowledgeChunks == nil || docID == "" {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoKnowledgeChunks.DeleteMany(ctx, bson.M{"document_id": docID})
	return err
}

func (s *Store) persistKnowledgeChunksUnsafe(chunks []model.KnowledgeChunk) error {
	if !s.mongoEnabled || s.mongoKnowledgeChunks == nil || len(chunks) == 0 {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	docs := make([]any, len(chunks))
	for i := range chunks {
		docs[i] = chunks[i]
	}
	_, err := s.mongoKnowledgeChunks.InsertMany(ctx, docs)
	return err
}

func (s *Store) loadArtifacts() (map[string][]model.TaskArtifact, error) {
	taskArtifacts := make(map[string][]model.TaskArtifact)
	if s.mongoArtifacts == nil {
		return taskArtifacts, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoArtifacts.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var artifacts []model.TaskArtifact
	if err := cursor.All(ctx, &artifacts); err != nil {
		return nil, err
	}
	for _, a := range artifacts {
		taskArtifacts[a.TaskID] = append(taskArtifacts[a.TaskID], a)
	}
	return taskArtifacts, nil
}

func (s *Store) persistArtifactUnsafe(artifact *model.TaskArtifact) error {
	if !s.mongoEnabled || s.mongoArtifacts == nil || artifact == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoArtifacts.ReplaceOne(ctx, bson.M{"_id": artifact.TransferID}, artifact, options.Replace().SetUpsert(true))
	return err
}

func (s *Store) loadProjectFiles() (map[string]*model.ProjectFile, map[string][]string, map[string]string, error) {
	files := make(map[string]*model.ProjectFile)
	index := make(map[string][]string)
	transferIndex := make(map[string]string)
	if s.mongoProjectFiles == nil {
		return files, index, transferIndex, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoProjectFiles.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, nil, nil, err
	}
	defer cursor.Close(ctx)

	var projectFiles []model.ProjectFile
	if err := cursor.All(ctx, &projectFiles); err != nil {
		return nil, nil, nil, err
	}
	for i := range projectFiles {
		pf := projectFiles[i]
		files[pf.ID] = &pf
		index[pf.ProjectID] = append(index[pf.ProjectID], pf.ID)
		if pf.TransferID != "" {
			transferIndex[pf.TransferID] = pf.ID
		}
	}
	return files, index, transferIndex, nil
}

// persistProjectFileUnsafe 是项目文件域的**权威提交原语**（T2.3b）：
//
//	① 先对记录做带乐观锁的版本化 Mongo 全量替换并确认（applyProjectFileVersionedReplaceLocked）；
//	② 成功后才由调用方（mutateProjectFileUnsafe / 各写路径）落实内存；失败则**不产生内存副作用**。
//
// 🔴 签名是 *transport.AppError（而非 error）：调用方必须以 `if appErr := ...; appErr != nil`
// 显式判空。若把「nil *transport.AppError」直接装进 error 接口会得到**非 nil**（Go typed-nil
// 陷阱，T2.2 曾因此让 CreateTaskByPMNode 返回 (nil, nil) 并 panic 3 个测试）。
//
// 注：persistFailForTest 注入检查放在 Mongo 启用判定**之前**，这样纯内存用例
// （New() + persistFailForTest=true）也能触发失败，用于验证「失败零副作用」。
func (s *Store) persistProjectFileUnsafe(pf *model.ProjectFile) *transport.AppError {
	if s.persistFailForTest {
		return mongoWriteError(fmt.Errorf("injected project file persist failure"))
	}
	if !s.mongoEnabled || s.mongoProjectFiles == nil || pf == nil {
		return nil
	}
	return s.applyProjectFileVersionedReplaceLocked(pf)
}

func (s *Store) deleteProjectFileUnsafe(fileID string) error {
	if !s.mongoEnabled || s.mongoProjectFiles == nil || fileID == "" {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoProjectFiles.DeleteOne(ctx, bson.M{"_id": fileID})
	return err
}

// FlushPersistAll 在停机时把全内存状态有界地回写 Mongo。
//
// 语义：TrustMesh 后端是「全内存状态机 + Mongo 仅作持久化镜像」。正常写入路径会
// 即时落盘，但 Mongo 抖动 / 不可用期间产生的内存写入会滞留在内存里，进程重启后
// 永久丢失。本函数在收到停机信号、HTTP 流量已 drain 之后、断开 Mongo 之前，做一次
// 尽力而为的全量回写，补齐这段差距。
//
// 有界性（关键）：ctx 到期即提前返回（fail-fast），绝不无限挂起。若 Mongo 不可用，
// 编排层最终会 SIGKILL，那反而必丢；因此宁可提前放弃，也不能拖到被强杀。单条写操作
// 自身由 s.mongoContext()（= s.mongoTimeout）+ client SetTimeout 兜底，不会永久阻塞；
// 但整轮 sweep 必须尊重传入的 ctx。
//
// 锁：全程持 s.mu，因为调用的是不带锁的 persistXxxUnsafe / replaceXxxUnsafe 助手。
//
// 返回：errors.Join 聚合各条写入错误；全部成功返回 nil；ctx 到期返回 ctx.Err() 并
// 立即中断整轮 sweep（不继续后续实体）。
func (s *Store) FlushPersistAll(ctx context.Context) error {
	// 未启用 Mongo（或未连接）时没有镜像可回写，直接成功返回（no-op）。
	if !s.mongoEnabled || s.mongoClient == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	start := time.Now()
	var flushErr error
	defer func() {
		metrics.Observe(metrics.ShutdownFlushDuration, time.Since(start))
		if flushErr != nil {
			metrics.Inc(metrics.ShutdownFlushFailedTotal)
		}
	}()

	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []error
	// collect 聚合单个实体写入的错误：单条失败不能中断整轮 sweep。
	collect := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}
	// finish 汇总并回写命名返回值，使上面的 defer 能看到最终结果。
	finish := func() error {
		flushErr = errors.Join(errs...)
		return flushErr
	}

	// sweeps 是「一类实体」的回写闭包，按依赖无关的稳定顺序排列。
	// 注意：每一类开始前由调用方做 ctx 有界性检查（见下方循环）。
	sweeps := []func(){
		func() { // users
			for _, user := range s.users {
				collect(s.persistUserUnsafe(user))
			}
		},
		func() { // agents
			for _, agent := range s.agents {
				collect(s.persistAgentUnsafe(agent))
			}
		},
		func() { // joinRequests
			for _, jr := range s.joinRequests {
				collect(s.persistJoinRequestUnsafe(jr))
			}
		},
		func() { // projects
			for _, project := range s.projects {
				// 显式判空是硬性要求：persistProjectUnsafe 返回 *transport.AppError，
				// 若裸传给 collect(error) 会把「成功的 nil 指针」当成非 nil 错误记一笔。
				if appErr := s.persistProjectUnsafe(project); appErr != nil {
					collect(appErr)
				}
			}
		},
		func() { // agentChats
			for _, chat := range s.agentChats {
				collect(s.persistAgentChatUnsafe(chat))
			}
		},
		func() { // tasks（含每个 task 的事件流）
			for taskID := range s.tasks {
				collect(s.persistTaskBundleUnsafe(taskID))
			}
		},
		func() { // comments
			for _, comments := range s.taskComments {
				for i := range comments {
					collect(s.persistCommentUnsafe(&comments[i]))
				}
			}
		},
		func() { // artifacts
			for _, artifacts := range s.taskArtifacts {
				for i := range artifacts {
					collect(s.persistArtifactUnsafe(&artifacts[i]))
				}
			}
		},
		func() { // projectFiles
			for _, pf := range s.projectFiles {
				// 显式判空是硬性要求：persistProjectFileUnsafe 返回 *transport.AppError，
				// 若裸传给 collect(error) 会把「成功的 nil 指针」当成非 nil 错误记一笔，
				// 从而给每一次成功写入都记一个假错误（typed-nil 陷阱）。
				if appErr := s.persistProjectFileUnsafe(pf); appErr != nil {
					collect(appErr)
				}
			}
		},
		func() { // knowledgeDocs
			for _, doc := range s.knowledgeDocs {
				collect(s.persistKnowledgeDocUnsafe(doc))
			}
		},
		func() { // workflowTemplates
			for _, doc := range s.workflowTemplates {
				collect(s.persistWorkflowTemplateUnsafe(doc))
			}
		},
		func() { // organizations
			for _, org := range s.organizations {
				collect(s.persistOrganizationUnsafe(org))
			}
		},
		func() { // orgMemberships
			for _, m := range s.orgMemberships {
				collect(s.persistMembershipUnsafe(m))
			}
		},
		func() { // projectMembers（按项目整体替换）
			for projectID, members := range s.projectMembers {
				collect(s.replaceProjectMembersUnsafe(projectID, members))
			}
		},
		func() { // opsIncidents
			for _, inc := range s.opsIncidents {
				collect(s.persistOpsIncidentUnsafe(inc))
			}
		},
		func() { // llmConfigs（平台设置）
			for _, cfg := range s.llmConfigs {
				collect(s.persistLLMSettingUnsafe(cfg))
			}
		},
		func() { // processedMessages（消息去重）
			for key := range s.processedMessages {
				collect(s.persistProcessedMessageUnsafe(key))
			}
		},
		func() { // notifications（助手无返回值，内部自记日志）
			for _, n := range s.notifications {
				s.persistNotificationUnsafe(n)
			}
		},
		func() { // externalApps
			for _, app := range s.externalApps {
				collect(s.persistExternalAppUnsafe(app))
			}
		},
	}

	for _, sweep := range sweeps {
		// 有界性：每一类实体开始前检查一次，ctx 到期立即中断整轮 sweep。
		// 到期直接返回 ctx.Err()（fail-fast 语义），不再继续后续实体。
		if err := ctx.Err(); err != nil {
			flushErr = err
			return err
		}
		sweep()
	}
	// 循环结束后再看一次，覆盖「最后一类写完后恰好到期」的情形。
	if err := ctx.Err(); err != nil {
		flushErr = err
		return err
	}
	return finish()
}
