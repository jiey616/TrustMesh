package model

import "testing"

func TestValidateStepInputRefsOK(t *testing.T) {
	steps := []WorkflowStep{
		{Name: "剧本创作", Outputs: []StepOutput{{Name: "剧本"}}},
		{Name: "分镜拆解", Inputs: []StepInput{
			{Name: "剧本", Source: StepIOLink{Step: "剧本创作", Output: "剧本"}},
		}},
		{Name: "资产制作", Inputs: []StepInput{
			{Name: "清单", Source: StepIOLink{Step: "prev"}},
		}},
	}
	if probs := ValidateStepInputRefs(steps); len(probs) != 0 {
		t.Fatalf("expected no problems, got %v", probs)
	}
}

func TestValidateStepInputRefsDanglingStep(t *testing.T) {
	// 复刻 2026-09-07 事故：来源步骤「剧本解析」不存在（实际叫「资产提取」）
	steps := []WorkflowStep{
		{Name: "资产提取"},
		{Name: "资产制作", Inputs: []StepInput{
			{Name: "角色清单", Source: StepIOLink{Step: "剧本解析", Output: "角色清单"}},
			{Name: "场景清单", Source: StepIOLink{Step: "剧本解析", Output: "场景清单"}},
		}},
	}
	probs := ValidateStepInputRefs(steps)
	if len(probs) != 2 {
		t.Fatalf("expected 2 problems, got %d: %v", len(probs), probs)
	}
	for _, p := range probs {
		if !contains(p, "剧本解析") {
			t.Fatalf("problem should mention the dangling step name: %q", p)
		}
	}
}

func TestValidateStepInputRefsEmptyStep(t *testing.T) {
	steps := []WorkflowStep{
		{Name: "A", Inputs: []StepInput{{Name: "x", Source: StepIOLink{Step: ""}}}},
	}
	if probs := ValidateStepInputRefs(steps); len(probs) != 1 {
		t.Fatalf("expected 1 problem for empty source.step, got %v", probs)
	}
}

func TestValidateStepInputRefsCaseInsensitive(t *testing.T) {
	steps := []WorkflowStep{
		{Name: "剧本创作"},
		{Name: "B", Inputs: []StepInput{{Name: "x", Source: StepIOLink{Step: " 剧本创作 "}}}},
	}
	if probs := ValidateStepInputRefs(steps); len(probs) != 0 {
		t.Fatalf("trimmed name should match, got %v", probs)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
