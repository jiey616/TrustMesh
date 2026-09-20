package store

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// ────────────────────────────────────────────────────────────────────────────
// 移动端安装包（Android APK）：mobile_app_releases 集合 + 磁盘文件。
//
// 与 desktop_releases 同策略：**Mongo 权威，不进全内存状态机**。单条 current
// 记录（_id="current"），上传即覆盖 —— 扫码下载只需要「最新可装的那个包」，
// 不需要桌面端那套 draft/publish/rollback 发布指针。
//
// 文件存储复用 LocalFileStorage，分区 mobile-app，按版本号分目录：
// {FILES_STORAGE_PATH}/mobile-app/{version}/{fileName}，覆盖上传新版本后
// 旧版本文件随即回收（不留历史 —— 需要历史包时重新打包即可）。
// ────────────────────────────────────────────────────────────────────────────

// mobileAppStoragePartition 是 LocalFileStorage 的 projectID 分区名。
const mobileAppStoragePartition = "mobile-app"

// requireMobileAppMongo 在 Mongo 不可用时给出明确失败（无内存兜底可降级）。
func (s *Store) requireMobileAppMongo() *transport.AppError {
	if s == nil || !s.mongoEnabled || s.mongoMobileAppReleases == nil {
		return transport.NewError(500, "INTERNAL_ERROR", "mobile app release store requires mongo")
	}
	return nil
}

// validateMobileAppUpload 校验版本号与 APK 文件名。抽成纯函数以便单测。
// 文件名会进公开下载 URL 路径段，复用桌面包的 ASCII 守门（免 URL 转义歧义）。
func validateMobileAppUpload(version, fileName string) *transport.AppError {
	version = strings.TrimSpace(version)
	if !desktopReleaseVersionRe.MatchString(version) {
		return transport.Validation("invalid version", map[string]any{
			"version": "must be semver (e.g. 1.0.0)",
		})
	}
	name := strings.TrimSpace(fileName)
	if !strings.HasSuffix(strings.ToLower(name), ".apk") {
		return transport.Validation("invalid file name", map[string]any{
			"file_name": "must be an .apk file",
		})
	}
	if !strings.Contains(name, version) {
		return transport.Validation("invalid file name", map[string]any{
			"file_name": "must contain the version (e.g. TrustMesh-" + version + ".apk)",
		})
	}
	// 与 validateDesktopReleaseFileName 同口径：拒路径语义 / 非 ASCII / URL 危险字符。
	if n := name; n == "" || n == "." || n == ".." ||
		strings.ContainsAny(n, `/\ ?#%&="'`) || strings.ContainsRune(n, 0) {
		return transport.Validation("invalid file name", map[string]any{
			"file_name": "must be printable ascii without path or url-unsafe characters",
		})
	}
	for _, r := range name {
		if r < 0x20 || r > 0x7e {
			return transport.Validation("invalid file name", map[string]any{
				"file_name": "must be printable ascii",
			})
		}
	}
	return nil
}

// GetMobileAppRelease 返回当前安装包记录；从未上传过时返回 (nil, nil)，
// 由 handler 决定空态语义（meta 端点回 200 + null，前端隐藏扫码入口）。
func (s *Store) GetMobileAppRelease() (*model.MobileAppRelease, *transport.AppError) {
	if appErr := s.requireMobileAppMongo(); appErr != nil {
		return nil, appErr
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	var rel model.MobileAppRelease
	err := s.mongoMobileAppReleases.FindOne(ctx,
		bson.M{"_id": model.MobileAppReleaseCurrentID}).Decode(&rel)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, mongoWriteError(err)
	}
	return &rel, nil
}

// ResolveMobileAppFilePath 把当前记录解析为磁盘绝对路径。
// 按**库字段**返回，不做任何路径拼接 —— 天然免疫目录穿越。
func (s *Store) ResolveMobileAppFilePath() (string, *transport.AppError) {
	rel, appErr := s.GetMobileAppRelease()
	if appErr != nil {
		return "", appErr
	}
	if rel == nil || strings.TrimSpace(rel.FilePath) == "" {
		return "", transport.NotFound("mobile app release file not found")
	}
	return rel.FilePath, nil
}

// UpsertMobileAppRelease 覆盖写入当前安装包：**先写盘、后写库**（写库失败回收新文件），
// 成功后回收旧版本文件。sha512 由服务端落盘后重算（浏览器下载链路不消费该字段，
// 记录它只为审计留证与人工核对）。
func (s *Store) UpsertMobileAppRelease(version, fileName string, file io.Reader, updatedBy string) (*model.MobileAppRelease, *transport.AppError) {
	if appErr := validateMobileAppUpload(version, fileName); appErr != nil {
		return nil, appErr
	}
	if file == nil {
		return nil, transport.Validation("missing file", map[string]any{"file": "required"})
	}
	if appErr := s.requireMobileAppMongo(); appErr != nil {
		return nil, appErr
	}
	fs := s.desktopReleaseFileStorage()
	if fs == nil {
		return nil, transport.NewError(500, "INTERNAL_ERROR", "file storage not configured")
	}

	version = strings.TrimSpace(version)
	// 客户端 multipart 文件名可能带本地路径前缀（老浏览器行为），统一取 basename
	// 再校验，避免「合法包因路径前缀被拒」。
	fileName = filepath.Base(strings.TrimSpace(fileName))

	// 🔴 文件 IO 全程在锁外（fs 的方法不得在持 s.mu 时调用）。
	// 按版本分目录：不同版本不互相覆盖，旧版本在入库成功后统一回收。
	path, err := fs.Save(context.Background(), mobileAppStoragePartition, version, fileName, file)
	if err != nil {
		return nil, mongoWriteError(err)
	}

	fi, statErr := os.Stat(path)
	if statErr != nil {
		_ = fs.Delete(context.Background(), path)
		return nil, mongoWriteError(statErr)
	}
	sum, hashErr := fileSha512FromPath(path)
	if hashErr != nil {
		_ = fs.Delete(context.Background(), path)
		return nil, mongoWriteError(hashErr)
	}

	prev, _ := s.GetMobileAppRelease()
	rel := &model.MobileAppRelease{
		ID:        model.MobileAppReleaseCurrentID,
		Version:   version,
		FileName:  fileName,
		FilePath:  path,
		Size:      fi.Size(),
		Sha512:    sum,
		UpdatedBy: updatedBy,
		UpdatedAt: time.Now().UTC(),
	}

	wctx, wcancel := s.mongoContext()
	_, werr := s.mongoMobileAppReleases.ReplaceOne(wctx,
		bson.M{"_id": rel.ID}, rel, options.Replace().SetUpsert(true))
	wcancel()
	if werr != nil {
		_ = fs.Delete(context.Background(), path)
		return nil, mongoWriteError(werr)
	}

	// 旧版本文件回收（路径不同才删；同版本重复上传时 Save 已覆盖同一路径）。
	if prev != nil && prev.FilePath != "" && prev.FilePath != path {
		if delErr := fs.Delete(context.Background(), prev.FilePath); delErr != nil && s.log != nil {
			s.log.Warn("discard previous mobile app file failed",
				zap.String("path", prev.FilePath), zap.Error(delErr))
		}
	}
	return rel, nil
}

// DeleteMobileAppRelease 删除当前记录与磁盘文件（幂等：不存在也算成功）。
func (s *Store) DeleteMobileAppRelease() *transport.AppError {
	if appErr := s.requireMobileAppMongo(); appErr != nil {
		return appErr
	}
	prev, appErr := s.GetMobileAppRelease()
	if appErr != nil {
		return appErr
	}
	ctx, cancel := s.mongoContext()
	_, err := s.mongoMobileAppReleases.DeleteOne(ctx, bson.M{"_id": model.MobileAppReleaseCurrentID})
	cancel()
	if err != nil {
		return mongoWriteError(err)
	}
	if prev != nil && strings.TrimSpace(prev.FilePath) != "" {
		if fs := s.desktopReleaseFileStorage(); fs != nil {
			if delErr := fs.Delete(context.Background(), prev.FilePath); delErr != nil && s.log != nil {
				s.log.Warn("discard deleted mobile app file failed",
					zap.String("path", prev.FilePath), zap.Error(delErr))
			}
		}
	}
	return nil
}

// fileSha512FromPath 流式重算落盘文件的 sha512（base64 标准编码，与桌面包口径一致）。
func fileSha512FromPath(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}
