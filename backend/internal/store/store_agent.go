package store

import (
	"sort"
	"strings"
	"time"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

type UpdateAgentInput struct {
	Name         *string
	Role         *string
	Description  *string
	Capabilities *[]string
}

type agentInsightItem struct {
	ID          string
	Kind        string
	Title       string
	Subtitle    string
	ProjectID   string
	ProjectName string
	Priority    string
	Status      string
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	FailedAt    *time.Time
}

var agentInsightPriorityOrder = []string{"urgent", "high", "medium", "low"}

var agentInsightPriorityLabels = map[string]string{
	"urgent": "紧急",
	"high":   "高",
	"medium": "中",
	"low":    "低",
}

var agentInsightAgingBuckets = []struct {
	Label string
	MinMs float64
	MaxMs float64
}{
	{Label: "1 小时内", MinMs: 0, MaxMs: 60 * 60 * 1000},
	{Label: "1-24 小时", MinMs: 60 * 60 * 1000, MaxMs: 24 * 60 * 60 * 1000},
	{Label: "1-3 天", MinMs: 24 * 60 * 60 * 1000, MaxMs: 3 * 24 * 60 * 60 * 1000},
	{Label: "3 天以上", MinMs: 3 * 24 * 60 * 60 * 1000, MaxMs: 1<<63 - 1},
}

func (s *Store) CreateAgent(userID, nodeID, name, role, description string, capabilities []string) (*model.Agent, *transport.AppError) {
	nodeID = strings.TrimSpace(nodeID)
	name = strings.TrimSpace(name)
	role = strings.TrimSpace(role)
	description = strings.TrimSpace(description)
	if nodeID == "" || name == "" || role == "" || description == "" {
		return nil, transport.Validation("invalid agent payload", map[string]any{
			"node_id":     "required",
			"name":        "required",
			"role":        "required",
			"description": "required",
		})
	}
	if !isValidRole(role) {
		return nil, transport.Validation("invalid role", map[string]any{"role": "must be one of pm/developer/reviewer/custom"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.agentByNode[nodeID]; exists {
		return nil, transport.Conflict("AGENT_NODE_ID_EXISTS", "node_id already exists")
	}

	now := time.Now().UTC()
	agent := &model.Agent{
		ID:           newID(),
		UserID:       userID,
		OrgID:        s.personalOrgOfUnsafe(userID),
		Name:         name,
		Description:  description,
		Role:         role,
		Capabilities: normalizeCapabilities(capabilities),
		NodeID:       nodeID,
		Product:      defaultProduct(""), // 手动创建默认平台原生 "trustmesh"；hermes/opc 走 JoinRequest 审批同步
		Status:       "offline",
		LastSeenAt:   nil,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.agents[agent.ID] = agent
	s.agentByNode[nodeID] = agent.ID
	if err := s.persistAgentUnsafe(agent); err != nil {
		return nil, mongoWriteError(err)
	}

	clone := copyAgent(agent)
	clone.Usage = s.agentUsageUnsafe(agent.ID)
	return clone, nil
}

func (s *Store) ListAgents(userID string) []model.Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]model.Agent, 0)
	for _, a := range s.agents {
		if a.UserID == userID && !a.Archived {
			clone := copyAgent(a)
			clone.Usage = s.agentUsageUnsafe(a.ID)
			items = append(items, *clone)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}

func (s *Store) GetAgent(userID, agentID string) (*model.Agent, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[agentID]
	if !ok || a.UserID != userID {
		return nil, transport.NotFound("agent not found")
	}
	clone := copyAgent(a)
	clone.Usage = s.agentUsageUnsafe(a.ID)
	return clone, nil
}

func (s *Store) UpdateAgent(userID, agentID string, in UpdateAgentInput) (*model.Agent, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.agents[agentID]
	if !ok || a.UserID != userID || a.Archived {
		return nil, transport.NotFound("agent not found")
	}

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, transport.Validation("invalid name", map[string]any{"name": "cannot be empty"})
		}
		a.Name = name
	}
	if in.Role != nil {
		role := strings.TrimSpace(*in.Role)
		if !isValidRole(role) {
			return nil, transport.Validation("invalid role", map[string]any{"role": "must be one of pm/developer/reviewer/custom"})
		}
		a.Role = role
	}
	if in.Description != nil {
		desc := strings.TrimSpace(*in.Description)
		if desc == "" {
			return nil, transport.Validation("invalid description", map[string]any{"description": "cannot be empty"})
		}
		a.Description = desc
	}
	if in.Capabilities != nil {
		a.Capabilities = normalizeCapabilities(*in.Capabilities)
	}
	a.UpdatedAt = time.Now().UTC()

	s.rebuildProjectPMSummariesUnsafe(a.ID)
	s.rebuildTaskPMSummariesUnsafe(a.ID)
	s.rebuildTodoAssigneeUnsafe(a.ID)
	if err := s.persistAgentGraphUnsafe(a.ID); err != nil {
		return nil, mongoWriteError(err)
	}

	clone := copyAgent(a)
	clone.Usage = s.agentUsageUnsafe(a.ID)
	return clone, nil
}

func (s *Store) DeleteAgent(userID, agentID string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.agents[agentID]
	if !ok || a.UserID != userID {
		return transport.NotFound("agent not found")
	}
	if a.Archived {
		return transport.NotFound("agent not found")
	}

	usage := s.agentUsageUnsafe(agentID)
	if usage.InUse {
		// 软删除：标记为归档，释放 node_id 以便复用
		a.Archived = true
		a.Status = "offline"
		a.UpdatedAt = time.Now().UTC()
		delete(s.agentByNode, a.NodeID)
		if err := s.persistAgentUnsafe(a); err != nil {
			return mongoWriteError(err)
		}
		return nil
	}
	// 无引用：硬删除
	delete(s.agentByNode, a.NodeID)
	delete(s.agents, agentID)
	if err := s.deleteAgentUnsafe(agentID); err != nil {
		return mongoWriteError(err)
	}
	return nil
}

func (s *Store) GetAgentByNodeID(nodeID string) (*model.Agent, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agentByNodeUnsafe(nodeID)
}

func (s *Store) agentByNodeUnsafe(nodeID string) (*model.Agent, *transport.AppError) {
	agentID, ok := s.agentByNode[nodeID]
	if !ok {
		return nil, transport.NotFound("agent not found by node_id")
	}
	agent, ok := s.agents[agentID]
	if !ok {
		return nil, transport.NotFound("agent not found")
	}
	return agent, nil
}

// GetAgentByIDUnsafe returns any non-archived agent by ID without userID filtering.
func (s *Store) GetAgentByIDUnsafe(agentID string) (*model.Agent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[agentID]
	if !ok || a.Archived {
		return nil, false
	}
	return a, true
}

func (s *Store) agentUsageUnsafe(agentID string) model.AgentUsage {
	usage := model.AgentUsage{}
	for _, p := range s.projects {
		if p.PMAgentID == agentID {
			usage.ProjectCount++
		}
	}
	for _, t := range s.tasks {
		if t.PMAgentID == agentID {
			usage.TaskCount++
		}
		for _, todo := range t.Todos {
			if todo.Assignee.AgentID == agentID {
				usage.TodoCount++
			}
		}
	}
	usage.TotalCount = usage.ProjectCount + usage.TaskCount + usage.TodoCount
	usage.InUse = usage.TotalCount > 0
	return usage
}

func (s *Store) rebuildProjectPMSummariesUnsafe(agentID string) {
	a, ok := s.agents[agentID]
	if !ok {
		return
	}
	for _, p := range s.projects {
		if p.PMAgentID == agentID {
			p.PMAgent = toPMSummary(a)
		}
	}
}

func (s *Store) rebuildTaskPMSummariesUnsafe(agentID string) {
	a, ok := s.agents[agentID]
	if !ok {
		return
	}
	for _, t := range s.tasks {
		if t.PMAgentID == agentID {
			t.PMAgent = toPMSummary(a)
		}
	}
}

func (s *Store) rebuildTodoAssigneeUnsafe(agentID string) {
	a, ok := s.agents[agentID]
	if !ok {
		return
	}
	for _, t := range s.tasks {
		for i := range t.Todos {
			if t.Todos[i].Assignee.AgentID == agentID {
				t.Todos[i].Assignee.Name = a.Name
				t.Todos[i].Assignee.NodeID = a.NodeID
			}
		}
	}
}

func (s *Store) GetAgentStats(userID, agentID string) (*model.AgentStats, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a, ok := s.agents[agentID]
	if !ok || a.UserID != userID {
		return nil, transport.NotFound("agent not found")
	}

	if a.Role == "pm" {
		return s.pmStatsUnsafe(agentID), nil
	}
	return s.executorStatsUnsafe(agentID), nil
}

func (s *Store) GetAgentInsights(userID, agentID string) (*model.AgentInsights, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a, ok := s.agents[agentID]
	if !ok || a.UserID != userID {
		return nil, transport.NotFound("agent not found")
	}

	if a.Role == "pm" {
		return s.pmInsightsUnsafe(agentID), nil
	}
	return s.executorInsightsUnsafe(agentID), nil
}

func (s *Store) pmInsightsUnsafe(agentID string) *model.AgentInsights {
	items := make([]agentInsightItem, 0)
	for _, task := range s.tasks {
		if task.PMAgentID != agentID {
			continue
		}
		var startedAt *time.Time
		if task.Status == "in_progress" {
			startedAt = &task.CreatedAt
		}
		var completedAt *time.Time
		if task.Status == "done" {
			completedAt = &task.UpdatedAt
		}
		var failedAt *time.Time
		if task.Status == "failed" {
			failedAt = &task.UpdatedAt
		}

		items = append(items, agentInsightItem{
			ID:          task.ID,
			Kind:        "task",
			Title:       task.Title,
			Subtitle:    "PM 任务",
			ProjectID:   task.ProjectID,
			ProjectName: s.projectNameUnsafe(task.ProjectID),
			Priority:    task.Priority,
			Status:      task.Status,
			CreatedAt:   task.CreatedAt,
			StartedAt:   startedAt,
			CompletedAt: completedAt,
			FailedAt:    failedAt,
		})
	}

	return s.buildAgentInsightsUnsafe("pm", items, nil, nil)
}

func (s *Store) executorInsightsUnsafe(agentID string) *model.AgentInsights {
	items := make([]agentInsightItem, 0)
	responseTimes := make([]float64, 0)
	completionTimes := make([]float64, 0)

	for _, task := range s.tasks {
		projectName := s.projectNameUnsafe(task.ProjectID)
		for i := range task.Todos {
			todo := task.Todos[i]
			if todo.Assignee.AgentID != agentID {
				continue
			}

			items = append(items, agentInsightItem{
				ID:          todo.ID,
				Kind:        "todo",
				Title:       todo.Title,
				Subtitle:    task.Title,
				ProjectID:   task.ProjectID,
				ProjectName: projectName,
				Priority:    task.Priority,
				Status:      todo.Status,
				CreatedAt:   todo.CreatedAt,
				StartedAt:   todo.StartedAt,
				CompletedAt: todo.CompletedAt,
				FailedAt:    todo.FailedAt,
			})

			if todo.StartedAt != nil {
				if d := todo.StartedAt.Sub(todo.CreatedAt).Seconds() * 1000; d >= 0 {
					responseTimes = append(responseTimes, d)
				}
			}
			if todo.StartedAt != nil && todo.CompletedAt != nil {
				if d := todo.CompletedAt.Sub(*todo.StartedAt).Seconds() * 1000; d >= 0 {
					completionTimes = append(completionTimes, d)
				}
			}
		}
	}

	return s.buildAgentInsightsUnsafe(s.agents[agentID].Role, items, responseTimes, completionTimes)
}

func (s *Store) buildAgentInsightsUnsafe(role string, items []agentInsightItem, responseTimes, completionTimes []float64) *model.AgentInsights {
	now := time.Now().UTC()
	recentCutoff := now.AddDate(0, 0, -7)

	insights := &model.AgentInsights{
		Role:      role,
		Aging:     make([]model.AgentAgingBucket, 0, len(agentInsightAgingBuckets)),
		RiskItems: []model.AgentRiskItem{},
	}

	projectRows := make(map[string]*model.AgentProjectContribution)
	priorityRows := make(map[string]*model.AgentPriorityBreakdown)

	for _, bucket := range agentInsightAgingBuckets {
		insights.Aging = append(insights.Aging, model.AgentAgingBucket{Label: bucket.Label})
	}
	for _, priority := range agentInsightPriorityOrder {
		priorityRows[priority] = &model.AgentPriorityBreakdown{
			Priority: priority,
			Label:    agentInsightPriorityLabels[priority],
		}
	}

	riskItems := make([]model.AgentRiskItem, 0)

	for _, item := range items {
		insights.TotalItems++

		projectRow, ok := projectRows[item.ProjectID]
		if !ok {
			projectRow = &model.AgentProjectContribution{
				ProjectID:   item.ProjectID,
				ProjectName: item.ProjectName,
			}
			projectRows[item.ProjectID] = projectRow
		}

		priorityRow, ok := priorityRows[item.Priority]
		if !ok {
			priorityRow = &model.AgentPriorityBreakdown{
				Priority: item.Priority,
				Label:    item.Priority,
			}
			priorityRows[item.Priority] = priorityRow
		}

		projectRow.Total++
		priorityRow.Total++

		switch item.Status {
		case "done":
			projectRow.Done++
			priorityRow.Done++
			if item.CompletedAt != nil && !item.CompletedAt.Before(recentCutoff) {
				insights.CompletionsLast7d++
			}
			continue
		case "failed":
			projectRow.Failed++
			priorityRow.Failed++
			if item.FailedAt != nil && !item.FailedAt.Before(recentCutoff) {
				insights.FailuresLast7d++
			}
			continue
		case "pending":
			projectRow.Pending++
			priorityRow.Pending++
		case "in_progress":
			projectRow.InProgress++
			priorityRow.InProgress++
			insights.ActiveItems++
		}

		ageMs := agentInsightAgeMs(item, now)
		if ageMs >= 24*60*60*1000 {
			insights.PendingOver24h++
		}
		if item.Status == "pending" {
			insights.OldestPendingMs = maxFloat64Ptr(insights.OldestPendingMs, ageMs)
		}
		if item.Status == "in_progress" {
			insights.LongestInProgressMs = maxFloat64Ptr(insights.LongestInProgressMs, ageMs)
		}

		for i, bucket := range agentInsightAgingBuckets {
			if ageMs >= bucket.MinMs && ageMs < bucket.MaxMs {
				insights.Aging[i].Count++
				break
			}
		}

		riskItems = append(riskItems, model.AgentRiskItem{
			ID:          item.ID,
			Kind:        item.Kind,
			Title:       item.Title,
			Subtitle:    item.Subtitle,
			ProjectID:   item.ProjectID,
			ProjectName: item.ProjectName,
			Status:      item.Status,
			AgeMs:       ageMs,
		})
	}

	for _, row := range projectRows {
		row.CompletionRate = agentInsightCompletionRate(row.Done, row.Failed)
		insights.ProjectContribution = append(insights.ProjectContribution, *row)
	}
	sort.Slice(insights.ProjectContribution, func(i, j int) bool {
		if insights.ProjectContribution[i].Total != insights.ProjectContribution[j].Total {
			return insights.ProjectContribution[i].Total > insights.ProjectContribution[j].Total
		}
		return insights.ProjectContribution[i].Done > insights.ProjectContribution[j].Done
	})
	if len(insights.ProjectContribution) > 5 {
		insights.ProjectContribution = insights.ProjectContribution[:5]
	}

	for _, priority := range agentInsightPriorityOrder {
		row := priorityRows[priority]
		if row == nil || row.Total == 0 {
			continue
		}
		row.CompletionRate = agentInsightCompletionRate(row.Done, row.Failed)
		insights.PriorityBreakdown = append(insights.PriorityBreakdown, *row)
	}

	sort.Slice(riskItems, func(i, j int) bool { return riskItems[i].AgeMs > riskItems[j].AgeMs })
	if len(riskItems) > 4 {
		riskItems = riskItems[:4]
	}
	insights.RiskItems = riskItems

	insights.ResponseP50Ms = agentInsightPercentile(responseTimes, 50)
	insights.ResponseP90Ms = agentInsightPercentile(responseTimes, 90)
	insights.CompletionP50Ms = agentInsightPercentile(completionTimes, 50)
	insights.CompletionP90Ms = agentInsightPercentile(completionTimes, 90)

	// 保证数组字段始终为 []，避免前端对 null 数组字段直接访问崩溃
	if insights.PriorityBreakdown == nil {
		insights.PriorityBreakdown = []model.AgentPriorityBreakdown{}
	}
	if insights.ProjectContribution == nil {
		insights.ProjectContribution = []model.AgentProjectContribution{}
	}
	if insights.Aging == nil {
		insights.Aging = []model.AgentAgingBucket{}
	}
	if insights.RiskItems == nil {
		insights.RiskItems = []model.AgentRiskItem{}
	}

	return insights
}

func (s *Store) projectNameUnsafe(projectID string) string {
	if project, ok := s.projects[projectID]; ok {
		return project.Name
	}
	return "未命名项目"
}

func agentInsightAgeMs(item agentInsightItem, now time.Time) float64 {
	baseAt := item.CreatedAt
	if item.Status == "in_progress" && item.StartedAt != nil {
		baseAt = *item.StartedAt
	}
	d := now.Sub(baseAt).Seconds() * 1000
	if d < 0 {
		return 0
	}
	return d
}

func agentInsightCompletionRate(done, failed int) float64 {
	finished := done + failed
	if finished == 0 {
		return 0
	}
	return float64(done) / float64(finished) * 100
}

func agentInsightPercentile(values []float64, percentile int) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := (percentile*len(sorted) + 99) / 100
	if index < 1 {
		index = 1
	}
	if index > len(sorted) {
		index = len(sorted)
	}
	value := sorted[index-1]
	return &value
}

func maxFloat64Ptr(current *float64, value float64) *float64 {
	if current == nil || value > *current {
		v := value
		return &v
	}
	return current
}

func (s *Store) initDailyBuckets() ([]model.DailyActivityItem, map[string]int, time.Time) {
	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -30)
	dailyMap := make(map[string]int, 30)
	dailyItems := make([]model.DailyActivityItem, 30)
	for i := 0; i < 30; i++ {
		d := now.AddDate(0, 0, -29+i)
		key := d.Format("2006-01-02")
		dailyItems[i] = model.DailyActivityItem{Date: key}
		dailyMap[key] = i
	}
	return dailyItems, dailyMap, cutoff
}

func (s *Store) pmStatsUnsafe(agentID string) *model.AgentStats {
	agent := s.agents[agentID]
	dailyItems, dailyMap, cutoff := s.initDailyBuckets()

	stats := &model.AgentStats{
		Role:            "pm",
		DailyActivity:   dailyItems,
		CurrentWorkload: []model.WorkloadItem{},
	}

	// projects managed
	for _, p := range s.projects {
		if p.PMAgentID == agentID {
			stats.ProjectsManaged++
		}
	}

	// tasks
	for _, t := range s.tasks {
		if t.PMAgentID != agentID {
			continue
		}
		stats.TasksCreated++

		switch t.Status {
		case "done":
			stats.TasksDone++
		case "failed":
			stats.TasksFailed++
		case "in_progress":
			stats.TasksInProgress++
			stats.CurrentWorkload = append(stats.CurrentWorkload, model.WorkloadItem{
				TaskID:    t.ID,
				TaskTitle: t.Title,
				ProjectID: t.ProjectID,
				StartedAt: t.CreatedAt.Format(time.RFC3339),
			})
		case "pending":
			stats.TasksPending++
		}

		// daily: task created
		if t.CreatedAt.After(cutoff) {
			if idx, ok := dailyMap[t.CreatedAt.Format("2006-01-02")]; ok {
				dailyItems[idx].Created++
			}
		}
		// daily: task completed / failed (use UpdatedAt as proxy)
		if t.Status == "done" && t.UpdatedAt.After(cutoff) {
			if idx, ok := dailyMap[t.UpdatedAt.Format("2006-01-02")]; ok {
				dailyItems[idx].Completed++
			}
		}
		if t.Status == "failed" && t.UpdatedAt.After(cutoff) {
			if idx, ok := dailyMap[t.UpdatedAt.Format("2006-01-02")]; ok {
				dailyItems[idx].Failed++
			}
		}
	}

	finished := stats.TasksDone + stats.TasksFailed
	if finished > 0 {
		stats.TaskSuccessRate = float64(stats.TasksDone) / float64(finished) * 100
	}

	// planning replies count from agent events
	for _, ev := range s.agentEvents[agentID] {
		if ev.EventType == "planning_reply" && ev.UserID == agent.UserID {
			stats.PlanningReplies++
		}
	}

	return stats
}

func (s *Store) executorStatsUnsafe(agentID string) *model.AgentStats {
	agent := s.agents[agentID]
	dailyItems, dailyMap, cutoff := s.initDailyBuckets()

	stats := &model.AgentStats{
		Role:            agent.Role,
		DailyActivity:   dailyItems,
		CurrentWorkload: []model.WorkloadItem{},
	}

	var totalResponseMs, totalCompletionMs float64
	var responseCount, completionCount int

	for _, t := range s.tasks {
		if t.UserID != agent.UserID {
			continue
		}
		for _, todo := range t.Todos {
			if todo.Assignee.AgentID != agentID {
				continue
			}
			stats.TodosTotal++

			switch todo.Status {
			case "done":
				stats.TodosDone++
			case "failed":
				stats.TodosFailed++
			case "in_progress":
				stats.TodosInProgress++
				if todo.StartedAt != nil {
					stats.CurrentWorkload = append(stats.CurrentWorkload, model.WorkloadItem{
						TodoID:    todo.ID,
						TodoTitle: todo.Title,
						TaskID:    t.ID,
						TaskTitle: t.Title,
						ProjectID: t.ProjectID,
						StartedAt: todo.StartedAt.Format(time.RFC3339),
					})
				}
			case "pending":
				stats.TodosPending++
			}

			if todo.StartedAt != nil {
				d := todo.StartedAt.Sub(todo.CreatedAt).Seconds() * 1000
				if d >= 0 {
					totalResponseMs += d
					responseCount++
				}
			}
			if todo.CompletedAt != nil && todo.StartedAt != nil {
				d := todo.CompletedAt.Sub(*todo.StartedAt).Seconds() * 1000
				if d >= 0 {
					totalCompletionMs += d
					completionCount++
				}
			}

			if todo.CompletedAt != nil && todo.CompletedAt.After(cutoff) {
				if idx, ok := dailyMap[todo.CompletedAt.Format("2006-01-02")]; ok {
					dailyItems[idx].Completed++
				}
			}
			if todo.FailedAt != nil && todo.FailedAt.After(cutoff) {
				if idx, ok := dailyMap[todo.FailedAt.Format("2006-01-02")]; ok {
					dailyItems[idx].Failed++
				}
			}
		}
	}

	finished := stats.TodosDone + stats.TodosFailed
	if finished > 0 {
		stats.SuccessRate = float64(stats.TodosDone) / float64(finished) * 100
	}
	if responseCount > 0 {
		avg := totalResponseMs / float64(responseCount)
		stats.AvgResponseTimeMs = &avg
	}
	if completionCount > 0 {
		avg := totalCompletionMs / float64(completionCount)
		stats.AvgCompletionTimeMs = &avg
	}

	return stats
}

func (s *Store) ListAgentTasks(userID, agentID, status string) ([]model.AgentTaskItem, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a, ok := s.agents[agentID]
	if !ok || a.UserID != userID {
		return nil, transport.NotFound("agent not found")
	}
	if status != "" && !isValidTaskStatus(status) {
		return nil, transport.Validation("invalid status", map[string]any{"status": "must be pending/in_progress/done/failed/canceled"})
	}

	seen := make(map[string]struct{})
	items := make([]model.AgentTaskItem, 0)

	for _, task := range s.tasks {
		if task.UserID != userID {
			continue
		}

		var relation string
		if a.Role == "pm" && task.PMAgentID == agentID {
			relation = "pm"
		} else {
			for _, todo := range task.Todos {
				if todo.Assignee.AgentID == agentID {
					relation = "executor"
					break
				}
			}
		}
		if relation == "" {
			continue
		}
		if _, dup := seen[task.ID]; dup {
			continue
		}
		seen[task.ID] = struct{}{}

		if status != "" && task.Status != status {
			continue
		}

		completed, failed := 0, 0
		for _, td := range task.Todos {
			if td.Status == "done" {
				completed++
			}
			if td.Status == "failed" {
				failed++
			}
		}

		items = append(items, model.AgentTaskItem{
			ID:                 task.ID,
			ProjectID:          task.ProjectID,
			ProjectName:        s.projectNameUnsafe(task.ProjectID),
			Title:              task.Title,
			Description:        task.Description,
			Status:             task.Status,
			Priority:           task.Priority,
			PMAgent:            task.PMAgent,
			Relation:           relation,
			TodoCount:          len(task.Todos),
			CompletedTodoCount: completed,
			FailedTodoCount:    failed,
			CreatedAt:          task.CreatedAt,
			UpdatedAt:          task.UpdatedAt,
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, nil
}

func normalizeCapabilities(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, item := range in {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func isValidRole(role string) bool {
	switch role {
	case "pm", "developer", "reviewer", "custom":
		return true
	default:
		return false
	}
}
