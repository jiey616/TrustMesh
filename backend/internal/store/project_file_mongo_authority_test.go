package store

import (
	"context"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"

	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// T2.3b「项目文件域 Mongo 权威」测试（结构镜像 T2.3 的 project_mongo_authority_test.go）。
//
// 三类用例：
//  1. 「Mongo 失败零副作用」与 typed-nil 回归 —— 纯内存 + persistFailForTest 测试开关
//     （生产零成本），不需要真 Mongo，CI 必然执行。**这是 T2.3b 的主护栏**：
//     新增路径（SaveProjectFile / CreateFolder / CreateMeetingMinutesFile）失败必须把
//     projectFiles / projectFileIndex / transferFileIndex 三张索引**逐字节**回滚（不留幽灵）；
//     就地更新路径（Rename / Move）失败必须经 mutateProjectFileUnsafe 还原快照。
//  2. 纯函数归一化 —— 无需 Mongo。
//  3. 需要真 Mongo 的正向用例（版本冲突 / 存量回填 / 双写一致性 / 删除不复活 / 重启读回）——
//     由 TRUSTMESH_TEST_MONGO_URI 门控，未设置时 t.Skip，不让 CI 因缺少 Mongo 变红。

// ─── 编译期护栏（T2.3b typed-nil 签名回归） ───
//
// 为什么必须是**编译期**护栏：纯内存模式（New()，无 Mongo）下 persistProjectFileUnsafe 会在
// 进入提交原语**之前**命中 `!mongoEnabled || mongoProjectFiles == nil || pf == nil` 的早返回，
// 直接返回字面 nil —— 因此运行时用例（TestPersistProjectFileUnsafeSuccessReturnsNilError）
// **无论签名是 error 还是 *transport.AppError 都会通过**，拦不住「签名退化」这类回归。
// T2.2 正是被这个形态咬过：persistTaskUnsafe 曾把 *transport.AppError 裸装进 error 接口，
// 成功时的 nil 指针变成「非 nil error」，调用方把成功当失败（CreateTaskByPMNode 返回 (nil,nil)）。
// T2.3b 的 FlushPersistAll projectFiles sweep 是最容易踩坑的调用点：裸传 collect(error) 会给
// **每一次成功写入**记一笔假错误。
//
// 下面这行在编译期断言 persistProjectFileUnsafe 的签名恒为
// `func(*Store, *model.ProjectFile) *transport.AppError`：任何人把它改回 error，本文件立刻
// **编译失败**（错误指向本行），CI 直接变红，无需依赖任何运行时用例。
var _ func(*Store, *model.ProjectFile) *transport.AppError = (*Store).persistProjectFileUnsafe

// ─── 脚手架 ───

// seedProjectFileForFailureTest 直接往内存摆一条项目文件（绕开 Mongo），
// 这样「失败零副作用」断言才有个干净的基线可比。
func seedProjectFileForFailureTest(t *testing.T, s *Store, id, projectID string) *model.ProjectFile {
	t.Helper()
	pf := &model.ProjectFile{
		ID:        id,
		ProjectID: projectID,
		FileName:  "seed-" + id + ".txt",
		FileSize:  8,
		MimeType:  "text/plain",
		Source:    "user_upload",
		Version:   projectFileVersionFloor,
		CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projectFiles[pf.ID] = pf
	s.projectFileIndex[projectID] = append(s.projectFileIndex[projectID], pf.ID)
	return pf
}

// projectFileMemorySnapshot 是三张项目文件索引的深拷贝快照，
// 用于「失败零副作用」的逐字节对比。
type projectFileMemorySnapshot struct {
	files        map[string]model.ProjectFile
	projectIndex map[string][]string
	transferIdx  map[string]string
}

// snapshotProjectFileMemory 在 RLock 下取三张索引的深拷贝，避免断言本身产生数据竞争。
func snapshotProjectFileMemory(t *testing.T, s *Store) projectFileMemorySnapshot {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := projectFileMemorySnapshot{
		files:        make(map[string]model.ProjectFile, len(s.projectFiles)),
		projectIndex: make(map[string][]string, len(s.projectFileIndex)),
		transferIdx:  make(map[string]string, len(s.transferFileIndex)),
	}
	for k, v := range s.projectFiles {
		if v != nil {
			snap.files[k] = *v
		}
	}
	for k, v := range s.projectFileIndex {
		snap.projectIndex[k] = append([]string(nil), v...)
	}
	for k, v := range s.transferFileIndex {
		snap.transferIdx[k] = v
	}
	return snap
}

// readProjectFileSnapshot 在 RLock 下取一条项目文件的值拷贝。
func readProjectFileSnapshot(t *testing.T, s *Store, id string) model.ProjectFile {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	pf, ok := s.projectFiles[id]
	if !ok || pf == nil {
		t.Fatalf("project file %q missing from memory", id)
	}
	return *pf
}

// newLiveProjectFileMongoStore 连接 TRUSTMESH_TEST_MONGO_URI 指向的 MongoDB（本地测试库），
// 并为每个用例清一次 project_files 集合，保证用例之间互不干扰。
// 环境变量未设置 / Mongo 不可达时跳过。
func newLiveProjectFileMongoStore(t *testing.T) *Store {
	t.Helper()
	uri := os.Getenv("TRUSTMESH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("skip: set TRUSTMESH_TEST_MONGO_URI to a reachable MongoDB to enable this test")
	}
	db := os.Getenv("TRUSTMESH_TEST_MONGO_DB")
	if db == "" {
		db = "trustmesh_t23b_test"
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
	if _, err := s.mongoProjectFiles.DeleteMany(ctx, bson.D{}); err != nil {
		t.Fatalf("clean project_files collection: %v", err)
	}
	// enableMongo 会先把库里所有项目文件载入内存，随后才轮到上面的清库；
	// 不清内存就会残留上一个用例写下的记录，让断言失真。
	s.mu.Lock()
	s.projectFiles = make(map[string]*model.ProjectFile)
	s.projectFileIndex = make(map[string][]string)
	s.transferFileIndex = make(map[string]string)
	s.mu.Unlock()
	return s
}

// bumpProjectFileVersionInMongo 绕过内存、直接把库内 version 推到指定值，
// 模拟「另一个写方已经改过这条项目文件」—— 这是制造版本冲突的唯一真实手段。
func bumpProjectFileVersionInMongo(t *testing.T, s *Store, fileID string, to int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.mongoProjectFiles.UpdateOne(ctx, bson.M{"_id": fileID}, bson.M{"$set": bson.M{"version": to}}); err != nil {
		t.Fatalf("bump project file version to %d: %v", to, err)
	}
}

// ─── 1. Mongo 失败零副作用（新增路径：三张索引全量回滚，不留幽灵） ───

func TestSaveProjectFileMongoFailureZeroSideEffect(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedProjectForFailureTest(t, s, "pf-zse")
	// 带 TransferID：把 transferFileIndex 的回滚也一并覆盖。
	before := snapshotProjectFileMemory(t, s)

	s.persistFailForTest = true
	defer func() { s.persistFailForTest = false }()

	pf, appErr := s.SaveProjectFile(Scope{UserID: "u1"}, "pf-zse", &model.ProjectFile{
		FileName:   "upload.txt",
		FileSize:   10,
		MimeType:   "text/plain",
		Source:     "user_upload",
		TransferID: "tr-zse",
	})
	if appErr == nil {
		t.Fatal("SaveProjectFile must fail when the persist seam is armed")
	}
	if pf != nil {
		t.Fatalf("failed SaveProjectFile returned a non-nil file: %+v", pf)
	}

	after := snapshotProjectFileMemory(t, s)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("memory (projectFiles/projectFileIndex/transferFileIndex) diverged after failed SaveProjectFile")
	}
}

// TestSaveProjectFileLeavesNoGhostOnPersistFailure 幽灵护栏：失败后内存中不存在该记录，
// ListProjectFiles / Browse 不得列出它。
func TestSaveProjectFileLeavesNoGhostOnPersistFailure(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedProjectForFailureTest(t, s, "pf-ghost")

	s.persistFailForTest = true
	_, appErr := s.SaveProjectFile(Scope{UserID: "u1"}, "pf-ghost", &model.ProjectFile{
		FileName: "ghost.txt", FileSize: 1, MimeType: "text/plain", Source: "user_upload",
	})
	if appErr == nil {
		t.Fatal("SaveProjectFile must fail when the persist seam is armed")
	}

	s.mu.RLock()
	count := len(s.projectFiles)
	idxLen := len(s.projectFileIndex["pf-ghost"])
	s.mu.RUnlock()
	if count != 0 {
		t.Fatalf("ghost project file left in memory after failed persist: %d entries", count)
	}
	if idxLen != 0 {
		t.Fatalf("ghost entry left in projectFileIndex after failed persist: %d entries", idxLen)
	}
	if got := s.ListProjectFiles(Scope{UserID: "u1"}, "pf-ghost", "", "", ""); len(got) != 0 {
		t.Fatalf("ListProjectFiles surfaced %d ghost file(s)", len(got))
	}
}

// TestSaveProjectFileRollbackRestoresForeignTransferMapping —— QA 路由回归护栏：
// 新增记录的 TransferID 与既有记录冲突 + persist 失败时，回滚必须把 transferFileIndex
// 还原给既有记录（不得删键、也不得指向新记录）。修复前的缺陷：插入路径无条件改写映射，
// 回滚的「映射指向本记录」恒等守卫因此恒通过 → 删键 → 无辜既有记录的映射被抹掉。
func TestSaveProjectFileRollbackRestoresForeignTransferMapping(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedProjectForFailureTest(t, s, "pf-tf")

	// 既有记录及其 transfer 映射（冲突场景下的无辜第三方）。
	owner := seedProjectFileForFailureTest(t, s, "pf-owner", "pf-tf")
	owner.TransferID = "tr-foreign"
	s.mu.Lock()
	s.transferFileIndex["tr-foreign"] = owner.ID
	s.mu.Unlock()

	s.persistFailForTest = true
	defer func() { s.persistFailForTest = false }()

	newFile, appErr := s.SaveProjectFile(Scope{UserID: "u1"}, "pf-tf", &model.ProjectFile{
		FileName:   "collide.txt",
		FileSize:   1,
		MimeType:   "text/plain",
		Source:     "user_upload",
		TransferID: "tr-foreign",
	})
	if appErr == nil {
		t.Fatal("SaveProjectFile must fail when the persist seam is armed")
	}
	if newFile != nil {
		t.Fatalf("failed SaveProjectFile returned a non-nil file: %+v", newFile)
	}

	// 无幽灵：内存里只剩既有的 pf-owner。
	s.mu.RLock()
	records := len(s.projectFiles)
	mapping, hasMapping := s.transferFileIndex["tr-foreign"]
	s.mu.RUnlock()

	if records != 1 {
		t.Fatalf("ghost record(s) left after rollback: %d entries, want 1 (pf-owner)", records)
	}
	if !hasMapping {
		t.Fatal("foreign transfer mapping was deleted by rollback, want restored to pf-owner")
	}
	if mapping != owner.ID {
		t.Fatalf("foreign transfer mapping = %q, want %q (pf-owner)", mapping, owner.ID)
	}
	if got := readProjectFileSnapshot(t, s, "pf-owner"); got.TransferID != "tr-foreign" {
		t.Fatalf("foreign record mutated by the collided save: TransferID=%q", got.TransferID)
	}
}

func TestCreateFolderLeavesNoGhostOnPersistFailure(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedProjectForFailureTest(t, s, "pf-folder")

	s.persistFailForTest = true
	folder, appErr := s.CreateFolder(Scope{UserID: "u1"}, "pf-folder", "新建文件夹", "")
	if appErr == nil {
		t.Fatal("CreateFolder must fail when the persist seam is armed")
	}
	if folder != nil {
		t.Fatalf("failed CreateFolder returned a non-nil folder: %+v", folder)
	}

	s.mu.RLock()
	count := len(s.projectFiles)
	idxLen := len(s.projectFileIndex["pf-folder"])
	s.mu.RUnlock()
	if count != 0 {
		t.Fatalf("ghost folder left in memory after failed persist: %d entries", count)
	}
	if idxLen != 0 {
		t.Fatalf("ghost entry left in projectFileIndex after failed persist: %d entries", idxLen)
	}
}

// TestCreateMeetingMinutesFileLeavesNoGhostOnPersistFailure 覆盖 webhook 链路的新增路径。
func TestCreateMeetingMinutesFileLeavesNoGhostOnPersistFailure(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedProjectForFailureTest(t, s, "pf-minutes")
	s.mu.Lock()
	s.meetings["mt-minutes"] = &model.Meeting{ID: "mt-minutes", ProjectID: "pf-minutes", CreatorID: "u1"}
	s.mu.Unlock()

	s.persistFailForTest = true
	pf, appErr := s.CreateMeetingMinutesFile("mt-minutes", "纪要.md", "# 纪要")
	if appErr == nil {
		t.Fatal("CreateMeetingMinutesFile must fail when the persist seam is armed")
	}
	if pf != nil {
		t.Fatalf("failed CreateMeetingMinutesFile returned a non-nil file: %+v", pf)
	}

	s.mu.RLock()
	count := len(s.projectFiles)
	s.mu.RUnlock()
	if count != 0 {
		t.Fatalf("ghost meeting-minutes file left in memory after failed persist: %d entries", count)
	}
}

// ─── 2. Mongo 失败零副作用（就地更新路径：mutateProjectFileUnsafe 快照回滚） ───

func TestRenameProjectFileMongoFailureZeroSideEffect(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedProjectForFailureTest(t, s, "pf-zse")
	seedProjectFileForFailureTest(t, s, "pf-rename", "pf-zse")
	before := snapshotProjectFileMemory(t, s)

	s.persistFailForTest = true
	defer func() { s.persistFailForTest = false }()

	got, appErr := s.RenameProjectFile(Scope{UserID: "u1"}, "pf-zse", "pf-rename", "renamed.txt")
	if appErr == nil {
		t.Fatal("RenameProjectFile must fail when the persist seam is armed")
	}
	if got != nil {
		t.Fatalf("failed RenameProjectFile returned a non-nil file: %+v", got)
	}

	after := snapshotProjectFileMemory(t, s)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("memory diverged after failed RenameProjectFile")
	}
	if pf := readProjectFileSnapshot(t, s, "pf-rename"); pf.FileName != "seed-pf-rename.txt" {
		t.Fatalf("file name mutated on failed persist: %q, want unchanged", pf.FileName)
	}
}

func TestMoveProjectFileMongoFailureZeroSideEffect(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedProjectForFailureTest(t, s, "pf-zse")
	seedProjectFileForFailureTest(t, s, "pf-move", "pf-zse")
	// 目标文件夹。
	s.mu.Lock()
	s.projectFiles["pf-target"] = &model.ProjectFile{
		ID: "pf-target", ProjectID: "pf-zse", FileName: "目标文件夹",
		Source: "user_upload", IsFolder: true, Version: projectFileVersionFloor, CreatedAt: time.Now().UTC(),
	}
	s.projectFileIndex["pf-zse"] = append(s.projectFileIndex["pf-zse"], "pf-target")
	s.mu.Unlock()
	before := snapshotProjectFileMemory(t, s)

	s.persistFailForTest = true
	defer func() { s.persistFailForTest = false }()

	got, appErr := s.MoveProjectFile(Scope{UserID: "u1"}, "pf-zse", "pf-move", "pf-target")
	if appErr == nil {
		t.Fatal("MoveProjectFile must fail when the persist seam is armed")
	}
	if got != nil {
		t.Fatalf("failed MoveProjectFile returned a non-nil file: %+v", got)
	}

	after := snapshotProjectFileMemory(t, s)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("memory diverged after failed MoveProjectFile")
	}
	if pf := readProjectFileSnapshot(t, s, "pf-move"); pf.ParentID != "" {
		t.Fatalf("parent mutated on failed persist: %q, want empty", pf.ParentID)
	}
}

// ─── 3. typed-nil 回归护栏（无 Mongo，CI 必跑） ───

func TestPersistProjectFileUnsafeSuccessReturnsNilError(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	seedProjectFileForFailureTest(t, s, "pf-typednil", "p-x")

	s.mu.Lock()
	pf := s.projectFiles["pf-typednil"]
	appErr := s.persistProjectFileUnsafe(pf)
	s.mu.Unlock()

	if appErr != nil {
		t.Fatalf("successful persistProjectFileUnsafe must return a nil error, got %#v", appErr)
	}
}

// ─── 4. 存量 version 归一化（纯函数，无需 Mongo） ───

func TestNormalizeProjectFileVersionBackfillsLegacyZero(t *testing.T) {
	// 存量项目文件文档（T2.3b 之前写入）没有 version 字段，解码后为 0。
	legacy := &model.ProjectFile{ID: "pf-legacy"}
	normalizeProjectFileVersion(legacy)
	if legacy.Version != projectFileVersionFloor {
		t.Fatalf("legacy version = %d, want %d", legacy.Version, projectFileVersionFloor)
	}

	// 幂等：已有版本不得被覆盖。
	legacy.Version = 7
	normalizeProjectFileVersion(legacy)
	if legacy.Version != 7 {
		t.Fatalf("existing version overwritten: %d, want 7", legacy.Version)
	}

	// 显式非法值（0 / 负数）也要一并归一。
	for _, bad := range []int{0, -3} {
		pf := &model.ProjectFile{ID: "pf-bad", Version: bad}
		normalizeProjectFileVersion(pf)
		if pf.Version != projectFileVersionFloor {
			t.Fatalf("version %d not normalized: got %d", bad, pf.Version)
		}
	}

	// nil 安全（解码路径可能拿到 nil）。
	normalizeProjectFileVersion(nil)
}

// ─── 5. 需要真 Mongo 的用例 ───

// TestProjectFileVersionConflictOnStaleRename 乐观锁：内存版本陈旧时，
// 改名必须返回 409 PROJECT_FILE_VERSION_CONFLICT（而不是静默覆盖别人的写入）。
func TestProjectFileVersionConflictOnStaleRename(t *testing.T) {
	s := newLiveProjectFileMongoStore(t)
	seedProjectForFailureTest(t, s, "pf-live-proj")
	seedProjectFileForFailureTest(t, s, "pf-conf", "pf-live-proj")

	// 首次落库：把内存与库内版本一起推进一步（1 -> 2）。
	s.mu.Lock()
	pf := s.projectFiles["pf-conf"]
	appErr := s.persistProjectFileUnsafe(pf)
	s.mu.Unlock()
	if appErr != nil {
		t.Fatalf("initial persist: %v", appErr)
	}
	after := readProjectFileSnapshot(t, s, "pf-conf")
	if after.Version != projectFileVersionFloor+1 {
		t.Fatalf("version after first write = %d, want %d", after.Version, projectFileVersionFloor+1)
	}

	// 另一个写方先把库内版本推到更后面（内存仍旧落后）。
	bumpProjectFileVersionInMongo(t, s, "pf-conf", after.Version+5)

	_, appErr = s.RenameProjectFile(Scope{UserID: "u1"}, "pf-live-proj", "pf-conf", "conflict.txt")
	if appErr == nil {
		t.Fatal("stale version rename must return a conflict, not silently overwrite")
	}
	if appErr.Status != http.StatusConflict {
		t.Fatalf("conflict status = %d, want 409", appErr.Status)
	}
	if appErr.Code != projectFileVersionConflictCode {
		t.Fatalf("conflict code = %q, want %s", appErr.Code, projectFileVersionConflictCode)
	}

	got := readProjectFileSnapshot(t, s, "pf-conf")
	if got.FileName != after.FileName {
		t.Fatalf("memory file name mutated by conflicted write: %q", got.FileName)
	}
	// W1 语义（refresh_on_conflict.go）：冲突后内存会被回源刷新为 Mongo 权威文档，
	// 版本应等于远端版本而非本地旧版本（同 task 域用例说明）。
	if want := after.Version + 5; got.Version != want {
		t.Fatalf("memory version after conflict = %d, want %d（应回源刷新为远端权威版本）", got.Version, want)
	}
}

// TestBackfillProjectFileVersionsRepairsLegacyZero 存量文档（无 version 字段 / 显式 0 /
// 负数）必须被回填成起始版本，否则带 {_id, version} filter 的更新永远失配 = 永久锁死。
// 回填后同一条记录必须**可写**（版本化替换成功）。
func TestBackfillProjectFileVersionsRepairsLegacyZero(t *testing.T) {
	s := newLiveProjectFileMongoStore(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 绕过结构体，直接插入「旧镜像」形态的文档：根本没有 version 字段。
	if _, err := s.mongoProjectFiles.InsertOne(ctx, bson.M{"_id": "pf-legacy", "project_id": "pf-live-proj", "file_name": "legacy.txt", "source": "user_upload"}); err != nil {
		t.Fatalf("insert legacy project file: %v", err)
	}
	// 显式 0 也要能被覆盖。
	if _, err := s.mongoProjectFiles.InsertOne(ctx, bson.M{"_id": "pf-zero", "project_id": "pf-live-proj", "file_name": "zero.txt", "source": "user_upload", "version": 0}); err != nil {
		t.Fatalf("insert zero-version project file: %v", err)
	}
	// 负数（非法历史取值）也要能被覆盖。
	if _, err := s.mongoProjectFiles.InsertOne(ctx, bson.M{"_id": "pf-neg", "project_id": "pf-live-proj", "file_name": "neg.txt", "source": "user_upload", "version": -2}); err != nil {
		t.Fatalf("insert negative-version project file: %v", err)
	}

	s.backfillProjectFileVersions()

	for _, id := range []string{"pf-legacy", "pf-zero", "pf-neg"} {
		var doc struct {
			Version int `bson:"version"`
		}
		if err := s.mongoProjectFiles.FindOne(ctx, bson.M{"_id": id}).Decode(&doc); err != nil {
			t.Fatalf("read back %s: %v", id, err)
		}
		if doc.Version != projectFileVersionFloor {
			t.Fatalf("%s version after backfill = %d, want %d", id, doc.Version, projectFileVersionFloor)
		}
	}

	// 回填后必须可写：把内存侧同 ID 记录（归一化到起始版本）做一次版本化替换 → 成功。
	seedProjectForFailureTest(t, s, "pf-live-proj")
	s.mu.Lock()
	s.projectFiles["pf-legacy"] = &model.ProjectFile{
		ID: "pf-legacy", ProjectID: "pf-live-proj", FileName: "legacy.txt",
		Source: "user_upload", Version: projectFileVersionFloor, CreatedAt: time.Now().UTC(),
	}
	writable := s.projectFiles["pf-legacy"]
	appErr := s.persistProjectFileUnsafe(writable)
	s.mu.Unlock()
	if appErr != nil {
		t.Fatalf("backfilled legacy doc must be writable: %v", appErr)
	}
	if got := readProjectFileSnapshot(t, s, "pf-legacy"); got.Version != projectFileVersionFloor+1 {
		t.Fatalf("version after first write = %d, want %d", got.Version, projectFileVersionFloor+1)
	}
}

// TestVerifyProjectFileConsistencyMismatchedZero 双写过渡期：内存与 Mongo 必须一致。
// 本用例是 LocalPath / CreatedAt「假 mismatch」陷阱的护栏 —— 若实现里误比了这两个字段，
// 本用例会因 mismatched > 0 立即变红。文件夹 + 子文件的组合同时覆盖 len(children) 比对。
func TestVerifyProjectFileConsistencyMismatchedZero(t *testing.T) {
	s := newLiveProjectFileMongoStore(t)
	seedProjectForFailureTest(t, s, "pf-live-proj")

	if _, appErr := s.SaveProjectFile(Scope{UserID: "u1"}, "pf-live-proj", &model.ProjectFile{
		FileName: "a.txt", FileSize: 1, MimeType: "text/plain", Source: "user_upload",
	}); appErr != nil {
		t.Fatalf("upload root file: %v", appErr)
	}
	folder, appErr := s.CreateFolder(Scope{UserID: "u1"}, "pf-live-proj", "资料", "")
	if appErr != nil {
		t.Fatalf("create folder: %v", appErr)
	}
	if _, appErr := s.SaveProjectFile(Scope{UserID: "u1"}, "pf-live-proj", &model.ProjectFile{
		FileName: "b.txt", FileSize: 2, MimeType: "text/plain", Source: "user_upload", ParentID: folder.ID,
	}); appErr != nil {
		t.Fatalf("upload into folder: %v", appErr)
	}

	checked, mismatched, err := s.VerifyProjectFileConsistency()
	if err != nil {
		t.Fatalf("VerifyProjectFileConsistency: %v", err)
	}
	if checked < 3 {
		t.Fatalf("checked = %d, want >= 3", checked)
	}
	if mismatched != 0 {
		t.Fatalf("mismatched = %d, want 0 (memory diverged from Mongo)", mismatched)
	}
}

// TestProjectFileDeletedRowsDoNotResurrectOnRehydrate 删除不复活：删除成功后重新载入，
// 被删的行（含级联删除的子文件）**不再出现**。这是修复前 `_ = s.deleteProjectFileUnsafe(id)`
// fire-and-forget 最危险的后果。
func TestProjectFileDeletedRowsDoNotResurrectOnRehydrate(t *testing.T) {
	s := newLiveProjectFileMongoStore(t)
	seedProjectForFailureTest(t, s, "pf-live-proj")

	rootFile, appErr := s.SaveProjectFile(Scope{UserID: "u1"}, "pf-live-proj", &model.ProjectFile{
		FileName: "root.txt", FileSize: 1, MimeType: "text/plain", Source: "user_upload",
	})
	if appErr != nil {
		t.Fatalf("upload root file: %v", appErr)
	}
	folder, appErr := s.CreateFolder(Scope{UserID: "u1"}, "pf-live-proj", "待删文件夹", "")
	if appErr != nil {
		t.Fatalf("create folder: %v", appErr)
	}
	childFile, appErr := s.SaveProjectFile(Scope{UserID: "u1"}, "pf-live-proj", &model.ProjectFile{
		FileName: "child.txt", FileSize: 2, MimeType: "text/plain", Source: "user_upload", ParentID: folder.ID,
	})
	if appErr != nil {
		t.Fatalf("upload into folder: %v", appErr)
	}

	// 删除文件夹（级联删除 childFile）与根文件。
	if _, appErr := s.DeleteProjectFile(Scope{UserID: "u1"}, folder.ID); appErr != nil {
		t.Fatalf("delete folder: %v", appErr)
	}
	if _, appErr := s.DeleteProjectFile(Scope{UserID: "u1"}, rootFile.ID); appErr != nil {
		t.Fatalf("delete root file: %v", appErr)
	}

	// 模拟进程重启：从同一 Mongo 重新载入 project_files 集合。
	loaded, _, _, err := s.loadProjectFiles()
	if err != nil {
		t.Fatalf("loadProjectFiles after restart: %v", err)
	}
	for _, id := range []string{rootFile.ID, folder.ID, childFile.ID} {
		if _, exists := loaded[id]; exists {
			t.Fatalf("deleted project file %q resurrected after rehydrate", id)
		}
	}
}

// TestProjectFileRehydrateAfterWritePaths 重启零丢失：上传 / 建夹 / 改名 / 移动后
// 重新载入，记录（含 Version 与层级关系）必须全部可读回。
func TestProjectFileRehydrateAfterWritePaths(t *testing.T) {
	s := newLiveProjectFileMongoStore(t)
	seedProjectForFailureTest(t, s, "pf-live-proj")

	file, appErr := s.SaveProjectFile(Scope{UserID: "u1"}, "pf-live-proj", &model.ProjectFile{
		FileName: "rehydrate.txt", FileSize: 3, MimeType: "text/plain", Source: "user_upload",
	})
	if appErr != nil {
		t.Fatalf("upload file: %v", appErr)
	}
	folder, appErr := s.CreateFolder(Scope{UserID: "u1"}, "pf-live-proj", "归档", "")
	if appErr != nil {
		t.Fatalf("create folder: %v", appErr)
	}
	renamed, appErr := s.RenameProjectFile(Scope{UserID: "u1"}, "pf-live-proj", file.ID, "rehydrate-2.txt")
	if appErr != nil {
		t.Fatalf("rename file: %v", appErr)
	}
	if renamed.FileName != "rehydrate-2.txt" {
		t.Fatalf("rename did not take effect in memory: %q", renamed.FileName)
	}
	moved, appErr := s.MoveProjectFile(Scope{UserID: "u1"}, "pf-live-proj", renamed.ID, folder.ID)
	if appErr != nil {
		t.Fatalf("move file: %v", appErr)
	}
	if moved.ParentID != folder.ID {
		t.Fatalf("move did not take effect in memory: %q", moved.ParentID)
	}

	// 模拟进程重启：从同一 Mongo 重新载入 project_files 集合。
	loaded, index, _, err := s.loadProjectFiles()
	if err != nil {
		t.Fatalf("loadProjectFiles after restart: %v", err)
	}
	got, ok := loaded[file.ID]
	if !ok || got == nil {
		t.Fatal("uploaded file missing after reload from Mongo")
	}
	if got.FileName != "rehydrate-2.txt" {
		t.Fatalf("rename lost across restart: %q, want rehydrate-2.txt", got.FileName)
	}
	if got.ParentID != folder.ID {
		t.Fatalf("move lost across restart: parent=%q, want %q", got.ParentID, folder.ID)
	}
	if got.Version < projectFileVersionFloor {
		t.Fatalf("version lost across restart: %d, want >= %d", got.Version, projectFileVersionFloor)
	}
	if _, ok := loaded[folder.ID]; !ok {
		t.Fatal("folder missing after reload from Mongo")
	}

	idx := index["pf-live-proj"]
	foundFile, foundFolder := false, false
	for _, id := range idx {
		if id == file.ID {
			foundFile = true
		}
		if id == folder.ID {
			foundFolder = true
		}
	}
	if !foundFile || !foundFolder {
		t.Fatalf("projectFileIndex lost across restart: file=%v folder=%v", foundFile, foundFolder)
	}
}
