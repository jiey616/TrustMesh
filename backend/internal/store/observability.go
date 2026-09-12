package store

import (
	"time"

	"trustmesh/backend/internal/metrics"
	"trustmesh/backend/internal/model"
)

// This file holds the Phase 0 observability helpers for the "pipeline stalls
// silently" family (T0.11): step-advance latency, the agent compliance
// sentinel, and their shared state. They live apart from workflow.go and
// timeout_monitor.go so the instrumentation is reviewable in one place.

// agentStallStreakThreshold is how many CONSECUTIVE steps the platform must
// advance on one agent's behalf before the compliance sentinel fires (T0.11③).
//
// Two, not one: a single auto-advance is usually a lost dispatch or a dropped
// report, which the reconciler exists to absorb. Two in a row on the same agent
// means the agent itself stopped advancing the pipeline — its skill context was
// pruned, the run died silently, or it never picked the work up — which is
// exactly the compliance failure that must be surfaced to a human rather than
// silently healed forever.
const agentStallStreakThreshold = 2

// observeStepAdvanceUnsafe records "step N completed -> step N+1 started"
// latency (T0.11②). It is the earliest direct indicator of the "stuck at step
// 1" failure mode: a healthy pipeline advances in seconds to minutes, so a p95
// drifting into the hours means reports are being lost while nothing has failed
// loudly yet.
//
// Only consecutive steps are measured. A task's first todo has no predecessor
// to measure from, and a predecessor that never completed (a reopened or
// reworked todo) yields no meaningful baseline. Callers must hold the write
// lock.
func observeStepAdvanceUnsafe(task *model.TaskDetail, todoIdx int, now time.Time) {
	if task == nil || todoIdx <= 0 || todoIdx >= len(task.Todos) {
		return
	}
	prev := task.Todos[todoIdx-1]
	if prev.CompletedAt == nil {
		return
	}
	d := now.Sub(*prev.CompletedAt)
	if d < 0 {
		// Clock went backwards (NTP step) or CompletedAt came from a snapshot
		// dated in the future. Not a latency sample.
		return
	}
	metrics.Observe(metrics.StepAdvanceDuration, d)
}

// bumpAutoAdvanceStreakUnsafe records that the platform had to advance a step
// on behalf of agentID, and reports whether THIS call tripped the compliance
// sentinel. Exactly-at-threshold, so one long stall emits one event instead of
// one per reconciler tick. Callers must hold the write lock.
func (s *Store) bumpAutoAdvanceStreakUnsafe(taskID, agentID string) bool {
	if taskID == "" || agentID == "" {
		return false
	}
	if s.autoAdvanceStreak == nil {
		s.autoAdvanceStreak = make(map[string]map[string]int)
	}
	byAgent, ok := s.autoAdvanceStreak[taskID]
	if !ok {
		byAgent = make(map[string]int)
		s.autoAdvanceStreak[taskID] = byAgent
	}
	byAgent[agentID]++
	return byAgent[agentID] == agentStallStreakThreshold
}

// autoAdvanceStreakUnsafe returns the current consecutive auto-advance count
// for a task/agent pair (0 when unknown). Callers must hold the lock.
func (s *Store) autoAdvanceStreakUnsafe(taskID, agentID string) int {
	if s.autoAdvanceStreak == nil || taskID == "" || agentID == "" {
		return 0
	}
	return s.autoAdvanceStreak[taskID][agentID]
}

// clearAutoAdvanceStreakUnsafe resets the sentinel for an agent that just
// reported real work on a task. The streak counts CONSECUTIVE failures to
// report, so genuine progress must clear it — otherwise an agent that stalled
// twice early on would stay flagged for the rest of the task. Callers must hold
// the lock.
func (s *Store) clearAutoAdvanceStreakUnsafe(taskID, agentID string) {
	if s.autoAdvanceStreak == nil || taskID == "" || agentID == "" {
		return
	}
	byAgent, ok := s.autoAdvanceStreak[taskID]
	if !ok {
		return
	}
	delete(byAgent, agentID)
	if len(byAgent) == 0 {
		delete(s.autoAdvanceStreak, taskID)
	}
}

// pruneAutoAdvanceStreaksUnsafe drops streaks for tasks that are gone or no
// longer in_progress, mirroring the lazy cleanup used for planningStallCount,
// so the map cannot grow without bound in a long-lived process. Callers must
// hold the write lock.
func (s *Store) pruneAutoAdvanceStreaksUnsafe() {
	if len(s.autoAdvanceStreak) == 0 {
		return
	}
	for taskID := range s.autoAdvanceStreak {
		task, ok := s.tasks[taskID]
		if !ok || task.Status != "in_progress" {
			delete(s.autoAdvanceStreak, taskID)
		}
	}
}
