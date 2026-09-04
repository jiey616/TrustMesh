package store

import (
	"testing"

	"trustmesh/backend/internal/model"
)

func steps(ns ...string) []model.WorkflowStep {
	out := make([]model.WorkflowStep, len(ns))
	for i, n := range ns {
		out[i] = model.WorkflowStep{Name: n, Role: "agent" + n}
	}
	return out
}

// seedProjectWithPM creates a user + PM agent + project, returning
// (store, userID, projectID).
func seedProjectWithPM(t *testing.T) (*Store, string, string) {
	t.Helper()
	s := New()
	user, appErr := s.CreateUser("u@example.com", "U", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	pm, appErr := s.CreateAgent(Scope{UserID: user.ID}, "node-pm-001", "PM Agent", "pm", "pm", []string{"plan"})
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	project, appErr := s.CreateProject(Scope{UserID: user.ID}, "p", "desc", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	return s, user.ID, project.ID
}

func TestWorkflowTemplateCRUDAndVersion(t *testing.T) {
	s := New()
	userID := "u1"
	tpl, appErr := s.CreateWorkflowTemplate(Scope{UserID: userID}, " 产线模板 ", "", steps("剧本创作", "剧本解析"))
	if appErr != nil {
		t.Fatalf("create: %v", appErr)
	}
	if tpl.Name != "产线模板" {
		t.Errorf("name not trimmed: %q", tpl.Name)
	}
	if tpl.Version != 1 {
		t.Errorf("initial version = %d, want 1", tpl.Version)
	}

	// update bumps version
	updated, appErr := s.UpdateWorkflowTemplate(Scope{UserID: userID}, tpl.ID, nil, nil, steps("剧本创作", "剧本解析", "分镜拆解"))
	if appErr != nil {
		t.Fatalf("update: %v", appErr)
	}
	if updated.Version != 2 {
		t.Errorf("version after update = %d, want 2", updated.Version)
	}
	if len(updated.Steps) != 3 {
		t.Errorf("steps after update = %d, want 3", len(updated.Steps))
	}

	// list
	items := s.ListWorkflowTemplates(Scope{UserID: userID})
	if len(items) != 1 {
		t.Fatalf("list len = %d, want 1", len(items))
	}

	// other user cannot see / mutate
	if other := s.ListWorkflowTemplates(Scope{UserID: "u2"}); len(other) != 0 {
		t.Errorf("other user sees %d templates", len(other))
	}
	if _, appErr := s.GetWorkflowTemplate(Scope{UserID: "u2"}, tpl.ID); appErr == nil {
		t.Error("other user should not read template")
	}

	// copy creates a fresh v1
	copied, appErr := s.CopyWorkflowTemplate(Scope{UserID: userID}, tpl.ID)
	if appErr != nil {
		t.Fatalf("copy: %v", appErr)
	}
	if copied.Version != 1 || len(copied.Steps) != 3 {
		t.Errorf("copied version=%d steps=%d, want 1/3", copied.Version, len(copied.Steps))
	}
	if copied.ID == tpl.ID {
		t.Error("copy must have its own ID")
	}

	// delete
	if _, appErr := s.DeleteWorkflowTemplate(Scope{UserID: userID}, tpl.ID); appErr != nil {
		t.Fatalf("delete: %v", appErr)
	}
	if _, appErr := s.GetWorkflowTemplate(Scope{UserID: userID}, tpl.ID); appErr == nil {
		t.Error("deleted template still readable")
	}
}

func TestInheritWorkflowTemplate(t *testing.T) {
	s, userID, projectID := seedProjectWithPM(t)
	tpl, _ := s.CreateWorkflowTemplate(Scope{UserID: userID}, "产线模板", "", steps("A", "B", "C"))

	p2, appErr := s.InheritWorkflowTemplate(Scope{UserID: userID}, projectID, tpl.ID)
	if appErr != nil {
		t.Fatalf("inherit: %v", appErr)
	}
	if len(p2.Workflows) != 1 {
		t.Fatalf("workflows len = %d, want 1", len(p2.Workflows))
	}
	wf := p2.Workflows[0]
	if wf.ParentTemplateID != tpl.ID || wf.TemplateVersion != 1 {
		t.Errorf("inherit meta = %+v, want parent=%s version=1", wf, tpl.ID)
	}
	if wf.ID == "" {
		t.Error("inherited workflow must have ID")
	}
	if len(wf.TemplateSnapshot) != 3 {
		t.Errorf("snapshot len = %d, want 3", len(wf.TemplateSnapshot))
	}

	// cannot inherit from other user's template
	if _, appErr := s.InheritWorkflowTemplate(Scope{UserID: "u2"}, projectID, tpl.ID); appErr == nil {
		t.Error("other user should not inherit")
	}
}

func TestThreeWayMergeKinds(t *testing.T) {
	base := steps("a", "b", "c")
	// project edited "b" (kept name, changed role), kept a and c.
	ours := []model.WorkflowStep{
		steps("a")[0],
		{Name: "b", Role: "local_b"},
		steps("c")[0],
	}
	// template updated b (kept name), kept a, removed c, added d.
	theirs := []model.WorkflowStep{
		steps("a")[0],
		{Name: "b", Role: "tpl_b"},
		steps("d")[0],
	}

	changes := computeThreeWayMerge(base, ours, theirs)
	got := make(map[string]model.WorkflowSyncChangeKind)
	for _, ch := range changes {
		got[ch.Name] = ch.Kind
	}
	if got["a"] != model.WorkflowSyncUpdate {
		t.Errorf("a kind = %v, want update", got["a"])
	}
	if got["b"] != model.WorkflowSyncKeep {
		t.Errorf("b kind = %v, want keep (project modified)", got["b"])
	}
	if got["c"] != model.WorkflowSyncRemovePending {
		t.Errorf("c kind = %v, want remove_pending", got["c"])
	}
	if got["d"] != model.WorkflowSyncAdd {
		t.Errorf("d kind = %v, want add", got["d"])
	}
}

func TestApplyWorkflowSync(t *testing.T) {
	s, userID, projectID := seedProjectWithPM(t)
	tpl, _ := s.CreateWorkflowTemplate(Scope{UserID: userID}, "产线模板", "", steps("a", "b", "c"))
	inherited, _ := s.InheritWorkflowTemplate(Scope{UserID: userID}, projectID, tpl.ID)
	wf := inherited.Workflows[0]
	wfID := wf.ID

	// Project edits step b (keeps name, changes role); a/c unchanged.
	// Note: the saved workflow intentionally omits template_snapshot and
	// template_version — the backend must preserve the inheritance snapshot.
	localSteps := []model.WorkflowStep{
		steps("a")[0],
		{Name: "b", Role: "local_b"},
		steps("c")[0],
	}
	plainWF := model.Workflow{
		ID:               wf.ID,
		ParentTemplateID: wf.ParentTemplateID,
		Name:             wf.Name,
		Steps:            localSteps,
	}
	project2, appErr := s.UpdateProject(Scope{UserID: userID}, projectID, UpdateProjectInput{Workflows: []model.Workflow{plainWF}})
	if appErr != nil {
		t.Fatalf("update project: %v", appErr)
	}
	_ = project2

	// Template bumps to v2: updates b (same name), keeps a, removes c, adds d.
	s.UpdateWorkflowTemplate(Scope{UserID: userID}, tpl.ID, nil, nil, []model.WorkflowStep{
		steps("a")[0],
		{Name: "b", Role: "tpl_b"},
		steps("d")[0],
	})

	diff, appErr := s.ComputeWorkflowSyncDiff(Scope{UserID: userID}, projectID, wfID)
	if appErr != nil {
		t.Fatalf("diff: %v", appErr)
	}
	if diff.CurrentVersion != 2 || diff.ProjectVersion != 1 {
		t.Errorf("versions = %d/%d, want 2/1", diff.CurrentVersion, diff.ProjectVersion)
	}
	if len(diff.Changes) != 4 {
		t.Fatalf("changes len = %d, want 4 (a,b,c,d)", len(diff.Changes))
	}

	// Apply sync keeping c (remove_pending not confirmed).
	synced, appErr := s.ApplyWorkflowSync(Scope{UserID: userID}, projectID, wfID, nil)
	if appErr != nil {
		t.Fatalf("apply sync: %v", appErr)
	}
	syncedWF := synced.Workflows[0]
	if syncedWF.TemplateVersion != 2 {
		t.Errorf("template version = %d, want 2", syncedWF.TemplateVersion)
	}
	if len(syncedWF.Steps) != 4 {
		t.Fatalf("steps len = %d, want 4 (a,b_local,c,d)", len(syncedWF.Steps))
	}
	// b must remain project's local edit (keep)
	if syncedWF.Steps[1].Name != "b" || syncedWF.Steps[1].Role != "local_b" {
		t.Errorf("b = %+v, want local edit (project kept)", syncedWF.Steps[1])
	}
	// d must be added
	if syncedWF.Steps[3].Name != "d" {
		t.Errorf("last step = %q, want d", syncedWF.Steps[3].Name)
	}

	// Sync again: template v3 removes d (a fresh remove_pending) and adds e.
	// Confirm deletion of d.
	s.UpdateWorkflowTemplate(Scope{UserID: userID}, tpl.ID, nil, nil, []model.WorkflowStep{
		steps("a")[0],
		{Name: "b", Role: "tpl_b"},
		steps("e")[0],
	})
	synced2, appErr := s.ApplyWorkflowSync(Scope{UserID: userID}, projectID, wfID, []string{"d"})
	if appErr != nil {
		t.Fatalf("apply sync 2: %v", appErr)
	}
	synced2WF := synced2.Workflows[0]
	if synced2WF.TemplateVersion != 3 {
		t.Errorf("template version = %d, want 3", synced2WF.TemplateVersion)
	}
	if len(synced2WF.Steps) != 4 {
		t.Fatalf("steps len after remove = %d, want 4 (a,b_local,c,e)", len(synced2WF.Steps))
	}
	// d should be gone, e added
	for _, st := range synced2WF.Steps {
		if st.Name == "d" {
			t.Error("step d should have been removed")
		}
	}
	names := map[string]bool{}
	for _, st := range synced2WF.Steps {
		names[st.Name] = true
	}
	if !names["e"] {
		t.Error("step e should have been added")
	}
}

func TestDetachWorkflowFromTemplate(t *testing.T) {
	s, userID, projectID := seedProjectWithPM(t)
	tpl, _ := s.CreateWorkflowTemplate(Scope{UserID: userID}, "tpl", "", steps("a", "b"))
	inherited, _ := s.InheritWorkflowTemplate(Scope{UserID: userID}, projectID, tpl.ID)
	wfID := inherited.Workflows[0].ID

	detached, appErr := s.DetachWorkflowFromTemplate(Scope{UserID: userID}, projectID, wfID)
	if appErr != nil {
		t.Fatalf("detach: %v", appErr)
	}
	wf := detached.Workflows[0]
	if wf.ParentTemplateID != "" || wf.TemplateVersion != 0 || len(wf.TemplateSnapshot) != 0 {
		t.Errorf("after detach meta = parent=%q ver=%d snap=%d", wf.ParentTemplateID, wf.TemplateVersion, len(wf.TemplateSnapshot))
	}
	if _, appErr := s.ComputeWorkflowSyncDiff(Scope{UserID: userID}, projectID, wfID); appErr == nil {
		t.Error("detached workflow should not compute sync diff")
	}
}

func TestUpdateProjectAssignsWorkflowIDs(t *testing.T) {
	s, userID, projectID := seedProjectWithPM(t)

	// legacy workflow without ID gets an ID on save
	updated, appErr := s.UpdateProject(Scope{UserID: userID}, projectID, UpdateProjectInput{
		Workflows:            []model.Workflow{{Name: "wf", Steps: steps("a")}},
		PrimaryWorkflowIndex: ptrInt(0),
	})
	if appErr != nil {
		t.Fatalf("update: %v", appErr)
	}
	if updated.Workflows[0].ID == "" {
		t.Error("workflow should get an ID")
	}
	if updated.PrimaryWorkflowID != updated.Workflows[0].ID {
		t.Errorf("primary_workflow_id = %q, want %q", updated.PrimaryWorkflowID, updated.Workflows[0].ID)
	}
	if !hasPrimaryWorkflow(updated) {
		t.Error("hasPrimaryWorkflow should be true")
	}
	if primaryWorkflowUnsafe(updated).Name != "wf" {
		t.Error("primary workflow name mismatch")
	}
}

func ptrInt(v int) *int { return &v }
