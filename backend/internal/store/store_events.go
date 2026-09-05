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

// resolveEventOrgUnsafe 推导事件的租户归属：任务 → 项目 → 执行 agent → 个人租户兜底。
// 事件本身没有独立的租户语义，归属应跟随它所在/所描述的资源。
// 仅能在持锁函数内调用。
func (s *Store) resolveEventOrgUnsafe(taskID, projectID, actorType, actorID, userID string) string {
	if taskID != "" {
		if t, ok := s.tasks[taskID]; ok && t.OrgID != "" {
			return t.OrgID
		}
	}
	if projectID != "" {
		if p, ok := s.projects[projectID]; ok && p.OrgID != "" {
			return p.OrgID
		}
	}
	if actorType == "agent" && actorID != "" {
		if a, ok := s.agents[actorID]; ok && a.OrgID != "" {
			return a.OrgID
		}
	}
	return s.personalOrgOfUnsafe(userID)
}

// reindexEventOrgsUnsafe 重建租户活动流索引，并顺带校正存量事件的租户归属。
// 背景：改造前事件恒挂邀请人个人租户，企业空间下无法按租户过滤（泄漏源）。
// userEvents 与 agentEvents 存的是同一批事件指针，用 visited 去重避免重复入索引。
// 幂等，启动时执行一次即可覆盖存量数据。仅能在持锁函数内调用。
func (s *Store) reindexEventOrgsUnsafe() {
	s.orgEvents = make(map[string][]*model.Event)
	visited := make(map[*model.Event]bool)
	reindex := func(event *model.Event) {
		if event == nil || visited[event] {
			return
		}
		visited[event] = true
		event.OrgID = s.resolveEventOrgUnsafe(event.TaskID, event.ProjectID, event.ActorType, event.ActorID, event.UserID)
		if event.OrgID != "" {
			s.orgEvents[event.OrgID] = append(s.orgEvents[event.OrgID], event)
		}
	}
	for _, events := range s.userEvents {
		for _, e := range events {
			reindex(e)
		}
	}
	for _, events := range s.agentEvents {
		for _, e := range events {
			reindex(e)
		}
	}
}

func (s *Store) addEventUnsafe(userID, projectID, taskID, todoID, actorType, actorID, actorName, eventType string, content *string, metadata map[string]any, at time.Time) *model.Event {
	// 多租户：归属跟随资源（任务 → 项目 → 执行 agent），不再恒挂个人租户
	orgID := s.resolveEventOrgUnsafe(taskID, projectID, actorType, actorID, userID)
	event := model.Event{
		ID:        newID(),
		UserID:    userID,
		OrgID:     orgID,
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
	if orgID != "" {
		s.orgEvents[orgID] = append(s.orgEvents[orgID], &event)
		// Upper-bound protection: truncate if too many events for a single org
		if len(s.orgEvents[orgID]) > maxEventsPerOrg {
			s.orgEvents[orgID] = s.orgEvents[orgID][len(s.orgEvents[orgID])-truncatedKeepCount:]
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
