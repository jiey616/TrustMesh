package handler

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// MobileAppReleaseHandler 移动端安装包（Android APK）：
//   - GET /api/v1/mobile/app/latest    （**公开**）当前包元信息（无包 → mobile_app:null）；
//   - GET /api/v1/mobile/app/download  （**公开**）APK 本体，扫码即下，不要求登录；
//   - GET/POST/DELETE /api/v1/platform/mobile-app（平台权限点）管理。
//
// 公开下载与桌面 feed 同理：扫码发生在**未登录**的手机浏览器里，
// 要求登录会把「下载安装包」变成不可能任务。APK 本身不含租户数据，
// 公开面只有「当前版本号 + 文件名」，无信息泄露风险。
type MobileAppReleaseHandler struct {
	store *store.Store
}

// NewMobileAppReleaseHandler 构造。
func NewMobileAppReleaseHandler(s *store.Store) *MobileAppReleaseHandler {
	return &MobileAppReleaseHandler{store: s}
}

// mobileAppMaxUpload 与网关 512m 对齐（APK 量级远小于桌面安装包，但上限保持同档）。
const mobileAppMaxUpload = 512 << 20

// Latest 公开元信息端点。前端登录页用它决定「扫码下载」入口是否显示，
// 以及把 download 拼成二维码内容。
func (h *MobileAppReleaseHandler) Latest(c *gin.Context) {
	rel, appErr := h.store.GetMobileAppRelease()
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if rel == nil {
		transport.WriteData(c, http.StatusOK, gin.H{"mobile_app": nil})
		return
	}
	transport.WriteData(c, http.StatusOK, gin.H{"mobile_app": gin.H{
		"version":    rel.Version,
		"file_name":  rel.FileName,
		"size":       rel.Size,
		"updated_at": rel.UpdatedAt,
		// 绝对地址由前端按自身 origin/serverUrl 拼装（见 appDownload.ts），
		// 这里给相对路径，避免服务端反向代理场景下拼错 host。
		"download_path": "/api/v1/mobile/app/download",
	}})
}

// Download 公开下载端点。支持 Range（手机浏览器断点续传），no-cache 保证
// 覆盖上传后扫码立刻拿到新包。
func (h *MobileAppReleaseHandler) Download(c *gin.Context) {
	path, appErr := h.store.ResolveMobileAppFilePath()
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		// 记录在库、文件不在盘 = 数据不一致，按 NotFound 回复。
		transport.WriteError(c, transport.NotFound("mobile app release file not found"))
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		transport.WriteError(c, transport.NewError(http.StatusInternalServerError, "INTERNAL_ERROR", "cannot stat mobile app file"))
		return
	}
	rel, appErr := h.store.GetMobileAppRelease()
	if appErr != nil || rel == nil {
		transport.WriteError(c, transport.NotFound("mobile app release not found"))
		return
	}
	c.Header("Cache-Control", "no-cache")
	// attachment 而非 inline：手机浏览器直接进下载流程，而不是尝试打开二进制。
	c.Header("Content-Disposition", `attachment; filename="`+rel.FileName+`"`)
	c.Header("Content-Type", "application/vnd.android.package-archive")
	http.ServeContent(c.Writer, c.Request, rel.FileName, fi.ModTime(), f)
}

// PlatformGet 管理端元信息（与公开端点同构，走平台权限点）。
func (h *MobileAppReleaseHandler) PlatformGet(c *gin.Context) {
	rel, appErr := h.store.GetMobileAppRelease()
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if rel == nil {
		transport.WriteData(c, http.StatusOK, gin.H{"mobile_app": nil})
		return
	}
	transport.WriteData(c, http.StatusOK, gin.H{"mobile_app": gin.H{
		"version":    rel.Version,
		"file_name":  rel.FileName,
		"size":       rel.Size,
		"sha512":     rel.Sha512,
		"updated_by": rel.UpdatedBy,
		"updated_at": rel.UpdatedAt,
	}})
}

// PlatformUpload 覆盖上传当前安装包（multipart：file + version 字段）。
func (h *MobileAppReleaseHandler) PlatformUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, mobileAppMaxUpload)
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid multipart form: "+err.Error()))
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		transport.WriteError(c, transport.Validation("missing file", map[string]any{
			"file": "multipart field 'file' is required",
		}))
		return
	}
	defer file.Close()

	version := c.PostForm("version")
	rel, appErr := h.store.UpsertMobileAppRelease(version, header.Filename, file, middleware.UserID(c))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, model.AuditScopePlatform, model.AuditActionMobileAppUpload,
		model.AuditTargetMobileApp, rel.Version, map[string]any{
			"file_name": rel.FileName,
			"size":      rel.Size,
			"sha512":    rel.Sha512,
		})
	transport.WriteData(c, 200, gin.H{"mobile_app": gin.H{
		"version":    rel.Version,
		"file_name":  rel.FileName,
		"size":       rel.Size,
		"updated_by": rel.UpdatedBy,
		"updated_at": rel.UpdatedAt,
	}})
}

// PlatformDelete 删除当前安装包（记录 + 磁盘文件，幂等）。
func (h *MobileAppReleaseHandler) PlatformDelete(c *gin.Context) {
	// 删除前取一次元数据用于审计留证（删后无据可查）。
	rel, appErr := h.store.GetMobileAppRelease()
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if appErr := h.store.DeleteMobileAppRelease(); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	version := ""
	if rel != nil {
		version = rel.Version
	}
	recordAudit(c, h.store, model.AuditScopePlatform, model.AuditActionMobileAppDelete,
		model.AuditTargetMobileApp, version, nil)
	transport.WriteData(c, 200, gin.H{"deleted": true})
}
