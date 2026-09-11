package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
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
	// 字段全部可选：空 body（io.EOF）合法，继续用零值；
	// 但畸形 JSON 必须 400，不能静默吞错。
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		transport.WriteError(c, transport.Validation("invalid llm config test payload", map[string]any{"body": "malformed json"}))
		return
	}

	// 解析层定位：personal=测当前用户个人空间配置；org=测指定租户；其余=平台默认。
	resolveOrgID, resolveUserID := "", ""
	switch req.Scope {
	case "personal":
		resolveUserID = userID // 任何登录用户可测自己的个人空间
	case "org":
		if req.OrgID == "" {
			transport.WriteError(c, transport.Validation("org_id required for org scope", nil))
			return
		}
		if !h.store.UserIsOrgAdmin(userID, req.OrgID) {
			transport.WriteError(c, transport.Forbidden("org owner/admin only"))
			return
		}
		resolveOrgID = req.OrgID
	default: // platform
		if !h.store.UserIsPlatformAdmin(userID) {
			transport.WriteError(c, transport.Forbidden("platform admin only"))
			return
		}
	}

	// 组装测试参数：提交值优先，缺省回落生效配置。
	apiURL, apiKey, chatModel := req.APIURL, req.APIKey, req.Model
	source := "request"
	if apiURL == "" || apiKey == "" || chatModel == "" {
		u, k, _, om, src := h.store.ResolveLLMParams(resolveOrgID, resolveUserID)
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

// resolveScopeForWrite 复用 Test 的 scope→权限→解析定位逻辑（供 Models 用）。
func (h *LLMConfigHandler) resolveScopeForWrite(userID string, req *model.LLMConfigTestRequest) (orgID, resUserID string, appErr *transport.AppError) {
	switch req.Scope {
	case "personal":
		return "", userID, nil
	case "org":
		if req.OrgID == "" {
			return "", "", transport.Validation("org_id required for org scope", nil)
		}
		if !h.store.UserIsOrgAdmin(userID, req.OrgID) {
			return "", "", transport.Forbidden("org owner/admin only")
		}
		return req.OrgID, "", nil
	default:
		if !h.store.UserIsPlatformAdmin(userID) {
			return "", "", transport.Forbidden("platform admin only")
		}
		return "", "", nil
	}
}

// Models POST /llm-config/models — 拉取 provider 的可用模型列表（OpenAI 兼容
// GET {api_url}/models）。字段全部可选：提交了 url/key 就用提交值（拉未保存
// 表单对应 provider 的列表），否则用解析后的生效配置。权限同 Test。
func (h *LLMConfigHandler) Models(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req model.LLMConfigTestRequest
	// 字段全部可选：空 body（io.EOF）合法；畸形 JSON 必须 400。
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		transport.WriteError(c, transport.Validation("invalid llm config models payload", map[string]any{"body": "malformed json"}))
		return
	}
	if _, _, appErr := h.resolveScopeForWrite(userID, &req); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	apiURL, apiKey := req.APIURL, req.APIKey
	if apiURL == "" || apiKey == "" {
		orgID, resUserID, _ := h.resolveScopeForWrite(userID, &req)
		u, k, _, _, _ := h.store.ResolveLLMParams(orgID, resUserID)
		if apiURL == "" {
			apiURL = u
		}
		if apiKey == "" {
			apiKey = k
		}
	}
	if apiURL == "" || apiKey == "" {
		transport.WriteData(c, 200, gin.H{"models": gin.H{"ok": false, "error": "LLM 未配置（无可用 url/key）"}})
		return
	}

	list, err := fetchModelIDs(c.Request.Context(), apiURL, apiKey)
	if err != nil {
		transport.WriteData(c, 200, gin.H{"models": gin.H{"ok": false, "error": err.Error()}})
		return
	}
	transport.WriteData(c, 200, gin.H{"models": gin.H{"ok": true, "items": list}})
}

// fetchModelIDs 调 OpenAI 兼容的 GET {base}/models，返回模型 ID 列表。
func fetchModelIDs(ctx context.Context, apiURL, apiKey string) ([]string, error) {
	url := strings.TrimRight(apiURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("响应不是 OpenAI 兼容的模型列表: %w", err)
	}
	ids := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return nil, &errNoModels{status: resp.StatusCode}
	}
	return ids, nil
}

type errNoModels struct{ status int }

func (e *errNoModels) Error() string {
	return "provider 未返回模型列表（HTTP " + strconv.Itoa(e.status) + "），请检查 API 地址是否含 /v1"
}

// ---- 个人空间层（任何登录用户，配置挂在本人个人租户键上） ----

// GetPersonal GET /llm-config/personal
func (h *LLMConfigHandler) GetPersonal(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	transport.WriteData(c, 200, gin.H{"config": h.store.GetPersonalLLMConfigView(userID)})
}

// PutPersonal PUT /llm-config/personal
func (h *LLMConfigHandler) PutPersonal(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req putLLMConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	view, appErr := h.store.SetPersonalLLMSetting(userID, userID, model.LLMConfigInput{
		APIURL: req.APIURL, APIKey: req.APIKey, Model: req.Model,
		OpsModel: req.OpsModel, ResetAPIKey: req.ResetAPIKey,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"config": view})
}

// DeletePersonal DELETE /llm-config/personal — 清除个人配置，回退平台默认/env。
func (h *LLMConfigHandler) DeletePersonal(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	pid := h.store.PersonalOrgIDOf(userID)
	if pid == "" {
		transport.WriteData(c, 200, gin.H{"deleted": true})
		return
	}
	if appErr := h.store.ClearLLMSetting(pid); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"deleted": true})
}
