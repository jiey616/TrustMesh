package store

import (
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// workflowStepsInputErr 将 ValidateStepInputRefs 的结果打包为 400 校验错误。
// steps 合法时返回 nil。wfLabel 用于错误信息里标明是哪个工作流（模板名/项目工作流名）。
func workflowStepsInputErr(wfLabel string, steps []model.WorkflowStep) *transport.AppError {
	problems := model.ValidateStepInputRefs(steps)
	if len(problems) == 0 {
		return nil
	}
	return transport.Validation("工作流步骤输入声明引用了不存在的来源步骤", map[string]any{
		"workflow": wfLabel,
		"problems": problems,
		"hint":     "inputs[].source.step 必须是步骤列表中已有的步骤名（或 prev）；上游步骤改名时须同步修改下游步骤的输入来源",
	})
}
