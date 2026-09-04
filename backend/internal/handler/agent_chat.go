package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"trustmesh/backend/internal/clawsynapse"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

type AgentChatHandler struct {
	store     *store.Store
	publisher *clawsynapse.Client
	log       *zap.Logger
}

func NewAgentChatHandler(s *store.Store, publisher *clawsynapse.Client, log *zap.Logger) *AgentChatHandler {
	return &AgentChatHandler{store: s, publisher: publisher, log: log}
}

type sendAgentChatMessageRequest struct {
	Content string `json:"content"`
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

	detail, msg, appErr := h.store.AppendAgentChatUserMessage(sc, c.Param("id"), req.Content)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if h.publisher == nil {
		updated, markErr := h.store.UpdateAgentChatMessageStatus(sc, detail.ID, msg.ID, "failed", "")
		if markErr == nil {
			detail = updated
		}
		transport.WriteError(c, transport.NewError(http.StatusServiceUnavailable, "CLAWSYNAPSE_UNAVAILABLE", "暂时无法发送消息到远程数字员工"))
		return
	}

	payloadContent := "[使用 clawsynapse skill 回复以下消息]\n" + req.Content
	result, err := h.publisher.Publish(context.Background(), detail.AgentNodeID, "chat.message", payloadContent, detail.SessionKey, map[string]any{
		"trustmeshAgentId": detail.AgentID,
		"chatId":           detail.ID,
		"messageId":        msg.ID,
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
		transport.WriteData(c, http.StatusOK, detail)
		return
	}

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
