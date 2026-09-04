package model

import "time"

// ProjectFile represents a file or folder stored within a project's file space.
// Files can be user uploads or agent artifacts auto-indexed from transfer.received.
// When IsFolder is true, the record represents a user-created directory.
type ProjectFile struct {
	ID          string    `json:"id" bson:"_id"`
	ProjectID   string    `json:"project_id" bson:"project_id"`
	OrgID       string    `json:"org_id,omitempty" bson:"org_id,omitempty"` // 多租户：租户归属（阶段 0 仅加字段）
	ParentID    string    `json:"parent_id,omitempty" bson:"parent_id,omitempty"`
	TaskID      string    `json:"task_id,omitempty" bson:"task_id,omitempty"`
	AgentID     string    `json:"agent_id,omitempty" bson:"agent_id,omitempty"`
	AgentName   string    `json:"agent_name,omitempty" bson:"agent_name,omitempty"`
	FileName    string    `json:"file_name" bson:"file_name"`
	FileSize    int64     `json:"file_size" bson:"file_size"`
	MimeType    string    `json:"mime_type" bson:"mime_type"`
	LocalPath   string    `json:"-" bson:"local_path"`
	Source      string    `json:"source" bson:"source"`           // "user_upload" | "agent_artifact" | "meeting_minutes"
	IsFolder    bool      `json:"is_folder" bson:"is_folder"`
	TransferID  string    `json:"transfer_id,omitempty" bson:"transfer_id,omitempty"`
	MeetingID   string    `json:"meeting_id,omitempty" bson:"meeting_id,omitempty"`
	// Kind carries the artifact file-nature for agent_artifact uploads:
	// "deliverable" (declared outputName, bound to a workflow step output)
	// or "process". Empty for user uploads / legacy records.
	Kind       string    `json:"kind,omitempty" bson:"kind,omitempty"`
	OutputName string    `json:"output_name,omitempty" bson:"output_name,omitempty"`
	UploadedBy string    `json:"uploaded_by" bson:"uploaded_by"`
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
}

// ProjectFileTreeNode represents a node in the project file tree.
type ProjectFileTreeNode struct {
	ID       string                `json:"id"`
	Name     string                `json:"name"`
	Type     string                `json:"type"` // "directory" | "file"
	Files    []ProjectFile         `json:"files,omitempty"`
	Children []ProjectFileTreeNode `json:"children,omitempty"`
}

// ProjectFileTree provides a hierarchical view of project files.
type ProjectFileTree struct {
	Uploads []ProjectFileTreeNode `json:"uploads"`
	Tasks   []ProjectFileTreeNode `json:"tasks"`
}

// CreateProjectFolderRequest is the request body for creating a folder.
type CreateProjectFolderRequest struct {
	Name     string `json:"name" binding:"required"`
	ParentID string `json:"parent_id,omitempty"`
}

// RenameProjectFileRequest is the request body for renaming a file or folder.
type RenameProjectFileRequest struct {
	Name string `json:"name" binding:"required"`
}

// MoveProjectFileRequest is the request body for moving a file or folder.
type MoveProjectFileRequest struct {
	ParentID string `json:"parent_id"`
}

// BatchDeleteProjectFilesRequest is the request body for batch-deleting files/folders.
type BatchDeleteProjectFilesRequest struct {
	IDs []string `json:"ids" binding:"required,min=1"`
}

// BatchDeleteResult reports the outcome of a batch-delete operation.
type BatchDeleteResult struct {
	Deleted int      `json:"deleted"`
	Failed  []string `json:"failed"`
}

// ---- Browse (flat folder view) ----

// BreadcrumbNode represents a node in the breadcrumb navigation path.
type BreadcrumbNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// VirtualFolder represents a synthetic folder node that is not persisted as a
// real ProjectFile record. Used for artifact hierarchy (task/agent grouping).
type VirtualFolder struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ItemCount int   `json:"item_count,omitempty"`
}

// BrowseFilesResult contains the contents of a single folder level plus breadcrumbs.
// VirtualFolders are synthetic paths (e.g. __artifacts__ / __task__<id> / __agent__<id><aid>)
// that organise agent artifacts into a browsable hierarchy.
type BrowseFilesResult struct {
	ParentID      string           `json:"parent_id"`
	VirtualFolders []VirtualFolder `json:"virtual_folders,omitempty"`
	Folders       []ProjectFile    `json:"folders"`
	Files         []ProjectFile    `json:"files"`
	Breadcrumbs   []BreadcrumbNode `json:"breadcrumbs"`
}

// ---- Artifact grouping (by Task → Agent) ----

// ArtifactAgentGroup groups artifacts by agent within a task.
type ArtifactAgentGroup struct {
	AgentID   string        `json:"agent_id"`
	AgentName string        `json:"agent_name"`
	Files     []ProjectFile `json:"files"`
}

// ArtifactTaskGroup groups artifacts by task.
type ArtifactTaskGroup struct {
	TaskID   string               `json:"task_id"`
	TaskName string               `json:"task_name"`
	Agents   []ArtifactAgentGroup `json:"agents"`
}
