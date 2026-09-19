package store

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"trustmesh/backend/internal/model"
)

// newBindArtifactFixture 造一个「任务 + 两个 todo + 工作流快照」的沙盘，用于测
// 按 (task, todo) 维度的手工绑定 BindArtifactOutput。第 2 步（TD_02）声明
// step2Outputs 个输出位。两个 todo 都用 AgentID 显式绑定，保证 ordered 对齐
// 稳定（不依赖 role 模糊匹配）。
func newBindArtifactFixture(t *testing.T, step2Outputs []model.StepOutput) (*Store, string, string) {
	t.Helper()
	const userID, projectID, taskID = "u1", "p1", "t1"

	steps := []model.WorkflowStep{
		{Name: "剧本创作", AgentID: "ag-writer", Outputs: []model.StepOutput{{Name: "剧本正文"}}},
		{Name: "分镜拆解", AgentID: "ag-board", Outputs: step2Outputs},
	}
	s := New()
	s.log = zap.NewNop()
	s.projects[projectID] = &model.Project{
		ID: projectID, UserID: userID, Name: "西游动画",
		Workflows: []model.Workflow{{Name: "主流程", Steps: steps}},
	}
	s.projectTasks[projectID] = []string{taskID}
	s.tasks[taskID] = &model.TaskDetail{
		ID: taskID, ProjectID: projectID, UserID: userID, Title: "西游动画任务", Status: "in_progress",
		WorkflowRef: &model.WorkflowRef{WorkflowName: "主流程", StepFrom: 0, StepTo: 1},
		Workflow:    &model.Workflow{Name: "主流程", Steps: steps},
		Todos: []model.Todo{
			{ID: "TD_01", Order: 0, Title: "剧本创作", Status: "done",
				Assignee: model.TodoAssignee{AgentID: "ag-writer", Name: "编剧"}},
			{ID: "TD_02", Order: 1, Title: "分镜拆解", Status: "in_progress",
				Assignee: model.TodoAssignee{AgentID: "ag-board", Name: "分镜师"}},
		},
	}
	return s, userID, taskID
}

// xlsxMime 是分镜表的真实 mime，用于让"mime 命中声明位"的场景成立。
const xlsxMime = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// TestBindArtifactOutputRejectsUndeclaredSlotName 第三个绑定入口同样只认声明位：
// 下游步骤按 (步骤, output_name) 取上一步产物，凭空拼一个名字取不到 ⇒ 必须当场拒。
// 关键：拒绝时**产物与输出位都不能被碰**（校验必须发生在任何写入之前）。
func TestBindArtifactOutputRejectsUndeclaredSlotName(t *testing.T) {
	s, userID, taskID := newBindArtifactFixture(t, []model.StepOutput{{Name: "剧名_分镜头脚本", MimeType: "xlsx"}})
	s.taskArtifacts[taskID] = []model.TaskArtifact{{
		TransferID: "tr-a", TaskID: taskID, TodoID: "TD_02",
		FileName: "shotlist.xlsx", MimeType: xlsxMime, Kind: model.ArtifactKindProcess,
	}}

	_, appErr := s.BindArtifactOutput(Scope{UserID: userID}, taskID, "TD_02", "tr-a", "我自己编的名字")
	if appErr == nil || appErr.Code != "OUTPUT_SLOT_NOT_DECLARED" {
		t.Fatalf("expected OUTPUT_SLOT_NOT_DECLARED, got %+v", appErr)
	}
	if a := s.taskArtifacts[taskID][0]; a.Kind != model.ArtifactKindProcess || a.OutputName != "" {
		t.Fatalf("被拒的绑定不得改动产物，got kind %q output %q", a.Kind, a.OutputName)
	}
	if got := len(s.tasks[taskID].Todos[1].Outputs); got != 0 {
		t.Fatalf("被拒的绑定不得占用输出位，got %d entries", got)
	}
}

// TestBindArtifactOutputKeepsFreeNameWhenStepDeclaresNoSlots 步骤没声明输出位时
// 保持自由命名 —— 无从校验，硬造一套白名单只会把存量数据与手工救场一并挡死。
func TestBindArtifactOutputKeepsFreeNameWhenStepDeclaresNoSlots(t *testing.T) {
	s, userID, taskID := newBindArtifactFixture(t, nil)
	s.taskArtifacts[taskID] = []model.TaskArtifact{{
		TransferID: "tr-free", TaskID: taskID, TodoID: "TD_02",
		FileName: "随手交的东西.md", MimeType: "text/markdown", Kind: model.ArtifactKindProcess,
	}}

	bound, appErr := s.BindArtifactOutput(Scope{UserID: userID}, taskID, "TD_02", "tr-free", "我自己编的名字")
	if appErr != nil {
		t.Fatalf("no declared slots must keep free-form naming: %v", appErr)
	}
	if bound.Kind != model.ArtifactKindDeliverable || bound.OutputName != "我自己编的名字" {
		t.Fatalf("bound = kind %q output %q", bound.Kind, bound.OutputName)
	}
}

// TestBindArtifactOutputKeepsFreeNameWhenTodoAlignsToNoStep 任务里有对不上任何
// 步骤的 todo（多出来的协作 todo / 存量数据）时同样保持自由命名 —— 对齐失败必须
// 走"未声明输出位"，绝不能因为解析不到而报错。
func TestBindArtifactOutputKeepsFreeNameWhenTodoAlignsToNoStep(t *testing.T) {
	s, userID, taskID := newBindArtifactFixture(t, []model.StepOutput{{Name: "剧名_分镜头脚本", MimeType: "xlsx"}})
	s.tasks[taskID].Todos = append(s.tasks[taskID].Todos, model.Todo{
		ID: "TD_09", Order: 2, Title: "杂项", Status: "in_progress",
		Assignee: model.TodoAssignee{AgentID: "ag-other", Name: "别人"},
	})
	s.taskArtifacts[taskID] = []model.TaskArtifact{{
		TransferID: "tr-extra", TaskID: taskID, TodoID: "TD_09",
		FileName: "杂项.md", MimeType: "text/markdown", Kind: model.ArtifactKindProcess,
	}}

	bound, appErr := s.BindArtifactOutput(Scope{UserID: userID}, taskID, "TD_09", "tr-extra", "随手一个名字")
	if appErr != nil {
		t.Fatalf("a todo that aligns to no step must not be blocked: %v", appErr)
	}
	if bound.Kind != model.ArtifactKindDeliverable || bound.OutputName != "随手一个名字" {
		t.Fatalf("bound = kind %q output %q", bound.Kind, bound.OutputName)
	}
}

// TestBindArtifactOutputRebindDemotesSupersededDeliverable 覆盖语义：一个输出位
// 只挂一个交付物。重新绑定后旧交付物（含指向同一物理文件的冗余副本）降级为过程
// 文件，否则 stepOutputsUnsafe 的 TodoID 兜底分支会在 pipeline 上把新旧两份并列。
func TestBindArtifactOutputRebindDemotesSupersededDeliverable(t *testing.T) {
	s, userID, taskID := newBindArtifactFixture(t, []model.StepOutput{{Name: "剧名_分镜头脚本", MimeType: "xlsx"}})
	s.taskArtifacts[taskID] = []model.TaskArtifact{
		{TransferID: "tr-v1", TaskID: taskID, TodoID: "TD_02", ProjectFileID: "pf-v1",
			FileName: "shotlist_v1.xlsx", MimeType: xlsxMime,
			Kind: model.ArtifactKindDeliverable, OutputName: "剧名_分镜头脚本"},
		// 同一物理文件的重复上传：只降级 todo.Outputs 里记的那一条不够。
		{TransferID: "tr-v1-dup", TaskID: taskID, TodoID: "TD_02", ProjectFileID: "pf-v1",
			FileName: "shotlist_v1.xlsx", MimeType: xlsxMime,
			Kind: model.ArtifactKindDeliverable, OutputName: "剧名_分镜头脚本"},
		{TransferID: "tr-v2", TaskID: taskID, TodoID: "TD_02", ProjectFileID: "pf-v2",
			FileName: "shotlist_v2.xlsx", MimeType: xlsxMime, Kind: model.ArtifactKindProcess},
	}
	s.tasks[taskID].Todos[1].Outputs = []model.TodoOutput{
		{OutputName: "剧名_分镜头脚本", ArtifactID: "tr-v1", FileRef: "pf-v1"},
	}

	bound, appErr := s.BindArtifactOutput(Scope{UserID: userID}, taskID, "TD_02", "tr-v2", "剧名_分镜头脚本")
	if appErr != nil {
		t.Fatalf("bind: %v", appErr)
	}
	if bound.Kind != model.ArtifactKindDeliverable {
		t.Fatalf("bound kind = %q, want deliverable", bound.Kind)
	}

	todo := &s.tasks[taskID].Todos[1]
	if len(todo.Outputs) != 1 || todo.Outputs[0].ArtifactID != "tr-v2" {
		t.Fatalf("todo.Outputs = %+v, want exactly the new revision", todo.Outputs)
	}
	for _, a := range s.taskArtifacts[taskID] {
		switch a.TransferID {
		case "tr-v1", "tr-v1-dup":
			if a.Kind != model.ArtifactKindProcess || a.OutputName != "" {
				t.Fatalf("%s = kind %q output %q, want a demoted process file", a.TransferID, a.Kind, a.OutputName)
			}
		case "tr-v2":
			if a.Kind != model.ArtifactKindDeliverable || a.OutputName != "剧名_分镜头脚本" {
				t.Fatalf("new revision = kind %q output %q, want the deliverable binding", a.Kind, a.OutputName)
			}
		}
	}
}

// TestBindArtifactOutputClearsOrphanFlag 手工绑定是显式意图，绝不算「终态后迟到」：
// 否则一个被 reopen 回补过的文件会一直带着 orphan 标，前端继续提示"迟到"。
func TestBindArtifactOutputClearsOrphanFlag(t *testing.T) {
	s, userID, taskID := newBindArtifactFixture(t, []model.StepOutput{{Name: "剧名_分镜头脚本", MimeType: "xlsx"}})
	s.taskArtifacts[taskID] = []model.TaskArtifact{{
		TransferID: "tr-late", TaskID: taskID, TodoID: "TD_02",
		FileName: "shotlist.xlsx", MimeType: xlsxMime,
		Kind: model.ArtifactKindProcess, Orphan: true,
	}}

	bound, appErr := s.BindArtifactOutput(Scope{UserID: userID}, taskID, "TD_02", "tr-late", "剧名_分镜头脚本")
	if appErr != nil {
		t.Fatalf("bind: %v", appErr)
	}
	if bound.Orphan {
		t.Fatalf("bound artifact must not stay orphaned")
	}
}

// TestBindArtifactOutputWarnsOnMimeMismatchButStillBinds 口径：mime 与声明位不符
// **只告警不拒绝**。本入口的存在意义之一就是"agent 上传 mime 意外"的救场，兄弟
// 实现 BindStepOutput 也不比对 mime；但必须留痕，否则没人知道契约被绕过了。
func TestBindArtifactOutputWarnsOnMimeMismatchButStillBinds(t *testing.T) {
	s, userID, taskID := newBindArtifactFixture(t, []model.StepOutput{{Name: "剧名_分镜头脚本", MimeType: "xlsx"}})
	core, logs := observer.New(zap.WarnLevel)
	s.log = zap.New(core)
	s.taskArtifacts[taskID] = []model.TaskArtifact{{
		TransferID: "tr-md", TaskID: taskID, TodoID: "TD_02",
		FileName: "shotlist.md", MimeType: "text/markdown", Kind: model.ArtifactKindProcess,
	}}

	bound, appErr := s.BindArtifactOutput(Scope{UserID: userID}, taskID, "TD_02", "tr-md", "剧名_分镜头脚本")
	if appErr != nil {
		t.Fatalf("mime mismatch must not block a manual bind: %v", appErr)
	}
	if bound.Kind != model.ArtifactKindDeliverable {
		t.Fatalf("bound kind = %q, want deliverable", bound.Kind)
	}
	if got := logs.FilterMessage("manual bind: artifact mime does not match the declared slot").Len(); got != 1 {
		t.Fatalf("expected exactly 1 mime-mismatch warning, got %d (%v)", got, logs.All())
	}
}
