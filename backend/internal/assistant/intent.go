package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// AgentCandidate is a simplified agent descriptor passed to the intent parser.
type AgentCandidate struct {
	ID   string
	Name string
}

// TaskIntentResult is the structured output from ParseTaskIntent.
// Mode mirrors the task workspace modes: "planning" or "building".
type TaskIntentResult struct {
	// Mode is "planning" (needs PM 数字员工 to plan) or "building" (direct assignment).
	Mode        string `json:"mode"`
	AgentID     string `json:"agent_id"`    // non-empty only when Mode == "building"
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"` // low | medium | high | urgent
}

// ParseTaskIntent sends userInput to the LLM and returns a structured TaskIntentResult.
// agents should contain only executor agents (not PM agents).
// Returns an error only on network/parse failure; the caller should fall back to planning mode.
func (c *LLMClient) ParseTaskIntent(ctx context.Context, userInput string, agents []AgentCandidate) (*TaskIntentResult, error) {
	systemPrompt := buildIntentSystemPrompt(agents)

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: userInput},
	}

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: messages,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("intent parse llm call failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("intent parse llm returned empty response")
	}

	var result TaskIntentResult
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
		return nil, fmt.Errorf("intent parse response unmarshal: %w", err)
	}

	return normalizeIntentResult(&result, userInput, agents), nil
}

func buildIntentSystemPrompt(agents []AgentCandidate) string {
	var agentList string
	if len(agents) == 0 {
		agentList = "（当前无可用执行 数字员工）"
	} else {
		var sb strings.Builder
		for _, a := range agents {
			fmt.Fprintf(&sb, "- %s (id: %s)\n", a.Name, a.ID)
		}
		agentList = strings.TrimRight(sb.String(), "\n")
	}

	return fmt.Sprintf(`你是任务意图解析器。分析用户输入，提取任务信息并判断任务模式。

可用执行 数字员工：
%s

判断规则：
1. 如果用户明确提及了某个 数字员工 的名字（如"让 Alice 做"、"@Bob"、"交给 Carol 处理"等），则 mode=building，填写对应 agent_id
2. 否则 mode=planning（由 PM 数字员工 负责规划和拆解）
3. title：提炼简洁的任务标题，不超过 20 字
4. description：保留用户输入的完整意图
5. priority：含"紧急/ASAP/尽快/马上" → urgent；含"重要/高优/优先" → high；否则默认 medium

以 JSON 格式返回，不要输出任何其他内容：
{"mode":"planning","agent_id":"","title":"...","description":"...","priority":"medium"}`, agentList)
}

func normalizeIntentResult(r *TaskIntentResult, rawInput string, agents []AgentCandidate) *TaskIntentResult {
	if r.Mode != "building" {
		r.Mode = "planning"
		r.AgentID = ""
	}

	// Verify agent_id actually exists in the candidates list
	if r.Mode == "building" {
		found := false
		for _, a := range agents {
			if a.ID == r.AgentID {
				found = true
				break
			}
		}
		if !found {
			r.Mode = "planning"
			r.AgentID = ""
		}
	}

	if !isValidTaskPriority(r.Priority) {
		r.Priority = "medium"
	}
	if strings.TrimSpace(r.Title) == "" {
		r.Title = truncateRunes(rawInput, 20)
	}
	if strings.TrimSpace(r.Description) == "" {
		r.Description = rawInput
	}

	return r
}

func isValidTaskPriority(p string) bool {
	switch p {
	case "low", "medium", "high", "urgent":
		return true
	}
	return false
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
