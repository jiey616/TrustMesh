package store

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"

	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// T2.3「项目域 Mongo 权威」测试（结构镜像 T2.2 的 task_mongo_authority_test.go）。
//
// 三类用例：
//  1. 「Mongo 失败零副作用」与 typed-nil 回归 —— 纯内存 + persistFailForTest 测试开关
//     （生产零成本），不需要真 Mongo，CI 必然执行。**这是 T2.3 的主护栏**。
//  2. 纯函数归一化 —— 无需 Mongo。
//  3. 需要真 Mongo 的正向用例（版本冲突 / 存量回填 / 双写一致性 / 重启读回）——
//     由 TRUSTMESH_TEST_MONGO_URI 门控，未设置时 t.Skip，不让 CI 因缺少 Mongo 变红。

// ─── 编译期护栏（T2.3 typed-nil 签名回归） ───
//
// 为什么必须是**编译期**护栏：纯内存模式（New()，无 Mongo）下 persistProjectUnsafe 会在
// 进入提交原语**之前**命中 `!mongoEnabled || mongoProjects == nil || project == nil` 的早返回，
// 直接返回字面 nil —— 因此运行时用例（TestPersistProjectUnsafeSuccessReturnsNilError）
// **无论签名是 error 还是 *transport.AppError 都会通过**，拦不住「签名退化」这类回归。
// T2.2 正是被这个形态咬过：persistTaskUnsafe 曾把 *transport.AppError 裸装进 error 接口，
// 成功时的 nil 指针变成「非 nil error」，调用方把成功当失败（CreateTaskByPMNode 返回 (nil,nil)）。
//
// 下面这行在编译期断言 persistProjectUnsafe 的签名恒为
// `func(*Store, *model.Project) *transport.AppError`：任何人把它改回 error，本文件立刻
// **编译失败**（错误指向本行），CI 直接变红，无需依赖任何运行时用例。
var _ func(*Store, *model.Project) *transport.AppError = (*Store).persistProjectUnsafe

// ─── 脚手架 ───

// seedProjectForFailureTest 直接往内存摆一条项目（绕开 Mongo），
// 这样「失败零副作用」断言才有个干净的基线可比。
func seedProjectForFailureTest(t *testing.T, s *Store, id string) *model.Project {
	t.Helper()
	now := time.Now().UTC()
	project := &model.Project{
		ID:        id,
		UserID:    "u1",
		Name:      "失败注入用项目",
		Status:    "active",
		PMAgentID: "pm-1",
		Version:   projectVersionFloor,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[project.ID] = project
	return project
}

// readProjectSnapshot 在 RLock 下取一份项目字段快照，避免断言本身产生数据竞争。
func readProjectSnapshot(t *testing.T, s *Store, id string) model.Project {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	project, ok := s.projects[id]
	if !ok || project == nil {
		t.Fatalf("project %q missing from memory", id)
	}
	return *project
}

// seedProjectOwner 建一个可归属的 PM agent，返回 (userID, agentID)。
// CreateProject 会校验 pm_agent_id 必须指向当前用户的 PM agent。
// 标识用 newID() 加唯一后缀：活库用例共享同一个数据库，固定 email / nodeID 会跨用例（甚至
// 跨多次测试运行）撞 EMAIL_EXISTS。
func seedProjectOwner(t *testing.T, s *Store) (string, string) {
	t.Helper()
	suffix := newID()
	user, appErr := s.CreateUser("owner-"+suffix+"@example.com", "Owner", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	pm, appErr := s.CreateAgent(Scope{UserID: user.ID}, "node-pm-"+suffix, "PM Agent", "pm", "pm", []string{"plan"})
	if appErr != nil {
		t.Fatalf("create pm agent: %v", appErr)
	}
	return user.ID, pm.ID
}

// newLiveProjectMongoStore 连接 TRUSTMESH_TEST_MONGO_URI 指向的 MongoDB（本地测试库），
// 并为每个用例清一次 projects 集合，保证用例之间互不干扰。
// 环境变量未设置 / Mongo 不可达时跳过。
func newLiveProjectMongoStore(t *testing.T) *Store {
	t.Helper()
	uri := os.Getenv("TRUSTMESH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("skip: set TRUSTMESH_TEST_MONGO_URI to a reachable MongoDB to enable this test")
	}
	db := os.Getenv("TRUSTMESH_TEST_MONGO_DB")
	if db == "" {
		db = "trustmesh_t23_test"
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
	if _, err := s.mongoProjects.DeleteMany(ctx, bson.D{}); err != nil {
		t.Fatalf("clean projects collection: %v", err)
	}
	// enableMongo 会先把库里所有项目载入内存，随后才轮到上面的清库；
	// 不清内存就会残留上一个用例写下的项目，让断言失真。
	s.mu.Lock()
	s.projects = make(map[string]*model.Project)
	s.mu.Unlock()
	return s
}

// bumpProjectVersionInMongo 绕过内存、直接把库内 version 推到指定值，
// 模拟「另一个写方已经改过这条项目」—— 这是制造版本冲突的唯一真实手段。
func bumpProjectVersionInMongo(t *testing.T, s *Store, projectID string, to int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.mongoProjects.UpdateOne(ctx, bson.M{"_id": projectID}, bson.M{"$set": bson.M{"version": to}}); err != nil {
		t.Fatalf("bump project version to %d: %v", to, err)
	}
}

// ─── 1. Mongo 失败零副作用（快照回滚包装器） ───

func TestProjectMongoFailureZeroSideEffect(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedProjectForFailureTest(t, s, "p-zse")

	// 启用测试用强制失败开关（生产零成本）——模拟持久化失败。
	s.persistFailForTest = true

	s.mu.Lock()
	appErr := s.mutateProjectUnsafe("p-zse", func(p *model.Project) *transport.AppError {
		p.Name = "被污染的名字"
		p.Status = "archived"
		return nil
	})
	s.mu.Unlock()

	if appErr == nil {
		t.Fatal("mutation must fail when the persist seam is armed")
	}

	got := readProjectSnapshot(t, s, "p-zse")
	if got.Name != "失败注入用项目" {
		t.Fatalf("project name mutated on failed persist: %q, want unchanged", got.Name)
	}
	if got.Status != "active" {
		t.Fatalf("project status mutated on failed persist: %q, want active", got.Status)
	}
	if got.Version != projectVersionFloor {
		t.Fatalf("version must not advance on failed persist: %d, want %d", got.Version, projectVersionFloor)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("snapshot restore dropped UpdatedAt")
	}

	// 解除开关后同一改动必须成功 —— 证明方才的失败确实来自持久化环节，
	// 而不是别的（例如 NotFound / fn 自身报错）在挡路。
	s.persistFailForTest = false
	s.mu.Lock()
	appErr = s.mutateProjectUnsafe("p-zse", func(p *model.Project) *transport.AppError {
		p.Status = "archived"
		return nil
	})
	s.mu.Unlock()
	if appErr != nil {
		t.Fatalf("mutation after disarming the seam: %v", appErr)
	}
	if readProjectSnapshot(t, s, "p-zse").Status != "archived" {
		t.Fatal("mutation did not take effect after the seam was disarmed")
	}
}

// TestMutateProjectUnsafeRollsBackOnFnError 覆盖「fn 自身返错也要回滚」这一半契约。
func TestMutateProjectUnsafeRollsBackOnFnError(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedProjectForFailureTest(t, s, "p-fnerr")

	s.mu.Lock()
	appErr := s.mutateProjectUnsafe("p-fnerr", func(p *model.Project) *transport.AppError {
		p.Name = "被污染的名字"
		return transport.Conflict("SOME_GUARD", "rejected mid-mutation")
	})
	s.mu.Unlock()

	if appErr == nil || appErr.Code != "SOME_GUARD" {
		t.Fatalf("fn error must propagate unchanged, got %+v", appErr)
	}
	if got := readProjectSnapshot(t, s, "p-fnerr"); got.Name != "失败注入用项目" {
		t.Fatalf("memory mutated despite fn error: name=%q, want unchanged", got.Name)
	}
}

// ─── 2. CreateProject 幽灵项目护栏（修 §2.3） ───

func TestCreateProjectLeavesNoGhostOnPersistFailure(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	userID, pmAgentID := seedProjectOwner(t, s)

	s.persistFailForTest = true
	project, appErr := s.CreateProject(Scope{UserID: userID}, "幽灵项目", "desc", pmAgentID)
	if appErr == nil {
		t.Fatal("CreateProject must fail when the persist seam is armed")
	}
	if project != nil {
		t.Fatalf("failed CreateProject returned a non-nil project: %+v", project)
	}

	s.mu.RLock()
	count := len(s.projects)
	s.mu.RUnlock()
	if count != 0 {
		t.Fatalf("ghost project left in memory after failed persist: %d entries", count)
	}
	if got := s.ListProjects(Scope{UserID: userID}); len(got) != 0 {
		t.Fatalf("ListProjects surfaced %d ghost project(s)", len(got))
	}
}

// ─── 3. typed-nil 回归护栏（无 Mongo，CI 必跑） ───
//
// 背景：persistProjectUnsafe 返回 *transport.AppError。若有人把它裸装进 error 接口
// （例如旧的 `return s.applyProjectVersionedReplaceLocked(project)` 写法），成功时的
// 「nil 指针」装进接口后是非 nil（Go typed-nil 陷阱），调用方会把成功误判为失败。
// 以下两条用例专门锁死这个回归。

func TestPersistProjectUnsafeSuccessReturnsNilError(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedProjectForFailureTest(t, s, "p-typednil")

	s.mu.Lock()
	project := s.projects["p-typednil"]
	appErr := s.persistProjectUnsafe(project)
	s.mu.Unlock()

	if appErr != nil {
		t.Fatalf("successful persistProjectUnsafe must return a nil error, got %#v", appErr)
	}
}

func TestCreateProjectReturnsProject(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	userID, pmAgentID := seedProjectOwner(t, s)

	project, appErr := s.CreateProject(Scope{UserID: userID}, "typed-nil 回归", "desc", pmAgentID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	if project == nil {
		t.Fatal("CreateProject returned a nil project with a nil error (typed-nil regression)")
	}
	if project.ID == "" {
		t.Fatal("created project has empty ID")
	}
	if project.Status != "active" {
		t.Fatalf("created project status = %q, want active", project.Status)
	}
}

// ─── 4. 存量 version 归一化（纯函数，无需 Mongo） ───

func TestNormalizeProjectVersionBackfillsLegacyZero(t *testing.T) {
	// 存量项目文档（T2.3 之前写入）没有 version 字段，解码后为 0。
	legacy := &model.Project{ID: "p-legacy"}
	normalizeProjectVersion(legacy)
	if legacy.Version != projectVersionFloor {
		t.Fatalf("legacy version = %d, want %d", legacy.Version, projectVersionFloor)
	}

	// 幂等：已有版本不得被覆盖。
	legacy.Version = 7
	normalizeProjectVersion(legacy)
	if legacy.Version != 7 {
		t.Fatalf("existing version overwritten: %d, want 7", legacy.Version)
	}

	// 显式非法值（0 / 负数）也要一并归一。
	for _, bad := range []int{0, -3} {
		project := &model.Project{ID: "p-bad", Version: bad}
		normalizeProjectVersion(project)
		if project.Version != projectVersionFloor {
			t.Fatalf("version %d not normalized: got %d", bad, project.Version)
		}
	}

	// nil 安全（解码路径可能拿到 nil）。
	normalizeProjectVersion(nil)
}

// ─── 5. 需要真 Mongo 的用例 ───

// TestProjectVersionConflictOnStaleVersion 乐观锁：内存版本陈旧时，
// 更新必须返回 409 冲突（而不是静默覆盖别人的写入）。
func TestProjectVersionConflictOnStaleVersion(t *testing.T) {
	s := newLiveProjectMongoStore(t)
	seedProjectForFailureTest(t, s, "p-conf")

	// 首次落库：把内存与库内版本一起推进一步。
	s.mu.Lock()
	appErr := s.persistProjectUnsafe(s.projects["p-conf"])
	s.mu.Unlock()
	if appErr != nil {
		t.Fatalf("initial persist: %v", appErr)
	}
	after := readProjectSnapshot(t, s, "p-conf")
	if after.Version < projectVersionFloor {
		t.Fatalf("version after first write = %d, want >= %d", after.Version, projectVersionFloor)
	}

	// 另一个写方先把库内版本推到更后面（内存仍旧落后）。
	bumpProjectVersionInMongo(t, s, "p-conf", after.Version+5)

	s.mu.Lock()
	appErr = s.mutateProjectUnsafe("p-conf", func(p *model.Project) *transport.AppError {
		p.Status = "archived"
		return nil
	})
	s.mu.Unlock()

	if appErr == nil {
		t.Fatal("stale version update must return a conflict, not silently overwrite")
	}
	if appErr.Status != http.StatusConflict {
		t.Fatalf("conflict status = %d, want 409", appErr.Status)
	}
	if appErr.Code != projectVersionConflictCode {
		t.Fatalf("conflict code = %q, want %s", appErr.Code, projectVersionConflictCode)
	}

	got := readProjectSnapshot(t, s, "p-conf")
	if got.Status != "active" {
		t.Fatalf("memory status mutated by conflicted write: %q", got.Status)
	}
	// W1 语义（refresh_on_conflict.go）：冲突后内存会被回源刷新为 Mongo 权威文档，
	// 版本应等于远端版本而非本地旧版本（同 task 域用例说明）。
	if want := after.Version + 5; got.Version != want {
		t.Fatalf("memory version after conflict = %d, want %d（应回源刷新为远端权威版本）", got.Version, want)
	}
}

// TestBackfillProjectVersionsRepairsLegacyZero 存量文档（无 version 字段 / 显式 0 /
// 非数字 / 负数）必须被回填成起始版本，否则带 {_id, version} filter 的更新永远失配
// = 永久锁死。
func TestBackfillProjectVersionsRepairsLegacyZero(t *testing.T) {
	s := newLiveProjectMongoStore(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 绕过结构体，直接插入「旧镜像」形态的文档：根本没有 version 字段。
	if _, err := s.mongoProjects.InsertOne(ctx, bson.M{"_id": "p-legacy", "name": "legacy", "status": "active"}); err != nil {
		t.Fatalf("insert legacy project: %v", err)
	}
	// 显式 0 也要能被覆盖。
	if _, err := s.mongoProjects.InsertOne(ctx, bson.M{"_id": "p-zero", "name": "zero", "status": "active", "version": 0}); err != nil {
		t.Fatalf("insert zero-version project: %v", err)
	}
	// 负数（非法历史取值）也要能被覆盖。
	if _, err := s.mongoProjects.InsertOne(ctx, bson.M{"_id": "p-neg", "name": "neg", "status": "active", "version": -2}); err != nil {
		t.Fatalf("insert negative-version project: %v", err)
	}

	s.backfillProjectVersions()

	for _, id := range []string{"p-legacy", "p-zero", "p-neg"} {
		var doc struct {
			Version int `bson:"version"`
		}
		if err := s.mongoProjects.FindOne(ctx, bson.M{"_id": id}).Decode(&doc); err != nil {
			t.Fatalf("read back %s: %v", id, err)
		}
		if doc.Version != projectVersionFloor {
			t.Fatalf("%s version after backfill = %d, want %d", id, doc.Version, projectVersionFloor)
		}
	}
}

// TestVerifyProjectConsistencyMismatchedZero 双写过渡期：内存与 Mongo 必须一致。
// 本用例是 TaskSummary / UpdatedAt「假 mismatch」陷阱的护栏 —— 若实现里误比了这两个字段，
// 本用例会因 mismatched > 0 立即变红。
func TestVerifyProjectConsistencyMismatchedZero(t *testing.T) {
	s := newLiveProjectMongoStore(t)
	seedProjectForFailureTest(t, s, "p-verify")

	s.mu.Lock()
	appErr := s.persistProjectUnsafe(s.projects["p-verify"])
	s.mu.Unlock()
	if appErr != nil {
		t.Fatalf("persist for consistency check: %v", appErr)
	}

	checked, mismatched, err := s.VerifyProjectConsistency()
	if err != nil {
		t.Fatalf("VerifyProjectConsistency: %v", err)
	}
	if checked == 0 {
		t.Fatal("expected at least one project to be checked")
	}
	if mismatched != 0 {
		t.Fatalf("mismatched = %d, want 0 (memory diverged from Mongo)", mismatched)
	}
}

// TestProjectRehydrateAfterWritePaths 重启零丢失：创建 / 更新（含工作流）/ 归档后
// 重新载入，项目、Workflows、PrimaryWorkflowID/Index、Status 必须全部读回。
func TestProjectRehydrateAfterWritePaths(t *testing.T) {
	s := newLiveProjectMongoStore(t)
	userID, pmAgentID := seedProjectOwner(t, s)

	project, appErr := s.CreateProject(Scope{UserID: userID}, "rehydrate", "desc", pmAgentID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}

	wfs := []model.Workflow{{
		ID:   "wf-1",
		Name: "总流程",
		Steps: []model.WorkflowStep{
			{Name: "剧本创作", Role: "developer"},
			{Name: "质量验收", Role: "developer"},
		},
	}}
	newName := "rehydrate-2"
	idx := 0
	updated, appErr := s.UpdateProject(Scope{UserID: userID}, project.ID, UpdateProjectInput{
		Name:                 &newName,
		Workflows:            wfs,
		PrimaryWorkflowIndex: &idx,
	})
	if appErr != nil {
		t.Fatalf("update project: %v", appErr)
	}
	if updated.PrimaryWorkflowID != "wf-1" {
		t.Fatalf("primary workflow id after update = %q, want wf-1", updated.PrimaryWorkflowID)
	}

	archived, appErr := s.ArchiveProject(Scope{UserID: userID}, project.ID)
	if appErr != nil {
		t.Fatalf("archive project: %v", appErr)
	}
	if archived.Status != "archived" {
		t.Fatalf("status after archive = %q, want archived", archived.Status)
	}

	// 模拟进程重启：从同一 Mongo 重新载入项目集合。
	loaded, err := s.loadProjects()
	if err != nil {
		t.Fatalf("loadProjects after restart: %v", err)
	}
	got, ok := loaded[project.ID]
	if !ok || got == nil {
		t.Fatal("project missing after reload from Mongo")
	}
	if got.Name != newName {
		t.Fatalf("name lost across restart: %q, want %q", got.Name, newName)
	}
	if got.Status != "archived" {
		t.Fatalf("status lost across restart: %q, want archived", got.Status)
	}
	if len(got.Workflows) != 1 {
		t.Fatalf("workflows lost across restart: %d, want 1", len(got.Workflows))
	}
	if got.PrimaryWorkflowID != "wf-1" || got.PrimaryWorkflowIndex != 0 {
		t.Fatalf("primary workflow lost across restart: id=%q index=%d, want wf-1/0", got.PrimaryWorkflowID, got.PrimaryWorkflowIndex)
	}
	if got.Version < projectVersionFloor {
		t.Fatalf("version lost across restart: %d, want >= %d", got.Version, projectVersionFloor)
	}
}
