package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// PlatformGuideHandler 平台「使用引导」：
//   - GET /api/v1/guide          （authed，登录用户）读全局引导 HTML；
//   - GET/PUT/DELETE /api/v1/platform/guide（平台权限点）管理与删除。
//
// 上传走 multipart（file 字段），与 desktop-releases 的表单协议保持同构。
// 安全模型：后端只守格式（.html + 2MiB），**不做 HTML 消毒** —— 前端渲染
// 必须用 sandbox iframe（默认禁脚本），那是比消毒更强的执行边界（见 model 注释）。
type PlatformGuideHandler struct {
	store *store.Store
}

// NewPlatformGuideHandler 构造。
func NewPlatformGuideHandler(s *store.Store) *PlatformGuideHandler {
	return &PlatformGuideHandler{store: s}
}

// guideMaxUpload 与 store 侧 platformGuideMaxUpload 同值双保险：
// 网关/MaxBytesReader 先拦一层，store 校验再兜一层（绕过 handler 直调时仍受保护）。
const guideMaxUpload = 2 << 20

// Get 面向登录用户的读取端点。从未上传过 → 200 + guide:null（前端空态，不算错误）。
func (h *PlatformGuideHandler) Get(c *gin.Context) {
	guide, appErr := h.store.GetPlatformGuide()
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if guide == nil {
		transport.WriteData(c, http.StatusOK, gin.H{"guide": nil})
		return
	}
	transport.WriteData(c, http.StatusOK, gin.H{
		"guide": gin.H{
			"file_name":  guide.FileName,
			"size":       guide.Size,
			"updated_at": guide.UpdatedAt,
			"html":       guide.HTML,
		},
	})
}

// PlatformGet 管理端读取（含正文，供上传前预览/对比）。
func (h *PlatformGuideHandler) PlatformGet(c *gin.Context) {
	guide, appErr := h.store.GetPlatformGuide()
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if guide == nil {
		transport.WriteData(c, http.StatusOK, gin.H{"guide": nil})
		return
	}
	transport.WriteData(c, http.StatusOK, gin.H{"guide": gin.H{
		"file_name":  guide.FileName,
		"size":       guide.Size,
		"updated_by": guide.UpdatedBy,
		"updated_at": guide.UpdatedAt,
		"html":       guide.HTML,
	}})
}

// PlatformUpload 覆盖上传全局引导文档（单篇语义）。
func (h *PlatformGuideHandler) PlatformUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, guideMaxUpload+(32<<20))
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

	html, err := io.ReadAll(file)
	if err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "cannot read uploaded file"))
		return
	}

	guide, appErr := h.store.UpsertPlatformGuide(header.Filename, html, middleware.UserID(c))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, model.AuditScopePlatform, model.AuditActionPlatformGuideUpload,
		model.AuditTargetPlatformGuide, guide.FileName, map[string]any{
			"file_name": guide.FileName,
			"size":      guide.Size,
		})
	transport.WriteData(c, 200, gin.H{"guide": gin.H{
		"file_name":  guide.FileName,
		"size":       guide.Size,
		"updated_by": guide.UpdatedBy,
		"updated_at": guide.UpdatedAt,
	}})
}

// PlatformDelete 删除全局引导文档（幂等）。
func (h *PlatformGuideHandler) PlatformDelete(c *gin.Context) {
	// 删除前取一次元数据用于审计留证（删后无据可查）。
	guide, appErr := h.store.GetPlatformGuide()
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if appErr := h.store.DeletePlatformGuide(); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	fileName := ""
	if guide != nil {
		fileName = strings.TrimSpace(guide.FileName)
	}
	recordAudit(c, h.store, model.AuditScopePlatform, model.AuditActionPlatformGuideDelete,
		model.AuditTargetPlatformGuide, fileName, nil)
	transport.WriteData(c, 200, gin.H{"deleted": true})
}
