package store

import (
	"net/http"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/model"
)

// T0.0 测试前置：会议/在途任务重启持久化 —— P0⑤ 的核心验证。
//
// 说明：越权（fail-closed）测试**已存在**且相当扎实
// （TestMeetingScopedVisibility, store_org_test.go:1116），本文件不重复覆盖，
// 只补此前为零测试的持久化 / rehydrate 路径。
//
// mongodb 依赖：s.mongoMeetings 是具体类型 *mongo.Collection（store.go:137），
// 没有接口可注入 fake，因此真实 rehydrate 断言需要连通 MongoDB，由
// TRUSTMESH_TEST_MONGO_URI 门控；未设置时自动跳过，不影响 CI。

// TestMeetingRehydrateAfterRestart 模拟「容器重启」：内存清空后，
// GetMeeting 必须能从 Mongo 懒加载回灌（meeting.go:66-81），字段保持一致，
// 且回灌后归属裁决仍然 fail-closed。
func TestMeetingRehydrateAfterRestart(t *testing.T) {
	uri := os.Getenv("TRUSTMESH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("skip: set TRUSTMESH_TEST_MONGO_URI to a reachable MongoDB to enable this rehydrate test")
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

	created, appErr := s.CreateMeeting(Scope{UserID: "u1"}, &model.Meeting{
		Title:  "重启前的会议",
		Agenda: "议程 A",
	})
	if appErr != nil {
		t.Fatalf("create meeting: %v", appErr)
	}

	// 模拟容器重启：会议内存态全部丢失
	s.mu.Lock()
	s.meetings = make(map[string]*model.Meeting)
	s.projectMeetings = make(map[string][]string)
	s.mu.Unlock()

	// 重启后必须能读回（rehydrate）
	got, appErr := s.GetMeeting(Scope{UserID: "u1"}, created.ID)
	if appErr != nil {
		t.Fatalf("get meeting after simulated restart: %v", appErr)
	}
	if got == nil {
		t.Fatal("get meeting after restart returned nil meeting")
	}
	if got.ID != created.ID || got.Title != created.Title || got.Agenda != created.Agenda {
		t.Fatalf("rehydrated meeting mismatch:\n got = %+v\nwant = %+v", got, created)
	}
	if got.CreatorID != "u1" {
		t.Fatalf("rehydrated meeting CreatorID = %q, want u1", got.CreatorID)
	}

	// rehydrate 之后归属裁决必须仍然 fail-closed：别的租户不能读到它
	orgB, appErr := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create orgB: %v", appErr)
	}
	if _, err := s.GetMeeting(Scope{UserID: "u9", OrgID: orgB.ID}, created.ID); err == nil {
		t.Fatal("rehydrated meeting must stay invisible to another tenant")
	}
}

// TestMeetingMissingFailClosedNoMongo 守住一条安全不变量：
// 无 Mongo 且会议不在内存时，必须返回 404（fail-closed），绝不可回空列表
// 或放行 —— 这正是 store/meeting.go:281 注释记载的旧 fail-open 缺陷。
func TestMeetingMissingFailClosedNoMongo(t *testing.T) {
	s := New() // mongoEnabled 默认为 false

	if _, err := s.GetMeeting(Scope{UserID: "u1"}, "meeting-does-not-exist"); err == nil {
		t.Fatal("GetMeeting on unknown id must fail closed with 404")
	} else if err.Status != http.StatusNotFound {
		t.Fatalf("GetMeeting status = %d, want 404", err.Status)
	}

	if _, err := s.ListMeetingMessages(Scope{UserID: "u1"}, "meeting-does-not-exist"); err == nil {
		t.Fatal("ListMeetingMessages on unknown id must fail closed with 404")
	} else if err.Status != http.StatusNotFound {
		t.Fatalf("ListMeetingMessages status = %d, want 404", err.Status)
	}

	// 零值 Scope 也不得旁路（SystemScope 才放行）
	if _, err := s.GetMeeting(Scope{}, "meeting-does-not-exist"); err == nil {
		t.Fatal("zero-value Scope must not bypass meeting ownership")
	}
}

// TestMeetingVisibilityAfterSimulatedRehydrate 在不依赖 Mongo 的前提下，
// 直接把会议「放回」内存（等价于 Mongo 懒加载回灌的结果），验证回灌之后
// 归属裁决依然生效：跨租户不可读写，且越权调用不改写任何状态。
func TestMeetingVisibilityAfterSimulatedRehydrate(t *testing.T) {
	s := New()
	orgA, appErr := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create orgA: %v", appErr)
	}
	orgB, appErr := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create orgB: %v", appErr)
	}

	// 模拟 rehydrate：会议由 Mongo 回灌进内存
	s.mu.Lock()
	s.meetings["m-rehydrated"] = &model.Meeting{
		ID:        "m-rehydrated",
		Title:     "被回灌的会议",
		OrgID:     orgA.ID,
		CreatorID: "u1",
		Status:    model.MeetingWaiting,
	}
	s.meetingMessageIndex["m-rehydrated"] = []string{"msg-1"}
	s.meetingMessages["msg-1"] = &model.MeetingMessage{
		ID: "msg-1", MeetingID: "m-rehydrated", OrgID: orgA.ID,
		SenderType: "user", SenderID: "u1", Content: "hi",
	}
	s.mu.Unlock()

	// 属主可见
	if _, err := s.GetMeeting(Scope{UserID: "u1"}, "m-rehydrated"); err != nil {
		t.Fatalf("owner must see rehydrated meeting: %v", err)
	}

	// 跨租户不可读
	if _, err := s.GetMeeting(Scope{UserID: "u9", OrgID: orgB.ID}, "m-rehydrated"); err == nil {
		t.Fatal("cross-org must not read rehydrated meeting")
	}
	if _, err := s.ListMeetingMessages(Scope{UserID: "u9", OrgID: orgB.ID}, "m-rehydrated"); err == nil {
		t.Fatal("cross-org must not read rehydrated meeting messages")
	}

	// 跨租户不可写，且不得改写状态
	if err := s.UpdateMeetingStatus(Scope{UserID: "u9", OrgID: orgB.ID}, "m-rehydrated", model.MeetingCompleted); err == nil {
		t.Fatal("cross-org must not update rehydrated meeting status")
	}
	if _, err := s.AddMeetingMessage(Scope{UserID: "u9", OrgID: orgB.ID}, &model.MeetingMessage{
		MeetingID: "m-rehydrated", SenderType: "user", SenderID: "u9", Content: "越权发言",
	}); err == nil {
		t.Fatal("cross-org must not post to rehydrated meeting")
	}

	s.mu.RLock()
	status := s.meetings["m-rehydrated"].Status
	msgCount := len(s.meetingMessageIndex["m-rehydrated"])
	s.mu.RUnlock()

	if status != model.MeetingWaiting {
		t.Fatalf("rehydrated meeting status mutated by cross-org call: %s", status)
	}
	if msgCount != 1 {
		t.Fatalf("cross-org message was appended, msgCount = %d", msgCount)
	}
}
