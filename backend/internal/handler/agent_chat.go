package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"trustmesh/backend/internal/clawsynapse"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

type AgentChatHandler struct {
	store       *store.Store
	publisher   *clawsynapse.Client
	externalURL string
	jwtSecret   []byte
	downloadTTL time.Duration
	log         *zap.Logger
}

func NewAgentChatHandler(s *store.Store, publisher *clawsynapse.Client, externalURL string, jwtSecret []byte, downloadTTL time.Duration, log *zap.Logger) *AgentChatHandler {
	return &AgentChatHandler{
		store:       s,
		publisher:   publisher,
		externalURL: externalURL,
		jwtSecret:   jwtSecret,
		downloadTTL: downloadTTL,
		log:         log,
	}
}

type sendAgentChatMessageRequest struct {
	Content     string                 `json:"content"`
	Attachments []model.ChatAttachment `json:"attachments,omitempty"`
}

func (h *AgentChatHandler) Get(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	detail, appErr := h.store.GetActiveAgentChat(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	EnrichAgentChatDetailURLs(detail, h.externalURL, h.jwtSecret, h.downloadTTL)
	transport.WriteData(c, http.StatusOK, detail)
}

func (h *AgentChatHandler) ListSessions(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	sessions, appErr := h.store.ListAgentChatSessions(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, sessions)
}

func (h *AgentChatHandler) GetSession(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	detail, appErr := h.store.GetAgentChatByID(sc, c.Param("id"), c.Param("sessionId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	EnrichAgentChatDetailURLs(detail, h.externalURL, h.jwtSecret, h.downloadTTL)
	transport.WriteData(c, http.StatusOK, detail)
}

func (h *AgentChatHandler) SendMessage(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	var req sendAgentChatMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}

	detail, msg, appErr := h.store.AppendAgentChatUserMessage(sc, c.Param("id"), req.Content, req.Attachments)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	// Enrich attachment download URLs so the UI can render them immediately.
	EnrichAgentChatDetailURLs(detail, h.externalURL, h.jwtSecret, h.downloadTTL)
	if h.publisher == nil {
		updated, markErr := h.store.UpdateAgentChatMessageStatus(sc, detail.ID, msg.ID, "failed", "")
		if markErr == nil {
			detail = updated
		}
		transport.WriteError(c, transport.NewError(http.StatusServiceUnavailable, "CLAWSYNAPSE_UNAVAILABLE", "暂时无法发送消息到远程数字员工"))
		return
	}

	payloadContent := "[使用 clawsynapse skill 回复以下消息]\n" + req.Content
	// Signed attachment URLs for the agent to fetch (only sent in metadata, never
	// persisted); the content body stays unchanged to avoid altering agent parsing.
	payloadAttachments := make([]model.ChatAttachment, len(req.Attachments))
	copy(payloadAttachments, req.Attachments)
	enrichChatAttachmentURLs(payloadAttachments, h.externalURL, h.jwtSecret, h.downloadTTL)
	result, err := h.publisher.Publish(context.Background(), detail.AgentNodeID, "chat.message", payloadContent, detail.SessionKey, map[string]any{
		"trustmeshAgentId": detail.AgentID,
		"chatId":           detail.ID,
		"messageId":        msg.ID,
		"attachments":      payloadAttachments,
	})
	if err != nil {
		updated, markErr := h.store.UpdateAgentChatMessageStatus(sc, detail.ID, msg.ID, "failed", "")
		if markErr == nil {
			detail = updated
		}
		if h.log != nil {
			h.log.Warn("publish agent chat.message failed", zap.String("agent_id", detail.AgentID), zap.String("chat_id", detail.ID), zap.Error(err))
		}
		appErr = transport.NewError(http.StatusBadGateway, "CHAT_DELIVERY_FAILED", "消息发送失败")
		appErr.Details = map[string]any{"cause": err.Error()}
		transport.WriteError(c, appErr)
		return
	}

	currentDetail := detail
	detail, appErr = h.store.UpdateAgentChatMessageStatus(sc, detail.ID, msg.ID, "sent", result.MessageID)
	if appErr != nil {
		detail = currentDetail
		if h.log != nil {
			h.log.Warn("agent chat delivery confirmed but local status update failed",
				zap.String("agent_id", detail.AgentID),
				zap.String("chat_id", detail.ID),
				zap.String("message_id", msg.ID),
				zap.String("remote_message_id", result.MessageID),
				zap.Error(appErr),
			)
		}
		fallback, getErr := h.store.GetActiveAgentChat(sc, c.Param("id"))
		if getErr == nil && fallback != nil {
			detail = fallback
		}
		for i := range detail.Messages {
			if detail.Messages[i].ID != msg.ID {
				continue
			}
			detail.Messages[i].Status = "sent"
			detail.Messages[i].RemoteMessageID = result.MessageID
			break
		}
		EnrichAgentChatDetailURLs(detail, h.externalURL, h.jwtSecret, h.downloadTTL)
		transport.WriteData(c, http.StatusOK, detail)
		return
	}

	EnrichAgentChatDetailURLs(detail, h.externalURL, h.jwtSecret, h.downloadTTL)
	transport.WriteData(c, http.StatusOK, detail)
}

func (h *AgentChatHandler) Reset(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	if appErr := h.store.ResetAgentChat(sc, c.Param("id")); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, nil)
}
