package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/clawsynapse"
	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// inviteReason 是 GetInvitePrompt 下发给节点 CLI 的 reason JSON 结构。
// 必须用 json.Marshal 生成：原先 fmt.Sprintf 手拼 JSON，userID / orgID 中
// 一旦出现 " 或 \ 就会破坏 JSON 结构，且直接拼进 CLI 提示词存在命令注入面。
type inviteReason struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Role         string `json:"role"`
	AgentProduct string `json:"agent_product"`
	UserID       string `json:"user_id"`
	// OrgID 仅企业空间下招聘时出现（个人空间用 omitempty 保持与改造前一致）。
	OrgID string `json:"org_id,omitempty"`
}

// shellSingleQuote 把字符串包进 shell 单引号，并把内部的单引号按 POSIX
// 规则转义：先闭合当前引号，再输出被反斜杠转义的单引号，最后重新开启引号。
// json.Marshal 不会转义单引号，而 reason 是作为 --reason '...' 拼进 CLI
// 提示词的，不转义会让单引号提前闭合引号，构成命令注入面。
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

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
	reason := inviteReason{
		Name:         "<你的名称>",
		Description:  "<能力简述>",
		Role:         "developer",
		AgentProduct: "<产品标识>",
		UserID:       userID,
	}
	orgIDDoc := ""
	enterpriseSection := ""
	if sc.OrgID != "" {
		reason.OrgID = sc.OrgID
		orgIDDoc = "- org_id: 不要修改此字段（本次招聘锁定的目标企业）\n"
		orgName := sc.OrgID
		if org, orgErr := h.store.GetOrganization(sc.OrgID); orgErr == nil && org != nil && strings.TrimSpace(org.Name) != "" {
			orgName = org.Name
		}
		enterpriseSection = fmt.Sprintf(`

## 本次招聘归属
你正在被企业「%s」招聘。审批通过后，你将加入该企业（org_id: %s），参与其下的项目、任务与会议室协作。`, orgName, sc.OrgID)
	}

	// 用 json.Marshal 生成而非 fmt.Sprintf 手拼：
	//  1. userID / orgID 含 " 或 \ 时会破坏 JSON 结构（原实现的注入面）；
	//  2. reason 以 `--reason '...'` 拼进 CLI 提示词，json.Marshal 不转义
	//     单引号，故再套一层 shell 单引号转义，消除命令注入面。
	reasonBytes, marshalErr := json.Marshal(reason)
	if marshalErr != nil {
		transport.WriteError(c, &transport.AppError{
			Status:  http.StatusInternalServerError,
			Code:    "INVITE_PROMPT_BUILD_FAILED",
			Message: "failed to build invite reason",
			Details: map[string]any{"cause": marshalErr.Error()},
		})
		return
	}
	reasonJSON := shellSingleQuote(string(reasonBytes))

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
	// body 的覆盖字段全部可选，空 body 是合法调用（io.EOF 表示无 body）。
	// 但畸形 JSON 必须 400：原先用 `_ =` 静默吞错，会让请求体解析失败被
	// 当成「无覆盖」继续走审批流程，错误被推迟且不可见。
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		transport.WriteError(c, transport.Validation("invalid join request payload", map[string]any{"body": "malformed json"}))
		return
	}

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
