package metrics

import (
	"testing"
	"time"
)

func TestCountersIncAddAndSnapshot(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	if got := Count("nope"); got != 0 {
		t.Fatalf("unknown counter = %d, want 0", got)
	}

	Inc(DispatchOrgMismatchTotal)
	Inc(DispatchOrgMismatchTotal)
	Add(TodoAutoAdvancedTotal, 3)

	if got := Count(DispatchOrgMismatchTotal); got != 2 {
		t.Fatalf("DispatchOrgMismatchTotal = %d, want 2", got)
	}
	if got := Count(TodoAutoAdvancedTotal); got != 3 {
		t.Fatalf("TodoAutoAdvancedTotal = %d, want 3", got)
	}

	snap := Snapshot()
	if snap[DispatchOrgMismatchTotal] != 2 || snap[TodoAutoAdvancedTotal] != 3 {
		t.Fatalf("snapshot = %v, want both counters present", snap)
	}
}

func TestHistogramQuantilesAreBucketUpperBounds(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	// 90 fast samples land in the <=1s bucket; 10 slow samples land in the
	// <=30m bucket. Quantiles must therefore resolve to those bucket bounds.
	for i := 0; i < 90; i++ {
		Observe(StepAdvanceDuration, 100*time.Millisecond)
	}
	for i := 0; i < 10; i++ {
		Observe(StepAdvanceDuration, 20*time.Minute)
	}

	rep := Histogram(StepAdvanceDuration)
	if rep.Count != 100 {
		t.Fatalf("count = %d, want 100", rep.Count)
	}
	if rep.P50Ms != time.Second.Milliseconds() {
		t.Fatalf("p50 = %dms, want %dms", rep.P50Ms, time.Second.Milliseconds())
	}
	if want := (30 * time.Minute).Milliseconds(); rep.P95Ms != want {
		t.Fatalf("p95 = %dms, want %dms", rep.P95Ms, want)
	}

	wantSum := 90*(100*time.Millisecond).Milliseconds() + 10*(20*time.Minute).Milliseconds()
	if rep.SumMs != wantSum {
		t.Fatalf("sum = %dms, want %dms", rep.SumMs, wantSum)
	}
	if rep.AvgMs != wantSum/100 {
		t.Fatalf("avg = %dms, want %dms", rep.AvgMs, wantSum/100)
	}
	if len(rep.Buckets) != len(histogramBounds)+1 {
		t.Fatalf("buckets = %d, want %d", len(rep.Buckets), len(histogramBounds)+1)
	}
	if rep.Buckets[len(rep.Buckets)-1].UpperBoundMs != 0 {
		t.Fatalf("last bucket must be the +Inf bucket (upper_bound_ms = 0)")
	}
	if last := rep.Buckets[len(rep.Buckets)-1].CumulativeCount; last != 100 {
		t.Fatalf("+Inf cumulative = %d, want 100", last)
	}
}

func TestHistogramEmptyAndNegativeSamples(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	rep := Histogram(TodoStalledDuration)
	if rep.Count != 0 || rep.P50Ms != 0 || rep.P95Ms != 0 || rep.AvgMs != 0 {
		t.Fatalf("empty histogram must be all zeros, got %+v", rep)
	}

	// A negative duration must be clamped to zero rather than corrupting sumNs.
	Observe(TodoStalledDuration, -5*time.Second)
	if got := Histogram(TodoStalledDuration).SumMs; got != 0 {
		t.Fatalf("negative sample sum = %dms, want 0", got)
	}
}

func TestAlertRulesCoverEveryDeclaredMetric(t *testing.T) {
	covered := make(map[string]bool, len(KnownCounterNames()))
	for _, name := range KnownCounterNames() {
		covered[name] = false
	}

	for _, rule := range AlertRules {
		if rule.Name == "" || rule.Metric == "" || rule.Expr == "" || rule.Severity == "" || rule.Summary == "" {
			t.Fatalf("alert rule %q has an empty field: %+v", rule.Name, rule)
		}
		switch rule.Severity {
		case "warning", "critical":
		default:
			t.Fatalf("alert rule %q has unsupported severity %q", rule.Name, rule.Severity)
		}
		if _, known := covered[rule.Metric]; !known {
			t.Fatalf("alert rule %q references undeclared metric %q", rule.Name, rule.Metric)
		}
		covered[rule.Metric] = true
	}

	for name, ok := range covered {
		if !ok {
			t.Fatalf("declared metric %q has no alert rule — T0.11 requires every metric to be alertable", name)
		}
	}
}

func TestKnownCounterNamesAreSortedAndUnique(t *testing.T) {
	names := KnownCounterNames()
	seen := make(map[string]bool, len(names))
	for i, n := range names {
		if seen[n] {
			t.Fatalf("duplicate metric name %q", n)
		}
		seen[n] = true
		if i > 0 && names[i-1] > n {
			t.Fatalf("names not sorted: %q before %q", names[i-1], n)
		}
	}
}

func TestResetClearsCountersAndHistograms(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	Inc(AgentStepStalledTotal)
	Observe(StepAdvanceDuration, time.Minute)
	Reset()

	if got := Count(AgentStepStalledTotal); got != 0 {
		t.Fatalf("counter after reset = %d, want 0", got)
	}
	if got := Histogram(StepAdvanceDuration).Count; got != 0 {
		t.Fatalf("histogram count after reset = %d, want 0", got)
	}
}
