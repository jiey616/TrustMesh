package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/clawsynapse"
	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

type JoinRequestHandler struct {
	store      *store.Store
	clawClient *clawsynapse.Client
	cfg        config.Config
}

func NewJoinRequestHandler(s *store.Store, clawClient *clawsynapse.Client, cfg config.Config) *JoinRequestHandler {
	return &JoinRequestHandler{store: s, clawClient: clawClient, cfg: cfg}
}

func (h *JoinRequestHandler) GetInvitePrompt(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	userID := sc.UserID

	if h.clawClient == nil {
		transport.WriteError(c, &transport.AppError{
			Status:  http.StatusServiceUnavailable,
			Code:    "CLAWSYNAPSE_UNAVAILABLE",
			Message: "暂时无法获取当前节点身份",
			Details: map[string]any{},
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.cfg.ClawSynapseTimeout)
	defer cancel()

	nodeID, err := h.clawClient.GetSelfNodeID(ctx)
	if err != nil {
		transport.WriteError(c, &transport.AppError{
			Status:  http.StatusServiceUnavailable,
			Code:    "CLAWSYNAPSE_UNAVAILABLE",
			Message: "暂时无法获取当前节点身份",
			Details: map[string]any{"cause": err.Error()},
		})
		return
	}

	// 多租户：企业空间下招聘 → reason 模板锁定 org_id + 注入企业语境；
	// 个人空间（无 X-Org-Id）提示词与 reason 均不含 org，行为与改造前一致。
	// 归属在发起招聘时锁定：节点只透传 reason（不感知内容），
	// sync 落库时校验后写 jr.OrgID，审批时 agent 继承该归属。
	reasonJSON := fmt.Sprintf(`'{"name":"<你的名称>","description":"<能力简述>","role":"developer","agent_product":"<产品标识>","user_id":"%s"}'`, userID)
	orgIDDoc := ""
	enterpriseSection := ""
	if sc.OrgID != "" {
		reasonJSON = fmt.Sprintf(`'{"name":"<你的名称>","description":"<能力简述>","role":"developer","agent_product":"<产品标识>","user_id":"%s","org_id":"%s"}'`, userID, sc.OrgID)
		orgIDDoc = "- org_id: 不要修改此字段（本次招聘锁定的目标企业）\n"
		orgName := sc.OrgID
		if org, orgErr := h.store.GetOrganization(sc.OrgID); orgErr == nil && org != nil && strings.TrimSpace(org.Name) != "" {
			orgName = org.Name
		}
		enterpriseSection = fmt.Sprintf(`

## 本次招聘归属
你正在被企业「%s」招聘。审批通过后，你将加入该企业（org_id: %s），参与其下的项目、任务与会议室协作。`, orgName, sc.OrgID)
	}

	prompt := fmt.Sprintf(`你被邀请加入 TrustMesh 工作平台协作网络。请按以下步骤操作：

## 1. 身份认证
clawsynapse auth challenge --target %s

## 2. 发送信任申请
clawsynapse trust request --target %s \
  --reason %s \
  --capability planning --capability task --capability todo

请根据实际情况填写 reason 中的 JSON 字段：
- name: 你的显示名称
- description: 简要描述你的能力和职责
- role: 选择 pm / developer / reviewer / custom
- agent_product: 你的产品标识（如 openclaw）
- user_id: 不要修改此字段（招聘发起人标识）
%s
--capability 参数声明你支持的消息类型，保持上述默认值即可。
%s
发送后等待平台管理员审批，审批通过后你将成为 TrustMesh 的协作 Agent。`, nodeID, nodeID, reasonJSON, orgIDDoc, enterpriseSection)

	transport.WriteData(c, http.StatusOK, gin.H{
		"prompt":  prompt,
		"node_id": nodeID,
	})
}

func (h *JoinRequestHandler) List(c *gin.Context) {
	sc := middleware.Scope(c)
	status := strings.TrimSpace(c.Query("status"))
	items := h.store.ListJoinRequests(sc, status)
	transport.WriteList(c, items, len(items))
}

type approveJoinRequestRequest struct {
	Name         *string  `json:"name,omitempty"`
	Role         *string  `json:"role,omitempty"`
	Description  *string  `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

func (h *JoinRequestHandler) Approve(c *gin.Context) {
	requestID := c.Param("id")

	var req approveJoinRequestRequest
	_ = c.ShouldBindJSON(&req)

	// Get the join request to find trust request ID
	jr, appErr := h.store.GetJoinRequest(middleware.Scope(c), requestID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Authenticate and approve trust in ClawSynapse
	if h.clawClient != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), h.cfg.ClawSynapseTimeout)
		defer cancel()

		// Step 1: Auth challenge with the requesting node
		if err := h.clawClient.AuthChallenge(ctx, jr.NodeID); err != nil {
			transport.WriteError(c, &transport.AppError{
				Status:  http.StatusBadGateway,
				Code:    "CLAWSYNAPSE_AUTH_ERROR",
				Message: "failed to authenticate with node",
				Details: map[string]any{"node_id": jr.NodeID, "cause": err.Error()},
			})
			return
		}

		// Step 2: Approve trust request
		if err := h.clawClient.ApproveTrustRequest(ctx, jr.TrustRequestID, "approved by TrustMesh"); err != nil {
			transport.WriteError(c, &transport.AppError{
				Status:  http.StatusBadGateway,
				Code:    "CLAWSYNAPSE_ERROR",
				Message: "failed to approve trust request in ClawSynapse",
				Details: map[string]any{"cause": err.Error()},
			})
			return
		}
	}

	// Approve in store and create agent
	agent, appErr := h.store.ApproveJoinRequest(middleware.Scope(c), requestID, store.JoinRequestOverrides{
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

func (h *JoinRequestHandler) Reject(c *gin.Context) {
	requestID := c.Param("id")

	// Get the join request to find trust request ID
	jr, appErr := h.store.GetJoinRequest(middleware.Scope(c), requestID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Reject trust in ClawSynapse first
	if h.clawClient != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), h.cfg.ClawSynapseTimeout)
		defer cancel()
		if err := h.clawClient.RejectTrustRequest(ctx, jr.TrustRequestID, "rejected by TrustMesh"); err != nil {
			transport.WriteError(c, &transport.AppError{
				Status:  http.StatusBadGateway,
				Code:    "CLAWSYNAPSE_ERROR",
				Message: "failed to reject trust request in ClawSynapse",
				Details: map[string]any{"cause": err.Error()},
			})
			return
		}
	}

	if appErr := h.store.RejectJoinRequest(middleware.Scope(c), requestID); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	c.Status(http.StatusNoContent)
}
