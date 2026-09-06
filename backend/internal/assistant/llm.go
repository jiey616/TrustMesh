package assistant

import (
	"context"
	"errors"
	"io"
	"sync"

	openai "github.com/sashabaranov/go-openai"
	"trustmesh/backend/internal/store"
)

const maxToolRounds = 3

// Complete 非流式单轮补全：运维归因等后台场景用（不需要 SSE，也不带工具）。
func (c *LLMClient) Complete(ctx context.Context, system, user string) (string, error) {
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: system},
			{Role: openai.ChatMessageRoleUser, Content: user},
		},
		Temperature: 0.2,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("empty completion response")
	}
	return resp.Choices[0].Message.Content, nil
}

// LLMClient wraps the OpenAI-compatible API for chat completions.
type LLMClient struct {
	client *openai.Client
	model  string
}

// NewLLMClient creates a client that talks to any OpenAI-compatible endpoint.
func NewLLMClient(apiURL, apiKey, model string) *LLMClient {
	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = apiURL
	return &LLMClient{
		client: openai.NewClientWithConfig(cfg),
		model:  model,
	}
}

// RunAgentLoop executes the tool-call loop:
//  1. Send messages + tools to LLM via streaming.
//  2. If LLM returns tool_calls → execute each tool → append tool messages → repeat.
//  3. When LLM returns pure text (no tool_calls) → stream tokens to frontend in real-time.
func (c *LLMClient) RunAgentLoop(
	ctx context.Context,
	messages []openai.ChatCompletionMessage,
	tools []openai.Tool,
	executor *ToolExecutor,
	sc store.Scope,
	w SSEWriter,
) error {
	for round := 0; round < maxToolRounds; round++ {
		content, toolCalls, err := c.streamRound(ctx, messages, tools, w)
		if err != nil {
			return err
		}

		// No tool calls → final answer already streamed
		if len(toolCalls) == 0 {
			return nil
		}

		// Append assistant message with tool calls
		assistantMsg := openai.ChatCompletionMessage{
			Role:      openai.ChatMessageRoleAssistant,
			Content:   content,
			ToolCalls: toolCalls,
		}
		messages = append(messages, assistantMsg)

		// Execute each tool call
		for _, tc := range toolCalls {
			w.WriteEvent("tool_call", ToolCallEvent{
				Tool: tc.Function.Name,
				Args: tc.Function.Arguments,
			})

			result, execErr := executor.Execute(ctx, sc, tc.Function.Name, tc.Function.Arguments)
			if execErr != nil {
				result = map[string]any{"error": execErr.Error()}
			}

			w.WriteEvent("tool_result", ToolResultEvent{
				Tool:   tc.Function.Name,
				Result: result,
			})

			resultJSON, _ := marshalJSON(result)
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    resultJSON,
				ToolCallID: tc.ID,
			})
		}
	}

	// After max rounds, do a final streaming call without tools
	return c.streamFinalResponse(ctx, messages, w)
}

// streamRound makes a single streaming call. It streams text deltas to the
// frontend in real-time and accumulates any tool calls. Returns the full
// text content and collected tool calls when the stream ends.
func (c *LLMClient) streamRound(
	ctx context.Context,
	messages []openai.ChatCompletionMessage,
	tools []openai.Tool,
	w SSEWriter,
) (string, []openai.ToolCall, error) {
	stream, err := c.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: messages,
		Tools:    tools,
	})
	if err != nil {
		return "", nil, err
	}
	defer stream.Close()

	var content string
	toolCallMap := map[int]*openai.ToolCall{}

	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, err
		}
		if len(resp.Choices) == 0 {
			continue
		}

		delta := resp.Choices[0].Delta

		// Stream text deltas to frontend immediately
		if delta.Content != "" {
			content += delta.Content
			w.WriteEvent("delta", DeltaEvent{Content: delta.Content})
		}

		// Accumulate tool call chunks
		for _, tc := range delta.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			existing, ok := toolCallMap[idx]
			if !ok {
				toolCallMap[idx] = &openai.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: openai.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			} else {
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Function.Name != "" {
					existing.Function.Name = tc.Function.Name
				}
				existing.Function.Arguments += tc.Function.Arguments
			}
		}
	}

	// Convert map to slice ordered by index
	var toolCalls []openai.ToolCall
	for i := 0; i < len(toolCallMap); i++ {
		if tc, ok := toolCallMap[i]; ok {
			toolCalls = append(toolCalls, *tc)
		}
	}

	return content, toolCalls, nil
}

// streamFinalResponse streams the last LLM response token by token.
func (c *LLMClient) streamFinalResponse(
	ctx context.Context,
	messages []openai.ChatCompletionMessage,
	w SSEWriter,
) error {
	stream, err := c.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: messages,
	})
	if err != nil {
		return err
	}
	defer stream.Close()

	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if len(resp.Choices) > 0 && resp.Choices[0].Delta.Content != "" {
			w.WriteEvent("delta", DeltaEvent{Content: resp.Choices[0].Delta.Content})
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// LLMProvider（A1+B2+C1）：按租户解析 LLM 配置并缓存客户端。
// lookup 由 app 层注入（闭包 store.ResolveLLMParams），每次调用实时解析——
// 配置保存即生效，无需重启；按配置指纹缓存 client，避免重复建连。
// store 不能 import assistant（反向依赖），所以解析闭包在这里组装。
// ─────────────────────────────────────────────────────────────────────────────

// LLMParams 是一次解析得到的 LLM 连接参数。
type LLMParams struct {
	APIURL   string
	APIKey   string
	Model    string // 对话模型
	OpsModel string // 归因模型（解析层已兜底 = Model）
	Source   string // org | platform | env
}

type LLMProvider struct {
	lookup func(orgID string) (LLMParams, bool)
	mu     sync.Mutex
	cache  map[string]*LLMClient // 指纹 → client
}

func NewLLMProvider(lookup func(orgID string) (LLMParams, bool)) *LLMProvider {
	return &LLMProvider{lookup: lookup, cache: make(map[string]*LLMClient)}
}

// ParamsFor 暴露解析结果（归因器需要 OpsModel 维度）。
func (p *LLMProvider) ParamsFor(orgID string) (LLMParams, bool) {
	if p == nil || p.lookup == nil {
		return LLMParams{}, false
	}
	return p.lookup(orgID)
}

func (p *LLMProvider) clientForParams(params LLMParams, model string) *LLMClient {
	if params.APIKey == "" || params.APIURL == "" || model == "" {
		return nil
	}
	fp := params.APIURL + "|" + params.APIKey + "|" + model
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.cache[fp]; ok {
		return c
	}
	c := NewLLMClient(params.APIURL, params.APIKey, model)
	p.cache[fp] = c
	return c
}

// ClientFor 对话客户端（chat 模型维度）。无可用配置返回 nil。
func (p *LLMProvider) ClientFor(orgID string) *LLMClient {
	params, ok := p.ParamsFor(orgID)
	if !ok {
		return nil
	}
	return p.clientForParams(params, params.Model)
}

// AttributionClientFor 归因客户端（ops_model 维度，解析层兜底为 chat 模型）。
func (p *LLMProvider) AttributionClientFor(orgID string) *LLMClient {
	params, ok := p.ParamsFor(orgID)
	if !ok {
		return nil
	}
	return p.clientForParams(params, params.OpsModel)
}
