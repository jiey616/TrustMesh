package store

import (
	"testing"

	"trustmesh/backend/internal/model"
)

// 仪表盘数字员工统计必须与 Agent 列表口径一致：已归档（离职）的不计入。
// 回归背景：GetDashboardStats 曾漏掉 Archived 过滤，导致统计比列表多出归档数。
func TestGetDashboardStatsExcludesArchivedAgents(t *testing.T) {
	s := New()
	u, err := s.CreateUser("u1@example.com", "u1", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	personal, err := s.EnsurePersonalOrg(u.ID, "u1")
	if err != nil {
		t.Fatalf("personal org: %v", err)
	}

	s.mu.Lock()
	s.agents["a1"] = &model.Agent{ID: "a1", UserID: u.ID, OrgID: personal.ID, Name: "在职-在线", Status: "online"}
	s.agents["a2"] = &model.Agent{ID: "a2", UserID: u.ID, OrgID: personal.ID, Name: "在职-离线", Status: "offline"}
	s.agents["a3"] = &model.Agent{ID: "a3", UserID: u.ID, OrgID: personal.ID, Name: "已归档", Status: "offline", Archived: true}
	s.mu.Unlock()

	sc := Scope{UserID: u.ID}
	stats := s.GetDashboardStats(sc)
	if stats.AgentsTotal != 2 {
		t.Fatalf("AgentsTotal = %d, want 2 (archived agent must be excluded)", stats.AgentsTotal)
	}
	if stats.AgentsOnline != 1 {
		t.Fatalf("AgentsOnline = %d, want 1", stats.AgentsOnline)
	}
	if got := len(s.ListAgents(sc)); got != stats.AgentsTotal {
		t.Fatalf("dashboard total (%d) must match agent list length (%d)", stats.AgentsTotal, got)
	}
}
