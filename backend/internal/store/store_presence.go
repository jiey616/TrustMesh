package store

import (
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

func (s *Store) SyncAgentPresence(items []AgentPresence, now time.Time) int {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	seen := make(map[string]time.Time, len(items))
	for _, item := range items {
		nodeID := strings.TrimSpace(item.NodeID)
		if nodeID == "" {
			continue
		}
		lastSeen := item.LastSeenAt
		if lastSeen.IsZero() {
			lastSeen = now
		}
		seen[nodeID] = lastSeen.UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	updated := 0
	persisted := make(map[string]struct{})
	for _, agent := range s.agents {
		if agent.Archived {
			continue
		}
		lastSeen, online := seen[agent.NodeID]
		nextStatus := "offline"
		if online {
			nextStatus = s.connectedAgentStatusUnsafe(agent.ID)
		}

		changed := false
		if online && (agent.LastSeenAt == nil || !agent.LastSeenAt.Equal(lastSeen)) {
			ts := lastSeen
			agent.LastSeenAt = &ts
			changed = true
		}
		prevStatus := agent.Status
		if agent.Status != nextStatus {
			agent.Status = nextStatus
			s.rebuildProjectPMSummariesUnsafe(agent.ID)
			s.rebuildTaskPMSummariesUnsafe(agent.ID)
			changed = true

			msg := fmt.Sprintf("数字员工 %s: %s -> %s", agent.Name, prevStatus, nextStatus)
			s.addEventUnsafe(agent.UserID, "", "", "", "system", agent.ID, agent.Name, "agent_status_changed", &msg, map[string]any{
				"prev_status": prevStatus,
				"new_status":  nextStatus,
			}, now)
		}
		if !changed {
			continue
		}
		agent.UpdatedAt = now
		if _, ok := persisted[agent.ID]; !ok {
			if err := s.persistAgentGraphUnsafe(agent.ID); err != nil && s.log != nil {
				s.log.Warn("failed to persist synced agent status", zap.String("agent_id", agent.ID), zap.Error(err))
			}
			persisted[agent.ID] = struct{}{}
		}
		updated++
	}
	return updated
}

func (s *Store) connectedAgentStatusUnsafe(agentID string) string {
	for _, task := range s.tasks {
		for _, todo := range task.Todos {
			if todo.Assignee.AgentID == agentID && todo.Status == "in_progress" {
				return "busy"
			}
		}
	}
	return "online"
}

func (s *Store) markAgentSeenUnsafe(agentID string, now time.Time) {
	agent, ok := s.agents[agentID]
	if !ok {
		return
	}
	ts := now.UTC()
	agent.LastSeenAt = &ts
	agent.Status = s.connectedAgentStatusUnsafe(agentID)
	agent.UpdatedAt = now
	s.rebuildProjectPMSummariesUnsafe(agentID)
	s.rebuildTaskPMSummariesUnsafe(agentID)
}

func (s *Store) refreshAgentExecutionStatusUnsafe(agentID string, now time.Time) {
	agent, ok := s.agents[agentID]
	if !ok {
		return
	}
	if agent.Status == "offline" {
		return
	}
	agent.Status = s.connectedAgentStatusUnsafe(agentID)
	agent.UpdatedAt = now
	s.rebuildProjectPMSummariesUnsafe(agentID)
	s.rebuildTaskPMSummariesUnsafe(agentID)
}
