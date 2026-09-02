package model

import "time"

type PMAgentSummary struct {
	ID     string `json:"id" bson:"id"`
	Name   string `json:"name" bson:"name"`
	NodeID string `json:"node_id" bson:"node_id"`
	Status string `json:"status" bson:"status"`
}

type Agent struct {
	ID           string     `json:"id" bson:"_id"`
	UserID       string     `json:"-" bson:"user_id"`
	Name         string     `json:"name" bson:"name"`
	Description  string     `json:"description" bson:"description"`
	Role         string     `json:"role" bson:"role"`
	Capabilities []string   `json:"capabilities" bson:"capabilities"`
	NodeID       string     `json:"node_id" bson:"node_id"`
	Product      string     `json:"product" bson:"product"` // 产品标识：trustmesh(默认)/hermes/opc，来自 JoinRequest.AgentProduct 审批同步
	Status       string     `json:"status" bson:"status"`
	Archived     bool       `json:"archived" bson:"archived"`
	LastSeenAt   *time.Time `json:"last_seen_at" bson:"last_seen_at"`
	Usage        AgentUsage `json:"usage" bson:"-"`
	CreatedAt    time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" bson:"updated_at"`
}

type AgentUsage struct {
	ProjectCount int  `json:"project_count" bson:"-"`
	TaskCount    int  `json:"task_count" bson:"-"`
	TodoCount    int  `json:"todo_count" bson:"-"`
	TotalCount   int  `json:"total_count" bson:"-"`
	InUse        bool `json:"in_use" bson:"-"`
}

type AgentStats struct {
	Role string `json:"role"`

	// 执行者维度（developer/reviewer）
	TodosTotal          int      `json:"todos_total"`
	TodosDone           int      `json:"todos_done"`
	TodosFailed         int      `json:"todos_failed"`
	TodosInProgress     int      `json:"todos_in_progress"`
	TodosPending        int      `json:"todos_pending"`
	SuccessRate         float64  `json:"success_rate"`
	AvgResponseTimeMs   *float64 `json:"avg_response_time_ms"`
	AvgCompletionTimeMs *float64 `json:"avg_completion_time_ms"`

	// PM 维度
	ProjectsManaged int     `json:"projects_managed"`
	TasksCreated    int     `json:"tasks_created"`
	TasksDone       int     `json:"tasks_done"`
	TasksFailed     int     `json:"tasks_failed"`
	TasksInProgress int     `json:"tasks_in_progress"`
	TasksPending    int     `json:"tasks_pending"`
	TaskSuccessRate float64 `json:"task_success_rate"`
	PlanningReplies int     `json:"planning_replies"`

	DailyActivity   []DailyActivityItem `json:"daily_activity"`
	CurrentWorkload []WorkloadItem      `json:"current_workload"`
}

type AgentInsights struct {
	Role                string                     `json:"role"`
	TotalItems          int                        `json:"total_items"`
	ActiveItems         int                        `json:"active_items"`
	PendingOver24h      int                        `json:"pending_over_24h"`
	FailuresLast7d      int                        `json:"failures_last_7d"`
	CompletionsLast7d   int                        `json:"completions_last_7d"`
	OldestPendingMs     *float64                   `json:"oldest_pending_ms"`
	LongestInProgressMs *float64                   `json:"longest_in_progress_ms"`
	ResponseP50Ms       *float64                   `json:"response_p50_ms"`
	ResponseP90Ms       *float64                   `json:"response_p90_ms"`
	CompletionP50Ms     *float64                   `json:"completion_p50_ms"`
	CompletionP90Ms     *float64                   `json:"completion_p90_ms"`
	Aging               []AgentAgingBucket         `json:"aging"`
	PriorityBreakdown   []AgentPriorityBreakdown   `json:"priority_breakdown"`
	ProjectContribution []AgentProjectContribution `json:"project_contribution"`
	RiskItems           []AgentRiskItem            `json:"risk_items"`
}

type AgentAgingBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type AgentPriorityBreakdown struct {
	Priority       string  `json:"priority"`
	Label          string  `json:"label"`
	Total          int     `json:"total"`
	Done           int     `json:"done"`
	Failed         int     `json:"failed"`
	Pending        int     `json:"pending"`
	InProgress     int     `json:"in_progress"`
	CompletionRate float64 `json:"completion_rate"`
}

type AgentProjectContribution struct {
	ProjectID      string  `json:"project_id"`
	ProjectName    string  `json:"project_name"`
	Total          int     `json:"total"`
	Done           int     `json:"done"`
	Failed         int     `json:"failed"`
	Pending        int     `json:"pending"`
	InProgress     int     `json:"in_progress"`
	CompletionRate float64 `json:"completion_rate"`
}

type AgentRiskItem struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Title       string  `json:"title"`
	Subtitle    string  `json:"subtitle"`
	ProjectID   string  `json:"project_id"`
	ProjectName string  `json:"project_name"`
	Status      string  `json:"status"`
	AgeMs       float64 `json:"age_ms"`
}

type DailyActivityItem struct {
	Date      string `json:"date"`
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
	Created   int    `json:"created"`
}

type WorkloadItem struct {
	TodoID    string `json:"todo_id"`
	TodoTitle string `json:"todo_title"`
	TaskID    string `json:"task_id"`
	TaskTitle string `json:"task_title"`
	ProjectID string `json:"project_id"`
	StartedAt string `json:"started_at"`
}

// AgentTaskItem represents a task associated with an agent (as PM or executor).
type AgentTaskItem struct {
	ID                 string         `json:"id"`
	ProjectID          string         `json:"project_id"`
	ProjectName        string         `json:"project_name"`
	Title              string         `json:"title"`
	Description        string         `json:"description"`
	Status             string         `json:"status"`
	Priority           string         `json:"priority"`
	PMAgent            PMAgentSummary `json:"pm_agent"`
	Relation           string         `json:"relation"` // "pm" or "executor"
	TodoCount          int            `json:"todo_count"`
	CompletedTodoCount int            `json:"completed_todo_count"`
	FailedTodoCount    int            `json:"failed_todo_count"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}
