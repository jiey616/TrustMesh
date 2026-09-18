package store

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/project"
)

// ────────────────────────────────────────────────────────────────────────────
// 桌面端发行版（docs/desktop-app-update-plan-2026-09-18.md）：
//
//   1. 纯函数层（无 Mongo）：版本号 / 文件名 / sha512 三个入参闸门 ——
//      它们是「坏包不许进库」的全部防线，必须逐条钉住。
//   2. 活库层（TRUSTMESH_TEST_MONGO_URI 门控，未设置时 t.Skip）：
//      发布指针的前移/后移、单条 published 的索引兜底、删除守卫、保留上限清理。
// ────────────────────────────────────────────────────────────────────────────

func TestDesktopReleaseVersionGate(t *testing.T) {
	valid := []string{"0.1.0", "1.2.3", "0.3.0-beta.1", "1.2.3+build.5", "0.3.0-rc.1+build.9"}
	for _, v := range valid {
		if !desktopReleaseVersionRe.MatchString(v) {
			t.Errorf("%q should be accepted as semver", v)
		}
	}
	// 拒绝集里每一条都能导致**静默无更新**（latest.yml 的 version 与 package.json
	// 逐字比较），所以「看起来差不多」的写法必须一律挡掉。
	invalid := []string{"", "v0.3.0", "0.3", "0.3.0.1", "0.3.0 ", " 0.3.0", "latest", "0.3.0-", "1.0.0-"}
	for _, v := range invalid {
		if desktopReleaseVersionRe.MatchString(v) {
			t.Errorf("%q should be rejected as non-semver", v)
		}
	}
}

func TestValidateDesktopReleaseFileName(t *testing.T) {
	ok := []string{"TrustMesh-Setup-0.3.0.exe", "TrustMesh-Setup-0.3.0.exe.blockmap", "a-0.1.0-rc.1.exe"}
	for _, n := range ok {
		if appErr := validateDesktopReleaseFileName(n); appErr != nil {
			t.Errorf("%q should be accepted, got %v", n, appErr)
		}
	}
	// 拒绝集：路径穿越（纵深防御）、URL 需转义字符（feed 把文件名放进 URL 路径段）、
	// 引号（latest.yml 是手写 YAML，见 handler.RenderLatestYML 的安全前提）。
	bad := []string{
		"", ".", "..", "../evil.exe", "a/b.exe", `a\b.exe`, "TrustMesh Setup 0.3.0.exe",
		"中文.exe", `bad"quote.exe`, "bad'quote.exe", "a?b.exe", "a#b.exe", "a%b.exe", "a&b.exe", "a=b.exe",
	}
	for _, n := range bad {
		if appErr := validateDesktopReleaseFileName(n); appErr == nil {
			t.Errorf("%q must be rejected as an unsafe file name", n)
		}
	}
}

func TestValidDesktopReleaseSha512(t *testing.T) {
	payload := []byte("trustmesh")
	sum := sha512.Sum512(payload)
	if !validDesktopReleaseSha512(base64.StdEncoding.EncodeToString(sum[:])) {
		t.Error("base64-encoded sha512 must be accepted")
	}
	bad := []string{
		"",
		"deadbeef", // 不是 base64
		base64.StdEncoding.EncodeToString(payload),          // 长度不对（不是 64 字节摘要）
		"!" + base64.StdEncoding.EncodeToString(sum[:])[1:], // 非法字符
		base64.StdEncoding.EncodeToString(sum[:]) + "AAAA",  // 超长
	}
	for _, v := range bad {
		if validDesktopReleaseSha512(v) {
			t.Errorf("%q must be rejected as an invalid sha512", v)
		}
	}
}

// ─── 活库：发布指针 / 索引兜底 / 删除守卫 / 保留上限 ───

func newLiveDesktopReleaseStore(t *testing.T) *Store {
	t.Helper()
	uri := os.Getenv("TRUSTMESH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("skip: set TRUSTMESH_TEST_MONGO_URI to a reachable MongoDB to enable this test")
	}
	db := os.Getenv("TRUSTMESH_TEST_MONGO_DB")
	if db == "" {
		db = "trustmesh_desktop_release_test"
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
	if _, err := s.mongoDesktopReleases.DeleteMany(ctx, bson.D{}); err != nil {
		t.Fatalf("clean desktop_releases collection: %v", err)
	}
	// 磁盘落盘用临时目录：不与真实 FILES_STORAGE_PATH 混在一起。
	s.SetFileStorage(project.NewLocalFileStorage(t.TempDir()))
	return s
}

func desktopReleaseTestFileName(version string) string {
	return "TrustMesh-Setup-" + version + ".exe"
}

func desktopReleaseTestSha512(payload []byte) string {
	sum := sha512.Sum512(payload)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func mustCreateDesktopRelease(t *testing.T, s *Store, version string, payload []byte) *model.DesktopRelease {
	t.Helper()
	rel, appErr := s.CreateDesktopRelease(model.DesktopReleaseInput{
		Version:    version,
		FileName:   desktopReleaseTestFileName(version),
		Size:       int64(len(payload)),
		Sha512:     desktopReleaseTestSha512(payload),
		UploadedBy: "u-platform",
	}, bytes.NewReader(payload))
	if appErr != nil {
		t.Fatalf("create desktop release %s: %v", version, appErr)
	}
	return rel
}

func TestDesktopReleaseIntegrityGatesLiveMongo(t *testing.T) {
	s := newLiveDesktopReleaseStore(t)
	payload := []byte("trustmesh-installer-payload")

	// sha512 不符 → 422，且**不得落库**（坏包进库 = 全体客户端都装不上）。
	_, appErr := s.CreateDesktopRelease(model.DesktopReleaseInput{
		Version:  "0.2.0",
		FileName: desktopReleaseTestFileName("0.2.0"),
		Size:     int64(len(payload)),
		Sha512:   desktopReleaseTestSha512([]byte("different payload")),
	}, bytes.NewReader(payload))
	if appErr == nil || appErr.Status != 422 {
		t.Fatalf("sha512 mismatch must be 422, got %v", appErr)
	}

	// 自报 size 与磁盘实际大小不符 → 422（截断上传的典型症状）。
	_, appErr = s.CreateDesktopRelease(model.DesktopReleaseInput{
		Version:  "0.2.0",
		FileName: desktopReleaseTestFileName("0.2.0"),
		Size:     int64(len(payload)) + 10,
		Sha512:   desktopReleaseTestSha512(payload),
	}, bytes.NewReader(payload))
	if appErr == nil || appErr.Status != 422 {
		t.Fatalf("size mismatch must be 422, got %v", appErr)
	}

	// 两次 422 之后库里必须是空的（失败上传不留残渣）。
	items, appErr := s.ListDesktopReleases()
	if appErr != nil {
		t.Fatalf("list: %v", appErr)
	}
	if len(items) != 0 {
		t.Fatalf("rejected uploads must not leave records, got %d", len(items))
	}

	// 正常上传成功，且默认状态是 draft（不自动发布）。
	rel := mustCreateDesktopRelease(t, s, "0.2.0", payload)
	if rel.Status != model.DesktopReleaseStatusDraft {
		t.Fatalf("new release status = %q, want draft", rel.Status)
	}
	if rel.Size != int64(len(payload)) {
		t.Fatalf("stored size = %d, want %d (must come from disk, not from the request)", rel.Size, len(payload))
	}

	// 重复版本 → 409（latest.yml 指向哪个版本必须有确定答案）。
	_, appErr = s.CreateDesktopRelease(model.DesktopReleaseInput{
		Version:  "0.2.0",
		FileName: desktopReleaseTestFileName("0.2.0"),
		Size:     int64(len(payload)),
		Sha512:   desktopReleaseTestSha512(payload),
	}, bytes.NewReader(payload))
	if appErr == nil || appErr.Status != 409 {
		t.Fatalf("duplicate version must be 409, got %v", appErr)
	}
}

func TestDesktopReleasePublishPointerLiveMongo(t *testing.T) {
	s := newLiveDesktopReleaseStore(t)

	// 空库：feed 必须 NotFound —— 更新端把 404 当「本次无更新」，而不是报错。
	if _, appErr := s.FeedDesktopRelease(); appErr == nil || appErr.Status != 404 {
		t.Fatalf("empty store feed must be 404, got %v", appErr)
	}

	rel1 := mustCreateDesktopRelease(t, s, "0.2.0", []byte("v020"))
	// draft 不进 feed：上传 ≠ 发布。
	if _, appErr := s.FeedDesktopRelease(); appErr == nil || appErr.Status != 404 {
		t.Fatalf("draft must not be served, got %v", appErr)
	}

	published1, appErr := s.PublishDesktopRelease(rel1.ID)
	if appErr != nil {
		t.Fatalf("publish 0.2.0: %v", appErr)
	}
	feed, appErr := s.FeedDesktopRelease()
	if appErr != nil {
		t.Fatalf("feed after publish: %v", appErr)
	}
	if feed.Version != "0.2.0" || feed.FileName != "TrustMesh-Setup-0.2.0.exe" {
		t.Fatalf("feed = %+v, want 0.2.0", feed)
	}
	if feed.ReleaseDate.IsZero() {
		t.Fatal("feed releaseDate must not be zero (electron-updater uses it as the release timestamp)")
	}
	if published1.PublishedAt.IsZero() {
		t.Fatal("publish must stamp published_at")
	}

	rel2 := mustCreateDesktopRelease(t, s, "0.3.0", []byte("v030"))
	if _, appErr := s.PublishDesktopRelease(rel2.ID); appErr != nil {
		t.Fatalf("publish 0.3.0: %v", appErr)
	}
	assertPublishedVersion(t, s, "0.3.0")
	assertReleaseStatus(t, s, rel1.ID, model.DesktopReleaseStatusArchived)

	// 回滚 = 把指针指回去（方案 §1 决策 5）。老版本重新成为 published，
	// 新的那个退回 archived —— 已升级的机器不受影响，未升级的拿到旧版。
	if _, appErr := s.RollbackDesktopRelease(rel1.ID); appErr != nil {
		t.Fatalf("rollback to 0.2.0: %v", appErr)
	}
	assertPublishedVersion(t, s, "0.2.0")
	assertReleaseStatus(t, s, rel2.ID, model.DesktopReleaseStatusArchived)

	// 删除守卫：published 不可删（删了所有未升级客户端就只剩 404，且没有替代版本）。
	if appErr := s.DeleteDesktopRelease(rel1.ID); appErr == nil || appErr.Status != 409 {
		t.Fatalf("deleting the published release must be 409, got %v", appErr)
	}
	// 非 published 可删，且磁盘文件一并回收。
	if appErr := s.DeleteDesktopRelease(rel2.ID); appErr != nil {
		t.Fatalf("delete archived release: %v", appErr)
	}
	if _, err := os.Stat(rel2.FilePath); !os.IsNotExist(err) {
		t.Fatalf("deleted release file still on disk: %v", err)
	}
}

// TestDesktopReleaseSinglePublishedIndexLiveMongo 直接打 Mongo，验证「全局至多一条
// published」是**索引级**约束而不是应用层自律：绕过 store 的「先归档再发布」写序，
// 硬插第二条 published 必须被 E11000 拒绝。
func TestDesktopReleaseSinglePublishedIndexLiveMongo(t *testing.T) {
	s := newLiveDesktopReleaseStore(t)
	rel1 := mustCreateDesktopRelease(t, s, "0.2.0", []byte("v020"))
	if _, appErr := s.PublishDesktopRelease(rel1.ID); appErr != nil {
		t.Fatalf("publish: %v", appErr)
	}

	forged := &model.DesktopRelease{
		ID:        "forged-desktop-release",
		Version:   "9.9.9",
		Channel:   model.DesktopReleaseChannelStable,
		FileName:  "TrustMesh-Setup-9.9.9.exe",
		Status:    model.DesktopReleaseStatusPublished,
		CreatedAt: time.Now().UTC(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := s.mongoDesktopReleases.InsertOne(ctx, forged)
	if err == nil {
		t.Fatal("second published release must be rejected by the partial unique index")
	}
	if !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("want duplicate-key error, got %v", err)
	}
}

func TestDesktopReleasePruneLiveMongo(t *testing.T) {
	s := newLiveDesktopReleaseStore(t)
	versions := []string{"0.1.0", "0.2.0", "0.3.0", "0.4.0", "0.5.0", "0.6.0", "0.7.0"}
	for _, v := range versions {
		rel := mustCreateDesktopRelease(t, s, v, []byte("payload-"+v))
		if _, appErr := s.PublishDesktopRelease(rel.ID); appErr != nil {
			t.Fatalf("publish %s: %v", v, appErr)
		}
		// 拉开创建时间，让「最旧优先」有确定答案（同毫秒内排序不可依赖）。
		time.Sleep(2 * time.Millisecond)
	}

	pruned := s.PruneDesktopReleases()
	if len(pruned) != len(versions)-model.DesktopReleaseRetain {
		t.Fatalf("pruned %v, want %d entries cut down to %d",
			pruned, len(versions)-model.DesktopReleaseRetain, model.DesktopReleaseRetain)
	}
	items, appErr := s.ListDesktopReleases()
	if appErr != nil {
		t.Fatalf("list: %v", appErr)
	}
	if len(items) != model.DesktopReleaseRetain {
		t.Fatalf("retained %d releases, want %d", len(items), model.DesktopReleaseRetain)
	}
	// 当前 published 绝不能被清理（清理只从 archived 里挑）。
	assertPublishedVersion(t, s, "0.7.0")
	if !strings.Contains(strings.Join(pruned, ","), "0.1.0") {
		t.Fatalf("prune must start from the oldest archived, pruned = %v", pruned)
	}
}

func assertPublishedVersion(t *testing.T, s *Store, want string) {
	t.Helper()
	feed, appErr := s.FeedDesktopRelease()
	if appErr != nil {
		t.Fatalf("feed: %v", appErr)
	}
	if feed.Version != want {
		t.Fatalf("published version = %q, want %q", feed.Version, want)
	}
}

func assertReleaseStatus(t *testing.T, s *Store, id, want string) {
	t.Helper()
	rel, appErr := s.GetDesktopRelease(id)
	if appErr != nil {
		t.Fatalf("get %s: %v", id, appErr)
	}
	if rel.Status != want {
		t.Fatalf("release %s status = %q, want %q", id, rel.Status, want)
	}
}
