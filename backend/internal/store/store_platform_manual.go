package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/project"
	"trustmesh/backend/internal/transport"
)

// ────────────────────────────────────────────────────────────────────────────
// 平台操作手册（结构化单篇文档）：platform_manual + manual_image_assets 集合。
//
// 与 platform_guides（整篇 HTML + iframe）并存：那套是"上传即渲染"的旧机制，
// 本套是"结构化数据 + 平台组件渲染 + 界面内编辑"的新机制。两者互不影响，
// 管理员可以任选（甚至先并存过渡）。
//
// 同 desktop_releases 策略：**Mongo 权威，不进全内存状态机** —— 平台级低频
// 管理数据，无需 FlushPersistAll 镜像。固定 _id="global"，保存即覆盖。
//
// 🔴 图片走文件存储（不内联 data URI）：BSON 单文档 16MiB 是硬顶，实测整篇
// 含图 HTML 已 2.9MB；若图片继续内联，多几张高清图就会触及上限。图片进
// {FILES_STORAGE_PATH}/manual-images/，DB 只存 URL。
// ────────────────────────────────────────────────────────────────────────────

const (
	// manualImageStoragePartition 是 LocalFileStorage 的分区名。
	manualImageStoragePartition = "manual-images"
	// manualImageMaxBytes 是单张配图上限（8MiB）——远小于 512m 网关上限，
	// 只允许图片类资源，超限在入口拒绝。
	manualImageMaxBytes = 8 << 20
	// manualMaxSections 是章节数上限（防单文档无限膨胀逼近 16MiB）。
	manualMaxSections = 60
	// manualMaxBlocksPerSection 是每章内容块上限。
	manualMaxBlocksPerSection = 120
)

// manualAllowedImageExt 是允许的图片扩展名白名单。
var manualAllowedImageExt = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}

// requirePlatformManualMongo 在 Mongo 不可用时明确失败（无内存兜底可降级）。
func (s *Store) requirePlatformManualMongo() *transport.AppError {
	if s == nil || !s.mongoEnabled || s.mongoPlatformManual == nil {
		return transport.NewError(500, "INTERNAL_ERROR", "platform manual store requires mongo")
	}
	return nil
}

// manualFileStorage 返回项目文件存储句柄（与 desktop-releases 复用同一 root，
// 仅分区名不同）。🔴 必须在 s.mu 之外调用返回值的任何方法（文件 IO 不得持锁）。
func (s *Store) manualFileStorage() project.FileStorage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fileStorage
}

// GetPlatformManual 返回全局手册；从未保存过时返回 (nil, nil)，
// 由 handler 决定空态语义（200 + manual:null，前端显示空态/引导上传）。
func (s *Store) GetPlatformManual() (*model.PlatformManual, *transport.AppError) {
	if appErr := s.requirePlatformManualMongo(); appErr != nil {
		return nil, appErr
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	var doc model.PlatformManual
	err := s.mongoPlatformManual.FindOne(ctx,
		bson.M{"_id": model.PlatformManualGlobalID}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, transport.NewError(500, "INTERNAL_ERROR", "failed to load platform manual")
	}
	return &doc, nil
}

// validatePlatformManual 校验章节树的结构约束（防膨胀 + 防脏数据）。
//
// 只做**结构性**校验：类型白名单、数量上限、必填字段。内容语义（文案好坏、
// 图片是否有效）不在这里管 —— 那是管理员的职责，后端越权审核只会误伤。
func validatePlatformManual(m *model.PlatformManual) *transport.AppError {
	if len(m.Sections) == 0 {
		return transport.Validation("empty manual", map[string]any{
			"sections": "at least one section is required",
		})
	}
	if len(m.Sections) > manualMaxSections {
		return transport.Validation("too many sections", map[string]any{
			"sections":  "must be at most 60",
			"max_count": manualMaxSections,
		})
	}
	for i, sec := range m.Sections {
		if strings.TrimSpace(sec.Title) == "" {
			return transport.Validation("invalid section", map[string]any{
				"sections": "section title must not be empty",
				"index":    i,
			})
		}
		if len(sec.Blocks) > manualMaxBlocksPerSection {
			return transport.Validation("too many blocks", map[string]any{
				"sections":  "each section may have at most 120 blocks",
				"index":     i,
				"max_count": manualMaxBlocksPerSection,
			})
		}
		for j, b := range sec.Blocks {
			if !manualBlockTypeAllowed(b.Type) {
				return transport.Validation("unknown block type", map[string]any{
					"section_index": i,
					"block_index":   j,
					"type":          b.Type,
				})
			}
		}
	}
	return nil
}

// manualBlockTypeAllowed 是前端能渲染的块类型白名单。
// 加新类型要**前后端同时**改（后端这里 + 前端渲染器）。
func manualBlockTypeAllowed(t string) bool {
	switch t {
	case "paragraph", "heading", "steps", "tip", "warn", "image", "table", "flow", "qa":
		return true
	default:
		return false
	}
}

// UpsertPlatformManual 覆盖写入全局手册（单篇语义：后存覆盖先存）。
//
// 🔴 用 ReplaceOne(upsert) 而非 $set：章节数组整体替换语义，避免旧章节
// 删不掉（$set 只覆盖已给的键，被删章节会残留在文档里）。
func (s *Store) UpsertPlatformManual(m *model.PlatformManual, updatedBy string) (*model.PlatformManual, *transport.AppError) {
	if m == nil {
		return nil, transport.BadRequest("BAD_REQUEST", "manual payload is required")
	}
	if appErr := validatePlatformManual(m); appErr != nil {
		return nil, appErr
	}
	if appErr := s.requirePlatformManualMongo(); appErr != nil {
		return nil, appErr
	}
	doc := &model.PlatformManual{
		ID:        model.PlatformManualGlobalID,
		Title:     strings.TrimSpace(m.Title),
		Subtitle:  strings.TrimSpace(m.Subtitle),
		Footer:    strings.TrimSpace(m.Footer),
		Sections:  m.Sections,
		UpdatedBy: updatedBy,
		UpdatedAt: time.Now().UTC(),
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoPlatformManual.ReplaceOne(ctx,
		bson.M{"_id": doc.ID}, doc, options.Replace().SetUpsert(true))
	if err != nil {
		return nil, transport.NewError(500, "INTERNAL_ERROR", "failed to save platform manual")
	}
	return doc, nil
}

// SaveManualImage 保存手册配图到文件存储，并登记元数据。返回含 URL 的资源。
//
// 流程：白名单校验扩展名/MIME → 读入内存（受 8MiB 上限保护）→ 写文件存储
// → 落一条 manual_image_assets 记录（DB 只存 URL，不存正文）。
func (s *Store) SaveManualImage(fileName string, r io.Reader, uploadedBy string) (*model.ManualImageAsset, *transport.AppError) {
	if appErr := s.requirePlatformManualMongo(); appErr != nil {
		return nil, appErr
	}
	name := strings.TrimSpace(fileName)
	if name == "" {
		return nil, transport.Validation("invalid file name", map[string]any{"file_name": "required"})
	}
	// 去掉任何路径成分（纵深防御：文件名不参与磁盘路径拼接，但仍收敛）。
	name = filepath.Base(name)
	ext := strings.ToLower(filepath.Ext(name))
	contentType, ok := manualAllowedImageExt[ext]
	if !ok {
		return nil, transport.Validation("unsupported image type", map[string]any{
			"file_name": "must be one of .png/.jpg/.jpeg/.gif/.webp/.svg",
		})
	}

	// 读入内存并卡上限：io.LimitReader 多读 1 字节以便区分「刚好到顶」与「超限」。
	data, err := io.ReadAll(io.LimitReader(r, manualImageMaxBytes+1))
	if err != nil {
		return nil, transport.BadRequest("BAD_REQUEST", "cannot read uploaded image")
	}
	if len(data) == 0 {
		return nil, transport.Validation("empty file", map[string]any{"file": "must not be empty"})
	}
	if len(data) > manualImageMaxBytes {
		return nil, transport.Validation("image too large", map[string]any{
			"file":      "must be at most 8MiB",
			"max_bytes": manualImageMaxBytes,
		})
	}

	fs := s.manualFileStorage()
	if fs == nil {
		return nil, transport.NewError(500, "INTERNAL_ERROR", "file storage is not configured")
	}

	id, err := newManualAssetID()
	if err != nil {
		return nil, transport.NewError(500, "INTERNAL_ERROR", "failed to allocate asset id")
	}

	// 🔴 不持锁做文件 IO：storage 句柄已取出，这里在锁外调用。
	// 用 bytes.NewReader 而非 strings.NewReader(string(data))——后者会为 8MiB
	// 以内的图片多拷贝一份字符串，纯浪费。
	path, err := fs.Save(context.Background(), manualImageStoragePartition, id, name, bytes.NewReader(data))
	if err != nil {
		return nil, transport.NewError(500, "INTERNAL_ERROR", "failed to store image: "+err.Error())
	}

	asset := &model.ManualImageAsset{
		ID:          id,
		FileName:    name,
		Size:        int64(len(data)),
		ContentType: contentType,
		Path:        path,
		URL:         "/api/v1/manual/images/" + id,
		UploadedBy:  uploadedBy,
		UploadedAt:  time.Now().UTC(),
	}

	ctx, cancel := s.mongoContext()
	defer cancel()
	if _, err := s.mongoManualImages.ReplaceOne(ctx,
		bson.M{"_id": asset.ID}, asset, options.Replace().SetUpsert(true)); err != nil {
		return nil, transport.NewError(500, "INTERNAL_ERROR", "failed to register image asset")
	}
	return asset, nil
}

// ResolveManualImagePath 按资源 ID 查磁盘路径（供下载端点）。
// 返回 (path, contentType, appErr)；不存在时 appErr 为 NotFound。
func (s *Store) ResolveManualImagePath(id string) (string, string, *transport.AppError) {
	if appErr := s.requirePlatformManualMongo(); appErr != nil {
		return "", "", appErr
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	var asset model.ManualImageAsset
	err := s.mongoManualImages.FindOne(ctx, bson.M{"_id": id}).Decode(&asset)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", "", transport.NotFound("manual image not found")
		}
		return "", "", transport.NewError(500, "INTERNAL_ERROR", "failed to load image asset")
	}
	return asset.Path, asset.ContentType, nil
}

// newManualAssetID 生成 16 字节随机十六进制 ID（文件名前缀，避免撞名/可猜）。
func newManualAssetID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
