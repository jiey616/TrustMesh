package store

import (
	"strings"

	"go.uber.org/zap"
	"time"

	"github.com/google/uuid"
	"trustmesh/backend/internal/model"
)

// ─────────────────────────────────────────────────────────────────────────────
// 运维工单（ops_incident）核心：发现层产出 → 工单聚合 → 反向验证关闭。
//
// 🔴 归属原则：工单 OrgID 跟随「被扫描的资源」推导（task → project → agent →
// 个人租户兜底），与 resolveEventOrgUnsafe 同思路。后台巡检是 Go 协程，
// 没有 HTTP 上下文，拿不到 X-Org-Id。
// 🔴 去重：同一 主体+规则 在活跃期只允许一个工单（dedupe_key 部分唯一索引兜底）；
// 重复命中只刷新 LastSeen（UpdatedAt），绝不重复记 action 刷屏。
// ─────────────────────────────────────────────────────────────────────────────

// OpsFinding 是发现层的一次异常判定，由扫描器或 webhook 信号点产出。
type OpsFinding struct {
	RuleID   string // model.Rule* 常量
	Severity string // model.OpsSeverity* 常量，空则 warn
	Title    string
	Summary  string
	TaskID   string
	TodoID   string
	NodeID   string // 上报来源节点（执行 agent 的 node id）
	AgentID  string
}

// opsDedupeKey 工单去重键：规则 + 主体（任务/todo）。
func opsDedupeKey(ruleID, taskID, todoID string) string {
	return strings.Join([]string{ruleID, taskID, todoID}, "|")
}

// resolveOpsOwnerUnsafe 推导工单归属（org 跟随资源）。仅能持锁调用。
func (s *Store) resolveOpsOwnerUnsafe(taskID, nodeID string) (orgID, userID string) {
	if taskID != "" {
		if t, ok := s.tasks[taskID]; ok {
			userID = t.UserID
			if t.OrgID != "" {
				return t.OrgID, userID
			}
			if p, ok := s.projects[t.ProjectID]; ok && p.OrgID != "" {
				return p.OrgID, userID
			}
		}
	}
	// 节点级问题（task 兜不住时）：按来源 agent 归属。
	if nodeID != "" {
		if a, ok := s.agents[nodeID]; ok && a.OrgID != "" {
			return a.OrgID, a.UserID
		}
	}
	if userID != "" {
		return s.personalOrgOfUnsafe(userID), userID
	}
	return "", ""
}

// ReportOpsFinding 发现层公共入口：命中即聚合进工单。
// 活跃工单存在 → 只刷新 LastSeen 并复位观察期（防刷屏）；否则新建工单。
// 返回工单 ID（无法归属时返回空串并丢弃——宁漏勿错）。
func (s *Store) ReportOpsFinding(f OpsFinding) string {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reportOpsFindingLocked(f, now)
}

func (s *Store) reportOpsFindingLocked(f OpsFinding, now time.Time) string {
	if f.RuleID == "" {
		return ""
	}
	orgID, userID := s.resolveOpsOwnerUnsafe(f.TaskID, f.NodeID)
	if orgID == "" && userID == "" {
		// 资源已不存在（任务被清等），无法归属——宁漏勿错。
		return ""
	}
	key := opsDedupeKey(f.RuleID, f.TaskID, f.TodoID)

	// 升级抑制窗：该主体刚被转人工（escalated），静默期内不再自动开工单，
	// 否则会立刻新建工单再次指导——「新建→指导→升级」无限循环。
	if s.opsSuppressedLocked(key, now) {
		return ""
	}

	if id, ok := s.opsByDedupeKey[key]; ok {
		inc := s.opsIncidents[id]
		if inc != nil && inc.IsActive() {
			// 重复命中：刷新 LastSeen，复位复发观察期，不重复记 action。
			inc.UpdatedAt = now
			delete(s.opsClearSince, inc.ID)
			s.persistOpsIncidentUnsafe(inc)
			return inc.ID
		}
		// 终态工单不该留在去重索引里（防御）：释放后走新建。
		delete(s.opsByDedupeKey, key)
	}

	severity := f.Severity
	if severity == "" {
		severity = model.OpsSeverityWarn
	}
	projectID := ""
	if f.TaskID != "" {
		if t, ok := s.tasks[f.TaskID]; ok {
			projectID = t.ProjectID
		}
	}
	inc := &model.OpsIncident{
		ID:        uuid.NewString(),
		OrgID:     orgID,
		UserID:    userID,
		DedupeKey: key,
		RuleID:    f.RuleID,
		Status:    model.OpsStatusOpen,
		Severity:  severity,
		Title:     f.Title,
		Summary:   f.Summary,
		ProjectID: projectID,
		TaskID:    f.TaskID,
		TodoID:    f.TodoID,
		AgentID:   f.AgentID,
		NodeID:    f.NodeID,
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	inc.Actions = append(inc.Actions, model.OpsAction{
		ID:      uuid.NewString(),
		At:      now,
		Level:   model.OpsLevelL0,
		Kind:    model.OpsActionCreated,
		Content: f.Summary,
	})

	s.opsIncidents[inc.ID] = inc
	s.opsByDedupeKey[key] = inc.ID
	if f.TaskID != "" {
		s.opsByTask[f.TaskID] = append(s.opsByTask[f.TaskID], inc.ID)
	}
	if err := s.persistOpsIncidentUnsafe(inc); err != nil && s.log != nil {
		s.log.Warn("persist ops incident failed", zap.Error(err))
	}
	return inc.ID
}

// markOpsIncidentClearedUnsafe 事件驱动的「规则不再满足」信号（如交付物绑定
// 成功）。只设观察起点，真正 resolved 由扫描器在观察期满后执行——扛住
// 「动一下又卡」的抖动。仅能持锁调用。
func (s *Store) markOpsIncidentClearedUnsafe(ruleID, taskID, todoID string, now time.Time) {
	key := opsDedupeKey(ruleID, taskID, todoID)
	id, ok := s.opsByDedupeKey[key]
	if !ok {
		return
	}
	if inc := s.opsIncidents[id]; inc == nil || !inc.IsActive() {
		return
	}
	if _, watching := s.opsClearSince[id]; !watching {
		s.opsClearSince[id] = now
	}
}

// resolveOpsIncidentLocked 关闭工单（自动：观察期满；规则不再满足）。
func (s *Store) resolveOpsIncidentLocked(inc *model.OpsIncident, now time.Time) {
	inc.Status = model.OpsStatusResolved
	inc.Active = false
	inc.ResolvedAt = &now
	inc.UpdatedAt = now
	inc.Actions = append(inc.Actions, model.OpsAction{
		ID:     uuid.NewString(),
		At:     now,
		Level:  model.OpsLevelL0,
		Kind:   model.OpsActionResolved,
		Detail: "规则不再满足且已过观察期，自动关闭",
	})
	delete(s.opsByDedupeKey, inc.DedupeKey)
	delete(s.opsClearSince, inc.ID)
	if err := s.persistOpsIncidentUnsafe(inc); err != nil && s.log != nil {
		s.log.Warn("persist ops incident resolve failed", zap.Error(err))
	}
}

// ListOpsIncidents 按租户 Scope 列工单（updated_at 倒序）。
// 🔴 统计口径与列表口径一致：org 上下文走 org 索引，无上下文走 user 维度。
func (s *Store) ListOpsIncidents(sc Scope) []*model.OpsIncident {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]*model.OpsIncident, 0)
	for _, inc := range s.opsIncidents {
		if !opsIncidentVisible(sc, inc) {
			continue
		}
		items = append(items, inc)
	}
	// 倒序：最新的在前。
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].UpdatedAt.After(items[i].UpdatedAt) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	return items
}

// GetOpsIncident 按 ID 取工单（Scope 校验，跨租户 404 语义返回 nil）。
func (s *Store) GetOpsIncident(sc Scope, incidentID string) *model.OpsIncident {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inc, ok := s.opsIncidents[incidentID]
	if !ok || !opsIncidentVisible(sc, inc) {
		return nil
	}
	return inc
}

// opsIncidentVisible 租户裁决。原则与多租户裁决一致：
// 无租户上下文退回 user 维度；有上下文时比 org（未回填者仅作者可见）。
func opsIncidentVisible(sc Scope, inc *model.OpsIncident) bool {
	if inc == nil {
		return false
	}
	if sc.HasOrg() {
		if inc.OrgID != "" {
			return inc.OrgID == sc.OrgID
		}
		// 未回填 org 的工单：宁可漏、不可泄。
		return inc.UserID == sc.UserID
	}
	return inc.UserID == sc.UserID
}
