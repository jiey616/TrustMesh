package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/config"
)

// T0.8 有界停机落盘测试。
//
// 覆盖两条最关键的契约：
//  1. 未启用 Mongo 时是纯粹 no-op（不能因为落盘把停机流程搞挂）。
//  2. ctx 到期时 fail-fast（有界性的核心：绝不无限挂起，否则会被 SIGKILL 反而必丢）。

// TestFlushPersistAllNoMongoIsNoop 验证 mongoEnabled=false 的 Store 上调用
// FlushPersistAll 直接返回 nil，且几乎不耗时（首行守卫短路，不触碰任何网络）。
func TestFlushPersistAllNoMongoIsNoop(t *testing.T) {
	// 空 Store：mongoEnabled=false、mongoClient=nil，命中 FlushPersistAll 首行守卫。
	s := &Store{}

	start := time.Now()
	err := s.FlushPersistAll(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("FlushPersistAll on mongo-disabled store = %v, want nil", err)
	}
	if elapsed > time.Second {
		t.Fatalf("no-op flush took %s, want < 1s", elapsed)
	}
}

// TestFlushPersistAllCanceledContextFailsFast 验证：当 Mongo 已连接时，传入一个
// 已取消的 ctx，FlushPersistAll 必须在第一类实体开始前的 ctx 检查点立即返回
// context.Canceled，且耗时远小于 30s 的停机回写预算。
//
// 为什么门控：FlushPersistAll 首行有 `if !s.mongoEnabled || s.mongoClient == nil`
// 守卫——在无 Mongo 的环境里会直接短路返回 nil，根本走不到 ctx 检查分支，无法验证
// 「有界」这条路径。因此本用例仅在设置了 TRUSTMESH_TEST_MONGO_URI 时执行，未设置时
// 自动 SKIP，不影响 CI（与 meeting_rehydrate_test.go 同一门控约定）。
func TestFlushPersistAllCanceledContextFailsFast(t *testing.T) {
	uri := os.Getenv("TRUSTMESH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("skip: set TRUSTMESH_TEST_MONGO_URI to a reachable MongoDB to exercise the bounded-flush ctx path; without a live mongo the mongoEnabled guard short-circuits before the ctx check")
	}
	db := os.Getenv("TRUSTMESH_TEST_MONGO_DB")
	if db == "" {
		db = "trustmesh_qa_test"
	}

	s := New()
	cfg := config.Config{
		MongoEnabled:  true,
		MongoURI:      uri,
		MongoDatabase: db,
		MongoTimeout:  10 * time.Second,
	}
	if err := s.enableMongo(cfg, zap.NewNop()); err != nil {
		t.Skipf("skip: mongo unavailable: %v", err)
	}
	defer func() { _ = s.Close() }()

	// 已取消的 ctx：走 ctx 检查点，必须立即返回，不发起任何写操作。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := s.FlushPersistAll(ctx)
	elapsed := time.Since(start)

	// 使用 errors.Is：即便未来把 ctx 错误包进聚合错误，语义仍然是 context.Canceled。
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("FlushPersistAll(canceled ctx) = %v, want context.Canceled", err)
	}
	if elapsed >= 30*time.Second {
		t.Fatalf("canceled-context flush took %s, want well under the 30s shutdown budget", elapsed)
	}
	if elapsed > time.Second {
		t.Fatalf("canceled-context flush took %s, want < 1s (fail-fast)", elapsed)
	}
}
