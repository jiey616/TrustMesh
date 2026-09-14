package store

// T2.1 独立 QA 验证用例（第 1 轮）。
//
// 与 meeting_mongo_authority_test.go 的分工：那边覆盖「零副作用 / 冲突 / 重启零丢失 /
// 一致性校验 / 存量回填」的主干断言；这里补的是边界与错误路径：
//   - 纯内存模式（mongoEnabled=false）下 6 条写路径是否仍可用；
//   - 版本号是否严格单调、内存与库内是否始终一致；
//   - 单进程并发写是否既不丢更新也不误报冲突（并供 -race 使用）；
//   - 三条懒加载读路径是否都做了 version 归一化；
//   - VerifyMeetingConsistency 是否真的只读；
//   - AddMeetingTodo 的切片是否有底层数组污染 / 别名问题；
//   - 两个「残差形态」探针：显式 version=0 的存量文档、以及会话态冲突后的自愈能力。
//
// 需要真 Mongo 的用例同样由 TRUSTMESH_TEST_MONGO_URI 门控。

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// ─── 测试脚手架 ───

type qaMeetingDoc struct {
	Version      int                     `bson:"version"`
	Status       string                  `bson:"status"`
	LastPhase    string                  `bson:"last_phase"`
	SpeakerTurns int                     `bson:"speaker_turns"`
	Todos        []model.MeetingTodoItem `bson:"todos"`
}

// qaReadDoc 直接读库内会议文档（绕过内存），用于校验「内存与库内是否同步」。
func qaReadDoc(t *testing.T, s *Store, id string) qaMeetingDoc {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var doc qaMeetingDoc
	if err := s.mongoMeetings.FindOne(ctx, bson.M{"_id": id}).Decode(&doc); err != nil {
		t.Fatalf("read meeting doc %q: %v", id, err)
	}
	return doc
}

// qaCountMessages 直接数库内某会议的消息条数（绕过内存）。
func qaCountMessages(t *testing.T, s *Store, meetingID string) int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n, err := s.mongoMeetingMessages.CountDocuments(ctx, bson.M{"meeting_id": meetingID})
	if err != nil {
		t.Fatalf("count messages for %q: %v", meetingID, err)
	}
	return n
}

// qaForgetMeeting 把某会议从内存里抹掉，逼迫读路径走 Mongo 懒加载。
func qaForgetMeeting(t *testing.T, s *Store, id string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.meetings, id)
}

// ─── 1. 纯内存模式：6 条写路径不能被版本逻辑搞坏 ───

func TestQAPureMemoryModeSixWritePathsRemainUsable(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	// New() 默认 mongoEnabled=false —— 正是「纯内存模式」。
	if s.mongoEnabled {
		t.Fatal("fixture error: expected a pure in-memory store")
	}

	sc := Scope{UserID: "u1"}

	created, appErr := s.CreateMeeting(sc, &model.Meeting{Title: "纯内存会议", ProjectID: "p-mem"})
	if appErr != nil {
		t.Fatalf("CreateMeeting in memory-only mode: %+v", appErr)
	}
	if created.Version != meetingVersionFloor {
		t.Fatalf("created version = %d, want %d", created.Version, meetingVersionFloor)
	}
	id := created.ID

	if appErr := s.UpdateMeetingStatus(sc, id, model.MeetingInProgress); appErr != nil {
		t.Fatalf("UpdateMeetingStatus in memory-only mode: %+v", appErr)
	}
	if got := readMeeting(t, s, id); got.Status != model.MeetingInProgress || got.Version != 2 {
		t.Fatalf("after status update: status=%s version=%d", got.Status, got.Version)
	}

	if _, appErr := s.AddMeetingMessage(sc, &model.MeetingMessage{
		MeetingID: id, SenderType: "agent", SenderID: "ag-1", SenderName: "编剧",
		Phase: "speak", Target: "host", Content: "观点",
	}); appErr != nil {
		t.Fatalf("AddMeetingMessage in memory-only mode: %+v", appErr)
	}
	if got := readMeeting(t, s, id); got.LastPhase != "speak" || got.SpeakerTurns != 1 || got.Version != 3 {
		t.Fatalf("after message: phase=%q turns=%d version=%d", got.LastPhase, got.SpeakerTurns, got.Version)
	}

	if appErr := s.UpdateMeetingSummary(sc, id, "file-summary"); appErr != nil {
		t.Fatalf("UpdateMeetingSummary in memory-only mode: %+v", appErr)
	}
	if got := readMeeting(t, s, id); got.SummaryFileID != "file-summary" || got.Version != 4 {
		t.Fatalf("after summary: fileID=%q version=%d", got.SummaryFileID, got.Version)
	}

	if appErr := s.UpdateMeetingMinutes(sc, id, "# 纪要", "file-minutes"); appErr != nil {
		t.Fatalf("UpdateMeetingMinutes in memory-only mode: %+v", appErr)
	}
	if got := readMeeting(t, s, id); got.Minutes != "# 纪要" || got.MinutesFileID != "file-minutes" || got.Version != 5 {
		t.Fatalf("after minutes: minutes=%q fileID=%q version=%d", got.Minutes, got.MinutesFileID, got.Version)
	}

	if _, appErr := s.AddMeetingTodo(sc, id, model.MeetingTodoItem{Description: "跟进"}); appErr != nil {
		t.Fatalf("AddMeetingTodo in memory-only mode: %+v", appErr)
	}
	got := readMeeting(t, s, id)
	if len(got.Todos) != 1 || got.Version != 6 {
		t.Fatalf("after todo: len(todos)=%d version=%d", len(got.Todos), got.Version)
	}

	// 读路径也要能看到这些改动（不能被版本逻辑挡住）。
	if m, appErr := s.GetMeeting(sc, id); appErr != nil || m.Status != model.MeetingInProgress {
		t.Fatalf("GetMeeting after writes: %+v / %+v", m, appErr)
	}
	if msgs, appErr := s.ListMeetingMessages(sc, id); appErr != nil || len(msgs) != 1 {
		t.Fatalf("ListMeetingMessages after writes: len=%d err=%+v", len(msgs), appErr)
	}
}

// ─── 2. 版本号严格单调（内存与库内同步推进） ───

func TestQAVersionStrictlyMonotonicAcrossSequentialWrites(t *testing.T) {
	s := newLiveMongoStore(t)
	sc := Scope{UserID: "u1"}

	created, appErr := s.CreateMeeting(sc, &model.Meeting{Title: "版本单调"})
	if appErr != nil {
		t.Fatalf("create meeting: %+v", appErr)
	}
	id := created.ID
	want := meetingVersionFloor

	// 10 轮混合写：每轮都必须恰好 +1，且库内与内存保持一致。
	for i := 0; i < 10; i++ {
		switch i % 4 {
		case 0:
			st := model.MeetingInProgress
			if i%8 == 4 {
				st = model.MeetingWaiting
			}
			if appErr := s.UpdateMeetingStatus(sc, id, st); appErr != nil {
				t.Fatalf("round %d UpdateMeetingStatus: %+v", i, appErr)
			}
		case 1:
			if _, appErr := s.AddMeetingMessage(sc, &model.MeetingMessage{
				MeetingID: id, SenderType: "agent", SenderID: "ag-1", Phase: "speak", Target: "host", Content: "观点",
			}); appErr != nil {
				t.Fatalf("round %d AddMeetingMessage: %+v", i, appErr)
			}
		case 2:
			if appErr := s.UpdateMeetingSummary(sc, id, "file-s"); appErr != nil {
				t.Fatalf("round %d UpdateMeetingSummary: %+v", i, appErr)
			}
		case 3:
			if _, appErr := s.AddMeetingTodo(sc, id, model.MeetingTodoItem{Description: "待办"}); appErr != nil {
				t.Fatalf("round %d AddMeetingTodo: %+v", i, appErr)
			}
		}
		want++

		mem := readMeeting(t, s, id)
		doc := qaReadDoc(t, s, id)
		if mem.Version != want {
			t.Fatalf("round %d: memory version = %d, want %d", i, mem.Version, want)
		}
		if doc.Version != want {
			t.Fatalf("round %d: mongo version = %d, want %d (memory %d)", i, doc.Version, want, mem.Version)
		}
	}

	// 会话态也必须同步到了库里（AddMeetingMessage 的 ② 真的落库了）。
	if doc := qaReadDoc(t, s, id); doc.LastPhase != "speak" || doc.SpeakerTurns != 3 {
		t.Fatalf("session state not persisted: phase=%q turns=%d", doc.LastPhase, doc.SpeakerTurns)
	}
}

// ─── 3. 单进程并发写：既不丢更新，也不误报冲突 ───

func TestQAConcurrentMeetingWritesNeitherLoseUpdatesNorFalseConflict(t *testing.T) {
	s := newLiveMongoStore(t)
	sc := Scope{UserID: "u1"}

	created, appErr := s.CreateMeeting(sc, &model.Meeting{Title: "并发写"})
	if appErr != nil {
		t.Fatalf("create meeting: %+v", appErr)
	}
	id := created.ID

	const n = 8
	var mu sync.Mutex
	ok, conflict, other := 0, 0, 0
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := s.AddMeetingTodo(sc, id, model.MeetingTodoItem{Description: "todo"})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case err.Status == http.StatusConflict:
				conflict++
			default:
				other++
			}
		}(i)
	}
	wg.Wait()

	if other != 0 {
		t.Fatalf("%d writes failed with a non-conflict error", other)
	}
	if ok+conflict != n {
		t.Fatalf("ok+conflict = %d, want %d", ok+conflict, n)
	}
	// s.mu 把写路径完全串行化，单进程内版本号不会错位 —— 因此不应当出现冲突，
	// 更不应当出现「成功但丢更新」。
	if conflict != 0 {
		t.Fatalf("unexpected conflicts in a single process: %d (version bookkeeping broken)", conflict)
	}
	if ok != n {
		t.Fatalf("successful writes = %d, want %d", ok, n)
	}
	// 无丢失更新：成功的次数必须等于最终待办条数（内存 + 库内都要对得上）。
	if got := readMeeting(t, s, id); len(got.Todos) != n {
		t.Fatalf("memory todos = %d, want %d (lost update)", len(got.Todos), n)
	}
	doc := qaReadDoc(t, s, id)
	if len(doc.Todos) != n {
		t.Fatalf("mongo todos = %d, want %d (lost update)", len(doc.Todos), n)
	}
	if doc.Version != meetingVersionFloor+n {
		t.Fatalf("mongo version = %d, want %d", doc.Version, meetingVersionFloor+n)
	}
}

// ─── 4. 懒加载读路径的 version 归一化 ───

func TestQALazyReadPathsNormalizeLegacyVersion(t *testing.T) {
	s := newLiveMongoStore(t)
	sc := Scope{UserID: "u1"}

	const legacyID = "m-qa-legacy-lazy"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.mongoMeetings.InsertOne(ctx, bson.D{
		{Key: "_id", Value: legacyID},
		{Key: "title", Value: "存量会议"},
		{Key: "project_id", Value: "p-qa-legacy"},
		{Key: "creator_id", Value: "u1"},
		{Key: "status", Value: string(model.MeetingWaiting)},
		{Key: "created_at", Value: time.Now().UTC()},
		{Key: "updated_at", Value: time.Now().UTC()},
	}); err != nil {
		t.Fatalf("insert legacy meeting doc: %v", err)
	}

	// ① GetMeeting 懒加载
	qaForgetMeeting(t, s, legacyID)
	m, appErr := s.GetMeeting(sc, legacyID)
	if appErr != nil {
		t.Fatalf("GetMeeting lazy load: %+v", appErr)
	}
	if m.Version != meetingVersionFloor {
		t.Fatalf("GetMeeting lazy-load version = %d, want %d", m.Version, meetingVersionFloor)
	}
	if got := readMeeting(t, s, legacyID); got.Version != meetingVersionFloor {
		t.Fatalf("cached version after GetMeeting = %d, want %d", got.Version, meetingVersionFloor)
	}

	// ② ListMeetingsByStatus 懒加载
	qaForgetMeeting(t, s, legacyID)
	var listed *model.Meeting
	for _, m := range s.ListMeetingsByStatus(sc, model.MeetingWaiting) {
		if m.ID == legacyID {
			listed = m
		}
	}
	if listed == nil {
		t.Fatal("legacy meeting not returned by ListMeetingsByStatus")
	}
	if listed.Version != meetingVersionFloor {
		t.Fatalf("ListMeetingsByStatus version = %d, want %d", listed.Version, meetingVersionFloor)
	}

	// ③ ListMeetingMessages 懒加载
	qaForgetMeeting(t, s, legacyID)
	if _, appErr := s.ListMeetingMessages(sc, legacyID); appErr != nil {
		t.Fatalf("ListMeetingMessages lazy load: %+v", appErr)
	}
	if got := readMeeting(t, s, legacyID); got.Version != meetingVersionFloor {
		t.Fatalf("version after ListMeetingMessages = %d, want %d", got.Version, meetingVersionFloor)
	}
}

// ─── 5. VerifyMeetingConsistency 必须只读 ───

func TestQAVerifyMeetingConsistencyIsReadOnly(t *testing.T) {
	s := newLiveMongoStore(t)
	sc := Scope{UserID: "u1"}

	created, appErr := s.CreateMeeting(sc, &model.Meeting{Title: "只读校验"})
	if appErr != nil {
		t.Fatalf("create meeting: %+v", appErr)
	}
	if _, appErr := s.AddMeetingMessage(sc, &model.MeetingMessage{
		MeetingID: created.ID, SenderType: "agent", SenderID: "ag-1", Phase: "speak", Target: "host", Content: "观点",
	}); appErr != nil {
		t.Fatalf("add message: %+v", appErr)
	}

	before := qaReadDoc(t, s, created.ID)

	checked1, mismatched1, err := s.VerifyMeetingConsistency()
	if err != nil {
		t.Fatalf("VerifyMeetingConsistency: %v", err)
	}
	checked2, mismatched2, err := s.VerifyMeetingConsistency()
	if err != nil {
		t.Fatalf("VerifyMeetingConsistency (2nd): %v", err)
	}
	if checked1 != 1 || checked2 != 1 {
		t.Fatalf("checked = %d / %d, want 1 / 1", checked1, checked2)
	}
	if mismatched1 != 0 || mismatched2 != 0 {
		t.Fatalf("mismatched = %d / %d, want 0 / 0", mismatched1, mismatched2)
	}

	after := qaReadDoc(t, s, created.ID)
	if after.Version != before.Version || after.Status != before.Status ||
		after.LastPhase != before.LastPhase || after.SpeakerTurns != before.SpeakerTurns ||
		len(after.Todos) != len(before.Todos) {
		t.Fatalf("VerifyMeetingConsistency mutated Mongo: before=%+v after=%+v", before, after)
	}
	// 内存侧也不能被校验动作改动（版本号是最敏感的指标）。
	if got := readMeeting(t, s, created.ID); got.Version != before.Version {
		t.Fatalf("memory version changed by verify: %d -> %d", before.Version, got.Version)
	}
}

// ─── 6. AddMeetingTodo：底层数组污染 / 切片别名 ───

func TestQAAddMeetingTodoNoBackingArrayAliasing(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	sc := Scope{UserID: "u1"}

	created, appErr := s.CreateMeeting(sc, &model.Meeting{Title: "切片别名", ProjectID: "p-mem"})
	if appErr != nil {
		t.Fatalf("create meeting: %+v", appErr)
	}
	// 预留多余容量，正是「原地 append 会污染底层数组」的经典前提。
	created.Todos = make([]model.MeetingTodoItem, 1, 4)
	created.Todos[0] = model.MeetingTodoItem{ID: "todo-0", Description: "原始待办"}

	first, appErr := s.AddMeetingTodo(sc, created.ID, model.MeetingTodoItem{Description: "第二条"})
	if appErr != nil {
		t.Fatalf("AddMeetingTodo: %+v", appErr)
	}
	if len(first.Todos) != 2 {
		t.Fatalf("after 1st add: len(todos) = %d, want 2", len(first.Todos))
	}
	// AddMeetingTodo 返回的是 store 内部的同一个 *Meeting，所以要抓住**切片头**
	// 才能判断第二次写入有没有就地改写第一次的底层数组。
	snapAfterFirst := first.Todos

	second, appErr := s.AddMeetingTodo(sc, created.ID, model.MeetingTodoItem{Description: "第三条"})
	if appErr != nil {
		t.Fatalf("AddMeetingTodo (2nd): %+v", appErr)
	}
	if len(second.Todos) != 3 {
		t.Fatalf("after 2nd add: len(todos) = %d, want 3", len(second.Todos))
	}
	// 关键断言：第一次写入产出的切片没有被第二次 append 就地改写
	// （说明每次都新建了底层数组，Mongo 失败时不会污染已发布出去的切片）。
	if len(snapAfterFirst) != 2 {
		t.Fatalf("first result slice aliased by second add: len = %d, want 2", len(snapAfterFirst))
	}
	if snapAfterFirst[0].Description != "原始待办" || snapAfterFirst[1].Description != "第二条" {
		t.Fatalf("first result slice corrupted: %+v", snapAfterFirst)
	}
}

// ─── 7. 探针：显式 version=0 的存量文档 ───
//
// P1 的回归守门：T2.1 之前 backfillMeetingVersions 的 filter 只有 {version: {$exists: false}}，
// 覆盖不到「字段存在但值为 0」的文档；而 loadMeetings 又把内存侧归一化成 1，于是
// filter {_id, version: 1} 永远匹配不上 → 该会议任何更新都 409，且重启也救不回来。
// 修复后 backfill filter 放宽为 {"version": {"$not": {"$gt": 0}}}，持有 0 的文档被归一为 1，
// 该会议即可写。本用例固化这个「修复后可写」的不变量。
func TestQALegacyZeroVersionDocumentUpdateFailsClosed(t *testing.T) {
	s := newLiveMongoStore(t)
	sc := Scope{UserID: "u1"}

	const legacyID = "m-qa-zero-version"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.mongoMeetings.InsertOne(ctx, bson.D{
		{Key: "_id", Value: legacyID},
		{Key: "title", Value: "显式零版本"},
		{Key: "project_id", Value: "p-qa-zero"},
		{Key: "creator_id", Value: "u1"},
		{Key: "status", Value: string(model.MeetingWaiting)},
		{Key: "version", Value: 0},
		{Key: "created_at", Value: time.Now().UTC()},
		{Key: "updated_at", Value: time.Now().UTC()},
	}); err != nil {
		t.Fatalf("insert zero-version meeting doc: %v", err)
	}

	s.backfillMeetingVersions()
	doc := qaReadDoc(t, s, legacyID)
	t.Logf("OBSERVED: mongo version after backfill = %d (backfill filter only covers {$exists:false})", doc.Version)

	items, _, err := s.loadMeetings()
	if err != nil {
		t.Fatalf("loadMeetings: %v", err)
	}
	legacy := items[legacyID]
	if legacy == nil {
		t.Fatal("zero-version meeting missing from loadMeetings")
	}
	t.Logf("OBSERVED: in-memory version after load = %d", legacy.Version)

	s.mu.Lock()
	s.meetings[legacyID] = legacy
	s.projectMeetings[legacy.ProjectID] = append(s.projectMeetings[legacy.ProjectID], legacyID)
	s.mu.Unlock()

	// 修复后：回填归一化过的存量会议必须可写（不再永久 409）。
	appErr := s.UpdateMeetingStatus(sc, legacyID, model.MeetingInProgress)
	if appErr != nil {
		t.Fatalf("legacy zero-version meeting must be writable after backfill, got %+v", appErr)
	}
	got := readMeeting(t, s, legacyID)
	if got.Status != model.MeetingInProgress || got.Version != meetingVersionFloor+1 {
		t.Fatalf("after writable update: status=%s version=%d", got.Status, got.Version)
	}
}

// ─── 8. 探针：AddMeetingMessage 会话态冲突后能自愈（P2） ───
//
// 修复前（QA 报告 P2）：内存 version 落后库内时，会话态写冲突会让该会议在本进程内
// 永久无法追加消息，且重试会留下重复消息。修复后 healZeroVersionMeetingLocked 在
// 「库内 version<=0」时自愈，retryOnceAfterVersionRefreshLocked 在「真实落后」时把内存
// version 刷到库内最新值后重试一次（会话态字段全由本条消息派生、整体覆盖，不吞其它语义）。
// 本用例固化：冲突自愈后首条消息即成功，内存/库内一致，重启后仍可续开。
//
// 已知代价（留给 T2.6 dedup 收实例）：无幂等键，客户端在超时后重试仍会重复入库消息。
func TestQAAddMeetingMessageSelfHealsOnVersionConflict(t *testing.T) {
	s := newLiveMongoStore(t)
	sc := Scope{UserID: "u1"}

	created, appErr := s.CreateMeeting(sc, &model.Meeting{Title: "残差探针"})
	if appErr != nil {
		t.Fatalf("create meeting: %+v", appErr)
	}
	// 模拟另一个写方把库内版本推到 5（内存仍是 1）。
	bumpMeetingVersionInMongo(t, s, created.ID, 5)

	send := func(content string) *transport.AppError {
		_, e := s.AddMeetingMessage(sc, &model.MeetingMessage{
			MeetingID: created.ID, SenderType: "agent", SenderID: "ag-1",
			Phase: "speak", Target: "host", Content: content,
		})
		return e
	}

	// 首条：库内 version(5) 与内存(1) 不一致 → 自愈刷新 version 并重试 → 必须成功。
	if err := send("第一条"); err != nil {
		t.Fatalf("first message must self-heal via version refresh + retry, got %+v", err)
	}
	// 第二条：内存已追上库内(6)，正常写 → 成功。
	if err := send("第二条"); err != nil {
		t.Fatalf("second message must succeed after self-heal, got %+v", err)
	}

	t.Logf("OBSERVED: %d message docs in Mongo after two self-healed sends",
		qaCountMessages(t, s, created.ID))

	// 自愈后内存与库内一致：会话态被推动、版本严格单调、消息已落内存索引。
	got := readMeeting(t, s, created.ID)
	if got.LastPhase != "speak" || got.SpeakerTurns != 2 {
		t.Fatalf("session state after self-heal: phase=%q turns=%d", got.LastPhase, got.SpeakerTurns)
	}
	if got.Version != 7 {
		t.Fatalf("version after two self-healed messages = %d, want 7", got.Version)
	}
	s.mu.RLock()
	inMem := len(s.meetingMessageIndex[created.ID])
	s.mu.RUnlock()
	if inMem != 2 {
		t.Fatalf("message index after self-heal = %d, want 2", inMem)
	}

	// 重启（重新载入）之后必须能自愈：版本追上库内，且能继续发言。
	s.mu.Lock()
	s.meetings = make(map[string]*model.Meeting)
	s.projectMeetings = make(map[string][]string)
	s.meetingMessages = make(map[string]*model.MeetingMessage)
	s.meetingMessageIndex = make(map[string][]string)
	s.mu.Unlock()
	if err := s.loadMongoState(); err != nil {
		t.Fatalf("loadMongoState: %v", err)
	}
	if _, appErr := s.AddMeetingMessage(sc, &model.MeetingMessage{
		MeetingID: created.ID, SenderType: "agent", SenderID: "ag-1",
		Phase: "speak", Target: "host", Content: "重启后第一条",
	}); appErr != nil {
		t.Fatalf("meeting not resumable after reload: %+v", appErr)
	}
	after := readMeeting(t, s, created.ID)
	if after.LastPhase != "speak" {
		t.Fatalf("session state not caught up after reload: phase=%q", after.LastPhase)
	}
	t.Logf("OBSERVED: after reload version=%d, messages in Mongo=%d, messages visible=%d",
		after.Version, qaCountMessages(t, s, created.ID), func() int {
			msgs, _ := s.ListMeetingMessages(sc, created.ID)
			return len(msgs)
		}())
}
