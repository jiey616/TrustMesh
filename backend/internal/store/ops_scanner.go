package store

import (
	"context"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/model"
)

// ─────────────────────────────────────────────────────────────────────────────
// 运维发现层：定时扫描 + 规则引擎。
//
// 架构裁决（docs/ops-agent-phase1-plan.md）：规则管「是否异常」（可审计、
// 可复现），LLM 管「为什么」（归因，二期接入）。沉默型故障（任务卡住不进
// 下一步、不发任何事件）只有定时全量扫描能发现，任何事件驱动方案都会漏。
//
// 🔴 聚合不得跨租户：本扫描逐任务评估、org 逐任务跟随推导，评估过程只触碰
// 单个任务的数据，天然无跨租户混合。禁止引入"全表失败率"这类跨租户聚合。
// ─────────────────────────────────────────────────────────────────────────────

// opsRuntime 运行参数，由 bootstrap 从 config 注入（Store 不持有整个 config）。
type opsRuntime struct {
	scanInterval    time.Duration
	silentThreshold time.Duration
	resolveObserve  time.Duration
}

func opsRuntimeFromConfig(cfg config.Config) opsRuntime {
	rt := opsRuntime{
		scanInterval:    cfg.OpsScanInterval,
		silentThreshold: cfg.OpsSilentThreshold,
		resolveObserve:  cfg.OpsResolveObserve,
	}
	if rt.scanInterval <= 0 {
		rt.scanInterval = 5 * time.Minute
	}
	if rt.silentThreshold <= 0 {
		rt.silentThreshold = 30 * time.Minute
	}
	if rt.resolveObserve <= 0 {
		rt.resolveObserve = 10 * time.Minute
	}
	return rt
}

// StartOpsScanner 启动运维扫描循环。仅当 OPS_ENABLED=1 时由 app 层启动。
// 仿照 StartTimeoutMonitor 的结构：ticker + select，ctx 取消即退出。
func (s *Store) StartOpsScanner(ctx context.Context) {
	rt := s.opsRuntime
	if s.log != nil {
		s.log.Info("ops scanner started",
			zap.Duration("scan_interval", rt.scanInterval),
			zap.Duration("silent_threshold", rt.silentThreshold),
			zap.Duration("resolve_observe", rt.resolveObserve))
	}
	ticker := time.NewTicker(rt.scanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOpsScanOnce(rt)
		}
	}
}

// runOpsScanOnce 单轮扫描：评估扫描型规则 → 反向验证 → 观察期关闭。
// 全程持锁一次完成（与 timeout monitor 的 checkTodoTimeouts 同款纪律）。
func (s *Store) runOpsScanOnce(rt opsRuntime) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	hits := make(map[string]bool) // dedupeKey → 本轮命中

	for _, task := range s.tasks {
		// 与 timeout monitor 同款口径：只盯执行态任务。
		// planning 有自己的监控；waiting_user / awaiting_review 是「预期等待」，
		// 沉默是正常的，开单只会制造噪音。
		if task.Status != "in_progress" && task.Status != "pending" {
			continue
		}

		// 规则 1：todo_stalled —— todo 处于 in_progress 但长时间无任何上报。
		for i := range task.Todos {
			todo := &task.Todos[i]
			if todo.Status != "in_progress" {
				continue
			}
			last := todoLastActivityAt(todo, task.CreatedAt)
			if now.Sub(last) < rt.silentThreshold {
				continue
			}
			key := opsDedupeKey(model.RuleTodoStalled, task.ID, todo.ID)
			hits[key] = true
			s.reportOpsFindingLocked(OpsFinding{
				RuleID:  model.RuleTodoStalled,
				Title:   "todo 长时间无进展",
				Summary: task.Title + " / " + todo.Title,
				TaskID:  task.ID,
				TodoID:  todo.ID,
				NodeID:  todo.Assignee.NodeID,
			}, now)
		}

		// 规则 2：task_silent —— 沉默型卡死：任务在执行态但全渠道无任何活动。
		// 活动口径 = max(任务事件流末条, 各 todo 上报/完结时间, task.UpdatedAt)。
		// task.UpdatedAt 只在 planning 流程 bump（勘察结论），单用它必误报，
		// 因此必须与事件流、todo 活动取 max。
		last := s.taskSilentLastActivity(task)
		if now.Sub(last) >= rt.silentThreshold {
			key := opsDedupeKey(model.RuleTaskSilent, task.ID, "")
			hits[key] = true
			s.reportOpsFindingLocked(OpsFinding{
				RuleID:  model.RuleTaskSilent,
				Title:   "任务长时间无任何活动",
				Summary: task.Title + "（超过 " + rt.silentThreshold.String() + " 无事件、无 todo 上报）",
				TaskID:  task.ID,
			}, now)
		}
	}

	// 规则 3+4（deliverable_unbound / deliverable_rejected）由 webhook 信号点
	// 事件驱动上报（warnUnboundDeliverable / warnTransferRejected），不在此扫描。
	// 其「规则不再满足」信号由绑定成功点 markOpsIncidentClearedUnsafe 设置。

	// 反向验证 + 观察期关闭。
	for _, inc := range s.opsIncidents {
		if !inc.IsActive() {
			continue
		}
		var cleared bool
		switch inc.RuleID {
		case model.RuleTodoStalled, model.RuleTaskSilent:
			cleared = !hits[inc.DedupeKey]
		case model.RuleDeliverableUnbound, model.RuleDeliverableReject:
			_, cleared = s.opsClearSince[inc.ID]
		default:
			continue
		}
		if !cleared {
			delete(s.opsClearSince, inc.ID)
			continue
		}
		if since, ok := s.opsClearSince[inc.ID]; !ok {
			s.opsClearSince[inc.ID] = now
		} else if now.Sub(since) >= rt.resolveObserve {
			if s.log != nil {
				s.log.Info("ops incident auto-resolved",
					zap.String("incident_id", inc.ID),
					zap.String("rule_id", inc.RuleID))
			}
			s.resolveOpsIncidentLocked(inc, now)
		}
	}
}

// todoLastActivityAt todo 最近活动时间：上报 > 完结/失败 > 指派 > 开始 > 兜底。
func todoLastActivityAt(todo *model.Todo, fallback time.Time) time.Time {
	last := fallback
	for _, t := range []*time.Time{todo.LastActivityAt, todo.CompletedAt, todo.FailedAt, todo.AssignedAt, todo.StartedAt} {
		if t != nil && t.After(last) {
			last = *t
		}
	}
	return last
}

// taskSilentLastActivity 任务最近活动时间（全渠道取 max）。仅能持锁调用。
func (s *Store) taskSilentLastActivity(task *model.TaskDetail) time.Time {
	last := task.CreatedAt
	if task.UpdatedAt.After(last) {
		last = task.UpdatedAt
	}
	if evs, ok := s.taskEvents[task.ID]; ok && len(evs) > 0 {
		if e := evs[len(evs)-1]; e.CreatedAt.After(last) {
			last = e.CreatedAt
		}
	}
	for i := range task.Todos {
		if t := todoLastActivityAt(&task.Todos[i], time.Time{}); t.After(last) {
			last = t
		}
	}
	return last
}
