package store

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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
