package model

import "strings"

// ValidateStepInputRefs 校验工作流步骤的输入声明（StepInput.Source）是否可解析：
//
//  1. source.step 必须非空；
//  2. source.step 为关键字 "prev"（紧邻上一步）时放行；
//  3. 其余情况 source.step 必须（大小写不敏感）匹配步骤列表中真实存在的步骤名。
//
// 背景（2026-09-07 事故）：全局模板 v10 的「资产制作」步骤把来源步骤写成了
// 不存在的「剧本解析」，派发时 FindTaskForWorkflowStep 找不到来源步骤，
// 三个输入全部 resolved:false，资产制作师拿不到上游交付物 download_url，
// 只能挂起或误用历史工作区残留。此校验在模板/项目工作流保存时拦截同类悬空引用。
//
// source.output 不在此校验（运行时按 todo.Outputs 的 outputName 匹配，
// 模板未声明上游 outputs 也可能通过交付物兜底解析成功，硬校验会误伤）。
//
// 返回问题描述切片（人类可读，含步骤名/输入名/来源步骤名）；空切片 = 通过。
func ValidateStepInputRefs(steps []WorkflowStep) []string {
	names := make(map[string]bool, len(steps))
	for i := range steps {
		names[strings.TrimSpace(steps[i].Name)] = true
	}
	var problems []string
	for i := range steps {
		for _, in := range steps[i].Inputs {
			src := strings.TrimSpace(in.Source.Step)
			inName := strings.TrimSpace(in.Name)
			switch {
			case src == "":
				problems = append(problems,
					"步骤「"+steps[i].Name+"」的输入「"+inName+"」未声明来源步骤（source.step 为空）")
			case strings.EqualFold(src, "prev"):
				// 关键字：紧邻上一步，放行
			case !names[src]:
				problems = append(problems,
					"步骤「"+steps[i].Name+"」的输入「"+inName+"」来源步骤「"+src+"」不存在于步骤列表")
			}
		}
	}
	return problems
}
