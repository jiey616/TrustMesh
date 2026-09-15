package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ─── T2.1：会议域「Mongo 权威」写序与乐观锁 ───
//
// 会议域自 T2.1 起改为 **Mongo 权威**：所有写路径一律「先落库成功，再改内存」。
// Mongo 写失败或版本冲突时**直接返错、内存零副作用**，杜绝改造前
// 「内存里有、库里没有，重启即丢且只留一条 warn」的问题。
//
// 已知代价：Mongo I/O 在 s.mu 持锁内执行，临界区被拉长（MONGO_TIMEOUT 当前 5s）。
// 这是本批接受的权衡 —— 写序正确优先于吞吐；T2.5 再拆 per-aggregate 细粒度锁。

// meetingVersionFloor 会议版本号的起始值：新建会议从 1 开始，
// 「存量 version <= 0」的文档在载入 / 回填时也统一归一化为 1，
// 保证乐观锁 filter（{_id, version}）永远有确定的基准值。
const meetingVersionFloor = 1

// meetingVersionConflictCode / meetingVersionConflictMessage 是所有版本冲突出口
// 共用的业务码（HTTP 409）与文案：调用方按 code 判定「要不要重试」，文案不参与判定。
const (
	meetingVersionConflictCode    = "MEETING_VERSION_CONFLICT"
	meetingVersionConflictMessage = "meeting was modified concurrently, please retry"
)

// meetingVersionConflict 构造统一形态的版本冲突错误。
func meetingVersionConflict() *transport.AppError {
	return transport.Conflict(meetingVersionConflictCode, meetingVersionConflictMessage)
}

// isMeetingVersionConflict 判定一个错误是否为版本冲突（调用方据此决定要不要重试）。
func isMeetingVersionConflict(appErr *transport.AppError) bool {
	return appErr != nil && appErr.Code == meetingVersionConflictCode
}

// normalizeMeetingVersion 把缺失 / 非法的 version 归一化为 meetingVersionFloor。
//
// 存量会议文档（T2.1 之前写入）没有 version 字段，解码后为 0；若不归一化，
// 后续带 {_id, version: 0} filter 的更新永远匹配不上 —— 该会议会被永久锁死。
// 归一化只在内存解码 / 载入路径上做，库内真值由 backfillMeetingVersions 补齐。
func normalizeMeetingVersion(m *model.Meeting) {
	if m == nil {
		return
	}
	if m.Version <= 0 {
		m.Version = meetingVersionFloor
	}
}

// applyMeetingVersionedUpdateLocked 在持锁内对会议执行一次带乐观锁的 Mongo 更新。
//
// 契约（调用方必须遵守）：
//   - filter = {_id: m.ID, version: cur}，$set 自动追加 version: cur+1；
//   - Mongo 报错         → mongoWriteError(500)，调用方须立即返回，**不得改任何内存字段**；
//   - 未命中             → 见下方 healZeroVersionMeetingLocked：先排除「存量 version<=0
//     文档」这种可自愈的形态，仍然不行才 409 MEETING_VERSION_CONFLICT；
//   - 成功               → 内存对象的 Version 推进到 cur+1，调用方再写其余内存字段。
//
// 为什么 ModifiedCount == 0 就能判定「未命中」：$set 里 version 恒从 cur 变成 cur+1，
// 因此「命中」必然「修改」，ModifiedCount == 0 ⟺ filter 未命中。
func (s *Store) applyMeetingVersionedUpdateLocked(m *model.Meeting, set bson.M) *transport.AppError {
	normalizeMeetingVersion(m)
	cur := m.Version
	next := cur + 1
	if set == nil {
		set = bson.M{}
	}
	set["version"] = next

	if !s.mongoEnabled || s.mongoMeetings == nil {
		// 纯内存模式（Mongo 未启用）：没有权威库可比，仍然推进版本以保持内存自洽。
		m.Version = next
		return nil
	}

	// Mongo 写在持锁内，会拉长临界区 —— 本批接受，T2.5 再拆细粒度锁。
	ctx, cancel := s.mongoContext()
	defer cancel()
	res, err := s.mongoMeetings.UpdateOne(ctx, bson.M{"_id": m.ID, "version": cur}, bson.M{"$set": set})
	if err != nil {
		return mongoWriteError(err)
	}
	if res != nil && res.ModifiedCount > 0 {
		m.Version = next
		return nil
	}
	// 未命中除了「真实并发落后」，还有一种**一旦发生就永久锁死**的存量形态
	// （库内 version<=0 而内存已被归一化成 1），必须自愈 —— 见下面的注释。
	if appErr := s.healZeroVersionMeetingLocked(m, set, cur); appErr != nil {
		return appErr
	}
	m.Version = next
	return nil
}

// healZeroVersionMeetingLocked 处理 filter 未命中的**存量兼容**分支（P1）。
//
// 未命中（ModifiedCount == 0）有两种成因，必须区分：
//  1. 真实并发落后：库内 version 是合法的（> 0）但已被别的写方推进 → 维持 409，
//     不允许掩盖，否则就退化成「后写静默覆盖先写」，正是乐观锁要消灭的问题；
//  2. 存量非法文档：version 缺失 / 显式 0 / null / 负数。这类文档一旦被命中，
//     **每次**更新都会 409，而且**重启也救不回来** —— 启动时的一次性回填覆盖不到
//     滚动发布期间由旧镜像（无 Version 字段）新建出来的文档，等于该会议永久锁死。
//
// 对 ② 做一次性自愈：先把库内 version 修成内存认定的 cur（此刻库内的值本来就是非法的，
// 因此这次修不用带 version filter），再重试一次正常的版本化更新。
//
// 这是纯存量兼容修复，**不改变并发语义**：唯一被放宽的是「version <= 0」这种明确
// 非法的历史取值，正常写路径永远不会在库里留下 <= 0 的版本。
func (s *Store) healZeroVersionMeetingLocked(m *model.Meeting, set bson.M, cur int) *transport.AppError {
	ctx, cancel := s.mongoContext()
	defer cancel()

	var doc struct {
		Version int `bson:"version"`
	}
	if err := s.mongoMeetings.FindOne(ctx, bson.M{"_id": m.ID}).Decode(&doc); err != nil {
		// 文档不存在（或读取失败）→ 不是版本问题，维持冲突语义。
		return meetingVersionConflict()
	}
	if doc.Version > 0 {
		// 合法但已落后 → 真实并发冲突，如实上报。
		return meetingVersionConflict()
	}

	// 存量非法值：先修成 cur，再按正常路径重试一次。
	if _, err := s.mongoMeetings.UpdateOne(ctx, bson.M{"_id": m.ID}, bson.M{"$set": bson.M{"version": cur}}); err != nil {
		return mongoWriteError(err)
	}
	retry, err := s.mongoMeetings.UpdateOne(ctx, bson.M{"_id": m.ID, "version": cur}, bson.M{"$set": set})
	if err != nil {
		return mongoWriteError(err)
	}
	if retry == nil || retry.ModifiedCount == 0 {
		return meetingVersionConflict()
	}
	return nil
}

// retryOnceAfterVersionRefreshLocked 版本冲突时，把内存 version 刷到库内最新值后重试一次。
// 返回 true 表示重试成功，调用方按成功路径继续。
//
// 只适用于「$set 里所有业务字段都由本次输入整体派生」的写入（目前只有会话态）：
// participants / last_phase / last_target / speaker_turns / last_activity_at / updated_at
// 全部由**当前这条消息**算出并整体覆盖，因此只刷新 version 再写一次，不会吞掉别的写方
// 留下的「本条消息之外」的语义；被放弃的只是并发方对同一批派生字段的写入，而这些字段
// 本来就会在下一条消息到达时被重新派生。
//
// 非冲突错误、以及刷新之后仍然冲突，都会如实返回 false，由调用方上交原错误 ——
// 这条路径不会把真实冲突伪装成成功。
func (s *Store) retryOnceAfterVersionRefreshLocked(appErr *transport.AppError, m *model.Meeting, set bson.M) bool {
	if !isMeetingVersionConflict(appErr) || s.mongoMeetings == nil {
		return false
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	var doc struct {
		Version int `bson:"version"`
	}
	if err := s.mongoMeetings.FindOne(ctx, bson.M{"_id": m.ID}).Decode(&doc); err != nil {
		return false
	}
	if doc.Version <= 0 || doc.Version == m.Version {
		// <= 0 属存量非法形态，已由 healing 路径处理；与内存相同说明不是落后。
		return false
	}
	m.Version = doc.Version
	return s.applyMeetingVersionedUpdateLocked(m, set) == nil
}

// ─── Meeting CRUD ───

func (s *Store) CreateMeeting(sc Scope, m *model.Meeting) (*model.Meeting, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m.ID = uuid.NewString()
	m.CreatedAt = time.Now().UTC()
	m.UpdatedAt = m.CreatedAt
	m.CreatorID = sc.UserID
	// 阶段 1 双写漏网：原本恒挂作者个人租户，企业租户下新建会议会落到
	// 个人租户、带租户头反而看不到。与 2-2 CreateTask / 2-4 CreateKnowledgeDocument
	// / 2-5a SaveProjectFile 同类。
	m.OrgID = s.resolveOwnerOrgUnsafe(sc)
	if m.Status == "" {
		m.Status = model.MeetingWaiting
	}
	m.Version = meetingVersionFloor

	// T2.1：先落库，成功才写内存 —— 失败时 s.meetings / s.projectMeetings 完全不变。
	if s.mongoEnabled && s.mongoMeetings != nil {
		ctx, cancel := s.mongoContext()
		defer cancel()
		if _, err := s.mongoMeetings.InsertOne(ctx, m); err != nil {
			return nil, mongoWriteError(err)
		}
	}

	s.meetings[m.ID] = m
	s.projectMeetings[m.ProjectID] = append(s.projectMeetings[m.ProjectID], m.ID)

	return m, nil
}

func (s *Store) GetMeeting(sc Scope, meetingID string) (*model.Meeting, *transport.AppError) {
	s.mu.RLock()
	m, ok := s.meetings[meetingID]
	if ok {
		// 裁决必须在锁内：meetingVisible 读 s.projects / s.projectMembers
		visible := s.meetingVisible(sc, m)
		s.mu.RUnlock()
		if !visible {
			return nil, transport.NotFound("meeting not found")
		}
		return m, nil
	}
	s.mu.RUnlock()

	// Lazy-load from MongoDB so callers keep working after a backend restart
	// (the in-memory map is empty until meetings are re-accessed).
	if s.mongoEnabled {
		ctx, cancel := s.mongoContext()
		defer cancel()
		var mm model.Meeting
		if err := s.mongoMeetings.FindOne(ctx, bson.M{"_id": meetingID}).Decode(&mm); err == nil {
			// T2.1：存量文档没有 version 字段 → 归一化为 1，否则后续更新必然冲突。
			normalizeMeetingVersion(&mm)
			s.mu.Lock()
			s.meetings[meetingID] = &mm
			s.projectMeetings[mm.ProjectID] = append(s.projectMeetings[mm.ProjectID], mm.ID)
			visible := s.meetingVisible(sc, &mm)
			s.mu.Unlock()
			if !visible {
				return nil, transport.NotFound("meeting not found")
			}
			return &mm, nil
		}
	}
	return nil, transport.NotFound("meeting not found")
}

func (s *Store) ListMeetings(sc Scope, projectID string) []*model.Meeting {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 项目不在内存（懒加载边界）时不裁决，行为与改造前一致 ——
	// 宁可沿用旧行为，也不因数据缺失误伤。
	if p, ok := s.projects[projectID]; ok && !s.projectVisible(sc, p) {
		return []*model.Meeting{}
	}
	ids := s.projectMeetings[projectID]
	out := make([]*model.Meeting, 0, len(ids))
	for _, id := range ids {
		if m, ok := s.meetings[id]; ok {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// ListMeetingsByStatus returns all meetings with the given status. It reads
// from MongoDB (not only the in-memory cache) so it works correctly after a
// backend restart when the in-memory map is empty. Results are also re-fed
// into the in-memory cache for subsequent access.
func (s *Store) ListMeetingsByStatus(sc Scope, status model.MeetingStatus) []*model.Meeting {
	if s.mongoEnabled {
		ctx, cancel := s.mongoContext()
		defer cancel()
		cur, err := s.mongoMeetings.Find(ctx, bson.M{"status": status})
		if err == nil {
			var out []*model.Meeting
			if cur.All(ctx, &out) == nil {
				s.mu.Lock()
				kept := make([]*model.Meeting, 0, len(out))
				for _, m := range out {
					// T2.1：存量文档没有 version 字段 → 归一化为 1（同上）。
					normalizeMeetingVersion(m)
					if _, exists := s.meetings[m.ID]; !exists {
						s.meetings[m.ID] = m
						s.projectMeetings[m.ProjectID] = append(s.projectMeetings[m.ProjectID], m.ID)
					}
					// 缓存照灌，但只返回本次 Scope 可见的会议
					if s.meetingVisible(sc, m) {
						kept = append(kept, m)
					}
				}
				s.mu.Unlock()
				return kept
			}
		}
	}
	return nil
}

func (s *Store) UpdateMeetingStatus(sc Scope, meetingID string, status model.MeetingStatus) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.meetings[meetingID]
	if !ok || !s.meetingVisible(sc, m) {
		return transport.NotFound("meeting not found")
	}
	prevStatus := m.Status
	updatedAt := time.Now().UTC()

	// T2.1：先落库（带版本锁），成功才改内存。失败 / 冲突 → 直接返错，内存零副作用。
	if appErr := s.applyMeetingVersionedUpdateLocked(m, bson.M{
		"status":     status,
		"updated_at": updatedAt,
	}); appErr != nil {
		return appErr
	}
	m.Status = status
	m.UpdatedAt = updatedAt

	// 办公室 SSE：会议状态变化时向前端推送，这样 3D 办公室才能驱动数字员工走过去开会。
	if prevStatus != status {
		targetUser := sc.UserID
		if targetUser == "" {
			targetUser = m.CreatorID
		}
		switch status {
		case model.MeetingInProgress:
			participantList := make([]map[string]any, 0, len(m.Participants))
			for _, p := range m.Participants {
				participantList = append(participantList, map[string]any{
					"agent_id":   p.AgentID,
					"agent_name": p.AgentName,
					"node_id":    p.NodeID,
					"status":     p.Status,
				})
			}
			s.publishUserEventUnsafe(targetUser, "meeting.started", map[string]any{
				"meeting_id":    meetingID,
				"project_id":    m.ProjectID,
				"title":         m.Title,
				"host_agent_id": m.HostAgentID,
				"participants":  participantList,
			}, m.UpdatedAt)
		case model.MeetingCompleted:
			s.publishUserEventUnsafe(targetUser, "meeting.ended", map[string]any{
				"meeting_id": meetingID,
				"project_id": m.ProjectID,
				"title":      m.Title,
				"status":     status,
			}, m.UpdatedAt)
		}
	}

	return nil
}

// ─── Meeting Message CRUD ───

func (s *Store) AddMeetingMessage(sc Scope, msg *model.MeetingMessage) (*model.MeetingMessage, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 改造前本函数没有任何归属校验：任何登录用户都能往任意 meetingID 发消息。
	// 内部 webhook（agent 发言）用 SystemScope 显式放行。
	if !s.meetingVisible(sc, s.meetings[msg.MeetingID]) {
		return nil, transport.NotFound("meeting not found")
	}

	// T2.6：会议消息幂等。客户端可带 IdempotencyKey（超时重试去重）；不带的旧客户端
	// 由后端派生软键。先在写序之前判定：命中（seen）→ 回查已存消息直接返回，不重复落库；
	// 失败 → 立即返错（宁失败不静默重复）；首次 → 把 key 回填到 msg，使其可回查。
	key := msg.IdempotencyKey
	if key == "" {
		key = meetingMessageSoftKey(msg.MeetingID, msg.SenderType, msg.SenderID, msg.Content)
	}
	seen, appErr := s.idemCheckOrRecord(key, 5*time.Minute)
	if appErr != nil {
		return nil, appErr
	}
	if seen {
		if existing := s.findStoredMeetingMessageByKeyUnsafe(key); existing != nil {
			return existing, nil
		}
		// 极端：seen 但查不到（TTL 窗口内被并发清理）。按「首次」继续，避免丢消息。
	}
	msg.IdempotencyKey = key

	msg.ID = uuid.NewString()
	// 多租户阶段 1：消息归属跟随所属会议。
	if m, ok := s.meetings[msg.MeetingID]; ok && m.OrgID != "" {
		msg.OrgID = m.OrgID
	}
	msg.CreatedAt = time.Now().UTC()

	// 取出会议对象，后续既要更新参会状态，也要推送 SSE。
	var meeting *model.Meeting
	if m, ok := s.meetings[msg.MeetingID]; ok {
		meeting = m
	}

	// T2.1 写序（三步全在持锁内，任一步失败 → 立即返错、内存零副作用）：
	//   ① 先落库消息本身；
	//   ② 再落库会话态（带版本锁）；
	//   ③ 两次 Mongo 写都成功后，才统一写内存（消息 map / 索引 + 会话态派生字段）。
	//
	// 为什么是「先消息、后会话态」而不是反过来：若先更新会话态（Mongo 里 version +1）
	// 再插消息、而插消息失败，内存 version 没跟上 → 之后所有带 version filter 的更新
	// 都会 ModifiedCount == 0 → **该会议被永久锁死**。先插消息可规避这个锁死面。
	// 代价：② 失败时消息已在库里而内存未变，属**可接受残差** —— 会话态是纯派生量，
	// 可由 backfillMeetingSessionUnsafe 按历史消息重算，重启后也能读回。

	// ① 消息落库
	if s.mongoEnabled && s.mongoMeetingMessages != nil {
		ctx, cancel := s.mongoContext()
		defer cancel()
		if _, err := s.mongoMeetingMessages.InsertOne(ctx, msg); err != nil {
			return nil, mongoWriteError(err)
		}
	}

	// ② 会话态落库：先在**副本**上算出派生值，落库成功后才合回内存，
	//    保证本步失败时内存仍是旧值。
	var draft *model.Meeting
	if meeting != nil {
		snap := *meeting
		// Participants 是切片，必须深拷一层，否则「invited → joined」会直接改到内存对象。
		snap.Participants = append([]model.MeetingParticipant(nil), meeting.Participants...)
		// Mark the participant as joined once they actually speak. Participants are
		// created in "invited" state; this keeps the participant roster accurate
		// without relying on a separate join handshake.
		if msg.SenderType == "agent" && msg.SenderID != "" {
			for i := range snap.Participants {
				if snap.Participants[i].AgentID == msg.SenderID && snap.Participants[i].Status == "invited" {
					snap.Participants[i].Status = "joined"
					break
				}
			}
		}
		// T1.2 会话态 write-through：由本条消息在**副本**上派生会话语义（phase/target/
		// 发言轮数/最后活动时间），深拷 Participants，落库成功后才合回内存。
		snap = deriveMeetingSession(snap, msg)
		draft = &snap

		// 会话态更新失败不再只 warn（T2.1）：静默丢会话态会让重启后「续开」判断失真。
		sessionSet := bson.M{
			"participants":     draft.Participants,
			"last_phase":       draft.LastPhase,
			"last_target":      draft.LastTarget,
			"speaker_turns":    draft.SpeakerTurns,
			"last_activity_at": draft.LastActivityAt,
			"updated_at":       draft.UpdatedAt,
		}
		if appErr := s.applyMeetingVersionedUpdateLocked(meeting, sessionSet); appErr != nil {
			// P2：会话态写冲突时，把内存 version 刷到库内最新值后重试一次。
			// 安全前提：上面的 $set 字段（participants/last_phase/last_target/
			// speaker_turns/last_activity_at/updated_at）全部由**当前这条消息**派生且整体
			// 覆盖，只刷新 version 再写一次不会吞掉别的写方留下的「本条消息之外」的语义；
			// 真实并发落后时 retryOnceAfterVersionRefreshLocked 会如实返回 false，原错误上交。
			if !s.retryOnceAfterVersionRefreshLocked(appErr, meeting, sessionSet) {
				return nil, appErr
			}
		}
	}

	// ③ 全部 Mongo 写成功 → 写内存
	s.meetingMessages[msg.ID] = msg
	s.meetingMessageIndex[msg.MeetingID] = append(s.meetingMessageIndex[msg.MeetingID], msg.ID)
	if meeting != nil && draft != nil {
		meeting.Participants = draft.Participants
		meeting.LastPhase = draft.LastPhase
		meeting.LastTarget = draft.LastTarget
		meeting.SpeakerTurns = draft.SpeakerTurns
		meeting.LastActivityAt = draft.LastActivityAt
		meeting.UpdatedAt = draft.UpdatedAt
	}

	// 办公室 SSE：任何会议发言（用户/数字员工/系统）都推送，3D 办公室显示说话人气泡。
	// 仅成功路径推送（失败不推）。
	s.publishMeetingMessageUnsafe(meeting, msg)

	return msg, nil
}

// deriveMeetingSession 纯函数：由一条会议消息派生出会议记录上的会话语义（T1.2），
// 返回**新的副本**，绝不就地改写输入。调用方可安全拿返回值去落库、成功后再合回内存，
// 失败时输入对象保持原样 —— 这是 T2.1「Mongo 失败零内存副作用」的关键保证之一。
//
// 派生规则（与历史行为一致）：phase / target 取该消息的值（空值不覆盖，避免用户或系统
// 消息把主持人刚声明的阶段清空）；SpeakerTurns 仅对 agent 发言累加；LastActivityAt /
// UpdatedAt 取消息时间。Participants 是切片，必须深拷一层，否则「invited → joined」会
// 直接改到调用方手里那份内存对象上 —— QA 反向验证曾证明这一点，见
// TestDeriveMeetingSessionDoesNotMutateInput。
func deriveMeetingSession(meeting model.Meeting, msg *model.MeetingMessage) model.Meeting {
	if msg == nil {
		return meeting
	}
	if msg.Phase != "" {
		meeting.LastPhase = msg.Phase
	}
	if msg.Target != "" {
		meeting.LastTarget = msg.Target
	}
	if msg.SenderType == "agent" {
		meeting.SpeakerTurns++
	}
	meeting.LastActivityAt = msg.CreatedAt
	meeting.UpdatedAt = msg.CreatedAt

	// 深拷 Participants：invited → joined 只改副本，不动输入。
	meeting.Participants = append([]model.MeetingParticipant(nil), meeting.Participants...)
	if msg.SenderType == "agent" && msg.SenderID != "" {
		for i := range meeting.Participants {
			if meeting.Participants[i].AgentID == msg.SenderID && meeting.Participants[i].Status == "invited" {
				meeting.Participants[i].Status = "joined"
				break
			}
		}
	}
	return meeting
}

func (s *Store) publishMeetingMessageUnsafe(m *model.Meeting, msg *model.MeetingMessage) {
	if m == nil || msg == nil {
		return
	}
	s.publishUserEventUnsafe(m.CreatorID, "meeting.message", map[string]any{
		"meeting_id":  msg.MeetingID,
		"project_id":  m.ProjectID,
		"message_id":  msg.ID,
		"sender_type": msg.SenderType,
		"sender_id":   msg.SenderID,
		"sender_name": msg.SenderName,
		"content":     msg.Content,
		"phase":       msg.Phase,
		"target":      msg.Target,
		"created_at":  msg.CreatedAt,
	}, msg.CreatedAt)
}

// ListMeetingMessages 返回会议消息。归属校验 fail-closed：
// 会议不在内存时对齐 GetMeeting 先懒加载 Mongo 再裁决，不可见一律 404。
// （旧实现「不在内存就不裁决、不可见回空列表」是 fail-open，越权探测抓出后修复。）
func (s *Store) ListMeetingMessages(sc Scope, meetingID string) ([]*model.MeetingMessage, *transport.AppError) {
	s.mu.RLock()
	m, ok := s.meetings[meetingID]
	if ok {
		visible := s.meetingVisible(sc, m)
		s.mu.RUnlock()
		if !visible {
			return nil, transport.NotFound("meeting not found")
		}
	} else {
		s.mu.RUnlock()
		if s.mongoEnabled {
			ctx, cancel := s.mongoContext()
			defer cancel()
			var mm model.Meeting
			if err := s.mongoMeetings.FindOne(ctx, bson.M{"_id": meetingID}).Decode(&mm); err == nil {
				// T2.1：存量文档没有 version 字段 → 归一化为 1（同 GetMeeting）。
				normalizeMeetingVersion(&mm)
				s.mu.Lock()
				s.meetings[meetingID] = &mm
				s.projectMeetings[mm.ProjectID] = append(s.projectMeetings[mm.ProjectID], mm.ID)
				visible := s.meetingVisible(sc, &mm)
				s.mu.Unlock()
				if !visible {
					return nil, transport.NotFound("meeting not found")
				}
			} else {
				return nil, transport.NotFound("meeting not found")
			}
		} else {
			return nil, transport.NotFound("meeting not found")
		}
	}

	ids := s.meetingMessageIndex[meetingID]
	out := make([]*model.MeetingMessage, 0, len(ids))
	for _, id := range ids {
		if msg, ok := s.meetingMessages[id]; ok {
			out = append(out, msg)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// ─── Meeting Summary / Todos / Agenda ───

func (s *Store) UpdateMeetingSummary(sc Scope, meetingID, summaryFileID string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.meetings[meetingID]
	if !ok || !s.meetingVisible(sc, m) {
		return transport.NotFound("meeting not found")
	}
	updatedAt := time.Now().UTC()

	// T2.1：先落库（带版本锁），成功才改内存。
	if appErr := s.applyMeetingVersionedUpdateLocked(m, bson.M{
		"summary_file_id": summaryFileID,
		"updated_at":      updatedAt,
	}); appErr != nil {
		return appErr
	}
	m.SummaryFileID = summaryFileID
	m.UpdatedAt = updatedAt
	return nil
}

// UpdateMeetingMinutes stores the generated meeting minutes (markdown text) and
// the linked project file ID on the meeting record.
func (s *Store) UpdateMeetingMinutes(sc Scope, meetingID, minutes, minutesFileID string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.meetings[meetingID]
	if !ok || !s.meetingVisible(sc, m) {
		return transport.NotFound("meeting not found")
	}
	updatedAt := time.Now().UTC()

	// T2.1：先落库（带版本锁），成功才改内存。
	if appErr := s.applyMeetingVersionedUpdateLocked(m, bson.M{
		"minutes":         minutes,
		"minutes_file_id": minutesFileID,
		"updated_at":      updatedAt,
	}); appErr != nil {
		return appErr
	}
	m.Minutes = minutes
	m.MinutesFileID = minutesFileID
	m.UpdatedAt = updatedAt
	return nil
}

// GenerateMeetingMinutesFile synthesizes meeting minutes from the transcript and
// saves them as a project file (Source = "meeting_minutes"), then links the file
// to the meeting record. It is the single source of truth for transcript-based
// minutes generation, shared by MeetingHandler.End (host did not upload minutes)
// and the timeout monitor (force-concluded idle meeting).
//
// Returns the generated project file ID, or an empty string if generation was
// skipped (no file storage configured, or already has minutes).
func (s *Store) GenerateMeetingMinutesFile(sc Scope, meeting *model.Meeting) (string, error) {
	if s.fileStorage == nil {
		return "", fmt.Errorf("file storage not configured")
	}
	if meeting.MinutesFileID != "" {
		return meeting.MinutesFileID, nil
	}

	// 归属校验（本函数后面会调 SaveProjectFile 拿写锁，裁决必须先放锁）
	s.mu.RLock()
	visible := s.meetingVisible(sc, meeting)
	s.mu.RUnlock()
	if !visible {
		return "", fmt.Errorf("meeting not found")
	}

	// 可见性已在本函数前置校验，此处错误不可能触发
	messages, _ := s.ListMeetingMessages(sc, meeting.ID)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 会议纪要：%s\n\n", meeting.Title))
	sb.WriteString(fmt.Sprintf("- **会议标题**：%s\n", meeting.Title))
	sb.WriteString(fmt.Sprintf("- **状态**：已结束\n"))
	sb.WriteString(fmt.Sprintf("- **生成时间**：%s\n", time.Now().Format("2006-01-02 15:04:05")))
	if len(meeting.Participants) > 0 {
		names := make([]string, 0, len(meeting.Participants))
		for _, p := range meeting.Participants {
			name := p.AgentName
			if name == "" {
				name = p.AgentID
			}
			names = append(names, name)
		}
		sb.WriteString(fmt.Sprintf("- **参会数字员工**：%s\n", strings.Join(names, "、")))
	}
	sb.WriteString("\n")

	sb.WriteString("## 议程\n\n")
	if strings.TrimSpace(meeting.Agenda) != "" {
		sb.WriteString(meeting.Agenda + "\n")
	} else if len(meeting.AgendaItems) > 0 {
		for _, item := range meeting.AgendaItems {
			sb.WriteString(fmt.Sprintf("%d. %s\n", item.Order, item.Description))
		}
	} else {
		sb.WriteString("（无）\n")
	}
	sb.WriteString("\n")

	sb.WriteString("## 会议记录\n\n")
	if len(messages) == 0 {
		sb.WriteString("（无发言记录）\n")
	}
	for _, m := range messages {
		if m.SenderType == "system" {
			continue
		}
		name := m.SenderName
		if name == "" {
			name = m.SenderType
		}
		sb.WriteString(fmt.Sprintf("**%s**：%s\n\n", name, m.Content))
	}

	sb.WriteString("## 待办事项\n\n")
	if len(meeting.Todos) == 0 {
		sb.WriteString("（无）\n")
	} else {
		for _, t := range meeting.Todos {
			owner := t.ResponsibleAgentName
			if owner == "" {
				owner = t.ResponsibleAgentID
			}
			status := t.Status
			if status == "" {
				status = "pending"
			}
			sb.WriteString(fmt.Sprintf("- %s（负责人：%s，状态：%s）\n", t.Description, owner, status))
		}
	}

	markdown := sb.String()
	fileName := fmt.Sprintf("%s_会议纪要.md", sanitizeMinutesTitle(meeting.Title))
	pf := &model.ProjectFile{
		ProjectID: meeting.ProjectID,
		OrgID:     meeting.OrgID,
		FileName:  fileName,
		FileSize:  int64(len([]byte(markdown))),
		MimeType:  "text/markdown",
		Source:    "meeting_minutes",
		MeetingID: meeting.ID,
	}

	// 落纪要文件要走 SaveProjectFile 的项目归属校验，归属上下文分两种：
	//   - HTTP 路径：直接用请求者身份，做真实的项目归属校验
	//   - 系统路径（timeout_monitor 等无用户会话）：兜底到项目 owner / 会议创建者，
	//     保证纪要始终可归属，不会因为空 UserID 被拒
	fileScope := sc
	if sc.System {
		ownerID := ""
		s.mu.RLock()
		if p, ok := s.projects[meeting.ProjectID]; ok && p.UserID != "" {
			ownerID = p.UserID
		}
		s.mu.RUnlock()
		if ownerID == "" {
			ownerID = meeting.CreatorID
		}
		fileScope = Scope{UserID: ownerID}
	}

	pf, appErr := s.SaveProjectFile(fileScope, meeting.ProjectID, pf)
	if appErr != nil {
		return "", fmt.Errorf("save project file: %w", appErr)
	}
	localPath, err := s.fileStorage.Save(context.Background(), meeting.ProjectID, pf.ID, fileName, strings.NewReader(markdown))
	if err != nil {
		return "", fmt.Errorf("store minutes file: %w", err)
	}
	_ = s.SetProjectFileLocalPath(pf.ID, localPath)
	_ = s.UpdateMeetingMinutes(sc, meeting.ID, markdown, pf.ID)
	return pf.ID, nil
}

// sanitizeMinutesTitle produces a safe file-name base from a meeting title.
func sanitizeMinutesTitle(title string) string {
	base := strings.TrimSpace(title)
	for _, c := range []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"} {
		base = strings.ReplaceAll(base, c, "_")
	}
	if base == "" {
		base = "会议纪要"
	}
	return base
}

func (s *Store) AddMeetingTodo(sc Scope, meetingID string, todo model.MeetingTodoItem) (*model.Meeting, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.meetings[meetingID]
	if !ok || !s.meetingVisible(sc, m) {
		return nil, transport.NotFound("meeting not found")
	}
	todo.ID = uuid.NewString()
	todo.Status = "pending"
	// 先构造新切片再落库：绝不先改 m.Todos，否则失败会留下内存副作用。
	todos := append(append([]model.MeetingTodoItem(nil), m.Todos...), todo)
	updatedAt := time.Now().UTC()

	// T2.1：先落库（带版本锁），成功才改内存。
	if appErr := s.applyMeetingVersionedUpdateLocked(m, bson.M{
		"todos":      todos,
		"updated_at": updatedAt,
	}); appErr != nil {
		return nil, appErr
	}
	m.Todos = todos
	m.UpdatedAt = updatedAt
	return m, nil
}

// ─── 双写校验（T2.1 过渡期） ───

// VerifyMeetingConsistency 校验内存会议缓存与 Mongo 文档是否一致（T2.1 双写过渡期）。
//
// 返回 (检查条数, 不一致条数, 错误)。生产可调用、**无副作用**：只在 RLock 下取一份
// 内存快照，然后逐条 FindOne 比对，不写任何集合。
// 过渡期应当恒为 mismatched == 0；非 0 表示内存与 Mongo 已经分叉，需要人工介入。
// Mongo 未启用时返回 (0, 0, nil) —— 没有第二份数据可比。
func (s *Store) VerifyMeetingConsistency() (int, int, error) {
	if !s.mongoEnabled || s.mongoMeetings == nil {
		return 0, 0, nil
	}

	s.mu.RLock()
	snapshot := make([]model.Meeting, 0, len(s.meetings))
	for _, m := range s.meetings {
		if m != nil {
			snapshot = append(snapshot, *m)
		}
	}
	s.mu.RUnlock()

	ctx, cancel := s.mongoContext()
	defer cancel()

	mismatched := 0
	for i := range snapshot {
		want := snapshot[i]
		var got model.Meeting
		err := s.mongoMeetings.FindOne(ctx, bson.M{"_id": want.ID}).Decode(&got)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				// 内存有、库里没有 —— 正是改造前「重启即丢」的分叉形态。
				mismatched++
				continue
			}
			return len(snapshot), mismatched, err
		}
		if !meetingSnapshotMatches(want, got) {
			mismatched++
		}
	}
	return len(snapshot), mismatched, nil
}

// meetingSnapshotMatches 判定一份内存快照与 Mongo 文档的关键字段是否一致。
// 只比写路径会改动的字段；todos 只比条数（数组深比会引入排序 / 时间的噪声）。
func meetingSnapshotMatches(want, got model.Meeting) bool {
	return want.Status == got.Status &&
		want.Version == got.Version &&
		want.SummaryFileID == got.SummaryFileID &&
		want.MinutesFileID == got.MinutesFileID &&
		want.Minutes == got.Minutes &&
		want.LastPhase == got.LastPhase &&
		want.LastTarget == got.LastTarget &&
		want.SpeakerTurns == got.SpeakerTurns &&
		len(want.Todos) == len(got.Todos)
}
