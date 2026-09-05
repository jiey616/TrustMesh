package store

import (
	"sort"
	"strings"
	"time"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

type CreateJoinRequestInput struct {
	TrustRequestID string
	UserID         string
	OrgID          string
	NodeID         string
	Name           string
	Description    string
	Role           string
	Capabilities   []string
	AgentProduct   string
	ReceivedAt     time.Time
}

type JoinRequestOverrides struct {
	Name         *string  `json:"name,omitempty"`
	Role         *string  `json:"role,omitempty"`
	Description  *string  `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// defaultProduct 归一化产品标识：空值回落为平台原生 "trustmesh"。
// 产品标识来源为 JoinRequest.AgentProduct（审批时同步），不预设白名单。
func defaultProduct(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "trustmesh"
	}
	return p
}

// HasTrustRequest checks if a trust request ID has already been processed (lock-free read).
func (s *Store) HasTrustRequest(trustRequestID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.trustRequestIndex[trustRequestID]
	return exists
}

func (s *Store) CreateJoinRequest(in CreateJoinRequestInput) (*model.JoinRequest, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Dedup by trust request ID
	if _, exists := s.trustRequestIndex[in.TrustRequestID]; exists {
		return nil, transport.Conflict("JOIN_REQUEST_EXISTS", "join request already exists for this trust request")
	}

	// Check if node is already registered as an agent
	if _, exists := s.agentByNode[in.NodeID]; exists {
		return nil, transport.Conflict("AGENT_NODE_ID_EXISTS", "node_id already registered as an agent")
	}

	// Check if there's already a pending request from this node
	for _, jr := range s.joinRequests {
		if jr.NodeID == in.NodeID && jr.Status == "pending" {
			return nil, transport.Conflict("JOIN_REQUEST_PENDING", "a pending join request already exists for this node")
		}
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = in.NodeID
	}
	role := strings.TrimSpace(in.Role)
	if role == "" || !isValidRole(role) {
		role = "custom"
	}

	now := in.ReceivedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// Resolve user_id: use provided user_id if valid, otherwise fall back to all users
	userID := strings.TrimSpace(in.UserID)
	if userID != "" {
		if _, exists := s.users[userID]; !exists {
			userID = ""
		}
	}

	// 发起时锁定归属：reason 携带 org_id（企业空间发起招聘）时，校验
	// 「org 存在且为企业租户 && 邀请人是该 org 成员」，通过则申请挂该企业；
	// 校验不过回落邀请人个人租户（宁漏不泄，不放大可见范围）。
	orgID := strings.TrimSpace(in.OrgID)
	lockedToEnterprise := false
	if orgID != "" && userID != "" {
		if org, ok := s.organizations[orgID]; ok && org.Kind == model.OrgKindEnterprise {
			if _, isMember := s.findMembershipUnsafe(orgID, userID); isMember {
				lockedToEnterprise = true
			} else {
				orgID = ""
			}
		} else {
			orgID = ""
		}
	}
	ownerOrgID := s.personalOrgOfUnsafe(userID)
	if lockedToEnterprise {
		ownerOrgID = orgID
	}

	jr := &model.JoinRequest{
		ID:             newID(),
		UserID:         userID,
		OrgID:          ownerOrgID,
		TrustRequestID: in.TrustRequestID,
		NodeID:         in.NodeID,
		Name:           name,
		Description:    strings.TrimSpace(in.Description),
		Role:           role,
		Capabilities:   normalizeCapabilities(in.Capabilities),
		AgentProduct:   strings.TrimSpace(in.AgentProduct),
		Status:         "pending",
		Metadata:       map[string]any{},
		CreatedAt:      now,
	}

	s.joinRequests[jr.ID] = jr
	s.trustRequestIndex[in.TrustRequestID] = jr.ID
	if err := s.persistJoinRequestUnsafe(jr); err != nil {
		return nil, mongoWriteError(err)
	}

	if userID != "" {
		// Associate with the specific user who generated the invite
		s.userJoinRequests[userID] = append(s.userJoinRequests[userID], jr.ID)
	} else {
		// No valid user_id — associate with all users as fallback
		for _, u := range s.users {
			s.userJoinRequests[u.ID] = append(s.userJoinRequests[u.ID], jr.ID)
		}
	}

	// Notify relevant users：企业锁定申请通知全企业成员；个人申请通知邀请人；
	// 无主申请广播全部用户（既有 fallback）。
	notifyUsers := make([]string, 0)
	switch {
	case lockedToEnterprise:
		for _, mid := range s.orgMemberIndex[orgID] {
			if m, ok := s.orgMemberships[mid]; ok {
				notifyUsers = append(notifyUsers, m.UserID)
			}
		}
	case userID != "":
		notifyUsers = append(notifyUsers, userID)
	default:
		for _, u := range s.users {
			notifyUsers = append(notifyUsers, u.ID)
		}
	}
	for _, uid := range notifyUsers {
		content := "数字员工「" + jr.Name + "」申请加入平台"
		event := &model.Event{
			ID:        newID(),
			UserID:    uid,
			EventType: "join_request_received",
			ActorType: "agent",
			ActorID:   jr.NodeID,
			ActorName: jr.Name,
			Content:   &content,
			Metadata:  map[string]any{"join_request_id": jr.ID, "node_id": jr.NodeID},
			CreatedAt: now,
		}
		s.maybeCreateNotificationUnsafe(event)
		s.publishUserEventUnsafe(uid, "join_request.created", map[string]any{
			"join_request": *jr,
		}, now)
	}

	return copyJoinRequest(jr), nil
}

// joinRequestVisible 加入申请可见性裁决。
// 申请由 agent 通过 trust_sync 提交（无用户会话），归属 = 邀请码生成者及其租户。
// 用户拍板：审批可见范围为「仅同租户」。
func (s *Store) joinRequestVisible(sc Scope, jr *model.JoinRequest) bool {
	if jr == nil {
		return false
	}
	if sc.System {
		return true
	}
	// 无主申请（agent 未带邀请人，UserID 与 OrgID 皆空）：既有行为是广播给
	// 所有用户可见（CreateJoinRequest 的 fallback 分支），保持零回归。
	// ⚠️ 已知敞口：这类申请跨租户可见，待阶段 3 节点 org 绑定后彻底收口。
	if jr.UserID == "" && jr.OrgID == "" {
		return sc.UserID != ""
	}
	// 邀请人本人永远可见：申请由 agent 内部路径创建（无租户上下文），恒挂邀请人
	// 个人租户；若邀请人此时切到企业租户审批，按 org 比对会看不到自己的邀请。
	// 自己的数据对自己可见不构成越权。
	if jr.UserID != "" && jr.UserID == sc.UserID {
		return true
	}
	return visibleToScope(sc, jr.OrgID, jr.UserID)
}

func (s *Store) ListJoinRequests(sc Scope, status string) []model.JoinRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 带租户上下文时，user 分区索引只覆盖邀请人本人的申请，
	// 必须全量扫描按归属裁决，否则同租户成员看不到别人邀请的申请。
	// 无租户上下文：走分区索引，与改造前完全一致。
	if sc.HasOrg() {
		items := make([]model.JoinRequest, 0)
		for _, jr := range s.joinRequests {
			if status != "" && jr.Status != status {
				continue
			}
			if !s.joinRequestVisible(sc, jr) {
				continue
			}
			items = append(items, *copyJoinRequest(jr))
		}
		sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
		return items
	}

	ids := s.userJoinRequests[sc.UserID]
	items := make([]model.JoinRequest, 0, len(ids))
	for _, id := range ids {
		jr, ok := s.joinRequests[id]
		if !ok {
			continue
		}
		if status != "" && jr.Status != status {
			continue
		}
		items = append(items, *copyJoinRequest(jr))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}

func (s *Store) GetJoinRequest(sc Scope, requestID string) (*model.JoinRequest, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jr, ok := s.joinRequests[requestID]
	if !ok || !s.joinRequestVisible(sc, jr) {
		return nil, transport.NotFound("join request not found")
	}
	return copyJoinRequest(jr), nil
}

func (s *Store) ApproveJoinRequest(sc Scope, requestID string, overrides JoinRequestOverrides) (*model.Agent, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jr, ok := s.joinRequests[requestID]
	if !ok || !s.joinRequestVisible(sc, jr) {
		return nil, transport.NotFound("join request not found")
	}
	if jr.Status != "pending" {
		return nil, transport.Validation("join request is not pending", map[string]any{"status": jr.Status})
	}

	// Check node_id not taken by active agent
	if _, exists := s.agentByNode[jr.NodeID]; exists {
		return nil, transport.Conflict("AGENT_NODE_ID_EXISTS", "node_id already registered as an agent")
	}

	// 发起时锁定语义：
	//   - 企业锁定申请（发起招聘时 reason 带 org_id）：仅该企业 owner/admin 可审；
	//     agent 归属继承申请归属（该企业 + 邀请人）。
	//   - 个人申请：可见即可审（joinRequestVisible 已裁决），归属继承申请（邀请人个人租户）。
	//   - 无主申请（UserID 空的广播 fallback）：保持审批者归属兜底。
	inviterID := jr.UserID
	enterpriseLocked := s.isEnterpriseOrgUnsafe(jr.OrgID)
	var agentUserID, agentOrgID string
	switch {
	case inviterID == "":
		agentUserID = sc.UserID
		agentOrgID = s.resolveOwnerOrgUnsafe(sc)
	case enterpriseLocked:
		if !sc.HasOrg() || sc.OrgID != jr.OrgID || (sc.Role != model.OrgRoleOwner && sc.Role != model.OrgRoleAdmin) {
			return nil, transport.Forbidden("only organization owner/admin can approve this join request")
		}
		agentUserID = inviterID
		agentOrgID = jr.OrgID
	default:
		agentUserID = inviterID
		agentOrgID = jr.OrgID
	}

	// Apply overrides
	name := jr.Name
	if overrides.Name != nil && strings.TrimSpace(*overrides.Name) != "" {
		name = strings.TrimSpace(*overrides.Name)
	}
	role := jr.Role
	if overrides.Role != nil && isValidRole(strings.TrimSpace(*overrides.Role)) {
		role = strings.TrimSpace(*overrides.Role)
	}
	description := jr.Description
	if overrides.Description != nil && strings.TrimSpace(*overrides.Description) != "" {
		description = strings.TrimSpace(*overrides.Description)
	}
	capabilities := normalizeCapabilities(jr.Capabilities)
	if overrides.Capabilities != nil {
		capabilities = normalizeCapabilities(overrides.Capabilities)
	}

	now := time.Now().UTC()

	// Check if there's an archived agent with the same node_id — restore it instead of creating a new one
	var agent *model.Agent
	for _, a := range s.agents {
		if a.NodeID == jr.NodeID && a.Archived {
			a.Name = name
			a.Description = description
			a.Role = role
			a.Capabilities = capabilities
			a.Product = defaultProduct(jr.AgentProduct) // 恢复时同步产品标识
			a.UserID = agentUserID                      // 归属继承申请（发起时锁定）
			a.OrgID = agentOrgID                        // 归属继承申请（发起时锁定）
			a.Archived = false
			a.Status = "offline"
			a.UpdatedAt = now
			agent = a
			break
		}
	}

	if agent == nil {
		// Create new agent
		agent = &model.Agent{
			ID:           newID(),
			UserID:       agentUserID,
			OrgID:        agentOrgID,
			Name:         name,
			Description:  description,
			Role:         role,
			Capabilities: capabilities,
			NodeID:       jr.NodeID,
			Product:      defaultProduct(jr.AgentProduct), // 审批时同步 JoinRequest.AgentProduct
			Status:       "offline",
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		s.agents[agent.ID] = agent
	}

	s.agentByNode[jr.NodeID] = agent.ID
	s.rebuildProjectPMSummariesUnsafe(agent.ID)
	s.rebuildTaskPMSummariesUnsafe(agent.ID)
	s.rebuildTodoAssigneeUnsafe(agent.ID)
	if err := s.persistAgentGraphUnsafe(agent.ID); err != nil {
		return nil, mongoWriteError(err)
	}

	// Mark join request as approved（归属发起时已锁定，不再改写为审批者；
	// 仅无主申请 fallback 时记审批者归属）
	jr.Status = "approved"
	jr.ApprovedTrustMeshAgentID = agent.ID
	if inviterID == "" {
		jr.UserID = sc.UserID
		jr.OrgID = agentOrgID
	}
	resolvedAt := now
	jr.ResolvedAt = &resolvedAt
	if err := s.persistJoinRequestUnsafe(jr); err != nil {
		return nil, mongoWriteError(err)
	}

	clone := copyAgent(agent)
	clone.Usage = s.agentUsageUnsafe(agent.ID)
	return clone, nil
}

// isEnterpriseOrgUnsafe 判断 org 是否企业租户；仅能在持锁函数内调用。
func (s *Store) isEnterpriseOrgUnsafe(orgID string) bool {
	if orgID == "" {
		return false
	}
	org, ok := s.organizations[orgID]
	return ok && org.Kind == model.OrgKindEnterprise
}

func (s *Store) RejectJoinRequest(sc Scope, requestID string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	jr, ok := s.joinRequests[requestID]
	if !ok || !s.joinRequestVisible(sc, jr) {
		return transport.NotFound("join request not found")
	}
	if jr.Status != "pending" {
		return transport.Validation("join request is not pending", map[string]any{"status": jr.Status})
	}

	now := time.Now().UTC()
	jr.Status = "rejected"
	jr.UserID = sc.UserID
	jr.ResolvedAt = &now
	if err := s.persistJoinRequestUnsafe(jr); err != nil {
		return mongoWriteError(err)
	}
	return nil
}

func (s *Store) PendingJoinRequestCount(sc Scope) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 与 ListJoinRequests 同构：带租户上下文全量扫描按归属裁决，
	// 无租户上下文走分区索引。
	if sc.HasOrg() {
		count := 0
		for _, jr := range s.joinRequests {
			if jr.Status == "pending" && s.joinRequestVisible(sc, jr) {
				count++
			}
		}
		return count
	}

	count := 0
	for _, id := range s.userJoinRequests[sc.UserID] {
		if jr, ok := s.joinRequests[id]; ok && jr.Status == "pending" {
			count++
		}
	}
	return count
}
