package store

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"trustmesh/backend/internal/model"
)

// 第 3 批验收：统一干预编排器的三闸门（冷却→上限→抑制窗）+ 留痕 + 模板内容。

type pubRecorder struct {
	mu   sync.Mutex
	reqs []OpsPublishRequest
}

func (p *pubRecorder) hook(_ context.Context, req OpsPublishRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reqs = append(p.reqs, req)
	return nil
}

func (p *pubRecorder) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.reqs)
}

// newDispatchFixture 造 store + 停滞任务 + 工单，返回 (store, incidentID, recorder, ownerScope)。
func newDispatchFixture(t *testing.T, guideMax int, cooldown time.Duration) (*Store, string, *pubRecorder, Scope) {
	t.Helper()
	s, orgA, _, ua := newOpsFixture(t)
	taskID, todoID := seedStalledTask(s, orgA, ua)
	scope := Scope{UserID: ua}

	rt := opsRuntime{
		scanInterval:    time.Minute,
		silentThreshold: time.Minute,
		resolveObserve:  time.Minute,
		guideMax:        guideMax,
		guideCooldown:   cooldown,
		suppressWindow:  time.Hour,
	}
	s.opsRuntime = rt

	rec := &pubRecorder{}
	s.SetOpsPublishHook(rec.hook)

	id := s.ReportOpsFinding(OpsFinding{
		RuleID:  model.RuleTodoStalled,
		Title:   "todo 长时间无进展",
		Summary: "卡住的编剧任务 / 写第一场",
		TaskID:  taskID,
		TodoID:  todoID,
		NodeID:  "node-stalled",
	})
	if id == "" {
		t.Fatal("expected incident id")
	}
	// 归属校验：org 跟随资源（任务）而非作者。
	inc := s.GetOpsIncident(Scope{UserID: ua}, id)
	if inc == nil {
		t.Fatal("incident invisible to owner scope")
	}
	if inc.OrgID != orgA {
		t.Fatalf("incident org = %q, want %q (must follow resource)", inc.OrgID, orgA)
	}
	return s, id, rec, scope
}

func TestOpsGuideDispatchAndThrottle(t *testing.T) {
	s, id, rec, scope := newDispatchFixture(t, 3, time.Hour)
	s.attributeAndGuide(context.Background(), id)

	if got := rec.count(); got != 1 {
		t.Fatalf("publish count = %d, want 1", got)
	}
	inc := s.GetOpsIncident(scope, id)
	if inc.Status != model.OpsStatusGuiding {
		t.Fatalf("status = %s, want guiding", inc.Status)
	}
	if inc.GuideCount != 1 {
		t.Fatalf("guide count = %d, want 1", inc.GuideCount)
	}
	if inc.AttrSource != model.OpsAttrNone {
		t.Fatalf("attr source = %s, want none (no LLM hook)", inc.AttrSource)
	}
	// 无 LLM 归因 → 降级记录 + 模板内容非空。
	var guide *model.OpsAction
	for i := range inc.Actions {
		if inc.Actions[i].Kind == model.OpsActionGuided && inc.Actions[i].Result == model.OpsResultSent {
			guide = &inc.Actions[i]
		}
	}
	if guide == nil || guide.Content == "" {
		t.Fatal("sent guide action with content missing")
	}
	if !strings.Contains(guide.Content, "todo.complete") {
		t.Fatal("guide content must contain concrete todo.complete instruction")
	}
	if rec.reqs[0].TargetNode != "node-stalled" {
		t.Fatalf("target = %s, want node-stalled", rec.reqs[0].TargetNode)
	}

	// 冷却期内再次下发 → throttled 留痕，不推送。
	s.DispatchOpsGuide(context.Background(), id, "again", "tpl")
	if got := rec.count(); got != 1 {
		t.Fatalf("publish count after throttle = %d, want 1", got)
	}
	inc = s.GetOpsIncident(scope, id)
	if inc.GuideCount != 1 {
		t.Fatalf("guide count after throttle = %d, want 1", inc.GuideCount)
	}
	found := false
	for i := range inc.Actions {
		if inc.Actions[i].Result == model.OpsResultThrottled {
			found = true
		}
	}
	if !found {
		t.Fatal("throttled action must be recorded")
	}
}

func TestOpsGuideEscalatesAndSuppresses(t *testing.T) {
	s, id, rec, scope := newDispatchFixture(t, 2, time.Nanosecond)

	// 无 LLM hook 时 attributeAndGuide 也会下发一次。
	s.attributeAndGuide(context.Background(), id)
	s.DispatchOpsGuide(context.Background(), id, "nudge 2", "tpl")
	if got := rec.count(); got != 2 {
		t.Fatalf("publish count = %d, want 2", got)
	}

	// 第 3 次：预算用尽 → escalated，不再推送。
	s.DispatchOpsGuide(context.Background(), id, "nudge 3", "tpl")
	if got := rec.count(); got != 2 {
		t.Fatalf("publish count after escalate = %d, want 2", got)
	}
	inc := s.GetOpsIncident(scope, id)
	if inc.Status != model.OpsStatusEscalated {
		t.Fatalf("status = %s, want escalated", inc.Status)
	}
	if inc.IsActive() {
		t.Fatal("escalated incident must be terminal")
	}

	// 抑制窗内同主体再命中 → 不新建工单（防「新建→指导→升级」循环）。
	if got := s.ReportOpsFinding(OpsFinding{
		RuleID: model.RuleTodoStalled, TaskID: inc.TaskID, TodoID: inc.TodoID, NodeID: "node-stalled",
	}); got != "" {
		t.Fatalf("suppressed finding must return empty, got %q", got)
	}
}

func TestOpsGuideTemplatesCoverRules(t *testing.T) {
	rules := []string{
		model.RuleTodoStalled, model.RuleTaskSilent,
		model.RuleDeliverableUnbound, model.RuleDeliverableReject,
	}
	for _, rule := range rules {
		content, tpl := opsGuideContent(opsGuideContext{
			RuleID: rule, TaskTitle: "任务X", TodoTitle: "步骤Y",
			StalledFor: 45 * time.Minute, Threshold: 30 * time.Minute,
		})
		if content == "" || tpl == "" {
			t.Fatalf("rule %s: empty content/template", rule)
		}
		if !strings.Contains(content, "任务X") {
			t.Fatalf("rule %s: content missing task title", rule)
		}
		// 实证要求：指引必须带具体命令，不能是"请检查配置"式废话。
		if !strings.Contains(content, "todo.") && !strings.Contains(content, "clawsynapse transfer send") {
			t.Fatalf("rule %s: content lacks concrete command", rule)
		}
	}
}

func TestOpsRecordWarningAndRemind(t *testing.T) {
	s, id, _, scope := newDispatchFixture(t, 3, time.Hour)

	// webhook 警告留痕：GuideCount 增加 + action 记录（下发由既有链路完成）。
	s.RecordOpsWarning(model.RuleTodoStalled, "", "", "", "") // 不存在的工单：无副作用
	inc := s.GetOpsIncident(scope, id)
	before := inc.GuideCount

	taskID, todoID := inc.TaskID, inc.TodoID
	s.RecordOpsWarning(model.RuleTodoStalled, taskID, todoID, "node-stalled", "警告内容")
	inc = s.GetOpsIncident(scope, id)
	if inc.GuideCount != before+1 {
		t.Fatalf("guide count = %d, want %d", inc.GuideCount, before+1)
	}
	if inc.Status != model.OpsStatusGuiding {
		t.Fatalf("status = %s, want guiding", inc.Status)
	}

	// timeout remind 留痕：不占指导预算。
	s.RecordTimeoutRemind(taskID, todoID)
	inc = s.GetOpsIncident(scope, id)
	if inc.GuideCount != before+1 {
		t.Fatalf("remind must not consume guide budget, count = %d", inc.GuideCount)
	}
	found := false
	for i := range inc.Actions {
		if inc.Actions[i].Kind == model.OpsActionReminded {
			found = true
		}
	}
	if !found {
		t.Fatal("reminded action missing")
	}
}
