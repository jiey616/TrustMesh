package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// PlatformDesktopReleaseHandler 是平台命名空间的桌面端发行版管理
// （/api/v1/platform/desktop-releases，权限点 platform.desktop.release），
// 并在同一条链路上服务两个**公开只读**的更新 feed 端点。
//
// 角色分工（方案 §1 决策 6）：管理端点走平台管理员 JWT；feed 端点**不鉴权** ——
// electron-updater 跑在独立的 session 分区（`electron-updater`），不携带登录 cookie，
// 若要求鉴权则登出已久的机器将永远无法升级。
//
// 方案全文：docs/desktop-app-update-plan-2026-09-18.md
type PlatformDesktopReleaseHandler struct {
	store *store.Store
}

func NewPlatformDesktopReleaseHandler(s *store.Store) *PlatformDesktopReleaseHandler {
	return &PlatformDesktopReleaseHandler{store: s}
}

// desktopReleaseMaxUpload 是单次上传的体积上限。与 nginx 的 client_max_body_size
// （生产为 512m）对齐：网关先拦一层，这里再兜一层（绕过网关直连 backend 时仍受保护）。
const desktopReleaseMaxUpload = 512 << 20

// desktopReleaseMetadata 是 release.json 的结构，由 `_deploy_desktop.py` 产出。
// 版本号一律来自脚本，不手填 —— latest.yml 的 version 与 package.json 必须逐字相同，
// 手填是「静默无更新」的唯一成因（方案 §3.7）。
type desktopReleaseMetadata struct {
	Version string `json:"version"`
	File    string `json:"file"`
	Size    int64  `json:"size"`
	Sha512  string `json:"sha512"`
	Notes   string `json:"notes"`
}

// List 返回全部发行版（最新在前），含 draft / published / archived 三态。
func (h *PlatformDesktopReleaseHandler) List(c *gin.Context) {
	items, appErr := h.store.ListDesktopReleases()
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"desktop_releases": items})
}

// Upload 接收一次发行版上传：安装包 + release.json（+ 可选 .blockmap）。
//
// 单请求 multipart（方案 §5）：前端一次提交带进度条，避免「先传包再传元数据」
// 的两段式在第二段失败时留下无主的孤儿包。
//
// 不接受上传方提供的 latest.yml —— feed 的 latest.yml 一律**按当前 published 动态生成**，
// 否则「发布指针」与「下载内容」会分属两个真相来源（上传者可以传一份指向旧版本的 yml）。
func (h *PlatformDesktopReleaseHandler) Upload(c *gin.Context) {
	sc := middleware.Scope(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, desktopReleaseMaxUpload)
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid multipart form: "+err.Error()))
		return
	}
	installerHeader, err := c.FormFile("installer")
	if err != nil {
		transport.WriteError(c, transport.Validation("missing installer", map[string]any{
			"installer": "required (multipart file field)",
		}))
		return
	}
	meta, appErr := readDesktopReleaseMetadata(c, installerHeader.Filename)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	installer, err := installerHeader.Open()
	if err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "cannot read uploaded installer"))
		return
	}
	defer installer.Close()

	rel, appErr := h.store.CreateDesktopRelease(model.DesktopReleaseInput{
		Version:    meta.Version,
		FileName:   installerHeader.Filename,
		Size:       meta.Size,
		Sha512:     meta.Sha512,
		Notes:      meta.Notes,
		UploadedBy: sc.UserID,
	}, installer)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// .blockmap 可选，但一旦提供就必须成功 —— 否则回滚整次上传，避免留下
	// 「看起来装好了、实际差分下载是坏的」的半成品（全量下载虽能兜住，
	// 但一个缺件的 draft 会被误当成完整包发布出去）。
	if bmHeader, bmErr := c.FormFile("blockmap"); bmErr == nil {
		bm, openErr := bmHeader.Open()
		if openErr != nil {
			_ = h.store.DeleteDesktopRelease(rel.ID)
			transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "cannot read uploaded blockmap"))
			return
		}
		defer bm.Close()
		updated, bmAppErr := h.store.SetDesktopReleaseBlockMap(rel.ID, bm)
		if bmAppErr != nil {
			_ = h.store.DeleteDesktopRelease(rel.ID)
			transport.WriteError(c, bmAppErr)
			return
		}
		rel = updated
	}

	recordAudit(c, h.store, model.AuditScopePlatform, model.AuditActionDesktopReleaseUpload,
		model.AuditTargetDesktopRelease, rel.Version, map[string]any{
			"release_id": rel.ID,
			"version":    rel.Version,
			"file_name":  rel.FileName,
			"size":       rel.Size,
			"sha512":     rel.Sha512,
		})
	transport.WriteData(c, 201, gin.H{"desktop_release": rel})
}

func readDesktopReleaseMetadata(c *gin.Context, installerName string) (desktopReleaseMetadata, *transport.AppError) {
	var meta desktopReleaseMetadata
	if fh, err := c.FormFile("metadata"); err == nil {
		f, openErr := fh.Open()
		if openErr != nil {
			return meta, transport.BadRequest("BAD_REQUEST", "cannot read release.json")
		}
		defer f.Close()
		raw, readErr := io.ReadAll(io.LimitReader(f, 1<<20))
		if readErr != nil {
			return meta, transport.BadRequest("BAD_REQUEST", "cannot read release.json")
		}
		if jsonErr := json.Unmarshal(raw, &meta); jsonErr != nil {
			return meta, transport.Validation("invalid release.json", map[string]any{
				"metadata": "must be json with version/file/size/sha512/notes",
			})
		}
	} else {
		// 表单字段兜底：便于脚本或 curl 手工上传（不依赖 release.json 文件）。
		meta.Version = c.PostForm("version")
		meta.Sha512 = c.PostForm("sha512")
		meta.Notes = c.PostForm("notes")
		if v := strings.TrimSpace(c.PostForm("size")); v != "" {
			if n, parseErr := strconv.ParseInt(v, 10, 64); parseErr == nil {
				meta.Size = n
			}
		}
	}

	version := strings.TrimSpace(meta.Version)
	if version == "" {
		return meta, transport.Validation("missing version", map[string]any{"version": "required"})
	}
	if strings.TrimSpace(meta.Sha512) == "" {
		return meta, transport.Validation("missing sha512", map[string]any{"sha512": "required"})
	}
	// 自报文件名必须与实际上传的文件一致：不一致几乎总是「拿 0.3.0 的包配 0.4.0 的
	// release.json」这类配错，放过去就会让全体客户端下载到错误版本。
	if f := strings.TrimSpace(meta.File); f != "" && f != installerName {
		return meta, transport.Validation("release.json file name does not match the uploaded installer", map[string]any{
			"metadata.file": f,
			"uploaded_file": installerName,
		})
	}
	if !strings.Contains(installerName, version) {
		return meta, transport.Validation("installer file name does not contain the version", map[string]any{
			"version":       version,
			"uploaded_file": installerName,
		})
	}
	meta.Version = version
	meta.File = installerName
	return meta, nil
}

// Publish 把发布指针前移到该版本（幂等），并顺带把版本数收敛到保留上限。
func (h *PlatformDesktopReleaseHandler) Publish(c *gin.Context) {
	rel, appErr := h.store.PublishDesktopRelease(c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, model.AuditScopePlatform, model.AuditActionDesktopReleasePublish,
		model.AuditTargetDesktopRelease, rel.Version, map[string]any{
			"release_id": rel.ID,
			"version":    rel.Version,
		})
	transport.WriteData(c, 200, gin.H{
		"desktop_release": rel,
		"pruned":          h.store.PruneDesktopReleases(),
	})
}

// Rollback 把发布指针后移到旧版本。
//
// 语义边界（方案 §1 决策 5）：只影响**尚未升级到更高版本**的客户端；已经升上去的
// 机器不受影响 —— electron-updater 只认「远程 version > 本地 version」，没有降级通道。
// 前端必须把这句写进二次确认文案，否则管理员会以为回滚能把所有人拉回来。
func (h *PlatformDesktopReleaseHandler) Rollback(c *gin.Context) {
	rel, appErr := h.store.RollbackDesktopRelease(c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, model.AuditScopePlatform, model.AuditActionDesktopReleaseRollback,
		model.AuditTargetDesktopRelease, rel.Version, map[string]any{
			"release_id": rel.ID,
			"version":    rel.Version,
		})
	transport.WriteData(c, 200, gin.H{"desktop_release": rel})
}

// Delete 删除一条发行版（记录 + 磁盘文件）。当前 published 不可删（见 store）。
func (h *PlatformDesktopReleaseHandler) Delete(c *gin.Context) {
	rel, appErr := h.store.GetDesktopRelease(c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if appErr := h.store.DeleteDesktopRelease(rel.ID); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, model.AuditScopePlatform, model.AuditActionDesktopReleaseDelete,
		model.AuditTargetDesktopRelease, rel.Version, map[string]any{
			"release_id": rel.ID,
			"version":    rel.Version,
		})
	transport.WriteData(c, 200, gin.H{"deleted": true})
}

// LatestYML 按当前 published 动态生成 electron-builder 兼容的 latest.yml。
//
// 为什么手写 YAML 而不引 yaml.Marshal：本仓构建门禁跑在 GOPROXY=off 的离线容器里
// （docs 部署铁律），新增依赖会直接打穿构建。此处所有字段都已被上游约束
// （semver / 校验过的可打印 ASCII 文件名 / base64 / 整数 / ISO 时间），
// 且全部值**加双引号**输出，构成 YAML 安全子集。
func (h *PlatformDesktopReleaseHandler) LatestYML(c *gin.Context) {
	feed, appErr := h.store.FeedDesktopRelease()
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	// no-store：发布/回滚之后必须立刻生效。任何中间缓存都会让「回滚了但客户端还在拿新版」
	// 变成一个需要人工排查的幽灵问题。
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/yaml; charset=utf-8", []byte(RenderLatestYML(feed)))
}

// RenderLatestYML 由 feed 投影渲染 electron-builder 兼容的 latest.yml。
//
// 抽成纯函数是为了**可单测**：这份 YAML 是更新链路的唯一协议面，
// 换行/缩进/字段名任何一处出错，客户端只会表现为「没有更新」，没有任何报错线索。
//
// 为什么手写 YAML 而不引 yaml.Marshal：本仓构建门禁跑在 GOPROXY=off 的离线容器里
// （docs 部署铁律），新增依赖会直接打穿构建。此处所有字段都已被上游约束
// （semver / 校验过的可打印 ASCII 文件名 / base64 / 整数 / ISO 时间），
// 且全部值**加双引号**输出，构成 YAML 安全子集。
func RenderLatestYML(feed *store.DesktopReleaseFeed) string {
	if feed == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("version: " + yamlQuote(feed.Version) + "\n")
	b.WriteString("files:\n")
	b.WriteString("  - url: " + yamlQuote(feed.FileName) + "\n")
	b.WriteString("    sha512: " + yamlQuote(feed.Sha512) + "\n")
	fmt.Fprintf(&b, "    size: %d\n", feed.Size)
	if feed.BlockMapSize > 0 {
		fmt.Fprintf(&b, "    blockMapSize: %d\n", feed.BlockMapSize)
	}
	b.WriteString("path: " + yamlQuote(feed.FileName) + "\n")
	b.WriteString("sha512: " + yamlQuote(feed.Sha512) + "\n")
	b.WriteString("releaseDate: " + yamlQuote(feed.ReleaseDate.UTC().Format("2006-01-02T15:04:05.000Z")) + "\n")
	return b.String()
}

// DownloadReleaseFile 以字节流返回安装包或 .blockmap。
//
// 🔴 必须支持 HTTP Range：electron-updater 的差分下载靠 .blockmap + Range 拉取差异块，
// 不支持 Range 时它不会报错，只会静默退化为每次全量 87MB（方案 §8 风险项）。
// 用 http.ServeContent 而不是自研 Range —— Range / If-Range / 416 / Accept-Ranges
// 这一组边界（尤其 `bytes=-N` 与多区间请求）自研几乎必错。
func (h *PlatformDesktopReleaseHandler) DownloadReleaseFile(c *gin.Context) {
	name := c.Param("filename")
	path, appErr := h.store.ResolveDesktopReleaseFilePath(name)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		// 记录在库、文件不在盘 = 数据不一致，按 NotFound 回复（更新端会当「本次无更新」）。
		transport.WriteError(c, transport.NotFound("desktop release file not found"))
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		transport.WriteError(c, transport.NewError(http.StatusInternalServerError, "INTERNAL_ERROR", "cannot stat release file"))
		return
	}
	c.Header("Cache-Control", "no-cache")
	// 显式给 octet-stream：否则 ServeContent 会按 .exe 后缀去 mime 表里猜。
	c.Header("Content-Type", "application/octet-stream")
	http.ServeContent(c.Writer, c.Request, name, fi.ModTime(), f)
}

// yamlQuote 输出一个 YAML 双引号标量。上游已挡住引号与反斜杠（见
// store.validateDesktopReleaseFileName），这里再转义一次，使本函数在任何输入下都安全。
func yamlQuote(v string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
}
