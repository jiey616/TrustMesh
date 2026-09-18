package store

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/project"
	"trustmesh/backend/internal/transport"
)

// ────────────────────────────────────────────────────────────────────────────
// 桌面端发行版（自建更新源）：desktop_releases 集合。
//
// 与审计日志一样**不进全内存状态机**：平台级低频管理数据，Mongo 即权威，
// 无需 FlushPersistAll 镜像（避免陈旧内存快照整文档覆盖 Mongo 的那类事故）。
//
// 「同一时刻至多一条 published」由 Mongo 部分唯一索引兜底
// （见 mongo_state.go 的 ensureMongoIndexes），应用层的先归档后发布只是
// 为了让冲突不上升到 500。
//
// 方案全文：docs/desktop-app-update-plan-2026-09-18.md
// ────────────────────────────────────────────────────────────────────────────

const (
	// desktopReleaseStoragePartition 是 LocalFileStorage 的 projectID 分区名：
	// 磁盘落点 {FILES_STORAGE_PATH}/desktop-releases/uploads/{releaseID}_{fileName}。
	// 用 releaseID 前缀而非版本号，保证同版本被删除后重新上传不会撞旧文件。
	desktopReleaseStoragePartition = "desktop-releases"
)

// desktopReleaseVersionRe 是宽松 semver（允许 -prerelease / +build 后缀）。
// 收紧到 semver 是必要的：latest.yml 的 version 与 package.json 逐字比较，
// 任何非 semver 值都会让更新端静默判定「无更新」。
var desktopReleaseVersionRe = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.\-]+)?(?:\+[0-9A-Za-z.\-]+)?$`)

// requireDesktopReleaseMongo 在 Mongo 不可用时给出明确失败，而不是「静默成功但没落库」。
// 发行版没有内存兜底（与审计的内存环不同：审计丢了只是少证据，发行版丢了就是掉版本）。
func (s *Store) requireDesktopReleaseMongo() *transport.AppError {
	if s == nil || !s.mongoEnabled || s.mongoDesktopReleases == nil {
		return transport.NewError(500, "INTERNAL_ERROR", "desktop release store requires mongo")
	}
	return nil
}

// desktopReleaseFileStorage 返回项目文件存储句柄。桌面发行版复用同一个 root
// （仅分区名不同），因此不新写存储层；返回 nil 表示未接线。
// 🔴 必须在 s.mu 之外调用返回值的任何方法（文件 IO 不得持锁）。
func (s *Store) desktopReleaseFileStorage() project.FileStorage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fileStorage
}

// validateDesktopReleaseFileName 拒绝任何带路径语义的文件名。
// 虽然下载端点按**库字段**查记录（不拼路径），这里仍做纵深防御：
// 一旦将来有人改成「按文件名拼路径」，这个名字就是现成的目录穿越载荷。
func validateDesktopReleaseFileName(name string) *transport.AppError {
	n := strings.TrimSpace(name)
	if n == "" || n == "." || n == ".." {
		return transport.Validation("invalid file name", map[string]any{"file_name": "required"})
	}
	if strings.ContainsAny(n, `/\`) || strings.ContainsRune(n, 0) {
		return transport.Validation("invalid file name", map[string]any{
			"file_name": "must not contain path separators",
		})
	}
	// 公开 feed 的下载路径会把文件名放进 URL 路径段（非 query），
	// 带空格/中文等需转义的字符会造成客户端 URL 拼装歧义 —— 打包侧
	// artifactName 已固定为 ASCII 连字符风格，这里只做守门。
	// 同时排除引号：latest.yml 是手写 YAML（无 yaml 依赖，见 handler 注释），
	// 文件名连引号都进不来才能让「全部值加双引号」成为充分安全的输出策略。
	for _, r := range n {
		if r < 0x20 || r > 0x7e {
			return transport.Validation("invalid file name", map[string]any{
				"file_name": "must be printable ascii (no spaces)",
			})
		}
	}
	if strings.ContainsAny(n, " ?#%&=\"'") {
		return transport.Validation("invalid file name", map[string]any{
			"file_name": "must not contain url-unsafe or quote characters",
		})
	}
	return nil
}

// validDesktopReleaseSha512 校验「base64 编码的 sha512」（electron-builder 的
// latest.yml 即此格式）——必须是可解码的 64 字节摘要，防止把 hex 摘要写进
// latest.yml（下载端会拿它做校验，格式错 = 每个客户端都装不上）。
func validDesktopReleaseSha512(v string) bool {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(v))
	return err == nil && len(raw) == sha512.Size
}

// DesktopReleaseFeed 是公开 feed 端点的投影（latest.yml 的直接数据源）。
type DesktopReleaseFeed struct {
	Version      string
	FileName     string
	Size         int64
	Sha512       string
	BlockMapSize int64
	ReleaseDate  time.Time
}

// FeedDesktopRelease 返回当前 published 版本的 feed 投影。
// 无 published（或库不可用）时返回 NotFound —— 更新端会把 404 当作
// 「本次没有更新」，下次启动再试，不会报错给用户。
func (s *Store) FeedDesktopRelease() (*DesktopReleaseFeed, *transport.AppError) {
	if appErr := s.requireDesktopReleaseMongo(); appErr != nil {
		return nil, appErr
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	var rel model.DesktopRelease
	err := s.mongoDesktopReleases.FindOne(ctx,
		bson.M{"status": model.DesktopReleaseStatusPublished}).Decode(&rel)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, transport.NotFound("no published desktop release")
		}
		return nil, mongoWriteError(err)
	}
	releaseDate := rel.PublishedAt
	if releaseDate.IsZero() {
		releaseDate = rel.CreatedAt
	}
	return &DesktopReleaseFeed{
		Version:      rel.Version,
		FileName:     rel.FileName,
		Size:         rel.Size,
		Sha512:       rel.Sha512,
		BlockMapSize: rel.BlockMapSize,
		ReleaseDate:  releaseDate,
	}, nil
}

// ListDesktopReleases 返回全部发行版，按创建时间倒序（最新在前）。
func (s *Store) ListDesktopReleases() ([]model.DesktopRelease, *transport.AppError) {
	if appErr := s.requireDesktopReleaseMongo(); appErr != nil {
		return nil, appErr
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cur, err := s.mongoDesktopReleases.Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, mongoWriteError(err)
	}
	defer cur.Close(ctx)
	out := make([]model.DesktopRelease, 0, model.DesktopReleaseRetain+1)
	if err := cur.All(ctx, &out); err != nil {
		return nil, mongoWriteError(err)
	}
	return out, nil
}

// GetDesktopRelease 按 ID 取单条。
func (s *Store) GetDesktopRelease(id string) (*model.DesktopRelease, *transport.AppError) {
	if appErr := s.requireDesktopReleaseMongo(); appErr != nil {
		return nil, appErr
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	var rel model.DesktopRelease
	if err := s.mongoDesktopReleases.FindOne(ctx, bson.M{"_id": id}).Decode(&rel); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, transport.NotFound("desktop release not found")
		}
		return nil, mongoWriteError(err)
	}
	return &rel, nil
}

// ResolveDesktopReleaseFilePath 把公开 feed 的文件名解析为磁盘绝对路径。
// 同时接受安装包名与 .blockmap 名（差分下载会直接来拉 blockmap）。
// 按**库字段**匹配，不做任何路径拼接 —— 天然免疫目录穿越。
func (s *Store) ResolveDesktopReleaseFilePath(name string) (string, *transport.AppError) {
	if appErr := s.requireDesktopReleaseMongo(); appErr != nil {
		return "", appErr
	}
	n := strings.TrimSpace(name)
	if n == "" || strings.ContainsAny(n, `/\`) {
		return "", transport.Validation("invalid file name", map[string]any{"filename": "invalid"})
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	var rel model.DesktopRelease
	err := s.mongoDesktopReleases.FindOne(ctx, bson.M{"$or": []bson.M{
		{"file_name": n},
		{"block_map_file_name": n},
	}}).Decode(&rel)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", transport.NotFound("desktop release file not found")
		}
		return "", mongoWriteError(err)
	}
	if rel.FileName == n {
		return rel.FilePath, nil
	}
	return rel.BlockMapPath, nil
}

// CreateDesktopRelease 落一条 draft 发行版：**先写盘、后写库**（写库失败则回收文件）。
//
// 两道完整性闸门，都是为了拦住「坏包发给全体客户端」：
//   - 落盘后按磁盘实际大小复核 release.json 自报的 size（截断上传的典型症状）；
//   - 落盘后重算 sha512 与 release.json 比对（静默损坏的典型症状）。
//
// 代价是上传时多一次顺序读（87MB 量级，单次操作可接受）。
func (s *Store) CreateDesktopRelease(in model.DesktopReleaseInput, file io.Reader) (*model.DesktopRelease, *transport.AppError) {
	if appErr := s.requireDesktopReleaseMongo(); appErr != nil {
		return nil, appErr
	}
	version := strings.TrimSpace(in.Version)
	if !desktopReleaseVersionRe.MatchString(version) {
		return nil, transport.Validation("invalid version", map[string]any{
			"version": "must be semver (e.g. 0.3.0)",
		})
	}
	channel := strings.TrimSpace(in.Channel)
	if channel == "" {
		channel = model.DesktopReleaseChannelStable
	}
	fileName := strings.TrimSpace(in.FileName)
	if fileName == "" {
		fileName = "TrustMesh-Setup-" + version + ".exe"
	}
	if appErr := validateDesktopReleaseFileName(fileName); appErr != nil {
		return nil, appErr
	}
	if strings.TrimSpace(in.Sha512) == "" || !validDesktopReleaseSha512(in.Sha512) {
		return nil, transport.Validation("invalid sha512", map[string]any{
			"sha512": "must be base64-encoded sha512 (64 bytes)",
		})
	}
	if !strings.Contains(fileName, ".exe") {
		return nil, transport.Validation("invalid file name", map[string]any{
			"file_name": "must end with .exe",
		})
	}
	if file == nil {
		return nil, transport.Validation("missing installer", map[string]any{"file": "required"})
	}

	// 版本唯一：应用层先给可读 409（唯一索引兜住并发漏判）。
	ctx, cancel := s.mongoContext()
	dup, err := s.mongoDesktopReleases.CountDocuments(ctx, bson.M{"version": version})
	cancel()
	if err != nil {
		return nil, mongoWriteError(err)
	}
	if dup > 0 {
		return nil, transport.Conflict("VERSION_EXISTS", "a release with this version already exists")
	}

	fs := s.desktopReleaseFileStorage()
	if fs == nil {
		return nil, transport.NewError(500, "INTERNAL_ERROR", "file storage not configured")
	}

	rel := &model.DesktopRelease{
		ID:         newID(),
		Version:    version,
		Channel:    channel,
		FileName:   fileName,
		Status:     model.DesktopReleaseStatusDraft,
		Notes:      strings.TrimSpace(in.Notes),
		UploadedBy: in.UploadedBy,
		CreatedAt:  time.Now().UTC(),
	}

	// 🔴 文件 IO 全程在锁外（fs 的方法不得在持 s.mu 时调用）。
	path, err := fs.Save(context.Background(), desktopReleaseStoragePartition, rel.ID, fileName, file)
	if err != nil {
		return nil, mongoWriteError(err)
	}
	if appErr := s.verifySavedInstaller(path, fileName, in.Size, in.Sha512); appErr != nil {
		if delErr := fs.Delete(context.Background(), path); delErr != nil && s.log != nil {
			s.log.Warn("discard rejected desktop release file failed",
				zap.String("path", path), zap.Error(delErr))
		}
		return nil, appErr
	}
	fi, statErr := os.Stat(path)
	if statErr != nil {
		_ = fs.Delete(context.Background(), path)
		return nil, mongoWriteError(statErr)
	}
	rel.FilePath = path
	rel.Size = fi.Size()
	rel.Sha512 = strings.TrimSpace(in.Sha512)

	wctx, wcancel := s.mongoContext()
	defer wcancel()
	if _, err := s.mongoDesktopReleases.InsertOne(wctx, rel); err != nil {
		_ = fs.Delete(context.Background(), path)
		if mongo.IsDuplicateKeyError(err) {
			return nil, transport.Conflict("VERSION_EXISTS", "a release with this version already exists")
		}
		return nil, mongoWriteError(err)
	}
	return rel, nil
}

// verifySavedInstaller 复核落盘内容：大小与 sha512 都必须与 release.json 自报值一致。
// 报 422（客户端可重传），不报 500 —— 这是可预期的上传失败，不是服务端故障。
func (s *Store) verifySavedInstaller(path, fileName string, wantSize int64, wantSha512 string) *transport.AppError {
	fi, err := os.Stat(path)
	if err != nil {
		return mongoWriteError(err)
	}
	if wantSize > 0 && fi.Size() != wantSize {
		return transport.Validation("uploaded installer size mismatch", map[string]any{
			"file_name":   fileName,
			"want_size":   wantSize,
			"actual_size": fi.Size(),
			"hint":        "upload was truncated; retry",
		})
	}
	f, err := os.Open(path)
	if err != nil {
		return mongoWriteError(err)
	}
	defer f.Close()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return mongoWriteError(err)
	}
	got := base64.StdEncoding.EncodeToString(h.Sum(nil))
	if got != strings.TrimSpace(wantSha512) {
		return transport.Validation("uploaded installer sha512 mismatch", map[string]any{
			"file_name":     fileName,
			"want_sha512":   strings.TrimSpace(wantSha512),
			"actual_sha512": got,
			"hint":          "file was corrupted in transit; retry",
		})
	}
	return nil
}

// SetDesktopReleaseBlockMap 落差分下载用的 .blockmap（可选）。
// 对外文件名强制为 "<安装包名>.blockmap"：electron-updater 就是按这个约定去拉的，
// 接受上传方自报的名字只会制造对不上的机会。
func (s *Store) SetDesktopReleaseBlockMap(id string, r io.Reader) (*model.DesktopRelease, *transport.AppError) {
	rel, appErr := s.GetDesktopRelease(id)
	if appErr != nil {
		return nil, appErr
	}
	if r == nil {
		return nil, transport.Validation("missing blockmap", map[string]any{"blockmap": "required"})
	}
	fs := s.desktopReleaseFileStorage()
	if fs == nil {
		return nil, transport.NewError(500, "INTERNAL_ERROR", "file storage not configured")
	}
	bmName := rel.FileName + ".blockmap"
	path, err := fs.Save(context.Background(), desktopReleaseStoragePartition, rel.ID+"-blockmap", bmName, r)
	if err != nil {
		return nil, mongoWriteError(err)
	}
	fi, statErr := os.Stat(path)
	if statErr != nil {
		_ = fs.Delete(context.Background(), path)
		return nil, mongoWriteError(statErr)
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	if _, err := s.mongoDesktopReleases.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{
		"block_map_file_name": bmName,
		"block_map_path":      path,
		"block_map_size":      fi.Size(),
	}}); err != nil {
		_ = fs.Delete(context.Background(), path)
		return nil, mongoWriteError(err)
	}
	rel.BlockMapFileName = bmName
	rel.BlockMapPath = path
	rel.BlockMapSize = fi.Size()
	return rel, nil
}

// PublishDesktopRelease 把发布指针前移到该版本（幂等）。
func (s *Store) PublishDesktopRelease(id string) (*model.DesktopRelease, *transport.AppError) {
	return s.setDesktopReleasePublished(id)
}

// RollbackDesktopRelease 把发布指针后移到该版本。
//
// 与 Publish 是**同一个动作**（改指针），差别只在审计动作与前端文案：
// electron-updater 只认「远程版本 > 本地版本」，所以回滚的影响面
// 严格等于「尚未升级到更高版本的机器」——已升级的机器不受影响（方案 §1 决策 5）。
func (s *Store) RollbackDesktopRelease(id string) (*model.DesktopRelease, *transport.AppError) {
	return s.setDesktopReleasePublished(id)
}

// setDesktopReleasePublished 是发布/回滚的唯一实现。
//
// 写序：先把**其它** published 降级为 archived，再把目标置为 published。
// 反过来会撞上「至多一条 published」的部分唯一索引。
// 代价：两步之间存在一个极短的无 published 窗口，此时公开 feed 返回 404 ——
// 更新端把 404 当「本次无更新」，下次启动重试，对内部十来人无影响。
func (s *Store) setDesktopReleasePublished(id string) (*model.DesktopRelease, *transport.AppError) {
	rel, appErr := s.GetDesktopRelease(id)
	if appErr != nil {
		return nil, appErr
	}
	now := time.Now().UTC()
	ctx, cancel := s.mongoContext()
	defer cancel()
	archiveOthers := bson.M{"$set": bson.M{"status": model.DesktopReleaseStatusArchived}}
	if _, err := s.mongoDesktopReleases.UpdateMany(ctx, bson.M{
		"status": model.DesktopReleaseStatusPublished,
		"_id":    bson.M{"$ne": id},
	}, archiveOthers); err != nil {
		return nil, mongoWriteError(err)
	}
	publishTarget := bson.M{"$set": bson.M{
		"status":       model.DesktopReleaseStatusPublished,
		"published_at": now,
	}}
	if _, err := s.mongoDesktopReleases.UpdateOne(ctx, bson.M{"_id": id}, publishTarget); err != nil {
		return nil, mongoWriteError(err)
	}
	rel.Status = model.DesktopReleaseStatusPublished
	rel.PublishedAt = now
	return rel, nil
}

// DeleteDesktopRelease 删除一条发行版（记录 + 文件）。
// **禁止删除当前 published**：那会让所有未升级客户端拿到 404 且没有任何替代版本。
// 删除一律 Mongo-first（先删记录、再删文件），与仓库其余删除路径一致。
func (s *Store) DeleteDesktopRelease(id string) *transport.AppError {
	rel, appErr := s.GetDesktopRelease(id)
	if appErr != nil {
		return appErr
	}
	if rel.Status == model.DesktopReleaseStatusPublished {
		return transport.Conflict("RELEASE_PUBLISHED",
			"cannot delete the published release; publish another version first")
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	if _, err := s.mongoDesktopReleases.DeleteOne(ctx, bson.M{"_id": id}); err != nil {
		return mongoWriteError(err)
	}
	s.deleteDesktopReleaseFiles(rel)
	return nil
}

// deleteDesktopReleaseFiles 尽力而为地回收磁盘文件：记录已删，孤儿文件只占磁盘、
// 不影响任何功能，因此失败只告警（不得让删除操作失败）。
func (s *Store) deleteDesktopReleaseFiles(rel *model.DesktopRelease) {
	if rel == nil {
		return
	}
	fs := s.desktopReleaseFileStorage()
	if fs == nil {
		return
	}
	for _, path := range []string{rel.FilePath, rel.BlockMapPath} {
		if path == "" {
			continue
		}
		if err := fs.Delete(context.Background(), path); err != nil && s.log != nil {
			s.log.Warn("delete desktop release file failed",
				zap.String("version", rel.Version), zap.String("path", path), zap.Error(err))
		}
	}
}

// PruneDesktopReleases 把版本数收敛到 DesktopReleaseRetain：每次发布后调用一次，
// 从**最旧的 archived** 开始删（published 永不参与，DeleteDesktopRelease 还会再挡一次）。
// 返回被删除的版本号列表，供日志/审计留痕。
func (s *Store) PruneDesktopReleases() []string {
	all, appErr := s.ListDesktopReleases()
	if appErr != nil {
		if s.log != nil {
			s.log.Warn("prune desktop releases: list failed", zap.Error(appErr))
		}
		return nil
	}
	if len(all) <= model.DesktopReleaseRetain {
		return nil
	}
	candidates := make([]model.DesktopRelease, 0, len(all))
	for _, rel := range all {
		if rel.Status == model.DesktopReleaseStatusArchived {
			candidates = append(candidates, rel)
		}
	}
	// List 是 created_at 倒序；转成升序 = 最旧优先。
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
	})
	excess := len(all) - model.DesktopReleaseRetain
	deleted := make([]string, 0, excess)
	for _, rel := range candidates {
		if len(deleted) >= excess {
			break
		}
		if appErr := s.DeleteDesktopRelease(rel.ID); appErr != nil {
			if s.log != nil {
				s.log.Warn("prune desktop release failed",
					zap.String("version", rel.Version), zap.Error(appErr))
			}
			continue
		}
		deleted = append(deleted, rel.Version)
	}
	return deleted
}
