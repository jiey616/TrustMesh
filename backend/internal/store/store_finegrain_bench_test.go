package store

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── T2.5 AC-1 / AC-3 断言型回归（基准只能看数字，断言才能守 CI） ───

// TestSingleAggregatePersistDoesNotBlockOtherAggregates 是 AC-1 的**断言型**验收：
// 模拟「某聚合（AggProject）正在持锁做长 persist（最坏 MONGO_TIMEOUT=5s）」，
// 期间其它聚合（AggTask / AggMeeting / AggProjectFile）的读写必须**几乎无等待**完成。
//
// 自带对照组：同一把 AggProject 锁此刻必须**确实阻塞**，否则说明 holder 根本没持上锁、
// 测量无意义（防止测试恒绿穿透）。若把锁退化回全局锁，本用例立即变红。
func TestSingleAggregatePersistDoesNotBlockOtherAggregates(t *testing.T) {
	s := New()
	const hold = 400 * time.Millisecond

	acquired := make(chan struct{})
	go func() {
		rel := s.lockProject() // 模拟 AggProject 长 persist 临界区
		close(acquired)
		time.Sleep(hold)
		rel()
	}()
	<-acquired // 确保 holder 已真正持上 AggProject

	// ① 主断言：其它聚合不被阻塞。
	for _, c := range []struct {
		name string
		lock func(*Store) func()
	}{
		{"task", func(s *Store) func() { return s.lockTask() }},
		{"meeting", func(s *Store) func() { return s.lockMeeting() }},
		{"projectFile", func(s *Store) func() { return s.lockProjectFile() }},
		{"task+meeting", func(s *Store) func() { return s.lockAggregates(AggMeeting, AggTask) }},
	} {
		start := time.Now()
		rel := c.lock(s)
		rel()
		if d := time.Since(start); d > hold/4 {
			t.Errorf("AC-1 违规：持有 AggProject 期间取 %s 锁等待 %v（阈值 %v），细粒度锁未生效",
				c.name, d, hold/4)
		}
	}

	// ② 对照组：同聚合此刻必须阻塞，证明测量有效。
	selfDone := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		rel := s.lockProject()
		rel()
		selfDone <- time.Since(start)
	}()
	select {
	case d := <-selfDone:
		if d < hold/2 {
			t.Errorf("对照组失效：同聚合 AggProject 仅等待 %v 即获得锁，holder 可能未真正持锁，本用例测量无意义", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("对照组超时：同聚合 AggProject 无法在 5s 内获得锁")
	}
}

// TestCancelTaskEarlyReturnReleasesAggregateLocks 是缺陷 #1 的**定点回归护栏**：
// CancelTask 命中 early-return（task not found）后，必须把外层 s.mu **与内层
// AggProject/AggTask 聚合锁**全部释放；否则后续任何触及这两把锁的操作永久死锁。
// 修复前该用例必然超时变红。
func TestCancelTaskEarlyReturnReleasesAggregateLocks(t *testing.T) {
	s := New()

	// 命中「task not found」early-return 分支（缺陷 #1 的修复点之一）。
	if _, appErr := s.CancelTask(Scope{UserID: "tester"}, TaskCancelInput{TaskID: "never-existed"}); appErr == nil {
		t.Fatal("期望 CancelTask 对不存在的任务返回错误")
	}

	// 该 store 无 Mongo、无 agent，故 CancelTask 必然走 early-return。
	// 若 release() 未释放内层聚合锁，下面的取锁会永久阻塞。
	probe := func(name string, lock func(*Store) func()) {
		done := make(chan struct{})
		go func() {
			rel := lock(s)
			rel()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("缺陷 #1 回归：CancelTask early-return 后 %s 锁仍不可获取（内层聚合锁泄漏）", name)
		}
	}
	probe("AggTask", func(s *Store) func() { return s.lockTask() })
	probe("AggProject", func(s *Store) func() { return s.lockProject() })
	probe("AggProject+AggTask", func(s *Store) func() { return s.lockAggregates(AggProject, AggTask) })
}

// ─── T2.5 并发基准（T04 / AC-1 验收） ───
// 证明「单聚合 persist 期间，其它聚合并发读写不被阻塞」：
//   - finegrained：某聚合持续持其细粒度锁（如 AggProject），其它聚合（AggTask / AggMeeting /
//     AggProjectFile）的读写只取自身聚合锁，与前者无冲突 → 高吞吐。
//   - coarse（遗留全局锁基线）：所有操作都挤在 s.mu 上 → 其它聚合被串行化 → 低吞吐。
// 二者差距即细粒度锁带来的并发度收益。

const benchHoldMs = 2 * time.Millisecond

func benchWindow() time.Duration {
	if testing.Short() {
		return 200 * time.Millisecond
	}
	return 1 * time.Second
}

// BenchmarkSingleAggregatePersistDoesNotBlockOthers 对比在「持有某聚合锁做 persist」期间，
// 另一聚合的并发读写吞吐。finegrained 子基准应显著高于 coarse 子基准。
func BenchmarkSingleAggregatePersistDoesNotBlockOthers(b *testing.B) {
	b.Run("finegrained", func(b *testing.B) { runContentionBench(b, false) })
	b.Run("coarse_legacy", func(b *testing.B) { runContentionBench(b, true) })
}

// runContentionBench 固定时间窗内，统计「被压测聚合」(AggTask) 的并发操作数。
//   - coarse=true：holder 与 workers 都走 s.mu（遗留全局锁）——其它聚合被迫串行；
//   - coarse=false：holder 持 AggProject（不取 s.mu），workers 持 AggTask——互不阻塞。
func runContentionBench(b *testing.B, coarse bool) {
	s := New()
	stop := make(chan struct{})

	// holder：持续「持锁 → 模拟 persist 临界区 → 释放」，模拟某聚合的长 persist。
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			var rel func()
			if coarse {
				s.mu.Lock()
				rel = func() { s.mu.Unlock() }
			} else {
				rel = s.lockProject() // 仅持 AggProject，不取 s.mu
			}
			time.Sleep(benchHoldMs)
			rel()
		}
	}()

	const workers = 8
	var wg sync.WaitGroup
	var count int64
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if coarse {
					// 遗留基线：所有操作都挤在全局 s.mu。
					s.mu.Lock()
					s.mu.Unlock()
				} else {
					// 细粒度：被压测聚合只取自身锁，与 holder 的 AggProject 无冲突。
					rel := s.lockTask()
					rel()
				}
				atomic.AddInt64(&count, 1)
			}
		}()
	}

	b.ResetTimer()
	time.Sleep(benchWindow())
	close(stop)
	wg.Wait()
	b.StopTimer()

	elapsed := benchWindow().Seconds()
	opsPerSec := float64(count) / elapsed
	b.ReportMetric(opsPerSec, "task_ops/sec")
	b.ReportMetric(float64(benchHoldMs.Microseconds()), "hold_us")
}

// BenchmarkAggregateIndependence 进一步证明：被压测聚合换成 AggMeeting / AggProjectFile 时，
// finegrained 模式同样不被持有 AggProject 的 persist 阻塞（AC-1 的聚合独立性）。
func BenchmarkAggregateIndependence(b *testing.B) {
	cases := []struct {
		name string
		lock func(*Store) func()
	}{
		{"task", func(s *Store) func() { return s.lockTask() }},
		{"meeting", func(s *Store) func() { return s.lockMeeting() }},
		{"projectFile", func(s *Store) func() { return s.lockProjectFile() }},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			s := New()
			stop := make(chan struct{})
			go func() { // holder 持续持 AggProject
				for {
					select {
					case <-stop:
						return
					default:
					}
					rel := s.lockProject()
					time.Sleep(benchHoldMs)
					rel()
				}
			}()
			const workers = 8
			var wg sync.WaitGroup
			var count int64
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-stop:
							return
						default:
						}
						rel := c.lock(s)
						rel()
						atomic.AddInt64(&count, 1)
					}
				}()
			}
			b.ResetTimer()
			time.Sleep(benchWindow())
			close(stop)
			wg.Wait()
			b.StopTimer()
			b.ReportMetric(float64(count)/benchWindow().Seconds(), "ops/sec")
		})
	}
}
