package store

// ─── T2.5 不变量 3 永久护栏（QA software-qa-engineer-2，2026-09-16） ───
//
// 原文件为 T2.5 的**独立对抗验证**集（8 个用例，其中 6 个故意为红，用于证伪
// 「四域细粒度锁迁移已完成」这一结论）。2026-09-16 主理人拍板 **方案 A：回退 T2.5
// 调用点改动、保留锁基础设施** 后，本文件按决策精简：
//
//   - **删除** 6 个「证伪已放弃迁移效果」的用例（AC-1 真实 API 阻塞、AC-6 护栏假阴性/
//     假阳性、AC-3 CancelTask 全分支、AC-5 临界区收敛及其活库变体）—— 它们断言的
//     是已放弃的行为，回退后保留只会制造噪音或假红。
//   - **保留** TestQAT25_Invariant3_SameMapMustHaveOneProtectingLock —— 它守护的是
//     与迁移进度无关的**永久不变量**：同一张 map 只能有一把保护锁。回退后应为绿，
//     且在未来 T05 做原子性整批迁移时，一旦再次出现「部分访问者迁到聚合锁、其余
//     仍持 s.mu」的半吊子状态，它会立刻转红。这是本文件存在的唯一理由。
//
// 设计约束不变：本文件**不修改任何实现代码**，只做观测与断言。

import (
	"testing"
	"time"
)

const (
	// qaProbeTimeout 是「判定某 API 是否被阻塞」的阈值。远大于纯内存 NotFound 路径的
	// 正常耗时（µs 级），远小于任何真实 persist 临界区（ms~5s 级），可稳定区分。
	qaProbeTimeout = 300 * time.Millisecond
)

// qaBlocksOnGlobalMu 在**独占持有全局兜底锁 s.mu** 的前提下调用 call：
// 若 call 在 qaProbeTimeout 内完成，说明它不依赖 s.mu；否则说明它仍需全局锁。
func qaBlocksOnGlobalMu(s *Store, call func()) bool {
	s.mu.Lock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		call()
	}()
	blocked := false
	select {
	case <-done:
	case <-time.After(qaProbeTimeout):
		blocked = true
	}
	s.mu.Unlock()
	if blocked {
		<-done // 待其跑完，避免脏 goroutine 干扰后续断言
	}
	return blocked
}

// TestQAT25_Invariant3_SameMapMustHaveOneProtectingLock 用**确定性**方式校验不变量 3
// （每把 map 只有一个保护锁，禁止同一 map 被两套锁并发保护）。
//
// 判定逻辑（双向，绿=不变量成立，红=缺口）：
//   - 若 B 类纯聚合锁路径（CheckTaskProjectActive）**也**取 s.mu ⇒ 与 A 类互斥 ⇒ 成立；
//   - 否则若 A 类裸 s.mu 路径（SetTodoAssignedAt 等）**也**取聚合锁 ⇒ 仍互斥 ⇒ 成立；
//   - 两者都不成立 ⇒ 同一张 s.tasks 被两把互不互斥的锁保护 ⇒ 缺口。
//
// 2026-09-16 T2.5 回退后的期望：**绿**。理由：调用点已整体回退至基线，CheckTaskProjectActive
// 与 SetTodoAssignedAt / GetTimedOutTodos 等所有 s.tasks 访问者统一由 s.mu 保护，
// 不存在第二把锁 ⇒ 不变量 3 成立。
//
// 反向验证（保持本用例有鉴别力）：若未来 T05 只把部分访问者迁到聚合锁（留下其余
// 访问者仍持 s.mu），本用例立即转红 —— 这正是 T2.5 被推翻的 P0 数据竞争根因。
func TestQAT25_Invariant3_SameMapMustHaveOneProtectingLock(t *testing.T) {
	s := New()

	// 步骤 1：B 类（纯聚合锁）路径是否也取 s.mu？
	bTakesMu := qaBlocksOnGlobalMu(s, func() { _ = s.CheckTaskProjectActive("t") })
	if bTakesMu {
		return // 互斥成立
	}

	// 步骤 2：B 类不取 s.mu，那 A 类（裸 s.mu）路径是否取聚合锁？
	rel := s.lockAggregates(AggProject, AggTask)
	aDone := make(chan struct{})
	go func() {
		defer close(aDone)
		s.SetTodoAssignedAt("t", "todo") // workflow/timeout_monitor 类：仅 s.mu
		s.GetTimedOutTodos()             // 仅 s.mu
	}()
	aTakesAgg := false
	select {
	case <-aDone: // A 类在聚合锁被占用时仍能完成 ⇒ 它不取聚合锁
		aTakesAgg = false
	case <-time.After(qaProbeTimeout):
		aTakesAgg = true
	}
	rel()
	if !aTakesAgg {
		<-aDone
	}

	if aTakesAgg {
		return // 互斥成立
	}
	t.Errorf("不变量 3 违反：s.tasks 同时被「s.mu（A 类：SetTodoAssignedAt / UpdateTodo / " +
		"AddTaskComment 等，均只持 s.mu）」与「AggProject+AggTask（B 类：CheckTaskProjectActive，" +
		"store_project.go，只持聚合锁）」保护，两把锁互不互斥 ⇒ 并发读写 s.tasks 无互斥保障。" +
		"生产可达：CheckTaskProjectActive 由 handler/task.go 与 clawsynapse/webhook.go 调用，" +
		"A 类写路径由 timeout_monitor / store_planning 等定时器与 HTTP 写接口并发触发")
}
