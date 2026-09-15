package store

// T2.6 去重 / 幂等键测试。
//
// 两类用例：
//  1. CI 门禁（纯内存 store，无 Mongo）：原语同键/异键/TTL 过期行为 + 会议消息去重 +
//     派发去重。不依赖 docker / 网络。
//  2. 活库门禁（TRUSTMESH_TEST_MONGO_URI 门控，未设置时 t.Skip）：跨实例共享 Mongo 的
//     两条硬指标——①同键两次 AddMeetingMessage 库内仅 1 条且第二次返回首条；
//     ②两个 store 共享同一 Mongo 并发同键派发，仅 1 次实际提升（=1 次派发）。

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
)

// ─── 1. CI 门禁：原语行为（纯内存 store） ───

func TestIdemCheckOrRecordSameKeyTwiceSeen(t *testing.T) {
	s := New()
	seen, err := s.idemCheckOrRecord("k1", 5*time.Minute)
	if err != nil {
		t.Fatalf("first call err: %v", err)
	}
	if seen {
		t.Fatal("first call must be unseen")
	}
	seen, err = s.idemCheckOrRecord("k1", 5*time.Minute)
	if err != nil {
		t.Fatalf("second call err: %v", err)
	}
	if !seen {
		t.Fatal("second call with same key must be seen=true")
	}
}

func TestIdemCheckOrRecordDifferentKeysBothUnseen(t *testing.T) {
	s := New()
	a, err := s.idemCheckOrRecord("a", 5*time.Minute)
	if err != nil || a {
		t.Fatalf("key a: seen=%v err=%v, want unseen/nil", a, err)
	}
	b, err := s.idemCheckOrRecord("b", 5*time.Minute)
	if err != nil || b {
		t.Fatalf("key b: seen=%v err=%v, want unseen/nil", b, err)
	}
}

func TestIdemCheckOrRecordTTLExpiryReentrant(t *testing.T) {
	s := New()
	// ttl=1ns → expireAt 已落在过去；首次写入但立即过期。
	seen, err := s.idemCheckOrRecord("exp", time.Nanosecond)
	if err != nil {
		t.Fatalf("first call err: %v", err)
	}
	if seen {
		t.Fatal("first call must be unseen")
	}
	// 让时钟越过记录的过期点，确保窗口外可重入。
	time.Sleep(2 * time.Millisecond)
	seen, err = s.idemCheckOrRecord("exp", time.Nanosecond)
	if err != nil {
		t.Fatalf("second call err: %v", err)
	}
	if seen {
		t.Fatal("expired key must be re-entrant (unseen=false)")
	}
}

// ─── 1. CI 门禁：会议消息去重（纯内存 store） ───

func TestAddMeetingMessageIdempotencySoftKeyInMemory(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	s.mu.Lock()
	s.meetings["m1"] = &model.Meeting{ID: "m1", ProjectID: "p1", Status: model.MeetingInProgress, Version: meetingVersionFloor}
	s.projectMeetings["p1"] = append(s.projectMeetings["p1"], "m1")
	s.mu.Unlock()

	first, appErr := s.AddMeetingMessage(SystemScope(), &model.MeetingMessage{
		MeetingID: "m1", SenderType: "user", SenderID: "u1", SenderName: "我", Content: "你好",
	})
	if appErr != nil {
		t.Fatalf("first add: %v", appErr)
	}

	// 同内容、不带头键（派生软键相同）→ 命中并返回首条，库内仅 1 条。
	dup, appErr := s.AddMeetingMessage(SystemScope(), &model.MeetingMessage{
		MeetingID: "m1", SenderType: "user", SenderID: "u1", SenderName: "我", Content: "你好",
	})
	if appErr != nil {
		t.Fatalf("second add: %v", appErr)
	}
	if dup.ID != first.ID {
		t.Fatalf("second must return first message, got %s want %s", dup.ID, first.ID)
	}
	if dup.IdempotencyKey != first.IdempotencyKey || dup.IdempotencyKey == "" {
		t.Fatalf("second must carry the same derived key, got %q want %q", dup.IdempotencyKey, first.IdempotencyKey)
	}

	s.mu.RLock()
	n := len(s.meetingMessages)
	s.mu.RUnlock()
	if n != 1 {
		t.Fatalf("meetingMessages must have exactly 1 entry, got %d", n)
	}
}

func TestAddMeetingMessageIdempotencyClientKeyInMemory(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	s.mu.Lock()
	s.meetings["m2"] = &model.Meeting{ID: "m2", ProjectID: "p2", Status: model.MeetingInProgress, Version: meetingVersionFloor}
	s.projectMeetings["p2"] = append(s.projectMeetings["p2"], "m2")
	s.mu.Unlock()

	first, appErr := s.AddMeetingMessage(SystemScope(), &model.MeetingMessage{
		MeetingID: "m2", SenderType: "user", SenderID: "u2", Content: "A", IdempotencyKey: "client-key-1",
	})
	if appErr != nil {
		t.Fatalf("first add: %v", appErr)
	}
	// 内容不同但同客户端键 → 仍命中并返回首条。
	second, appErr := s.AddMeetingMessage(SystemScope(), &model.MeetingMessage{
		MeetingID: "m2", SenderType: "user", SenderID: "u2", Content: "B", IdempotencyKey: "client-key-1",
	})
	if appErr != nil {
		t.Fatalf("second add: %v", appErr)
	}
	if second.ID != first.ID {
		t.Fatalf("client key must dedup across different content, got %s want %s", second.ID, first.ID)
	}

	s.mu.RLock()
	n := len(s.meetingMessages)
	s.mu.RUnlock()
	if n != 1 {
		t.Fatalf("meetingMessages must have exactly 1 entry, got %d", n)
	}
}

// ─── 1. CI 门禁：派发去重（纯内存 store） ───

func TestRecordSequentialTodoDispatchIdempotencyInMemory(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	s.mu.Lock()
	s.projects["p1"] = &model.Project{ID: "p1", UserID: "u1", Status: "active"}
	s.agents["dev"] = &model.Agent{ID: "dev", NodeID: "node-dev", Name: "开发"}
	s.tasks["t1"] = &model.TaskDetail{
		ID: "t1", UserID: "u1", ProjectID: "p1", Status: "in_progress", Version: 1,
		Todos: []model.Todo{{
			ID: "td1", Status: "pending",
			Assignee: model.TodoAssignee{AgentID: "dev", Name: "开发", NodeID: "node-dev"},
		}},
	}
	s.mu.Unlock()

	if _, appErr := s.RecordSequentialTodoDispatch("t1", "td1"); appErr != nil {
		t.Fatalf("first dispatch: %v", appErr)
	}
	// 同键第二次（同进程，缓存命中）→ 跳过提升，不重复落库。
	_, appErr := s.RecordSequentialTodoDispatch("t1", "td1")
	if appErr != nil {
		t.Fatalf("second dispatch: %v", appErr)
	}

	// 仅首次真正提升：todo 仍只被派发一次（DispatchAttempts 不被二次累加）。
	s.mu.RLock()
	attempts := s.tasks["t1"].Todos[0].DispatchAttempts
	s.mu.RUnlock()
	if attempts != 1 {
		t.Fatalf("DispatchAttempts must be 1 (one real dispatch), got %d", attempts)
	}
}

// ─── 2. 活库门禁（TRUSTMESH_TEST_MONGO_URI） ───

func TestAddMeetingMessageIdempotencyLiveMongo(t *testing.T) {
	s := newLiveMongoStore(t) // 门控：未设 URI 时 t.Skip；清 meetings/meeting_messages + 内存

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// newLiveMongoStore 不清 idempotency_keys，此处显式清，避免用例间串扰。
	if _, err := s.mongoIdempotencyKeys.DeleteMany(ctx, bson.M{}); err != nil {
		t.Fatalf("clean idempotency_keys: %v", err)
	}

	// 必须用 CreateMeeting 落库（带版本），否则后续 AddMeetingMessage 的会议版本化
	// 写会因 Mongo 无该会议文档而报 MEETING_VERSION_CONFLICT。
	created, cErr := s.CreateMeeting(SystemScope(), &model.Meeting{Title: "t2.6 live dedup"})
	if cErr != nil {
		t.Fatalf("create meeting: %v", cErr)
	}
	meetingID := created.ID

	first, appErr := s.AddMeetingMessage(SystemScope(), &model.MeetingMessage{
		MeetingID: meetingID, SenderType: "user", SenderID: "u1", Content: "同一条",
	})
	if appErr != nil {
		t.Fatalf("first add: %v", appErr)
	}

	// 同软键第二次 → 命中，返回首条，库内仅 1 条。
	second, appErr := s.AddMeetingMessage(SystemScope(), &model.MeetingMessage{
		MeetingID: meetingID, SenderType: "user", SenderID: "u1", Content: "同一条",
	})
	if appErr != nil {
		t.Fatalf("second add: %v", appErr)
	}
	if second.ID != first.ID {
		t.Fatalf("second must return first, got %s want %s", second.ID, first.ID)
	}

	cnt, err := s.mongoMeetingMessages.CountDocuments(ctx, bson.M{"meeting_id": meetingID})
	if err != nil {
		t.Fatalf("count meeting_messages: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("meeting_messages must have exactly 1 doc, got %d", cnt)
	}
}

func TestDispatchIdempotencyAcrossStoresSharedMongo(t *testing.T) {
	uri := os.Getenv("TRUSTMESH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("skip: set TRUSTMESH_TEST_MONGO_URI to enable cross-instance dispatch dedup test")
	}
	db := os.Getenv("TRUSTMESH_TEST_MONGO_DB")
	if db == "" {
		db = "trustmesh_t26_test"
	}

	mkStore := func() *Store {
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
		t.Cleanup(func() { _ = s.Close() })
		return s
	}
	s1 := mkStore()
	s2 := mkStore()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// 清幂等键集合与 tasks 集合，避免用例间串扰（两 store 共享同一 Mongo DB）。
	if _, err := s1.mongoIdempotencyKeys.DeleteMany(ctx, bson.M{}); err != nil {
		t.Fatalf("clean idempotency_keys: %v", err)
	}
	if _, err := s1.mongoTasks.DeleteMany(ctx, bson.M{}); err != nil {
		t.Fatalf("clean tasks: %v", err)
	}

	seedTask := func(s *Store) {
		s.mu.Lock()
		s.projects["p1"] = &model.Project{ID: "p1", UserID: "u1", Status: "active"}
		s.agents["dev"] = &model.Agent{ID: "dev", NodeID: "node-dev", Name: "开发"}
		s.tasks["t1"] = &model.TaskDetail{
			ID: "t1", UserID: "u1", ProjectID: "p1", Status: "in_progress", Version: 1,
			Todos: []model.Todo{{
				ID: "td1", Status: "pending",
				Assignee: model.TodoAssignee{AgentID: "dev", Name: "开发", NodeID: "node-dev"},
			}},
		}
		s.mu.Unlock()
	}
	seedTask(s1)
	seedTask(s2)

	// 两个 store 并发对同 (task,todo) 派发：唯一索引保证仅一次 InsertOne 成功，
	// 另一次 E11000 → seen → 跳过提升。
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = s1.RecordSequentialTodoDispatch("t1", "td1")
	}()
	go func() {
		defer wg.Done()
		_, _ = s2.RecordSequentialTodoDispatch("t1", "td1")
	}()
	wg.Wait()

	// 幂等键集合应仅有 1 条（一次 InsertOne 成功，另一次 E11000）。
	cnt, err := s1.mongoIdempotencyKeys.CountDocuments(ctx, bson.M{"_id": "dispatch|t1|td1|node-dev"})
	if err != nil {
		t.Fatalf("count idempotency_keys: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("idempotency_keys must have exactly 1 doc, got %d", cnt)
	}

	// 两实例中「仅一个」把 todo 提升为 in_progress（= 仅一次实际派发）。
	s1.mu.RLock()
	s1promoted := s1.tasks["t1"].Todos[0].Status == "in_progress"
	s1.mu.RUnlock()
	s2.mu.RLock()
	s2promoted := s2.tasks["t1"].Todos[0].Status == "in_progress"
	s2.mu.RUnlock()
	if s1promoted == s2promoted {
		t.Fatalf("exactly one instance must promote (one real dispatch), got s1=%v s2=%v", s1promoted, s2promoted)
	}
}
