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

// PlatformManualHandler 平台操作手册（结构化文档）：
//   - GET  /api/v1/manual                       （authed）读结构化手册；
//   - GET  /api/v1/platform/manual              （平台权限点）管理端读取（含正文）；
//   - PUT  /api/v1/platform/manual              （平台权限点）整篇覆盖保存；
//   - POST /api/v1/platform/manual/images       （平台权限点）上传配图；
//   - GET  /api/v1/manual/images/:id            （公开只读）配图本体。
//
// 与 platform_guides（整篇 HTML + sandbox iframe）并存不冲突：
// 本 handler 走**结构化数据**，前端用平台自身的 antd 组件渲染，视觉与平台一致，
// 且管理员可在界面内表单化编辑（无需懂 HTML）。
//
// 配图为什么公开只读：图片本身不含租户数据，且手册正文（含 img src）会下发给
// 所有登录用户；若图片要鉴权，前端就得给每个 <img> 挂 token 或改用 blob 加载，
// 复杂度远高于收益（对照：桌面端桌面安装包 feed 同样是公开只读）。

type PlatformManualHandler struct {
	store *store.Store
}

// NewPlatformManualHandler 构造。
func NewPlatformManualHandler(s *store.Store) *PlatformManualHandler {
	return &PlatformManualHandler{store: s}
}

// manualImageMaxUpload 与 store 侧 manualImageMaxBytes 同值双保险。
const manualImageMaxUpload = 8 << 20

// Get 面向登录用户的读取端点。从未保存过 → 200 + manual:null（前端空态）。
func (h *PlatformManualHandler) Get(c *gin.Context) {
	h.get(c)
}

// PlatformGet 管理端读取（同一份内容，独立路由便于权限点与语义区分）。
// 与 Get 走同一实现：管理端不需要额外字段，差别只在路由挂的权限点。
func (h *PlatformManualHandler) PlatformGet(c *gin.Context) {
	h.get(c)
}

// get 是 Get/PlatformGet 的共用实现。
func (h *PlatformManualHandler) get(c *gin.Context) {
	m, appErr := h.store.GetPlatformManual()
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if m == nil {
		transport.WriteData(c, http.StatusOK, gin.H{"manual": nil})
		return
	}
	transport.WriteData(c, http.StatusOK, gin.H{"manual": m})
}

// PlatformSave 整篇覆盖保存（单篇语义）。
func (h *PlatformManualHandler) PlatformSave(c *gin.Context) {
	var payload model.PlatformManual
	if err := c.ShouldBindJSON(&payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid manual payload: "+err.Error()))
		return
	}
	saved, appErr := h.store.UpsertPlatformManual(&payload, middleware.UserID(c))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, model.AuditScopePlatform, model.AuditActionPlatformGuideUpload,
		model.AuditTargetPlatformGuide, saved.Title, map[string]any{
			"title":    saved.Title,
			"sections": len(saved.Sections),
		})
	transport.WriteData(c, http.StatusOK, gin.H{"manual": saved})
}

// UploadImage 上传手册配图（multipart，file 字段，与其它上传端点同构）。
func (h *PlatformManualHandler) UploadImage(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, manualImageMaxUpload+(32<<20))
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

	asset, appErr := h.store.SaveManualImage(header.Filename, file, middleware.UserID(c))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, gin.H{"image": asset})
}

// ServeImage 公开只读地返回配图本体。支持 Range/缓存协商（http.ServeContent）。
func (h *PlatformManualHandler) ServeImage(c *gin.Context) {
	id := c.Param("id")
	path, contentType, appErr := h.store.ResolveManualImagePath(id)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		// 记录在库、文件不在盘 = 数据不一致，按 NotFound 回复。
		transport.WriteError(c, transport.NotFound("manual image file not found"))
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		transport.WriteError(c, transport.NewError(http.StatusInternalServerError, "INTERNAL_ERROR", "cannot stat image file"))
		return
	}
	// 资源 ID 含 16 字节随机量且内容不可变（覆盖上传会得到新 ID），可长缓存。
	if contentType != "" {
		c.Header("Content-Type", contentType)
	} else {
		c.Header("Content-Type", "application/octet-stream")
	}
	c.Header("Cache-Control", "public, max-age=604800, immutable")
	// 🔴 必须挡「按 MIME 嗅探执行」：SVG 可含脚本，嗅探型浏览器有 XSS 面。
	// 公开图片一律禁止内联执行上下文之外的解释（配合 CSP 更稳）。
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, id, fi.ModTime(), f)
}
