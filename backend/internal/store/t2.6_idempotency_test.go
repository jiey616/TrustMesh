package store

// T2.6 去重 / 幂等键测试（as-built：范围收敛为「会议消息幂等」，派发不纳入 —— descoped）。
//
// 两类用例：
//  1. CI 门禁（纯内存 store，无 Mongo）：原语同键/异键/TTL 过期行为 + 会议消息去重
//     （客户端键与派生软键）。不依赖 docker / 网络。
//  2. 活库门禁（TRUSTMESH_TEST_MONGO_URI 门控，未设置时 t.Skip）：同键两次
//     AddMeetingMessage 库内仅 1 条且第二次返回首条（跨实例/重启后的权威去重）。

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"

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
