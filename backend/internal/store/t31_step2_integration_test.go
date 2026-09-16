package store

// T3.1 第 2 步集成测试（W1 冲突回源 + W2 SSE 跨实例广播 + 第 1 步真 Mongo CAS）。
//
// 需要真实 Mongo：设 TRUSTMESH_TEST_MONGO_URI 才运行，否则 skip（沿用 t2.6 既有惯例）。
// 每个用例用**独立数据库名**并在结束时 drop，绝不触碰生产 trustmesh 库。
//
// 本地跑法（一次性 mongo，用完即删）：
//   docker run -d --name t31mongo -p 27018:27017 mongodb/mongodb-community-server:7.0-ubuntu2204
//   TRUSTMESH_TEST_MONGO_URI=mongodb://localhost:27018 go test -race -run TestT31 ./internal/store/

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"

	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// t31Store 建一个接真 Mongo 的 Store。enableMongo 内部会 loadMongoState，
// 因此**构造顺序决定了各实例看到的初始版本**——这是本组用例的关键手法。
func t31Store(t *testing.T, dbName string, tune func(*Store)) *Store {
	t.Helper()
	uri := os.Getenv("TRUSTMESH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("skip: set TRUSTMESH_TEST_MONGO_URI to run T3.1 integration tests")
	}
	s := New()
	cfg := config.Config{
		MongoEnabled: true, MongoURI: uri, MongoDatabase: dbName,
		MongoTimeout: 10 * time.Second,
	}
	if err := s.enableMongo(cfg, zap.NewNop()); err != nil {
		t.Skipf("skip: mongo unavailable: %v", err)
	}
	s.log = zap.NewNop()
	if tune != nil {
		tune(s)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if s.mongoClient != nil {
			_ = s.mongoClient.Database(dbName).Drop(ctx)
		}
	})
	return s
}

// seedTask 把一张 version=1 的任务同时塞进本实例内存和 Mongo（模拟两实例同源载入）。
func seedTask(t *testing.T, s *Store, id, title string) {
	t.Helper()
	task := &model.TaskDetail{
		ID: id, UserID: "u1", OrgID: "org1", ProjectID: "p1",
		Status: "in_progress", Title: title, Version: 1,
		Todos: []model.Todo{{ID: "td1", Status: "pending"}},
	}
	s.mu.Lock()
	s.tasks[id] = copyTask(task)
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.mongoTasks.InsertOne(ctx, task); err != nil {
		t.Fatalf("seed task doc: %v", err)
	}
}

// writeTitle 走真实提交原语 mutateTaskUnsafe（需持写锁），返回其错误。
func writeTitle(s *Store, taskID, title string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateTaskUnsafe(taskID, func(tk *model.TaskDetail) *transport.AppError {
		tk.Title = title
		return nil
	})
}

// ─── W1：版本冲突后回源刷新 ───

// 核心回归：双实例共享同一 Mongo，A 因持有落后版本而写冲突时，
// A 冲突后**必须**能读到 B 写入的新值。改造前 A 会持续吐旧快照（脏窗口无界）。
func TestT31W1LoserReadsWinnerValueAfterConflict(t *testing.T) {
	db := "trustmesh_t31_w1_conflict"
	a := t31Store(t, db, nil)
	seedTask(t, a, "t1", "orig")
	// B 在种子之后构造 → 载入 version=1 的同一文档。
	b := t31Store(t, db, nil)

	// B 成功写入：Mongo version 1 → 2。
	if err := writeTitle(b, "t1", "written-by-B"); err != nil {
		t.Fatalf("B write must succeed, got %+v", err)
	}

	// A 内存仍是 version=1 → 必然 409。
	err := writeTitle(a, "t1", "written-by-A")
	if err == nil {
		t.Fatalf("A write must conflict (A holds a stale version)")
	}
	if !isTaskVersionConflict(err) {
		t.Fatalf("A write error = %+v, want %s", err, taskVersionConflictCode)
	}

	// 🔴 W1 的关键断言：冲突后 A 的读路径必须返回 B 的值，而不是回滚后的旧快照。
	got := a.GetTaskInternal("t1")
	if got == nil {
		t.Fatalf("A must still see the task after conflict")
	}
	if got.Title != "written-by-B" {
		t.Fatalf("A read stale data after conflict: title=%q want %q (W1 refresh not working)",
			got.Title, "written-by-B")
	}
	if got.Version < 2 {
		t.Fatalf("A's in-memory version must advance to remote's, got %d", got.Version)
	}

	// 刷新后 A 不再被永久锁死，可正常写入。
	if err := writeTitle(a, "t1", "written-by-A-after-refresh"); err != nil {
		t.Fatalf("A must succeed after refresh, got %+v", err)
	}
}

// 反向护栏：非冲突类写失败（Mongo 真不可用）**不得**触发回源——
// 此时读库同样无意义，且会把「写失败」伪装成「读到新数据」。
func TestT31W1RefreshGuards(t *testing.T) {
	db := "trustmesh_t31_w1_guards"
	a := t31Store(t, db, nil)
	seedTask(t, a, "t1", "orig")

	// 集合为 nil（模拟 Mongo 掉线）→ 不刷新、不 panic。
	saved := a.mongoTasks
	a.mongoTasks = nil
	if a.refreshTaskFromMongoLocked("t1", 1) {
		t.Fatalf("refresh must be a no-op when the collection is nil")
	}
	a.mongoTasks = saved

	// 文档不存在 → 报告未刷新，且内存原值不受影响。
	if a.refreshTaskFromMongoLocked("no-such-task", 0) {
		t.Fatalf("refresh of a missing doc must report false")
	}
	if got := a.GetTaskInternal("t1"); got == nil || got.Title != "orig" {
		t.Fatalf("original in-memory copy must survive a failed refresh, got %+v", got)
	}

	// 远端版本不比本地新 → 刷新无意义，必须返回 false 且不动内存。
	if a.refreshTaskFromMongoLocked("t1", 99) {
		t.Fatalf("refresh must skip when remote version is not newer")
	}
	if got := a.GetTaskInternal("t1"); got.Title != "orig" {
		t.Fatalf("memory must be untouched by a skipped refresh, got %q", got.Title)
	}
}

// ─── W2：SSE 跨实例广播 ───

func armBroadcast(instanceID string) func(*Store) {
	return func(s *Store) {
		s.sseBroadcastEnabled = true
		s.sseInstanceID = instanceID
		s.outboxCh = make(chan sseUserEventDoc, sseOutboxCapacity)
	}
}

// 核心回归：A 发布的事件必须到达连在 B 上的订阅者；且不得回灌给 origin 造成重复。
func TestT31W2EventReachesOtherInstanceSubscriber(t *testing.T) {
	db := "trustmesh_t31_w2_broadcast"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := t31Store(t, db, armBroadcast("instance-A"))
	b := t31Store(t, db, armBroadcast("instance-B"))
	go a.StartSSEBroadcast(ctx)
	go b.StartSSEBroadcast(ctx)
	// 等两个 tailer 完成游标对齐（首轮 awaitSSECursor + 一个 tick）。
	time.Sleep(1500 * time.Millisecond)

	chB, unsubB := b.SubscribeUser("u-target")
	defer unsubB()

	a.publishUserEventUnsafe("u-target", "task.updated", map[string]any{"task_id": "t-9"}, time.Now().UTC())

	select {
	case got := <-chB:
		if got.Type != "task.updated" {
			t.Fatalf("event type = %q, want task.updated", got.Type)
		}
		if got.Payload["task_id"] != "t-9" {
			t.Fatalf("payload lost across instances: %+v", got.Payload)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("B's subscriber never received A's broadcast event")
	}

	// origin 侧：本地直投恰好一次，广播回灌必须被 origin 跳过。
	chA, unsubA := a.SubscribeUser("u-target")
	defer unsubA()
	a.publishUserEventUnsafe("u-target", "notification.created", map[string]any{"n": float64(1)}, time.Now().UTC())

	select {
	case got := <-chA:
		if got.Type != "notification.created" {
			t.Fatalf("local direct delivery type = %q", got.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("local direct delivery must still work when broadcast is on")
	}
	select {
	case dup := <-chA:
		t.Fatalf("broadcast must not be echoed back to the origin instance: %+v", dup)
	case <-time.After(4 * time.Second):
	}
}

// outbox 落库必须带 origin 与自增 seq（tailer 的排序与去重都依赖它们）。
func TestT31W2OutboxPersistsOriginAndSeq(t *testing.T) {
	db := "trustmesh_t31_w2_outbox"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := t31Store(t, db, armBroadcast("instance-A"))
	go a.StartSSEBroadcast(ctx)
	time.Sleep(1 * time.Second)

	a.publishUserEventUnsafe("u-1", "task.updated", map[string]any{"k": "v"}, time.Now().UTC())
	time.Sleep(2 * time.Second)

	queryCtx, qCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer qCancel()
	cur, err := a.mongoUserEvents.Find(queryCtx, bson.M{"user_id": "u-1"})
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	defer func() { _ = cur.Close(queryCtx) }()

	var docs []sseUserEventDoc
	for cur.Next(queryCtx) {
		var d sseUserEventDoc
		if err := cur.Decode(&d); err != nil {
			t.Fatalf("decode outbox doc: %v", err)
		}
		docs = append(docs, d)
	}
	if len(docs) != 1 {
		t.Fatalf("outbox docs = %d, want 1", len(docs))
	}
	if docs[0].OriginID != "instance-A" {
		t.Fatalf("origin_id = %q, want instance-A", docs[0].OriginID)
	}
	if docs[0].Seq <= 0 {
		t.Fatalf("seq must be a positive $inc value, got %d", docs[0].Seq)
	}
	if docs[0].EventJSON == "" {
		t.Fatalf("event_json must be persisted")
	}
	if docs[0].CreatedAt.IsZero() {
		t.Fatalf("created_at must be set (TTL sweep depends on it)")
	}
}

// 默认关闭 → 绝不写 outbox（「关闭 = 行为与改造前逐字节一致」的落库侧护栏）。
func TestT31W2DisabledWritesNoOutbox(t *testing.T) {
	db := "trustmesh_t31_w2_disabled"
	a := t31Store(t, db, nil)

	if a.SSEBroadcastArmed() {
		t.Fatalf("broadcast must not be armed by default")
	}
	a.publishUserEventUnsafe("u-1", "task.updated", map[string]any{"k": "v"}, time.Now().UTC())
	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n, err := a.mongoUserEvents.CountDocuments(ctx, bson.M{"user_id": "u-1"})
	if err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if n != 0 {
		t.Fatalf("disabled broadcast must write nothing, wrote %d docs", n)
	}
}

// outbox 的 TTL 索引确实建成（否则集合会无限增长）。
func TestT31W2OutboxHasTTLSweepIndex(t *testing.T) {
	db := "trustmesh_t31_w2_ttl"
	a := t31Store(t, db, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cur, err := a.mongoUserEvents.Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	found := false
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if _, ok := idx["expireAfterSeconds"]; ok {
			found = true
		}
	}
	if err := cur.Err(); err != nil {
		t.Fatalf("iterate indexes: %v", err)
	}
	if !found {
		t.Fatalf("user_events must carry a TTL index to bound collection growth")
	}
}

// ─── 第 1 步 leader 选举：真实 Mongo CAS（fake 覆盖不到的 E11000 分支）───

func TestT31LeaderRealMongoCAS(t *testing.T) {
	db := "trustmesh_t31_leader"
	arm := func(id string) func(*Store) {
		return func(s *Store) {
			s.leaderElectionEnabled = true
			s.leaderCAS = mongoLeaderCAS{col: s.mongoLeaderLeases}
			s.leaderID = id
		}
	}
	a := t31Store(t, db, arm("leader-A"))
	b := t31Store(t, db, arm("leader-B"))

	now := time.Now().UTC()
	a.renewLeadershipOnce(now)
	b.renewLeadershipOnce(now)

	if !a.isLeader.Load() {
		t.Fatalf("A must acquire the lease against real Mongo")
	}
	if b.isLeader.Load() {
		t.Fatalf("B must be blocked by the real duplicate-key path (no double dispatch)")
	}
	if !a.isLeaderForBackground() {
		t.Fatalf("A must run the background tickers")
	}
	if b.isLeaderForBackground() {
		t.Fatalf("B must skip its background tickers")
	}

	// A 释放 → B 立即接管（无需等 TTL）。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.leaderCAS.release(ctx, leaderLeaseKey, "leader-A"); err != nil {
		t.Fatalf("release: %v", err)
	}
	b.renewLeadershipOnce(now.Add(time.Second))
	if !b.isLeader.Load() {
		t.Fatalf("B must take over after A released the lease")
	}
}
