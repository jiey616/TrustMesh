package store

import (
	"testing"

	"trustmesh/backend/internal/model"
)

// newBindStepFixture 造一个带「项目总流程」的项目：2 个步骤分别由两个 agent 承担，
// 任务已派发且 todo 就位，并预置一个用户手工上传的项目文件（没有任何 TaskArtifact）。
func newBindStepFixture(t *testing.T, step2Outputs []model.StepOutput) (*Store, string, string) {
	t.Helper()
	const userID, projectID, taskID = "u1", "p1", "t1"

	steps := []model.WorkflowStep{
		{Name: "剧本创作", AgentID: "ag-writer", Outputs: []model.StepOutput{{Name: "剧本正文"}}},
		{Name: "分镜拆解", AgentID: "ag-board", Outputs: step2Outputs},
	}
	s := New()
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
	// 用户手工上传的项目文件：source=user_upload，磁盘上只有 project_files 一行。
	s.projectFiles["f-manual"] = &model.ProjectFile{
		ID: "f-manual", ProjectID: projectID, FileName: "分镜_v3.xlsx", FileSize: 2048,
		MimeType: "application/vnd.ms-excel", Source: "user_upload", UploadedBy: userID,
	}
	s.projectFiles["f-manual2"] = &model.ProjectFile{
		ID: "f-manual2", ProjectID: projectID, FileName: "分镜_v4.xlsx", FileSize: 4096,
		MimeType: "application/vnd.ms-excel", Source: "user_upload", UploadedBy: userID,
	}
	// 别的项目的文件：必须被拒。
	s.projectFiles["f-other"] = &model.ProjectFile{
		ID: "f-other", ProjectID: "p-other", FileName: "别项目的.xlsx",
		Source: "user_upload", UploadedBy: userID,
	}
	return s, userID, projectID
}

// TestBindStepOutputFromManualUpload 覆盖核心诉求：用户手工上传的文件（无 TaskArtifact）
// 也能随时绑成任意步骤的交付物 —— 后端按需物化一条 artifact 记录。
func TestBindStepOutputFromManualUpload(t *testing.T) {
	s, userID, projectID := newBindStepFixture(t, []model.StepOutput{{Name: "分镜脚本"}})

	art, appErr := s.BindStepOutput(Scope{UserID: userID}, projectID, 1, BindStepOutputRequest{
		FileID: "f-manual", OutputName: "分镜脚本",
	})
	if appErr != nil {
		t.Fatalf("bind: %v", appErr)
	}
	if art.Kind != model.ArtifactKindDeliverable || art.OutputName != "分镜脚本" {
		t.Fatalf("artifact = kind %q output %q", art.Kind, art.OutputName)
	}
	if art.TodoID != "TD_02" {
		t.Fatalf("artifact todo = %q, want TD_02", art.TodoID)
	}
	if art.ProjectFileID != "f-manual" {
		t.Fatalf("artifact project file = %q, want f-manual", art.ProjectFileID)
	}

	// 物化后的 artifact 必须落在目标任务下，pipeline 才认。
	arts := s.taskArtifacts["t1"]
	if len(arts) != 1 || arts[0].TransferID != art.TransferID {
		t.Fatalf("task artifacts = %+v, want the materialised one", arts)
	}

	task := s.tasks["t1"]
	if len(task.Todos[1].Outputs) != 1 || task.Todos[1].Outputs[0].OutputName != "分镜脚本" {
		t.Fatalf("todo outputs = %+v", task.Todos[1].Outputs)
	}
	if task.Todos[1].Outputs[0].FileRef != "f-manual" {
		t.Fatalf("todo output file_ref = %q, want f-manual", task.Todos[1].Outputs[0].FileRef)
	}
	// 项目文件树同步标成交付物，文件区才能显示「交付」标签。
	if pf := s.projectFiles["f-manual"]; pf.Kind != model.ArtifactKindDeliverable || pf.OutputName != "分镜脚本" {
		t.Fatalf("project file = kind %q output %q", pf.Kind, pf.OutputName)
	}
	// 手工绑定算正向进展（P-03 硬闸依赖它）。
	if task.Todos[1].LastProgressAt == nil {
		t.Fatal("expected LastProgressAt to be refreshed")
	}
	// 事件要落，否则审计链看不到人工干预。
	found := false
	for _, ev := range s.taskEvents["t1"] {
		if ev.EventType == "todo_output_bound_manually" {
			found = true
			if ev.Metadata["source"] != "project_file" {
				t.Fatalf("event source = %v, want project_file", ev.Metadata["source"])
			}
		}
	}
	if !found {
		t.Fatal("expected a todo_output_bound_manually event")
	}
}

// TestBindStepOutputRejectsUndeclaredSlot 步骤声明了输出位就只能选这些，
// 避免拼出下游步骤 source 解析不到的名字。
func TestBindStepOutputRejectsUndeclaredSlot(t *testing.T) {
	s, userID, projectID := newBindStepFixture(t, []model.StepOutput{{Name: "分镜脚本"}})

	_, appErr := s.BindStepOutput(Scope{UserID: userID}, projectID, 1, BindStepOutputRequest{
		FileID: "f-manual", OutputName: "我自己编的名字",
	})
	if appErr == nil || appErr.Code != "OUTPUT_SLOT_NOT_DECLARED" {
		t.Fatalf("expected OUTPUT_SLOT_NOT_DECLARED, got %+v", appErr)
	}
}

// TestBindStepOutputFreeNameWhenNoDeclaredSlots 步骤没声明输出位时保持自由命名（兼容存量数据）。
func TestBindStepOutputFreeNameWhenNoDeclaredSlots(t *testing.T) {
	s, userID, projectID := newBindStepFixture(t, nil)

	art, appErr := s.BindStepOutput(Scope{UserID: userID}, projectID, 1, BindStepOutputRequest{
		FileID: "f-manual", OutputName: "随便一个名字",
	})
	if appErr != nil {
		t.Fatalf("bind with free-form name: %v", appErr)
	}
	if art.OutputName != "随便一个名字" {
		t.Fatalf("output = %q", art.OutputName)
	}
}

// TestBindStepOutputRejectsUndispatchedStep 步骤还没派发任务时明确拒绝，
// 不留下无人认领的半成品绑定。
func TestBindStepOutputRejectsUndispatchedStep(t *testing.T) {
	s, userID, projectID := newBindStepFixture(t, []model.StepOutput{{Name: "分镜脚本"}})
	// 让任务只覆盖 step 0：step 1 尚未派发。
	s.tasks["t1"].WorkflowRef = &model.WorkflowRef{WorkflowName: "主流程", StepFrom: 0, StepTo: 0}

	_, appErr := s.BindStepOutput(Scope{UserID: userID}, projectID, 1, BindStepOutputRequest{
		FileID: "f-manual", OutputName: "分镜脚本",
	})
	if appErr == nil || appErr.Code != "STEP_NOT_STARTED" {
		t.Fatalf("expected STEP_NOT_STARTED, got %+v", appErr)
	}
}

// TestBindStepOutputRejectsForeignProjectFile 别的项目的文件不能绑过来。
func TestBindStepOutputRejectsForeignProjectFile(t *testing.T) {
	s, userID, projectID := newBindStepFixture(t, []model.StepOutput{{Name: "分镜脚本"}})

	_, appErr := s.BindStepOutput(Scope{UserID: userID}, projectID, 1, BindStepOutputRequest{
		FileID: "f-other", OutputName: "分镜脚本",
	})
	if appErr == nil || appErr.Code != "FILE_NOT_IN_PROJECT" {
		t.Fatalf("expected FILE_NOT_IN_PROJECT, got %+v", appErr)
	}
}

// TestBindStepOutputStepIndexOutOfRange 越界的步骤下标要被挡下。
func TestBindStepOutputStepIndexOutOfRange(t *testing.T) {
	s, userID, projectID := newBindStepFixture(t, []model.StepOutput{{Name: "分镜脚本"}})

	if _, appErr := s.BindStepOutput(Scope{UserID: userID}, projectID, 9, BindStepOutputRequest{
		FileID: "f-manual", OutputName: "分镜脚本",
	}); appErr == nil {
		t.Fatal("expected an error for an out-of-range step index")
	}
	if _, appErr := s.BindStepOutput(Scope{UserID: userID}, projectID, -1, BindStepOutputRequest{
		FileID: "f-manual", OutputName: "分镜脚本",
	}); appErr == nil {
		t.Fatal("expected an error for a negative step index")
	}
}

// TestBindStepOutputCrossTaskArtifact 别的任务产出的 artifact 也能绑到本步骤：
// 在目标任务下新建一条绑定记录复用同一个物理文件，不动源任务的历史。
func TestBindStepOutputCrossTaskArtifact(t *testing.T) {
	s, userID, projectID := newBindStepFixture(t, []model.StepOutput{{Name: "分镜脚本"}})
	// 同项目里另一个任务产出的 artifact。
	s.tasks["t2"] = &model.TaskDetail{ID: "t2", ProjectID: projectID, UserID: userID, Title: "素材任务"}
	s.taskArtifacts["t2"] = []model.TaskArtifact{
		{TransferID: "tid-src", TaskID: "t2", FileName: "素材包.zip", ProjectFileID: "f-src"},
	}
	s.projectFiles["f-src"] = &model.ProjectFile{
		ID: "f-src", ProjectID: projectID, FileName: "素材包.zip",
		Source: "agent_artifact", TransferID: "tid-src",
	}

	art, appErr := s.BindStepOutput(Scope{UserID: userID}, projectID, 1, BindStepOutputRequest{
		ArtifactID: "tid-src", OutputName: "分镜脚本",
	})
	if appErr != nil {
		t.Fatalf("bind cross-task artifact: %v", appErr)
	}
	if art.TaskID != "t1" {
		t.Fatalf("bound artifact task = %q, want t1", art.TaskID)
	}
	if art.TransferID == "tid-src" {
		t.Fatal("expected a new artifact record under the target task, got the source one")
	}
	// 源任务的历史不能被改动。
	if len(s.taskArtifacts["t2"]) != 1 || s.taskArtifacts["t2"][0].OutputName != "" {
		t.Fatalf("source task artifact was mutated: %+v", s.taskArtifacts["t2"])
	}
}

// TestBindStepOutputReplaceExistingSlot 同一输出位重复绑定必须显式 replace，
// 覆盖后该位只能有一份产物。
func TestBindStepOutputReplaceExistingSlot(t *testing.T) {
	s, userID, projectID := newBindStepFixture(t, []model.StepOutput{{Name: "分镜脚本"}})

	if _, appErr := s.BindStepOutput(Scope{UserID: userID}, projectID, 1, BindStepOutputRequest{
		FileID: "f-manual", OutputName: "分镜脚本",
	}); appErr != nil {
		t.Fatalf("first bind: %v", appErr)
	}

	// 不带 replace：拒绝。
	_, appErr := s.BindStepOutput(Scope{UserID: userID}, projectID, 1, BindStepOutputRequest{
		FileID: "f-manual2", OutputName: "分镜脚本",
	})
	if appErr == nil || appErr.Code != "OUTPUT_SLOT_OCCUPIED" {
		t.Fatalf("expected OUTPUT_SLOT_OCCUPIED, got %+v", appErr)
	}

	// agent 重复上传会留下指向同一文件的冗余副本，它不在 todo.Outputs 里，
	// 但 stepOutputsUnsafe 的兜底分支照样会把它列出来 → 必须一并降级。
	s.taskArtifacts["t1"] = append(s.taskArtifacts["t1"], model.TaskArtifact{
		TransferID: "tid-dup", TaskID: "t1", TodoID: "TD_02",
		ProjectFileID: "f-manual", Kind: model.ArtifactKindDeliverable, OutputName: "分镜脚本",
	})

	// 带 replace：覆盖成功，且输出位仍然只有一条。
	art, appErr := s.BindStepOutput(Scope{UserID: userID}, projectID, 1, BindStepOutputRequest{
		FileID: "f-manual2", OutputName: "分镜脚本", Replace: true,
	})
	if appErr != nil {
		t.Fatalf("replace bind: %v", appErr)
	}
	if art.ProjectFileID != "f-manual2" {
		t.Fatalf("replaced artifact file = %q, want f-manual2", art.ProjectFileID)
	}
	outputs := s.tasks["t1"].Todos[1].Outputs
	if len(outputs) != 1 || outputs[0].FileRef != "f-manual2" {
		t.Fatalf("todo outputs after replace = %+v", outputs)
	}
	// 被顶掉的旧产物不再被任何输出位引用 → 必须降级为过程文件，否则
	// stepOutputsUnsafe 的兜底分支（按 TodoID 收 deliverable）会继续列出它，
	// pipeline 上同一位置就会新旧两份并存。
	demoted := 0
	for _, a := range s.taskArtifacts["t1"] {
		if a.ProjectFileID == "f-manual" {
			demoted++
			if a.Kind != model.ArtifactKindProcess || a.OutputName != "" {
				t.Fatalf("displaced artifact %s = kind %q output %q, want process/empty",
					a.TransferID, a.Kind, a.OutputName)
			}
		}
	}
	// 正本 + 冗余副本，两条都要降级。
	if demoted != 2 {
		t.Fatalf("expected both copies of the displaced file to be demoted, got %d", demoted)
	}
}

// TestBindStepOutputRejectsMissingIdentity 至少要给 file_id 或 artifact_id 之一。
func TestBindStepOutputRejectsMissingIdentity(t *testing.T) {
	s, userID, projectID := newBindStepFixture(t, []model.StepOutput{{Name: "分镜脚本"}})

	if _, appErr := s.BindStepOutput(Scope{UserID: userID}, projectID, 1, BindStepOutputRequest{
		OutputName: "分镜脚本",
	}); appErr == nil {
		t.Fatal("expected an error when neither file_id nor artifact_id is given")
	}
	if _, appErr := s.BindStepOutput(Scope{UserID: userID}, projectID, 1, BindStepOutputRequest{
		FileID: "f-manual",
	}); appErr == nil {
		t.Fatal("expected an error when output_name is missing")
	}
}
