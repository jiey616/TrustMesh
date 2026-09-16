package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/clawsynapse"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

type AgentHandler struct {
	store      *store.Store
	clawClient *clawsynapse.Client
}

func NewAgentHandler(s *store.Store, clawClient *clawsynapse.Client) *AgentHandler {
	return &AgentHandler{store: s, clawClient: clawClient}
}

type createAgentRequest struct {
	NodeID       string   `json:"node_id"`
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
}

func (h *AgentHandler) Create(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var req createAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	if nodeID := strings.TrimSpace(req.NodeID); nodeID != "" {
		if appErr := h.ensureNodeOnline(c.Request.Context(), nodeID); appErr != nil {
			transport.WriteError(c, appErr)
			return
		}
	}

	agent, appErr := h.store.CreateAgent(sc, req.NodeID, req.Name, req.Role, req.Description, req.Capabilities)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusCreated, agent)
}

func (h *AgentHandler) List(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	items := h.store.ListAgents(sc)
	transport.WriteList(c, items, len(items))
}

func (h *AgentHandler) Get(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	agent, appErr := h.store.GetAgent(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, agent)
}

type updateAgentRequest struct {
	Name         *string   `json:"name"`
	Role         *string   `json:"role"`
	Description  *string   `json:"description"`
	Capabilities *[]string `json:"capabilities"`
	NodeID       *string   `json:"node_id"`
}

func (h *AgentHandler) Update(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var req updateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	if req.NodeID != nil {
		transport.WriteError(c, transport.Validation("node_id is immutable", map[string]any{"node_id": "not allowed to update"}))
		return
	}

	agent, appErr := h.store.UpdateAgent(sc, c.Param("id"), store.UpdateAgentInput{
		Name:         req.Name,
		Role:         req.Role,
		Description:  req.Description,
		Capabilities: req.Capabilities,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, agent)
}

func (h *AgentHandler) Delete(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	agentID := c.Param("id")

	// Get agent info before deletion to obtain node_id
	agent, appErr := h.store.GetAgent(sc, agentID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	if appErr := h.store.DeleteAgent(sc, agentID); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Revoke trust in ClawSynapse (best-effort, don't block deletion)
	if h.clawClient != nil && agent.NodeID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.clawClient.RevokeTrust(ctx, agent.NodeID, "agent removed from TrustMesh")
	}

	recordAudit(c, h.store, agent.OrgID, model.AuditActionAgentDelete, model.AuditTargetAgent, agent.ID,
		map[string]any{"name": agent.Name, "node_id": agent.NodeID})

	c.Status(http.StatusNoContent)
}

func (h *AgentHandler) Stats(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	stats, appErr := h.store.GetAgentStats(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, stats)
}

func (h *AgentHandler) Insights(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	insights, appErr := h.store.GetAgentInsights(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, insights)
}

func (h *AgentHandler) Tasks(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	status := c.Query("status")
	items, appErr := h.store.ListAgentTasks(sc, c.Param("id"), status)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteList(c, items, len(items))
}

// GetCapabilities 查询 Agent 节点的能力（技能/模型/cron）。
// 仅 hermes 产品节点提供能力查询；非 hermes 返回 available:false 由前端降级。
// 依赖 ClawSynapse 侧 capability 模块 + 旁挂 daemon 端点；未就绪时同样降级，不影响主流程。
func (h *AgentHandler) GetCapabilities(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	agent, appErr := h.store.GetAgent(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// 仅 hermes 节点提供能力展示（与前端 Tab 显示条件一致）
	if agent.Product != "hermes" {
		transport.WriteData(c, http.StatusOK, map[string]any{
			"available": false,
			"reason":    "not a hermes node",
		})
		return
	}

	if h.clawClient == nil {
		transport.WriteData(c, http.StatusOK, map[string]any{
			"available": false,
			"reason":    "clawsynapse client disabled",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	info, err := h.clawClient.GetCapabilities(ctx, agent.NodeID)
	if err != nil {
		transport.WriteData(c, http.StatusOK, map[string]any{
			"available": false,
			"reason":    err.Error(),
		})
		return
	}
	transport.WriteData(c, http.StatusOK, info)
}

// ListCronExecutions 查询 Agent 节点定时任务的执行历史（capability.executions 契约）。
// 仅 hermes 产品节点；jobId 可选（空返回全部），limit 可选（默认 20，daemon 侧钳制上限 100）。
// 超时/节点不支持时仍返回 200 + 空 executions + error 字段。
func (h *AgentHandler) ListCronExecutions(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	agent, appErr := h.store.GetAgent(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	if agent.Product != "hermes" {
		transport.WriteData(c, http.StatusOK, clawsynapse.CronExecutionsResult{
			Executions: []clawsynapse.ExecutionInfo{},
			Error:      "not a hermes node",
		})
		return
	}

	if h.clawClient == nil {
		transport.WriteData(c, http.StatusOK, clawsynapse.CronExecutionsResult{
			Executions: []clawsynapse.ExecutionInfo{},
			Error:      "clawsynapse client disabled",
		})
		return
	}

	jobID := strings.TrimSpace(c.Query("jobId"))
	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	result, err := h.clawClient.GetCronExecutions(ctx, agent.NodeID, jobID, limit)
	if err != nil {
		transport.WriteData(c, http.StatusOK, clawsynapse.CronExecutionsResult{
			Executions: []clawsynapse.ExecutionInfo{},
			Error:      err.Error(),
		})
		return
	}
	transport.WriteData(c, http.StatusOK, result)
}

// SetCapabilities 写回 Agent 节点的能力（技能/模型/cron）。
// 仅 hermes 产品节点可写回；参数透传给 daemon 的 capability.set。
func (h *AgentHandler) SetCapabilities(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	agent, appErr := h.store.GetAgent(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	if agent.Product != "hermes" {
		transport.WriteData(c, http.StatusOK, clawsynapse.SetCapabilityResult{
			OK:     false,
			Error:  "not a hermes node",
			Target: "",
		})
		return
	}

	var req clawsynapse.SetCapabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	if strings.TrimSpace(req.Target) == "" || strings.TrimSpace(req.Action) == "" {
		transport.WriteError(c, transport.Validation("target and action are required", map[string]any{
			"target": "required",
			"action": "required",
		}))
		return
	}

	if h.clawClient == nil {
		transport.WriteData(c, http.StatusOK, clawsynapse.SetCapabilityResult{
			OK:     false,
			Error:  "clawsynapse client disabled",
			Target: req.Target,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	result, err := h.clawClient.SetCapabilities(ctx, agent.NodeID, &req)
	if err != nil {
		transport.WriteData(c, http.StatusOK, clawsynapse.SetCapabilityResult{
			OK:     false,
			Error:  err.Error(),
			Target: req.Target,
		})
		return
	}
	transport.WriteData(c, http.StatusOK, result)
}

// UploadSkillFile 上传技能文件包到目标节点，返回 fileId 供后续写回 skill 时引用。
func (h *AgentHandler) UploadSkillFile(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	agent, appErr := h.store.GetAgent(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	if agent.Product != "hermes" {
		transport.WriteError(c, transport.Validation("skill upload only for hermes nodes", map[string]any{"product": agent.Product}))
		return
	}

	applyUploadLimit(c)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "file is required"))
		return
	}
	defer file.Close()

	if !validateFileExtension(header.Filename) {
		transport.WriteError(c, transport.BadRequest("UNSUPPORTED_FILE_TYPE", "不支持的文件类型。技能包应为 zip 压缩包或代码/文档文件。"))
		return
	}

	if h.clawClient == nil {
		transport.WriteError(c, transport.NewError(http.StatusServiceUnavailable, "CLAWSYNAPSE_UNAVAILABLE", "clawsynapse client disabled"))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	fileID, err := h.clawClient.UploadSkillFile(ctx, agent.NodeID, header.Filename, file)
	if err != nil {
		transport.WriteError(c, transport.NewError(http.StatusBadGateway, "UPLOAD_FAILED", "技能文件上传失败: "+err.Error()))
		return
	}
	transport.WriteData(c, http.StatusOK, map[string]any{"fileId": fileID})
}

func (h *AgentHandler) ensureNodeOnline(ctx context.Context, nodeID string) *transport.AppError {
	if h.clawClient == nil {
		err := transport.NewError(http.StatusServiceUnavailable, "CLAWSYNAPSE_UNAVAILABLE", "暂时无法校验节点在线状态")
		err.Details = map[string]any{"node_id": nodeID}
		return err
	}

	peers, err := h.clawClient.GetPeers(ctx)
	if err != nil {
		appErr := transport.NewError(http.StatusServiceUnavailable, "CLAWSYNAPSE_UNAVAILABLE", "暂时无法校验节点在线状态")
		appErr.Details = map[string]any{
			"node_id": nodeID,
			"cause":   err.Error(),
		}
		return appErr
	}

	for _, peer := range peers {
		if strings.TrimSpace(peer.NodeID) == nodeID {
			return nil
		}
	}

	return transport.Validation("node_id 必须对应一个在线中的 ClawSynapse 节点", map[string]any{
		"node_id": "offline_or_not_found",
	})
}
