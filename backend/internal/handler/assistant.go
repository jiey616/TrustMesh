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
	llm   *assistant.LLMClient
	tools *assistant.ToolExecutor
	defs  []openai.Tool
	log   *zap.Logger
}

func NewAssistantHandler(
	llm *assistant.LLMClient,
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

	// Run agent loop
	if err := h.llm.RunAgentLoop(c.Request.Context(), messages, h.defs, h.tools, sc, w); err != nil {
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
