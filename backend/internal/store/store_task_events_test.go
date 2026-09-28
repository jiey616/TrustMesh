package store

import (
	"testing"
	"time"

	"trustmesh/backend/internal/model"
)

// ListTaskEvents 的 since 游标（2026-09-28）契约。
//
// 这个游标是「轮询只传增量」的基础：首次全量之后，前端每 4 秒的轮询只带
// 上次最后一条的 created_at，于是不再重传整条 542 KB 的历史。
//
// 🔴 两条必须钉死的性质：
//  1. **零值游标 = 全量**，与改造前逐字节等价 —— 移动端（frontend-mobile）与
//     旧桌面端都在调同一个端点，语义一变就会静默少数据显示。
//  2. **游标语义是 `>=` 而不是 `>`**：相邻事件可能落在同一毫秒，用 `>` 会吞掉
//     边界那条，表现为「执行过程」偶发缺一条。多回一条由调用方按 id 去重即可。
//
// 测试刻意用 time.Sleep 把「游标时刻」和「新事件时刻」拉开，否则连续写入很可能
// 落在同一毫秒，断言会 flaky（并掩盖 `>` 与 `>=` 的差异）。
func TestListTaskEventsSinceCursor(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "since 游标",
		Description: "验证增量事件流",
		Todos: []TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Build API",
				Description:    "implement",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}
	sc := Scope{UserID: task.UserID}

	before, appErr := s.ListTaskEvents(sc, task.ID, time.Time{})
	if appErr != nil {
		t.Fatalf("list all events: %v", appErr)
	}
	if len(before) == 0 {
		t.Fatal("前置条件不成立：创建任务后应已有事件")
	}
	cursor := before[len(before)-1].CreatedAt

	// 拉开 5ms，确保后续事件的 CreatedAt 严格晚于游标（大于时钟精度）。
	time.Sleep(5 * time.Millisecond)

	if _, _, appErr := s.CompleteTodoByNodeWithMessageID(developer.NodeID, "msg-since-1", TodoCompleteInput{
		TaskID: task.ID,
		TodoID: "todo-1",
		Result: model.TodoResult{Summary: "done", Output: "ok"},
	}); appErr != nil {
		t.Fatalf("complete todo: %v", appErr)
	}

	after, appErr := s.ListTaskEvents(sc, task.ID, time.Time{})
	if appErr != nil {
		t.Fatalf("list all events after: %v", appErr)
	}
	added := len(after) - len(before)
	if added <= 0 {
		t.Fatalf("前置条件不成立：complete 之后事件数应从 %d 增加", len(before))
	}

	// 1) 增量恰好是新加的那几条（sleep 保证边界不在同一毫秒）。
	incremental, appErr := s.ListTaskEvents(sc, task.ID, cursor)
	if appErr != nil {
		t.Fatalf("list events since cursor: %v", appErr)
	}
	// 新增的 added 条**一条都不能漏** —— 这是本次改动存在的全部意义。
	for _, e := range after[len(after)-added:] {
		found := false
		for _, got := range incremental {
			if got.ID == e.ID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("增量漏掉了新增事件 %s（类型 %s）", e.ID, e.EventType)
		}
	}
	// 且增量必须是全量的后缀（顺序与内容都对得上）。
	suffix := after[len(after)-len(incremental):]
	for i := range incremental {
		if incremental[i].ID != suffix[i].ID {
			t.Fatalf("增量不是全量的后缀：第 %d 条 %s != %s", i, incremental[i].ID, suffix[i].ID)
		}
	}
	// 游标语义是 `>=`：**与游标同毫秒的旧事件会一并回来**（由调用方按 id 去重吃掉），
	// 所以这里断言的是「不早于游标」，而不是「恰好等于新增数」——后者会 flaky。
	for _, e := range incremental {
		if e.CreatedAt.Before(cursor) {
			t.Fatalf("增量包含早于游标的事件 %s（%s < %s）", e.ID, e.CreatedAt, cursor)
		}
	}
	// 游标确实在过滤：用一个严格晚于第 0 条的游标，至少能滤掉第一条。
	mid, appErr := s.ListTaskEvents(sc, task.ID, after[0].CreatedAt.Add(time.Nanosecond))
	if appErr != nil {
		t.Fatalf("list events with mid cursor: %v", appErr)
	}
	if len(mid) >= len(after) {
		t.Fatalf("游标未滤掉任何事件：%d 条 vs 全量 %d 条", len(mid), len(after))
	}

	// 2) 零值游标 = 全量（向后兼容红线：移动端与旧桌面端依赖这条）。
	zeroed, appErr := s.ListTaskEvents(sc, task.ID, time.Time{})
	if appErr != nil {
		t.Fatalf("list events with zero cursor: %v", appErr)
	}
	if len(zeroed) != len(after) {
		t.Fatalf("零值游标返回 %d 条，want %d（必须与全量等价）", len(zeroed), len(after))
	}

	// 3) 未来游标 ⇒ 空集（证明游标确实在过滤，而不是被忽略）。
	empty, appErr := s.ListTaskEvents(sc, task.ID, time.Now().Add(time.Hour))
	if appErr != nil {
		t.Fatalf("list events with future cursor: %v", appErr)
	}
	if len(empty) != 0 {
		t.Fatalf("未来游标应返回空集，got %d 条", len(empty))
	}
}
