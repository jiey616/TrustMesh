package store

import (
	"sort"
	"strings"
	"time"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

func activeAgentChatKey(userID, agentID string) string {
	return userID + "|" + agentID
}

func copyAgentChatMessages(messages []model.AgentChatMessage) []model.AgentChatMessage {
	if len(messages) == 0 {
		return []model.AgentChatMessage{}
	}
	return append([]model.AgentChatMessage{}, messages...)
}

func (s *Store) GetActiveAgentChat(sc Scope, agentID string) (*model.AgentChatDetail, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, err := s.agentForUserUnsafe(sc, agentID); err != nil {
		return nil, err
	}
	chatID := s.activeAgentChats[activeAgentChatKey(sc.UserID, agentID)]
	if chatID == "" {
		return nil, nil
	}
	chat, ok := s.agentChats[chatID]
	if !ok {
		return nil, nil
	}
	detail := s.toAgentChatDetailUnsafe(chat)
	return &detail, nil
}

func (s *Store) ListAgentChatSessions(sc Scope, agentID string) ([]model.AgentChatSessionSummary, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, err := s.agentForUserUnsafe(sc, agentID); err != nil {
		return nil, err
	}

	sessions := make([]model.AgentChatSessionSummary, 0)
	for _, chat := range s.agentChats {
		if chat.UserID != sc.UserID || chat.AgentID != agentID {
			continue
		}
		sessions = append(sessions, s.toAgentChatSessionSummaryUnsafe(chat))
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
		}
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

func (s *Store) GetAgentChatByID(sc Scope, agentID, chatID string) (*model.AgentChatDetail, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, err := s.agentForUserUnsafe(sc, agentID); err != nil {
		return nil, err
	}

	chat, ok := s.agentChats[chatID]
	if !ok || chat.UserID != sc.UserID || chat.AgentID != agentID {
		return nil, transport.NotFound("agent chat not found")
	}

	detail := s.toAgentChatDetailUnsafe(chat)
	return &detail, nil
}

func (s *Store) ResetAgentChat(sc Scope, agentID string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, err := s.agentForUserUnsafe(sc, agentID)
	if err != nil {
		return err
	}
	if appErr := validateAgentChatAvailability(agent); appErr != nil {
		return appErr
	}

	key := activeAgentChatKey(sc.UserID, agentID)
	if chatID := s.activeAgentChats[key]; chatID != "" {
		if existing, ok := s.agentChats[chatID]; ok {
			existing.Status = "closed"
			existing.UpdatedAt = time.Now().UTC()
			if err := s.persistAgentChatUnsafe(existing); err != nil {
				return mongoWriteError(err)
			}
			delete(s.agentChatBySession, existing.SessionKey)
		}
		delete(s.activeAgentChats, key)
	}

	return nil
}

func (s *Store) AppendAgentChatUserMessage(sc Scope, agentID, content string, attachments []model.ChatAttachment) (*model.AgentChatDetail, *model.AgentChatMessage, *transport.AppError) {
	content = strings.TrimSpace(content)
	if content == "" && len(attachments) == 0 {
		return nil, nil, transport.Validation("invalid content", map[string]any{"content": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	agent, err := s.agentForUserUnsafe(sc, agentID)
	if err != nil {
		return nil, nil, err
	}
	if appErr := validateAgentChatAvailability(agent); appErr != nil {
		return nil, nil, appErr
	}

	chat := s.getOrCreateActiveAgentChatUnsafe(sc.UserID, agent)
	now := time.Now().UTC()
	msg := model.AgentChatMessage{
		ID:          newID(),
		SenderType:  "user",
		Direction:   "outbound",
		Content:     content,
		Status:      "pending",
		Attachments: attachments,
		CreatedAt:   now,
	}
	chat.Messages = append(chat.Messages, msg)
	chat.UpdatedAt = now
	if err := s.persistAgentChatUnsafe(chat); err != nil {
		return nil, nil, mongoWriteError(err)
	}
	s.publishAgentChatUnsafe(chat.ID)
	detail := s.toAgentChatDetailUnsafe(chat)
	return &detail, &msg, nil
}

func (s *Store) UpdateAgentChatMessageStatus(sc Scope, chatID, messageID, status, remoteMessageID string) (*model.AgentChatDetail, *transport.AppError) {
	status = strings.TrimSpace(status)
	if status == "" {
		return nil, transport.Validation("invalid status", map[string]any{"status": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	chat, ok := s.agentChats[chatID]
	if !ok || chat.UserID != sc.UserID {
		return nil, transport.NotFound("agent chat not found")
	}
	for i := range chat.Messages {
		if chat.Messages[i].ID != messageID {
			continue
		}
		chat.Messages[i].Status = status
		if strings.TrimSpace(remoteMessageID) != "" {
			chat.Messages[i].RemoteMessageID = strings.TrimSpace(remoteMessageID)
		}
		chat.UpdatedAt = time.Now().UTC()
		if err := s.persistAgentChatUnsafe(chat); err != nil {
			return nil, mongoWriteError(err)
		}
		s.publishAgentChatUnsafe(chat.ID)
		detail := s.toAgentChatDetailUnsafe(chat)
		return &detail, nil
	}

	return nil, transport.NotFound("agent chat message not found")
}

func (s *Store) AppendAgentChatMessageByNode(nodeID, sessionKey, content, remoteMessageID string) (*model.AgentChatDetail, *transport.AppError) {
	sessionKey = strings.TrimSpace(sessionKey)
	content = strings.TrimSpace(content)
	remoteMessageID = strings.TrimSpace(remoteMessageID)
	if sessionKey == "" || content == "" {
		return nil, transport.Validation("invalid agent chat payload", map[string]any{"session_key": "required", "content": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	agent, err := s.agentByNodeUnsafe(nodeID)
	if err != nil {
		return nil, err
	}
	chatID := s.agentChatBySession[sessionKey]
	if chatID == "" {
		return nil, transport.NotFound("agent chat not found")
	}
	chat, ok := s.agentChats[chatID]
	if !ok {
		return nil, transport.NotFound("agent chat not found")
	}
	if chat.AgentID != agent.ID || chat.AgentNodeID != nodeID {
		return nil, transport.Forbidden("agent is not allowed to reply this chat")
	}
	if chat.Status != "active" {
		delete(s.agentChatBySession, sessionKey)
		return nil, transport.NotFound("agent chat not found")
	}
	for i := range chat.Messages {
		if remoteMessageID != "" && chat.Messages[i].RemoteMessageID == remoteMessageID {
			detail := s.toAgentChatDetailUnsafe(chat)
			return &detail, nil
		}
	}

	now := time.Now().UTC()
	s.markAgentSeenUnsafe(agent.ID, now)
	msg := model.AgentChatMessage{
		ID:              newID(),
		SenderType:      "agent",
		Direction:       "inbound",
		Content:         content,
		Status:          "sent",
		RemoteMessageID: remoteMessageID,
		CreatedAt:       now,
	}
	chat.Messages = append(chat.Messages, msg)
	chat.UpdatedAt = now
	if err := s.persistAgentChatUnsafe(chat); err != nil {
		return nil, mongoWriteError(err)
	}
	if err := s.persistAgentGraphUnsafe(agent.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	if chat.Status == "active" {
		s.publishAgentChatUnsafe(chat.ID)
	}
	detail := s.toAgentChatDetailUnsafe(chat)
	return &detail, nil
}

func (s *Store) agentForUserUnsafe(sc Scope, agentID string) (*model.Agent, *transport.AppError) {
	agent, ok := s.agents[agentID]
	if !ok || !visibleToScope(sc, agent.OrgID, agent.UserID) {
		return nil, transport.NotFound("agent not found")
	}
	return agent, nil
}

func validateAgentChatAvailability(agent *model.Agent) *transport.AppError {
	if agent.Archived {
		return transport.NotFound("agent not found")
	}
	if agent.Status != "online" {
		return transport.Conflict("AGENT_OFFLINE", "agent 当前离线，无法发送消息")
	}
	return nil
}

func (s *Store) getOrCreateActiveAgentChatUnsafe(userID string, agent *model.Agent) *model.AgentChat {
	key := activeAgentChatKey(userID, agent.ID)
	if chatID := s.activeAgentChats[key]; chatID != "" {
		if chat, ok := s.agentChats[chatID]; ok {
			return chat
		}
	}
	chat := s.newAgentChatUnsafe(userID, agent)
	return chat
}

func (s *Store) newAgentChatUnsafe(userID string, agent *model.Agent) *model.AgentChat {
	now := time.Now().UTC()
	chat := &model.AgentChat{
		ID:          newID(),
		UserID:      userID,
		AgentID:     agent.ID,
		AgentNodeID: agent.NodeID,
		Status:      "active",
		Messages:    []model.AgentChatMessage{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	chat.SessionKey = chat.ID
	s.agentChats[chat.ID] = chat
	s.activeAgentChats[activeAgentChatKey(userID, agent.ID)] = chat.ID
	s.agentChatBySession[chat.SessionKey] = chat.ID
	return chat
}

func (s *Store) toAgentChatDetailUnsafe(chat *model.AgentChat) model.AgentChatDetail {
	messages := copyAgentChatMessages(chat.Messages)
	return model.AgentChatDetail{
		ID:          chat.ID,
		AgentID:     chat.AgentID,
		AgentNodeID: chat.AgentNodeID,
		SessionKey:  chat.SessionKey,
		Status:      chat.Status,
		Messages:    messages,
		CreatedAt:   chat.CreatedAt,
		UpdatedAt:   chat.UpdatedAt,
	}
}

func (s *Store) toAgentChatSessionSummaryUnsafe(chat *model.AgentChat) model.AgentChatSessionSummary {
	summary := model.AgentChatSessionSummary{
		ID:           chat.ID,
		AgentID:      chat.AgentID,
		SessionKey:   chat.SessionKey,
		Status:       chat.Status,
		MessageCount: len(chat.Messages),
		CreatedAt:    chat.CreatedAt,
		UpdatedAt:    chat.UpdatedAt,
	}

	if n := len(chat.Messages); n > 0 {
		last := chat.Messages[n-1]
		summary.LastMessagePreview = last.Content
		lastAt := last.CreatedAt
		summary.LastMessageAt = &lastAt
	}

	return summary
}
