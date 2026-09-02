package store

import (
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// taskArtifactsClosed reports whether a task no longer accepts new artifacts.
// Only terminal tasks close the deliverable channel; in_progress and
// awaiting_review stay open so late final deliverables still land.
func taskArtifactsClosed(taskStatus string) bool {
	switch taskStatus {
	case "done", "failed", "canceled":
		return true
	default:
		return false
	}
}

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
	var ownerTodo *model.Todo
	if artifact.TodoID != "" {
		found := false
		for i := range task.Todos {
			if task.Todos[i].ID == artifact.TodoID {
				found = true
				ownerTodo = &task.Todos[i]
			// Reject artifacts delivered to a todo of an already-closed task.
			// An agent that keeps producing after todo.complete (a common LLM
			// behaviour) would otherwise pile up duplicate deliverables.
			// Rework resets the todo to pending, so re-done todos can
			// upload again normally.
			//
			// The channel stays open while the task itself is not terminal:
			// agents routinely report todo.complete first and then attach the
			// finished deliverable (final script, rendered docx, ...). Closing
			// the door right at todo.done silently drops the real deliverable —
			// see 2026-09-02, where the finished screenplay was rejected twice
			// with TODO_ALREADY_DONE and never reached the platform.
			// Duplicates are still collapsed by (todo, file name) below.
			if task.Todos[i].Status == "done" && taskArtifactsClosed(task.Status) {
				return transport.Conflict("TODO_ALREADY_DONE", "todo already done; deliverables are closed")
			}
				break
			}
		}
		if !found {
			return transport.NotFound("todo not found in task")
		}
	}

	// Classify the file nature. An upload carrying an outputName is a declared
	// final deliverable bound to a workflow step output; everything else is a
	// process file (drafts, intermediate notes, ...). Only deliverables
	// participate in downstream step-input resolution — see resolveStepInput.
	// Legacy artifacts persisted before this field default to process.
	if artifact.OutputName != "" {
		artifact.Kind = "deliverable"
	} else {
		artifact.Kind = "process"
	}

	// Deduplicate by transfer ID or by (todo, file name) — an agent re-uploading
	// the same deliverable under a new transfer id (a common LLM behaviour)
	// overwrites the previous artifact instead of piling up duplicates.
	existing := s.taskArtifacts[artifact.TaskID]
	replaced := false
	for i, a := range existing {
		if a.TransferID == artifact.TransferID ||
			(a.TodoID != "" && a.TodoID == artifact.TodoID && a.FileName == artifact.FileName) {
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
				"kind":        artifact.Kind,
				"output_name": artifact.OutputName,
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
	var pf *model.ProjectFile
	if created, err := s.saveProjectFileFromArtifactUnsafe(artifact); err != nil {
		if s.log != nil {
			s.log.Warn("failed to auto-create project file from artifact",
				zap.String("transfer_id", artifact.TransferID),
				zap.Error(err))
		}
	} else {
		pf = created
	}

	// Link the produced project file onto the artifact so the pipeline
	// progress panel can resolve it for preview/download even when the agent
	// did not declare an output name. Done after the ProjectFile is created so
	// the real file id is captured.
	if pf != nil {
		artifact.ProjectFileID = pf.ID
	}

	// Bind the artifact to the owning todo's declared output slot when the
	// upload carried an outputName (matches a workflow StepOutput.Name). This
	// is what lets downstream steps resolve "上一个流程的输出文件". Done after
	// the ProjectFile is created so FileRef can reference its real ID.
	if ownerTodo != nil && artifact.OutputName != "" {
		bound := model.TodoOutput{
			OutputName: artifact.OutputName,
			ArtifactID: artifact.TransferID,
		}
		if pf != nil {
			bound.FileRef = pf.ID
		}
		replacedOut := false
		for i := range ownerTodo.Outputs {
			if ownerTodo.Outputs[i].OutputName == artifact.OutputName {
				ownerTodo.Outputs[i] = bound
				replacedOut = true
				break
			}
		}
		if !replacedOut {
			ownerTodo.Outputs = append(ownerTodo.Outputs, bound)
		}
		if err := s.persistTaskUnsafe(task); err != nil && s.log != nil {
			s.log.Warn("failed to persist todo output binding",
				zap.String("task_id", artifact.TaskID),
				zap.String("output_name", artifact.OutputName),
				zap.Error(err))
		}
	}

	// Persist the artifact-with-project-file link (covers the common case
	// where no output name was declared but the file must still be reachable
	// from the pipeline progress panel).
	if err := s.persistArtifactUnsafe(&artifact); err != nil && s.log != nil {
		s.log.Warn("failed to persist artifact project file link",
			zap.String("transfer_id", artifact.TransferID), zap.Error(err))
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
