package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/assistant"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// LLMConfigHandler 暴露平台/租户两级 LLM 配置（A1+B2+D1）：
//   - 平台默认层：仅平台管理员（IsAdmin）可读写；
//   - 租户覆盖层：该租户 owner/admin 可读写；
//   - key 全程 write-only，接口只回掩码；保存即时生效（C1），无需重启。
type LLMConfigHandler struct {
	store *store.Store
}

func NewLLMConfigHandler(s *store.Store) *LLMConfigHandler {
	return &LLMConfigHandler{store: s}
}

// GetPlatform GET /platform/llm-config — 平台默认层视图（未配置时展示 env 兜底值）。
func (h *LLMConfigHandler) GetPlatform(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if !h.store.UserIsPlatformAdmin(userID) {
		transport.WriteError(c, transport.Forbidden("platform admin only"))
		return
	}
	transport.WriteData(c, 200, gin.H{"config": h.store.GetPlatformLLMConfigView()})
}

type putLLMConfigRequest struct {
	APIURL      string `json:"api_url"`
	APIKey      string `json:"api_key"`
	Model       string `json:"model"`
	OpsModel    string `json:"ops_model"`
	ResetAPIKey bool   `json:"reset_api_key"`
}

// PutPlatform PUT /platform/llm-config — 保存平台默认配置。
func (h *LLMConfigHandler) PutPlatform(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if !h.store.UserIsPlatformAdmin(userID) {
		transport.WriteError(c, transport.Forbidden("platform admin only"))
		return
	}
	var req putLLMConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	view, appErr := h.store.SetPlatformLLMSetting(userID, model.LLMConfigInput{
		APIURL: req.APIURL, APIKey: req.APIKey, Model: req.Model,
		OpsModel: req.OpsModel, ResetAPIKey: req.ResetAPIKey,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"config": view})
}

// DeletePlatform DELETE /platform/llm-config — 删除平台默认配置，回落 env。
func (h *LLMConfigHandler) DeletePlatform(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if !h.store.UserIsPlatformAdmin(userID) {
		transport.WriteError(c, transport.Forbidden("platform admin only"))
		return
	}
	if appErr := h.store.ClearLLMSetting(""); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"deleted": true})
}

// GetOrg GET /organizations/:id/llm-config — 租户层视图。
func (h *LLMConfigHandler) GetOrg(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	orgID := c.Param("id")
	if !h.store.UserIsOrgAdmin(userID, orgID) {
		transport.WriteError(c, transport.Forbidden("org owner/admin only"))
		return
	}
	transport.WriteData(c, 200, gin.H{"config": h.store.GetOrgLLMConfigView(orgID)})
}

// PutOrg PUT /organizations/:id/llm-config — 保存租户覆盖配置。
func (h *LLMConfigHandler) PutOrg(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	orgID := c.Param("id")
	if !h.store.UserIsOrgAdmin(userID, orgID) {
		transport.WriteError(c, transport.Forbidden("org owner/admin only"))
		return
	}
	var req putLLMConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	view, appErr := h.store.SetOrgLLMSetting(orgID, userID, model.LLMConfigInput{
		APIURL: req.APIURL, APIKey: req.APIKey, Model: req.Model,
		OpsModel: req.OpsModel, ResetAPIKey: req.ResetAPIKey,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"config": view})
}

// DeleteOrg DELETE /organizations/:id/llm-config — 删除租户覆盖，回落平台默认。
func (h *LLMConfigHandler) DeleteOrg(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	orgID := c.Param("id")
	if !h.store.UserIsOrgAdmin(userID, orgID) {
		transport.WriteError(c, transport.Forbidden("org owner/admin only"))
		return
	}
	if appErr := h.store.ClearLLMSetting(orgID); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"deleted": true})
}

// Test POST /llm-config/test — 连接测试。字段全部可选：
// 提供了就用提交值测（测未保存的表单值），没提供就测解析后的生效配置。
// 权限：测平台层（org_id 空）需平台管理员；测租户层需该租户 owner/admin。
func (h *LLMConfigHandler) Test(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req model.LLMConfigTestRequest
	_ = c.ShouldBindJSON(&req)

	if req.OrgID == "" {
		if !h.store.UserIsPlatformAdmin(userID) {
			transport.WriteError(c, transport.Forbidden("platform admin only"))
			return
		}
	} else if !h.store.UserIsOrgAdmin(userID, req.OrgID) {
		transport.WriteError(c, transport.Forbidden("org owner/admin only"))
		return
	}

	// 组装测试参数：提交值优先，缺省回落生效配置。
	apiURL, apiKey, chatModel := req.APIURL, req.APIKey, req.Model
	source := "request"
	if apiURL == "" || apiKey == "" || chatModel == "" {
		u, k, _, om, src := h.store.ResolveLLMParams(req.OrgID)
		if apiURL == "" {
			apiURL = u
		}
		if apiKey == "" {
			apiKey = k
		}
		if chatModel == "" {
			chatModel = om
		}
		source = src
	}
	if apiURL == "" || apiKey == "" || chatModel == "" {
		transport.WriteData(c, 200, gin.H{"test": gin.H{
			"ok": false, "error": "LLM 未配置（无可用 url/key/model）", "source": source,
		}})
		return
	}

	client := assistant.NewLLMClient(apiURL, apiKey, chatModel)
	start := time.Now()
	_, err := client.Complete(c.Request.Context(),
		"You are a connectivity probe. Reply with exactly: OK", "ping")
	latency := time.Since(start).Milliseconds()
	if err != nil {
		transport.WriteData(c, 200, gin.H{"test": gin.H{
			"ok": false, "error": err.Error(), "model": chatModel, "source": source, "latency_ms": latency,
		}})
		return
	}
	transport.WriteData(c, 200, gin.H{"test": gin.H{
		"ok": true, "model": chatModel, "source": source, "latency_ms": latency,
	}})
}
