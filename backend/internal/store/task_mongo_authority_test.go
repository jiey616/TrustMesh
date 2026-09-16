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
	"trustmesh/backend/internal/transport"
)

// T2.2「任务域 Mongo 权威」测试（结构镜像 T2.1 会议域的 meeting_mongo_authority_test.go）。
//
// 三类用例：
//  1. 「Mongo 失败零副作用」与 typed-nil 回归 —— 纯内存 + persistFailForTest 测试开关
//     （生产零成本），不需要真 Mongo，CI 必然执行。**这是 T2.2 的主护栏**。
//  2. 纯函数归一化 —— 无需 Mongo。
//  3. 需要真 Mongo 的正向用例（版本冲突 / 存量回填 / 双写一致性 / 重启续推）——
//     由 TRUSTMESH_TEST_MONGO_URI 门控，未设置时 t.Skip，不让 CI 因缺少 Mongo 变红。

// ─── 编译期护栏（T2.2 typed-nil 签名回归） ───
//
// 与 T2.3 的 project 域护栏（project_mongo_authority_test.go:39）同源：把「签名回退」
// 这一整类回归变成构建失败，不依赖任何运行时用例、不依赖 Mongo。
//
// 注意这里钉的对象与 project 域护栏**语义相反**：Task 域 persistTaskUnsafe 的**合法**
// 签名就是 `error`（其修复方式是显式判空：`if appErr := s.applyTaskVersionedReplaceLocked(task);
// appErr != nil { return appErr }; return nil`），因此本断言钉住的是「当前正确形态」——
// 防止有人改回裸 `return s.applyTaskVersionedReplaceLocked(task)` 那种把
// *transport.AppError 直接装进 error 接口的写法（成功时 nil 指针变非 nil 的 typed-nil 陷阱）。
// 真正能在运行时咬住「裸返回」的是 TestPersistTaskUnsafeSuccessReturnsNilError —— 但它在纯内存
// 下会命中提前返回而恒绿，所以这行编译期断言是唯一稳定的 CI 可见护栏。
// 任何人把 persistTaskUnsafe 的返回类型改成 *transport.AppError，本文件立刻编译失败。
var _ func(*Store, *model.TaskDetail) error = (*Store).persistTaskUnsafe

// ─── 脚手架 ───

// newTaskMongoFailingStore 构造「Mongo 恒失败」的 Store。
//
// 手法与 T2.1 一致：mongo.Connect 是惰性的（不建连、不报错），把 mongoTimeout 设成 1ns 后，
// 任何 Mongo 操作都会立刻返回 context deadline exceeded —— 拿到的是驱动真实错误，
// 且完全不依赖 docker / 网络。
func newTaskMongoFailingStore(t *testing.T) *Store {
	t.Helper()
	cli, err := mongo.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:1/"))
	if err != nil {
		t.Fatalf("mongo.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cli.Disconnect(context.Background()) })

	s := New()
	s.log = zap.NewNop()
	s.mongoEnabled = true
	db := cli.Database("trustmesh_t22_test")
	s.mongoTasks = db.Collection("tasks")
	s.mongoEvents = db.Collection("events")
	s.mongoTimeout = time.Nanosecond
	return s
}

// seedTaskForFailureTest 直接往内存摆一条任务（绕开 Mongo），
// 这样「失败零副作用」断言才有个干净的基线可比。
func seedTaskForFailureTest(t *testing.T, s *Store, id string) *model.TaskDetail {
	t.Helper()
	task := &model.TaskDetail{
		ID:        id,
		UserID:    "u1",
		ProjectID: "p1",
		Title:     "失败注入用任务",
		Status:    "pending",
		Version:   taskVersionFloor,
		Todos: []model.Todo{{
			ID:       "td-1",
			Title:    "阶段一",
			Status:   "pending",
			Assignee: model.TodoAssignee{AgentID: "a1", Name: "开发", NodeID: "node-1"},
		}},
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
	s.projectTasks[task.ProjectID] = append(s.projectTasks[task.ProjectID], task.ID)
	return task
}

// readTaskSnapshot 在 RLock 下取一份任务字段快照，避免断言本身产生数据竞争。
func readTaskSnapshot(t *testing.T, s *Store, id string) model.TaskDetail {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.tasks[id]
	if !ok || task == nil {
		t.Fatalf("task %q missing from memory", id)
	}
	return *task
}

// newLiveTaskMongoStore 连接 TRUSTMESH_TEST_MONGO_URI 指向的 MongoDB（本地测试库），
// 并为每个用例清一次 tasks / events 集合，保证用例之间互不干扰。
// 环境变量未设置 / Mongo 不可达时跳过。
func newLiveTaskMongoStore(t *testing.T) *Store {
	t.Helper()
	uri := os.Getenv("TRUSTMESH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("skip: set TRUSTMESH_TEST_MONGO_URI to a reachable MongoDB to enable this test")
	}
	db := os.Getenv("TRUSTMESH_TEST_MONGO_DB")
	if db == "" {
		db = "trustmesh_t22_test"
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
	if _, err := s.mongoTasks.DeleteMany(ctx, bson.D{}); err != nil {
		t.Fatalf("clean tasks collection: %v", err)
	}
	if _, err := s.mongoEvents.DeleteMany(ctx, bson.D{}); err != nil {
		t.Fatalf("clean events collection: %v", err)
	}
	// enableMongo 会先把库里所有任务载入内存，随后才轮到上面的清库；
	// 不清内存就会残留上一个用例写下的任务，让断言失真。
	s.mu.Lock()
	s.tasks = make(map[string]*model.TaskDetail)
	s.projectTasks = make(map[string][]string)
	s.taskEvents = make(map[string][]model.Event)
	s.mu.Unlock()
	return s
}

// bumpTaskVersionInMongo 绕过内存、直接把库内 version 推到指定值，
// 模拟「另一个写方已经改过这条任务」—— 这是制造版本冲突的唯一真实手段。
func bumpTaskVersionInMongo(t *testing.T, s *Store, taskID string, to int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.mongoTasks.UpdateOne(ctx, bson.M{"_id": taskID}, bson.M{"$set": bson.M{"version": to}}); err != nil {
		t.Fatalf("bump task version to %d: %v", to, err)
	}
}

// ─── 1. Mongo 失败零副作用（快照回滚包装器） ───

func TestTaskMongoFailureZeroSideEffect(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedTaskForFailureTest(t, s, "t-zse")

	// 启用测试用强制失败开关（生产零成本）——模拟持久化失败。
	s.persistFailForTest = true

	s.mu.Lock()
	appErr := s.mutateTaskUnsafe("t-zse", func(task *model.TaskDetail) *transport.AppError {
		task.Status = "in_progress"
		task.Todos[0].Status = "in_progress"
		return nil
	})
	s.mu.Unlock()

	if appErr == nil {
		t.Fatal("mutation must fail when the persist seam is armed")
	}

	got := readTaskSnapshot(t, s, "t-zse")
	if got.Status != "pending" {
		t.Fatalf("task status mutated on failed persist: %q, want pending", got.Status)
	}
	if len(got.Todos) != 1 || got.Todos[0].Status != "pending" {
		t.Fatalf("todo mutated on failed persist: %+v", got.Todos)
	}
	if got.Version != taskVersionFloor {
		t.Fatalf("version must not advance on failed persist: %d, want %d", got.Version, taskVersionFloor)
	}

	// 解除开关后同一改动必须成功 —— 证明方才的失败确实来自持久化环节，
	// 而不是别的（例如 NotFound / fn 自身报错）在挡路。
	s.persistFailForTest = false
	s.mu.Lock()
	appErr = s.mutateTaskUnsafe("t-zse", func(task *model.TaskDetail) *transport.AppError {
		task.Status = "in_progress"
		return nil
	})
	s.mu.Unlock()
	if appErr != nil {
		t.Fatalf("mutation after disarming the seam: %v", appErr)
	}
	if readTaskSnapshot(t, s, "t-zse").Status != "in_progress" {
		t.Fatal("mutation did not take effect after the seam was disarmed")
	}
}

// TestMutateTaskUnsafeRollsBackOnFnError 覆盖「fn 自身返错也要回滚」这一半契约。
func TestMutateTaskUnsafeRollsBackOnFnError(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedTaskForFailureTest(t, s, "t-fnerr")

	s.mu.Lock()
	appErr := s.mutateTaskUnsafe("t-fnerr", func(task *model.TaskDetail) *transport.AppError {
		task.Status = "in_progress"
		return transport.Conflict("SOME_GUARD", "rejected mid-mutation")
	})
	s.mu.Unlock()

	if appErr == nil || appErr.Code != "SOME_GUARD" {
		t.Fatalf("fn error must propagate unchanged, got %+v", appErr)
	}
	if got := readTaskSnapshot(t, s, "t-fnerr"); got.Status != "pending" {
		t.Fatalf("memory mutated despite fn error: status=%q, want pending", got.Status)
	}
}

// ─── 2. typed-nil 回归护栏（无 Mongo，CI 必跑） ───
//
// 背景：persistTaskUnsafe 签名是 error，但其成功路径委托给
// applyTaskVersionedReplaceLocked（返回 *transport.AppError）。若直接 return 其返回值，
// 成功时的「nil 指针」装进 error 接口后是非 nil（Go typed-nil 陷阱），
// 会让 persistAgentGraphUnsafe -> CreateTaskByPMNode 把成功误判为失败，
// 最终返回 (nil, nil) —— 调用方解引用即 panic。以下两条用例专门锁死这个回归。

func TestPersistTaskUnsafeSuccessReturnsNilError(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedTaskForFailureTest(t, s, "t-typednil")

	s.mu.Lock()
	task := s.tasks["t-typednil"]
	err := s.persistTaskUnsafe(task)
	s.mu.Unlock()

	if err != nil {
		t.Fatalf("successful persistTaskUnsafe must return a nil error, got %#v", err)
	}
}

func TestCreateTaskByPMNodeReturnsTask(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)
	s.log = zap.NewNop()

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "typed-nil 回归",
		Description: "创建路径必须返回非 nil 任务",
		Todos: []TaskCreateTodoInput{
			{Title: "阶段一", Description: "d", AssigneeNodeID: developer.NodeID},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}
	if task == nil {
		t.Fatal("CreateTaskByPMNode returned a nil task with a nil error (typed-nil regression)")
	}
	if len(task.Todos) != 1 {
		t.Fatalf("created task todos = %d, want 1", len(task.Todos))
	}
}

// ─── 3. 存量 version 归一化（纯函数，无需 Mongo） ───

func TestNormalizeTaskVersionBackfillsLegacyZero(t *testing.T) {
	// 存量任务文档（T2.2 之前写入）没有 version 字段，解码后为 0。
	legacy := &model.TaskDetail{ID: "t-legacy"}
	normalizeTaskVersion(legacy)
	if legacy.Version != taskVersionFloor {
		t.Fatalf("legacy version = %d, want %d", legacy.Version, taskVersionFloor)
	}

	// 幂等：已有版本不得被覆盖。
	legacy.Version = 7
	normalizeTaskVersion(legacy)
	if legacy.Version != 7 {
		t.Fatalf("existing version overwritten: %d, want 7", legacy.Version)
	}

	// 显式非法值（0 / 负数）也要一并归一。
	for _, bad := range []int{0, -3} {
		task := &model.TaskDetail{ID: "t-bad", Version: bad}
		normalizeTaskVersion(task)
		if task.Version != taskVersionFloor {
			t.Fatalf("version %d not normalized: got %d", bad, task.Version)
		}
	}

	// nil 安全（解码路径可能拿到 nil）。
	normalizeTaskVersion(nil)
}

// ─── 4. 需要真 Mongo 的用例 ───

// TestTaskVersionConflictOnStaleVersion 乐观锁：内存版本陈旧时，
// 更新必须返回 409 冲突（而不是静默覆盖别人的写入）。
func TestTaskVersionConflictOnStaleVersion(t *testing.T) {
	s := newLiveTaskMongoStore(t)
	seedTaskForFailureTest(t, s, "t-conf")

	// 首次落库：把内存与库内版本一起推进一步（1 -> 2）。
	s.mu.Lock()
	err := s.persistTaskBundleUnsafe("t-conf")
	s.mu.Unlock()
	if err != nil {
		t.Fatalf("initial persist: %v", err)
	}
	after := readTaskSnapshot(t, s, "t-conf")
	if after.Version != taskVersionFloor+1 {
		t.Fatalf("version after first write = %d, want %d", after.Version, taskVersionFloor+1)
	}

	// 另一个写方先把库内版本推到更后面（内存仍旧落后）。
	bumpTaskVersionInMongo(t, s, "t-conf", after.Version+5)

	s.mu.Lock()
	appErr := s.mutateTaskUnsafe("t-conf", func(task *model.TaskDetail) *transport.AppError {
		task.Status = "in_progress"
		return nil
	})
	s.mu.Unlock()

	if appErr == nil {
		t.Fatal("stale version update must return a conflict, not silently overwrite")
	}
	if appErr.Status != http.StatusConflict {
		t.Fatalf("conflict status = %d, want 409", appErr.Status)
	}
	if appErr.Code != "TASK_VERSION_CONFLICT" {
		t.Fatalf("conflict code = %q, want TASK_VERSION_CONFLICT", appErr.Code)
	}

	got := readTaskSnapshot(t, s, "t-conf")
	if got.Status != "pending" {
		t.Fatalf("memory status mutated by conflicted write: %q", got.Status)
	}
	// W1 语义（refresh_on_conflict.go）：冲突后内存会被回源刷新为 Mongo 权威文档，
	// 所以版本应等于远端（被另一写方推到的）版本，而不是本次写之前的本地旧版本。
	// 断言「回滚成旧版本」是 W1 之前的行为，会让这条用例与已上线的收敛逻辑互相矛盾。
	if want := after.Version + 5; got.Version != want {
		t.Fatalf("memory version after conflict = %d, want %d（应回源刷新为远端权威版本）", got.Version, want)
	}
}

// TestBackfillTaskVersionsRepairsLegacyZero 存量文档（无 version 字段 / 显式 0）
// 必须被回填成起始版本，否则带 {_id, version} filter 的更新永远失配 = 永久锁死。
func TestBackfillTaskVersionsRepairsLegacyZero(t *testing.T) {
	s := newLiveTaskMongoStore(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 绕过结构体，直接插入「旧镜像」形态的文档：根本没有 version 字段。
	if _, err := s.mongoTasks.InsertOne(ctx, bson.M{"_id": "t-legacy", "title": "legacy", "status": "pending"}); err != nil {
		t.Fatalf("insert legacy task: %v", err)
	}
	// 显式 0 也要能被覆盖。
	if _, err := s.mongoTasks.InsertOne(ctx, bson.M{"_id": "t-zero", "title": "zero", "status": "pending", "version": 0}); err != nil {
		t.Fatalf("insert zero-version task: %v", err)
	}

	s.backfillTaskVersions()

	for _, id := range []string{"t-legacy", "t-zero"} {
		var doc struct {
			Version int `bson:"version"`
		}
		if err := s.mongoTasks.FindOne(ctx, bson.M{"_id": id}).Decode(&doc); err != nil {
			t.Fatalf("read back %s: %v", id, err)
		}
		if doc.Version != taskVersionFloor {
			t.Fatalf("%s version after backfill = %d, want %d", id, doc.Version, taskVersionFloor)
		}
	}
}

// TestVerifyTaskConsistencyMismatchedZero 双写过渡期：内存与 Mongo 必须一致。
func TestVerifyTaskConsistencyMismatchedZero(t *testing.T) {
	s := newLiveTaskMongoStore(t)
	seedTaskForFailureTest(t, s, "t-verify")

	s.mu.Lock()
	err := s.persistTaskBundleUnsafe("t-verify")
	s.mu.Unlock()
	if err != nil {
		t.Fatalf("persist for consistency check: %v", err)
	}

	checked, mismatched, err := s.VerifyTaskConsistency()
	if err != nil {
		t.Fatalf("VerifyTaskConsistency: %v", err)
	}
	if checked == 0 {
		t.Fatal("expected at least one task to be checked")
	}
	if mismatched != 0 {
		t.Fatalf("mismatched = %d, want 0 (memory diverged from Mongo)", mismatched)
	}
}

// TestTaskRehydrateContinuesPipeline 重启后流水线可继续：落库的任务必须能被完整读回
// （含 todos 与 version），否则重启即丢推进状态。
func TestTaskRehydrateContinuesPipeline(t *testing.T) {
	s := newLiveTaskMongoStore(t)
	seedTaskForFailureTest(t, s, "t-rehydrate")

	s.mu.Lock()
	err := s.persistTaskBundleUnsafe("t-rehydrate")
	s.mu.Unlock()
	if err != nil {
		t.Fatalf("persist before rehydrate: %v", err)
	}
	wantVersion := readTaskSnapshot(t, s, "t-rehydrate").Version

	// 模拟进程重启：从同一 Mongo 重新载入任务集合。
	loaded, _, err := s.loadTasks()
	if err != nil {
		t.Fatalf("loadTasks after restart: %v", err)
	}
	got, ok := loaded["t-rehydrate"]
	if !ok || got == nil {
		t.Fatal("task missing after reload from Mongo")
	}
	if len(got.Todos) != 1 {
		t.Fatalf("todos lost across restart: %d, want 1", len(got.Todos))
	}
	if got.Version != wantVersion {
		t.Fatalf("version lost across restart: %d, want %d", got.Version, wantVersion)
	}
	if got.Status != "pending" {
		t.Fatalf("status lost across restart: %q, want pending", got.Status)
	}
}
