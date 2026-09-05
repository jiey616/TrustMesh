package store

import "trustmesh/backend/internal/model"

func (s *Store) GetDashboardStats(sc Scope) model.DashboardStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := model.DashboardStats{}
	for _, a := range s.agents {
		if !visibleToScope(sc, a.OrgID, a.UserID) {
			continue
		}
		stats.AgentsTotal++
		if a.Status != "offline" {
			stats.AgentsOnline++
		}
	}

	for _, t := range s.tasks {
		if !visibleToScope(sc, t.OrgID, t.UserID) {
			continue
		}
		stats.TasksTotal++
		switch t.Status {
		case "in_progress":
			stats.TasksInProgress++
		case "done":
			stats.TasksDoneCount++
		case "failed":
			stats.TasksFailedCount++
		}
		for _, todo := range t.Todos {
			if todo.Status == "pending" {
				stats.TodosPending++
			}
		}
	}

	finished := stats.TasksDoneCount + stats.TasksFailedCount
	if finished > 0 {
		stats.SuccessRate = float64(stats.TasksDoneCount) / float64(finished) * 100
	}
	return stats
}
