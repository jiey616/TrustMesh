package store

import (
	"testing"
)

// ─── T2.5 锁基础设施单测（T01 验收） ───
// 覆盖：Aggregate 枚举值稳定、固定升序取锁断言、去重、单聚合便捷方法等价性、
// 释放后栈清空、以及 debug 持锁追踪下的反向嵌套 panic（AC-6 运行期护栏）。

// snapshotHeld 在 lockTrackingMu 保护下复制当前持锁栈（debug 追踪专用）。
func snapshotHeld() []Aggregate {
	lockTrackingMu.Lock()
	defer lockTrackingMu.Unlock()
	out := make([]Aggregate, len(lockHeldStack))
	copy(out, lockHeldStack)
	return out
}

// TestAggregateEnumStable 守护「枚举值即全局固定取锁顺序」契约：新增聚合只能追加在
// numAggregates 之前，已定义枚举的**数值**不得变动，否则会破坏所有 lockAggregates /
// coarseWithAggregates 调用点的取锁顺序 → 死锁。
func TestAggregateEnumStable(t *testing.T) {
	cases := []struct {
		name string
		got  Aggregate
		want Aggregate
	}{
		{"AggUser", AggUser, 0},
		{"AggAgent", AggAgent, 1},
		{"AggOrganization", AggOrganization, 2},
		{"AggProject", AggProject, 3},
		{"AggProjectFile", AggProjectFile, 4},
		{"AggTask", AggTask, 5},
		{"AggMeeting", AggMeeting, 6},
		{"AggAgentChat", AggAgentChat, 7},
		{"AggKnowledge", AggKnowledge, 8},
		{"AggWorkflowTemplate", AggWorkflowTemplate, 9},
		{"AggJoinRequest", AggJoinRequest, 10},
		{"AggNotification", AggNotification, 11},
		{"AggExternalApp", AggExternalApp, 12},
		{"AggOpsIncident", AggOpsIncident, 13},
		{"AggProcessedMessage", AggProcessedMessage, 14},
		{"numAggregates", numAggregates, 15},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s 枚举值漂移：got %d, want %d", c.name, c.got, c.want)
		}
	}
}

// TestLockAggregatesAscendingOrder 断言 lockAggregates 内部按枚举升序取锁（无论调用方传入
// 顺序如何），且释放闭包逆序释放。
func TestLockAggregatesAscendingOrder(t *testing.T) {
	ResetLockTracking()
	SetLockTracking(true)
	defer ResetLockTracking()

	s := New()
	// 故意乱序传入：AggTask(5) / AggMeeting(6) / AggProject(3)
	release := s.lockAggregates(AggTask, AggMeeting, AggProject)

	got := snapshotHeld()
	want := []Aggregate{AggProject, AggTask, AggMeeting} // 升序（枚举值 3,5,6）
	if len(got) != len(want) {
		t.Fatalf("持锁数错误：got %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("取锁顺序非升序：idx %d got %v want %v（full=%v）", i, got[i], want[i], got)
		}
	}

	release()
	if got := snapshotHeld(); len(got) != 0 {
		t.Errorf("release 后持锁栈应清空，实际仍残留 %v", got)
	}
}

// TestLockAggregatesDedup 断言重复传入同一聚合只取一次锁（去重）。
func TestLockAggregatesDedup(t *testing.T) {
	ResetLockTracking()
	SetLockTracking(true)
	defer ResetLockTracking()

	s := New()
	release := s.lockAggregates(AggTask, AggTask, AggProject, AggProject)
	got := snapshotHeld()
	if len(got) != 2 {
		t.Fatalf("去重失败：期望持 2 把锁，实际 %v", got)
	}
	release()
}

// TestSingleAggregateConvenience 断言 lockProject / lockTask / lockMeeting /
// lockProjectFile 分别等价于 lockAggregates(对应聚合)，只持一把锁。
func TestSingleAggregateConvenience(t *testing.T) {
	ResetLockTracking()
	SetLockTracking(true)
	defer ResetLockTracking()

	s := New()
	checks := []struct {
		agg    Aggregate
		lockFn func() func()
	}{
		{AggProject, s.lockProject},
		{AggProjectFile, s.lockProjectFile},
		{AggTask, s.lockTask},
		{AggMeeting, s.lockMeeting},
	}
	for _, c := range checks {
		release := c.lockFn()
		got := snapshotHeld()
		if len(got) != 1 || got[0] != c.agg {
			t.Errorf("便捷方法持锁错误：期望仅持 %v，实际 %v", c.agg, got)
		}
		release()
		if got := snapshotHeld(); len(got) != 0 {
			t.Errorf("%v release 后栈未清空：%v", c.agg, got)
		}
	}
}

// TestCoarseAllowedDirection 断言 coarseWithAggregates 的允许方向（先 s.mu 外层、后聚合锁
// 内层）不触发 panic，且释放后全部归还。
func TestCoarseAllowedDirection(t *testing.T) {
	ResetLockTracking()
	SetLockTracking(true)
	defer ResetLockTracking()

	s := New()
	release := s.coarseWithAggregates(AggProject, AggTask)
	// 粗粒度模式下 s.mu 由 coarseWithAggregates 内部持有（无法从聚合栈观测），
	// 聚合栈应升序记录两聚合。
	got := snapshotHeld()
	if len(got) != 2 || got[0] != AggProject || got[1] != AggTask {
		t.Errorf("coarse 模式聚合栈异常：%v", got)
	}
	release()
	if got := snapshotHeld(); len(got) != 0 {
		t.Errorf("coarse release 后聚合栈未清空：%v", got)
	}
}

// TestReverseNestingPanic 断言 AC-6 运行期护栏：已持聚合锁后再取 s.mu（反向嵌套），
// 以及已持聚合锁后再取更多聚合锁（嵌套），在 debug 追踪开启时均 panic。
func TestReverseNestingPanic(t *testing.T) {
	ResetLockTracking()
	SetLockTracking(true)
	defer ResetLockTracking()

	s := New()

	// 场景 1：持 AggProject 后再 coarseWithAggregates（取 s.mu）→ 反向嵌套 panic。
	func() {
		release1 := s.lockProject()
		defer func() {
			if r := recover(); r == nil {
				t.Error("场景1：持聚合锁后取 s.mu 应 panic，但未 panic")
			} else {
				release1() // 仅 AggProject 已持，coarse 在 panic 前未取任何锁
			}
		}()
		_ = s.coarseWithAggregates(AggTask)
	}()

	// 场景 2：持 AggProject 后再 lockTask（嵌套取聚合锁）→ panic。
	func() {
		release1 := s.lockProject()
		defer func() {
			if r := recover(); r == nil {
				t.Error("场景2：持聚合锁后再取更多聚合锁应 panic，但未 panic")
			} else {
				release1()
			}
		}()
		_ = s.lockTask()
	}()

	// 触发后应能正常取锁，证明状态已恢复、未被死锁残留污染。
	func() {
		release := s.lockAggregates(AggProject, AggTask)
		defer release()
		if got := snapshotHeld(); len(got) != 2 {
			t.Errorf("护栏恢复后取锁异常：%v", got)
		}
	}()
}
