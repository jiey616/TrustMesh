package store

import (
	"testing"
	"time"
)

func TestResetAgentChatClearsActiveSessionWithoutCreatingEmptyChat(t *testing.T) {
	s := New()
	user, appErr := s.CreateUser("user@example.com", "User", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	agent, appErr := s.CreateAgent(Scope{UserID: user.ID}, "node-chat-000", "Chat Agent", "developer", "dev", []string{"conversation"})
	if appErr != nil {
		t.Fatalf("create agent: %v", appErr)
	}
	now := time.Now().UTC()
	s.SyncAgentPresence([]AgentPresence{{NodeID: agent.NodeID, LastSeenAt: now}}, now)

	first, _, appErr := s.AppendAgentChatUserMessage(Scope{UserID: user.ID}, agent.ID, "hello")
	if appErr != nil {
		t.Fatalf("append first message: %v", appErr)
	}
	if len(s.agentChats) != 1 {
		t.Fatalf("expected 1 chat before reset, got %d", len(s.agentChats))
	}

	if appErr := s.ResetAgentChat(Scope{UserID: user.ID}, agent.ID); appErr != nil {
		t.Fatalf("reset chat: %v", appErr)
	}

	if got := s.activeAgentChats[activeAgentChatKey(user.ID, agent.ID)]; got != "" {
		t.Fatalf("expected no active chat after reset, got %s", got)
	}
	if len(s.agentChats) != 1 {
		t.Fatalf("expected reset to avoid creating empty chat, got %d chats", len(s.agentChats))
	}
	stored, ok := s.agentChats[first.ID]
	if !ok {
		t.Fatalf("expected chat %s in store", first.ID)
	}
	if stored.Status != "closed" {
		t.Fatalf("expected chat to be closed, got %s", stored.Status)
	}
	if _, ok := s.agentChatBySession[first.SessionKey]; ok {
		t.Fatal("expected session routing to be removed after reset")
	}
}

func TestResetAgentChatRequiresChatCapableOnlineAgent(t *testing.T) {
	s := New()
	user, appErr := s.CreateUser("user@example.com", "User", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}

	offlineAgent, appErr := s.CreateAgent(Scope{UserID: user.ID}, "node-offline-001", "Offline", "developer", "dev", []string{"conversation"})
	if appErr != nil {
		t.Fatalf("create offline agent: %v", appErr)
	}
	if appErr := s.ResetAgentChat(Scope{UserID: user.ID}, offlineAgent.ID); appErr == nil || appErr.Code != "AGENT_OFFLINE" {
		t.Fatalf("expected AGENT_OFFLINE, got %v", appErr)
	}
}

func TestResetAgentChatRemovesOldSessionRouting(t *testing.T) {
	s := New()
	user, appErr := s.CreateUser("user@example.com", "User", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	agent, appErr := s.CreateAgent(Scope{UserID: user.ID}, "node-chat-001", "Chat Agent", "developer", "dev", []string{"conversation"})
	if appErr != nil {
		t.Fatalf("create agent: %v", appErr)
	}
	now := time.Now().UTC()
	s.SyncAgentPresence([]AgentPresence{{NodeID: agent.NodeID, LastSeenAt: now}}, now)

	first, _, appErr := s.AppendAgentChatUserMessage(Scope{UserID: user.ID}, agent.ID, "first")
	if appErr != nil {
		t.Fatalf("append first message: %v", appErr)
	}
	if appErr := s.ResetAgentChat(Scope{UserID: user.ID}, agent.ID); appErr != nil {
		t.Fatalf("reset chat: %v", appErr)
	}
	second, _, appErr := s.AppendAgentChatUserMessage(Scope{UserID: user.ID}, agent.ID, "second")
	if appErr != nil {
		t.Fatalf("append second message: %v", appErr)
	}
	if first.SessionKey == second.SessionKey {
		t.Fatal("expected a new session key after first reset")
	}
	if _, ok := s.agentChatBySession[first.SessionKey]; ok {
		t.Fatal("expected old session key to be removed from routing table")
	}
	if _, appErr := s.AppendAgentChatMessageByNode(agent.NodeID, first.SessionKey, "stale reply", "remote-1"); appErr == nil || appErr.Code != "NOT_FOUND" {
		t.Fatalf("expected NOT_FOUND for stale session, got %v", appErr)
	}
}

func TestListAgentChatSessionsReturnsNewestFirst(t *testing.T) {
	s := New()
	user, appErr := s.CreateUser("user@example.com", "User", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	agent, appErr := s.CreateAgent(Scope{UserID: user.ID}, "node-chat-002", "Chat Agent", "developer", "dev", []string{"conversation"})
	if appErr != nil {
		t.Fatalf("create agent: %v", appErr)
	}
	now := time.Now().UTC()
	s.SyncAgentPresence([]AgentPresence{{NodeID: agent.NodeID, LastSeenAt: now}}, now)

	first, _, appErr := s.AppendAgentChatUserMessage(Scope{UserID: user.ID}, agent.ID, "older")
	if appErr != nil {
		t.Fatalf("append first message: %v", appErr)
	}

	if appErr := s.ResetAgentChat(Scope{UserID: user.ID}, agent.ID); appErr != nil {
		t.Fatalf("reset second chat: %v", appErr)
	}
	second, _, appErr := s.AppendAgentChatUserMessage(Scope{UserID: user.ID}, agent.ID, "newer")
	if appErr != nil {
		t.Fatalf("append second message: %v", appErr)
	}

	sessions, appErr := s.ListAgentChatSessions(Scope{UserID: user.ID}, agent.ID)
	if appErr != nil {
		t.Fatalf("list sessions: %v", appErr)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].ID != second.ID {
		t.Fatalf("expected newest session first, got %s", sessions[0].ID)
	}
	if sessions[0].LastMessagePreview != "newer" {
		t.Fatalf("unexpected preview: %q", sessions[0].LastMessagePreview)
	}
	if sessions[0].MessageCount != 1 {
		t.Fatalf("expected message_count=1, got %d", sessions[0].MessageCount)
	}
	if sessions[1].ID != first.ID {
		t.Fatalf("expected older session second, got %s", sessions[1].ID)
	}
	if sessions[1].Status != "closed" {
		t.Fatalf("expected first session closed, got %s", sessions[1].Status)
	}
}
