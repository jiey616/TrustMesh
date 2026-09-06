package handler

import (
	"github.com/gin-gonic/gin"
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
	transport.WriteData(c, 200, gin.H{"incident": inc})
}

type opsManualActionRequest struct {
	Reason string `json:"reason"`
}

// Ignore POST /ops/incidents/:id/ignore — 人工忽略，之后不再自动干预该问题。
func (h *OpsHandler) Ignore(c *gin.Context) {
	sc := middleware.Scope(c)
	var req opsManualActionRequest
	_ = c.ShouldBindJSON(&req) // body 可选：无理由时用默认文案
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
	_ = c.ShouldBindJSON(&req)
	inc, appErr := h.store.CloseOpsIncident(sc, c.Param("id"), req.Reason)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"incident": inc})
}
