package store

import (
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// SaveArtifact stores a new artifact and persists it to MongoDB.
func (s *Store) SaveArtifact(artifact model.TaskArtifact) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Resolve agent from node ID.
	agent, agentErr := s.agentByNodeUnsafe(artifact.FromNodeID)
	if agentErr != nil {
		return agentErr
	}
	artifact.FromAgentID = agent.ID
	artifact.FromAgentName = agent.Name

	// Verify the task exists.
	task, ok := s.tasks[artifact.TaskID]
	if !ok || task.UserID != agent.UserID {
		return transport.NotFound("task not found")
	}

	// Verify todo belongs to the task if specified.
	if artifact.TodoID != "" {
		found := false
		for i := range task.Todos {
			if task.Todos[i].ID == artifact.TodoID {
				found = true
				break
			}
		}
		if !found {
			return transport.NotFound("todo not found in task")
		}
	}

	// Deduplicate by transfer ID — overwrite if same transfer already exists.
	existing := s.taskArtifacts[artifact.TaskID]
	replaced := false
	for i, a := range existing {
		if a.TransferID == artifact.TransferID {
			existing[i] = artifact
			replaced = true
			break
		}
	}
	if !replaced {
		s.taskArtifacts[artifact.TaskID] = append(existing, artifact)
	}

	if err := s.persistArtifactUnsafe(&artifact); err != nil {
		if s.log != nil {
			s.log.Warn("failed to persist artifact", zap.String("transfer_id", artifact.TransferID), zap.Error(err))
		}
	}

	// Create a timeline event only for new artifacts (not overwrites).
	if !replaced {
		now := time.Now().UTC()
		content := artifact.FileName
		s.addEventUnsafe(task.UserID, task.ProjectID, artifact.TaskID, artifact.TodoID,
			"agent", agent.ID, agent.Name, "artifact_received", &content,
			map[string]any{
				"transfer_id": artifact.TransferID,
				"file_name":   artifact.FileName,
				"file_size":   artifact.FileSize,
				"mime_type":   artifact.MimeType,
				"task_title":  task.Title,
			}, now)

		if err := s.persistTaskEventsUnsafe(artifact.TaskID); err != nil {
			if s.log != nil {
				s.log.Warn("failed to persist artifact event", zap.String("task_id", artifact.TaskID), zap.Error(err))
			}
		}
	}

	s.publishTaskUnsafe(artifact.TaskID)

	// Auto-create ProjectFile record for the artifact (best-effort).
	// NOTE: Call Unsafe variant because we already hold s.mu.Lock.
	if _, err := s.saveProjectFileFromArtifactUnsafe(artifact); err != nil {
		if s.log != nil {
			s.log.Warn("failed to auto-create project file from artifact",
				zap.String("transfer_id", artifact.TransferID),
				zap.Error(err))
		}
	}

	return nil
}

// GetArtifactsByTaskID returns all artifacts for a given task.
func (s *Store) GetArtifactsByTaskID(taskID string) []model.TaskArtifact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getArtifactsByTaskIDUnsafe(taskID)
}

func (s *Store) getArtifactsByTaskIDUnsafe(taskID string) []model.TaskArtifact {
	artifacts := s.taskArtifacts[taskID]
	if len(artifacts) == 0 {
		return []model.TaskArtifact{}
	}
	out := make([]model.TaskArtifact, len(artifacts))
	copy(out, artifacts)
	return out
}

// GetArtifact returns a single artifact by task ID and transfer ID.
func (s *Store) GetArtifact(taskID, transferID string) (*model.TaskArtifact, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.taskArtifacts[taskID] {
		if a.TransferID == transferID {
			clone := a
			return &clone, nil
		}
	}
	return nil, transport.NotFound("artifact not found")
}

// fillTaskArtifactsUnsafe populates task.Artifacts from the artifact store.
// Caller must hold at least s.mu.RLock.
func (s *Store) fillTaskArtifactsUnsafe(task *model.TaskDetail) {
	if task == nil {
		return
	}
	task.Artifacts = s.getArtifactsByTaskIDUnsafe(task.ID)
}

// copyTaskWithArtifactsUnsafe returns a deep copy of the task with artifacts filled.
// Caller must hold at least s.mu.RLock.
func (s *Store) copyTaskWithArtifactsUnsafe(task *model.TaskDetail) *model.TaskDetail {
	clone := copyTask(task)
	s.fillTaskArtifactsUnsafe(clone)
	return clone
}
