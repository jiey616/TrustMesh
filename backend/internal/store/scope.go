package store

import "trustmesh/backend/internal/model"

// Scope 是一次请求的归属上下文（阶段 0 只建模与注入，不参与业务判断）。
// 阶段 2 会把散落的 `x.UserID != userID` 收敛到这里的裁决函数。
type Scope struct {
	UserID string
	OrgID  string
	Role   string // owner | admin | member（自定义角色为 role_id，见 RoleID）
	// RoleID 是 org_roles 的角色引用（设计文档 §5 权限解析链的起点）：
	// 非空时权限集由该角色定义决定；为空（兼容期未迁移）才按 Role 字符串映射内置角色。
	RoleID string
	// System 标记内部系统路径（agent webhook / timeout_monitor / 后台定时器等
	// 没有用户会话的调用）。系统路径不做归属裁决，直接放行。
	//
	// 这是既有行为：改造前这些调用点一律传空 userID 绕过校验。阶段 2 收敛后
	// 不能让它们「碰巧」因为 UserID 为空而继续放行 —— 语义上「故意放行」与
	// 「忘了传 userID」必须可区分，否则等于给越权留后门。
	// 🔴 HTTP 路径永远拿不到 System Scope：currentScope 不会设置它。
	System bool
}

// HasOrg 判断本次请求是否带上了有效租户上下文。
// 阶段 0/1 存量账号没有 org 数据，此时 OrgID 为空，业务走原有 user 维度逻辑。
func (sc Scope) HasOrg() bool {
	return sc.OrgID != ""
}

// ownedByOrg 判断资源是否归属于当前租户。
// 无租户上下文（OrgID 为空）时返回 false：阶段 2 引入后，未回填 org_id 的
// 数据必须显式走兜底逻辑，不能默认放行。
func ownedByOrg(sc Scope, ownerOrgID string) bool {
	return sc.OrgID != "" && ownerOrgID == sc.OrgID
}

// ownedByUser 判断资源是否归属于当前用户（个人维度数据：已读态、偏好等）。
func ownedByUser(sc Scope, ownerUserID string) bool {
	return sc.UserID != "" && ownerUserID == sc.UserID
}

// visibleToScope 通用资源归属裁决（阶段 2 收敛 `x.UserID != userID` 的统一入口）：
//   - 有租户上下文：按 org 归属裁决（同租户成员共享资源）
//   - 无租户上下文：退回 user 维度，与改造前行为完全一致
//
// 阶段 1 已完成全量回填，存量资源都有 org_id；个别没回填到的资源
// （ownerOrgID 为空）在无租户上下文时仍能按 user 维度兜底命中。
func visibleToScope(sc Scope, ownerOrgID, ownerUserID string) bool {
	// 内部系统路径（无用户会话）：不做归属裁决，与改造前传 "" 的行为一致。
	if sc.System {
		return true
	}
	// 只有「请求带租户上下文」且「资源已回填 org」时才走 org 裁决；
	// 其余一律退回 user 维度，保证无租户头的存量客户端零回归。
	if sc.HasOrg() && ownerOrgID != "" {
		return ownedByOrg(sc, ownerOrgID)
	}
	return ownedByUser(sc, ownerUserID)
}

// resolveOwnerOrgUnsafe 决定新建资源挂到哪个租户（调用方必须持锁）：
//   - 请求带租户上下文（用户在企业租户里操作）→ 挂到当前活跃租户
//   - 否则 → 挂到作者的个人租户（阶段 1 兜底，失败则返回空串不阻断写入）
func (s *Store) resolveOwnerOrgUnsafe(sc Scope) string {
	if sc.HasOrg() {
		return sc.OrgID
	}
	return s.personalOrgOfUnsafe(sc.UserID)
}

// SystemScope 返回内部系统路径使用的归属上下文。
// 用于 agent webhook、timeout_monitor、内部定时器等没有用户会话的调用点 ——
// 它们改造前一律传空 userID 绕过校验，语义上就是系统旁路。
// 🔴 只能由 store 包内部与 clawsynapse webhook 构造，HTTP handler 禁止使用。
func SystemScope() Scope {
	return Scope{System: true}
}

// meetingVisible 会议可见性裁决。
// 会议挂在项目下，优先走项目裁决以继承「项目成员白名单」；项目不在内存
// （懒加载边界）时退回会议自身归属 —— 数据缺失时宁可漏、不可泄。
func (s *Store) meetingVisible(sc Scope, m *model.Meeting) bool {
	if m == nil {
		return false
	}
	if sc.System {
		return true
	}
	if m.ProjectID != "" {
		if p, ok := s.projects[m.ProjectID]; ok {
			return s.projectVisible(sc, p)
		}
	}
	return visibleToScope(sc, m.OrgID, m.CreatorID)
}

// projectVisible 项目可见性裁决：
//   - 无租户上下文：退回 user 维度判断（阶段 0 的唯一生效路径）
//   - 有租户上下文：先看 org 归属；私有项目（有成员白名单）要求成员命中
func (s *Store) projectVisible(sc Scope, p *model.Project) bool {
	if p == nil {
		return false
	}
	// 内部系统路径：不做归属裁决（见 Scope.System 注释）。
	if sc.System {
		return true
	}
	// 无租户上下文（存量客户端 / 内部路径）：一律 user 维度，与改造前完全一致。
	if !sc.HasOrg() {
		return ownedByUser(sc, p.UserID)
	}
	// 有租户上下文：资源未回填 org 时退回 user 维度兜底，绝不因数据缺失而失联。
	if p.OrgID == "" {
		return ownedByUser(sc, p.UserID)
	}
	if !ownedByOrg(sc, p.OrgID) {
		return false
	}
	members := s.projectMembers[p.ID]
	if len(members) == 0 {
		return true // 未设成员 → 全员可见
	}
	for _, m := range members {
		if m.UserID == sc.UserID {
			return true
		}
	}
	return false
}

// agentCanWriteTaskUnsafe 节点回写路径（todo.progress/complete/fail/ask、
// task.comment、artifact 归档等）的任务归属裁决。webhook 调用没有 HTTP
// 租户上下文，不能直接用 visibleToScope，需要按 agent 自身身份裁决。
// 放行条件（任一满足）：
//  1. agent 与任务同属一个用户（存量行为，零回归兜底）；
//  2. agent 显式归属任务所在租户（agent.org_id == task.org_id）；
//  3. agent 的属主用户是任务租户（org）的成员 —— 支撑「同租户跨用户协作」：
//     org 内任何成员创建的任务派给 org 共享的 agent 后，agent 回写不再因
//     task.UserID != agent.UserID 被 404 弹回（2026-09-11 实锤：山雨账号
//     建任务，编剧全部回报被 webhook 404 吞掉，前端零显示）。
//
// 任务未回填 org_id 时仅条件 1 生效，与改造前行为一致（宁可漏、不可泄）。
// 调用方必须持锁（findMembershipUnsafe 为 Unsafe 层）。
func (s *Store) agentCanWriteTaskUnsafe(task *model.TaskDetail, agent *model.Agent) bool {
	if task == nil || agent == nil {
		return false
	}
	if task.UserID == agent.UserID {
		return true
	}
	if task.OrgID == "" {
		return false
	}
	if agent.OrgID == task.OrgID {
		return true
	}
	_, ok := s.findMembershipUnsafe(task.OrgID, agent.UserID)
	return ok
}

// taskVisibleInProjectUnsafe 判断任务是否归属于某个项目（资源间归属关系）。
//
// ⚠️ 与访问裁决 visibleToScope 的区别：visibleToScope 回答的是
// "当前请求能否看到这个资源"（入参是 Scope），而这里回答的是
// "这个任务算不算这个项目的任务"（入参是资源本身）。项目聚合统计、
// 归档重置等场景需要的是后者 —— 用 visibleToScope 替换会把它变成
// "请求者能否看到这个任务"，语义不同且会把 Scope 透传到 7 个调用点。
//
// 修复的问题：企业共享项目下，成员 B 建的任务因
// task.UserID(B) != project.UserID(A) 被排除，导致项目任务统计/归档
// 静默漏掉同租户其他成员创建的任务。
//
// 🔴 空 org 短路：存量个人项目/任务（OrgID 为空）只按 user 严格相等，
// 避免 空org == 空org 被误判为同租户而把他人任务算进来。
// 调用方必须持锁（命名沿用 Unsafe 约定）。
func taskVisibleInProjectUnsafe(task *model.TaskDetail, project *model.Project) bool {
	if task == nil || project == nil {
		return false
	}
	if task.UserID == project.UserID {
		return true
	}
	if task.OrgID == "" || project.OrgID == "" {
		return false
	}
	return task.OrgID == project.OrgID
}

// agentCanAssignableToProjectUnsafe 判断一个 agent 是否可以被指派到该项目下
// 创建的任务（创建期校验）。
//
// 为什么不用 agentCanWriteTaskUnsafe：后者签名是
// (task *model.TaskDetail, agent *model.Agent)，要求 task 已存在；
// 而 CreateTaskByPMNode 此刻只有 project、task 尚未创建，签名不成立。
// 这里是它的 project 维度镜像，放行条件与之一致（三选一）：
//  1. agent 与项目同属一个用户（存量行为，零回归兜底）；
//  2. agent 显式归属项目所在租户（agent.OrgID == project.OrgID）；
//  3. agent 的属主用户是项目租户的成员 —— 支撑「同租户跨用户协作」。
//
// 🔴 空 org 短路（v2 关键修正）：存量个人项目 project.OrgID 为空时，
// 只走条件 1（严格 user 相等），绝不进入 org 匹配。否则 空org == 空org
// 会被判为同租户，导致任意 agent 都能指派到存量个人项目 —— 即全量
// 跨用户越权放开。
//
// 调用方必须持锁（findMembershipUnsafe 为 Unsafe 层）。
func (s *Store) agentCanAssignableToProjectUnsafe(agent *model.Agent, project *model.Project) bool {
	if agent == nil || project == nil {
		return false
	}
	if agent.UserID == project.UserID {
		return true
	}
	// 🔴 空 org 短路：存量个人项目不做 org 匹配，仅按 user 严格相等。
	if project.OrgID == "" {
		return false
	}
	if agent.OrgID == project.OrgID {
		return true
	}
	_, ok := s.findMembershipUnsafe(project.OrgID, agent.UserID)
	return ok
}
