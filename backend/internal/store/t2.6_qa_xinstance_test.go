package store

// T2.6 QA 独立对抗探针（由 QA 编写，独立于工程师用例）。
//
//  1. 证伪/证实 descope 的承重主张——「跨实例提升去重已由 T2.x 乐观锁覆盖」：
//     两个 Store 共享同一 Mongo，对同一 pending todo 并发 RecordSequentialTodoDispatch，
//     期望恰好一个成功提升、另一个拿 TASK_VERSION_CONFLICT（或读非 pending → TODO_NOT_PENDING），
//     且库内 DispatchAttempts == 1（不双派）。
//  2. seen-but-missing 自愈：幂等键已记录但对应消息不存在时，AddMeetingMessage 仍应落库
//     （不丢消息）。
//  3. 致命化：idemCheckOrRecord 遇非 dup 的 Mongo 错误时，AddMeetingMessage 不静默放行。
//  4. TTL：idempotency_keys.expire_at 上确实建有 expireAfterSeconds=0 的 TTL 索引。

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"

	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

func qaSharedMongoStores(t *testing.T, n int) []*Store {
	t.Helper()
	uri := os.Getenv("TRUSTMESH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("skip: set TRUSTMESH_TEST_MONGO_URI")
	}
	db := os.Getenv("TRUSTMESH_TEST_MONGO_DB")
	if db == "" {
		db = "trustmesh_t26_qa"
	}
	mk := func() *Store {
		s := New()
		cfg := config.Config{MongoEnabled: true, MongoURI: uri, MongoDatabase: db, MongoTimeout: 10 * time.Second}
		if err := s.enableMongo(cfg, zap.NewNop()); err != nil {
			t.Skipf("skip: mongo unavailable: %v", err)
		}
		s.log = zap.NewNop()
		t.Cleanup(func() { _ = s.Close() })
		return s
	}
	out := make([]*Store, n)
	for i := range out {
		out[i] = mk()
	}
	return out
}

func qaSeedDispatchTask(s *Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects["p1"] = &model.Project{ID: "p1", UserID: "u1", Status: "active"}
	s.agents["dev"] = &model.Agent{ID: "dev", NodeID: "node-dev", Name: "开发"}
	s.tasks["t1"] = &model.TaskDetail{
		ID: "t1", UserID: "u1", ProjectID: "p1", Status: "in_progress", Version: 1,
		Todos: []model.Todo{{
			ID: "td1", Status: "pending",
			Assignee: model.TodoAssignee{AgentID: "dev", Name: "开发", NodeID: "node-dev"},
		}},
	}
}

// TestQACrossInstanceNoDoublePromoteOptimisticLock 独立证实「乐观锁覆盖跨实例提升去重」。
func TestQACrossInstanceNoDoublePromoteOptimisticLock(t *testing.T) {
	stores := qaSharedMongoStores(t, 2)
	s1, s2 := stores[0], stores[1]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s1.mongoTasks.DeleteMany(ctx, bson.M{}); err != nil {
		t.Fatalf("clean tasks: %v", err)
	}
	if _, err := s1.mongoAgents.DeleteMany(ctx, bson.M{}); err != nil {
		t.Fatalf("clean agents: %v", err)
	}

	qaSeedDispatchTask(s1)
	qaSeedDispatchTask(s2)

	// Mongo 权威库持有 version=1 的同一任务文档（模拟两实例启动时加载的同一版本）。
	s1.mu.RLock()
	taskDoc := copyTask(s1.tasks["t1"])
	s1.mu.RUnlock()
	if _, err := s1.mongoTasks.InsertOne(ctx, taskDoc); err != nil {
		t.Fatalf("seed task doc: %v", err)
	}

	var e1, e2 *transport.AppError
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, e1 = s1.RecordSequentialTodoDispatch("t1", "td1") }()
	go func() { defer wg.Done(); _, e2 = s2.RecordSequentialTodoDispatch("t1", "td1") }()
	wg.Wait()

	succ := 0
	if e1 == nil {
		succ++
	}
	if e2 == nil {
		succ++
	}
	if succ != 1 {
		t.Fatalf("expected exactly ONE promote, got succ=%d (e1=%+v, e2=%+v)", succ, e1, e2)
	}
	loser := e1
	if e1 == nil {
		loser = e2
	}
	if loser.Code != taskVersionConflictCode && loser.Code != "TODO_NOT_PENDING" {
		t.Fatalf("loser must be version-conflict or todo-not-pending, got code=%q msg=%q", loser.Code, loser.Message)
	}

	var m model.TaskDetail
	if err := s1.mongoTasks.FindOne(ctx, bson.M{"_id": "t1"}).Decode(&m); err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if len(m.Todos) != 1 {
		t.Fatalf("todos len=%d", len(m.Todos))
	}
	if m.Todos[0].Status != "in_progress" || m.Todos[0].DispatchAttempts != 1 {
		t.Fatalf("Mongo todo status=%q attempts=%d (want in_progress/1, no double dispatch)",
			m.Todos[0].Status, m.Todos[0].DispatchAttempts)
	}
	t.Logf("OBSERVED: exactly one promote; loser code=%s; Mongo status=%s attempts=%d version=%d",
		loser.Code, m.Todos[0].Status, m.Todos[0].DispatchAttempts, m.Version)
}

// TestQAAddMeetingMessageSeenButMissingStillWrites 验证 seen-but-missing 自愈不丢消息。
func TestQAAddMeetingMessageSeenButMissingStillWrites(t *testing.T) {
	s := newLiveMongoStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.mongoIdempotencyKeys.DeleteMany(ctx, bson.M{}); err != nil {
		t.Fatalf("clean idempotency_keys: %v", err)
	}
	created, cErr := s.CreateMeeting(SystemScope(), &model.Meeting{Title: "qa seen-but-missing"})
	if cErr != nil {
		t.Fatalf("create meeting: %v", cErr)
	}

	content := "只记录键不落消息"
	key := meetingMessageSoftKey(created.ID, "user", "u1", content)
	if seen, appErr := s.idemCheckOrRecord(key, 5*time.Minute); appErr != nil || seen {
		t.Fatalf("pre-record key: seen=%v err=%v (want false/nil)", seen, appErr)
	}

	// 键已记录但无对应消息 → AddMeetingMessage 必须仍落库（不丢消息）。
	msg, appErr := s.AddMeetingMessage(SystemScope(), &model.MeetingMessage{
		MeetingID: created.ID, SenderType: "user", SenderID: "u1", Content: content,
	})
	if appErr != nil {
		t.Fatalf("add after pre-recorded key: %v", appErr)
	}
	if msg == nil || msg.ID == "" {
		t.Fatal("message must still be written on seen-but-missing")
	}
	cnt, err := s.mongoMeetingMessages.CountDocuments(ctx, bson.M{"meeting_id": created.ID})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("meeting_messages count=%d want 1 (no message loss)", cnt)
	}
	t.Logf("OBSERVED: seen-but-missing persisted a real message id=%s", msg.ID)
}

// TestQAIdemErrorFatalizedNoSilentDuplicate 验证 idem 记录失败时 AddMeetingMessage 不静默放行。
func TestQAIdemErrorFatalizedNoSilentDuplicate(t *testing.T) {
	s := newLiveMongoStore(t)
	s.mu.Lock()
	s.meetings["m-fatal"] = &model.Meeting{ID: "m-fatal", ProjectID: "p-fatal", Status: model.MeetingInProgress, Version: meetingVersionFloor}
	s.projectMeetings["p-fatal"] = append(s.projectMeetings["p-fatal"], "m-fatal")
	s.mu.Unlock()

	// 关闭 Mongo 客户端 → 幂等键 InsertOne 是非 dup 错误（连接断开）。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.mongoClient.Disconnect(ctx); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	_, appErr := s.AddMeetingMessage(SystemScope(), &model.MeetingMessage{
		MeetingID: "m-fatal", SenderType: "user", SenderID: "u1", Content: "致命化探针",
	})
	if appErr == nil {
		t.Fatal("AddMeetingMessage must return an error when idem record fails (no silent pass)")
	}
	s.mu.RLock()
	n := len(s.meetingMessages)
	s.mu.RUnlock()
	if n != 0 {
		t.Fatalf("no message should be persisted on idem error, got %d", n)
	}
	t.Logf("OBSERVED: idem write error fatalized (code=%s status=%d), no message persisted", appErr.Code, appErr.Status)
}

// TestQAIdempotencyTTLIndexExists 验证 idempotency_keys.expire_at 上建有 TTL 索引。
func TestQAIdempotencyTTLIndexExists(t *testing.T) {
	s := newLiveMongoStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cur, err := s.mongoIdempotencyKeys.Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	found := false
	for cur.Next(ctx) {
		var idx struct {
			Key                bson.M `bson:"key"`
			ExpireAfterSeconds *int32 `bson:"expireAfterSeconds"`
		}
		if err := cur.Decode(&idx); err != nil {
			t.Fatalf("decode index: %v", err)
		}
		if _, ok := idx.Key["expire_at"]; ok && idx.ExpireAfterSeconds != nil && *idx.ExpireAfterSeconds == 0 {
			found = true
		}
	}
	if err := cur.Err(); err != nil {
		t.Fatalf("cursor: %v", err)
	}
	if !found {
		t.Fatal("idempotency_keys must have a TTL index on expire_at (expireAfterSeconds=0)")
	}
	t.Log("OBSERVED: idempotency_keys.expire_at TTL index present (expireAfterSeconds=0)")
}
