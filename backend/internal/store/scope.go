package store

import "trustmesh/backend/internal/model"

// Scope 是一次请求的归属上下文（阶段 0 只建模与注入，不参与业务判断）。
// 阶段 2 会把散落的 `x.UserID != userID` 收敛到这里的裁决函数。
type Scope struct {
	UserID string
	OrgID  string
	Role   string // owner | admin | member
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

// projectVisible 项目可见性裁决：
//   - 无租户上下文：退回 user 维度判断（阶段 0 的唯一生效路径）
//   - 有租户上下文：先看 org 归属；私有项目（有成员白名单）要求成员命中
func (s *Store) projectVisible(sc Scope, p *model.Project) bool {
	if p == nil {
		return false
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
