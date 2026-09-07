package clawsynapse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	// writeClient 用于写回类请求：skill/model 写回会重启目标 gateway（契约语义），
	// 超过默认的 3s 全局超时，需独立的长超时客户端。
	writeClient *http.Client
}

type PublishResult struct {
	TargetNode string `json:"targetNode"`
	MessageID  string `json:"messageId"`
}

type Peer struct {
	NodeID       string         `json:"nodeId"`
	AgentProduct string         `json:"agentProduct"`
	Version      string         `json:"version"`
	Capabilities []string       `json:"capabilities"`
	Inbox        string         `json:"inbox"`
	AuthStatus   string         `json:"authStatus"`
	TrustStatus  string         `json:"trustStatus"`
	LastSeenMs   int64          `json:"lastSeenMs"`
	Metadata     map[string]any `json:"metadata"`
}

type publishRequest struct {
	TargetNode string         `json:"targetNode"`
	Type       string         `json:"type"`
	Message    string         `json:"message"`
	SessionKey string         `json:"sessionKey,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type publishResponse struct {
	OK      bool          `json:"ok"`
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Data    PublishResult `json:"data"`
	TS      int64         `json:"ts"`
}

type peersResponse struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Items []Peer `json:"items"`
	} `json:"data"`
	TS int64 `json:"ts"`
}

type TrustPendingItem struct {
	RequestID    string `json:"requestId"`
	From         string `json:"from"`
	To           string `json:"to"`
	Direction    string `json:"direction"`
	Reason       string `json:"reason"`
	ReceivedAtMs int64  `json:"receivedAtMs"`
}

type trustPendingResponse struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Items []TrustPendingItem `json:"items"`
	} `json:"data"`
	TS int64 `json:"ts"`
}

type trustActionRequest struct {
	RequestID string `json:"requestId"`
	Reason    string `json:"reason,omitempty"`
}

type trustActionResponse struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	TS      int64  `json:"ts"`
}

type transfersResponse struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Items []map[string]any `json:"items"`
	} `json:"data"`
	TS int64 `json:"ts"`
}

type transferResponse struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Transfer map[string]any `json:"transfer"`
	} `json:"data"`
	TS int64 `json:"ts"`
}

type HealthSelf struct {
	NodeID              string `json:"nodeId"`
	DID                 string `json:"did"`
	IdentityFingerprint string `json:"identityFingerprint"`
	TrustMode           string `json:"trustMode"`
}

type HealthAdapter struct {
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

type HealthNATS struct {
	Name             string `json:"name"`
	ServerURL        string `json:"serverUrl"`
	Connected        bool   `json:"connected"`
	Status           string `json:"status"`
	ConnectedAt      int64  `json:"connectedAt"`
	LastDisconnectAt int64  `json:"lastDisconnectAt"`
	LastReconnectAt  int64  `json:"lastReconnectAt"`
	Disconnects      int64  `json:"disconnects"`
	Reconnects       int64  `json:"reconnects"`
	LastError        string `json:"lastError"`
	InMsgs           uint64 `json:"inMsgs"`
	OutMsgs          uint64 `json:"outMsgs"`
	InBytes          uint64 `json:"inBytes"`
	OutBytes         uint64 `json:"outBytes"`
}

type HealthData struct {
	Self       HealthSelf    `json:"self"`
	PeersCount int           `json:"peersCount"`
	Adapter    HealthAdapter `json:"adapter"`
	NATS       HealthNATS    `json:"nats"`
}

type healthResponse struct {
	OK      bool       `json:"ok"`
	Code    string     `json:"code"`
	Message string     `json:"message"`
	Data    HealthData `json:"data"`
	TS      int64      `json:"ts"`
}

// --- 节点能力查询（capability.* 契约） ---

type CapabilitySkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

type CapabilityModel struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	IsDefault bool   `json:"isDefault"`
}

// ExecutionInfo 是 cron 定时任务的一次执行记录（ClawSynapse 适配器直读 gateway executions.db）。
type ExecutionInfo struct {
	ExecutionID   string `json:"executionId"`
	JobID         string `json:"jobId"`
	Status        string `json:"status"` // running | completed | failed | unknown
	StartedAtMs   int64  `json:"startedAtMs"`
	FinishedAtMs  int64  `json:"finishedAtMs"` // 运行中为 0
	DurationMs    int64  `json:"durationMs"`
	Error         string `json:"error"`
	OutputFile    string `json:"outputFile"`
	OutputPreview string `json:"outputPreview"`
}

type CapabilityJob struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Schedule   string          `json:"schedule"`
	Enabled    bool            `json:"enabled"`
	Prompt     string          `json:"prompt"`
	Skills     []string        `json:"skills,omitempty"`
	NextRun    string          `json:"nextRun,omitempty"`
	Executions []ExecutionInfo `json:"executions,omitempty"`
}

type CapabilityInfo struct {
	Product   string            `json:"product"`
	Available bool              `json:"available"`
	Skills    []CapabilitySkill `json:"skills"`
	Models    []CapabilityModel `json:"models"`
	Jobs      []CapabilityJob   `json:"jobs"`
	Reason    string            `json:"reason"`
	TS        int64             `json:"ts"`
}

type capabilityResponse struct {
	OK      bool           `json:"ok"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Data    CapabilityInfo `json:"data"`
	TS      int64          `json:"ts"`
}

// --- 节点能力写回（capability.* 契约） ---

// ProviderConfig 是 model 写回时的 provider 配置（api_key 仅在写回时携带，读响应不回显）。
// 注：hermes 适配器要求 model add 必须带 name（provider 标识）。
type ProviderConfig struct {
	Name         string `json:"name,omitempty"`
	BaseURL      string `json:"base_url,omitempty"` // OpenAI 兼容 API 地址（如 https://api.deepseek.com/v1）
	APIMode      string `json:"api_mode,omitempty"`
	Transport    string `json:"transport,omitempty"`
	Model        string `json:"model,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
}

// SetCapabilityRequest 是 capability.set 的 payload。
type SetCapabilityRequest struct {
	Target   string          `json:"target"` // skill | model | cron
	Action   string          `json:"action"` // 见契约 §2.2 动作映射
	Skill    string          `json:"skill,omitempty"`
	FileIds  []string        `json:"fileIds,omitempty"`
	Model    string          `json:"model,omitempty"`
	Provider *ProviderConfig `json:"provider,omitempty"`
	Job      map[string]any  `json:"job,omitempty"`
	JobID    string          `json:"jobId,omitempty"`
}

// SetCapabilityResult 是 capability.set_response 的映射。
type SetCapabilityResult struct {
	OK            bool   `json:"ok"`
	Target        string `json:"target"`
	Action        string `json:"action"`
	Skill         string `json:"skill,omitempty"`
	Model         string `json:"model,omitempty"`
	JobID         string `json:"jobId,omitempty"`
	RestartStatus string `json:"restartStatus"` // none | restarted | restart_failed
	Error         string `json:"error,omitempty"`
}

type setCapabilityResponse struct {
	OK      bool                `json:"ok"`
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Data    SetCapabilityResult `json:"data"`
	TS      int64               `json:"ts"`
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	writeTimeout := timeout
	if writeTimeout < 30*time.Second {
		// skill/model 写回需重启 gateway（数秒窗口），默认 3s 不够
		writeTimeout = 30 * time.Second
	}
	return &Client{
		baseURL:     baseURL,
		httpClient:  &http.Client{Timeout: timeout},
		writeClient: &http.Client{Timeout: writeTimeout},
	}
}

func (c *Client) Publish(ctx context.Context, targetNode, msgType string, payload any, sessionKey string, metadata map[string]any) (*PublishResult, error) {
	if c == nil {
		return nil, fmt.Errorf("clawsynapse client is disabled")
	}
	messageBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal publish payload: %w", err)
	}

	reqBody, err := json.Marshal(publishRequest{
		TargetNode: strings.TrimSpace(targetNode),
		Type:       strings.TrimSpace(msgType),
		Message:    string(messageBytes),
		SessionKey: strings.TrimSpace(sessionKey),
		Metadata:   metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal publish request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/publish", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("publish request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("publish request returned status %d", resp.StatusCode)
	}

	var out publishResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode publish response: %w", err)
	}
	if !out.OK {
		return nil, fmt.Errorf("publish rejected: %s", out.Code)
	}
	return &out.Data, nil
}

func (c *Client) GetPeers(ctx context.Context) ([]Peer, error) {
	if c == nil {
		return nil, fmt.Errorf("clawsynapse client is disabled")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/peers", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get peers request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get peers returned status %d", resp.StatusCode)
	}

	var out peersResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode peers response: %w", err)
	}
	if !out.OK {
		return nil, fmt.Errorf("get peers rejected: %s", out.Code)
	}
	return out.Data.Items, nil
}

func (c *Client) GetTransfer(ctx context.Context, transferID string) (map[string]any, error) {
	if c == nil {
		return nil, fmt.Errorf("clawsynapse client is disabled")
	}
	transferID = strings.TrimSpace(transferID)
	if transferID == "" {
		return nil, fmt.Errorf("transfer id is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/transfer/"+url.PathEscape(transferID), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get transfer request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("get transfer returned status 404")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get transfer returned status %d", resp.StatusCode)
	}

	var out transferResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode transfer response: %w", err)
	}
	if !out.OK {
		return nil, fmt.Errorf("get transfer rejected: %s", out.Code)
	}
	if out.Data.Transfer == nil {
		return map[string]any{}, nil
	}
	return out.Data.Transfer, nil
}

func (c *Client) GetHealth(ctx context.Context) (*HealthData, error) {
	if c == nil {
		return nil, fmt.Errorf("clawsynapse client is disabled")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/health", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get health request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get health returned status %d", resp.StatusCode)
	}

	var out healthResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode health response: %w", err)
	}
	if !out.OK {
		return nil, fmt.Errorf("get health rejected: %s", out.Code)
	}

	return &out.Data, nil
}

func (c *Client) GetSelfNodeID(ctx context.Context) (string, error) {
	health, err := c.GetHealth(ctx)
	if err != nil {
		return "", err
	}

	nodeID := strings.TrimSpace(health.Self.NodeID)
	if nodeID == "" {
		return "", fmt.Errorf("get health missing data.self.nodeId")
	}

	return nodeID, nil
}

// GetCapabilities 查询目标节点的能力（技能/模型/cron）。
// 走旁挂 daemon 的 GET /v1/peers/{nodeId}/capabilities，daemon 内部经 NATS capability.query
// 穿透到目标节点并同步等待 response。按契约约定：超时/拒绝/离线时 HTTP 仍 200，
// body 内 available=false + reason 表达真实状态，前端据此降级显示。
func (c *Client) GetCapabilities(ctx context.Context, nodeID string) (*CapabilityInfo, error) {
	if c == nil {
		return nil, fmt.Errorf("clawsynapse client is disabled")
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("nodeId is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/peers/"+url.PathEscape(nodeID)+"/capabilities", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get capabilities request failed: %w", err)
	}
	defer resp.Body.Close()

	// 契约：HTTP 恒 200，状态在 body 里表达；但 5xx 仍按错误处理
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("get capabilities returned status %d", resp.StatusCode)
	}

	var out capabilityResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode capabilities response: %w", err)
	}
	if !out.OK {
		// daemon 侧拒绝（如未实现端点），降级为不可用
		return &CapabilityInfo{Available: false, Reason: out.Code}, nil
	}
	if out.Data.Skills == nil {
		out.Data.Skills = []CapabilitySkill{}
	}
	if out.Data.Models == nil {
		out.Data.Models = []CapabilityModel{}
	}
	if out.Data.Jobs == nil {
		out.Data.Jobs = []CapabilityJob{}
	}
	return &out.Data, nil
}

// CronExecutionsResult 是 cron 执行历史查询的返回（含降级 error 字段）。
type CronExecutionsResult struct {
	Executions []ExecutionInfo `json:"executions"`
	Error      string          `json:"error,omitempty"`
}

type cronExecutionsResponse struct {
	OK      bool                 `json:"ok"`
	Code    string               `json:"code"`
	Message string               `json:"message"`
	Data    CronExecutionsResult `json:"data"`
	TS      int64                `json:"ts"`
}

// GetCronExecutions 查询目标节点定时任务的执行历史（capability.executions 契约）。
// 走旁挂 daemon 的 GET /v1/peers/{nodeId}/cron/executions，daemon 内部经 NATS capability.executions
// 穿透到目标节点并同步等待。按契约约定：超时/节点不支持时 HTTP 仍 200，
// body 内 executions 为空数组 + error 表达真实状态。
func (c *Client) GetCronExecutions(ctx context.Context, nodeID, jobID string, limit int) (*CronExecutionsResult, error) {
	if c == nil {
		return nil, fmt.Errorf("clawsynapse client is disabled")
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("nodeId is required")
	}

	u := c.baseURL + "/v1/peers/" + url.PathEscape(nodeID) + "/cron/executions"
	q := url.Values{}
	if jobID = strings.TrimSpace(jobID); jobID != "" {
		q.Set("jobId", jobID)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get cron executions request failed: %w", err)
	}
	defer resp.Body.Close()

	// 非 2xx（404/405/5xx）：daemon 未实现端点或异常，按降级处理（不 decode，避免解析纯文本报错）
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &CronExecutionsResult{Executions: []ExecutionInfo{}, Error: fmt.Sprintf("daemon returned status %d", resp.StatusCode)}, nil
	}

	var out cronExecutionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode cron executions response: %w", err)
	}
	if !out.OK {
		// daemon 侧拒绝（如未实现端点），降级为空列表 + code
		return &CronExecutionsResult{Executions: []ExecutionInfo{}, Error: out.Code}, nil
	}
	if out.Data.Executions == nil {
		out.Data.Executions = []ExecutionInfo{}
	}
	return &out.Data, nil
}

// SetCapabilities 写回目标节点的能力（技能/模型/cron）。
// 走旁挂 daemon 的 POST /v1/peers/{nodeId}/capabilities，daemon 内部经 NATS capability.set
// 穿透到目标节点并同步等待 set_response。按契约约定：超时/拒绝/离线时 HTTP 仍 200，
// body 内 ok=false + error 表达真实状态，前端据此提示。
func (c *Client) SetCapabilities(ctx context.Context, nodeID string, req *SetCapabilityRequest) (*SetCapabilityResult, error) {
	if c == nil {
		return nil, fmt.Errorf("clawsynapse client is disabled")
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("nodeId is required")
	}
	if req == nil || req.Target == "" || req.Action == "" {
		return nil, fmt.Errorf("target and action are required")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal set capabilities request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/peers/"+url.PathEscape(nodeID)+"/capabilities", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.writeClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("set capabilities request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("set capabilities returned status %d", resp.StatusCode)
	}

	var out setCapabilityResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode set capabilities response: %w", err)
	}
	if !out.OK {
		// daemon 侧拒绝（如未实现端点），降级为失败结果
		return &SetCapabilityResult{OK: false, Error: out.Code}, nil
	}
	return &out.Data, nil
}

// UploadSkillFile 上传技能文件包到目标节点，返回 fileId 供后续 capability.set 的 fileIds 引用。
// 走旁挂 daemon 的 POST /v1/peers/{nodeId}/skills（multipart/form-data, 字段 file）。
// daemon 将文件落盘到 transfer 目录并登记到 transfer store，返回 fileId；
// 随后 capability.set（skill add/update）携带该 fileId，capability service 解析为本地路径
// 交给 hermes adapter 安装到 managed skill 目录。
func (c *Client) UploadSkillFile(ctx context.Context, nodeID, filename string, file io.Reader) (string, error) {
	if c == nil {
		return "", fmt.Errorf("clawsynapse client is disabled")
	}
	nodeID = strings.TrimSpace(nodeID)
	filename = strings.TrimSpace(filename)
	if nodeID == "" {
		return "", fmt.Errorf("nodeId is required")
	}
	if filename == "" {
		return "", fmt.Errorf("filename is required")
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	fw, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("create multipart form file: %w", err)
	}
	if _, err := io.Copy(fw, file); err != nil {
		return "", fmt.Errorf("copy file into multipart: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/peers/"+url.PathEscape(nodeID)+"/skills", &buf)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.writeClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("upload skill file request failed: %w", err)
	}
	defer resp.Body.Close()

	// 非 2xx（404/405/5xx）：daemon 未实现该端点或异常，返回明确错误而非解析纯文本
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("upload skill file returned status %d (daemon may not support skill upload)", resp.StatusCode)
	}

	var out struct {
		OK      bool   `json:"ok"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    struct {
			FileID string `json:"fileId"`
		} `json:"data"`
		TS int64 `json:"ts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode upload skill file response: %w", err)
	}
	if !out.OK {
		return "", fmt.Errorf("upload skill file rejected: %s", out.Code)
	}
	if out.Data.FileID == "" {
		return "", fmt.Errorf("upload skill file missing fileId")
	}
	return out.Data.FileID, nil
}

func (c *Client) ListTransfers(ctx context.Context) ([]map[string]any, error) {
	if c == nil {
		return nil, fmt.Errorf("clawsynapse client is disabled")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/transfers", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list transfers request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list transfers returned status %d", resp.StatusCode)
	}

	var out transfersResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode transfers response: %w", err)
	}
	if !out.OK {
		return nil, fmt.Errorf("list transfers rejected: %s", out.Code)
	}
	if out.Data.Items == nil {
		return []map[string]any{}, nil
	}
	return out.Data.Items, nil
}

func (c *Client) GetPendingTrustRequests(ctx context.Context) ([]TrustPendingItem, error) {
	if c == nil {
		return nil, fmt.Errorf("clawsynapse client is disabled")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/trust/pending", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get trust pending request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get trust pending returned status %d", resp.StatusCode)
	}

	var out trustPendingResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode trust pending response: %w", err)
	}
	if !out.OK {
		return nil, fmt.Errorf("get trust pending rejected: %s", out.Code)
	}

	// Only return incoming requests
	incoming := make([]TrustPendingItem, 0, len(out.Data.Items))
	for _, item := range out.Data.Items {
		if item.Direction == "inbound" {
			incoming = append(incoming, item)
		}
	}
	return incoming, nil
}

func (c *Client) AuthChallenge(ctx context.Context, targetNode string) error {
	if c == nil {
		return fmt.Errorf("clawsynapse client is disabled")
	}
	body, err := json.Marshal(map[string]string{"targetNode": targetNode})
	if err != nil {
		return fmt.Errorf("marshal auth challenge request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/auth/challenge", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth challenge request failed: %w", err)
	}
	defer resp.Body.Close()

	var out trustActionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("decode auth challenge response: %w", err)
	}
	if !out.OK {
		return fmt.Errorf("auth challenge failed: %s — %s", out.Code, out.Message)
	}
	return nil
}

func (c *Client) ApproveTrustRequest(ctx context.Context, requestID, reason string) error {
	return c.trustAction(ctx, "/v1/trust/approve", requestID, reason)
}

func (c *Client) RejectTrustRequest(ctx context.Context, requestID, reason string) error {
	return c.trustAction(ctx, "/v1/trust/reject", requestID, reason)
}

func (c *Client) RevokeTrust(ctx context.Context, targetNode, reason string) error {
	if c == nil {
		return fmt.Errorf("clawsynapse client is disabled")
	}
	body, err := json.Marshal(map[string]string{"targetNode": targetNode, "reason": reason})
	if err != nil {
		return fmt.Errorf("marshal revoke trust request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/trust/revoke", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("revoke trust request failed: %w", err)
	}
	defer resp.Body.Close()

	var out trustActionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("decode revoke trust response: %w", err)
	}
	if !out.OK {
		return fmt.Errorf("revoke trust failed: %s — %s", out.Code, out.Message)
	}
	return nil
}

func (c *Client) trustAction(ctx context.Context, path, requestID, reason string) error {
	if c == nil {
		return fmt.Errorf("clawsynapse client is disabled")
	}
	body, err := json.Marshal(trustActionRequest{RequestID: requestID, Reason: reason})
	if err != nil {
		return fmt.Errorf("marshal trust action request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("trust action request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("trust action returned status %d", resp.StatusCode)
	}

	var out trustActionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("decode trust action response: %w", err)
	}
	if !out.OK {
		return fmt.Errorf("trust action rejected: %s", out.Code)
	}
	return nil
}
