package model

import "time"

type ProjectTaskSummary struct {
	TaskTotal       int        `json:"task_total" bson:"task_total"`
	PendingCount    int        `json:"pending_count" bson:"pending_count"`
	InProgressCount int        `json:"in_progress_count" bson:"in_progress_count"`
	DoneCount       int        `json:"done_count" bson:"done_count"`
	FailedCount     int        `json:"failed_count" bson:"failed_count"`
	CanceledCount   int        `json:"canceled_count" bson:"canceled_count"`
	WorkStatus      string     `json:"work_status" bson:"work_status"`
	LatestTaskAt    *time.Time `json:"latest_task_at" bson:"latest_task_at"`
}

type Project struct {
	ID          string             `json:"id" bson:"_id"`
	UserID      string             `json:"-" bson:"user_id"`
	OrgID       string             `json:"org_id,omitempty" bson:"org_id,omitempty"` // 多租户：租户归属（阶段 0 仅加字段）
	Name        string             `json:"name" bson:"name"`
	Description string             `json:"description" bson:"description"`
	Status      string             `json:"status" bson:"status"`
	TaskSummary ProjectTaskSummary `json:"task_summary" bson:"task_summary"`
	PMAgentID   string             `json:"-" bson:"pm_agent_id"`
	PMAgent     PMAgentSummary     `json:"pm_agent" bson:"pm_agent"`
	// Workflows is the project's list of predefined workflows. A task may be
	// created with a workflow snapshot chosen from this list (or none).
	Workflows []Workflow `json:"workflows,omitempty" bson:"workflows,omitempty"`
	// PrimaryWorkflowIndex marks which entry in Workflows is the project's
	// overall pipeline ("项目总流程"). A task created against it must declare
	// the step range it owns (WorkflowRef.StepFrom/StepTo). -1 (default)
	// means no primary workflow is set.
	PrimaryWorkflowIndex int `json:"primary_workflow_index" bson:"primary_workflow_index"`
	// PrimaryWorkflowID is the ID-based reference to the primary workflow
	// (new style, preferred over PrimaryWorkflowIndex).
	PrimaryWorkflowID string `json:"primary_workflow_id,omitempty" bson:"primary_workflow_id,omitempty"`
	// Version is the project's optimistic-lock version (T2.3). It is advanced
	// by the authoritative commit primitive on every successful persist. No
	// omitempty: the field must always be written to Mongo so the versioned
	// filter ({_id, version}) has a stable baseline.
	Version   int       `json:"version" bson:"version"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}
