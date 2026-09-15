package store

import (
	"math/rand"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// ─── T2.5 死锁压测（T03 / AC-3 验收） ───
// 高并发随机交错多聚合写：同时猛戳所有取锁 helper（lockAggregates / coarseWithAggregates /
// 单聚合便捷方法 / flushSectionLock）与真实写路径的 early-return 释放分支（CancelTask），
// 持续 ≥30s。watchdog + runtime 栈 dump 探测挂死/死锁；-race 同时覆盖数据竞争。

const deadlockTestDuration = 30 * time.Second

func deadlockDuration() time.Duration {
	if testing.Short() {
		return 5 * time.Second
	}
	return deadlockTestDuration
}

// TestNoDeadlockUnderLockInterleaving 是高并发随机交错取锁压测。它刻意构造「任意顺序、
// 任意聚合组合」的取锁/释放，验证固定升序取锁（不变量 1）+ 禁止反向嵌套（不变量 2）能从
// 构造上消除死锁。CancelTask 的 early-return 路径被并发戳刺，专门回归缺陷 #1（取锁后
// 仅在部分 return 路径释放、漏放内层聚合锁 → 永久死锁）。
func TestNoDeadlockUnderLockInterleaving(t *testing.T) {
	s := New() // mongoEnabled 默认 false：写路径做内存变更并跳过 Mongo，聚焦锁行为。
	dur := deadlockDuration()

	var ops atomic.Int64
	var stalled atomic.Bool
	done := make(chan struct{})

	// worker：每个 goroutine 随机选一种取锁姿势，持极短临界区后释放。
	worker := func(rng *rand.Rand) {
		for {
			select {
			case <-done:
				return
			default:
			}
			switch rng.Intn(7) {
			case 0:
				rel := s.lockAggregates(AggProject, AggTask, AggMeeting, AggProjectFile)
				busyWork(rng)
				rel()
			case 1:
				rel := s.coarseWithAggregates(AggProject, AggTask)
				busyWork(rng)
				rel()
			case 2:
				rel := s.coarseWithAggregates(AggMeeting, AggProject)
				busyWork(rng)
				rel()
			case 3:
				rel := s.lockProject()
				busyWork(rng)
				rel()
			case 4:
				rel := s.lockTask()
				busyWork(rng)
				rel()
			case 5:
				rel := s.flushSectionLock(AggProject)
				busyWork(rng)
				rel()
			case 6:
				// 真实写路径 early-return：对不存在/已存在的任务调 CancelTask，
				// 命中 release() 分支（缺陷 #1 修复点）。
				id := randomTaskID(rng, 8)
				_, _ = s.CancelTask(Scope{UserID: "tester"}, TaskCancelInput{TaskID: id})
			}
			ops.Add(1)
		}
	}

	const workers = 32
	for i := 0; i < workers; i++ {
		go worker(rand.New(rand.NewSource(time.Now().UnixNano() + int64(i))))
	}

	// watchdog：每 500ms 检查一次进度；若连续 5s 无进展，判定死锁并 dump 全栈。
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		last := int64(0)
		noProgressTicks := 0
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				cur := ops.Load()
				if cur == last {
					noProgressTicks++
					if noProgressTicks >= 10 { // 5s 无进展
						stalled.Store(true)
						buf := make([]byte, 1<<20)
						n := runtime.Stack(buf, true)
						t.Errorf("DEADLOCK SUSPECTED: 无进度超过 5s；goroutine 栈:\n%s", buf[:n])
						return
					}
				} else {
					noProgressTicks = 0
					last = cur
				}
			}
		}
	}()

	select {
	case <-time.After(dur):
	case <-blockUntilStalled(&stalled):
	}

	close(done)

	if stalled.Load() {
		t.Fatalf("死锁压测在 %s 内检测到挂死（见上方栈）", dur)
	}
	// 收尾后再戳一次 CancelTask，确认锁状态已完全释放、可正常获取。
	for i := 0; i < 100; i++ {
		_, _ = s.CancelTask(Scope{UserID: "tester"}, TaskCancelInput{TaskID: "never-existed"})
	}
	t.Logf("死锁压测完成：%s 内 %d 次取锁操作，无死锁", dur, ops.Load())
}

// blockUntilStalled 在 stalled 置位时立即返回，供主流程感知 watchdog 判定。
// QA（严过关）修正：原实现按值传参 atomic.Bool，会拷贝锁值（go vet 报
// "passes lock by value"），且轮询 goroutine 读到的是调用瞬间的**副本**，
// watchdog 的 Store(true) 永远不会被观察到 ⇒ 死锁提前中断链路是死代码。
// 改为传指针，恢复「检测到挂死立即结束压测」的语义。
func blockUntilStalled(stalled *atomic.Bool) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		for !stalled.Load() {
			time.Sleep(10 * time.Millisecond)
		}
		close(ch)
	}()
	return ch
}

// busyWork 模拟极小临界区工作（不触 Mongo），制造取锁交错窗口。
func busyWork(rng *rand.Rand) {
	_ = rng.Intn(1 << 20)
	time.Sleep(time.Duration(rng.Intn(50)) * time.Microsecond)
}

// randomTaskID 返回 [0,n) 区间内的任务 ID 字符串（供 CancelTask 随机戳刺）。
// store 内存为空，故这些 ID 必然命中 CancelTask 的「task not found」early-return 分支，
// 该分支正是缺陷 #1 的修复点（取锁后必须在 return 前调用 release()）。
func randomTaskID(rng *rand.Rand, n int) string {
	return "task-" + string(rune('0'+rng.Intn(n)))
}
