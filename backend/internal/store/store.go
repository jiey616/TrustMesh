package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/project"
	"trustmesh/backend/internal/transport"
)

type Store struct {
	mu sync.RWMutex
	// dispatchHook is set by the app layer to re-dispatch a todo (e.g.
	dispatchHook func(ctx context.Context, taskID, todoID string)
	// remindHook is set by the app layer to send a timeout reminder to a
	// todo's assignee WITHOUT re-dispatching the todo (which would cause
	// duplicate executions). Unlike dispatchHook it only nudges the agent.
	remindHook func(ctx context.Context, taskID, todoID string)

	// cancelNotifyHook is set by the app layer to notify assignee nodes that
	// a task's todos were canceled while possibly in flight (adapter lifecycle
	// cancel chain: node stops the active run). Invoked OUTSIDE the store lock
	// with a plain snapshot of the canceled todos.
	cancelNotifyHook func(taskID string, taskVersion int, notices []model.TodoCancelNotice)

	streamMu sync.RWMutex

	users       map[string]*model.User
	usersByMail map[string]string

	agents      map[string]*model.Agent
	agentByNode map[string]string

	projects map[string]*model.Project

	agentChats         map[string]*model.AgentChat
	activeAgentChats   map[string]string
	agentChatBySession map[string]string
	// planRejectNotify 记录「任务+规划被拒指纹」已自动催 PM 的次数（进程内节流）。
	planRejectNotify map[string]int
	// planningStallHook is set by the app layer to nudge the PM when a task
	// has been stuck in planning (no finalized plan, PM owes a response).
	// Runs outside the store lock.
	planningStallHook func(ctx context.Context, taskID string)
	// planningStallCount / planningStallLastRemind are in-memory per-task
	// throttle state for the planning-stall monitor.
	planningStallCount      map[string]int
	planningStallLastRemind map[string]time.Time
	// autoAdvanceStreak counts, per task and assignee agent, how many
	// CONSECUTIVE steps the platform had to advance on that agent's behalf
	// (the T0.7c promotions). Reaching agentStallStreakThreshold trips the
	// agent compliance sentinel (T0.11③). Guarded by s.mu.
	autoAdvanceStreak map[string]map[string]int
	tasks              map[string]*model.TaskDetail
	projectTasks       map[string][]string
	taskEvents         map[string][]model.Event
	userEvents         map[string][]*model.Event
	agentEvents        map[string][]*model.Event
	orgEvents          map[string][]*model.Event // 多租户：orgID → 该租户活动流（与 userEvents 共享事件指针）

	// 统一干预编排器（ops_dispatcher.go）：实际推送与 LLM 归因由 app 层注入
	// （store 不能 import clawsynapse/assistant，与 remindHook 同款注入模式）。
	opsPublishHook     func(ctx context.Context, req OpsPublishRequest) error
	opsAttributionHook func(ctx context.Context, inc *model.OpsIncident, snapshot string) (string, error)

	// 运维工单（ops_incidents）：与其余资源一致，全内存状态机 + Mongo 持久化镜像
	llmConfigs     map[string]*model.PlatformLLMSetting // orgID(""=平台默认) → LLM 配置
	llmEnvURL      string                        // env 兜底（bootstrap 注入）
	llmEnvKey      string
	llmEnvModel    string
	opsIncidents   map[string]*model.OpsIncident // 工单 ID → 工单
	opsByDedupeKey map[string]string             // dedupeKey → 工单 ID（活跃工单去重）
	opsByTask      map[string][]string           // taskID → 工单 ID 列表
	opsClearSince  map[string]time.Time          // 工单 ID → 规则不再满足的观察起点（内存即可，重启后重新观察）
	opsRuntime     opsRuntime                    // 运维扫描运行参数（bootstrap 注入）
	opsSuppress    map[string]time.Time          // dedupeKey → 升级抑制窗截止时间（内存即可）
	processedMessages  map[string]processedMessage

	taskArtifacts map[string][]model.TaskArtifact // taskID → []TaskArtifact

	externalApps map[string]*model.ExternalApp // externalAppID → ExternalApp

	taskComments map[string][]model.Comment

	notifications     map[string]*model.Notification
	userNotifications map[string][]string

	joinRequests      map[string]*model.JoinRequest // joinRequestID → JoinRequest
	userJoinRequests  map[string][]string           // userID → []joinRequestID
	trustRequestIndex map[string]string             // trustRequestID → joinRequestID

	knowledgeDocs     map[string]*model.KnowledgeDocument
	userKnowledgeDocs map[string][]string // userID → []docID

	workflowTemplates     map[string]*model.WorkflowTemplate
	userWorkflowTemplates map[string][]string // userID → []templateID

	projectFiles      map[string]*model.ProjectFile // fileID → ProjectFile
	projectFileIndex  map[string][]string           // projectID → []fileID
	transferFileIndex map[string]string             // transferID → fileID

	// 多租户阶段 0：仅建模与骨架，不参与任何业务判断
	organizations  map[string]*model.Organization
	orgMemberships map[string]*model.OrgMembership
	orgMemberIndex map[string][]string // orgID → []membershipID
	userOrgIndex   map[string][]string // userID → []membershipID
	projectMembers map[string][]model.ProjectMember

	meetings            map[string]*model.Meeting // meetingID → Meeting
	projectMeetings     map[string][]string       // projectID → []meetingID
	meetingMessages     map[string]*model.MeetingMessage
	meetingMessageIndex map[string][]string // meetingID → []messageID

	// fileStorage is used by the timeout monitor to auto-generate meeting
	// minutes when a meeting is force-concluded without a host upload.
	fileStorage project.FileStorage

	mongoEnabled           bool
	mongoClient            *mongo.Client
	mongoUsers             *mongo.Collection
	mongoAgents            *mongo.Collection
	mongoJoinRequests      *mongo.Collection
	mongoProjects          *mongo.Collection
	mongoAgentChats        *mongo.Collection
	mongoTasks             *mongo.Collection
	mongoEvents            *mongo.Collection
	mongoComments          *mongo.Collection
	mongoProcessedMessages *mongo.Collection
	mongoNotifications     *mongo.Collection
	mongoArtifacts         *mongo.Collection
	mongoExternalApps      *mongo.Collection
	mongoKnowledgeDocs     *mongo.Collection
	mongoKnowledgeChunks   *mongo.Collection
	mongoProjectFiles      *mongo.Collection
	mongoMeetings          *mongo.Collection
	mongoMeetingMessages   *mongo.Collection
	mongoOrganizations  *mongo.Collection
	mongoOrgMemberships *mongo.Collection
	mongoProjectMembers *mongo.Collection
	mongoWorkflowTemplates *mongo.Collection
	mongoOpsIncidents      *mongo.Collection
	mongoLLMSettings      *mongo.Collection
	mongoTimeout           time.Duration
	log                    *zap.Logger

	userSubscribers map[string]map[chan model.UserStreamEvent]struct{}
}

// SetFileStorage injects the project file storage, used by the timeout monitor
// to auto-generate meeting minutes when a meeting is force-concluded without a
// host-uploaded summary.
func (s *Store) SetFileStorage(fs project.FileStorage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fileStorage = fs
}

type processedMessage struct {
	Action     string `bson:"action" json:"action"`
	ResourceID string `bson:"resource_id" json:"resource_id"`
}

type AgentPresence struct {
	NodeID     string
	LastSeenAt time.Time
}

// SetDispatchHook registers a callback used to re-dispatch a todo (e.g. when
// the timeout monitor retries a stuck todo). The hook runs outside the store
// lock and should publish todo.assigned to the assignee.
func (s *Store) SetDispatchHook(hook func(ctx context.Context, taskID, todoID string)) {
	s.dispatchHook = hook
}

// SetRemindHook registers a callback used to send a timeout reminder to a
// todo's assignee WITHOUT re-dispatching the todo (unlike dispatchHook). The
// hook runs outside the store lock.
func (s *Store) SetRemindHook(hook func(ctx context.Context, taskID, todoID string)) {
	s.remindHook = hook
}

// SetCancelNotifyHook registers a callback used to notify assignee nodes about
// canceled todos so their in-flight runs can be stopped. The hook runs outside
// the store lock.
func (s *Store) SetCancelNotifyHook(hook func(taskID string, taskVersion int, notices []model.TodoCancelNotice)) {
	s.cancelNotifyHook = hook
}

// SetPlanningStallHook registers a callback used to nudge the PM when a task
// has been stuck in planning without a finalized plan. The hook runs outside
// the store lock.
func (s *Store) SetPlanningStallHook(hook func(ctx context.Context, taskID string)) {
	s.planningStallHook = hook
}

func New() *Store {
	return &Store{
		users:              make(map[string]*model.User),
		usersByMail:        make(map[string]string),
		agents:             make(map[string]*model.Agent),
		agentByNode:        make(map[string]string),
		projects:           make(map[string]*model.Project),
		agentChats:         make(map[string]*model.AgentChat),
		activeAgentChats:   make(map[string]string),
		agentChatBySession: make(map[string]string),
		planRejectNotify:   make(map[string]int),
		planningStallCount:      make(map[string]int),
		planningStallLastRemind: make(map[string]time.Time),
		autoAdvanceStreak:       make(map[string]map[string]int),
		tasks:              make(map[string]*model.TaskDetail),
		projectTasks:       make(map[string][]string),
		taskEvents:         make(map[string][]model.Event),
		userEvents:         make(map[string][]*model.Event),
		agentEvents:        make(map[string][]*model.Event),
		orgEvents:          make(map[string][]*model.Event),
		llmConfigs:         make(map[string]*model.PlatformLLMSetting),
		opsIncidents:       make(map[string]*model.OpsIncident),
		opsByDedupeKey:     make(map[string]string),
		opsByTask:          make(map[string][]string),
		opsClearSince:      make(map[string]time.Time),
		opsSuppress:        make(map[string]time.Time),
		processedMessages:  make(map[string]processedMessage),
		taskArtifacts:      make(map[string][]model.TaskArtifact),
		taskComments:       make(map[string][]model.Comment),
		notifications:      make(map[string]*model.Notification),
		userNotifications:  make(map[string][]string),
		joinRequests:       make(map[string]*model.JoinRequest),
		userJoinRequests:   make(map[string][]string),
		trustRequestIndex:  make(map[string]string),
		knowledgeDocs:      make(map[string]*model.KnowledgeDocument),
		userKnowledgeDocs:  make(map[string][]string),
		workflowTemplates:  make(map[string]*model.WorkflowTemplate),
		userWorkflowTemplates: make(map[string][]string),
		projectFiles:       make(map[string]*model.ProjectFile),
		projectFileIndex:   make(map[string][]string),
		transferFileIndex:  make(map[string]string),
		externalApps:       make(map[string]*model.ExternalApp),

		organizations:  make(map[string]*model.Organization),
		orgMemberships: make(map[string]*model.OrgMembership),
		orgMemberIndex: make(map[string][]string),
		userOrgIndex:   make(map[string][]string),
		projectMembers: make(map[string][]model.ProjectMember),
		meetings:            make(map[string]*model.Meeting),
		projectMeetings:     make(map[string][]string),
		meetingMessages:     make(map[string]*model.MeetingMessage),
		meetingMessageIndex: make(map[string][]string),
		userSubscribers:     make(map[string]map[chan model.UserStreamEvent]struct{}),
	}
}

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		panic(errors.New("failed to generate id"))
	}
	return hex.EncodeToString(buf)
}

func copyUser(u *model.User) *model.User {
	clone := *u
	return &clone
}

func copyAgent(a *model.Agent) *model.Agent {
	clone := *a
	if a.Capabilities != nil {
		clone.Capabilities = append([]string(nil), a.Capabilities...)
	}
	if a.LastSeenAt != nil {
		t := *a.LastSeenAt
		clone.LastSeenAt = &t
	}
	return &clone
}

func copyJoinRequest(jr *model.JoinRequest) *model.JoinRequest {
	clone := *jr
	if jr.Capabilities != nil {
		clone.Capabilities = append([]string(nil), jr.Capabilities...)
	}
	clone.Metadata = copyMap(jr.Metadata)
	if jr.ResolvedAt != nil {
		t := *jr.ResolvedAt
		clone.ResolvedAt = &t
	}
	return &clone
}

func copyProject(p *model.Project) *model.Project {
	clone := *p
	if p.TaskSummary.LatestTaskAt != nil {
		t := *p.TaskSummary.LatestTaskAt
		clone.TaskSummary.LatestTaskAt = &t
	}
	if len(p.Workflows) > 0 {
		clone.Workflows = make([]model.Workflow, len(p.Workflows))
		for i := range p.Workflows {
			if c := p.Workflows[i].Clone(); c != nil {
				clone.Workflows[i] = *c
			}
		}
	}
	return &clone
}

func copyTask(t *model.TaskDetail) *model.TaskDetail {
	clone := *t
	clone.Messages = copyTaskMessages(t.Messages)
	clone.Todos = make([]model.Todo, len(t.Todos))
	for i := range t.Todos {
		clone.Todos[i] = t.Todos[i]
		clone.Todos[i].Questions = append([]model.TodoQuestion{}, t.Todos[i].Questions...)
		clone.Todos[i].Outputs = append([]model.TodoOutput{}, t.Todos[i].Outputs...)
		clone.Todos[i].Result.Metadata = copyMap(t.Todos[i].Result.Metadata)
	}
	clone.Artifacts = append([]model.TaskArtifact{}, t.Artifacts...)
	clone.Result = model.TaskResult{
		Summary:     t.Result.Summary,
		FinalOutput: t.Result.FinalOutput,
		Metadata:    copyMap(t.Result.Metadata),
	}
	if t.CanceledAt != nil {
		at := *t.CanceledAt
		clone.CanceledAt = &at
	}
	if t.CanceledBy != nil {
		actor := *t.CanceledBy
		clone.CanceledBy = &actor
	}
	if t.CancelReason != nil {
		reason := *t.CancelReason
		clone.CancelReason = &reason
	}
	for i := range clone.Todos {
		if clone.Todos[i].StartedAt != nil {
			at := *clone.Todos[i].StartedAt
			clone.Todos[i].StartedAt = &at
		}
		if clone.Todos[i].CompletedAt != nil {
			at := *clone.Todos[i].CompletedAt
			clone.Todos[i].CompletedAt = &at
		}
		if clone.Todos[i].FailedAt != nil {
			at := *clone.Todos[i].FailedAt
			clone.Todos[i].FailedAt = &at
		}
		if clone.Todos[i].CanceledAt != nil {
			at := *clone.Todos[i].CanceledAt
			clone.Todos[i].CanceledAt = &at
		}
		if clone.Todos[i].AssignedAt != nil {
			at := *clone.Todos[i].AssignedAt
			clone.Todos[i].AssignedAt = &at
		}
		if clone.Todos[i].Error != nil {
			errCopy := *clone.Todos[i].Error
			clone.Todos[i].Error = &errCopy
		}
		if clone.Todos[i].CancelReason != nil {
			reason := *clone.Todos[i].CancelReason
			clone.Todos[i].CancelReason = &reason
		}
		clone.Todos[i].Result.Metadata = copyMap(clone.Todos[i].Result.Metadata)
	}
	return &clone
}

func copyTaskMessages(messages []model.TaskMessage) []model.TaskMessage {
	if len(messages) == 0 {
		return nil
	}
	clone := make([]model.TaskMessage, len(messages))
	for i := range messages {
		clone[i] = messages[i]
		clone[i].UIBlocks = copyUIBlocks(messages[i].UIBlocks)
		clone[i].UIResponse = copyUIResponse(messages[i].UIResponse)
	}
	return clone
}

func copyUIBlocks(blocks []model.UIBlock) []model.UIBlock {
	if len(blocks) == 0 {
		return nil
	}
	clone := make([]model.UIBlock, len(blocks))
	for i := range blocks {
		clone[i] = blocks[i]
		clone[i].Options = append([]model.UIBlockOption{}, blocks[i].Options...)
		clone[i].Default = append([]string{}, blocks[i].Default...)
		if blocks[i].Required != nil {
			required := *blocks[i].Required
			clone[i].Required = &required
		}
	}
	return clone
}

func copyUIResponse(resp *model.UIResponse) *model.UIResponse {
	if resp == nil {
		return nil
	}
	clone := &model.UIResponse{Blocks: make(map[string]model.UIBlockResponse, len(resp.Blocks))}
	for key, value := range resp.Blocks {
		copied := value
		copied.Selected = append([]string{}, value.Selected...)
		if value.Confirmed != nil {
			confirmed := *value.Confirmed
			copied.Confirmed = &confirmed
		}
		clone.Blocks[key] = copied
	}
	return clone
}

func copyMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mongoWriteError(err error) *transport.AppError {
	return &transport.AppError{
		Status:  500,
		Code:    "INTERNAL_ERROR",
		Message: "failed to persist state",
		Details: map[string]any{"cause": err.Error()},
	}
}
