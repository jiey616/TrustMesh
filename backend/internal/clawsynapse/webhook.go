package clawsynapse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"trustmesh/backend/internal/agentfile"
	"trustmesh/backend/internal/embedding"
	"trustmesh/backend/internal/knowledge"
	"trustmesh/backend/internal/metrics"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/project"
	"trustmesh/backend/internal/protocol"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

type WebhookHandler struct {
	store    *store.Store
	client   *Client
	log      *zap.Logger
	embedder embedding.Client
	qdrant   *knowledge.QdrantClient

	projectFileStorage project.FileStorage

	externalURL string
	jwtSecret   []byte
	downloadTTL time.Duration

	onMeetingActivity func(meetingID string)

	// unboundWarnMu guards unboundWarned, which throttles the "疑似最终交付物
	// 未绑定" system comment to one per todo — an agent that uploads several
	// intermediate drafts must not flood the task timeline.
	unboundWarnMu sync.Mutex
	unboundWarned map[string]bool

	// rejectedWarnMu guards rejectedWarned, which de-duplicates the "文件上
	// 传未入库" system comment. The platform-side forwarder fires
	// transfer.received twice for every single transfer (measured 2026-09-03:
	// 114 sends for 57 distinct transfer ids in 24h), so without this every
	// rejection wrote its warning in duplicate. Successful transfers never
	// showed the bug because artifact persistence is idempotent on transfer id.
	rejectedWarnMu sync.Mutex
	rejectedWarned map[string]time.Time
}

// transferWarnTTL bounds how long a suppressed duplicate warning is remembered.
const transferWarnTTL = 30 * time.Minute

func NewWebhookHandler(st *store.Store, client *Client, log *zap.Logger) *WebhookHandler {
	return &WebhookHandler{
		store:  st,
		client: client,
		log:    log,
	}
}

// SetKnowledgeComponents injects optional knowledge base dependencies.
func (h *WebhookHandler) SetKnowledgeComponents(embedder embedding.Client, qdrant *knowledge.QdrantClient) {
	h.embedder = embedder
	h.qdrant = qdrant
}

// SetProjectFileStorage injects the project file storage for artifact auto-indexing.
func (h *WebhookHandler) SetProjectFileStorage(storage project.FileStorage) {
	h.projectFileStorage = storage
}

// SetAgentFileConfig injects external URL and JWT secret for building download URLs.
func (h *WebhookHandler) SetAgentFileConfig(externalURL string, jwtSecret []byte, downloadTTL time.Duration) {
	h.externalURL = externalURL
	h.jwtSecret = jwtSecret
	h.downloadTTL = downloadTTL
}

// SetMeetingActivityNotifier registers a callback that is invoked whenever a
// meeting.chat message is received from any agent. MeetingHandler uses this to
// reset its inactivity timeout watchdog.
func (h *WebhookHandler) SetMeetingActivityNotifier(fn func(meetingID string)) {
	h.onMeetingActivity = fn
}

func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	var payload protocol.WebhookPayload
	// Read the raw body ourselves instead of c.ShouldBindJSON: the latter
	// hard-fails with 400 when the agent runtime omits/garbles the
	// "Content-Type: application/json" header, silently dropping otherwise
	// valid meeting messages (observed: a 400 at 07:16:05 during a live
	// meeting). Parsing the body directly tolerates a missing/invalid header.
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "failed to read webhook body"))
		return
	}
	if strings.TrimSpace(string(body)) == "" {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "empty webhook body"))
		return
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		// Diagnostic log: capture contentType + body preview so a future 400
		// can be traced to the exact offending message instead of being silent.
		if h.log != nil {
			preview := body
			if len(preview) > 240 {
				preview = preview[:240]
			}
			h.log.Warn("webhook payload JSON decode failed",
				zap.String("content_type", c.ContentType()),
				zap.String("from", payload.From),
				zap.String("body_preview", string(preview)),
				zap.Error(err))
		}
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid webhook payload"))
		return
	}

	if payload.NodeID != "" {
		localNodeID, appErr := h.resolveLocalNodeID(c.Request.Context())
		if appErr != nil {
			transport.WriteError(c, appErr)
			return
		}
		if payload.NodeID != localNodeID {
			transport.WriteError(c, transport.Validation("invalid webhook target node", map[string]any{"nodeId": "does not match local node"}))
			return
		}
	}

	// Strip known agent-runtime noise prefixes (e.g. Hermes' Tirith scanner
	// notice) up front so they never reach the conversation or break parsing.
	payload.Message = cleanAgentNoise(payload.Message)

	switch strings.TrimSpace(payload.Type) {
	case "chat.message", "chat.response":
		// DEPRECATED meeting routing via chat.message — agents should migrate
		// to meeting.chat. Keep as compatibility layer.
		if isMeetingChatContext(payload.Metadata) || h.isMeetingSessionKey(payload.SessionKey) {
			h.handleMeetingChat(c, payload)
			return
		}
		h.handleChatMessage(c, payload)
	case "task.create":
		h.handleTaskCreate(c, payload)
	case "task.reply":
		h.handleTaskReply(c, payload)
	case "task.plan_ready":
		h.handleTaskPlanReady(c, payload)
	case "task.todo_add":
		h.handleTodoAdd(c, payload)
	case "task.todo_modify":
		h.handleTodoModify(c, payload)
	case "todo.progress":
		h.handleTodoProgress(c, payload)
	case "todo.complete":
		h.handleTodoComplete(c, payload)
	case "todo.fail":
		h.handleTodoFail(c, payload)
	case "todo.ask":
		// todo.ask requests human input while the agent executes: the todo is
		// parked in waiting_user until the user answers (todo.answer resumes it).
		h.handleTodoAsk(c, payload)
	case "todo.review":
		// todo.review carries a review verdict (approve | reject) for a
		// completed todo awaiting review. Sent by the PM agent.
		h.handleTodoReview(c, payload)
	case "task.comment":
		h.handleTaskComment(c, payload)
	case "knowledge.query":
		h.handleKnowledgeQuery(c, payload)
	case "task.response", "todo.response":
		// todo.response：Hermes persona 型执行者（如编剧山雨）回复 todo 派发时
		// 会用这个类型。此前没有 handler，default 分支直接 400 拒收 ——
		// 整段工作汇报（含产出文件表）在入口就被丢弃，平台时间线永远看不到。
		// 复用 task.response 的宽松解析入库。
		h.handleTaskResponse(c, payload)
	case "todo.error":
		// 执行侧 run 失败（如 Hermes 上下文压缩失败、轮询超时）通过 todo.error
		// 回报。此前没有 handler，default 400 丢弃 → 平台对执行侧故障零感知。
		// 降级为任务评论入库，前缀标识来源，保证故障可见可追溯。
		if trimmed := strings.TrimSpace(payload.Message); trimmed != "" {
			payload.Message = "⚠️【执行侧故障上报】" + trimmed
		}
		h.handleTaskComment(c, payload)
	case "task.error":
		// Agent/pipeline runtime error (e.g. Hermes crash, upstream rejection
		// of a PM publish). Previously silently ignored — the PM's own
		// "已派发/已确认" ACK was then the only visible trace, single-sidely
		// misleading the user while the task silently stalled. Persist it as
		// a flagged task comment (same degradation as todo.error) so failures
		// are visible and auditable in the timeline.
		if trimmed := strings.TrimSpace(payload.Message); trimmed != "" {
			payload.Message = "⚠️【执行侧故障上报】" + trimmed
		}
		h.handleTaskComment(c, payload)
	case "transfer.received":
		h.handleTransferReceived(c, payload)
	case "task.context.query":
		h.handleContextQuery(c, payload)
	case "meeting.chat":
		h.handleMeetingChatUnified(c, payload)
	case "meeting.control":
		h.handleMeetingControlUnified(c, payload)
	case "meeting.message":
		h.handleMeetingMessage(c, payload)
	case "meeting.response", "meeting.ack":
		// meeting.response / meeting.ack carry a participant agent's actual
		// meeting speech (e.g. an analysis) with a trailing ACK boilerplate.
		// We must NOT drop them wholesale: strip the ACK noise, and if any
		// genuine content remains, store it in the transcript AND forward it to
		// the host so the meeting keeps advancing (fixes prep-phase deadlock and
		// invisible participant speech). Pure ACKs collapse to empty → ignored.
		h.handleMeetingResponse(c, payload)
	case "meeting.error":
		// Agent encountered a runtime error (e.g. context deadline exceeded).
		// Already logged by ClawSynapse; no platform action needed.
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
	default:
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "unsupported webhook type"))
	}
}

func (h *WebhookHandler) handleChatMessage(c *gin.Context, webhook protocol.WebhookPayload) {
	// Meeting-scoped chat: unify meeting conversation + control onto the chat
	// channel. Route to meeting transcript / control instead of 1:1 agent chat.
	if isMeetingChatContext(webhook.Metadata) || h.isMeetingSessionKey(webhook.SessionKey) {
		h.handleMeetingChat(c, webhook)
		return
	}
	content := strings.TrimSpace(webhook.Message)

	// Task-protocol envelopes smuggled over the chat channel: 山雨-style
	// orchestrators occasionally hand-write a full protocol JSON
	// ({"protocol":"clawsynapse/1.0","type":"todo.complete",...,"task_id":...})
	// and publish it as chat.message with a random session key. The chat
	// lookup then 404s ("agent chat not found") and the todo stalls forever.
	// Unwrap into the real task handler instead. Measured 2026-09-04: after a
	// 400 on todo.complete, the writer node retried exactly this way.
	if unwrapped, ok := unwrapTaskProtocolEnvelope(webhook); ok {
		if h.log != nil {
			h.log.Warn("chat.message carried a task-protocol envelope; rerouting",
				zap.String("session", webhook.SessionKey),
				zap.String("type", unwrapped.Type),
				zap.String("message", truncateStr(content, 200)))
		}
		switch strings.TrimSpace(unwrapped.Type) {
		case "task.comment":
			h.handleTaskComment(c, unwrapped)
			return
		case "todo.complete":
			h.handleTodoComplete(c, unwrapped)
			return
		case "todo.progress":
			h.handleTodoProgress(c, unwrapped)
			return
		case "todo.fail":
			h.handleTodoFail(c, unwrapped)
			return
		case "todo.ask":
			h.handleTodoAsk(c, unwrapped)
			return
		}
	}

	// Strip agent-runtime noise (ACK/WAITING/English monologue leakage) but
	// KEEP any genuine reply content. A message that is pure noise collapses to
	// empty and is silently ignored; a real reply with a trailing "ACK
	// chat.message" keeps its body.
	cleaned := cleanConversationNoise(content)
	if cleaned == "" {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}
	content = cleaned
	if content == "" {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid chat.message payload"))
		return
	}

	detail, appErr := h.store.AppendAgentChatMessageByNode(webhook.From, webhook.SessionKey, content, messageIDFromMetadata(webhook.Metadata))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, detail)
}

// unwrapTaskProtocolEnvelope detects a task-protocol envelope smuggled over
// the chat channel (content is a JSON object carrying protocol + type + task
// payload fields) and rewrites the webhook into the wrapped message type so
// the dispatcher's handler can decode it. The envelope's payload fields
// (task_id/todo_id/comment/result) live at its top level, which is exactly
// what the payload structs unmarshal — unknown envelope keys are ignored.
func unwrapTaskProtocolEnvelope(webhook protocol.WebhookPayload) (protocol.WebhookPayload, bool) {
	content := strings.TrimSpace(webhook.Message)
	if !strings.HasPrefix(content, "{") {
		return webhook, false
	}
	var probe struct {
		Protocol string          `json:"protocol"`
		Type     string          `json:"type"`
		Body     json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal([]byte(content), &probe); err != nil {
		return webhook, false
	}
	if !strings.Contains(probe.Protocol, "clawsynapse") {
		return webhook, false
	}
	switch strings.TrimSpace(probe.Type) {
	case "task.comment", "todo.complete", "todo.progress", "todo.fail", "todo.ask":
		// The 山雨 orchestrator nests the actual task payload under a "body"
		// key ({"protocol":...,"type":"todo.complete","body":{"task_id":...}}).
		// When present and an object, the body IS the payload — swap it in so
		// the handler's struct decode sees task_id/todo_id at the top level.
		if b := []byte(probe.Body); len(b) > 0 && b[0] == '{' {
			webhook.Message = string(b)
		}
		webhook.Type = probe.Type
		return webhook, true
	}
	return webhook, false
}

func (h *WebhookHandler) resolveLocalNodeID(ctx context.Context) (string, *transport.AppError) {
	if h == nil || h.client == nil {
		return "", &transport.AppError{
			Status:  http.StatusServiceUnavailable,
			Code:    "CLAWSYNAPSE_UNAVAILABLE",
			Message: "暂时无法校验本地节点身份",
			Details: map[string]any{},
		}
	}

	nodeID, err := h.client.GetSelfNodeID(ctx)
	if err != nil {
		return "", &transport.AppError{
			Status:  http.StatusServiceUnavailable,
			Code:    "CLAWSYNAPSE_UNAVAILABLE",
			Message: "暂时无法校验本地节点身份",
			Details: map[string]any{"cause": err.Error()},
		}
	}

	return nodeID, nil
}

func (h *WebhookHandler) handleTaskCreate(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TaskCreatePayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid task.create message"))
		return
	}

	in := store.TaskCreateInput{
		ProjectID:    payload.ProjectID,
		Title:        payload.Title,
		Description:  payload.Description,
		SourceTaskID: payload.SourceTaskID,
		Todos:        make([]store.TaskCreateTodoInput, 0, len(payload.Todos)),
	}
	for _, todo := range payload.Todos {
		in.Todos = append(in.Todos, store.TaskCreateTodoInput{
			ID:             todo.ID,
			Order:          todo.Order,
			Title:          todo.Title,
			Description:    todo.Description,
			AssigneeNodeID: todo.AssigneeNodeID,
		})
	}

	task, appErr := h.store.CreateTaskByPMNodeWithMessageID(webhook.From, messageIDFromMetadata(webhook.Metadata), in)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	h.publishTaskCreated(task)
	task = h.dispatchNextTodo(context.Background(), task)
	transport.WriteData(c, http.StatusOK, task)
}

func (h *WebhookHandler) handleTaskReply(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TaskReplyPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		// Tolerate malformed task.reply payloads from PM agents. Agents often
		// embed unescaped English quotes in their content (e.g. quoting a
		// field name like "need_review"), which breaks JSON parsing. Instead of
		// failing the whole planning/dialog flow, fall back to a loose decode:
		// task_id from the session key, content = the extracted "content" value
		// (or the raw message if extraction fails).
		loose := decodeTaskReplyLoose(webhook.SessionKey, webhook.Message)
		payload = loose
		if h.log != nil {
			h.log.Warn("task.reply JSON decode failed; used loose decode",
				zap.String("session", webhook.SessionKey),
				zap.String("message", truncateStr(webhook.Message, 200)),
				zap.Error(err))
		}
	}

	// Silent ACK receipts ("ACK task.reply ...", "ACK task.response ...",
	// "ACK chat.message ...") are protocol chatter, not user-facing content.
	// Drop them so they don't pollute the task/meeting transcript.
	if isSilentACK(strings.TrimSpace(payload.Content)) {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}

	// Strip a leading "[reply]"/"[response]" publish marker from the reply text
	// before it is stored or forwarded to the user.
	payload.Content = stripBracketReplyPrefix(strings.TrimSpace(payload.Content))

	// Check if this is a meeting reply (sessionKey = meeting id)
	if meeting, _ := h.store.GetMeeting(store.SystemScope(), payload.TaskID); meeting != nil {
		// Ignore messages if meeting is already completed
		if h.isMeetingCompleted(payload.TaskID) {
			transport.WriteData(c, http.StatusOK, gin.H{"status": "ignored", "reason": "meeting completed"})
			return
		}
		// Route to meeting message storage
		msgContent := payload.Content
		var uiBlocks []model.UIBlock
		// PM agent may double-wrap executor instructions in task.reply:
		// payload.Content = {"task_id":"<meeting_id>","content":"<clean_text>"}
		if extracted := extractContentFromTaskWrapper(payload.Content); extracted != "" {
			msgContent = extracted
		}
		uiBlocks = payload.UIBlocks
		msg := &model.MeetingMessage{
			MeetingID:  payload.TaskID,
			SenderType: "agent",
			SenderID:   webhook.From,
			SenderName: h.resolveMeetingSenderName(webhook.From),
			Content:    msgContent,
			UIBlocks:   uiBlocks,
		}
		if _, appErr := h.store.AddMeetingMessage(store.SystemScope(), msg); appErr != nil {
			transport.WriteError(c, appErr)
			return
		}
		// Broadcast this response to other meeting participants
		h.broadcastMeetingReply(context.Background(), meeting, webhook.From, msgContent)
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok"})
		return
	}

	// Also check sessionKey — executor task.reply messages may use the task
	// ID as TaskID but the meeting ID as SessionKey. Store a copy in the
	// meeting transcript so the meeting page shows executor responses.
	if meeting, _ := h.store.GetMeeting(store.SystemScope(), webhook.SessionKey); meeting != nil {
		if !h.isMeetingCompleted(webhook.SessionKey) {
			skContent := payload.Content
			if extracted := extractContentFromTaskWrapper(payload.Content); extracted != "" {
				skContent = extracted
			}
			msg := &model.MeetingMessage{
				MeetingID:  webhook.SessionKey,
				SenderType: "agent",
				SenderID:   webhook.From,
				SenderName: h.resolveMeetingSenderName(webhook.From),
				Content:    skContent,
				UIBlocks:   payload.UIBlocks,
			}
			if _, appErr := h.store.AddMeetingMessage(store.SystemScope(), msg); appErr == nil {
				h.broadcastMeetingReply(context.Background(), meeting, webhook.From, skContent)
			} else if h.log != nil {
				h.log.Warn("failed to store executor meeting reply", zap.Error(appErr))
			}
		}
	}

	// Normal task reply (also reached after sessionKey meeting check above)
	task, appErr := h.store.AppendPMTaskReply(webhook.From, payload.TaskID, payload.Content, payload.UIBlocks)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, task)
}

// handleTaskResponse processes task.response messages from PM agents.
// PM agents use this type for status updates, ACKs, and intermediate replies.
// It supports three message formats:
//  1. Structured: {"data":{"id":"...", ...}} — extracts task_id from data.id
//  2. JSON reply:  {"task_id":"...", "content":"..."} — treats as task.reply
//  3. Plain text:  plain status message — stored as PM reply
//
// Silent ACKs ("ACK ...\n\nWAITING", "↻ Resumed session") are ignored.
func (h *WebhookHandler) handleTaskResponse(c *gin.Context, webhook protocol.WebhookPayload) {
	msg := strings.TrimSpace(webhook.Message)

	// 「先 ACK 后正文」型回复：剥掉首行 ACK 保留正文入库（见 stripLeadingExplicitAck）。
	// 纯 ACK 剥完为空 → 返回 "" → 原样进入下面的 isSilentACK 被静默忽略。
	if rest := stripLeadingExplicitAck(msg); rest != "" {
		msg = rest
	}

	// Ignore silent ACK messages that carry no actionable content.
	if isSilentACK(msg) {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}

	// Try to extract structured content from the message.
	taskID, content := extractTaskResponseContent(msg, webhook.SessionKey)
	if taskID == "" || content == "" {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}
	// Strip a leading "[reply]"/"[response]" publish marker from the response
	// text before it is stored or forwarded to the user.
	content = stripBracketReplyPrefix(content)

	// Strip session resume header from the content before storing/forwarding
	if strings.HasPrefix(strings.TrimSpace(msg), "↻ Resumed session") {
		cleaned := stripSessionHeader(content)
		if cleaned != "" {
			content = cleaned
		}
	}

	// Check if this is a meeting response (taskID = meeting id)
	if meeting, _ := h.store.GetMeeting(store.SystemScope(), taskID); meeting != nil {
		if h.isMeetingCompleted(taskID) {
			transport.WriteData(c, http.StatusOK, gin.H{"status": "ignored", "reason": "meeting completed"})
			return
		}
		storeMsg := &model.MeetingMessage{
			MeetingID:  taskID,
			SenderType: "agent",
			SenderID:   webhook.From,
			SenderName: h.resolveMeetingSenderName(webhook.From),
			Content:    content,
		}
		if _, appErr := h.store.AddMeetingMessage(store.SystemScope(), storeMsg); appErr != nil {
			transport.WriteError(c, appErr)
			return
		}
		h.broadcastMeetingReply(context.Background(), meeting, webhook.From, content)
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok"})
		return
	}

	// Store as a PM reply so the user can see the status update.
	task, appErr := h.store.AppendPMTaskReply(webhook.From, taskID, content, nil)
	if appErr != nil {
		// AppendPMTaskReply 仅允许 PM 身份（非 PM 会得到 FORBIDDEN）。
		// todo.response/task.response 也会来自普通执行者（如编剧山雨通过
		// adapter 回报工作汇报），此时降级为任务评论入库，保证回复不丢失。
		if comment, cErr := h.store.AddTaskCommentByNode(webhook.From, store.TaskCommentInput{
			TaskID:  taskID,
			Content: content,
		}); cErr == nil {
			transport.WriteData(c, http.StatusOK, comment)
			return
		}
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, task)
}

// noiseLineRE matches a standalone Tirith security-scanner notice line
// (Hermes emits this when the tirith binary is not installed). Go regexp
// (RE2) does not support lookahead, so we use a two-phase strategy:
//  1. strip the full line if the noise occupies its own line(s);
//  2. if noise and ACK are on the same line, strip everything before ACK.
var noiseLineRE = regexp.MustCompile(`(?is)^(⚠\s*)?tirith security scanner[^\n]*\n*\s*`)

// cleanAgentNoise removes the agent-runtime noise (e.g. Hermes' Tirith scanner
// notice) from a message so it neither pollutes the conversation nor breaks
// message parsing / silent-ACK detection.
func cleanAgentNoise(msg string) string {
	s := strings.TrimSpace(msg)
	// Phase 1 — strip standalone tirith line(s)
	s = noiseLineRE.ReplaceAllString(s, "")
	// Phase 2 — same-line fallback: if "tirith" still appears before "ACK ",
	// it is residual noise from a single-line concatenation.
	if idx := strings.Index(s, "ACK "); idx > 0 && strings.Contains(strings.ToLower(s[:idx]), "tirith") {
		s = s[idx:]
	}
	return strings.TrimSpace(s)
}

// ackBoilerplate matches the fixed tokens that appear inside a PM empty ACK
// (e.g. "ACK task.reply", "ACK task.reply WAITING"). Stripping them lets us
// tell a pure acknowledgement apart from a real reply that merely starts with
// "ACK ".
var ackBoilerplate = regexp.MustCompile(`(?i)\b((?:task|todo)\.(?:reply|response|comment|message|plan_ready|assign|assigned|create|todo_add|todo_modify|progress|complete|fail|ask|review)|chat\.(?:message|response)|meeting\.(?:chat|response)|waiting)\b`)

// explicitAckRe matches a message that is an explicit protocol receipt emitted
// by the PM Agent after publishing a message of a given type, e.g.
// "ACK task.reply", "ACK task.response", "ACK task.comment",
// "ACK chat.message", "ACK chat.response", "ACK meeting.chat",
// "ACK meeting.response", and planning/dispatch receipts such as
// "ACK task.plan_ready" / "ACK todo.assigned". Any trailing text (e.g. a
// Chinese elaboration such as "等待用户回复澄清问题。") is decoration, never
// user-facing content, so the whole message is treated as a silent
// acknowledgement.
var explicitAckRe = regexp.MustCompile(`(?i)^\s*ACK\s+((?:task|chat|meeting)\.(?:reply|response|comment|message|chat|plan_ready|assign|assigned|create|todo_add|todo_modify|status_changed)|todo\.(?:assigned|progress|complete|fail|comment|ask|review|status_changed))\b`)

// stripLeadingExplicitAck handles "先 ACK 后正文" 型回复（persona 型运行时，
// 如编剧山雨："ACK task.comment\n\n已完成第5步视听蓝图…"）。
// 此前 explicitAckRe 会把整条消息判成静默回执直接丢弃，真实工作汇报随之丢失。
// 规则：首行是显式 ACK 时剥掉该行返回剩余正文；剩余部分全是协议样板
// （WAITING / 消息类型名）视为纯回执返回 ""；无 ACK 前缀也返回 ""。
// 返回 "" 时调用方继续走 isSilentACK 原有判定，语义不变。
func stripLeadingExplicitAck(msg string) string {
	loc := explicitAckRe.FindStringIndex(msg)
	if loc == nil {
		return ""
	}
	rest := strings.TrimSpace(msg[loc[1]:])
	if rest == "" {
		return ""
	}
	if ackBoilerplate.ReplaceAllString(strings.ToLower(rest), "") == "" {
		return ""
	}
	return rest
}

// bracketAckRe matches a publish receipt emitted by the ClawSynapse runtime in
// the form "[reply] ACK" or "[response] ACK" (optionally followed by a Chinese
// elaboration such as "[reply] ACK — 已记录导演数字员工就绪，剧本已加载。").
// Unlike the bare "ACK <type>" receipt matched by explicitAckRe, these are
// wrapped in square brackets and were previously leaked into the transcript as
// if they were real meeting content. They carry no user-facing value and must
// be suppressed.
var bracketAckRe = regexp.MustCompile(`(?i)^\[(?:reply|response)\]\s*ACK\b`)

// bracketReplyPrefixRe matches a leading bracketed publish marker that agents
// use to tag outbound messages, e.g. "[reply] 好的，马上处理" or
// "[response] ...". The marker itself is protocol decoration and must not leak// into the user-facing transcript — only the real content after it is shown.
var bracketReplyPrefixRe = regexp.MustCompile(`(?i)^\[(?:reply|response)\]\s*`)

// stripBracketReplyPrefix removes a leading "[reply]"/"[response]" publish
// marker from content, keeping the real content after it (or "" if none).
func stripBracketReplyPrefix(s string) string {
	if m := bracketReplyPrefixRe.FindString(s); m != "" {
		return strings.TrimSpace(strings.TrimPrefix(s, m))
	}
	return s
}

// stripLeadingAck removes a leading "ACK <message-type>" protocol receipt token
// from a single line and returns the remaining text (possibly empty). It is
// used by cleanConversationNoise so a genuine reply that happens to be prefixed
// with an ACK receipt (e.g. "ACK meeting.chat 已就绪") keeps its real content
// instead of being dropped whole. A pure receipt line collapses to "".
func stripLeadingAck(s string) string {
	if m := explicitAckRe.FindString(s); m != "" {
		return strings.TrimSpace(strings.TrimPrefix(s, m))
	}
	return s
}

// isSilentACK returns true for messages that should be silently ignored.
func isSilentACK(msg string) bool {
	msg = cleanAgentNoise(msg)
	if msg == "" {
		return true
	}
	// Explicit protocol receipt: "ACK <message-type> ...". The PM Agent emits
	// this as a one-line confirmation that it published a message of that type.
	// Always silent regardless of any trailing decoration text.
	if explicitAckRe.MatchString(msg) {
		return true
	}
	// Bracketed publish receipt ("[reply] ACK", "[response] ACK") — same class
	// of silent protocol artifact as the bare "ACK <type>" receipt above.
	if bracketAckRe.MatchString(msg) {
		return true
	}
	// Session resume notifications — may contain real content after the header
	if strings.HasPrefix(msg, "↻ Resumed session") {
		// Strip the first line (session resume header) and check if anything
		// meaningful remains. If only boilerplate (ACK/WAITING) remains, ignore it.
		rest := stripSessionHeader(msg)
		rest = ackBoilerplate.ReplaceAllString(rest, "")
		if strings.TrimSpace(rest) == "" {
			return true
		}
		// Has real content — not a silent ACK
		return false
	}
	// Bare runtime receipt ("WAITING") — the ClawSynapse response contract
	// forces agents to emit this when they have nothing to add. Pure chatter
	// that previously leaked into the comment timeline.
	if strings.EqualFold(strings.TrimSpace(msg), "waiting") {
		return true
	}
	// Empty ACK from PM, e.g. "ACK task.reply", "ACK task.reply WAITING",
	// "ACK task.response\n\nWAITING", "ACK task.comment\n\nWAITING".
	// Strip the known boilerplate tokens; if nothing meaningful remains
	// the message is a pure acknowledgement.
	if strings.HasPrefix(msg, "ACK ") {
		rest := strings.TrimSpace(strings.TrimPrefix(msg, "ACK "))
		rest = ackBoilerplate.ReplaceAllString(rest, "")
		if strings.TrimSpace(rest) == "" {
			return true
		}
	}
	return false
}

// monologueMarkers are phrases that betray an agent runtime leaking its
// internal thinking or protocol chatter (the "response contract" forced by the
// ClawSynapse runtime) into a chat/meeting message. These are never user-facing
// content and must be suppressed from any transcript.
var monologueMarkers = []string{
	"ack chat.message",
	"ack chat.response",
	"ack meeting.chat",
	"ack meeting.response",
	"according to the clawsynapse",
	"response contract",
	"system artifact",
	"message published successfully",
	"published successfully",
	"i can see the meeting",
	"my reply has already been sent",
	"my reply has already",
	"i should output exactly waiting",
	"the user is showing me",
	"let me check if",
}

// runtimeErrorMarkers are substring signatures of agent-runtime error
// artifacts (LLM context overflow, rate limiting, downstream timeouts) that
// some runtimes leak into a meeting/chat message as if it were the reply.
// Unlike monologueMarkers (which are stripped from a line, keeping any
// co-located real content), these error lines carry NO user-facing value, so
// the whole line is dropped. Observed in the wild:
//
//	"Context length exceeded: max compression attempts (3) reached."
var runtimeErrorMarkers = []string{
	"context length exceeded",
	"max compression attempts",
	"maximum context length",
	"context window is full",
	"token limit exceeded",
	"rate limit exceeded",
	"exceeded the context window",
	"exceeded the maximum context",
}

// isRuntimeErrorMessage reports whether a (lowercased) line is essentially an
// agent-runtime error artifact with no meeting value.
func isRuntimeErrorMessage(low string) bool {
	for _, m := range runtimeErrorMarkers {
		if strings.Contains(low, m) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Meeting outgoing de-duplication (backend backstop for agent over-publishing)
// ---------------------------------------------------------------------------
// meetingOutgoingDedup suppresses duplicate outgoing meeting.chat messages from
// the same sender to the same target+phase within a short window. This is a
// backend enforcement against agent runtimes that emit multiple publishes in a
// single reasoning turn — e.g. the host @-mentioning the same participant 3x
// within one second during the first speak phase. Keyed by
// meetingID|senderNode|target|phase.
var meetingOutgoingDedup = struct {
	sync.RWMutex
	m map[string]time.Time
}{m: make(map[string]time.Time)}

const meetingOutgoingDedupWindow = 30 * time.Second

func meetingOutgoingKey(meetingID, senderNode, target, phase string) string {
	return meetingID + "|" + senderNode + "|" + target + "|" + phase
}

// shouldAllowMeetingOutgoing reports whether this outgoing is NOT a recent
// duplicate. It does NOT mutate the map; callers must call markMeetingOutgoing
// only after deciding to actually store/forward the message.
func shouldAllowMeetingOutgoing(meetingID, senderNode, target, phase string) bool {
	key := meetingOutgoingKey(meetingID, senderNode, target, phase)
	meetingOutgoingDedup.RLock()
	last, ok := meetingOutgoingDedup.m[key]
	meetingOutgoingDedup.RUnlock()
	if ok && time.Since(last) < meetingOutgoingDedupWindow {
		return false
	}
	return true
}

// markMeetingOutgoing records that an outgoing was just sent. Call it after the
// dedup check passes AND the message is confirmed non-empty / will be stored.
// It also lazily expires stale entries to keep the map bounded.
func markMeetingOutgoing(meetingID, senderNode, target, phase string) {
	key := meetingOutgoingKey(meetingID, senderNode, target, phase)
	now := time.Now()
	meetingOutgoingDedup.Lock()
	for k, t := range meetingOutgoingDedup.m {
		if now.Sub(t) >= meetingOutgoingDedupWindow {
			delete(meetingOutgoingDedup.m, k)
		}
	}
	meetingOutgoingDedup.m[key] = now
	meetingOutgoingDedup.Unlock()
}

// resolveMeetingHostNodeID returns the node ID of the meeting host (PM) agent,
// or "" if unknown. Used to exempt host-originated messages from the no-phase
// filter (e.g. the host replying to a user interruption without a phase).
func (h *WebhookHandler) resolveMeetingHostNodeID(meeting *model.Meeting) string {
	if meeting == nil || meeting.HostAgentID == "" {
		return ""
	}
	if agent, ok := h.store.GetAgentByIDUnsafe(meeting.HostAgentID); ok {
		return agent.NodeID
	}
	return ""
}

// meetingMsgExemptFromNoPhase reports whether an incoming meeting message is
// allowed to omit a phase. User messages legitimately carry no phase. The host
// (PM) is also exempted so its replies to user interruptions — which may
// legitimately omit a phase — are preserved. Every other agent message without
// a phase is protocol noise (e.g. an agent "explaining" why it stays silent)
// and must be dropped.
func meetingMsgExemptFromNoPhase(role, senderNode, hostNodeID string) bool {
	if strings.TrimSpace(role) == "user" {
		return true
	}
	if senderNode != "" && senderNode == hostNodeID {
		return true
	}
	return false
}

// phaseFromRaw extracts the "phase" field from a raw JSON webhook body. The
// second return value reports whether a phase field was present at all, so
// callers can distinguish "phase present but empty" (drop) from "no phase
// field" (legacy payload — leave untouched).
func phaseFromRaw(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !json.Valid([]byte(raw)) {
		return "", false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "", false
	}
	v, ok := m["phase"]
	if !ok {
		return "", false
	}
	s, isStr := v.(string)
	if !isStr {
		// phase present but not a string (e.g. null) → treat as empty
		return "", true
	}
	return strings.TrimSpace(s), true
}

// isSilentConversationNoise reports whether an inbound chat message is merely a
// protocol receipt (ACK/WAITING) or an internal monologue leaked by the agent
// runtime, and therefore must be suppressed from the transcript. It extends
// isSilentACK with: bare WAITING receipts and English runtime-artifact leakage
// (e.g. "Message published successfully", "according to the clawsynapse skill's
// response contract") that the bare ACK-prefix check misses.
func isSilentConversationNoise(content string) bool {
	s := strings.TrimSpace(content)
	if s == "" {
		return true
	}
	lower := strings.ToLower(s)
	// Pure protocol receipts that isSilentACK does not catch on its own.
	if lower == "waiting" || lower == "ack" || lower == "err" {
		return true
	}
	// ACK-prefixed receipts (handled by isSilentACK, incl. "↻ Resumed session").
	if isSilentACK(s) {
		return true
	}
	// Runtime artifact / internal monologue leakage.
	for _, m := range monologueMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// cleanConversationNoise strips agent-runtime noise (protocol receipts like
// ACK/WAITING and English internal-monologue leakage) from an inbound chat
// message while PRESERVING any genuine user-facing content. Unlike
// isSilentConversationNoise — which drops the whole message — this keeps the
// real reply and only removes the noise lines/fragments. This prevents an
// agent that appends "ACK chat.message" (or "Message published successfully")
// after a valid Chinese reply from being silently swallowed whole. A message
// that is pure noise collapses to an empty string and is ignored by the caller.
func cleanConversationNoise(content string) string {
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue // drop blank lines
		}
		// Drop agent-runtime session-resume header lines entirely (the real
		// content follows on subsequent lines and is kept).
		if strings.HasPrefix(t, "↻ Resumed session") {
			continue
		}
		// Drop bracketed publish receipts ("[reply] ACK", "[response] ACK") and
		// any trailing decoration — these are ClawSynapse runtime artifacts, not
		// meeting content, and were previously leaked into the transcript.
		if bracketAckRe.MatchString(t) {
			continue
		}
		// Strip a leading bracketed publish marker ("[reply] ..." /
		// "[response] ..."). Unlike bracketAckRe above (pure receipts, dropped
		// whole), a "[reply] <real content>" line keeps its real content with
		// the marker removed — the marker must not leak into the UI.
		t = stripBracketReplyPrefix(t)
		// Drop agent-runtime error artifacts (e.g. "Context length exceeded:
		// max compression attempts (3) reached.") leaked as if they were the
		// reply. These are not meeting content and pollute the transcript.
		if isRuntimeErrorMessage(strings.ToLower(t)) {
			continue
		}
		// Strip a leading "ACK <message-type>" protocol receipt token from the
		// line and KEEP any genuine content that follows it. Only the receipt is
		// removed, never the agent's real reply — so a message like
		// "ACK meeting.chat 已就绪" keeps its "已就绪" content instead of being
		// dropped whole. A line that collapses to nothing (e.g. just
		// "ACK meeting.chat" or "ACK task.reply WAITING") is dropped below.
		t = stripLeadingAck(t)
		t = ackBoilerplate.ReplaceAllString(t, "")
		t = strings.TrimSpace(t)
		low := strings.ToLower(t)
		// Drop a pure protocol receipt line entirely.
		if t == "" || low == "waiting" || low == "ack" || low == "err" {
			continue
		}
		// Strip runtime-artifact leakage phrases (e.g. "ACK chat.message",
		// "Message published successfully", "I can see the meeting session")
		// from the line. If removing them leaves nothing, the whole line was
		// noise and is dropped; otherwise the cleaned text is kept.
		cleaned := t
		for _, m := range monologueMarkers {
			if strings.Contains(low, m) {
				re := regexp.MustCompile(`(?i)\s*` + regexp.QuoteMeta(m) + `\s*`)
				cleaned = re.ReplaceAllString(cleaned, " ")
			}
		}
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" {
			continue
		}
		kept = append(kept, cleaned)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// stripSessionHeader removes the first line from a session resume message,
// returning the remaining content (or empty string if there's nothing after).
func stripSessionHeader(msg string) string {
	if idx := strings.Index(msg, "\n"); idx > 0 {
		return strings.TrimSpace(msg[idx:])
	}
	return ""
}

// extractTaskResponseContent parses a task.response message and returns
// (taskID, content). It handles three formats:
//
//	{"data": {"id": "task_xxx", ...}}        → extracts data.id, content from data
//	{"task_id": "task_xxx", "content": "..."}  → direct extraction
//	plain text                                → uses sessionKey as taskID
func extractTaskResponseContent(msg, sessionKey string) (taskID, content string) {
	// Try structured JSON first.
	var raw map[string]any
	if err := json.Unmarshal([]byte(msg), &raw); err != nil {
		// Plain text fallback.
		return sessionKey, msg
	}

	// Format 2: direct task_id + content fields.
	if tid, ok := raw["task_id"].(string); ok && tid != "" {
		if c, ok := raw["content"].(string); ok {
			return tid, c
		}
		return tid, msg
	}

	// Format 1: {data: {id: "...", ...}}
	if data, ok := raw["data"].(map[string]any); ok {
		tid, _ := data["id"].(string)
		if tid == "" {
			tid = sessionKey
		}

		// Try to extract meaningful content from the data wrapper.
		// Prefer status/title fields that convey progress.
		var parts []string
		if title, ok := data["title"].(string); ok && title != "" {
			parts = append(parts, "📋 当前任务：**"+title+"**")
		}
		if status, ok := data["status"].(string); ok && status != "" {
			statusMap := map[string]string{
				"planning":    "🔍 PM 规划中",
				"in_progress": "⚙️ 执行中",
				"done":        "✅ 已完成",
				"canceled":    "❌ 已取消",
			}
			label := statusMap[status]
			if label == "" {
				label = status
			}
			parts = append(parts, "状态："+label)
		}
		if todos, ok := data["todos"].([]any); ok && len(todos) > 0 {
			parts = append(parts, fmt.Sprintf("已规划 %d 个子任务", len(todos)))
		}

		if len(parts) > 0 {
			return tid, strings.Join(parts, "  |  ")
		}
		return tid, "" // only have task ID, no meaningful content
	}

	// Unknown JSON shape — try as plain text.
	return sessionKey, msg
}

// handleTodoAdd processes task.todo_add messages from PM Agent.
// PM Agent can dynamically add a TODO to an existing task after planning.
func (h *WebhookHandler) handleTodoAdd(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TodoAddPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid task.todo_add message"))
		return
	}

	// Validate that the sender is the PM agent for this task.
	task, appErr := h.store.GetTaskByNodeID(webhook.From, payload.TaskID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if task.PMAgent.NodeID != webhook.From {
		transport.WriteError(c, transport.Forbidden("only the task's PM agent can add todos"))
		return
	}

	// Find the assignee agent by node ID.
	assigneeAgent, appErr := h.store.GetAgentByNodeID(payload.AssigneeNodeID)
	if appErr != nil {
		transport.WriteError(c, transport.Validation("invalid assignee_node_id", map[string]any{"assignee_node_id": payload.AssigneeNodeID}))
		return
	}

	// Add the todo via store.
	if payload.BeforeTodoID != "" {
		task, appErr = h.store.InsertTodo(store.SystemScope(), payload.TaskID, payload.BeforeTodoID, store.TodoModifyInput{
			Title:       payload.Title,
			Description: payload.Description,
			AssigneeID:  assigneeAgent.ID,
		})
	} else {
		task, appErr = h.store.AppendTodo(store.SystemScope(), payload.TaskID, store.TodoModifyInput{
			Title:       payload.Title,
			Description: payload.Description,
			AssigneeID:  assigneeAgent.ID,
		})
	}
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Dispatch via ClawSynapse if this todo is the next one in sequence
	task = h.dispatchNextTodo(c.Request.Context(), task)

	transport.WriteData(c, http.StatusCreated, task)
}

// handleTodoModify processes task.todo_modify messages from PM Agent.
// PM Agent can update a TODO's title, description, and/or assignee.
func (h *WebhookHandler) handleTodoModify(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TodoModifyPM
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid task.todo_modify message"))
		return
	}

	// Validate that the sender is the PM agent for this task.
	task, appErr := h.store.GetTaskByNodeID(webhook.From, payload.TaskID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if task.PMAgent.NodeID != webhook.From {
		transport.WriteError(c, transport.Forbidden("only the task's PM agent can modify todos"))
		return
	}

	// Resolve assignee if provided.
	assigneeID := payload.AssigneeNodeID
	if assigneeID != "" {
		assigneeAgent, appErr := h.store.GetAgentByNodeID(assigneeID)
		if appErr != nil {
			transport.WriteError(c, transport.Validation("invalid assignee_node_id", map[string]any{"assignee_node_id": assigneeID}))
			return
		}
		assigneeID = assigneeAgent.ID
	}

	task, appErr = h.store.UpdateTodo(store.SystemScope(), payload.TaskID, payload.TodoID, store.TodoModifyInput{
		Title:       payload.Title,
		Description: payload.Description,
		AssigneeID:  assigneeID,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, task)
}

// deliverScopeFromPayload converts the PM-declared delivery scope (wire
// format) into the stored model. Returns nil when the PM declared nothing,
// which keeps the historical "all steps required" behaviour.
func deliverScopeFromPayload(scope *protocol.PlanDeliverScope) *model.TaskDeliverScope {
	if scope == nil {
		return nil
	}
	upTo := strings.TrimSpace(scope.UpToStep)
	if upTo == "" {
		return nil
	}
	return &model.TaskDeliverScope{UpToStep: upTo}
}

func (h *WebhookHandler) handleTaskPlanReady(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TaskPlanReadyPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		// A garbage plan body (e.g. a literal "@/tmp/payload_plan.json" @file
		// reference that failed to expand) used to be rejected with a bare 400:
		// invisible in the UI while the PM still reported success. Surface the
		// rejection as a system comment so the timeline tells the real story.
		taskID, _ := webhook.Metadata["taskId"].(string)
		if taskID == "" {
			taskID = strings.TrimSpace(webhook.SessionKey)
		}
		if taskID != "" {
			preview := strings.TrimSpace(webhook.Message)
			if len(preview) > 200 {
				preview = preview[:200] + "…"
			}
			if _, cErr := h.store.AppendSystemTaskComment(taskID,
				fmt.Sprintf("⚠️ PM 规划提交被拒：task.plan_ready 消息体无法解析（BAD_PAYLOAD），内容片段：%s。请 PM 以约定的 JSON 格式重新提交 task.plan_ready。", preview)); cErr != nil && h.log != nil {
				h.log.Warn("append plan bad-payload system comment failed", zap.String("task_id", taskID), zap.Error(cErr))
			}
		}
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid task.plan_ready message"))
		return
	}

	in := store.TaskPlanReadyInput{
		TaskID:       payload.TaskID,
		Title:        payload.Title,
		Description:  payload.Description,
		Todos:        make([]store.TaskCreateTodoInput, 0, len(payload.Todos)),
		DeliverScope: deliverScopeFromPayload(payload.DeliverScope),
	}
	for _, todo := range payload.Todos {
		in.Todos = append(in.Todos, store.TaskCreateTodoInput{
			ID:             todo.ID,
			Order:          todo.Order,
			Title:          todo.Title,
			Description:    todo.Description,
			AssigneeNodeID: todo.AssigneeNodeID,
		})
	}

	// Enforce the task's workflow (if any) before accepting the plan.
	if taskWithWF := h.store.GetTaskInternal(payload.TaskID); taskWithWF != nil && taskWithWF.Workflow != nil {
		if mismatch := validatePlanAgainstWorkflowWithScope(taskWithWF.Workflow, payload.Todos, h.roleOfAgentNode, deliverScopeFromPayload(payload.DeliverScope)); mismatch != "" {
			// 收集 PM 实际提交的 todo 绑定（含原样 assignee_node_id），
			// 让「绑定了字面量/错误节点」这类问题一眼可见。
			submitted := make([]map[string]string, 0, len(payload.Todos))
			for _, t := range payload.Todos {
				submitted = append(submitted, map[string]string{
					"id":               t.ID,
					"title":            t.Title,
					"assignee_node_id": t.AssigneeNodeID,
				})
			}
			submittedJSON, _ := json.Marshal(submitted)
			// 失败透明化：同步写一条系统评论，用户在 UI 能直接看到规划被拒
			// 及原因，不再被执行侧的「已派发」类误报单边误导。
			if _, cErr := h.store.AppendSystemTaskComment(payload.TaskID,
				fmt.Sprintf("⚠️ PM 规划校验未通过：%s。提交的 todos：%s。请 PM 修正 assignee_node_id 后重新提交 task.plan_ready。", mismatch, string(submittedJSON))); cErr != nil && h.log != nil {
				h.log.Warn("append plan-reject system comment failed", zap.String("task_id", payload.TaskID), zap.Error(cErr))
			}
			// 422 回执在 ClawSynapse 侧不会触发 PM 的新一轮 LLM 推理（它只回一句 ACK 就停在
			// WAITING），任务会永久卡在 planning。这里平台主动推一条 task.message 把修正指令
			// 送进 PM 会话，让它自行重发 plan_ready（用户消息可唤醒 PM，已实证）。
			h.notifyPMPlanRejected(c, taskWithWF, mismatch, string(submittedJSON))

			transport.WriteError(c, transport.Validation("规划不符合项目工作流，请修正后重新提交 task.plan_ready", map[string]any{
				"code":            "WORKFLOW_MISMATCH",
				"details":         mismatch,
				"workflow":        taskWithWF.Workflow,
				"submitted_todos": json.RawMessage(submittedJSON),
			}))
			return
		}
	}

	task, appErr := h.store.FinalizePlanByPMNode(webhook.From, messageIDFromMetadata(webhook.Metadata), in)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, task)
}

// planRejectNotifyLimit 是同一个任务、同一个 mismatch 指纹最多自动催 PM 的次数，
// 防止 PM 反复提交同一份错误规划形成推送风暴。
const planRejectNotifyLimit = 2

// notifyPMPlanRejected 在规划校验失败后主动给 PM 推一条修正指令。
//
// 背景：task.plan_ready 的 422 在 ClawSynapse 侧以 task.error 形式回投，不会触发
// PM 的新一轮 LLM 推理 —— PM 停在 WAITING，任务永久卡在 planning 且 todos 为 0。
// 而 task.message（用户消息通道）能唤醒 PM，因此由平台代发修正指令。
// 节流：同一 (task, mismatch 指纹) 最多自动催 planRejectNotifyLimit 次。
func (h *WebhookHandler) notifyPMPlanRejected(c *gin.Context, task *model.TaskDetail, mismatch, submittedTodos string) {
	if h.client == nil || task == nil {
		return
	}
	if !h.store.ClaimPlanRejectNotify(task.ID, mismatch, planRejectNotifyLimit) {
		if h.log != nil {
			h.log.Warn("plan-reject auto-notify throttled", zap.String("task_id", task.ID))
		}
		return
	}
	pmNodeID, appErr := h.store.GetTaskPMPublishTarget(store.Scope{UserID: task.UserID}, task.ID)
	if appErr != nil {
		if h.log != nil {
			h.log.Warn("skip plan-reject notify", zap.String("task_id", task.ID), zap.String("code", appErr.Code))
		}
		return
	}
	instruction := fmt.Sprintf(
		"你刚才提交的 task.plan_ready 被平台校验拒绝，规划未生效，任务仍停留在 planning（todo 数为 0）。\n\n"+
			"拒绝原因：%s\n\n"+
			"你提交的 todos：%s\n\n"+
			"请按拒绝原因修正后，立即重新发送 task.plan_ready（不要回到澄清流程，需求已经确认过）。\n"+
			"若缺少的步骤是用户明确表示不需要的尾部步骤，请在 task.plan_ready 中声明 deliver_scope.up_to_step 为你打算止步的那一步（该步骤名必须真实存在于工作流中）。",
		mismatch, submittedTodos)
	payload := protocol.PMTaskMessage{
		SchemaVersion: "1.0",
		TaskID:        task.ID,
		ProjectID:     task.ProjectID,
		Content:       instruction,
		UserContent:   instruction,
		IsInitial:     false,
		Workflow:      task.Workflow,
	}
	if _, err := h.client.Publish(c.Request.Context(), pmNodeID, "task.message", payload, task.ID, nil); err != nil {
		if h.log != nil {
			h.log.Warn("plan-reject notify publish failed", zap.String("task_id", task.ID), zap.Error(err))
		}
		return
	}
	if h.log != nil {
		h.log.Info("plan-reject auto-notify sent", zap.String("task_id", task.ID), zap.String("pm_node", pmNodeID))
	}
}

func (h *WebhookHandler) handleTodoProgress(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TodoProgressPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid todo.progress message"))
		return
	}

	if payload.TaskID == "" {
		payload.TaskID = webhook.SessionKey
	}
	if payload.TodoID == "" {
		if id, rerr := h.store.ResolveActiveTodoForNode(payload.TaskID, webhook.From); rerr == nil && id != "" {
			payload.TodoID = id
		}
	}

	task, appErr := h.store.UpdateTodoProgressByNode(webhook.From, store.TodoProgressInput{
		TaskID:  payload.TaskID,
		TodoID:  payload.TodoID,
		Message: payload.Message,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	h.publishTaskAndTodoStatusChanges(task, payload.TodoID, payload.Message, webhook.From, "todo.progress")
	transport.WriteData(c, http.StatusOK, task)
}

func (h *WebhookHandler) handleTodoComplete(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TodoCompletePayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		// Include the decode error detail: LLM executors only see this message
		// and must be able to self-correct. A bare "invalid ... message" gave
		// the agent nothing to fix, so it retried blind over the chat channel.
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid todo.complete message: "+err.Error()))
		return
	}

	if payload.TaskID == "" {
		payload.TaskID = webhook.SessionKey
	}
	if payload.TodoID == "" {
		if id, rerr := h.store.ResolveActiveTodoForNode(payload.TaskID, webhook.From); rerr == nil && id != "" {
			payload.TodoID = id
		}
	}
	if payload.Result.Summary == "" && payload.Content != "" {
		payload.Result.Summary = payload.Content
	}

	needReview := payload.NeedReview != nil && *payload.NeedReview
	returnPrevious := payload.ReturnPrevious != nil && *payload.ReturnPrevious

	task, reworked, appErr := h.store.CompleteTodoByNodeWithMessageID(webhook.From, messageIDFromMetadata(webhook.Metadata), store.TodoCompleteInput{
		TaskID:         payload.TaskID,
		TodoID:         payload.TodoID,
		Result:         model.TodoResult(payload.Result),
		NeedReview:     needReview,
		ReturnPrevious: returnPrevious,
		ReworkReason:   strings.TrimSpace(payload.ReworkReason),
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// ReturnPrevious already cascade-reset inside the store; publish the
	// reworked predecessor so its assignee re-executes it.
	// For NeedReview the sequential pipeline is blocked (NextDispatchableTodo
	// returns nil while pending_approval), so dispatchNextTodo is a safe no-op.
	h.publishTaskAndTodoStatusChanges(task, payload.TodoID, "completed", webhook.From, "todo.complete")
	if reworked != nil {
		reason := strings.TrimSpace(payload.ReworkReason)
		if reason == "" {
			reason = "数字员工判定前序产出不合格，退回重做"
		}
		h.publishReworkDispatch(context.Background(), task, reworked, reason)
	} else {
		task = h.dispatchNextTodo(context.Background(), task)
	}

	// Action items → notify the PM so it can convert them into new tasks.
	h.notifyPMActionItems(task, payload.TodoID)

	transport.WriteData(c, http.StatusOK, task)
}

// notifyPMActionItems pushes a task.result message to the task's PM agent when
// the completed todo carries unconverted action items. Falls back to the
// project's PM agent when the task has none bound.
func (h *WebhookHandler) notifyPMActionItems(task *model.TaskDetail, todoID string) {
	if task == nil {
		return
	}
	todo := findTodo(task, todoID)
	if todo == nil || len(todo.Result.ActionItems) == 0 {
		return
	}
	unconverted := make([]model.ActionItem, 0, len(todo.Result.ActionItems))
	for _, it := range todo.Result.ActionItems {
		if it.Status != model.ActionItemConverted {
			unconverted = append(unconverted, it)
		}
	}
	if len(unconverted) == 0 {
		return
	}

	pmNode := task.PMAgent.NodeID
	if pmNode == "" {
		// Fallback to the project's PM agent.
		if project, pErr := h.store.GetProject(store.Scope{UserID: task.UserID}, task.ProjectID); pErr == nil && project != nil && project.PMAgent.NodeID != "" {
			pmNode = project.PMAgent.NodeID
		}
	}
	if pmNode == "" {
		return
	}

	payload := protocol.TaskResultPayload{
		TaskID:      task.ID,
		TodoID:      todo.ID,
		TodoTitle:   todo.Title,
		ProjectID:   task.ProjectID,
		ActionItems: unconverted,
	}
	// h.publish logs failures internally and never returns an error.
	h.publish(context.Background(), pmNode, "task.result", payload, task.ID)
}

// handleTodoReview processes a todo.review message from the PM agent:
// approve unblocks the pipeline; reject triggers the rework cascade.
func (h *WebhookHandler) handleTodoReview(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TodoReviewPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid todo.review message"))
		return
	}
	if payload.TaskID == "" {
		payload.TaskID = webhook.SessionKey
	}

	task, reworked, appErr := h.store.ReviewTodo(store.SystemScope(), webhook.From, payload.TaskID, payload.TodoID, payload.Action, payload.Reason)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Approve unblocks the pipeline: dispatch the next ready todo.
	if payload.Action == "approve" {
		task = h.dispatchNextTodo(context.Background(), task)
	} else if reworked != nil {
		// Reject triggered a rework: publish todo.assigned for the audited
		// predecessor so its assignee re-executes it.
		h.publishReworkDispatch(context.Background(), task, reworked, payload.Reason)
	}
	transport.WriteData(c, http.StatusOK, task)
}

// publishReworkDispatch publishes a todo.assigned message for a todo that was
// sent back for rework (review rejection or return_previous), telling the
// assignee to redo it.
func (h *WebhookHandler) publishReworkDispatch(ctx context.Context, task *model.TaskDetail, todo *model.Todo, reason string) {
	if h == nil || h.client == nil || task == nil || todo == nil {
		return
	}
	payload := h.buildTodoAssignedPayload(task, todo)
	count := todo.ReworkCount
	if count < 1 {
		count = 1
	}
	payload.Content = fmt.Sprintf(
		"【重做 · 第 %d 次】你负责的 Todo 上一轮产出未通过审核，已被退回重做。\n退回原因：%s\n请先通过 prior_results / task.context 读取前序结果，特别是审核者的退回意见（summary 及审核意见书等 artifacts 文件，可下载），逐条修复后重新交付；交付说明中请逐条回应（如「M-1 已改：L263 广寒宫→广寒弓」）。",
		count, reason)
	if _, err := h.client.Publish(ctx, todo.Assignee.NodeID, "todo.assigned", payload, task.ID, map[string]any{"source": "rework", "reason": reason, "rework_count": count}); err != nil {
		if h.log != nil {
			h.log.Warn("rework todo dispatch failed", zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.String("target_node", todo.Assignee.NodeID), zap.Error(err))
		}
	}
}

// RedispatchTodo publishes a todo.assigned for a specific todo and records the
// dispatch. Used as the store's dispatch hook so the timeout monitor can
// actually re-dispatch retried todos (Bug: retry previously reset to pending
// without ever notifying the assignee).
func (h *WebhookHandler) RedispatchTodo(ctx context.Context, taskID, todoID string) {
	if h == nil || h.client == nil {
		return
	}
	task := h.store.GetTaskInternal(taskID)
	if task == nil {
		return
	}
	todo := findTodo(task, todoID)
	if todo == nil {
		return
	}
	payload := h.buildTodoAssignedPayload(task, todo)
	payload.Content = "该 Todo 执行超时，系统已自动重试。请重新执行并按要求回报进度和结果。"
	if _, err := h.client.PublishWithRetry(ctx, todo.Assignee.NodeID, "todo.assigned", payload, task.ID, map[string]any{"source": "timeout_retry"}); err != nil {
		if h.log != nil {
			h.log.Warn("timeout retry dispatch failed", zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.String("target_node", todo.Assignee.NodeID), zap.Error(err))
		}
		h.recordDispatchFailureFor(todo, task, "超时重试派发失败："+err.Error())
		return
	}
	if _, appErr := h.store.RecordSequentialTodoDispatch(task.ID, todo.ID); appErr != nil {
		if h.log != nil {
			h.log.Warn("failed to persist timeout retry dispatch", zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.Error(appErr))
		}
	}
}

// RemindTodo sends a todo.remind reminder to a timed-out todo's assignee.
// Unlike RedispatchTodo it does NOT re-dispatch the todo (which caused
// duplicate executions for long-running tasks); it only nudges the agent to
// report progress. If the agent never responds after defaultMaxReminders
// reminders, the timeout monitor fails the todo.
//
// NOTE: it uses "todo.remind" (not "task.mention") so the ClawSynapse node
// routes it through the runs API — which continues the SAME todo session the
// assignee is already working in. task.mention would go through the chat
// session instead, leaving the agent with no todo context and causing it to
// restart the workflow from step 1 after every reminder.
func (h *WebhookHandler) RemindTodo(ctx context.Context, taskID, todoID string) {
	if h == nil || h.client == nil {
		return
	}
	task := h.store.GetTaskInternal(taskID)
	if task == nil {
		return
	}
	todo := findTodo(task, todoID)
	if todo == nil {
		return
	}
	payload := map[string]any{
		"task_id":    task.ID,
		"project_id": task.ProjectID,
		"todo_id":    todo.ID,
		"todo_title": todo.Title,
		// 催办文案必须显式声明「不是新任务」：上下文压缩失败/失忆的执行者会把
		// 提醒当成重新指派，从第 1 步重跑整个流程（山雨编剧实测案例）。
		"content": fmt.Sprintf(
			"【执行超时催办】这是对既有任务《%s》的进度催办，不是新任务指派，请勿重新开始执行流程，应基于已完成的工作继续推进。请立即用 todo.progress 回报当前进度与剩余工作；若实际工作已完成，请直接用 todo.complete 交付；确实无法继续才用 todo.fail 说明原因。多次提醒无响应平台将判定任务失败。",
			todo.Title,
		),
	}
	if _, err := h.client.Publish(ctx, todo.Assignee.NodeID, "todo.remind", payload, task.ID, map[string]any{"source": "timeout_remind"}); err != nil {
		if h.log != nil {
			h.log.Warn("timeout reminder failed", zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.String("target_node", todo.Assignee.NodeID), zap.Error(err))
		}
	}
}

// NudgePlanningPM is the planning-stall hook: the store timeout monitor calls
// it when a task has been stuck in planning (no finalized plan, PM owes a
// response) for planningStallTimeout. It publishes a task.message to the PM
// (the only channel proven to wake the PM into a new LLM turn) so it resumes
// planning.
func (h *WebhookHandler) NudgePlanningPM(ctx context.Context, taskID string) {
	if h == nil || h.client == nil {
		return
	}
	task := h.store.GetTaskInternal(taskID)
	if task == nil {
		return
	}
	pmNodeID, appErr := h.store.GetTaskPMPublishTarget(store.Scope{UserID: task.UserID}, task.ID)
	if appErr != nil {
		if h.log != nil {
			h.log.Warn("skip planning-stall nudge", zap.String("task_id", taskID), zap.String("code", appErr.Code))
		}
		return
	}
	instruction := "【规划停滞提醒】这是对既有任务《" + task.Title + "》的规划催办，不是新任务指派。任务已较长时间停留在规划阶段且尚未产生任何 Todo。" +
		"请检查你上一轮的 task.plan_ready 是否被拒绝或尚未提交：若被拒绝，请按拒绝原因修正后重新发送 task.plan_ready；若尚未提交，请立即提交（需求已经确认过，不要回到澄清流程）。" +
		"若用户明确只要交付到某个步骤为止，请在 task.plan_ready 中声明 deliver_scope.up_to_step。多次提醒无响应平台将标记该任务待人工介入。"
	payload := protocol.PMTaskMessage{
		SchemaVersion: "1.0",
		TaskID:        task.ID,
		ProjectID:     task.ProjectID,
		Content:       instruction,
		UserContent:   instruction,
		IsInitial:     false,
	}
	if _, err := h.client.Publish(ctx, pmNodeID, "task.message", payload, task.ID, map[string]any{"source": "planning_stall"}); err != nil {
		if h.log != nil {
			h.log.Warn("planning-stall nudge publish failed", zap.String("task_id", taskID), zap.String("target_node", pmNodeID), zap.Error(err))
		}
	}
}

func (h *WebhookHandler) handleTodoFail(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TodoFailPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid todo.fail message: "+err.Error()))
		return
	}

	if payload.TaskID == "" {
		payload.TaskID = webhook.SessionKey
	}
	if payload.TodoID == "" {
		if id, rerr := h.store.ResolveActiveTodoForNode(payload.TaskID, webhook.From); rerr == nil && id != "" {
			payload.TodoID = id
		}
	}

	task, appErr := h.store.FailTodoByNodeWithMessageID(webhook.From, messageIDFromMetadata(webhook.Metadata), store.TodoFailInput{
		TaskID: payload.TaskID,
		TodoID: payload.TodoID,
		Error:  payload.Error,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	h.publishTaskAndTodoStatusChanges(task, payload.TodoID, payload.Error, webhook.From, "todo.fail")
	transport.WriteData(c, http.StatusOK, task)
}

// handleTodoAsk processes a todo.ask message from the assignee agent: records
// the question on the todo and parks it in waiting_user until the user answers.
func (h *WebhookHandler) handleTodoAsk(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TodoAskPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid todo.ask message"))
		return
	}

	if payload.TaskID == "" {
		payload.TaskID = webhook.SessionKey
	}
	if payload.TodoID == "" {
		if id, rerr := h.store.ResolveActiveTodoForNode(payload.TaskID, webhook.From); rerr == nil && id != "" {
			payload.TodoID = id
		}
	}

	required := true
	if payload.Required != nil {
		required = *payload.Required
	}

	task, question, appErr := h.store.AskTodoByNode(webhook.From, store.TodoAskInput{
		TaskID:     payload.TaskID,
		TodoID:     payload.TodoID,
		QuestionID: payload.QuestionID,
		Question:   payload.Question,
		Options:    payload.Options,
		Required:   required,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	h.publishTaskAndTodoStatusChanges(task, payload.TodoID, "agent asked user: "+payload.Question, webhook.From, "todo.ask")
	transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "question_id": question.ID, "required": required})
}

func (h *WebhookHandler) handleTaskComment(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TaskCommentPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		// 宽松降级：persona 型执行者（如编剧山雨）会把回复原文直接塞进 message
		// （纯中文文本，不是约定的 {"task_id":..,"content":..} JSON）。
		// 此前直接 400 拒收，整段汇报凭空消失。这里把非 JSON 的 message
		// 当作纯文本评论正文，task_id 回退 SessionKey —— 与 task.response
		// 的 plain-text 回退语义保持一致。空 message 才真正拒收。
		if text := strings.TrimSpace(webhook.Message); text != "" {
			payload = protocol.TaskCommentPayload{
				TaskID:  webhook.SessionKey,
				Content: text,
			}
		} else {
			transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid task.comment message"))
			return
		}
	}
	if payload.TaskID == "" {
		payload.TaskID = webhook.SessionKey
	}
	if payload.Content == "" {
		payload.Content = webhook.Message
	}
	// Strip a leading "[reply]"/"[response]" publish marker from comment text
	// so the marker never shows up in the task/meeting transcript.
	payload.Content = stripBracketReplyPrefix(strings.TrimSpace(payload.Content))

	// Check if this is a meeting comment (task_id = meeting id)
	if meeting, _ := h.store.GetMeeting(store.SystemScope(), payload.TaskID); meeting != nil {
		if h.isMeetingCompleted(payload.TaskID) {
			transport.WriteData(c, http.StatusOK, gin.H{"status": "ignored", "reason": "meeting completed"})
			return
		}
		msg := &model.MeetingMessage{
			MeetingID:  payload.TaskID,
			SenderType: "agent",
			SenderID:   webhook.From,
			SenderName: h.resolveMeetingSenderName(webhook.From),
			Content:    payload.Content,
		}
		if _, appErr := h.store.AddMeetingMessage(store.SystemScope(), msg); appErr != nil {
			transport.WriteError(c, appErr)
			return
		}
		// Broadcast this response to other meeting participants
		h.broadcastMeetingReply(context.Background(), meeting, webhook.From, payload.Content)
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok"})
		return
	}

	event, appErr := func() (*model.Comment, *transport.AppError) {
		// Pure protocol receipts ("ACK todo.status_changed - TD_01 -> ...",
		// bare "WAITING") used to pollute the comment timeline. Strip a
		// leading ACK receipt: real content after it is kept, a pure receipt
		// (or bare WAITING) is ignored entirely — same silent-ACK contract
		// as task.reply, minus the content loss.
		if rest := stripLeadingExplicitAck(payload.Content); rest != "" {
			payload.Content = rest
		} else if isSilentACK(payload.Content) {
			return nil, nil
		}
		return h.store.AddTaskCommentByNode(webhook.From, store.TaskCommentInput{
			TaskID:  payload.TaskID,
			TodoID:  payload.TodoID,
			Content: payload.Content,
		})
	}()
	if event == nil && appErr == nil {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, event)
}

func (h *WebhookHandler) handleContextQuery(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.ContextQueryPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid task.context.query message"))
		return
	}

	task, appErr := h.store.GetTaskByNodeID(webhook.From, payload.TaskID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	resultPayload := protocol.ContextResultPayload{
		TaskID:      task.ID,
		TaskContext: buildTaskContext(task, ""),
	}
	for i := range task.Todos {
		t := &task.Todos[i]
		if t.Status == "done" {
			resultPayload.AllResults = append(resultPayload.AllResults, h.buildPriorResult(task, t))
		}
	}

	h.publish(context.Background(), webhook.From, "task.context.result", resultPayload, task.ID)
	transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "task_id": task.ID})
}

func (h *WebhookHandler) publishTaskCreated(task *model.TaskDetail) {
	h.publish(context.Background(), task.PMAgent.NodeID, "task.created", protocol.TaskCreatedPayload{
		TaskID:    task.ID,
		ProjectID: task.ProjectID,
		Title:     task.Title,
	}, task.ID)
}

// PublishTaskCreated notifies the PM agent that a task has been created/approved.
func (h *WebhookHandler) PublishTaskCreated(task *model.TaskDetail) {
	h.publishTaskCreated(task)
}

// DispatchNextTodo dispatches the next pending todo in a task to its assignee agent.
func (h *WebhookHandler) DispatchNextTodo(ctx context.Context, task *model.TaskDetail) *model.TaskDetail {
	return h.dispatchNextTodo(ctx, task)
}

func (h *WebhookHandler) dispatchNextTodo(ctx context.Context, task *model.TaskDetail) *model.TaskDetail {
	if h == nil || task == nil || h.client == nil {
		h.recordDispatchFailure(task, "clawsynapse client 未就绪（NATS/转发节点未连接）")
		return task
	}
	if task.Status == "canceled" {
		return task
	}

	todo := task.NextDispatchableTodo()
	if todo == nil {
		return task
	}
	if appErr := h.store.CheckTaskProjectActive(task.ID); appErr != nil {
		// 项目归档时跳过派发是预期行为，不是故障：只记日志，不落失败事件
		// （否则每个归档项目的 pending todo 都会给用户推一条「派发失败」）。
		if h.log != nil {
			h.log.Warn("skip sequential todo dispatch for archived task project", zap.String("task_id", task.ID), zap.Error(appErr))
		}
		return task
	}

	payload := h.buildTodoAssignedPayload(task, todo)
	// T0.3: make the tenant trust assumption explicit and observable before we
	// hand work to a node. Observe-only in Phase 0 — blocking here would break
	// every existing shared-agent dispatch, so enforcement waits for T1.1.
	h.observeDispatchOrgMismatch(task, todo)
	if _, err := h.client.PublishWithRetry(ctx, todo.Assignee.NodeID, "todo.assigned", payload, task.ID, nil); err != nil {
		if h.log != nil {
			h.log.Error("sequential todo dispatch failed after retries",
				zap.String("task_id", task.ID), zap.String("todo_id", todo.ID),
				zap.String("target_node", todo.Assignee.NodeID), zap.Error(err))
		}
		h.recordDispatchFailureFor(todo, task, err.Error())
		return task
	}

	updatedTask, appErr := h.store.RecordSequentialTodoDispatch(task.ID, todo.ID)
	if appErr != nil {
		if h.log != nil {
			h.log.Error("failed to persist sequential todo dispatch",
				zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.Error(appErr))
		}
		h.recordDispatchFailureFor(todo, task, "派发已送达但落库失败："+appErr.Message)
		return task
	}
	return updatedTask
}

// dispatchOrgMismatch reports the assignee agent's tenant and whether it
// disagrees with the task's tenant.
//
// This is deliberately the raw comparison the plan calls for
// (`agent.OrgID == task.OrgID || task.OrgID == ""`), NOT the richer
// authorization decision from store.agentCanWriteTaskUnsafe. The difference
// matters: an agent whose owner is a member of the task's org is authorized
// even when its own org_id is empty (store/scope.go condition 3). So a
// mismatch flag here means "the agent carries no explicit org_id matching this
// tenant" — i.e. a backfill gap — rather than "this dispatch is unauthorized".
// That gap count is exactly the input T1.1 needs before it can enforce.
//
// Two cases are never a mismatch: a legacy task with no org_id (dispatch is
// governed by the author's user identity) and an unknown/absent agent (a
// different failure, surfaced by the dispatch itself).
func (h *WebhookHandler) dispatchOrgMismatch(task *model.TaskDetail, todo *model.Todo) (agentOrg string, mismatched bool) {
	if h == nil || h.store == nil || task == nil || todo == nil {
		return "", false
	}
	if task.OrgID == "" {
		return "", false
	}
	agentID := todo.Assignee.AgentID
	if agentID == "" {
		return "", false
	}
	agent, ok := h.store.GetAgentByIDUnsafe(agentID)
	if !ok || agent == nil {
		return "", false
	}
	return agent.OrgID, agent.OrgID != task.OrgID
}

// observeDispatchOrgMismatch records a tenant mismatch as a metric and a warn
// log. It returns nothing, and its caller does not branch on it — Phase 0
// observes; enforcement is T1.1 (T0.3 acceptance: "warns, does not block").
func (h *WebhookHandler) observeDispatchOrgMismatch(task *model.TaskDetail, todo *model.Todo) {
	agentOrg, mismatched := h.dispatchOrgMismatch(task, todo)
	if !mismatched {
		return
	}
	metrics.Inc(metrics.DispatchOrgMismatchTotal)
	if h.log != nil {
		h.log.Warn("dispatch tenant mismatch: assignee agent carries no matching org_id",
			zap.String("task_id", task.ID),
			zap.String("todo_id", todo.ID),
			zap.String("task_org_id", task.OrgID),
			zap.String("agent_org_id", agentOrg),
			zap.String("agent_id", todo.Assignee.AgentID),
			zap.String("agent_node_id", todo.Assignee.NodeID),
		)
	}
}

// recordDispatchFailureFor records a dispatch failure for a specific todo so the
// stall becomes visible (event + notification) and the reconciler can retry it.
// Nil-safe so the handler stays usable in tests without a store.
func (h *WebhookHandler) recordDispatchFailureFor(todo *model.Todo, task *model.TaskDetail, reason string) {
	if h == nil || h.store == nil || todo == nil || task == nil {
		return
	}
	if appErr := h.store.RecordSequentialDispatchFailure(task.ID, todo.ID, reason); appErr != nil && h.log != nil {
		h.log.Warn("record dispatch failure failed",
			zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.Error(appErr))
	}
}

// recordDispatchFailure is the fallback for paths that cannot resolve the todo.
func (h *WebhookHandler) recordDispatchFailure(task *model.TaskDetail, reason string) {
	if task == nil {
		return
	}
	if todo := task.NextDispatchableTodo(); todo != nil {
		h.recordDispatchFailureFor(todo, task, reason)
	}
}

// RetryDispatch re-publishes a specific todo to its assignee. Used by the
// background dispatch reconciler (store.StartDispatchReconciler) to heal
// pipelines broken by a silently failed dispatch. It deliberately does NOT go
// through dispatchNextTodo (no recursion), and relies on the store for
// idempotency: RecordSequentialTodoDispatch rejects non-pending todos.
func (h *WebhookHandler) RetryDispatch(ctx context.Context, taskID, todoID string) error {
	if h == nil || h.client == nil || h.store == nil {
		return fmt.Errorf("dispatch dependencies not ready")
	}
	task := h.store.GetTaskInternal(taskID)
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}
	// 归档项目的 pending todo 不应被补派（与 dispatchNextTodo 语义一致）。
	if appErr := h.store.CheckTaskProjectActive(taskID); appErr != nil {
		return nil
	}
	todo := findTodo(task, todoID)
	if todo == nil {
		return fmt.Errorf("todo not found: %s", todoID)
	}
	if todo.Status != "pending" {
		return nil // already dispatched/started elsewhere: nothing to heal
	}
	payload := h.buildTodoAssignedPayload(task, todo)
	payload.Content = "系统检测到该 Todo 未被派发或派发中断，现已自动补派。请执行并按要求回报进度和结果。"
	if _, err := h.client.PublishWithRetry(ctx, todo.Assignee.NodeID, "todo.assigned", payload, task.ID, map[string]any{"source": "dispatch_reconcile"}); err != nil {
		h.recordDispatchFailureFor(todo, task, "对账补派失败："+err.Error())
		return err
	}
	if _, appErr := h.store.RecordSequentialTodoDispatch(task.ID, todo.ID); appErr != nil {
		// TODO_NOT_PENDING is benign: another path got there first.
		if appErr.Code != "TODO_NOT_PENDING" {
			h.recordDispatchFailureFor(todo, task, "对账补派落库失败："+appErr.Message)
			return fmt.Errorf("%s", appErr.Message)
		}
	}
	return nil
}

func (h *WebhookHandler) buildTodoAssignedPayload(task *model.TaskDetail, todo *model.Todo) protocol.TodoAssignedPayload {
	payload := protocol.TodoAssignedPayload{
		TaskID:      task.ID,
		TodoID:      todo.ID,
		Title:       todo.Title,
		Description: todo.Description,
		Content:     "你收到了一个新的 Todo 任务。请使用 /tm-task-exec skill 执行此任务，按要求回报进度和结果。",
		ExecBrief:   defaultExecBrief(),
	}

	firstTime := !agentHasPriorTodoInTask(task, todo)
	if firstTime {
		payload.TaskContext = buildTaskContext(task, todo.ID)
		payload.PriorResults = buildAllPriorResults(h, task, todo)
	} else {
		payload.PriorResults = buildCrossAgentPriorResults(h, task, todo)
	}

	// attached_files carries ONLY user-uploaded files (source: "user_upload").
	// Predecessor todo products are delivered separately through
	// prior_results[].artifacts (and on demand via task.context.query →
	// all_results[].artifacts), each entry carrying a download_url. Keeping the
	// two kinds of files in distinct fields lets the executing agent clearly
	// tell user-provided inputs apart from predecessor outputs.
	payload.AttachedFiles = h.enrichAttachedFiles(dedupeAttachedFiles(task.AttachedFiles))

	// Workflow-declared inputs: resolve each StepInput's StepIOLink to the
	// upstream todo's produced output file so the agent can fetch "上一个流程的
	// 输出文件" directly via download_url. Non-blocking: unresolved links are
	// still surfaced (Resolved=false) as a soft hint.
	payload.Inputs = h.BuildTodoInputs(task, todo)
	// Workflow-declared outputs: tell the agent which slot name(s) its
	// deliverable must claim via `--metadata outputName=`. Without this the
	// agent has no way to learn the slot name and silently files everything as
	// a process artifact (2026-09-02: 军旅任务终稿 DOCX 因此未上工作流图).
	payload.Outputs = h.BuildTodoOutputs(task, todo)

	return payload
}

// artifactDownloadURL builds a time-limited download URL for an artifact's
// backing ProjectFile, or "" if the project file / download machinery is
// unavailable. Shared by prior-result and workflow-input resolution.
func (h *WebhookHandler) artifactDownloadURL(transferID string) string {
	if h == nil || h.store == nil || transferID == "" || len(h.jwtSecret) == 0 || h.externalURL == "" {
		return ""
	}
	pf, _ := h.store.GetProjectFileByTransferID(transferID)
	if pf == nil {
		return ""
	}
	tok, err := agentfile.GenerateDownloadToken(h.jwtSecret, pf.ID, h.downloadTTL)
	if err != nil {
		return ""
	}
	return agentfile.BuildDownloadURL(h.externalURL, pf.ID, tok)
}

// buildTodoInputs resolves the current todo's workflow step inputs against the
// upstream todos' produced outputs (Todo.Outputs, populated by P1 when an
// agent uploads with metadata outputName). Returns nil when the task has no
// workflow or the step declares no inputs.
func (h *WebhookHandler) BuildTodoInputs(task *model.TaskDetail, todo *model.Todo) []protocol.TodoInputRef {
	if task.Workflow == nil || len(task.Workflow.Steps) == 0 {
		return nil
	}
	stepIdx := h.stepIndexForTodo(task, todo)
	if stepIdx < 0 || stepIdx >= len(task.Workflow.Steps) {
		return nil
	}
	step := task.Workflow.Steps[stepIdx]
	if len(step.Inputs) == 0 {
		return nil
	}
	var inputs []protocol.TodoInputRef
	for _, in := range step.Inputs {
		ref := h.resolveStepInput(task, todo, in)
		if ref == nil {
			// Declared but not yet resolvable: surface the expectation.
			inputs = append(inputs, protocol.TodoInputRef{
				Name:        in.Name,
				Description: in.Description,
				MimeType:    in.MimeType,
				SourceStep:  in.Source.Step,
				OutputName:  in.Source.Output,
				Resolved:    false,
			})
			continue
		}
		inputs = append(inputs, *ref)
	}
	if len(inputs) == 0 {
		return nil
	}
	return inputs
}

// BuildTodoOutputs returns the workflow output slots declared by the step this
// todo belongs to, so the executing agent can name its deliverable correctly.
// Returns nil when the task has no workflow, the todo cannot be aligned to a
// step, or that step declares no outputs.
//
// The slot Name is an identifier, not a file name: templates such as
// "剧名_剧本类型_版本_时间" must be copied verbatim into
// `--metadata outputName=...`. Substituting real values into the placeholders
// breaks the strict name matching used to resolve downstream inputs.
//
// Step resolution deliberately reuses stepIndexForTodo — the same alignment
// BuildTodoInputs uses — so inputs and outputs always describe the same step.
func (h *WebhookHandler) BuildTodoOutputs(task *model.TaskDetail, todo *model.Todo) []protocol.TodoOutputRef {
	outs := h.stepOutputsForTodo(task, todo)
	if len(outs) == 0 {
		return nil
	}
	out := make([]protocol.TodoOutputRef, 0, len(outs))
	for _, o := range outs {
		out = append(out, protocol.TodoOutputRef{
			Name:        o.Name,
			Description: o.Description,
			MimeType:    o.MimeType,
		})
	}
	return out
}

// stepOutputsForTodo returns the raw model-level output slots declared by the
// step this todo belongs to, used both to tell the agent the slot names on
// dispatch and to classify inbound uploads on receipt.
func (h *WebhookHandler) stepOutputsForTodo(task *model.TaskDetail, todo *model.Todo) []model.StepOutput {
	if task.Workflow == nil || len(task.Workflow.Steps) == 0 {
		return nil
	}
	idx := h.stepIndexForTodo(task, todo)
	if idx < 0 || idx >= len(task.Workflow.Steps) {
		return nil
	}
	return task.Workflow.Steps[idx].Outputs
}

// resolveStepInput resolves a single StepInput's link (StepIOLink) to the
// concrete upstream output file. Returns nil when the source step / output
// cannot be found among completed predecessor todos.
//
// When the task belongs to the project's primary workflow and the source step
// is owned by a different task (cross-task pipeline), the source todo is
// resolved against the predecessor task via Store.FindTaskForWorkflowStep and
// the artifact metadata is read from that task's artifacts.
func (h *WebhookHandler) resolveStepInput(task *model.TaskDetail, current *model.Todo, in model.StepInput) *protocol.TodoInputRef {
	// Candidate source todos: the in-task predecessor todo, or (when the task is
	// workflow-bound and the source lives in a predecessor task) every todo of
	// the source step in that task.
	var candidates []*model.Todo
	var srcTask *model.TaskDetail
	if srcTodo := h.resolveSourceTodo(task, current, in.Source); srcTodo != nil {
		candidates = append(candidates, srcTodo)
	} else if task.WorkflowRef != nil {
		step := in.Source.Step
		if strings.EqualFold(strings.TrimSpace(step), "prev") {
			// "prev" 在单 step 任务（step_from==step_to）里本任务没有前一个 todo，
			// prevTodoByOrder 返回 nil。此时应跨任务解析为 workflow 中当前 step 的
			// 前一个 step（如「剧本解析」的 prev = 「分镜拆解」），再查前序任务产出。
			step = h.resolvePrevStepName(task)
			if step == "" {
				return nil
			}
		}
		if st, tds, ok := h.store.FindTaskForWorkflowStep(store.SystemScope(), task.ProjectID, task.WorkflowRef.WorkflowName, step); ok {
			srcTask = st
			candidates = append(candidates, tds...)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	artifactTask := task
	if srcTask != nil {
		artifactTask = srcTask
	}
	// A single pipeline step is frequently split into several same-agent todos,
	// and the produced deliverable may be linked to any of them. Walk every
	// candidate todo's outputs and take the first output matching the requested
	// name. Matching is strict: candidates are deliverables only (bound via
	// todo.Outputs or the deliverable-kind artifact fallback), and an input
	// that declares a source output name only binds to exactly that name —
	// no more "matches anything" fallback that silently handed drafts to
	// downstream steps. Unresolved inputs surface as Resolved:false refs.
	want := strings.TrimSpace(in.Source.Output)
	for _, srcTodo := range candidates {
		if srcTodo == nil {
			continue
		}
		outputs := h.effectiveTodoOutputs(artifactTask, srcTodo)
		for i := range outputs {
			out := &outputs[i]
			if want != "" && out.OutputName != want {
				continue
			}
			ref := &protocol.TodoInputRef{
				Name:         in.Name,
				Description:  in.Description,
				MimeType:     in.MimeType,
				SourceStep:   in.Source.Step,
				SourceTodoID: srcTodo.ID,
				OutputName:   out.OutputName,
				ArtifactID:   out.ArtifactID,
				FileRef:      out.FileID,
				Resolved:     true,
			}
			if out.ArtifactID != "" {
				ref.DownloadUrl = h.artifactDownloadURL(out.ArtifactID)
			}
			// Enrich file name/size from the backing artifact when available.
			if a := findArtifact(artifactTask, out.ArtifactID); a != nil {
				ref.FileName = a.FileName
				ref.FileSize = a.FileSize
				if ref.MimeType == "" {
					ref.MimeType = a.MimeType
				}
			}
			return ref
		}
	}
	return nil
}

// effectiveTodoOutputs returns the deliverables of a todo to be fed into a
// downstream consumer. It prefers the explicitly bound todo.Outputs (populated
// when an agent uploads a file with a declared outputName) and falls back to
// the todo's artifacts classified as deliverable (newest first). Process-kind
// artifacts are never returned: agents routinely upload drafts and intermediate
// notes alongside the real deliverable, and silently feeding the first of them
// into a downstream step produced the 2026-09-01/02 wrong-input incidents.
// Tasks whose agents never declared an outputName resolve no inputs at all —
// the declared-but-unresolved ref (Resolved:false) surfaces that expectation
// instead of guessing.
func (h *WebhookHandler) effectiveTodoOutputs(task *model.TaskDetail, todo *model.Todo) []model.WorkflowStepOutputRef {
	if len(todo.Outputs) > 0 {
		outs := make([]model.WorkflowStepOutputRef, 0, len(todo.Outputs))
		for _, o := range todo.Outputs {
			outs = append(outs, model.WorkflowStepOutputRef{
				OutputName: o.OutputName,
				ArtifactID: o.ArtifactID,
				FileID:     o.FileRef,
			})
		}
		return outs
	}
	if task == nil {
		return nil
	}
	// Only declared deliverables feed downstream resolution. Process files
	// (drafts, intermediate notes) stay visible in the file tree but must
	// never be silently picked as a step input — see 2026-09-02, where the
	// first process file (01-intake.md) was resolved as the screenplay input
	// for the storyboard step. Deliverables are returned newest-first so a
	// first-match caller gets the latest revision.
	var idx []int
	for i := range task.Artifacts {
		a := &task.Artifacts[i]
		if a.TodoID == todo.ID && a.Kind == model.ArtifactKindDeliverable {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return nil
	}
	sort.Slice(idx, func(x, y int) bool {
		return task.Artifacts[idx[x]].CreatedAt.After(task.Artifacts[idx[y]].CreatedAt)
	})
	outs := make([]model.WorkflowStepOutputRef, 0, len(idx))
	for _, i := range idx {
		a := &task.Artifacts[i]
		outs = append(outs, model.WorkflowStepOutputRef{
			OutputName: a.OutputName,
			ArtifactID: a.TransferID,
			FileID:     a.ProjectFileID,
			FileName:   a.FileName,
			MimeType:   a.MimeType,
			FileSize:   a.FileSize,
		})
	}
	return outs
}

// resolveSourceTodo maps a StepIOLink to the upstream todo that produced the
// referenced output. "prev"/"" resolves to the immediately preceding todo by
// Order; a named step resolves via the task workflow (step name → matched
// todo using the same role/order alignment as applyWorkflowReviewFlags).
func (h *WebhookHandler) resolveSourceTodo(task *model.TaskDetail, current *model.Todo, link model.StepIOLink) *model.Todo {
	stepName := strings.TrimSpace(link.Step)
	if stepName == "" || strings.EqualFold(stepName, "prev") {
		return prevTodoByOrder(task, current)
	}
	if task.Workflow == nil {
		return nil
	}
	// Walk steps matched to todos in order (mirror applyWorkflowReviewFlags),
	// then return the todo whose step name matches the link's step name.
	cur := 0
	sorted := sortedTodosByOrder(task)
	for ti := 0; ti < len(sorted) && cur < len(task.Workflow.Steps); ti++ {
		step := task.Workflow.Steps[cur]
		if !h.stepMatchesTodo(step, sorted[ti]) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(step.Name), stepName) {
			return &sorted[ti]
		}
		cur++
	}
	return nil
}

// prevTodoByOrder returns the todo with the largest Order strictly less than
// current's Order (i.e. the immediately preceding step).
func prevTodoByOrder(task *model.TaskDetail, current *model.Todo) *model.Todo {
	sorted := sortedTodosByOrder(task)
	for i := len(sorted) - 1; i >= 0; i-- {
		if sorted[i].Order < current.Order {
			return &sorted[i]
		}
	}
	return nil
}

// resolvePrevStepName resolves the "prev" source-step link of a single-step
// task (step_from == step_to) to the name of the workflow step immediately
// before it. Single-step tasks own exactly one step, so the previous step can
// only live in a predecessor task; without this the "prev" input stays
// unresolved (Resolved=false) and the agent never receives the upstream
// deliverable. Returns "" when there is no preceding step.
func (h *WebhookHandler) resolvePrevStepName(task *model.TaskDetail) string {
	if task == nil || task.WorkflowRef == nil {
		return ""
	}
	proj, appErr := h.store.GetProject(store.Scope{UserID: task.UserID}, task.ProjectID)
	if appErr != nil || proj == nil {
		return ""
	}
	idx := task.WorkflowRef.WorkflowIndex
	if idx < 0 || idx >= len(proj.Workflows) {
		return ""
	}
	steps := proj.Workflows[idx].Steps
	prevIdx := task.WorkflowRef.StepFrom - 1
	if prevIdx < 0 || prevIdx >= len(steps) {
		return ""
	}
	return strings.TrimSpace(steps[prevIdx].Name)
}

// sortedTodosByOrder returns the task's todos ordered by their Order field.
func sortedTodosByOrder(task *model.TaskDetail) []model.Todo {
	sorted := make([]model.Todo, len(task.Todos))
	copy(sorted, task.Todos)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Order < sorted[j].Order })
	return sorted
}

// stepIndexForTodo returns the workflow step index that corresponds to the
// given todo, using the same ordered role-matching alignment as
// applyWorkflowReviewFlags. Returns -1 when no match is found.
func (h *WebhookHandler) stepIndexForTodo(task *model.TaskDetail, todo *model.Todo) int {
	if task.Workflow == nil {
		return -1
	}
	sorted := sortedTodosByOrder(task)
	cur := 0
	for ti := 0; ti < len(sorted) && cur < len(task.Workflow.Steps); ti++ {
		step := task.Workflow.Steps[cur]
		if !h.stepMatchesTodo(step, sorted[ti]) {
			continue
		}
		if sorted[ti].ID == todo.ID {
			return cur
		}
		cur++
	}
	return -1
}

// assigneeRoleOf returns the assigned agent's role, used to match a todo to its
// workflow step (mirrors internal/store.stepMatchesTodo semantics without
// importing that package's unexported helper).
func (h *WebhookHandler) assigneeRoleOf(todo model.Todo) string {
	if h.store == nil {
		return ""
	}
	if ag, ok := h.store.GetAgentByIDUnsafe(todo.Assignee.AgentID); ok && ag != nil {
		return ag.Role
	}
	return ""
}

// stepMatchesTodo reports whether the todo's assignee satisfies the step
// binding, mirroring internal/store.stepMatchesTodo.
func (h *WebhookHandler) stepMatchesTodo(step model.WorkflowStep, todo model.Todo) bool {
	if step.AgentID != "" {
		return step.AgentID == todo.Assignee.AgentID
	}
	role := strings.TrimSpace(step.Role)
	if role == "" {
		return false
	}
	todoRole := h.assigneeRoleOf(todo)
	return fuzzyMatch(todo.Assignee.Name, role) || fuzzyMatch(todoRole, role)
}

// fuzzyMatch is a loose containment-equality compare on either side.
func fuzzyMatch(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

// findArtifact returns the task artifact with the given transfer id.
func findArtifact(task *model.TaskDetail, transferID string) *model.TaskArtifact {
	if transferID == "" {
		return nil
	}
	for i := range task.Artifacts {
		if task.Artifacts[i].TransferID == transferID {
			return &task.Artifacts[i]
		}
	}
	return nil
}

// dedupeAttachedFiles removes entries sharing the same (ID, FileName) pair to
// avoid duplicate download references when task-level files overlap with
// predecessor todo products.
func dedupeAttachedFiles(files []model.TaskAttachedFile) []model.TaskAttachedFile {
	seen := make(map[string]bool, len(files))
	out := make([]model.TaskAttachedFile, 0, len(files))
	for _, f := range files {
		key := f.ID + "\x00" + f.FileName
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}

// agentHasPriorTodoInTask returns true if the same agent has already completed
// or failed an earlier todo in this task, meaning the agent's session already
// contains task-level context.
func agentHasPriorTodoInTask(task *model.TaskDetail, currentTodo *model.Todo) bool {
	for i := range task.Todos {
		t := &task.Todos[i]
		if t.ID == currentTodo.ID {
			continue
		}
		if t.Assignee.NodeID == currentTodo.Assignee.NodeID &&
			t.Order < currentTodo.Order &&
			(t.Status == "done" || t.Status == "failed") {
			return true
		}
	}
	return false
}

func buildTaskContext(task *model.TaskDetail, currentTodoID string) *protocol.TaskContext {
	ctx := &protocol.TaskContext{
		Title:       task.Title,
		Description: task.Description,
		Todos:       make([]protocol.TodoSummary, 0, len(task.Todos)),
	}
	for i := range task.Todos {
		t := &task.Todos[i]
		ctx.Todos = append(ctx.Todos, protocol.TodoSummary{
			TodoID:       t.ID,
			Order:        t.Order,
			Title:        t.Title,
			Status:       t.Status,
			AssigneeName: t.Assignee.Name,
			IsCurrent:    t.ID == currentTodoID,
		})
	}
	return ctx
}

// buildAllPriorResults returns results from all completed todos before the
// current one. Used when the agent has no prior session context for this task.
func buildAllPriorResults(h *WebhookHandler, task *model.TaskDetail, currentTodo *model.Todo) []protocol.TodoPriorResult {
	var results []protocol.TodoPriorResult
	for i := range task.Todos {
		t := &task.Todos[i]
		if t.Status != "done" || t.Order >= currentTodo.Order {
			continue
		}
		results = append(results, h.buildPriorResult(task, t))
	}
	return results
}

// buildCrossAgentPriorResults returns only results from todos completed by
// OTHER agents. The current agent's own prior results are already visible
// in its session via sessionKey continuity.
func buildCrossAgentPriorResults(h *WebhookHandler, task *model.TaskDetail, currentTodo *model.Todo) []protocol.TodoPriorResult {
	var results []protocol.TodoPriorResult
	for i := range task.Todos {
		t := &task.Todos[i]
		if t.Status != "done" || t.Order >= currentTodo.Order {
			continue
		}
		if t.Assignee.NodeID == currentTodo.Assignee.NodeID {
			continue
		}
		results = append(results, h.buildPriorResult(task, t))
	}
	return results
}

// buildPriorResult builds a prior-result payload for a completed todo,
// attaching the artifact products it produced (with download URLs) so the
// executing agent can fetch the predecessor's actual outputs.
func (h *WebhookHandler) buildPriorResult(task *model.TaskDetail, todo *model.Todo) protocol.TodoPriorResult {
	ref := protocol.TodoPriorResult{
		TodoID:  todo.ID,
		Title:   todo.Title,
		Summary: todo.Result.Summary,
		Output:  todo.Result.Output,
	}
	var arts []protocol.TodoArtifactRef
	for i := range task.Artifacts {
		a := &task.Artifacts[i]
		if a.TodoID != todo.ID {
			continue
		}
		arts = append(arts, protocol.TodoArtifactRef{
			TodoID:      a.TodoID,
			TransferID:  a.TransferID,
			FileName:    a.FileName,
			FileSize:    a.FileSize,
			MimeType:    a.MimeType,
			DownloadUrl: h.artifactDownloadURL(a.TransferID),
		})
	}
	if len(arts) > 0 {
		ref.Artifacts = arts
	}
	return ref
}

func (h *WebhookHandler) publishTaskAndTodoStatusChanges(task *model.TaskDetail, todoID, message, actorNodeID, cause string) {
	h.publish(context.Background(), task.PMAgent.NodeID, "task.status_changed", protocol.TaskStatusChangedPayload{
		TaskID:      task.ID,
		Status:      task.Status,
		ActorNodeID: strings.TrimSpace(actorNodeID),
		Cause:       strings.TrimSpace(cause),
		Reason:      message,
		Version:     task.Version,
	}, task.ID)

	todo := findTodo(task, todoID)
	if todo == nil {
		return
	}

	payload := protocol.TodoStatusChangedPayload{
		TaskID:      task.ID,
		TodoID:      todo.ID,
		Status:      todo.Status,
		ActorNodeID: strings.TrimSpace(actorNodeID),
		Cause:       strings.TrimSpace(cause),
		Reason:      message,
		Version:     task.Version,
		Message:     message,
	}
	if task.PMAgent.NodeID != "" {
		h.publish(context.Background(), task.PMAgent.NodeID, "todo.status_changed", payload, task.ID)
	}
	if todo.Assignee.NodeID != "" && todo.Assignee.NodeID != strings.TrimSpace(actorNodeID) && todo.Assignee.NodeID != task.PMAgent.NodeID {
		h.publish(context.Background(), todo.Assignee.NodeID, "todo.status_changed", payload, task.ID)
	}
}

func (h *WebhookHandler) publish(ctx context.Context, targetNode, msgType string, payload any, sessionKey string) {
	if h.client == nil {
		return
	}
	if _, err := h.client.Publish(ctx, targetNode, msgType, payload, sessionKey, nil); err != nil && h.log != nil {
		h.log.Warn("clawsynapse publish failed", zap.String("target_node", targetNode), zap.String("type", msgType), zap.Error(err))
	}
}

// NotifyTaskCanceled publishes todo.status_changed(canceled) notices to the
// assignee nodes of todos that were canceled while possibly in flight. The
// node's handleTaskControl reacts by stopping the active run for the task via
// the gateway /stop endpoint (adapter lifecycle cancel chain, spec §6). The
// node replies with a silent ACK; late reports from the stopped run are
// rejected by the regular TODO_CANCELED guards — no special suppression here.
func (h *WebhookHandler) NotifyTaskCanceled(taskID string, taskVersion int, notices []model.TodoCancelNotice) {
	for _, n := range notices {
		if strings.TrimSpace(n.NodeID) == "" {
			continue
		}
		h.publish(context.Background(), n.NodeID, "todo.status_changed", protocol.TodoStatusChangedPayload{
			TaskID:  taskID,
			TodoID:  n.TodoID,
			Status:  "canceled",
			Cause:   "user_cancel",
			Reason:  n.Reason,
			Version: taskVersion,
		}, taskID)
		if h.log != nil {
			h.log.Info("cancel notice published", zap.String("task_id", taskID), zap.String("todo_id", n.TodoID), zap.String("node_id", n.NodeID))
		}
	}
}

func defaultExecBrief() *protocol.TodoExecBrief {
	return &protocol.TodoExecBrief{
		Objective:    "执行分派的 Todo 任务；及时回报进度；完成后提交结果，失败时说明原因。",
		MustUseSkill: "tm-task-exec",
	}
}

func decodeWebhookMessage(raw string, out any) error {
	if err := json.Unmarshal([]byte(raw), out); err == nil {
		return nil
	}
	// LLMs occasionally produce JSON with escaping mistakes inside string
	// values (bare newlines/tabs/quotes). Attempt a conservative repair before
	// giving up so a single bad character does not silently drop an agent
	// message (e.g. a todo.ask with a long story synopsis in `question`).
	if fixed := fixLLMJSON(raw); fixed != raw {
		if err2 := json.Unmarshal([]byte(fixed), out); err2 == nil {
			return nil
		}
	}
	return json.Unmarshal([]byte(raw), out)
}

// fixLLMJSON repairs common JSON escaping mistakes produced by LLMs inside
// string values: bare control characters (newline, carriage return, tab) and
// bare double-quotes that appear as content rather than structure delimiters.
// It is conservative — outside strings it only tracks quote state, and the
// caller still falls back to the original error if the repaired text fails to
// parse.
func fixLLMJSON(raw string) string {
	var b strings.Builder
	b.Grow(len(raw) + 16)
	inString := false
	escaped := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if inString {
			if escaped {
				b.WriteByte(c)
				escaped = false
				continue
			}
			switch c {
			case '\\':
				b.WriteByte(c)
				escaped = true
			case '"':
				if jsonQuoteIsStructural(raw, i+1) {
					b.WriteByte(c)
					inString = false
				} else {
					b.WriteString(`\"`)
				}
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			default:
				b.WriteByte(c)
			}
		} else {
			if c == '"' {
				inString = true
			}
			b.WriteByte(c)
		}
	}
	return b.String()
}

// jsonQuoteIsStructural reports whether a double-quote at index i terminates a
// JSON string value (i.e. is followed, after whitespace, by a structural
// character or the end of input).
func jsonQuoteIsStructural(raw string, i int) bool {
	for ; i < len(raw); i++ {
		switch raw[i] {
		case ' ', '\t', '\n', '\r':
			continue
		case ',', ':', '}', ']':
			return true
		default:
			return false
		}
	}
	return true
}

// roleOfAgentNode resolves (role, name) for an agent node id, used by the
// workflow plan validation.
func (h *WebhookHandler) roleOfAgentNode(nodeID string) (role, name, agentID string, ok bool) {
	if h == nil || h.store == nil || nodeID == "" {
		return "", "", "", false
	}
	agent, appErr := h.store.GetAgentByNodeID(nodeID)
	if appErr != nil || agent == nil {
		return "", "", "", false
	}
	return agent.Role, agent.Name, agent.ID, true
}

// decodeTaskReplyLoose extracts a usable TaskReplyPayload from a malformed
// task.reply message. PM agents sometimes produce content with unescaped
// English quotes, making the whole message invalid JSON. We salvage:
//   - task_id: the session key (it is the task id / meeting id in practice)
//   - content: the first "content" string value found (best-effort regex), or
//     the raw message when nothing is extractable
//   - ui_blocks: nil (not recoverable from malformed JSON)
func decodeTaskReplyLoose(sessionKey, raw string) protocol.TaskReplyPayload {
	payload := protocol.TaskReplyPayload{
		TaskID:  strings.TrimSpace(sessionKey),
		Content: strings.TrimSpace(raw),
	}
	// Best-effort: pull the "content" value out of a half-broken JSON object.
	re := regexp.MustCompile(`"content"\s*:\s*"((?:[^"\\]|\\.)*)"`)
	if m := re.FindStringSubmatch(raw); len(m) > 1 {
		payload.Content = strings.TrimSpace(m[1])
	}
	// Same for task_id (prefer an embedded one over the session key).
	reID := regexp.MustCompile(`"task_id"\s*:\s*"([^"]+)"`)
	if m := reID.FindStringSubmatch(raw); len(m) > 1 {
		payload.TaskID = strings.TrimSpace(m[1])
	}
	return payload
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func messageIDFromMetadata(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	for _, key := range []string{"messageId", "message_id", "id"} {
		if value, ok := metadata[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func findTodo(task *model.TaskDetail, todoID string) *model.Todo {
	for i := range task.Todos {
		if task.Todos[i].ID == todoID {
			return &task.Todos[i]
		}
	}
	return nil
}

// warnTransferRejected surfaces an upload that will not be filed. A rejected
// transfer used to be completely invisible: the file stayed on the transfer
// volume, the sending agent reported success, and the platform showed nothing
// — the user only found out when a deliverable never appeared. Every rejection
// path now writes a ⚠️ system comment on the owning task (best-effort, never
// fatal), mirroring how task.plan_ready rejections are surfaced.
// warnOnce reports whether a warning keyed by `key` may be emitted now. The
// platform-side forwarder delivers every transfer.received twice (0.2ms apart),
// so an unguarded warning path writes each notice in duplicate.
func (h *WebhookHandler) warnOnce(key string) bool {
	h.rejectedWarnMu.Lock()
	defer h.rejectedWarnMu.Unlock()
	if h.rejectedWarned == nil {
		h.rejectedWarned = map[string]time.Time{}
	}
	now := time.Now()
	if last, ok := h.rejectedWarned[key]; ok && now.Sub(last) < transferWarnTTL {
		return false
	}
	// Opportunistic sweep so a long uptime cannot leak entries indefinitely.
	for k, t := range h.rejectedWarned {
		if now.Sub(t) >= transferWarnTTL {
			delete(h.rejectedWarned, k)
		}
	}
	h.rejectedWarned[key] = now
	return true
}

func (h *WebhookHandler) warnTransferRejected(taskID, fromNode, transferID, fileName, reason string) {
	if taskID == "" {
		// No declared task: try to still find a home for the warning.
		if owner, err := h.store.ResolveTransferOwner(fromNode); err == nil && owner != nil {
			taskID = owner.TaskID
		}
	}
	// Key on the transfer when we have one: two deliveries of the same
	// transfer are the same event, not two events. Malformed payloads have no
	// transfer id, so fall back to the (task, file, reason) triple.
	key := strings.TrimSpace(transferID)
	if key == "" {
		key = taskID + "|" + strings.TrimSpace(fileName) + "|" + reason
	}
	if !h.warnOnce(key) {
		return
	}
	content := fmt.Sprintf("⚠️ 文件上传未入库：%s（%s）。文件已传输到平台但未写入文件列表，请立即用 `clawsynapse transfer send --metadata taskId=… todoId=…` 重新上传。",
		strings.TrimSpace(fileName), reason)
	if transferID != "" {
		content += fmt.Sprintf(" transferId=%s", transferID)
	}
	if taskID == "" {
		if h.log != nil {
			h.log.Error("transfer rejected with no resolvable task",
				zap.String("transfer_id", transferID),
				zap.String("from_node", fromNode),
				zap.String("file_name", fileName),
				zap.String("reason", reason))
		}
		return
	}
	if _, cErr := h.store.AppendSystemTaskComment(taskID, content); cErr != nil && h.log != nil {
		h.log.Warn("append transfer rejection system comment failed",
			zap.String("task_id", taskID), zap.String("transfer_id", transferID), zap.Error(cErr))
	}
	h.notifySystemWarningToAgent(context.Background(), taskID, "", fromNode, content)
	// 运维发现层：交付物被拒 → 聚合进 ops_incident 工单（去重由 store 侧保证）。
	h.store.ReportOpsFinding(store.OpsFinding{
		RuleID:   model.RuleDeliverableReject,
		Severity: model.OpsSeverityCritical,
		Title:    "文件上传未入库",
		Summary:  fmt.Sprintf("%s（%s）", strings.TrimSpace(fileName), reason),
		TaskID:   taskID,
		NodeID:   fromNode,
	})
	// 干预留痕：把这次系统警告记进工单时间线（下发本体在上面既有链路完成）。
	h.store.RecordOpsWarning(model.RuleDeliverableReject, taskID, "", fromNode, content)
}

// warnUnboundDeliverable reports an upload that looks like a final deliverable
// but could not be matched to a declared output slot, so it was filed as a
// process artifact and will never reach downstream steps or the workflow
// diagram. The warning is throttled to one per todo: agents routinely upload
// several intermediate drafts, and unthrottled warnings would bury the timeline.
func (h *WebhookHandler) warnUnboundDeliverable(taskID, todoID, fromNode, transferID, fileName string, declaredNames []string) {
	key := taskID + "|" + todoID
	h.unboundWarnMu.Lock()
	if h.unboundWarned == nil {
		h.unboundWarned = map[string]bool{}
	}
	if h.unboundWarned[key] {
		h.unboundWarnMu.Unlock()
		return
	}
	h.unboundWarned[key] = true
	h.unboundWarnMu.Unlock()

	slots := strings.Join(declaredNames, " / ")
	if slots == "" {
		slots = "（该步骤未声明 mime 匹配的输出位）"
	}
	content := fmt.Sprintf(
		"⚠️ 疑似最终交付物未绑定：本次上传的 `%s` 未携带 outputName，已按**过程文件**入库，不会出现在工作流图上、也不会作为下游步骤的输入。"+
			"该步骤声明的输出位为：%s。"+
			"请立即用 `clawsynapse transfer send --metadata taskId=… --metadata todoId=… --metadata outputName=<输出位名>` 重新上传同名文件，"+
			"或在任务详情页手工「绑定为交付物」。",
		strings.TrimSpace(fileName), slots)
	if transferID != "" {
		content += fmt.Sprintf(" transferId=%s", transferID)
	}
	if _, cErr := h.store.AppendSystemTaskComment(taskID, content); cErr != nil && h.log != nil {
		h.log.Warn("append unbound deliverable system comment failed",
			zap.String("task_id", taskID), zap.String("todo_id", todoID),
			zap.String("transfer_id", transferID), zap.Error(cErr))
	}
	h.notifySystemWarningToAgent(context.Background(), taskID, todoID, fromNode, content)
	// 运维发现层：疑似交付物未绑定 → 聚合进 ops_incident 工单；
	// 绑定成功后由 store_artifact 的挂点进入观察期并自动关闭。
	h.store.ReportOpsFinding(store.OpsFinding{
		RuleID:   model.RuleDeliverableUnbound,
		Severity: model.OpsSeverityCritical,
		Title:    "交付物未绑定输出位",
		Summary:  fmt.Sprintf("%s 未携带 outputName，已按过程文件入库（输出位：%s）", strings.TrimSpace(fileName), slots),
		TaskID:   taskID,
		TodoID:   todoID,
		NodeID:   fromNode,
	})
	// 干预留痕：把这次系统警告记进工单时间线（下发本体在上面既有链路完成）。
	h.store.RecordOpsWarning(model.RuleDeliverableUnbound, taskID, todoID, fromNode, content)
}

// findTodoByID locates a todo by id in a task detail; nil when absent or id empty.
func findTodoByID(task *model.TaskDetail, todoID string) *model.Todo {
	if task == nil || todoID == "" {
		return nil
	}
	for i := range task.Todos {
		if task.Todos[i].ID == todoID {
			return &task.Todos[i]
		}
	}
	return nil
}

// notifySystemWarningToAgent pushes a system task-timeline warning to the
// responsible executor agent as a task.mention, so the agent wakes up and
// self-corrects instead of the warning sitting unread in the timeline
// (measured 2026-09-04: the storyboard agent never saw the unbound-deliverable
// warning until a human @-mentioned it 27 minutes later).
//
// Target resolution order: the todo's assignee → the assignee of the
// uploading node's active todo → the uploading node itself. Fire-and-forget:
// delivery failures are logged, never surfaced to the webhook caller.
// Callers keep their own throttling (unboundWarned / rejectedWarned), so each
// warning wakes at most one LLM round on the agent side.
func (h *WebhookHandler) notifySystemWarningToAgent(ctx context.Context, taskID, todoID, fromNode, content string) {
	if taskID == "" || h.client == nil {
		return
	}
	task := h.store.GetTaskInternal(taskID)
	if task == nil {
		return
	}
	target := strings.TrimSpace(fromNode)
	if todo := findTodoByID(task, todoID); todo != nil && todo.Assignee.NodeID != "" {
		target = todo.Assignee.NodeID
	} else if todoID == "" {
		if id, rerr := h.store.ResolveActiveTodoForNode(taskID, fromNode); rerr == nil && id != "" {
			if todo := findTodoByID(task, id); todo != nil && todo.Assignee.NodeID != "" {
				target = todo.Assignee.NodeID
			}
		}
	}
	if target == "" {
		return
	}
	payload := protocol.TaskMentionPayload{
		TaskID:      task.ID,
		ProjectID:   task.ProjectID,
		TodoID:      todoID,
		TaskTitle:   task.Title,
		TaskStatus:  task.Status,
		AuthorName:  "系统",
		UserContent: content,
		Content:     "系统检测到文件交付问题（警告全文见 user_content）。请立即按警告中的指引重新上传；`clawsynapse transfer send` 必须携带 --metadata taskId=… --metadata todoId=…，声明输出位的步骤还要加 --metadata outputName=<输出位名>。",
	}
	metadata := map[string]any{"source": "system_warning", "task_id": taskID}
	if todoID != "" {
		metadata["todo_id"] = todoID
	}
	if _, err := h.client.Publish(ctx, target, "task.mention", payload, task.ID, metadata); err != nil && h.log != nil {
		h.log.Warn("push system warning to agent failed",
			zap.String("task_id", taskID), zap.String("todo_id", todoID),
			zap.String("target_node", target), zap.Error(err))
	}
}

// PublishOpsMention 统一干预编排器的推送适配器：把 store 侧的
// OpsPublishRequest 组装成 task.mention 并经节点 daemon 下发。
// 由 app 层注册为 store 的 opsPublishHook。
func (h *WebhookHandler) PublishOpsMention(ctx context.Context, req store.OpsPublishRequest) error {
	if h == nil || h.client == nil {
		return errors.New("clawsynapse client unavailable")
	}
	payload := protocol.TaskMentionPayload{
		TaskID:      req.TaskID,
		ProjectID:   req.ProjectID,
		TodoID:      req.TodoID,
		TaskTitle:   req.TaskTitle,
		TaskStatus:  req.TaskStatus,
		AuthorName:  req.AuthorName,
		UserContent: req.Content,
		Content:     req.Content,
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["task_id"] = req.TaskID
	if _, err := h.client.Publish(ctx, req.TargetNode, "task.mention", payload, req.TaskID, metadata); err != nil {
		return err
	}
	return nil
}

func (h *WebhookHandler) handleTransferReceived(c *gin.Context, webhook protocol.WebhookPayload) {
	var msg struct {
		TransferID string `json:"transferId"`
		FileName   string `json:"fileName"`
		FileSize   int64  `json:"fileSize"`
		LocalPath  string `json:"localPath"`
		MimeType   string `json:"mimeType"`
	}
	if err := decodeWebhookMessage(webhook.Message, &msg); err != nil || msg.TransferID == "" {
		taskID, _ := webhook.Metadata["taskId"].(string)
		h.warnTransferRejected(taskID, webhook.From, msg.TransferID, msg.FileName,
			"消息体无法解析（BAD_PAYLOAD）")
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid transfer.received message"))
		return
	}

	taskID, _ := webhook.Metadata["taskId"].(string)
	todoID, _ := webhook.Metadata["todoId"].(string)
	if taskID == "" {
		// Platform-side safety net for uploads that forgot the task context
		// (see 2026-09-02: the screenwriter node sent seven files with no
		// metadata at all; every one of them was silently 422'd and the agent
		// kept reporting success). Fall back to whatever the sending node is
		// currently assigned to instead of dropping the file on the floor.
		if owner, ownerErr := h.store.ResolveTransferOwner(webhook.From); ownerErr == nil && owner != nil {
			taskID = owner.TaskID
			if todoID == "" {
				todoID = owner.TodoID
			}
			if h.log != nil {
				h.log.Warn("transfer.received without taskId; owner inferred",
					zap.String("transfer_id", msg.TransferID),
					zap.String("from_node", webhook.From),
					zap.String("task_id", taskID),
					zap.String("todo_id", todoID))
			}
		}
	}
	if taskID == "" {
		h.warnTransferRejected("", webhook.From, msg.TransferID, msg.FileName,
			"缺少 metadata.taskId，且无法从该节点的在办任务推断归属")
		transport.WriteError(c, transport.Validation("missing metadata", map[string]any{"taskId": "required"}))
		return
	}
	outputName, _ := webhook.Metadata["outputName"].(string)

	mimeType := msg.MimeType
	if strings.TrimSpace(mimeType) == "" {
		// The ClawSynapse CLI sends mimeType:"" for every transfer; backfill
		// from the file name so downstream mime-based classification
		// (inferOutputBinding) sees the real type instead of "unknown".
		mimeType = store.InferMimeFromName(msg.FileName)
	}

	artifact := model.TaskArtifact{
		TransferID: msg.TransferID,
		TaskID:     taskID,
		TodoID:     todoID,
		FileName:   msg.FileName,
		FileSize:   msg.FileSize,
		LocalPath:  msg.LocalPath,
		MimeType:   mimeType,
		FromNodeID: webhook.From,
		CreatedAt:  time.Now().UTC(),
		OutputName: outputName,
	}
	// Look up the output slots the owning step declares. An agent that was
	// never told the slot name uploads without outputName; matching the file
	// against the declaration lets the backend file it as the deliverable it
	// actually is instead of silently degrading it to a process artifact.
	var declared []model.StepOutput
	if todoID != "" {
		if task := h.store.GetTaskInternal(taskID); task != nil {
			for i := range task.Todos {
				if task.Todos[i].ID == todoID {
					declared = h.stepOutputsForTodo(task, &task.Todos[i])
					break
				}
			}
		}
	}

	// Classify up front so every copy of the artifact handed downstream
	// (SaveArtifact AND the separate SaveProjectFileFromArtifact call below)
	// carries the same file nature. SaveArtifact re-derives it as a fallback.
	if artifact.OutputName != "" {
		artifact.Kind = model.ArtifactKindDeliverable
	} else {
		artifact.Kind = model.ArtifactKindProcess
	}

	filing, appErr := h.store.SaveArtifactWithFiling(artifact, declared)
	if appErr != nil {
		// A rejection does not automatically mean something is missing. Agents
		// re-run finished steps and the forwarder re-sends transfers, so a
		// 409 frequently lands on a file that is already filed under the same
		// name. Screaming "未入库" for it is actively misleading — the user
		// sees a deliverable's name in a scary comment and concludes the
		// deliverable was lost, when it is sitting in the file list.
		if h.store.HasArtifactNamed(taskID, msg.FileName) {
			if h.log != nil {
				h.log.Info("duplicate transfer rejected; identical file already filed",
					zap.String("transfer_id", msg.TransferID),
					zap.String("task_id", taskID),
					zap.String("todo_id", todoID),
					zap.String("file_name", msg.FileName),
					zap.String("code", appErr.Code))
			}
		} else {
			h.warnTransferRejected(taskID, webhook.From, msg.TransferID, msg.FileName,
				"入库被拒（"+appErr.Code+"："+appErr.Message+"）")
		}
		transport.WriteError(c, appErr)
		return
	}
	// Mirror the authoritative classification back onto the local copy: the
	// backend may have inferred a binding the caller did not declare, and the
	// ProjectFile created below must carry the same kind.
	artifact.OutputName = filing.OutputName
	if filing.Bound {
		artifact.Kind = model.ArtifactKindDeliverable
	} else {
		artifact.Kind = model.ArtifactKindProcess
	}
	if filing.BoundBy == "inferred" && h.log != nil {
		h.log.Info("transfer bound to declared output slot by inference",
			zap.String("transfer_id", msg.TransferID),
			zap.String("task_id", taskID),
			zap.String("todo_id", todoID),
			zap.String("output_name", filing.OutputName))
	}
	if filing.UnboundWarn {
		h.warnUnboundDeliverable(taskID, todoID, webhook.From, msg.TransferID, msg.FileName, filing.DeclaredNames)
	}

	// Auto-create ProjectFile and copy artifact to project files volume.
	if h.projectFileStorage != nil {
		if projectFile, err := h.store.SaveProjectFileFromArtifact(artifact); err != nil {
			if h.log != nil {
				h.log.Warn("failed to create project file from artifact",
					zap.String("transfer_id", msg.TransferID),
					zap.Error(err))
			}
		} else if projectFile != nil {
			// Copy the file from transfer volume to project files volume.
			dstPath, copyErr := h.projectFileStorage.CopyFromPath(
				c.Request.Context(),
				msg.LocalPath,
				projectFile.ProjectID,
				taskID,
				artifact.FromAgentID,
				msg.FileName,
			)
			if copyErr != nil {
				if h.log != nil {
					h.log.Warn("failed to copy artifact to project files",
						zap.String("transfer_id", msg.TransferID),
						zap.Error(copyErr))
				}
			} else {
				_ = h.store.SetProjectFileLocalPath(projectFile.ID, dstPath)
			}
		}
	}

	transport.WriteData(c, http.StatusOK, gin.H{
		"transfer_id": msg.TransferID,
		"task_id":     taskID,
		"kind":        artifact.Kind,
		"output_name": filing.OutputName,
		"bound":       filing.Bound,
		"bound_by":    filing.BoundBy,
	})
}

func (h *WebhookHandler) handleKnowledgeQuery(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.KnowledgeQueryPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid knowledge.query message"))
		return
	}

	if strings.TrimSpace(payload.Query) == "" {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "query is required"))
		return
	}
	if payload.TopK <= 0 {
		payload.TopK = 5
	}
	if payload.MinScore <= 0 {
		payload.MinScore = 0.7
	}

	// Resolve agent → user ownership
	userID, appErr := h.store.ResolveKnowledgeDocOwnerByAgentNode(webhook.From)
	if appErr != nil {
		h.sendKnowledgeError(webhook.From, payload, "agent not recognized: "+appErr.Message)
		transport.WriteError(c, appErr)
		return
	}

	// Validate project ownership
	if payload.ProjectID != "" {
		if appErr := h.store.ValidateProjectOwnership(store.Scope{UserID: userID}, payload.ProjectID); appErr != nil {
			h.sendKnowledgeError(webhook.From, payload, "access denied to project")
			transport.WriteError(c, appErr)
			return
		}
	}

	// Perform search
	results, err := h.knowledgeSearch(c.Request.Context(), userID, payload)
	if err != nil {
		if h.log != nil {
			h.log.Error("knowledge query search failed", zap.Error(err))
		}
		h.sendKnowledgeError(webhook.From, payload, "search failed: "+err.Error())
		transport.WriteData(c, http.StatusOK, gin.H{"status": "error", "error": err.Error()})
		return
	}

	// Send results back to agent
	resultPayload := protocol.KnowledgeResultPayload{
		QueryID:   payload.QueryID,
		ProjectID: payload.ProjectID,
		Results:   results,
	}
	h.publish(context.Background(), webhook.From, "knowledge.result", resultPayload, payload.QueryID)
	transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "results_count": len(results)})
}

func (h *WebhookHandler) knowledgeSearch(ctx context.Context, userID string, payload protocol.KnowledgeQueryPayload) ([]protocol.KnowledgeResultItem, error) {
	// Try vector search first
	if h.embedder != nil && h.qdrant != nil {
		embeddings, err := h.embedder.Embed(ctx, []string{payload.Query})
		if err != nil {
			return nil, err
		}
		if len(embeddings) > 0 && len(embeddings[0]) > 0 {
			mustConditions := []knowledge.QdrantCondition{
				{Key: "user_id", Match: map[string]any{"value": userID}},
			}
			var filter *knowledge.QdrantFilter
			if payload.ProjectID != "" {
				filter = &knowledge.QdrantFilter{
					Must: mustConditions,
					Should: []knowledge.QdrantCondition{
						{Key: "project_id", Match: map[string]any{"value": payload.ProjectID}},
					},
				}
			} else {
				filter = &knowledge.QdrantFilter{Must: mustConditions}
			}

			hits, err := h.qdrant.Search(ctx, embeddings[0], filter, payload.TopK)
			if err != nil {
				return nil, err
			}

			var results []protocol.KnowledgeResultItem
			for _, hit := range hits {
				if hit.Score < payload.MinScore {
					continue
				}
				chunkID, _ := hit.Payload["chunk_id"].(string)
				docID, _ := hit.Payload["document_id"].(string)

				chunks, err := h.store.GetKnowledgeChunksByIDs([]string{chunkID})
				if err != nil || len(chunks) == 0 {
					continue
				}
				chunk := chunks[0]

				results = append(results, protocol.KnowledgeResultItem{
					ChunkID:       chunkID,
					DocumentID:    docID,
					DocumentTitle: h.store.GetKnowledgeDocTitle(docID),
					Content:       chunk.Content,
					Score:         hit.Score,
					ChunkIndex:    chunk.ChunkIndex,
					Metadata:      chunk.Metadata,
				})
			}
			return results, nil
		}
	}

	// Fallback to text search
	var projectID *string
	if payload.ProjectID != "" {
		projectID = &payload.ProjectID
	}
	chunks, err := h.store.SearchKnowledgeChunks(ctx, store.Scope{UserID: userID}, projectID, payload.Query, payload.TopK)
	if err != nil {
		return nil, err
	}
	var results []protocol.KnowledgeResultItem
	for _, chunk := range chunks {
		results = append(results, protocol.KnowledgeResultItem{
			ChunkID:       chunk.ID,
			DocumentID:    chunk.DocumentID,
			DocumentTitle: h.store.GetKnowledgeDocTitle(chunk.DocumentID),
			Content:       chunk.Content,
			Score:         1.0,
			ChunkIndex:    chunk.ChunkIndex,
			Metadata:      chunk.Metadata,
		})
	}
	return results, nil
}

func (h *WebhookHandler) sendKnowledgeError(targetNode string, payload protocol.KnowledgeQueryPayload, errMsg string) {
	h.publish(context.Background(), targetNode, "knowledge.result", protocol.KnowledgeResultPayload{
		QueryID:   payload.QueryID,
		ProjectID: payload.ProjectID,
		Error:     errMsg,
	}, payload.QueryID)
}

func (h *WebhookHandler) enrichAttachedFiles(files []model.TaskAttachedFile) []protocol.TaskAttachedFileRef {
	return agentfile.EnrichWithDownloadURLs(files, h.externalURL, h.jwtSecret, h.downloadTTL)
}

// ——— Meeting Room webhook handlers ———

// resolveMeetingSenderName looks up an agent's display name by node ID.
func (h *WebhookHandler) resolveMeetingSenderName(nodeID string) string {
	agent, err := h.store.GetAgentByNodeID(nodeID)
	if err != nil {
		return nodeID
	}
	return agent.Name
}

// extractContentFromTaskWrapper extracts the content value from a malformed
// non-JSON task wrapper like:
//
//	{task_id: <id>, content: <text>}
//
// The PM agent's SKILL sometimes sends this format instead of proper JSON.
// Returns empty string if no extraction was possible.
func extractContentFromTaskWrapper(raw string) string {
	idx := strings.Index(raw, "content:")
	if idx < 0 {
		return ""
	}
	val := strings.TrimSpace(raw[idx+8:]) // skip "content:"
	// Strip trailing closing brace (clean only the outermost one)
	val = strings.TrimSuffix(strings.TrimSpace(val), "}")
	val = strings.TrimSpace(val)
	// Strip surrounding quotes if present
	val = strings.Trim(val, `"'`)
	val = strings.TrimSpace(val)
	if val == "" || val == raw {
		return ""
	}
	return val
}

// isMeetingCompleted checks if a meeting (by ID) is in completed status.
func (h *WebhookHandler) isMeetingCompleted(meetingID string) bool {
	meeting, err := h.store.GetMeeting(store.SystemScope(), meetingID)
	if err != nil || meeting == nil {
		return false
	}
	return meeting.Status == model.MeetingCompleted
}

// broadcastMeetingReply routes an agent's meeting response to the right recipient.
//   - PM Agent reply with @mention → forward only to the mentioned executor
//   - PM Agent reply without @mention → nothing to forward (just a meeting message)
//   - Executor reply → forward only to PM (so PM can coordinate next steps)
//
// All messages are stored as meeting messages BEFORE this is called, so the
// platform always has a record of every exchange.
func (h *WebhookHandler) broadcastMeetingReply(ctx context.Context, meeting *model.Meeting, fromNodeID, content string) {
	if h.client == nil {
		return
	}

	// Determine who the PM is
	var pmNodeID string
	if meeting.HostAgentID != "" {
		if agent, ok := h.store.GetAgentByIDUnsafe(meeting.HostAgentID); ok {
			pmNodeID = agent.NodeID
		}
	}

	// Build a lookup: agent name → node ID (from meeting participants)
	nameToNode := make(map[string]string)
	for _, p := range meeting.Participants {
		if agent, ok := h.store.GetAgentByIDUnsafe(p.AgentID); ok {
			if agent.NodeID != "" {
				nameToNode[agent.Name] = agent.NodeID
			}
		}
	}

	var targetIDs []string

	if fromNodeID == pmNodeID {
		// PM replied — check for @mentions to route to specific executor
		mentionedNodes := parseAgentMentions(content, nameToNode)
		for _, nodeID := range mentionedNodes {
			if nodeID != "" && nodeID != pmNodeID {
				targetIDs = append(targetIDs, nodeID)
			}
		}
		if len(targetIDs) == 0 {
			return // No @mention, just a meeting message — nothing to forward
		}
	} else {
		// Executor replied → forward only to PM
		if pmNodeID != "" {
			targetIDs = append(targetIDs, pmNodeID)
		}
		if len(targetIDs) == 0 {
			return
		}
	}

	// Hard guard (issue #2): only deliver to actual meeting participants or the
	// host. nameToNode is built strictly from meeting.Participants, so any node
	// outside it is illegitimate and must be dropped.
	allowedNodes := make(map[string]bool, len(nameToNode)+1)
	for _, n := range nameToNode {
		allowedNodes[n] = true
	}
	if pmNodeID != "" {
		allowedNodes[pmNodeID] = true
	}
	guarded := targetIDs[:0]
	for _, id := range targetIDs {
		if allowedNodes[id] {
			guarded = append(guarded, id)
		} else if h.log != nil {
			h.log.Warn("dropping meeting broadcast target outside participants",
				zap.String("meeting_id", meeting.ID),
				zap.String("node", id))
		}
	}
	targetIDs = guarded
	if len(targetIDs) == 0 {
		return
	}

	// Build reference file download URLs if meeting has attached files
	var fileSection string
	refFiles := meeting.AttachedFiles
	if len(refFiles) == 0 {
		// Fallback: resolve from raw FileIDs for meetings created before AttachedFiles
		for _, fid := range meeting.FileIDs {
			f, _ := h.store.GetProjectFileByID(fid)
			if f != nil {
				refFiles = append(refFiles, model.MeetingAttachedFile{
					ID: f.ID, FileName: f.FileName, FileSize: f.FileSize, MimeType: f.MimeType, Source: f.Source,
				})
			}
		}
	}
	if len(refFiles) > 0 && h.externalURL != "" && len(h.jwtSecret) > 0 {
		var refs []string
		for _, f := range refFiles {
			token, err := agentfile.GenerateDownloadToken(h.jwtSecret, f.ID, h.downloadTTL)
			if err == nil {
				url := agentfile.BuildDownloadURL(h.externalURL, f.ID, token)
				name := f.FileName
				if name == "" {
					name = f.ID
				}
				refs = append(refs, fmt.Sprintf("- **%s**: `%s`", name, url))
			}
		}
		if len(refs) > 0 {
			fileSection = "\n\n---\n**参考文件下载链接：**\n" + strings.Join(refs, "\n")
		}
	}

	msg := protocol.PMTaskMessage{
		TaskID:      meeting.ID,
		ProjectID:   meeting.ProjectID,
		Content:     content + fileSection,
		UserContent: content,
		IsInitial:   false,
	}

	for _, nodeID := range targetIDs {
		_, _ = h.client.Publish(ctx, nodeID, "chat.message", msg, meeting.ID, map[string]any{"context": "meeting", "meeting_id": meeting.ID})
	}
}

// parseAgentMentions extracts @mentioned agent names from content and returns
// their node IDs. Matches @后连续的中文/字母/数字/下划线（不含 \n 等控制字符）。
func parseAgentMentions(content string, nameToNode map[string]string) []string {
	var nodeIDs []string
	re := regexp.MustCompile(`@([\p{Han}\w]+)`)
	matches := re.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		name := strings.TrimRight(m[1], "，,、。.）)（(：:")
		if nodeID, ok := nameToNode[name]; ok {
			nodeIDs = append(nodeIDs, nodeID)
		}
	}
	return nodeIDs
}

// handleMeetingMessage processes meeting.message sent by agents.
func (h *WebhookHandler) handleMeetingMessage(c *gin.Context, payload protocol.WebhookPayload) {
	var msg protocol.MeetingMessagePayload
	if err := json.Unmarshal([]byte(normalizeWebhookMessage(payload.Message)), &msg); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid meeting.message payload"))
		return
	}

	// PROBLEM 2 (defensive) — drop a legacy meeting.message only when the agent
	// explicitly sends an empty "phase" field. Legacy meeting.message payloads
	// have no phase field, so genuine speech (no phase field) is preserved.
	if phase, has := phaseFromRaw(payload.Message); has && phase == "" {
		if !meetingMsgExemptFromNoPhase("", payload.From, "") {
			if h.log != nil {
				h.log.Debug("dropping meeting.message without phase",
					zap.String("meeting_id", msg.MeetingID),
					zap.String("from", payload.From),
					zap.String("reason", "no_phase"))
			}
			transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true, "reason": "no_phase"})
			return
		}
	}

	storeMsg := &model.MeetingMessage{
		MeetingID:  msg.MeetingID,
		SenderType: "agent",
		SenderID:   payload.From,
		SenderName: h.resolveMeetingSenderName(payload.From),
		Content:    msg.Content,
	}

	if h.isMeetingCompleted(msg.MeetingID) {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ignored", "reason": "meeting completed"})
		return
	}

	if _, appErr := h.store.AddMeetingMessage(store.SystemScope(), storeMsg); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Update meeting to in_progress if still waiting
	_ = h.store.UpdateMeetingStatus(store.SystemScope(), msg.MeetingID, model.MeetingInProgress)

	// Broadcast to other participants
	meeting, _ := h.store.GetMeeting(store.SystemScope(), msg.MeetingID)
	if meeting != nil {
		h.broadcastMeetingReply(context.Background(), meeting, payload.From, msg.Content)
	}

	transport.WriteData(c, http.StatusOK, gin.H{"status": "ok"})
}

// isMeetingChatContext reports whether a chat.message carries meeting scope,
// i.e. it should be routed to the meeting channel rather than a 1:1 agent chat.
func isMeetingChatContext(meta map[string]any) bool {
	if meta == nil {
		return false
	}
	if ctx, ok := meta["context"].(string); ok && ctx == "meeting" {
		return true
	}
	if _, ok := meta["meeting_id"]; ok {
		return true
	}
	return false
}

// isMeetingSessionKey reports whether the given session key is a meeting id,
// so inbound chat.message from agents (which only carry --session-key, no
// metadata) can still be routed to the meeting channel instead of 1:1 chat.
func (h *WebhookHandler) isMeetingSessionKey(sessionKey string) bool {
	if sessionKey == "" {
		return false
	}
	m, err := h.store.GetMeeting(store.SystemScope(), sessionKey)
	return err == nil && m != nil
}

// meetingIDFromMetadata extracts the meeting id from webhook metadata.
func meetingIDFromMetadata(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	for _, key := range []string{"meeting_id", "meetingId", "meetingID"} {
		if v, ok := meta[key].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// normalizeWebhookMessage tolerates agent runtimes that double-encode the JSON
// payload as base64. Some agent nodes emit base64 for larger meeting.chat /
// meeting.control bodies (e.g. a long summary), which makes a naive
// json.Unmarshal fail with BAD_PAYLOAD and can deadlock the whole meeting.
// We try the raw body first, then fall back to base64 (std + URL-safe) decode.
// Truly malformed payloads still fail the downstream unmarshal, so behaviour
// for invalid input is unchanged.
func normalizeWebhookMessage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || json.Valid([]byte(raw)) {
		return raw
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
		if dec, err := enc.DecodeString(raw); err == nil {
			if json.Valid(dec) {
				return string(dec)
			}
		}
	}
	return raw
}

// ——— Unified Meeting Chat handler (v2 protocol) ———

// handleMeetingChatUnified processes the new meeting.chat type.
// This is the primary meeting conversation handler going forward.
// It replaces the old route of routing chat.message (with meeting metadata)
// and task.reply (with task_id = meeting_id) through separate handlers.
func (h *WebhookHandler) handleMeetingChatUnified(c *gin.Context, webhook protocol.WebhookPayload) {
	var msg protocol.MeetingChatPayload
	if err := json.Unmarshal([]byte(normalizeWebhookMessage(webhook.Message)), &msg); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid meeting.chat payload"))
		return
	}

	meetingID := strings.TrimSpace(msg.MeetingID)
	if meetingID == "" {
		meetingID = strings.TrimSpace(webhook.SessionKey)
	}
	if meetingID == "" {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}

	if h.isMeetingCompleted(meetingID) {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ignored", "reason": "meeting completed"})
		return
	}

	meeting, _ := h.store.GetMeeting(store.SystemScope(), meetingID)
	if meeting == nil {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}

	phase := strings.TrimSpace(msg.Phase)

	// Resolve whether the incoming sender is the host (PM) node.
	var pmNodeID string
	if meeting.HostAgentID != "" {
		if agent, ok := h.store.GetAgentByIDUnsafe(meeting.HostAgentID); ok {
			pmNodeID = agent.NodeID
		}
	}
	isHost := webhook.From == pmNodeID

	// PROBLEM 2 — filter meeting.chat messages with no/empty phase.
	// Participant (and other non-host) messages without a phase are protocol
	// noise (e.g. agents "explaining" why they stay silent). They are neither
	// stored nor forwarded. User messages and host messages are exempt, so the
	// host's replies to user interruptions — which legitimately carry no phase —
	// are preserved.
	if phase == "" {
		if strings.TrimSpace(msg.Role) == "user" || isHost {
			// keep (user message, or host message such as a user-interruption reply)
		} else {
			if h.log != nil {
				h.log.Debug("dropping meeting.chat without phase",
					zap.String("meeting_id", meetingID),
					zap.String("from", webhook.From),
					zap.String("reason", "no_phase"))
			}
			transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true, "reason": "no_phase"})
			return
		}
	}

	// PROBLEM 1 — suppress duplicate outgoing naming from the same sender to the
	// same target+phase within a short window. This is a backend backstop for the
	// host over-@-ing the same participant in the speak phase (e.g. firing 3
	// identical speak messages in one second).
	target := strings.TrimSpace(msg.Target)
	if !shouldAllowMeetingOutgoing(meetingID, webhook.From, target, phase) {
		if h.log != nil {
			h.log.Warn("duplicate meeting.chat suppressed",
				zap.String("meeting_id", meetingID),
				zap.String("from", webhook.From),
				zap.String("target", target),
				zap.String("phase", phase))
		}
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true, "reason": "duplicate_outgoing"})
		return
	}

	content := strings.TrimSpace(msg.Content)
	cleaned := cleanConversationNoise(content)
	if cleaned == "" {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}

	storeMsg := &model.MeetingMessage{
		MeetingID:    meetingID,
		SenderType:   "agent",
		SenderID:     webhook.From,
		SenderName:   h.resolveMeetingSenderName(webhook.From),
		Phase:        phase,
		Target:       target,
		ContextBrief: strings.TrimSpace(msg.ContextBrief),
		Content:      cleaned,
		UIBlocks:     msg.UIBlocks,
	}
	if _, appErr := h.store.AddMeetingMessage(store.SystemScope(), storeMsg); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Record the outgoing for de-duplication only after we know it will be stored.
	markMeetingOutgoing(meetingID, webhook.From, target, phase)

	_ = h.store.UpdateMeetingStatus(store.SystemScope(), meetingID, model.MeetingInProgress)

	h.broadcastMeetingReplyUnified(context.Background(), meeting, webhook.From, msg, cleaned)

	// Reset the meeting inactivity timeout watchdog — the meeting is still active.
	if h.onMeetingActivity != nil {
		h.onMeetingActivity(meetingID)
	}

	transport.WriteData(c, http.StatusOK, gin.H{"status": "ok"})
}

// handleMeetingResponse processes meeting.response / meeting.ack messages.
//
// Historically these were dropped wholesale as "internal ACK artifacts", which
// caused two linked failures: (1) participant agents' real speech (delivered as
// meeting.response) never entered the transcript → invisible on the meeting
// page; (2) the host never received the "participant ready / done" signal →
// the meeting stalled after the prep phase and only recovered via the 30-min
// idle watchdog.
//
// Fix: parse the payload defensively (JSON MeetingChatPayload, or raw text as a
// fallback), strip ACK / runtime noise via cleanConversationNoise, and:
//   - pure ACK (collapses to empty) → silently acknowledge (old behaviour);
//   - genuine content → store as a meeting message AND forward to the host
//     (participant → host) so the meeting keeps advancing.
func (h *WebhookHandler) handleMeetingResponse(c *gin.Context, webhook protocol.WebhookPayload) {
	// Defensive parse: the payload is usually a MeetingChatPayload-shaped JSON,
	// but some agent runtimes send a bare text body. Try JSON first, then fall
	// back to treating the whole message as content.
	var msg protocol.MeetingChatPayload
	raw := normalizeWebhookMessage(webhook.Message)
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		msg = protocol.MeetingChatPayload{Content: strings.TrimSpace(webhook.Message)}
	}

	// Resolve the meeting id: explicit payload field wins, else the sessionKey.
	meetingID := strings.TrimSpace(msg.MeetingID)
	if meetingID == "" {
		meetingID = strings.TrimSpace(webhook.SessionKey)
	}
	if meetingID == "" {
		// No meeting context — nothing to store; ack silently as before.
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}

	meeting, _ := h.store.GetMeeting(store.SystemScope(), meetingID)
	if meeting == nil {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}
	if h.isMeetingCompleted(meetingID) {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ignored", "reason": "meeting completed"})
		return
	}

	// PROBLEM 2 — filter meeting.response messages with no/empty phase.
	// Participant (and other non-host) messages without a phase are protocol
	// noise (e.g. agents "explaining" why they stay silent). They are neither
	// stored nor forwarded. User messages and host messages are exempt, so the
	// host's replies to user interruptions — which legitimately carry no phase —
	// are preserved.
	{
		phase := strings.TrimSpace(msg.Phase)
		if phase == "" {
			hostNodeID := h.resolveMeetingHostNodeID(meeting)
			if !meetingMsgExemptFromNoPhase(msg.Role, webhook.From, hostNodeID) {
				if h.log != nil {
					h.log.Debug("dropping meeting.response without phase",
						zap.String("meeting_id", meetingID),
						zap.String("from", webhook.From),
						zap.String("reason", "no_phase"))
				}
				transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true, "reason": "no_phase"})
				return
			}
		}
	}

	// Strip ACK / WAITING / runtime noise but keep genuine speech.
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		content = strings.TrimSpace(webhook.Message)
	}
	cleaned := cleanConversationNoise(content)
	if cleaned == "" {
		// Pure ACK / receipt → preserve original silent-acknowledge behaviour.
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}

	storeMsg := &model.MeetingMessage{
		MeetingID:    meetingID,
		SenderType:   "agent",
		SenderID:     webhook.From,
		SenderName:   h.resolveMeetingSenderName(webhook.From),
		Phase:        strings.TrimSpace(msg.Phase),
		Target:       strings.TrimSpace(msg.Target),
		ContextBrief: strings.TrimSpace(msg.ContextBrief),
		Content:      cleaned,
		UIBlocks:     msg.UIBlocks,
	}
	if _, appErr := h.store.AddMeetingMessage(store.SystemScope(), storeMsg); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	_ = h.store.UpdateMeetingStatus(store.SystemScope(), meetingID, model.MeetingInProgress)

	// Forward to the host so it receives the "participant done" signal and can
	// advance the agenda. Use the unified router (participant → host).
	h.broadcastMeetingReplyUnified(context.Background(), meeting, webhook.From, msg, cleaned)

	// Reset the inactivity watchdog — the meeting is still active.
	if h.onMeetingActivity != nil {
		h.onMeetingActivity(meetingID)
	}

	transport.WriteData(c, http.StatusOK, gin.H{"status": "ok"})
}

// handleMeetingControlUnified processes the enhanced meeting.control type.
// Supports: start, conclude, minutes, ping, status.
func (h *WebhookHandler) handleMeetingControlUnified(c *gin.Context, webhook protocol.WebhookPayload) {
	var ctl protocol.MeetingControlPayloadV2
	if err := json.Unmarshal([]byte(normalizeWebhookMessage(webhook.Message)), &ctl); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid meeting.control payload"))
		return
	}

	switch ctl.Action {
	case "start":
		_ = h.store.UpdateMeetingStatus(store.SystemScope(), ctl.MeetingID, model.MeetingInProgress)
	case "conclude":
		_ = h.store.UpdateMeetingStatus(store.SystemScope(), ctl.MeetingID, model.MeetingCompleted)
	case "minutes":
		markdown := ctl.Content
		if ctl.MinutesData != nil && ctl.MinutesData.FullMarkdown != "" {
			markdown = ctl.MinutesData.FullMarkdown
		}
		h.saveMeetingMinutes(c, ctl.MeetingID, markdown)
	case "ping":
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "pong": true})
		return
	case "status":
		meeting, _ := h.store.GetMeeting(store.SystemScope(), ctl.MeetingID)
		if meeting != nil {
			transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "meeting_status": meeting.Status, "participant_count": len(meeting.Participants)})
		} else {
			transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "meeting_status": "not_found"})
		}
		return
	default:
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "unknown meeting.control action: "+ctl.Action))
		return
	}

	transport.WriteData(c, http.StatusOK, gin.H{"status": "ok"})
}

// logDeprecatedMeetingRoute logs a warning when old protocol types are used
// for meeting communication, hinting that the agent should upgrade.
func (h *WebhookHandler) logDeprecatedMeetingRoute(oldType string, payload protocol.WebhookPayload) {
	if h.log != nil {
		h.log.Warn("deprecated meeting route used",
			zap.String("old_type", oldType),
			zap.String("from", payload.From),
			zap.String("session_key", payload.SessionKey),
			zap.String("hint", "upgrade agent skill to use meeting.chat or meeting.control"),
		)
	}
}

// broadcastMeetingReplyUnified routes an agent's meeting response using the
// new meeting protocol types:
//   - Host → participant: meeting.instruction (to @mentioned / targeted agents)
//   - Participant → host: meeting.chat
//
// Routing priority (Host → participant):
//  1. If payload.Target is set, resolve it to node ID(s):
//     - "all"  → every participant node
//     - "host" → the PM node
//     - agent_id / node_id → exactly that participant node (structured routing)
//  2. Otherwise fall back to parsing @mentions from the content (legacy).
//
// Participant → host always routes only to the PM node.
func (h *WebhookHandler) broadcastMeetingReplyUnified(ctx context.Context, meeting *model.Meeting, fromNodeID string, msg protocol.MeetingChatPayload, content string) {
	if h.client == nil {
		return
	}

	var pmNodeID string
	if meeting.HostAgentID != "" {
		if agent, ok := h.store.GetAgentByIDUnsafe(meeting.HostAgentID); ok {
			pmNodeID = agent.NodeID
		}
	}

	nameToNode := make(map[string]string)
	agentIDToNode := make(map[string]string)
	for _, p := range meeting.Participants {
		if agent, ok := h.store.GetAgentByIDUnsafe(p.AgentID); ok && agent.NodeID != "" {
			nameToNode[agent.Name] = agent.NodeID
			agentIDToNode[p.AgentID] = agent.NodeID
			agentIDToNode[agent.NodeID] = agent.NodeID // allow node_id as target too
		}
	}

	var targetIDs []string
	var messageType string

	if fromNodeID == pmNodeID {
		// Host → participant
		target := strings.TrimSpace(msg.Target)
		switch {
		case target == "":
			// Legacy fallback: route to @mentioned agents in the content.
			for _, nodeID := range parseAgentMentions(content, nameToNode) {
				if nodeID != "" && nodeID != pmNodeID {
					targetIDs = append(targetIDs, nodeID)
				}
			}
		case target == "all":
			for _, nodeID := range agentIDToNode {
				if nodeID != pmNodeID {
					targetIDs = append(targetIDs, nodeID)
				}
			}
			// de-dup
			seen := make(map[string]bool)
			uniq := targetIDs[:0]
			for _, id := range targetIDs {
				if !seen[id] {
					seen[id] = true
					uniq = append(uniq, id)
				}
			}
			targetIDs = uniq
		case target == "host":
			if pmNodeID != "" {
				targetIDs = append(targetIDs, pmNodeID)
			}
		default:
			// Resolve agent_id or node_id to a single participant node.
			if nodeID, ok := agentIDToNode[target]; ok && nodeID != pmNodeID {
				targetIDs = append(targetIDs, nodeID)
			}
		}
		if len(targetIDs) == 0 {
			// The host's target could not be resolved to a participant (e.g. it
			// mistakenly pointed at itself, used an unknown id, or sent an empty
			// target with no @mentions). Silently dropping the message would
			// deadlock the meeting — the intended participant never gets pinged
			// and the host waits forever. Fall back to broadcasting to all
			// participants instead, and log a warning for diagnosis.
			if h.log != nil {
				h.log.Warn("meeting reply target unresolved; falling back to all participants",
					zap.String("meeting_id", meeting.ID),
					zap.String("from", fromNodeID),
					zap.String("target", strings.TrimSpace(msg.Target)))
			}
			for _, nodeID := range agentIDToNode {
				if nodeID != pmNodeID {
					targetIDs = append(targetIDs, nodeID)
				}
			}
			// de-dup
			seen := make(map[string]bool)
			uniq := targetIDs[:0]
			for _, id := range targetIDs {
				if !seen[id] {
					seen[id] = true
					uniq = append(uniq, id)
				}
			}
			targetIDs = uniq
		}
		if len(targetIDs) == 0 {
			// Genuinely no participants to route to.
			return
		}
		messageType = "meeting.chat" // executors receive as meeting chat
	} else {
		// Participant → host
		if pmNodeID != "" {
			targetIDs = append(targetIDs, pmNodeID)
		}
		if len(targetIDs) == 0 {
			return
		}
		messageType = "meeting.chat" // host also receives as meeting chat
	}

	// Hard guard (issue #2): a meeting broadcast must NEVER reach an agent that
	// is not an actual meeting participant (or the host). Drop any resolved
	// target node that is outside the participant set so a misrouted/forged
	// target can never leak the message to an unrelated agent.
	allowedNodes := make(map[string]bool, len(agentIDToNode)+1)
	for _, n := range agentIDToNode {
		allowedNodes[n] = true
	}
	if pmNodeID != "" {
		allowedNodes[pmNodeID] = true
	}
	guarded := targetIDs[:0]
	for _, id := range targetIDs {
		if allowedNodes[id] {
			guarded = append(guarded, id)
		} else if h.log != nil {
			h.log.Warn("dropping meeting broadcast target outside participants",
				zap.String("meeting_id", meeting.ID),
				zap.String("node", id))
		}
	}
	targetIDs = guarded
	if len(targetIDs) == 0 {
		return // no legitimate recipient
	}

	// Build reference file download URLs
	refFiles := meeting.AttachedFiles
	if len(refFiles) == 0 {
		for _, fid := range meeting.FileIDs {
			f, _ := h.store.GetProjectFileByID(fid)
			if f != nil {
				refFiles = append(refFiles, model.MeetingAttachedFile{
					ID: f.ID, FileName: f.FileName, FileSize: f.FileSize, MimeType: f.MimeType, Source: f.Source,
				})
			}
		}
	}
	var fileSection string
	if len(refFiles) > 0 && h.externalURL != "" && len(h.jwtSecret) > 0 {
		var refs []string
		for _, f := range refFiles {
			token, err := agentfile.GenerateDownloadToken(h.jwtSecret, f.ID, h.downloadTTL)
			if err == nil {
				url := agentfile.BuildDownloadURL(h.externalURL, f.ID, token)
				name := f.FileName
				if name == "" {
					name = f.ID
				}
				refs = append(refs, fmt.Sprintf("- **%s**: `%s`", name, url))
			}
		}
		if len(refs) > 0 {
			fileSection = "\n\n---\n**参考文件下载链接：**\n" + strings.Join(refs, "\n")
		}
	}

	role := "host"
	var skillHint string
	if fromNodeID != pmNodeID {
		role = "participant"
	} else {
		// When host sends to participants, include a skill hint so the
		// executor knows to load /tm-meeting-participant for meeting.chat.
		skillHint = "\n\n> ℹ️ 请加载 `/tm-meeting-participant` 技能处理本消息。"
	}

	for _, nodeID := range targetIDs {
		out := protocol.MeetingChatPayload{
			MeetingID:    meeting.ID,
			Role:         role,
			Phase:        msg.Phase,
			Target:       nodeID, // echo the resolved recipient so the participant can verify it is addressed
			ContextBrief: msg.ContextBrief,
			Content:      content + skillHint + fileSection,
		}
		_, _ = h.client.Publish(ctx, nodeID, messageType, out, meeting.ID, nil)
	}
}

// handleMeetingChat processes a chat.message that is scoped to a meeting
// (metadata context=meeting / meeting_id). It replaces the old meeting.message
// and meeting.control types so the whole meeting flow rides the chat channel.
func (h *WebhookHandler) handleMeetingChat(c *gin.Context, webhook protocol.WebhookPayload) {
	meetingID := meetingIDFromMetadata(webhook.Metadata)
	if meetingID == "" {
		meetingID = webhook.SessionKey
	}

	// Control action (start/conclude/summarize/minutes) takes precedence over transcript.
	if action, ok := webhook.Metadata["action"].(string); ok && action != "" {
		content := webhook.Message
		// Minutes may be sent as JSON {content: "..."} (legacy task.reply shape).
		var pm protocol.TaskReplyPayload
		if err := decodeWebhookMessage(webhook.Message, &pm); err == nil {
			if strings.TrimSpace(pm.Content) != "" {
				content = strings.TrimSpace(pm.Content)
			}
		}
		h.applyMeetingControl(c, meetingID, action, content)
		return
	}

	content := strings.TrimSpace(webhook.Message)
	var uiBlocks []model.UIBlock
	// Meeting chat messages may carry JSON {task_id, content, ui_blocks} (the
	// legacy task.reply shape used for minutes + confirm button) or plain text.
	// Support both so the agent skill only needs to switch the message type.
	var pm protocol.TaskReplyPayload
	if err := decodeWebhookMessage(webhook.Message, &pm); err == nil {
		if strings.TrimSpace(pm.Content) != "" {
			content = strings.TrimSpace(pm.Content)
		}
		uiBlocks = pm.UIBlocks
	} else {
		// Fallback: when JSON decoding fails (PM agent sends malformed
		// non-JSON format like {task_id: <id>, content: <text>} instead of
		// {"task_id":"<id>","content":"<text>"}), extract the content value
		// via simple key search.
		if extracted := extractContentFromTaskWrapper(content); extracted != "" {
			content = extracted
		}
	}
	// Strip agent-runtime noise (ACK/WAITING/English monologue leakage) but
	// KEEP any genuine reply content. A message that is pure noise collapses to
	// empty and is silently ignored; a real reply with a trailing "ACK
	// chat.message" keeps its body.
	cleaned := cleanConversationNoise(content)
	if cleaned == "" {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}
	content = cleaned
	if content == "" {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ignored", "reason": "empty meeting message"})
		return
	}

	// PROBLEM 2 (defensive) — for a chat.message scoped to a meeting, drop it
	// only when the agent explicitly sends an empty "phase" field. Legacy
	// chat.message payloads carry no phase field at all, so genuine speech
	// (which has no phase field) is preserved; only noise that opts into the
	// phase protocol but omits the value is dropped. User messages never arrive
	// on this webhook path (they go through the REST SendMessage handler), so
	// no user-exemption lookup is needed here.
	if phase, has := phaseFromRaw(webhook.Message); has && phase == "" {
		if !meetingMsgExemptFromNoPhase("", webhook.From, "") {
			if h.log != nil {
				h.log.Debug("dropping meeting chat.message without phase",
					zap.String("meeting_id", meetingID),
					zap.String("from", webhook.From),
					zap.String("reason", "no_phase"))
			}
			transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true, "reason": "no_phase"})
			return
		}
	}

	storeMsg := &model.MeetingMessage{
		MeetingID:  meetingID,
		SenderType: "agent",
		SenderID:   webhook.From,
		SenderName: h.resolveMeetingSenderName(webhook.From),
		Content:    content,
		UIBlocks:   uiBlocks,
	}
	if h.isMeetingCompleted(meetingID) {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ignored", "reason": "meeting completed"})
		return
	}
	if _, appErr := h.store.AddMeetingMessage(store.SystemScope(), storeMsg); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	_ = h.store.UpdateMeetingStatus(store.SystemScope(), meetingID, model.MeetingInProgress)
	meeting, _ := h.store.GetMeeting(store.SystemScope(), meetingID)
	if meeting != nil {
		h.broadcastMeetingReply(context.Background(), meeting, webhook.From, content)
	}

	// Reset the meeting inactivity timeout watchdog.
	if h.onMeetingActivity != nil {
		h.onMeetingActivity(meetingID)
	}

	transport.WriteData(c, http.StatusOK, gin.H{"status": "ok"})
}

// applyMeetingControl applies a meeting control action (start/conclude/summarize/minutes).
func (h *WebhookHandler) applyMeetingControl(c *gin.Context, meetingID, action, content string) {
	switch action {
	case "start":
		_ = h.store.UpdateMeetingStatus(store.SystemScope(), meetingID, model.MeetingInProgress)
	case "conclude", "summarize":
		_ = h.store.UpdateMeetingStatus(store.SystemScope(), meetingID, model.MeetingCompleted)
	case "minutes":
		h.saveMeetingMinutes(c, meetingID, content)
	}
	transport.WriteData(c, http.StatusOK, gin.H{"status": "ok"})
}

// saveMeetingMinutes persists the host-generated meeting minutes (markdown) as a
// project file (Source = "meeting_minutes") and links it to the meeting record.
// The host (PM agent) sends the minutes via a chat.message with
// metadata.action = "minutes" and the full markdown as the message body.
func (h *WebhookHandler) saveMeetingMinutes(c *gin.Context, meetingID, markdown string) {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "meeting minutes content is empty"))
		return
	}
	meeting, appErr := h.store.GetMeeting(store.SystemScope(), meetingID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if h.projectFileStorage == nil {
		transport.WriteError(c, transport.NewError(http.StatusInternalServerError, "STORAGE_UNAVAILABLE", "project file storage not configured"))
		return
	}

	fileName := buildMinutesFileName(meeting.Title, meetingID)
	pf, appErr := h.store.CreateMeetingMinutesFile(meetingID, fileName, markdown)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	localPath, err := h.projectFileStorage.Save(c.Request.Context(), meeting.ProjectID, pf.ID, fileName, strings.NewReader(markdown))
	if err != nil {
		if h.log != nil {
			h.log.Warn("failed to save meeting minutes file", zap.String("meeting_id", meetingID), zap.Error(err))
		}
		transport.WriteError(c, transport.NewError(http.StatusInternalServerError, "MINUTES_SAVE_FAILED", "failed to store meeting minutes"))
		return
	}
	_ = h.store.SetProjectFileLocalPath(pf.ID, localPath)
	_ = h.store.UpdateMeetingMinutes(store.SystemScope(), meetingID, markdown, pf.ID)

	h.appendSystemMeetingMessage(meeting, fmt.Sprintf("📄 会议纪要已生成并上传至项目文件管理：%s", fileName))
	transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "file_id": pf.ID})
}

// appendSystemMeetingMessage records a system note in the meeting transcript so
// the user can see it in the meeting UI. It is intentionally NOT forwarded to
// agent nodes (unlike broadcastMeetingReply) to avoid re-triggering agents.
func (h *WebhookHandler) appendSystemMeetingMessage(meeting *model.Meeting, text string) {
	msg := &model.MeetingMessage{
		MeetingID:  meeting.ID,
		SenderType: "system",
		SenderID:   "system",
		SenderName: "系统",
		Content:    text,
	}
	if _, appErr := h.store.AddMeetingMessage(store.SystemScope(), msg); appErr != nil && h.log != nil {
		h.log.Warn("failed to append system meeting message", zap.String("meeting_id", meeting.ID), zap.Error(appErr))
	}
}

// buildMinutesFileName produces a safe file name for the meeting minutes.
func buildMinutesFileName(title, meetingID string) string {
	base := strings.TrimSpace(title)
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, "\\", "_")
	base = strings.ReplaceAll(base, ":", "_")
	base = strings.ReplaceAll(base, "*", "_")
	base = strings.ReplaceAll(base, "?", "_")
	base = strings.ReplaceAll(base, "\"", "_")
	base = strings.ReplaceAll(base, "<", "_")
	base = strings.ReplaceAll(base, ">", "_")
	base = strings.ReplaceAll(base, "|", "_")
	if base == "" {
		base = "会议纪要"
	}
	return fmt.Sprintf("%s_会议纪要.md", base)
}

// handleMeetingControl processes meeting.control sent by the PM agent (host).
func (h *WebhookHandler) handleMeetingControl(c *gin.Context, payload protocol.WebhookPayload) {
	var ctl protocol.MeetingControlPayload
	if err := json.Unmarshal([]byte(payload.Message), &ctl); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid meeting.control payload"))
		return
	}
	// Backward-compatible alias for the chat-based control path.
	h.applyMeetingControl(c, ctl.MeetingID, ctl.Action, ctl.Content)
}
