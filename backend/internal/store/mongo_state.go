package store

import (
	"context"
	"errors"
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
	projects, err := s.loadProjects()
	if err != nil {
		return err
	}
	agentChats, activeAgentChats, agentChatBySession, err := s.loadAgentChats()
	if err != nil {
		return err
	}
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

func (s *Store) persistProjectUnsafe(project *model.Project) error {
	if !s.mongoEnabled || s.mongoProjects == nil || project == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoProjects.ReplaceOne(ctx, bson.M{"_id": project.ID}, copyProject(project), options.Replace().SetUpsert(true))
	return err
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
	if !s.mongoEnabled || s.mongoTasks == nil || task == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoTasks.ReplaceOne(ctx, bson.M{"_id": task.ID}, copyTask(task), options.Replace().SetUpsert(true))
	return err
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
			if err := s.persistProjectUnsafe(project); err != nil {
				return err
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

func (s *Store) persistTaskBundleUnsafe(taskID string) error {
	task, ok := s.tasks[taskID]
	if !ok {
		return nil
	}
	if err := s.persistTaskUnsafe(task); err != nil {
		return err
	}
	return s.persistTaskEventsUnsafe(taskID)
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
		items[m.ID] = m
		projectIdx[m.ProjectID] = append(projectIdx[m.ProjectID], m.ID)
	}
	return items, projectIdx, nil
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

func (s *Store) persistProjectFileUnsafe(pf *model.ProjectFile) error {
	if !s.mongoEnabled || s.mongoProjectFiles == nil || pf == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoProjectFiles.ReplaceOne(ctx, bson.M{"_id": pf.ID}, pf, options.Replace().SetUpsert(true))
	return err
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
				collect(s.persistProjectUnsafe(project))
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
				collect(s.persistProjectFileUnsafe(pf))
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
