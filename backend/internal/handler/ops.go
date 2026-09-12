package handler

import (
	"errors"
	"io"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/metrics"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// OpsHandler 暴露运维工单查询与人工干预端点（一期第 4 批）。
// 查询吃租户 Scope（X-Org-Id，无头退回 user 维度）；忽略/关闭是人工动作，
// 只对活跃工单有效，操作即留痕（OpsAction）。
type OpsHandler struct {
	store *store.Store
}

func NewOpsHandler(s *store.Store) *OpsHandler {
	return &OpsHandler{store: s}
}

// enrichOpsIncident 填充展示辅助字段（任务标题/项目名/Agent 名）。
// 工单可见性已在 store 层裁决，关联资源与工单同租户，反查名称不会跨租户泄漏；
// 名称反查全走内存 map（GetTaskInternal 等），miss 时留空——前端回退显示 ID。
func (h *OpsHandler) enrichOpsIncident(sc store.Scope, inc *model.OpsIncident) {
	if inc == nil {
		return
	}
	if inc.TaskID != "" {
		if t := h.store.GetTaskInternal(inc.TaskID); t != nil {
			inc.TaskTitle = t.Title
			// 工单未回填 project_id 时从任务补（老工单兼容）
			if inc.ProjectID == "" {
				inc.ProjectID = t.ProjectID
			}
		}
	}
	if inc.ProjectID != "" {
		if p, err := h.store.GetProject(sc, inc.ProjectID); err == nil && p != nil {
			inc.ProjectName = p.Name
		}
	}
	if inc.AgentID != "" {
		if a, err := h.store.GetAgent(sc, inc.AgentID); err == nil && a != nil {
			inc.AgentName = a.Name
		}
	}
}

// List GET /ops/incidents?status=active|terminal|<具体状态>
// 默认全量（updated_at 倒序，store 层保证）；status 过滤在 handler 层做，
// 统计口径与列表口径一致：过滤不改变可见性裁决（裁决在 store 层）。
func (h *OpsHandler) List(c *gin.Context) {
	sc := middleware.Scope(c)
	items := h.store.ListOpsIncidents(sc)
	status := c.Query("status")
	if status != "" && status != "all" {
		filtered := make([]*model.OpsIncident, 0, len(items))
		for _, inc := range items {
			switch status {
			case "active":
				if inc.IsActive() {
					filtered = append(filtered, inc)
				}
			case "terminal":
				if inc.IsTerminal() {
					filtered = append(filtered, inc)
				}
			default:
				if inc.Status == status {
					filtered = append(filtered, inc)
				}
			}
		}
		items = filtered
	}
	for _, inc := range items {
		h.enrichOpsIncident(sc, inc)
	}
	transport.WriteData(c, 200, gin.H{"incidents": items})
}

// Get GET /ops/incidents/:id
func (h *OpsHandler) Get(c *gin.Context) {
	sc := middleware.Scope(c)
	inc := h.store.GetOpsIncident(sc, c.Param("id"))
	if inc == nil {
		transport.WriteError(c, transport.NotFound("ops incident not found"))
		return
	}
	h.enrichOpsIncident(sc, inc)
	transport.WriteData(c, 200, gin.H{"incident": inc})
}

type opsManualActionRequest struct {
	Reason string `json:"reason"`
}

// Ignore POST /ops/incidents/:id/ignore — 人工忽略，之后不再自动干预该问题。
func (h *OpsHandler) Ignore(c *gin.Context) {
	sc := middleware.Scope(c)
	var req opsManualActionRequest
	// body 可选：无理由时用默认文案（io.EOF = 无 body，属合法调用）。
	// 但畸形 JSON 必须 400 —— 人工干预会落 OpsAction 留痕，
	// 静默吞错会让 reason 被静默丢弃且不可见。
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		transport.WriteError(c, transport.Validation("invalid ops ignore payload", map[string]any{"body": "malformed json"}))
		return
	}
	inc, appErr := h.store.IgnoreOpsIncident(sc, c.Param("id"), req.Reason)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"incident": inc})
}

// Close POST /ops/incidents/:id/close — 人工关闭，语义等同 resolved。
func (h *OpsHandler) Close(c *gin.Context) {
	sc := middleware.Scope(c)
	var req opsManualActionRequest
	// 同 Ignore：body 可选（io.EOF 合法），畸形 JSON 必须 400。
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		transport.WriteError(c, transport.Validation("invalid ops close payload", map[string]any{"body": "malformed json"}))
		return
	}
	inc, appErr := h.store.CloseOpsIncident(sc, c.Param("id"), req.Reason)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"incident": inc})
}

// Metrics GET /ops/metrics — Phase 0 可观测性三件套（T0.11）的只读出口：
// 计数器快照 + 直方图（步骤推进/静默时长）+ 告警规则清单。
//
// 指标是进程级聚合值（不含任何资源标识、任务名或用户），但对普通成员暴露
// 平台整体体量仍不合适，因此要求「带租户上下文且角色为 owner/admin」。
// 个人空间（无 X-Org-Id）拿不到指标 —— 需要观测时以企业身份访问。
func (h *OpsHandler) Metrics(c *gin.Context) {
	sc := middleware.Scope(c)
	if !isOrgAdminScope(sc) {
		transport.WriteError(c, transport.Forbidden("ops metrics require org owner/admin role"))
		return
	}
	histograms := make([]metrics.HistogramReport, 0, len(metrics.KnownCounterNames()))
	for _, name := range metrics.KnownCounterNames() {
		histograms = append(histograms, metrics.Histogram(name))
	}
	transport.WriteData(c, 200, gin.H{
		"counters":   metrics.Snapshot(),
		"histograms": histograms,
		"alerts":     metrics.AlertRules,
	})
}

// isOrgAdminScope reports whether the request carries an org context with an
// owner/admin role. Used to gate platform-wide (cross-tenant) read endpoints.
func isOrgAdminScope(sc store.Scope) bool {
	if !sc.HasOrg() {
		return false
	}
	return sc.Role == model.OrgRoleOwner || sc.Role == model.OrgRoleAdmin
}
