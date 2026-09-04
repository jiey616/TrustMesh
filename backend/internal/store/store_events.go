package store

import (
	"strings"
	"time"

	"trustmesh/backend/internal/model"
)

func processedMessageKey(action, nodeID, messageID string) string {
	if strings.TrimSpace(messageID) == "" {
		return ""
	}
	return action + "|" + nodeID + "|" + messageID
}

func (s *Store) findProcessedTaskUnsafe(key string) (*model.TaskDetail, bool) {
	if key == "" {
		return nil, false
	}
	record, ok := s.processedMessages[key]
	if !ok {
		return nil, false
	}
	task, ok := s.tasks[record.ResourceID]
	if !ok {
		return nil, false
	}
	return copyTask(task), true
}

func (s *Store) rememberProcessedMessageUnsafe(key, action, resourceID string) {
	if key == "" {
		return
	}
	s.processedMessages[key] = processedMessage{
		Action:     action,
		ResourceID: resourceID,
	}
}

func (s *Store) addEventUnsafe(userID, projectID, taskID, todoID, actorType, actorID, actorName, eventType string, content *string, metadata map[string]any, at time.Time) *model.Event {
	event := model.Event{
		ID:        newID(),
		UserID:    userID,
		OrgID:     s.personalOrgOfUnsafe(userID),
		ProjectID: projectID,
		TaskID:    taskID,
		TodoID:    todoID,
		ActorType: actorType,
		ActorID:   actorID,
		ActorName: actorName,
		EventType: eventType,
		Content:   content,
		Metadata:  copyMap(metadata),
		CreatedAt: at,
	}
	if taskID != "" {
		s.taskEvents[taskID] = append(s.taskEvents[taskID], event)
		// Upper-bound protection: truncate if too many events for a single task
		if len(s.taskEvents[taskID]) > maxEventsPerTask {
			s.taskEvents[taskID] = s.taskEvents[taskID][len(s.taskEvents[taskID])-truncatedKeepCount:]
		}
	}
	if userID != "" {
		s.userEvents[userID] = append(s.userEvents[userID], &event)
		// Upper-bound protection: truncate if too many events for a single user
		if len(s.userEvents[userID]) > maxEventsPerUser {
			s.userEvents[userID] = s.userEvents[userID][len(s.userEvents[userID])-truncatedKeepCount:]
		}
	}
	if actorType == "agent" && actorID != "" {
		s.agentEvents[actorID] = append(s.agentEvents[actorID], &event)
		// Upper-bound protection: truncate if too many events for a single agent
		if len(s.agentEvents[actorID]) > maxEventsPerAgent {
			s.agentEvents[actorID] = s.agentEvents[actorID][len(s.agentEvents[actorID])-truncatedKeepCount:]
		}
	}
	s.maybeCreateNotificationUnsafe(&event)
	if taskID != "" {
		s.publishUserEventUnsafe(userID, "task.event.created", map[string]any{
			"task_id":    taskID,
			"project_id": projectID,
			"event":      event,
		}, at)
	}
	if eventType == "agent_status_changed" {
		payload := map[string]any{
			"event": event,
		}
		if agent, ok := s.agents[actorID]; ok {
			payload["agent"] = *copyAgent(agent)
		}
		s.publishUserEventUnsafe(userID, "agent.status.changed", payload, at)
	}
	return &event
}
