package store

import (
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// ────────────────────────────────────────────────────────────────────────────
// 平台「使用引导」（单篇全局 HTML 文档）：platform_guides 集合。
//
// 与 desktop_releases 同策略：**Mongo 权威，不进全内存状态机** —— 平台级低频
// 管理数据，无需 FlushPersistAll 镜像。固定 _id="global"，重复上传即覆盖。
//
// 上限取 15MiB：BSON 单文档 16MiB 是硬顶（留出字段名/元数据的编码余量），
// 自包含 HTML（内联图片走 data URI）实测轻松超 2MiB —— 旧值 2MiB 会卡住
// 正常文档；仍保持「静态文档、有上限」的定位，入口拒绝优于进库后拖慢拉取。
// ────────────────────────────────────────────────────────────────────────────

const (
	// platformGuideMaxUpload 是 HTML 正文的字节上限（15MiB，BSON 16MiB 硬顶内）。
	platformGuideMaxUpload = 15 << 20
	// platformGuideHTMLSuffix 是允许的文件扩展名（需求约定：.html）。
	platformGuideHTMLSuffix = ".html"
)

// requirePlatformGuideMongo 在 Mongo 不可用时给出明确失败（无内存兜底可降级）。
func (s *Store) requirePlatformGuideMongo() *transport.AppError {
	if s == nil || !s.mongoEnabled || s.mongoPlatformGuides == nil {
		return transport.NewError(500, "INTERNAL_ERROR", "platform guide store requires mongo")
	}
	return nil
}

// validateGuideUpload 校验上传文件名与正文的入口约束。抽成纯函数以便单测：
// 文件名最终会出现在管理端 UI 与审计日志里，格式必须收敛。
func validateGuideUpload(fileName string, html []byte) *transport.AppError {
	name := strings.TrimSpace(fileName)
	if name == "" {
		return transport.Validation("invalid file name", map[string]any{"file_name": "required"})
	}
	if !strings.HasSuffix(strings.ToLower(name), platformGuideHTMLSuffix) {
		return transport.Validation("invalid file name", map[string]any{
			"file_name": "must be an .html file",
		})
	}
	if len(html) == 0 {
		return transport.Validation("empty document", map[string]any{"file": "must not be empty"})
	}
	if len(html) > platformGuideMaxUpload {
		return transport.Validation("document too large", map[string]any{
			"file":      "must be at most 15MiB",
			"max_bytes": platformGuideMaxUpload,
		})
	}
	return nil
}

// GetPlatformGuide 返回全局引导文档；从未上传过时返回 (nil, nil)，
// 由 handler 决定空态语义（200 + guide:null，前端显示「暂无使用引导」）。
func (s *Store) GetPlatformGuide() (*model.PlatformGuide, *transport.AppError) {
	if appErr := s.requirePlatformGuideMongo(); appErr != nil {
		return nil, appErr
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	var guide model.PlatformGuide
	err := s.mongoPlatformGuides.FindOne(ctx,
		bson.M{"_id": model.PlatformGuideGlobalID}).Decode(&guide)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, transport.NewError(500, "INTERNAL_ERROR", "failed to load platform guide")
	}
	return &guide, nil
}

// UpsertPlatformGuide 覆盖写入全局引导文档（单篇语义：后传覆盖先传）。
func (s *Store) UpsertPlatformGuide(fileName string, html []byte, updatedBy string) (*model.PlatformGuide, *transport.AppError) {
	if appErr := validateGuideUpload(fileName, html); appErr != nil {
		return nil, appErr
	}
	if appErr := s.requirePlatformGuideMongo(); appErr != nil {
		return nil, appErr
	}
	guide := &model.PlatformGuide{
		ID:        model.PlatformGuideGlobalID,
		FileName:  strings.TrimSpace(fileName),
		Size:      len(html),
		HTML:      string(html),
		UpdatedBy: updatedBy,
		UpdatedAt: time.Now().UTC(),
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoPlatformGuides.ReplaceOne(ctx,
		bson.M{"_id": guide.ID}, guide, options.Replace().SetUpsert(true))
	if err != nil {
		return nil, transport.NewError(500, "INTERNAL_ERROR", "failed to save platform guide")
	}
	return guide, nil
}

// DeletePlatformGuide 删除全局引导文档（幂等：不存在也算成功）。
func (s *Store) DeletePlatformGuide() *transport.AppError {
	if appErr := s.requirePlatformGuideMongo(); appErr != nil {
		return appErr
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoPlatformGuides.DeleteOne(ctx, bson.M{"_id": model.PlatformGuideGlobalID})
	if err != nil {
		return transport.NewError(500, "INTERNAL_ERROR", "failed to delete platform guide")
	}
	return nil
}
