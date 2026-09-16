package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// fakeLeaderCAS 是 leaderCAS 的内存实现，模拟 Mongo 单文档 CAS 语义：
// 「持有者本人」或「租约已过期」→ 获取成功；他人持有且未过期 → (false, nil)。
// 可选 failErr 模拟 Mongo 抖动（CAS 本身失败）。
type fakeLeaderCAS struct {
	mu       sync.Mutex
	holder   string
	expireAt time.Time
	failErr  error
	calls    int
}

func (f *fakeLeaderCAS) tryAcquireOrRenew(_ context.Context, key, holder string, now time.Time, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.failErr != nil {
		return false, f.failErr
	}
	if f.holder == "" || f.holder == holder || now.After(f.expireAt) {
		f.holder = holder
		f.expireAt = now.Add(ttl)
		return true, nil
	}
	return false, nil
}

func (f *fakeLeaderCAS) release(_ context.Context, key, holder string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.holder == holder {
		f.holder = ""
		f.expireAt = time.Time{}
	}
	return nil
}

func newLeaderStore(cas leaderCAS) *Store {
	s := New()
	s.log = zap.NewNop()
	s.leaderElectionEnabled = true
	s.leaderCAS = cas
	return s
}

// 门禁关闭（默认）→ isLeaderForBackground 恒 true，单实例行为与改造前一致。
func TestLeaderGateDisabledAlwaysAllowsTickers(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	if s.leaderElectionEnabled {
		t.Fatalf("leader election must default to disabled")
	}
	if !s.isLeaderForBackground() {
		t.Fatalf("gate disabled must allow background tickers (single-instance parity)")
	}
	if s.LeaderElectionArmed() {
		t.Fatalf("gate disabled → not armed → StartLeaderElection must not run")
	}
}

// 单实例首轮：无人持有 → 抢到租约 → leader。
func TestLeaderAcquiresLeaseOnFirstRound(t *testing.T) {
	cas := &fakeLeaderCAS{}
	s := newLeaderStore(cas)

	s.renewLeadershipOnce(time.Now().UTC())

	if !s.isLeader.Load() {
		t.Fatalf("first CAS round must acquire leadership")
	}
	if !s.isLeaderForBackground() {
		t.Fatalf("leader must pass the ticker gate")
	}
}

// 双实例竞争：B 在 A 持有且未过期时尝试 → 非 leader（消除双派的核心断言）。
func TestLeaderSecondInstanceIsBlocked(t *testing.T) {
	cas := &fakeLeaderCAS{}
	a := newLeaderStore(cas)
	a.leaderID = "instance-A"
	b := newLeaderStore(cas)
	b.leaderID = "instance-B"

	now := time.Now().UTC()
	a.renewLeadershipOnce(now)
	b.renewLeadershipOnce(now)

	if !a.isLeader.Load() {
		t.Fatalf("A acquired first and must stay leader")
	}
	if b.isLeader.Load() {
		t.Fatalf("B must NOT be leader while A holds a fresh lease (no double dispatch)")
	}
	if b.isLeaderForBackground() {
		t.Fatalf("B must skip its background tickers")
	}
}

// 租约过期接管：A 停止续租，TTL 过后 B 抢占成功（故障转移路径）。
func TestLeaderTakeoverAfterLeaseExpiry(t *testing.T) {
	cas := &fakeLeaderCAS{}
	a := newLeaderStore(cas)
	a.leaderID = "instance-A"
	b := newLeaderStore(cas)
	b.leaderID = "instance-B"

	t0 := time.Now().UTC()
	a.renewLeadershipOnce(t0)
	if !a.isLeader.Load() {
		t.Fatalf("A must acquire at t0")
	}

	// A 挂掉不再续租；t0 + TTL + 1s 后 B 尝试 → 租约已过期 → B 接管。
	later := t0.Add(leaderLeaseTTL + time.Second)
	b.renewLeadershipOnce(later)
	if !b.isLeader.Load() {
		t.Fatalf("B must take over after A's lease expired")
	}

	// A 复活后继续续租 → CAS 判他人持有且未过期 → A 降级为非 leader（不抢回、无双跑）。
	a.renewLeadershipOnce(later.Add(time.Second))
	if a.isLeader.Load() {
		t.Fatalf("A must step down once B holds a fresh lease")
	}
}

// 持有者持续续租不会自我阻塞（幂等续租）。
func TestLeaderRenewIsIdempotentForHolder(t *testing.T) {
	cas := &fakeLeaderCAS{}
	s := newLeaderStore(cas)

	now := time.Now().UTC()
	s.renewLeadershipOnce(now)
	s.renewLeadershipOnce(now.Add(leaderRenewInterval))
	s.renewLeadershipOnce(now.Add(2 * leaderRenewInterval))

	if !s.isLeader.Load() {
		t.Fatalf("holder must keep leadership across renew cycles")
	}
}

// fail-safe：CAS 报错（Mongo 抖动）→ 降级为非 leader，宁可暂停安全网也不冒双派风险。
func TestLeaderCasErrorDegradesToNonLeader(t *testing.T) {
	cas := &fakeLeaderCAS{}
	s := newLeaderStore(cas)

	now := time.Now().UTC()
	s.renewLeadershipOnce(now)
	if !s.isLeader.Load() {
		t.Fatalf("precondition: s must be leader before the error round")
	}

	cas.failErr = errors.New("mongo server selection timeout")
	s.renewLeadershipOnce(now.Add(leaderRenewInterval))

	if s.isLeader.Load() {
		t.Fatalf("CAS error must demote to non-leader (fail-safe: pause safety nets, never double-dispatch)")
	}
	if s.isLeaderForBackground() {
		t.Fatalf("demoted instance must skip its background tickers")
	}

	// Mongo 恢复 → 下一轮重新确认 leader（租约仍是自己的，未过期）。
	cas.failErr = nil
	s.renewLeadershipOnce(now.Add(2 * leaderRenewInterval))
	if !s.isLeader.Load() {
		t.Fatalf("recovered CAS must reclaim leadership while lease is still ours")
	}
}

// 优雅停机：ctx 取消 → 释放租约 → 新实例无需等 TTL 即可接管。
func TestLeaderReleaseOnShutdownEnablesFastTakeover(t *testing.T) {
	cas := &fakeLeaderCAS{}
	a := newLeaderStore(cas)
	a.leaderID = "instance-A"
	b := newLeaderStore(cas)
	b.leaderID = "instance-B"

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.StartLeaderElection(ctx)
		close(done)
	}()

	// 等 A 完成首轮抢占。
	deadline := time.Now().Add(2 * time.Second)
	for !a.isLeader.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !a.isLeader.Load() {
		t.Fatalf("A must acquire leadership after StartLeaderElection")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("StartLeaderElection must return on ctx cancel")
	}

	// A 已释放 → B 立即接管（无需等 leaderLeaseTTL）。
	b.renewLeadershipOnce(time.Now().UTC())
	if !b.isLeader.Load() {
		t.Fatalf("B must take over immediately after A released the lease")
	}
}

// StartLeaderElection 在 CAS 未接线时绝不 panic（防御：app 层误启动）。
func TestLeaderElectionNilCASIsNoop(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	s.leaderElectionEnabled = true
	// leaderCAS 保持 nil
	if s.LeaderElectionArmed() {
		t.Fatalf("nil CAS must not be armed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.StartLeaderElection(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("StartLeaderElection with nil CAS must exit on ctx cancel")
	}
}
