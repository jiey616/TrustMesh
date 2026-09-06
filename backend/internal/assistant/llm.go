package assistant

import (
	"context"
	"errors"
	"io"

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
