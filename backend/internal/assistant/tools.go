package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
	"trustmesh/backend/internal/embedding"
	"trustmesh/backend/internal/knowledge"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
)

// validRoutes defines all valid frontend routes.
// Static routes are matched exactly; dynamic routes use regex patterns.
var validRoutes = []routeEntry{
	{pattern: "/dashboard", label: "仪表盘"},
	{pattern: "/inbox", label: "收件箱"},
	{pattern: "/projects", label: "项目列表"},
	{pattern: "/projects/[a-f0-9]+", label: "项目详情", dynamic: true},
	{pattern: "/agents", label: "数字员工 管理"},
	{pattern: "/agents/[a-f0-9]+", label: "数字员工 详情", dynamic: true},
	{pattern: "/knowledge", label: "知识库"},
}

type routeEntry struct {
	pattern string
	label   string
	dynamic bool
}

// isValidRoute checks if a given path matches any known frontend route.
func isValidRoute(path string) bool {
	for _, r := range validRoutes {
		if r.dynamic {
			if matched, _ := regexp.MatchString("^"+r.pattern+"$", path); matched {
				return true
			}
		} else {
			if path == r.pattern {
				return true
			}
		}
	}
	return false
}

// routeDescriptionForLLM builds a description string listing all available routes.
func routeDescriptionForLLM() string {
	var parts []string
	for _, r := range validRoutes {
		example := r.pattern
		if r.dynamic {
			// Show a human-friendly placeholder
			example = regexp.MustCompile(`\[a-f0-9\]\+`).ReplaceAllString(r.pattern, "{id}")
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", example, r.label))
	}
	return "可用路由: " + strings.Join(parts, ", ")
}

// ToolExecutor executes tool calls using the application's Store and services.
type ToolExecutor struct {
	store    *store.Store
	embedder embedding.Client
	qdrant   *knowledge.QdrantClient
}

func NewToolExecutor(s *store.Store, embedder embedding.Client, qdrant *knowledge.QdrantClient) *ToolExecutor {
	return &ToolExecutor{store: s, embedder: embedder, qdrant: qdrant}
}

// ToolDefinitions returns the OpenAI function tool definitions for the LLM.
func ToolDefinitions(hasKnowledge bool) []openai.Tool {
	tools := []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "search_tasks",
				Description: "搜索用户的任务。可以按状态、项目、关键词过滤。",
				Parameters: &jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"query": {
							Type:        jsonschema.String,
							Description: "搜索关键词，匹配任务标题和描述",
						},
						"status": {
							Type:        jsonschema.String,
							Description: "任务状态过滤: pending, in_progress, done, failed",
							Enum:        []string{"pending", "in_progress", "done", "failed"},
						},
						"project_id": {
							Type:        jsonschema.String,
							Description: "限定在某个项目内搜索",
						},
					},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "get_task_detail",
				Description: "获取任务的完整详情，包括所有 todo 项、交付物和结果。",
				Parameters: &jsonschema.Definition{
					Type:     jsonschema.Object,
					Required: []string{"task_id"},
					Properties: map[string]jsonschema.Definition{
						"task_id": {
							Type:        jsonschema.String,
							Description: "任务 ID",
						},
					},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "get_dashboard_stats",
				Description: "获取仪表盘统计数据，包括 数字员工 在线数、任务进度、成功率等。",
				Parameters: &jsonschema.Definition{
					Type:       jsonschema.Object,
					Properties: map[string]jsonschema.Definition{},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "list_projects",
				Description: "列出用户的所有项目及其任务统计概览。",
				Parameters: &jsonschema.Definition{
					Type:       jsonschema.Object,
					Properties: map[string]jsonschema.Definition{},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "navigate",
				Description: "导航到指定页面。当用户要求打开某个页面、查看某个任务或项目时使用。",
				Parameters: &jsonschema.Definition{
					Type:     jsonschema.Object,
					Required: []string{"path", "label"},
					Properties: map[string]jsonschema.Definition{
						"path": {
							Type:        jsonschema.String,
							Description: routeDescriptionForLLM(),
						},
						"label": {
							Type:        jsonschema.String,
							Description: "导航目标的友好描述，如：仪表盘、项目列表",
						},
					},
				},
			},
		},
	}

	if hasKnowledge {
		tools = append([]openai.Tool{{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "search_knowledge",
				Description: "搜索知识库中的文档内容。使用语义向量搜索匹配相关文档片段。",
				Parameters: &jsonschema.Definition{
					Type:     jsonschema.Object,
					Required: []string{"query"},
					Properties: map[string]jsonschema.Definition{
						"query": {
							Type:        jsonschema.String,
							Description: "搜索查询文本",
						},
						"project_id": {
							Type:        jsonschema.String,
							Description: "限定在某个项目的知识库中搜索",
						},
						"top_k": {
							Type:        jsonschema.Integer,
							Description: "返回结果数量，默认 5",
						},
					},
				},
			},
		}}, tools...)
	}

	return tools
}

// Execute runs a tool by name with the given JSON arguments.
func (e *ToolExecutor) Execute(ctx context.Context, sc store.Scope, toolName, argsJSON string) (any, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("invalid tool args: %w", err)
	}

	switch toolName {
	case "search_knowledge":
		return e.searchKnowledge(ctx, sc, args)
	case "search_tasks":
		return e.searchTasks(sc, args)
	case "get_task_detail":
		return e.getTaskDetail(sc, args)
	case "get_dashboard_stats":
		return e.getDashboardStats(sc)
	case "list_projects":
		return e.listProjects(sc)
	case "navigate":
		return e.navigate(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func (e *ToolExecutor) searchKnowledge(ctx context.Context, sc store.Scope, args map[string]any) (any, error) {
	query, _ := args["query"].(string)
	projectID, _ := args["project_id"].(string)
	topK := intArg(args, "top_k", 5)

	if e.embedder == nil || e.qdrant == nil {
		// Fallback to text search
		var pid *string
		if projectID != "" {
			pid = &projectID
		}
		// 直接透传 sc：无租户头退回 user 维度；带头时由 store 内部做 org 裁决
		chunks, err := e.store.SearchKnowledgeChunks(ctx, sc, pid, query, topK)
		if err != nil {
			return nil, err
		}
		results := make([]map[string]any, 0, len(chunks))
		for _, c := range chunks {
			results = append(results, map[string]any{
				"document_id":    c.DocumentID,
				"document_title": e.store.GetKnowledgeDocTitle(c.DocumentID),
				"content":        c.Content,
				"chunk_index":    c.ChunkIndex,
			})
		}
		return map[string]any{"results": results, "count": len(results)}, nil
	}

	// Vector search
	embeddings, err := e.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return map[string]any{"results": []any{}, "count": 0}, nil
	}

	mustConds := []knowledge.QdrantCondition{
// ⚠️ Qdrant 向量过滤保持 user 维度（宁可漏不可泄）：org 维度检索
	// 需在 chunk 落库时写入 org_id payload，属二期改造。
		{Key: "user_id", Match: map[string]any{"value": sc.UserID}},
	}
	filter := &knowledge.QdrantFilter{Must: mustConds}
	if projectID != "" {
		filter.Should = []knowledge.QdrantCondition{
			{Key: "project_id", Match: map[string]any{"value": projectID}},
		}
	}

	hits, err := e.qdrant.Search(ctx, embeddings[0], filter, topK)
	if err != nil {
		return nil, err
	}

	results := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		if hit.Score < 0.5 {
			continue
		}
		chunkID, _ := hit.Payload["chunk_id"].(string)
		docID, _ := hit.Payload["document_id"].(string)
		chunks, err := e.store.GetKnowledgeChunksByIDs([]string{chunkID})
		if err != nil || len(chunks) == 0 {
			continue
		}
		results = append(results, map[string]any{
			"document_id":    docID,
			"document_title": e.store.GetKnowledgeDocTitle(docID),
			"content":        chunks[0].Content,
			"score":          hit.Score,
			"chunk_index":    chunks[0].ChunkIndex,
		})
	}
	return map[string]any{"results": results, "count": len(results)}, nil
}

func (e *ToolExecutor) searchTasks(sc store.Scope, args map[string]any) (any, error) {
	query, _ := args["query"].(string)
	status, _ := args["status"].(string)
	projectID, _ := args["project_id"].(string)

	// If project_id is specified, search within that project
	if projectID != "" {
		tasks, appErr := e.store.ListTasks(sc, projectID, status)
		if appErr != nil {
			return nil, fmt.Errorf("%s", appErr.Message)
		}
		filtered := filterTasksByQuery(tasks, query)
		return map[string]any{"tasks": filtered, "count": len(filtered)}, nil
	}

	// Otherwise search across all projects
	projects := e.store.ListProjects(sc)
	var allTasks []model.TaskListItem
	for _, p := range projects {
		tasks, appErr := e.store.ListTasks(sc, p.ID, status)
		if appErr != nil {
			continue
		}
		allTasks = append(allTasks, filterTasksByQuery(tasks, query)...)
	}
	if allTasks == nil {
		allTasks = []model.TaskListItem{}
	}
	return map[string]any{"tasks": allTasks, "count": len(allTasks)}, nil
}

func (e *ToolExecutor) getTaskDetail(sc store.Scope, args map[string]any) (any, error) {
	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	task, appErr := e.store.GetTask(sc, taskID)
	if appErr != nil {
		return nil, fmt.Errorf("%s", appErr.Message)
	}
	return task, nil
}

func (e *ToolExecutor) getDashboardStats(sc store.Scope) (any, error) {
	stats := e.store.GetDashboardStats(sc)
	return stats, nil
}

func (e *ToolExecutor) listProjects(sc store.Scope) (any, error) {
	projects := e.store.ListProjects(sc)
	items := make([]map[string]any, 0, len(projects))
	for _, p := range projects {
		items = append(items, map[string]any{
			"id":          p.ID,
			"name":        p.Name,
			"description": p.Description,
			"status":      p.Status,
			"task_summary": map[string]any{
				"total":       p.TaskSummary.TaskTotal,
				"pending":     p.TaskSummary.PendingCount,
				"in_progress": p.TaskSummary.InProgressCount,
				"done":        p.TaskSummary.DoneCount,
				"failed":      p.TaskSummary.FailedCount,
			},
		})
	}
	return map[string]any{"projects": items, "count": len(items)}, nil
}

func (e *ToolExecutor) navigate(args map[string]any) (any, error) {
	path, _ := args["path"].(string)
	label, _ := args["label"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if !isValidRoute(path) {
		return nil, fmt.Errorf("无效的导航路径: %s", path)
	}
	return NavigateEvent{Path: path, Label: label}, nil
}

// helpers

func filterTasksByQuery(tasks []model.TaskListItem, query string) []model.TaskListItem {
	if query == "" {
		return tasks
	}
	q := strings.ToLower(query)
	var matched []model.TaskListItem
	for _, t := range tasks {
		if strings.Contains(strings.ToLower(t.Title), q) ||
			strings.Contains(strings.ToLower(t.Description), q) {
			matched = append(matched, t)
		}
	}
	return matched
}

func intArg(args map[string]any, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return defaultVal
}

func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}
