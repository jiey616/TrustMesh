package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	openai "github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
	"trustmesh/backend/internal/assistant"
	"trustmesh/backend/internal/transport"
)

type AssistantHandler struct {
	llm   *assistant.LLMProvider
	tools *assistant.ToolExecutor
	defs  []openai.Tool
	log   *zap.Logger
}

func NewAssistantHandler(
	llm *assistant.LLMProvider,
	tools *assistant.ToolExecutor,
	hasKnowledge bool,
	log *zap.Logger,
) *AssistantHandler {
	return &AssistantHandler{
		llm:   llm,
		tools: tools,
		defs:  assistant.ToolDefinitions(hasKnowledge),
		log:   log,
	}
}

func (h *AssistantHandler) Chat(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var req assistant.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	if req.Message == "" {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "message is required"))
		return
	}

	// Build OpenAI messages
	messages := assistant.BuildMessages(&req)

	// Set up SSE
	beginSSE(c)
	w := &ginSSEWriter{c: c}

	// Resolve per-tenant LLM client (B2/C1): nil = 当前租户/平台/env 均未配置
	client := h.llm.ClientFor(sc.OrgID)
	if client == nil {
		w.WriteEvent("error", map[string]string{"message": "LLM 未配置：请在 设置 → LLM 配置 中完成平台或租户配置"})
		w.WriteEvent("done", map[string]any{})
		return
	}

	// Run agent loop
	if err := client.RunAgentLoop(c.Request.Context(), messages, h.defs, h.tools, sc, w); err != nil {
		h.log.Error("assistant agent loop failed", zap.Error(err))
		w.WriteEvent("error", map[string]string{"message": err.Error()})
	}
	w.WriteEvent("done", map[string]any{})
}

// ginSSEWriter implements assistant.SSEWriter using Gin's SSE support.
type ginSSEWriter struct {
	c *gin.Context
}

func (w *ginSSEWriter) WriteEvent(event string, data any) {
	// For navigate events from tool results, emit as a separate navigate SSE event
	if event == "tool_result" {
		if tr, ok := data.(assistant.ToolResultEvent); ok {
			if nav, ok := tr.Result.(assistant.NavigateEvent); ok {
				w.c.SSEvent("navigate", nav)
				if f, ok := w.c.Writer.(http.Flusher); ok {
					f.Flush()
				}
				return
			}
		}
	}

	w.c.SSEvent(event, data)
	if f, ok := w.c.Writer.(http.Flusher); ok {
		f.Flush()
	}
}
