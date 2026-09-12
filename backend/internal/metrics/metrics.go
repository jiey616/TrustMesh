// Package metrics is a dependency-free, in-process registry of counters and
// coarse histograms.
//
// Why it exists: Phase 0 needs cheap, always-on observability for the
// "pipeline stalls silently" failure mode (T0.3 dispatch tenant mismatch,
// T0.11 observability trio) and the backend has no metrics dependency. go.mod
// is deliberately tight and CI builds run with GOPROXY=off, so nothing may be
// added. Everything here is stdlib.
//
// Design notes:
//   - Counters are monotonic and cheap (sync/atomic); hot paths may call Inc
//     without allocating.
//   - Histograms use fixed upper bounds and report quantiles by bucket
//     boundary. That is an approximation: Histogram returns the upper bound of
//     the bucket that first crosses the requested cumulative share, so the true
//     value lies in (previous bound, returned bound]. Good enough to answer
//     "is step-advance p95 seconds or hours?" without pulling in a dependency.
//   - Everything is process-local and resets on restart. These metrics drive
//     trend alerts, not billing.
package metrics

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Counter and histogram names emitted by the platform. Declared as constants so
// AlertRules and tests reference them without string drift.
const (
	// DispatchOrgMismatchTotal counts dispatches whose assignee agent does not
	// belong to the task's tenant. Observe-only in Phase 0; T1.1 enforces.
	DispatchOrgMismatchTotal = "dispatch_org_mismatch_total"

	// TodoAutoAdvancedTotal counts stalled pending todos the dispatch
	// reconciler promoted into in_progress (T0.7c).
	TodoAutoAdvancedTotal = "todo_auto_advanced_total"

	// AgentStepStalledTotal counts agent compliance sentinel trips: an assignee
	// that had to be auto-advanced on consecutive steps (T0.11③).
	AgentStepStalledTotal = "agent_step_stalled_total"

	// TodoTimeoutFailedTotal counts todos the timeout monitor failed.
	TodoTimeoutFailedTotal = "todo_timeout_failed_total"

	// StepAdvanceDuration histograms "step N completed -> step N+1 started"
	// latency. This is the earliest direct indicator of the "stuck at step 1"
	// failure mode (T0.11②).
	StepAdvanceDuration = "step_advance_seconds"

	// TodoStalledDuration histograms how long an in_progress todo had been
	// silent when the timeout monitor first flagged it as stalled (T0.11①).
	TodoStalledDuration = "todo_stalled_seconds"
)

// histogramBounds are the upper bounds of the histogram buckets, ascending.
// The final implicit bucket is +Inf. Ranges are tuned for pipeline latency:
// seconds matter for fast steps, hours for video generation.
var histogramBounds = []time.Duration{
	1 * time.Second,
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	1 * time.Minute,
	2 * time.Minute,
	5 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
	1 * time.Hour,
	2 * time.Hour,
	4 * time.Hour,
	8 * time.Hour,
}

// ---------------------------------------------------------------------------
// Counters
// ---------------------------------------------------------------------------

// counters maps name -> *int64. A sync.Map keeps the read path lock-free.
var counters sync.Map

func counter(name string) *int64 {
	if v, ok := counters.Load(name); ok {
		return v.(*int64)
	}
	v, _ := counters.LoadOrStore(name, new(int64))
	return v.(*int64)
}

// Inc adds one to the named counter, creating it on first use.
func Inc(name string) { Add(name, 1) }

// Add adds delta to the named counter, creating it on first use.
func Add(name string, delta int64) { atomic.AddInt64(counter(name), delta) }

// Count returns the current value of the named counter (0 if unknown).
func Count(name string) int64 { return atomic.LoadInt64(counter(name)) }

// Snapshot returns a copy of all counters. Intended for the ops endpoint.
func Snapshot() map[string]int64 {
	out := make(map[string]int64)
	counters.Range(func(k, v any) bool {
		out[k.(string)] = atomic.LoadInt64(v.(*int64))
		return true
	})
	return out
}

// ---------------------------------------------------------------------------
// Histograms
// ---------------------------------------------------------------------------

type histogram struct {
	mu      sync.Mutex
	count   int64
	sumNs   int64
	buckets []int64 // len(histogramBounds)+1; the last is the +Inf bucket
}

func newHistogram() *histogram {
	return &histogram{buckets: make([]int64, len(histogramBounds)+1)}
}

var histograms sync.Map

func histogramFor(name string) *histogram {
	if v, ok := histograms.Load(name); ok {
		return v.(*histogram)
	}
	v, _ := histograms.LoadOrStore(name, newHistogram())
	return v.(*histogram)
}

// Observe records one duration sample in the named histogram.
func Observe(name string, d time.Duration) {
	if d < 0 {
		d = 0
	}
	h := histogramFor(name)
	idx := len(histogramBounds) // +Inf bucket by default
	for i, bound := range histogramBounds {
		if d <= bound {
			idx = i
			break
		}
	}
	h.mu.Lock()
	h.count++
	h.sumNs += d.Nanoseconds()
	h.buckets[idx]++
	h.mu.Unlock()
}

// BucketReport is one histogram bucket. UpperBoundMs == 0 marks the +Inf
// bucket. CumulativeCount is the running total of samples in this bucket and
// all faster ones, so the +Inf bucket's cumulative count equals the histogram
// total.
type BucketReport struct {
	UpperBoundMs    int64 `json:"upper_bound_ms"`
	CumulativeCount int64 `json:"cumulative_count"`
}

// HistogramReport is a point-in-time view of one histogram.
type HistogramReport struct {
	Name    string         `json:"name"`
	Count   int64          `json:"count"`
	SumMs   int64          `json:"sum_ms"`
	AvgMs   int64          `json:"avg_ms"`
	P50Ms   int64          `json:"p50_ms"`
	P95Ms   int64          `json:"p95_ms"`
	Buckets []BucketReport `json:"buckets"`
}

// Histogram returns a snapshot with quantiles approximated by bucket upper
// bounds (see the package doc). Returns a zero-count report for unknown names.
func Histogram(name string) HistogramReport {
	h := histogramFor(name)
	return h.report(name)
}

func (h *histogram) report(name string) HistogramReport {
	h.mu.Lock()
	count := h.count
	sumNs := h.sumNs
	buckets := make([]int64, len(h.buckets))
	copy(buckets, h.buckets)
	h.mu.Unlock()

	rep := HistogramReport{Name: name, Count: count, SumMs: sumNs / int64(time.Millisecond)}
	if count > 0 {
		rep.AvgMs = rep.SumMs / count
	}

	cumulative := int64(0)
	for i, c := range buckets {
		cumulative += c
		upper := int64(0) // +Inf
		if i < len(histogramBounds) {
			upper = histogramBounds[i].Milliseconds()
		}
		rep.Buckets = append(rep.Buckets, BucketReport{UpperBoundMs: upper, CumulativeCount: cumulative})
		// Guard on count > 0: with no samples every cumulative share is
		// trivially satisfied, which would report the fastest bucket bound as
		// a quantile for an empty histogram.
		if count == 0 {
			continue
		}
		if rep.P50Ms == 0 && cumulative*2 >= count {
			rep.P50Ms = upper
		}
		if rep.P95Ms == 0 && cumulative*20 >= count*19 {
			rep.P95Ms = upper
		}
	}
	return rep
}

// ---------------------------------------------------------------------------
// Alert rules
// ---------------------------------------------------------------------------

// AlertRule documents a condition that should page or open a ticket. Phase 0
// ships the definitions plus a unit test that pins them (T0.11 acceptance:
// "alert rule existence"), so "the alert exists" is checkable in CI instead of
// relying on tribal knowledge. Wiring the expr to a real alerting backend is an
// operator concern.
type AlertRule struct {
	Name     string `json:"name"`
	Metric   string `json:"metric"`
	Expr     string `json:"expr"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}

// AlertRules is the Phase 0 rule set for the stall family. Reference time
// window is 1h unless the expression says otherwise.
var AlertRules = []AlertRule{
	{
		Name:     "DispatchOrgMismatch",
		Metric:   DispatchOrgMismatchTotal,
		Expr:     "increase(dispatch_org_mismatch_total[1h]) > 0",
		Severity: "warning",
		Summary:  "任务被派发给不属于该租户的 agent：租户信任假设被打破，检查 agent.org_id 回填与项目归属",
	},
	{
		Name:     "TodoAutoAdvanced",
		Metric:   TodoAutoAdvancedTotal,
		Expr:     "increase(todo_auto_advanced_total[1h]) > 0",
		Severity: "warning",
		Summary:  "有 todo 因执行方无回报被平台自愈推进：确认是偶发丢包还是 agent 合规问题",
	},
	{
		Name:     "AgentStepStalled",
		Metric:   AgentStepStalledTotal,
		Expr:     "increase(agent_step_stalled_total[1h]) > 0",
		Severity: "critical",
		Summary:  "同一 agent 在连续多步上都未回报、需平台推进：skill 被裁剪或上下文丢失，人工介入该 agent",
	},
	{
		Name:     "StepAdvanceP95",
		Metric:   StepAdvanceDuration,
		Expr:     "histogram_quantile(0.95, step_advance_seconds) > 900",
		Severity: "warning",
		Summary:  "步骤推进 p95 超过 15 分钟：流水线开始变慢，是「卡第 1 步」的最早预警",
	},
	{
		Name:     "TodoStalledP95",
		Metric:   TodoStalledDuration,
		Expr:     "histogram_quantile(0.95, todo_stalled_seconds) > 1800",
		Severity: "warning",
		Summary:  "todo 静默时长 p95 超过 30 分钟：执行侧普遍变慢或大量 run 静默失败",
	},
	{
		Name:     "TodoTimeoutFailed",
		Metric:   TodoTimeoutFailedTotal,
		Expr:     "increase(todo_timeout_failed_total[1h]) > 5",
		Severity: "warning",
		Summary:  "一小时内超时判失败的 todo 超过 5 个：执行节点或模型网关异常",
	},
}

// KnownCounterNames returns every declared counter/histogram name, sorted. Used
// by the ops endpoint and by tests that assert the alert rules cover the
// declared metrics.
func KnownCounterNames() []string {
	names := []string{
		DispatchOrgMismatchTotal,
		TodoAutoAdvancedTotal,
		AgentStepStalledTotal,
		TodoTimeoutFailedTotal,
		StepAdvanceDuration,
		TodoStalledDuration,
	}
	sort.Strings(names)
	return names
}

// Reset clears all counters and histograms. Test-only helper: Phase 0 metrics
// are process-global, so tests must isolate themselves.
func Reset() {
	counters = sync.Map{}
	histograms = sync.Map{}
}
