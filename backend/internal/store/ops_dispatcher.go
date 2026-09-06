package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
)

// ─────────────────────────────────────────────────────────────────────────────
// 统一干预编排器（C.2）：对执行体下发的唯一出口。
//
// 收敛三类消息：运维修复指引（本文件 DispatchOpsGuide）、timeout_monitor 催办
// （RecordTimeoutRemind 留痕）、webhook 系统警告（RecordOpsWarning 留痕）。
// 实际推送由注入的 opsPublishHook 完成（app 层用 clawsynapse task.mention 实现）
// —— store 不能 import clawsynapse（反向依赖），与 remindHook 同款注入模式。
//
// 🔴 防失控三闸门：冷却期（OpsGuideCooldown）→ 次数上限（OpsGuideMaxPerTodo，
// 用尽转 escalated）→ 升级抑制窗（escalated 后同主体静默 suppressWindow，
// 防止「新建工单→再指导→再升级」无限循环）。
// 🔴 所有下发（含被节流跳过的）都写 OpsAction 留痕，否则无法回溯
// "到底通知过 agent 没有"。
// ─────────────────────────────────────────────────────────────────────────────

// OpsPublishRequest 一次对执行体的推送请求（由 app 层适配成 task.mention）。
type OpsPublishRequest struct {
	TargetNode string
	TaskID     string
	ProjectID  string
	TodoID     string
	TaskTitle  string
	TaskStatus string
	AuthorName string
	Content    string
	Metadata   map[string]any
}

// SetOpsPublishHook 注入实际推送通道（app 层注册）。nil = 无下发通道，
// 规则引擎与工单仍工作，只是不下发（L0 记录模式）。
func (s *Store) SetOpsPublishHook(hook func(ctx context.Context, req OpsPublishRequest) error) {
	s.opsPublishHook = hook
}

// SetOpsAttributionHook 注入 LLM 归因实现（assistant 包适配 LLMClient）。
// nil 或调用失败时优雅降级：模板指引照常下发，工单标记无归因需人工复核。
func (s *Store) SetOpsAttributionHook(hook func(ctx context.Context, inc *model.OpsIncident, snapshot string) (string, error)) {
	s.opsAttributionHook = hook
}

// attributeAndGuide 工单干预入口：归因（可选）→ 模板指引下发。
// 由扫描器在发现「新工单」后调用（锁外），webhook 信号规则不走这里
// （它们沿用 notifySystemWarningToAgent 既有链路 + RecordOpsWarning 留痕）。
func (s *Store) attributeAndGuide(ctx context.Context, incidentID string) {
	now := time.Now().UTC()

	// Phase 1（持锁）：置 diagnosing、采集快照与模板内容。
	s.mu.Lock()
	inc, ok := s.opsIncidents[incidentID]
	if !ok || !inc.IsActive() {
		s.mu.Unlock()
		return
	}
	snap := s.opsIncidentSnapshotLocked(inc, now)
	gctx := opsGuideContext{
		RuleID:     inc.RuleID,
		TaskID:     inc.TaskID,
		ProjectID:  inc.ProjectID,
		TodoID:     inc.TodoID,
		TaskTitle:  snap.taskTitle,
		TaskStatus: snap.taskStatus,
		TodoTitle:  snap.todoTitle,
		NodeID:     inc.NodeID,
		StalledFor: snap.stalledFor,
		Threshold:  s.opsRuntime.silentThreshold,
	}
	content, templateID := opsGuideContent(gctx)
	inc.Status = model.OpsStatusDiagnosing
	inc.UpdatedAt = now
	s.persistOpsIncidentUnsafe(inc)
	s.mu.Unlock()

	// Phase 2（锁外）：LLM 归因（每工单仅一次——归因后不再进本函数，
	// 因为只有「新建工单」才触发干预）。
	rootCause := ""
	attrSource := model.OpsAttrNone
	attrErr := ""
	if s.opsAttributionHook != nil {
		if rc, err := s.opsAttributionHook(ctx, inc, snap.text); err != nil {
			attrErr = err.Error()
			if s.log != nil {
				s.log.Warn("ops attribution failed, fallback to template",
					zap.String("incident_id", incidentID), zap.Error(err))
			}
		} else {
			rootCause = strings.TrimSpace(rc)
			attrSource = model.OpsAttrLLM
		}
	}

	// Phase 3（持锁）：落归因结果，重生成含根因的指引内容。
	s.mu.Lock()
	inc, ok = s.opsIncidents[incidentID]
	if !ok || !inc.IsActive() {
		s.mu.Unlock()
		return
	}
	inc.RootCause = rootCause
	inc.AttrSource = attrSource
	if attrSource == model.OpsAttrLLM {
		inc.Actions = append(inc.Actions, model.OpsAction{
			ID: newOpsActionID(), At: time.Now().UTC(), Level: model.OpsLevelL0,
			Kind: model.OpsActionDiagnosed, Result: model.OpsResultSent, Detail: rootCause,
		})
	} else {
		detail := "LLM 归因不可用，已降级为模板指引，需人工复核"
		if attrErr != "" {
			detail += "：" + attrErr
		}
		inc.Actions = append(inc.Actions, model.OpsAction{
			ID: newOpsActionID(), At: time.Now().UTC(), Level: model.OpsLevelL0,
			Kind: model.OpsActionDiagnosed, Result: model.OpsResultSkipped, Detail: detail,
		})
	}
	if rootCause != "" {
		content, templateID = opsGuideContent(gctx.withRootCause(rootCause))
	}
	s.mu.Unlock()

	if s.opsPublishHook == nil {
		// 无下发通道：保持 L0 记录模式，工单留在 diagnosing 等人工。
		return
	}
	s.DispatchOpsGuide(ctx, incidentID, content, templateID)
}

// DispatchOpsGuide 下发一条修复指引（统一出口）。节流/上限/留痕全部在此。
func (s *Store) DispatchOpsGuide(ctx context.Context, incidentID, content, templateID string) {
	now := time.Now().UTC()

	// Phase 1（持锁）：闸门校验 + 构造推送请求。
	s.mu.Lock()
	inc, ok := s.opsIncidents[incidentID]
	if !ok || !inc.IsActive() {
		s.mu.Unlock()
		return
	}
	if !inc.CanGuide(s.opsRuntime.guideMax) {
		s.escalateOpsIncidentLocked(inc, now)
		s.mu.Unlock()
		return
	}
	if last := lastGuideSentAtLocked(inc); last != nil && now.Sub(*last) < s.opsRuntime.guideCooldown {
		inc.Actions = append(inc.Actions, model.OpsAction{
			ID: newOpsActionID(), At: now, Level: model.OpsLevelL1,
			Kind: model.OpsActionGuided, Result: model.OpsResultThrottled,
			Detail: "冷却期内，跳过本次下发",
		})
		s.persistOpsIncidentUnsafe(inc)
		s.mu.Unlock()
		return
	}
	req, target := s.buildOpsPublishRequestLocked(inc, content)
	if target == "" {
		inc.Actions = append(inc.Actions, model.OpsAction{
			ID: newOpsActionID(), At: now, Level: model.OpsLevelL1,
			Kind: model.OpsActionGuided, Result: model.OpsResultSkipped,
			Detail: "无法解析目标节点（todo 无 assignee）",
		})
		s.persistOpsIncidentUnsafe(inc)
		s.mu.Unlock()
		return
	}
	inc.Status = model.OpsStatusGuiding
	inc.UpdatedAt = now
	s.mu.Unlock()

	// Phase 2（锁外）：实际推送。
	sendErr := s.opsPublishHook(ctx, req)

	// Phase 3（持锁）：留痕 + 计数。
	s.mu.Lock()
	defer s.mu.Unlock()
	inc, ok = s.opsIncidents[incidentID]
	if !ok {
		return
	}
	result := model.OpsResultSent
	detail := ""
	if sendErr != nil {
		result = model.OpsResultFailed
		detail = sendErr.Error()
		if s.log != nil {
			s.log.Warn("ops guide publish failed",
				zap.String("incident_id", incidentID), zap.String("target", target), zap.Error(sendErr))
		}
	}
	inc.GuideCount++
	inc.Actions = append(inc.Actions, model.OpsAction{
		ID: newOpsActionID(), At: time.Now().UTC(), Level: model.OpsLevelL1,
		Kind: model.OpsActionGuided, TemplateID: templateID, Target: target,
		Content: content, Result: result, Detail: detail,
	})
	inc.UpdatedAt = time.Now().UTC()
	s.persistOpsIncidentUnsafe(inc)
}

// RecordOpsWarning webhook 系统警告留痕：信号点已自行完成 task.mention 下发
// （notifySystemWarningToAgent，实测有效链路，不改动），本方法只把这次干预记
// 进工单并计入指导预算，保证时间线完整。
func (s *Store) RecordOpsWarning(ruleID, taskID, todoID, target, content string) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.opsByDedupeKey[opsDedupeKey(ruleID, taskID, todoID)]
	if !ok {
		return
	}
	inc := s.opsIncidents[id]
	if inc == nil || !inc.IsActive() {
		return
	}
	inc.GuideCount++
	inc.Status = model.OpsStatusGuiding
	inc.Actions = append(inc.Actions, model.OpsAction{
		ID: newOpsActionID(), At: now, Level: model.OpsLevelL1,
		Kind: model.OpsActionGuided, Target: target, Content: content,
		Result: model.OpsResultSent, Detail: "webhook 系统警告（既有链路下发）",
		Metadata: map[string]any{"source": "system_warning"},
	})
	inc.UpdatedAt = now
	s.persistOpsIncidentUnsafe(inc)
}

// RecordTimeoutRemind timeout_monitor 催办留痕：只记时间线，不占指导预算
// （remind 的节奏与判死由 timeout_monitor 负责，编排器不改变其语义）。
func (s *Store) RecordTimeoutRemind(taskID, todoID string) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.opsByDedupeKey[opsDedupeKey(model.RuleTodoStalled, taskID, todoID)]
	if !ok {
		return
	}
	inc := s.opsIncidents[id]
	if inc == nil || !inc.IsActive() {
		return
	}
	inc.Actions = append(inc.Actions, model.OpsAction{
		ID: newOpsActionID(), At: now, Level: model.OpsLevelL0,
		Kind: model.OpsActionReminded, Result: model.OpsResultSent,
		Detail: "timeout_monitor 执行超时催办",
	})
	inc.UpdatedAt = now
	s.persistOpsIncidentUnsafe(inc)
}

// ─────────────────────────────────────────────────────────────────────────────
// 内部工具
// ─────────────────────────────────────────────────────────────────────────────

func newOpsActionID() string { return fmt.Sprintf("act-%d", time.Now().UnixNano()) }

// escalateOpsIncidentLocked 次数用尽转人工，并设置抑制窗（防无限循环）。
// 仅能持锁调用。
func (s *Store) escalateOpsIncidentLocked(inc *model.OpsIncident, now time.Time) {
	inc.Status = model.OpsStatusEscalated
	inc.Active = false
	inc.UpdatedAt = now
	inc.Actions = append(inc.Actions, model.OpsAction{
		ID: newOpsActionID(), At: now, Level: model.OpsLevelL0,
		Kind: model.OpsActionEscalated, Result: model.OpsResultSkipped,
		Detail: fmt.Sprintf("自动干预 %d 次仍无改善，转人工处理；%s 内同主体不再自动开工单",
			inc.GuideCount, humanizeDuration(s.opsRuntime.suppressWindow)),
	})
	delete(s.opsByDedupeKey, inc.DedupeKey)
	s.opsSuppress[inc.DedupeKey] = now.Add(s.opsRuntime.suppressWindow)
	if err := s.persistOpsIncidentUnsafe(inc); err != nil && s.log != nil {
		s.log.Warn("persist ops incident escalate failed", zap.Error(err))
	}
}

// opsSuppressedLocked 主体是否处于升级抑制窗内。仅能持锁调用。
func (s *Store) opsSuppressedLocked(key string, now time.Time) bool {
	until, ok := s.opsSuppress[key]
	if !ok {
		return false
	}
	if now.After(until) {
		delete(s.opsSuppress, key)
		return false
	}
	return true
}

// lastGuideSentAtLocked 最近一次成功下发指引的时间（冷却判定用）。仅能持锁调用。
func lastGuideSentAtLocked(inc *model.OpsIncident) *time.Time {
	for i := len(inc.Actions) - 1; i >= 0; i-- {
		a := inc.Actions[i]
		if a.Kind == model.OpsActionGuided && a.Result == model.OpsResultSent {
			at := a.At
			return &at
		}
	}
	return nil
}

// buildOpsPublishRequestLocked 构造推送请求并解析目标节点：
// 工单 NodeID → todo assignee。仅能持锁调用。
func (s *Store) buildOpsPublishRequestLocked(inc *model.OpsIncident, content string) (OpsPublishRequest, string) {
	target := inc.NodeID
	if target == "" && inc.TodoID != "" && inc.TaskID != "" {
		if t, ok := s.tasks[inc.TaskID]; ok {
			for i := range t.Todos {
				if t.Todos[i].ID == inc.TodoID && t.Todos[i].Assignee.NodeID != "" {
					target = t.Todos[i].Assignee.NodeID
					break
				}
			}
		}
	}
	if target == "" {
		return OpsPublishRequest{}, ""
	}
	return OpsPublishRequest{
		TargetNode: target,
		TaskID:     inc.TaskID,
		ProjectID:  inc.ProjectID,
		TodoID:     inc.TodoID,
		TaskTitle:  s.taskTitleOfLocked(inc.TaskID),
		TaskStatus: s.taskStatusOfLocked(inc.TaskID),
		AuthorName: "运维助手",
		Content:    content,
		Metadata:   map[string]any{"source": "ops_guide", "incident_id": inc.ID, "rule_id": inc.RuleID},
	}, target
}

func (s *Store) taskTitleOfLocked(taskID string) string {
	if t, ok := s.tasks[taskID]; ok {
		return t.Title
	}
	return ""
}

func (s *Store) taskStatusOfLocked(taskID string) string {
	if t, ok := s.tasks[taskID]; ok {
		return t.Status
	}
	return ""
}

// opsGuideSnapshot 工单现场快照（持锁采集）：给 LLM 归因的输入 + 指引素材。
type opsGuideSnapshot struct {
	taskTitle  string
	taskStatus string
	todoTitle  string
	stalledFor time.Duration
	text       string
}

// opsIncidentSnapshotLocked 采集快照。仅能持锁调用。
func (s *Store) opsIncidentSnapshotLocked(inc *model.OpsIncident, now time.Time) opsGuideSnapshot {
	snap := opsGuideSnapshot{}
	var b strings.Builder
	fmt.Fprintf(&b, "规则: %s\n现象: %s\n", inc.RuleID, inc.Summary)

	if t, ok := s.tasks[inc.TaskID]; ok {
		snap.taskTitle = t.Title
		snap.taskStatus = t.Status
		fmt.Fprintf(&b, "任务: %s（状态 %s）\n", t.Title, t.Status)
		if p, ok := s.projects[t.ProjectID]; ok {
			fmt.Fprintf(&b, "项目: %s\n", p.Name)
		}
		if inc.TodoID != "" {
			for i := range t.Todos {
				td := &t.Todos[i]
				if td.ID != inc.TodoID {
					continue
				}
				snap.todoTitle = td.Title
				fmt.Fprintf(&b, "todo: %s（状态 %s，执行节点 %s）\n", td.Title, td.Status, td.Assignee.NodeID)
				last := todoLastActivityAt(td, time.Time{})
				if !last.IsZero() {
					snap.stalledFor = now.Sub(last)
					fmt.Fprintf(&b, "todo 最近活动: %s 前\n", humanizeDuration(snap.stalledFor))
				}
			}
		}
	}
	if snap.stalledFor == 0 {
		if t, ok := s.tasks[inc.TaskID]; ok {
			last := s.taskSilentLastActivity(t)
			snap.stalledFor = now.Sub(last)
			fmt.Fprintf(&b, "任务最近活动: %s 前\n", humanizeDuration(snap.stalledFor))
		}
	}
	// 事件流尾部：给归因一个「最后发生了什么」的线索。
	if evs, ok := s.taskEvents[inc.TaskID]; ok {
		n := len(evs)
		if n > 5 {
			n = 5
		}
		b.WriteString("最近事件:\n")
		for i := len(evs) - n; i < len(evs); i++ {
			e := evs[i]
			fmt.Fprintf(&b, "  - %s (%s)\n", e.EventType, e.CreatedAt.Format("01-02 15:04"))
		}
	}
	snap.text = b.String()
	return snap
}

// withRootCause 归因结果并入指引上下文（重生成内容用）。
func (c opsGuideContext) withRootCause(rc string) opsGuideContext {
	c.RootCause = rc
	return c
}
