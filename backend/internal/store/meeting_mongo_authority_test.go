package store

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/model"
)

// T2.1「会议域 Mongo 权威」测试。
//
// 两类用例：
//  1. 「Mongo 失败零副作用」—— 用 mongoTimeout=1ns 注入失败，拿到的是驱动真实返回的
//     错误；不需要 fake 接口、不需要真 Mongo 服务，也不为测试改任何生产代码。
//  2. 需要真 Mongo 的正向用例（重启零丢失 / 版本冲突 / 一致性校验 / 存量回填）——
//     沿用 meeting_rehydrate_test.go 的模式，由 TRUSTMESH_TEST_MONGO_URI 门控，
//     未设置时 t.Skip，不让 CI 因缺少 Mongo 变红。

// ─── 测试脚手架 ───

// newMongoFailingStore 构造一个「Mongo 恒失败」的 Store。
//
// 手法：mongo.Connect 是惰性的（不建连、不报错），把 mongoTimeout 设成 1ns 后，
// 任何 Mongo 操作都会立刻返回 context deadline exceeded。这样拿到的是驱动真实错误，
// 且完全不依赖 docker / 网络。
func newMongoFailingStore(t *testing.T) *Store {
	t.Helper()
	cli, err := mongo.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:1/"))
	if err != nil {
		t.Fatalf("mongo.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cli.Disconnect(context.Background()) })

	s := New()
	s.log = zap.NewNop()
	s.mongoEnabled = true
	s.mongoMeetings = cli.Database("trustmesh_t21_test").Collection("meetings")
	s.mongoMeetingMessages = cli.Database("trustmesh_t21_test").Collection("meeting_messages")
	s.mongoTimeout = time.Nanosecond
	return s
}

// seedMeetingForFailureTest 直接往内存摆一条会议（绕开 Mongo），
// 这样「失败零副作用」断言才有个干净的基线可比。
func seedMeetingForFailureTest(t *testing.T, s *Store) *model.Meeting {
	t.Helper()
	m := &model.Meeting{
		ID:        "m-fail",
		ProjectID: "p-fail",
		Title:     "失败注入用会议",
		CreatorID: "u1",
		Status:    model.MeetingInProgress,
		Version:   meetingVersionFloor,
		Participants: []model.MeetingParticipant{
			{AgentID: "ag-1", AgentName: "编剧", Status: "invited"},
		},
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meetings[m.ID] = m
	s.projectMeetings[m.ProjectID] = append(s.projectMeetings[m.ProjectID], m.ID)
	return m
}

// readMeeting 在 RLock 下取一份会议字段快照，避免断言本身产生数据竞争。
func readMeeting(t *testing.T, s *Store, id string) model.Meeting {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.meetings[id]
	if !ok || m == nil {
		t.Fatalf("meeting %q missing from memory", id)
	}
	return *m
}

// newLiveMongoStore 连接 TRUSTMESH_TEST_MONGO_URI 指向的 MongoDB（本地测试库），
// 并为每个用例清一次会议集合，保证用例之间互不干扰。
// 环境变量未设置 / Mongo 不可达时跳过。
func newLiveMongoStore(t *testing.T) *Store {
	t.Helper()
	uri := os.Getenv("TRUSTMESH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("skip: set TRUSTMESH_TEST_MONGO_URI to a reachable MongoDB to enable this test")
	}
	db := os.Getenv("TRUSTMESH_TEST_MONGO_DB")
	if db == "" {
		db = "trustmesh_t21_test"
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
	t.Cleanup(func() { _ = s.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.mongoMeetings.DeleteMany(ctx, bson.D{}); err != nil {
		t.Fatalf("clean meetings collection: %v", err)
	}
	if _, err := s.mongoMeetingMessages.DeleteMany(ctx, bson.D{}); err != nil {
		t.Fatalf("clean meeting_messages collection: %v", err)
	}
	// enableMongo 会先把库里所有会议载入内存，随后才轮到上面的清库；
	// 不清内存就会残留上一个用例写下的会议，让「只应有 1 条」的断言失真。
	s.mu.Lock()
	s.meetings = make(map[string]*model.Meeting)
	s.projectMeetings = make(map[string][]string)
	s.meetingMessages = make(map[string]*model.MeetingMessage)
	s.meetingMessageIndex = make(map[string][]string)
	s.mu.Unlock()
	return s
}

// bumpMeetingVersionInMongo 绕过内存、直接把库内 version 推到指定值，
// 模拟「另一个写方已经改过这条会议」——这是制造版本冲突的唯一真实手段。
func bumpMeetingVersionInMongo(t *testing.T, s *Store, meetingID string, to int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.mongoMeetings.UpdateOne(ctx, bson.M{"_id": meetingID}, bson.M{"$set": bson.M{"version": to}}); err != nil {
		t.Fatalf("bump meeting version to %d: %v", to, err)
	}
}

// ─── 1. Mongo 失败零副作用（每个写路径一条） ───

func TestCreateMeetingMongoFailureLeavesMemoryUntouched(t *testing.T) {
	s := newMongoFailingStore(t)

	input := &model.Meeting{Title: "写不进去的会议", Agenda: "议程", ProjectID: "p-new"}
	got, appErr := s.CreateMeeting(Scope{UserID: "u1"}, input)
	if appErr == nil {
		t.Fatal("CreateMeeting must fail when Mongo write fails")
	}
	if got != nil {
		t.Fatalf("CreateMeeting must not return a meeting on failure, got %+v", got)
	}

	s.mu.RLock()
	meetings := len(s.meetings)
	idx := len(s.projectMeetings["p-new"])
	_, present := s.meetings[input.ID]
	s.mu.RUnlock()

	if meetings != 0 || idx != 0 {
		t.Fatalf("memory mutated on failed create: len(meetings)=%d, len(projectMeetings[p-new])=%d", meetings, idx)
	}
	if present {
		t.Fatal("failed CreateMeeting must not insert into s.meetings")
	}
}

func TestUpdateMeetingStatusMongoFailureLeavesMemoryUntouched(t *testing.T) {
	s := newMongoFailingStore(t)
	seedMeetingForFailureTest(t, s)

	appErr := s.UpdateMeetingStatus(Scope{UserID: "u1"}, "m-fail", model.MeetingCompleted)
	if appErr == nil {
		t.Fatal("UpdateMeetingStatus must fail when Mongo write fails")
	}

	got := readMeeting(t, s, "m-fail")
	if got.Status != model.MeetingInProgress {
		t.Fatalf("status mutated on failed write: %s, want in_progress", got.Status)
	}
	if got.Version != meetingVersionFloor {
		t.Fatalf("version must not advance on failed write: %d, want %d", got.Version, meetingVersionFloor)
	}
}

func TestAddMeetingMessageMongoFailureLeavesMemoryUntouched(t *testing.T) {
	s := newMongoFailingStore(t)
	seedMeetingForFailureTest(t, s)

	_, appErr := s.AddMeetingMessage(Scope{UserID: "u1"}, &model.MeetingMessage{
		MeetingID:  "m-fail",
		SenderType: "agent",
		SenderID:   "ag-1",
		SenderName: "编剧",
		Phase:      "speak",
		Target:     "host",
		Content:    "我的观点",
	})
	if appErr == nil {
		t.Fatal("AddMeetingMessage must fail when Mongo write fails")
	}

	s.mu.RLock()
	msgs := len(s.meetingMessages)
	idx := len(s.meetingMessageIndex["m-fail"])
	s.mu.RUnlock()
	if msgs != 0 || idx != 0 {
		t.Fatalf("message maps mutated on failed write: len(messages)=%d, len(index[m-fail])=%d", msgs, idx)
	}

	got := readMeeting(t, s, "m-fail")
	if got.LastPhase != "" || got.LastTarget != "" || got.SpeakerTurns != 0 {
		t.Fatalf("session state mutated on failed write: phase=%q target=%q turns=%d", got.LastPhase, got.LastTarget, got.SpeakerTurns)
	}
	if !got.LastActivityAt.IsZero() {
		t.Fatalf("LastActivityAt mutated on failed write: %v", got.LastActivityAt)
	}
	if len(got.Participants) != 1 || got.Participants[0].Status != "invited" {
		t.Fatalf("participants mutated on failed write: %+v", got.Participants)
	}
	if got.Version != meetingVersionFloor {
		t.Fatalf("version must not advance on failed write: %d, want %d", got.Version, meetingVersionFloor)
	}
}

func TestUpdateMeetingSummaryMongoFailureLeavesMemoryUntouched(t *testing.T) {
	s := newMongoFailingStore(t)
	seedMeetingForFailureTest(t, s)

	appErr := s.UpdateMeetingSummary(Scope{UserID: "u1"}, "m-fail", "file-summary-1")
	if appErr == nil {
		t.Fatal("UpdateMeetingSummary must fail when Mongo write fails")
	}

	got := readMeeting(t, s, "m-fail")
	if got.SummaryFileID != "" {
		t.Fatalf("SummaryFileID mutated on failed write: %q", got.SummaryFileID)
	}
	if got.Version != meetingVersionFloor {
		t.Fatalf("version must not advance on failed write: %d, want %d", got.Version, meetingVersionFloor)
	}
}

func TestUpdateMeetingMinutesMongoFailureLeavesMemoryUntouched(t *testing.T) {
	s := newMongoFailingStore(t)
	seedMeetingForFailureTest(t, s)

	appErr := s.UpdateMeetingMinutes(Scope{UserID: "u1"}, "m-fail", "# 纪要", "file-minutes-1")
	if appErr == nil {
		t.Fatal("UpdateMeetingMinutes must fail when Mongo write fails")
	}

	got := readMeeting(t, s, "m-fail")
	if got.Minutes != "" || got.MinutesFileID != "" {
		t.Fatalf("minutes mutated on failed write: minutes=%q fileID=%q", got.Minutes, got.MinutesFileID)
	}
	if got.Version != meetingVersionFloor {
		t.Fatalf("version must not advance on failed write: %d, want %d", got.Version, meetingVersionFloor)
	}
}

func TestAddMeetingTodoMongoFailureLeavesMemoryUntouched(t *testing.T) {
	s := newMongoFailingStore(t)
	seedMeetingForFailureTest(t, s)

	got, appErr := s.AddMeetingTodo(Scope{UserID: "u1"}, "m-fail", model.MeetingTodoItem{Description: "整理纪要"})
	if appErr == nil {
		t.Fatal("AddMeetingTodo must fail when Mongo write fails")
	}
	if got != nil {
		t.Fatal("AddMeetingTodo must not return a meeting on failure")
	}

	m := readMeeting(t, s, "m-fail")
	if len(m.Todos) != 0 {
		t.Fatalf("todos mutated on failed write: %+v", m.Todos)
	}
	if m.Version != meetingVersionFloor {
		t.Fatalf("version must not advance on failed write: %d, want %d", m.Version, meetingVersionFloor)
	}
}

// ─── 2. 存量 version 归一化（纯函数，无需 Mongo） ───

func TestNormalizeMeetingVersionBackfillsLegacyZero(t *testing.T) {
	// 存量会议文档（T2.1 之前写入）没有 version 字段，解码后为 0。
	legacy := &model.Meeting{ID: "m-legacy"}
	normalizeMeetingVersion(legacy)
	if legacy.Version != meetingVersionFloor {
		t.Fatalf("legacy version = %d, want %d", legacy.Version, meetingVersionFloor)
	}

	// 幂等：已有版本不得被覆盖。
	legacy.Version = 7
	normalizeMeetingVersion(legacy)
	if legacy.Version != 7 {
		t.Fatalf("existing version overwritten: %d, want 7", legacy.Version)
	}

	// nil 安全（解码路径可能拿到 nil）。
	normalizeMeetingVersion(nil)
}

// ─── 3. 需要真 Mongo 的用例 ───

// TestMeetingVersionConflictOnStaleVersion 乐观锁：内存版本陈旧时，
// 更新必须返回 409 冲突（而不是静默覆盖别人的写入）。
func TestMeetingVersionConflictOnStaleVersion(t *testing.T) {
	s := newLiveMongoStore(t)

	created, appErr := s.CreateMeeting(Scope{UserID: "u1"}, &model.Meeting{Title: "乐观锁", Agenda: "议程"})
	if appErr != nil {
		t.Fatalf("create meeting: %v", appErr)
	}
	if created.Version != meetingVersionFloor {
		t.Fatalf("fresh meeting version = %d, want %d", created.Version, meetingVersionFloor)
	}

	// 另一个写方先把库内版本推到 2（内存仍是 1）。
	bumpMeetingVersionInMongo(t, s, created.ID, 2)

	appErr = s.UpdateMeetingStatus(Scope{UserID: "u1"}, created.ID, model.MeetingInProgress)
	if appErr == nil {
		t.Fatal("stale version update must return a conflict, not silently overwrite")
	}
	if appErr.Status != http.StatusConflict {
		t.Fatalf("conflict status = %d, want 409", appErr.Status)
	}
	if appErr.Code != "MEETING_VERSION_CONFLICT" {
		t.Fatalf("conflict code = %q, want MEETING_VERSION_CONFLICT", appErr.Code)
	}

	got := readMeeting(t, s, created.ID)
	if got.Status != model.MeetingWaiting {
		t.Fatalf("memory status mutated by conflicted write: %s", got.Status)
	}
	if got.Version != meetingVersionFloor {
		t.Fatalf("memory version advanced by conflicted write: %d, want %d", got.Version, meetingVersionFloor)
	}
}

// TestAddMeetingMessageSelfHealLeavesNoResidueInMongo 覆盖「会话态写冲突」的**自愈后**不变式。
//
// 背景：早先的断言是「会话态写冲突必须返错」（T2.1 初期形态）。P2 自愈修复
// （retryOnceAfterVersionRefreshLocked：刷新内存 version 后重试一次）上线后，该场景会
// **成功**而不是返错 —— 因为 $set 的会话态字段全部由当前这条消息派生并整体覆盖，
// 刷新 version 再写一次不会吞掉别的写方留下的语义（见 TestQAAddMeetingMessageSelfHealsOnVersionConflict）。
//
// 若沿用「必须返错」的旧断言，这条用例会稳定变红（且是断言过期、不是产品缺陷）——
// 正是这种「只有活库才跑得到」的用例最容易漏更新，故在此改成断言更本质的性质：
// 自愈之后**内存与 Mongo 无残差**，不会出现「消息已落库、会话态没跟上」的分叉。
func TestAddMeetingMessageSelfHealLeavesNoResidueInMongo(t *testing.T) {
	s := newLiveMongoStore(t)
	sc := Scope{UserID: "u1"}

	created, appErr := s.CreateMeeting(sc, &model.Meeting{Title: "自愈无残差"})
	if appErr != nil {
		t.Fatalf("create meeting: %v", appErr)
	}
	s.mu.Lock()
	created.Participants = []model.MeetingParticipant{{AgentID: "ag-1", AgentName: "编剧", Status: "invited"}}
	s.mu.Unlock()

	// 内存 version=1、库内 version=5 → 会话态首写冲突 → 触发自愈（刷新 version + 重试）。
	bumpMeetingVersionInMongo(t, s, created.ID, 5)

	if _, appErr := s.AddMeetingMessage(sc, &model.MeetingMessage{
		MeetingID:  created.ID,
		SenderType: "agent",
		SenderID:   "ag-1",
		SenderName: "编剧",
		Phase:      "speak",
		Target:     "host",
		Content:    "我的观点",
	}); appErr != nil {
		t.Fatalf("session-state conflict must self-heal via version refresh + retry, got %+v", appErr)
	}

	// 内存侧：消息入索引、会话态推进、版本追上并推进库内（5 → 6）。
	s.mu.RLock()
	inMem := len(s.meetingMessageIndex[created.ID])
	s.mu.RUnlock()
	if inMem != 1 {
		t.Fatalf("message index after self-heal = %d, want 1", inMem)
	}
	got := readMeeting(t, s, created.ID)
	if got.LastPhase != "speak" || got.LastTarget != "host" {
		t.Fatalf("session state after self-heal: phase=%q target=%q", got.LastPhase, got.LastTarget)
	}
	if got.Version != 6 {
		t.Fatalf("version after self-heal = %d, want 6", got.Version)
	}

	// Mongo 侧：会话态必须已被同一次提交写下去 —— 这才是「无残差」的判据。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var persisted model.Meeting
	if err := s.mongoMeetings.FindOne(ctx, bson.M{"_id": created.ID}).Decode(&persisted); err != nil {
		t.Fatalf("read meeting back from mongo: %v", err)
	}
	if persisted.LastPhase != "speak" || persisted.LastTarget != "host" {
		t.Fatalf("session state not persisted (residue): phase=%q target=%q", persisted.LastPhase, persisted.LastTarget)
	}
	if persisted.Version != 6 {
		t.Fatalf("mongo version after self-heal = %d, want 6", persisted.Version)
	}
}

// TestMeetingRestartZeroLossRehydrate T2.1 验收②：写入后模拟重启（清空内存 + 重新载入），
// 会议、消息、会话态（phase / target / turns / last_activity）全部可读回且能续开。
func TestMeetingRestartZeroLossRehydrate(t *testing.T) {
	s := newLiveMongoStore(t)

	created, appErr := s.CreateMeeting(Scope{UserID: "u1"}, &model.Meeting{
		Title: "重启零丢失", Agenda: "议程 A", ProjectID: "p-restart",
	})
	if appErr != nil {
		t.Fatalf("create meeting: %v", appErr)
	}
	if _, appErr := s.AddMeetingMessage(Scope{UserID: "u1"}, &model.MeetingMessage{
		MeetingID:  created.ID,
		SenderType: "agent",
		SenderID:   "host",
		SenderName: "PM",
		Phase:      "speak",
		Target:     "ag-1",
		Content:    "请发言",
	}); appErr != nil {
		t.Fatalf("add message: %v", appErr)
	}

	wantVersion := readMeeting(t, s, created.ID).Version
	if wantVersion != meetingVersionFloor+1 {
		t.Fatalf("version after one message = %d, want %d", wantVersion, meetingVersionFloor+1)
	}

	// 模拟容器重启：内存全丢。
	s.mu.Lock()
	s.meetings = make(map[string]*model.Meeting)
	s.projectMeetings = make(map[string][]string)
	s.meetingMessages = make(map[string]*model.MeetingMessage)
	s.meetingMessageIndex = make(map[string][]string)
	s.mu.Unlock()

	// 重新载入（等价进程重启后的 loadMongoState）。
	if err := s.loadMongoState(); err != nil {
		t.Fatalf("loadMongoState after simulated restart: %v", err)
	}

	got, appErr := s.GetMeeting(Scope{UserID: "u1"}, created.ID)
	if appErr != nil {
		t.Fatalf("get meeting after restart: %v", appErr)
	}
	if got.Title != "重启零丢失" || got.Agenda != "议程 A" || got.ProjectID != "p-restart" {
		t.Fatalf("rehydrated meeting mismatch: %+v", got)
	}
	if got.Version != wantVersion {
		t.Fatalf("rehydrated version = %d, want %d", got.Version, wantVersion)
	}
	if got.LastPhase != "speak" || got.LastTarget != "ag-1" || got.SpeakerTurns != 1 {
		t.Fatalf("rehydrated session state mismatch: phase=%q target=%q turns=%d", got.LastPhase, got.LastTarget, got.SpeakerTurns)
	}
	if got.LastActivityAt.IsZero() {
		t.Fatal("rehydrated LastActivityAt must be set")
	}

	msgs, appErr := s.ListMeetingMessages(Scope{UserID: "u1"}, created.ID)
	if appErr != nil {
		t.Fatalf("list messages after restart: %v", appErr)
	}
	if len(msgs) != 1 || msgs[0].Content != "请发言" {
		t.Fatalf("rehydrated messages = %+v, want 1 message 请发言", msgs)
	}

	// 可续开：重启后继续发言，会话态接着累加，版本继续推进。
	if _, appErr := s.AddMeetingMessage(Scope{UserID: "u1"}, &model.MeetingMessage{
		MeetingID:  created.ID,
		SenderType: "agent",
		SenderID:   "ag-1",
		SenderName: "编剧",
		Phase:      "speak",
		Target:     "host",
		Content:    "我的观点",
	}); appErr != nil {
		t.Fatalf("continue meeting after restart: %v", appErr)
	}
	after := readMeeting(t, s, created.ID)
	if after.SpeakerTurns != 2 || after.LastTarget != "host" {
		t.Fatalf("meeting not resumable after restart: turns=%d target=%q", after.SpeakerTurns, after.LastTarget)
	}
	if after.Version != wantVersion+1 {
		t.Fatalf("version after resumed message = %d, want %d", after.Version, wantVersion+1)
	}
}

// TestVerifyMeetingConsistencyNoMismatch 双写校验：正常写路径后 mismatched 必须为 0；
// 人为在库里制造分歧后必须能被检出（mismatched > 0）。
func TestVerifyMeetingConsistencyNoMismatch(t *testing.T) {
	s := newLiveMongoStore(t)

	created, appErr := s.CreateMeeting(Scope{UserID: "u1"}, &model.Meeting{Title: "一致性校验"})
	if appErr != nil {
		t.Fatalf("create meeting: %v", appErr)
	}
	if appErr := s.UpdateMeetingStatus(Scope{UserID: "u1"}, created.ID, model.MeetingInProgress); appErr != nil {
		t.Fatalf("update status: %v", appErr)
	}
	if _, appErr := s.AddMeetingMessage(Scope{UserID: "u1"}, &model.MeetingMessage{
		MeetingID: created.ID, SenderType: "agent", SenderID: "ag-1", Phase: "speak", Target: "host", Content: "观点",
	}); appErr != nil {
		t.Fatalf("add message: %v", appErr)
	}
	if appErr := s.UpdateMeetingSummary(Scope{UserID: "u1"}, created.ID, "file-summary"); appErr != nil {
		t.Fatalf("update summary: %v", appErr)
	}
	if appErr := s.UpdateMeetingMinutes(Scope{UserID: "u1"}, created.ID, "# 纪要", "file-minutes"); appErr != nil {
		t.Fatalf("update minutes: %v", appErr)
	}
	if _, appErr := s.AddMeetingTodo(Scope{UserID: "u1"}, created.ID, model.MeetingTodoItem{Description: "跟进"}); appErr != nil {
		t.Fatalf("add todo: %v", appErr)
	}

	checked, mismatched, err := s.VerifyMeetingConsistency()
	if err != nil {
		t.Fatalf("VerifyMeetingConsistency: %v", err)
	}
	if checked != 1 {
		t.Fatalf("checked = %d, want 1", checked)
	}
	if mismatched != 0 {
		t.Fatalf("mismatched = %d, want 0 (memory and Mongo must agree after every write)", mismatched)
	}

	// 人为制造分歧：绕过内存直接改库里的 status，校验必须能抓出来。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.mongoMeetings.UpdateOne(ctx, bson.M{"_id": created.ID}, bson.M{"$set": bson.M{"status": string(model.MeetingCompleted)}}); err != nil {
		t.Fatalf("tamper mongo status: %v", err)
	}
	if _, mismatched, err = s.VerifyMeetingConsistency(); err != nil {
		t.Fatalf("VerifyMeetingConsistency after tamper: %v", err)
	}
	if mismatched != 1 {
		t.Fatalf("mismatched after tamper = %d, want 1", mismatched)
	}
}

// TestBackfillMeetingVersionForLegacyDocuments 存量兼容：T2.1 之前写入的会议文档
// 没有 version 字段，启动回填必须补成 1（库内 + 内存），否则带 {_id, version}
// filter 的更新永远匹配不上 → 生产全量写失败。
func TestBackfillMeetingVersionForLegacyDocuments(t *testing.T) {
	s := newLiveMongoStore(t)

	legacyID := "m-legacy-no-version"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.mongoMeetings.InsertOne(ctx, bson.D{
		{Key: "_id", Value: legacyID},
		{Key: "title", Value: "存量会议"},
		{Key: "project_id", Value: "p-legacy"},
		{Key: "creator_id", Value: "u1"},
		{Key: "status", Value: string(model.MeetingWaiting)},
		{Key: "created_at", Value: time.Now().UTC()},
		{Key: "updated_at", Value: time.Now().UTC()},
	}); err != nil {
		t.Fatalf("insert legacy meeting doc: %v", err)
	}

	s.backfillMeetingVersions()

	// 库内必须已经是 1。
	var doc struct {
		Version int `bson:"version"`
	}
	if err := s.mongoMeetings.FindOne(ctx, bson.M{"_id": legacyID}).Decode(&doc); err != nil {
		t.Fatalf("read back legacy doc: %v", err)
	}
	if doc.Version != meetingVersionFloor {
		t.Fatalf("mongo version after backfill = %d, want %d", doc.Version, meetingVersionFloor)
	}

	// 载入路径（重启）后内存也必须是 1。
	items, _, err := s.loadMeetings()
	if err != nil {
		t.Fatalf("loadMeetings: %v", err)
	}
	legacy := items[legacyID]
	if legacy == nil {
		t.Fatal("legacy meeting missing from loadMeetings")
	}
	if legacy.Version != meetingVersionFloor {
		t.Fatalf("in-memory version after load = %d, want %d", legacy.Version, meetingVersionFloor)
	}

	// 最关键：回填之后这条存量会议必须**仍然可写**（不会因 filter 失配而永久冲突）。
	s.mu.Lock()
	s.meetings[legacyID] = legacy
	s.projectMeetings[legacy.ProjectID] = append(s.projectMeetings[legacy.ProjectID], legacyID)
	s.mu.Unlock()

	if appErr := s.UpdateMeetingStatus(Scope{UserID: "u1"}, legacyID, model.MeetingInProgress); appErr != nil {
		t.Fatalf("legacy meeting must remain writable after version backfill: %+v", appErr)
	}
	if got := readMeeting(t, s, legacyID); got.Status != model.MeetingInProgress || got.Version != meetingVersionFloor+1 {
		t.Fatalf("legacy meeting after update: status=%s version=%d", got.Status, got.Version)
	}
}

// TestDeriveMeetingSessionDoesNotMutateInput 守住 P2-4 的回归盲区：deriveMeetingSession
// 必须是纯函数 —— 输入会议的 Participants 不被就地改写，派生结果只在返回值上。
// 这条不依赖真 Mongo，CI 恒跑；QA 反向验证曾证明：若改成就地改 meeting.Participants，
// 无 Mongo 的「失败零副作用」用例会 PASS（因为 1ns 超时让消息插库那步就 return），
// 从而让浅拷贝回归逃过 CI —— 本用例直接打在纯函数上，能独立抓住它。
func TestDeriveMeetingSessionDoesNotMutateInput(t *testing.T) {
	base := model.Meeting{
		ID:           "m-derive",
		Title:        "派生语义",
		Status:       model.MeetingWaiting,
		SpeakerTurns: 1,
		Participants: []model.MeetingParticipant{
			{AgentID: "ag-1", AgentName: "编剧", Status: "invited"},
			{AgentID: "host", AgentName: "PM", Status: "joined"},
		},
	}
	// 备份输入切片与其中一个元素，用于后置比对。
	baseJoined0 := base.Participants[0].Status
	baseSliceData0 := &base.Participants[0] // 指向底层数组元素，验证浅拷是否改到底层

	msg := &model.MeetingMessage{
		MeetingID:  "m-derive",
		SenderType: "agent",
		SenderID:   "ag-1",
		SenderName: "编剧",
		Phase:      "speak",
		Target:     "host",
		Content:    "我的观点",
		CreatedAt:  time.Now().UTC(),
	}

	out := deriveMeetingSession(base, msg)

	// ① 输入对象的 Participants[0] 仍应是 invited（未被就地改）。
	if base.Participants[0].Status != baseJoined0 {
		t.Fatalf("deriveMeetingSession mutated input: Participants[0].Status = %q, want %q", base.Participants[0].Status, baseJoined0)
	}
	// ② 指向底层的指针也未变，证明不是共享底层数组的浅拷。
	if baseSliceData0.Status != baseJoined0 {
		t.Fatalf("deriveMeetingSession shared backing array with input: %q", baseSliceData0.Status)
	}
	// ③ 返回值上 ag-1 应被标记 joined（派生生效）。
	if out.Participants[0].Status != "joined" {
		t.Fatalf("derived Participants[0].Status = %q, want joined", out.Participants[0].Status)
	}
	// ④ 返回值与输入是不同切片头（深拷）。
	if &out.Participants[0] == &base.Participants[0] {
		t.Fatal("deriveMeetingSession returned the same underlying slice element as input (shallow copy)")
	}
	// ⑤ 派生字段正确。
	if out.LastPhase != "speak" || out.LastTarget != "host" || out.SpeakerTurns != 2 {
		t.Fatalf("derived session fields wrong: phase=%q target=%q turns=%d", out.LastPhase, out.LastTarget, out.SpeakerTurns)
	}
	// ⑥ 输入的其他派生字段也不应被改。
	if base.LastPhase != "" || base.SpeakerTurns != 1 {
		t.Fatalf("deriveMeetingSession mutated input derived fields: lastPhase=%q turns=%d", base.LastPhase, base.SpeakerTurns)
	}
}
