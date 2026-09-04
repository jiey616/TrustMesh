package store

import (
	"strings"
	"time"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// ---------- 全局工作流模板 CRUD ----------

func (s *Store) CreateWorkflowTemplate(userID, name, description string, steps []model.WorkflowStep) (*model.WorkflowTemplate, *transport.AppError) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, transport.Validation("invalid workflow template", map[string]any{"name": "required"})
	}
	now := time.Now().UTC()
	doc := &model.WorkflowTemplate{
		ID:          "wt_" + newID(),
		UserID:      userID,
		Name:        name,
		Description: strings.TrimSpace(description),
		Steps:       cloneSteps(steps),
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	s.mu.Lock()
	doc.OrgID = s.personalOrgOfUnsafe(userID)

	defer s.mu.Unlock()
	s.workflowTemplates[doc.ID] = doc
	s.userWorkflowTemplates[userID] = append(s.userWorkflowTemplates[userID], doc.ID)
	if err := s.persistWorkflowTemplateUnsafe(doc); err != nil {
		return nil, mongoWriteError(err)
	}
	return cloneWorkflowTemplate(doc), nil
}

// ListWorkflowTemplates returns the user's global workflow templates,
// newest first.
func (s *Store) ListWorkflowTemplates(userID string) []*model.WorkflowTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.userWorkflowTemplates[userID]
	out := make([]*model.WorkflowTemplate, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		if t, ok := s.workflowTemplates[ids[i]]; ok {
			out = append(out, cloneWorkflowTemplate(t))
		}
	}
	return out
}

func (s *Store) GetWorkflowTemplate(userID, templateID string) (*model.WorkflowTemplate, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.workflowTemplates[templateID]
	if !ok {
		return nil, transport.NotFound("workflow template not found")
	}
	if t.UserID != userID {
		return nil, transport.Forbidden("access denied")
	}
	return cloneWorkflowTemplate(t), nil
}

// UpdateWorkflowTemplate updates a template and bumps its version on every save.
func (s *Store) UpdateWorkflowTemplate(userID, templateID string, name, description *string, steps []model.WorkflowStep) (*model.WorkflowTemplate, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.workflowTemplates[templateID]
	if !ok {
		return nil, transport.NotFound("workflow template not found")
	}
	if t.UserID != userID {
		return nil, transport.Forbidden("access denied")
	}
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return nil, transport.Validation("invalid name", map[string]any{"name": "cannot be empty"})
		}
		t.Name = n
	}
	if description != nil {
		t.Description = strings.TrimSpace(*description)
	}
	if steps != nil {
		t.Steps = cloneSteps(steps)
	}
	t.Version++
	t.UpdatedAt = time.Now().UTC()
	if err := s.persistWorkflowTemplateUnsafe(t); err != nil {
		return nil, mongoWriteError(err)
	}
	return cloneWorkflowTemplate(t), nil
}

// CopyWorkflowTemplate duplicates the user's own template as a fresh v1.
func (s *Store) CopyWorkflowTemplate(userID, templateID string) (*model.WorkflowTemplate, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src, ok := s.workflowTemplates[templateID]
	if !ok {
		return nil, transport.NotFound("workflow template not found")
	}
	if src.UserID != userID {
		return nil, transport.Forbidden("access denied")
	}
	now := time.Now().UTC()
	doc := &model.WorkflowTemplate{
		ID:          "wt_" + newID(),
		UserID:      userID,
		OrgID:      s.personalOrgOfUnsafe(userID),
		Name:        src.Name + "（副本）",
		Description: src.Description,
		Steps:       cloneSteps(src.Steps),
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.workflowTemplates[doc.ID] = doc
	s.userWorkflowTemplates[userID] = append(s.userWorkflowTemplates[userID], doc.ID)
	if err := s.persistWorkflowTemplateUnsafe(doc); err != nil {
		return nil, mongoWriteError(err)
	}
	return cloneWorkflowTemplate(doc), nil
}

func (s *Store) DeleteWorkflowTemplate(userID, templateID string) (*model.WorkflowTemplate, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.workflowTemplates[templateID]
	if !ok {
		return nil, transport.NotFound("workflow template not found")
	}
	if t.UserID != userID {
		return nil, transport.Forbidden("access denied")
	}
	delete(s.workflowTemplates, templateID)
	ids := s.userWorkflowTemplates[userID]
	for i, id := range ids {
		if id == templateID {
			s.userWorkflowTemplates[userID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	_ = s.deleteWorkflowTemplateUnsafe(templateID)
	return cloneWorkflowTemplate(t), nil
}

// ---------- 项目继承 / 同步 / 解除 ----------

// InheritWorkflowTemplate clones a global template into the project as a new
// workflow (with inheritance metadata) so the project can customize it.
func (s *Store) InheritWorkflowTemplate(userID, projectID, templateID string) (*model.Project, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[projectID]
	if !ok || p.UserID != userID {
		return nil, transport.NotFound("project not found")
	}
	t, ok := s.workflowTemplates[templateID]
	if !ok || t.UserID != userID {
		return nil, transport.NotFound("workflow template not found")
	}

	steps := cloneSteps(t.Steps)
	wf := model.Workflow{
		ID:                "wf_" + newID(),
		ParentTemplateID:  t.ID,
		TemplateVersion:   t.Version,
		TemplateSnapshot:  cloneSteps(t.Steps),
		Name:              t.Name,
		Steps:             steps,
	}
	p.Workflows = append(p.Workflows, wf)
	p.UpdatedAt = time.Now().UTC()
	if err := s.persistProjectUnsafe(p); err != nil {
		return nil, mongoWriteError(err)
	}
	return s.buildProjectViewUnsafe(p), nil
}

// ComputeWorkflowSyncDiff produces a three-way merge preview of a global
// template update against the project's inherited workflow.
// base = template snapshot at inherit/last-sync, ours = project current,
// theirs = template latest.
func (s *Store) ComputeWorkflowSyncDiff(userID, projectID, workflowID string) (*model.WorkflowSyncDiff, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[projectID]
	if !ok || p.UserID != userID {
		return nil, transport.NotFound("project not found")
	}
	wf := findWorkflowByID(p.Workflows, workflowID)
	if wf == nil {
		return nil, transport.NotFound("workflow not found in project")
	}
	if wf.ParentTemplateID == "" {
		return nil, transport.BadRequest("BAD_REQUEST", "workflow is not inherited from a global template")
	}
	t, ok := s.workflowTemplates[wf.ParentTemplateID]
	if !ok || t.UserID != userID {
		return nil, transport.NotFound("parent workflow template not found")
	}
	if t.Version <= wf.TemplateVersion {
		return &model.WorkflowSyncDiff{
			TemplateID:     t.ID,
			TemplateName:   t.Name,
			CurrentVersion: t.Version,
			ProjectVersion: wf.TemplateVersion,
			Changes:        []model.WorkflowSyncChange{},
		}, nil
	}

	diff := computeThreeWayMerge(wf.TemplateSnapshot, wf.Steps, t.Steps)
	return &model.WorkflowSyncDiff{
		TemplateID:     t.ID,
		TemplateName:   t.Name,
		CurrentVersion: t.Version,
		ProjectVersion: wf.TemplateVersion,
		Changes:        diff,
	}, nil
}

// ApplyWorkflowSync applies a confirmed template sync to the project's
// inherited workflow. removeSteps lists step names marked remove_pending that
// the user decided to drop. On success the workflow's TemplateSnapshot and
// TemplateVersion advance to the template's latest.
func (s *Store) ApplyWorkflowSync(userID, projectID, workflowID string, removeSteps []string) (*model.Project, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[projectID]
	if !ok || p.UserID != userID {
		return nil, transport.NotFound("project not found")
	}
	idx := findWorkflowIndexByID(p.Workflows, workflowID)
	if idx < 0 {
		return nil, transport.NotFound("workflow not found in project")
	}
	wf := &p.Workflows[idx]
	if wf.ParentTemplateID == "" {
		return nil, transport.BadRequest("BAD_REQUEST", "workflow is not inherited from a global template")
	}
	t, ok := s.workflowTemplates[wf.ParentTemplateID]
	if !ok || t.UserID != userID {
		return nil, transport.NotFound("parent workflow template not found")
	}
	if t.Version <= wf.TemplateVersion {
		return nil, transport.BadRequest("BAD_REQUEST", "workflow is already up to date")
	}

	removeSet := make(map[string]bool, len(removeSteps))
	for _, name := range removeSteps {
		removeSet[name] = true
	}

	changes := computeThreeWayMerge(wf.TemplateSnapshot, wf.Steps, t.Steps)
	byName := make(map[string]model.WorkflowSyncChange, len(changes))
	for _, ch := range changes {
		byName[ch.Name] = ch
	}
	oursByName := make(map[string]*model.WorkflowStep, len(wf.Steps))
	for i := range wf.Steps {
		oursByName[wf.Steps[i].Name] = &wf.Steps[i]
	}
	theirsByName := make(map[string]*model.WorkflowStep, len(t.Steps))
	for i := range t.Steps {
		theirsByName[t.Steps[i].Name] = &t.Steps[i]
	}
	baseByName := make(map[string]bool, len(wf.TemplateSnapshot))
	for _, st := range wf.TemplateSnapshot {
		baseByName[st.Name] = true
	}

	// Result follows the project's current step order: template updates
	// overwrite unmodified steps, confirmed remove_pending steps are dropped,
	// local/kept steps stay as-is.
	var result []model.WorkflowStep
	for _, o := range wf.Steps {
		ch, known := byName[o.Name]
		if known && ch.Kind == model.WorkflowSyncRemovePending && removeSet[o.Name] {
			continue // user confirmed template deletion
		}
		if known && ch.Kind == model.WorkflowSyncUpdate {
			if theirs := theirsByName[o.Name]; theirs != nil {
				result = append(result, *theirs.Clone())
				continue
			}
		}
		result = append(result, *o.Clone())
	}
	// Append genuinely-new template steps (absent from both project and base).
	for _, th := range t.Steps {
		if _, inOurs := oursByName[th.Name]; inOurs {
			continue
		}
		if _, inBase := baseByName[th.Name]; inBase {
			continue // project deliberately removed it → respect project
		}
		result = append(result, *th.Clone())
	}

	wf.Steps = result
	wf.TemplateSnapshot = cloneSteps(t.Steps)
	wf.TemplateVersion = t.Version
	p.UpdatedAt = time.Now().UTC()
	if err := s.persistProjectUnsafe(p); err != nil {
		return nil, mongoWriteError(err)
	}
	return s.buildProjectViewUnsafe(p), nil
}

// DetachWorkflowFromTemplate breaks the inheritance link, turning the workflow
// into a plain private workflow (irreversible).
func (s *Store) DetachWorkflowFromTemplate(userID, projectID, workflowID string) (*model.Project, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[projectID]
	if !ok || p.UserID != userID {
		return nil, transport.NotFound("project not found")
	}
	idx := findWorkflowIndexByID(p.Workflows, workflowID)
	if idx < 0 {
		return nil, transport.NotFound("workflow not found in project")
	}
	wf := &p.Workflows[idx]
	if wf.ParentTemplateID == "" {
		return nil, transport.BadRequest("BAD_REQUEST", "workflow is not inherited from a global template")
	}
	wf.ParentTemplateID = ""
	wf.TemplateVersion = 0
	wf.TemplateSnapshot = nil
	p.UpdatedAt = time.Now().UTC()
	if err := s.persistProjectUnsafe(p); err != nil {
		return nil, mongoWriteError(err)
	}
	return s.buildProjectViewUnsafe(p), nil
}

// ---------- 三路合并 ----------

// computeThreeWayMerge classifies each step by name across base (template
// snapshot), ours (project current) and theirs (template latest).
func computeThreeWayMerge(base, ours, theirs []model.WorkflowStep) []model.WorkflowSyncChange {
	baseByName := stepNameMap(base)
	oursByName := stepNameMap(ours)
	theirsByName := stepNameMap(theirs)

	var changes []model.WorkflowSyncChange
	for _, t := range theirs {
		b, inBase := baseByName[t.Name]
		o, inOurs := oursByName[t.Name]
		switch {
		case !inBase && !inOurs:
			changes = append(changes, model.WorkflowSyncChange{Name: t.Name, Kind: model.WorkflowSyncAdd})
		case inBase && inOurs:
			if stepsEqual(*o, *b) {
				changes = append(changes, model.WorkflowSyncChange{Name: t.Name, Kind: model.WorkflowSyncUpdate})
			} else {
				changes = append(changes, model.WorkflowSyncChange{Name: t.Name, Kind: model.WorkflowSyncKeep, ProjectModified: true})
			}
		case !inBase && inOurs:
			// project added a step sharing the template's new name: keep ours
			changes = append(changes, model.WorkflowSyncChange{Name: t.Name, Kind: model.WorkflowSyncKeepProject, ProjectModified: true})
		default: // inBase && !inOurs: project deleted it; template still has it → respect project
			changes = append(changes, model.WorkflowSyncChange{Name: t.Name, Kind: model.WorkflowSyncKeepProject})
		}
	}
	// Steps the template removed that the project still has → pending removal.
	for _, o := range ours {
		if _, inTheirs := theirsByName[o.Name]; !inTheirs {
			if _, inBase := baseByName[o.Name]; inBase {
				changes = append(changes, model.WorkflowSyncChange{Name: o.Name, Kind: model.WorkflowSyncRemovePending})
			}
		}
	}
	return changes
}

func stepNameMap(steps []model.WorkflowStep) map[string]*model.WorkflowStep {
	m := make(map[string]*model.WorkflowStep, len(steps))
	for i := range steps {
		m[steps[i].Name] = &steps[i]
	}
	return m
}

func stepsEqual(a, b model.WorkflowStep) bool {
	if a.Name != b.Name || a.Role != b.Role || a.AgentID != b.AgentID || a.NeedReview != b.NeedReview {
		return false
	}
	if !stepInputsEqual(a.Inputs, b.Inputs) || !stepOutputsEqual(a.Outputs, b.Outputs) {
		return false
	}
	return true
}

func stepInputsEqual(a, b []model.StepInput) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stepOutputsEqual(a, b []model.StepOutput) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// cloneSteps deep-copies a step slice.
func cloneSteps(steps []model.WorkflowStep) []model.WorkflowStep {
	if steps == nil {
		return nil
	}
	out := make([]model.WorkflowStep, len(steps))
	for i := range steps {
		out[i] = *steps[i].Clone()
	}
	return out
}

func cloneWorkflowTemplate(t *model.WorkflowTemplate) *model.WorkflowTemplate {
	if t == nil {
		return nil
	}
	c := *t
	c.Steps = cloneSteps(t.Steps)
	return &c
}

func findWorkflowByID(workflows []model.Workflow, id string) *model.Workflow {
	for i := range workflows {
		if workflows[i].ID == id {
			return &workflows[i]
		}
	}
	return nil
}

func findWorkflowIndexByID(workflows []model.Workflow, id string) int {
	for i := range workflows {
		if workflows[i].ID == id {
			return i
		}
	}
	return -1
}

// hasPrimaryWorkflow reports whether the project has a resolvable primary
// workflow (ID reference preferred, index fallback for legacy data).
func hasPrimaryWorkflow(p *model.Project) bool {
	if p == nil {
		return false
	}
	if p.PrimaryWorkflowID != "" {
		return findWorkflowByID(p.Workflows, p.PrimaryWorkflowID) != nil
	}
	return p.PrimaryWorkflowIndex >= 0 && p.PrimaryWorkflowIndex < len(p.Workflows)
}

// primaryWorkflowUnsafe returns the project's primary workflow. Callers must
// call hasPrimaryWorkflow first.
func primaryWorkflowUnsafe(p *model.Project) model.Workflow {
	if p.PrimaryWorkflowID != "" {
		if wf := findWorkflowByID(p.Workflows, p.PrimaryWorkflowID); wf != nil {
			return *wf
		}
	}
	return p.Workflows[p.PrimaryWorkflowIndex]
}
