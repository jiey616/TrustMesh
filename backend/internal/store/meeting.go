package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

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

	s.meetings[m.ID] = m
	s.projectMeetings[m.ProjectID] = append(s.projectMeetings[m.ProjectID], m.ID)

	if s.mongoEnabled {
		ctx, cancel := s.mongoContext()
		defer cancel()
		if _, err := s.mongoMeetings.InsertOne(ctx, m); err != nil {
			return nil, mongoWriteError(err)
		}
	}

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
	m.Status = status
	m.UpdatedAt = time.Now().UTC()

	if s.mongoEnabled {
		ctx, cancel := s.mongoContext()
		defer cancel()
		filter := bson.M{"_id": meetingID}
		update := bson.M{"$set": bson.M{"status": status, "updated_at": m.UpdatedAt}}
		if _, err := s.mongoMeetings.UpdateOne(ctx, filter, update); err != nil {
			return mongoWriteError(err)
		}
	}

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

	msg.ID = uuid.NewString()
	// 多租户阶段 1：消息归属跟随所属会议。
	if m, ok := s.meetings[msg.MeetingID]; ok && m.OrgID != "" {
		msg.OrgID = m.OrgID
	}
	msg.CreatedAt = time.Now().UTC()

	s.meetingMessages[msg.ID] = msg
	s.meetingMessageIndex[msg.MeetingID] = append(s.meetingMessageIndex[msg.MeetingID], msg.ID)

	// 取出会议对象，后续既要更新参会状态，也要推送 SSE。
	var meeting *model.Meeting
	if m, ok := s.meetings[msg.MeetingID]; ok {
		meeting = m
	}

	if meeting != nil {
		// Mark the participant as joined once they actually speak. Participants are
		// created in "invited" state; this keeps the participant roster accurate
		// without relying on a separate join handshake.
		if msg.SenderType == "agent" && msg.SenderID != "" {
			for i := range meeting.Participants {
				if meeting.Participants[i].AgentID == msg.SenderID && meeting.Participants[i].Status == "invited" {
					meeting.Participants[i].Status = "joined"
					break
				}
			}
		}

		// T1.2 会话态 write-through：把本条消息的会话语义（phase/target/发言轮数/
		// 最后活动时间）镜到会议记录上并**立即落库**。消息本身早就写入，
		// 这一步保证重启后不仅能读回记录，还知道「会议进行到哪、上一次点到谁」。
		applyMeetingSessionFromMessageUnsafe(meeting, msg)

		if s.mongoEnabled {
			ctx, cancel := s.mongoContext()
			upd := bson.M{"$set": bson.M{
				"participants":     meeting.Participants,
				"last_phase":       meeting.LastPhase,
				"last_target":      meeting.LastTarget,
				"speaker_turns":    meeting.SpeakerTurns,
				"last_activity_at": meeting.LastActivityAt,
				"updated_at":       meeting.UpdatedAt,
			}}
			if _, e := s.mongoMeetings.UpdateOne(ctx, bson.M{"_id": meeting.ID}, upd); e != nil {
				s.log.Warn("failed to persist meeting session state", zap.String("meeting_id", meeting.ID), zap.Error(e))
			}
			cancel()
		}
	}

	if s.mongoEnabled {
		ctx, cancel := s.mongoContext()
		defer cancel()
		if _, err := s.mongoMeetingMessages.InsertOne(ctx, msg); err != nil {
			return nil, mongoWriteError(err)
		}
	}

	// 办公室 SSE：任何会议发言（用户/数字员工/系统）都推送，3D 办公室显示说话人气泡。
	s.publishMeetingMessageUnsafe(meeting, msg)

	return msg, nil
}

// applyMeetingSessionFromMessageUnsafe 把一条会议消息的会话语义镜像到会议记录上（T1.2）。
//
// 纯派生，不改变任何编排语义：phase / target 取该消息的值（空值不覆盖，避免用户或系统
// 消息把主持人刚声明的阶段清空）；SpeakerTurns 仅对 agent 发言累加；LastActivityAt 取
// 消息时间。调用方必须持有写锁。
func applyMeetingSessionFromMessageUnsafe(m *model.Meeting, msg *model.MeetingMessage) {
	if m == nil || msg == nil {
		return
	}
	if msg.Phase != "" {
		m.LastPhase = msg.Phase
	}
	if msg.Target != "" {
		m.LastTarget = msg.Target
	}
	if msg.SenderType == "agent" {
		m.SpeakerTurns++
	}
	m.LastActivityAt = msg.CreatedAt
	m.UpdatedAt = msg.CreatedAt
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
	m.SummaryFileID = summaryFileID
	m.UpdatedAt = time.Now().UTC()

	if s.mongoEnabled {
		ctx, cancel := s.mongoContext()
		defer cancel()
		filter := bson.M{"_id": meetingID}
		update := bson.M{"$set": bson.M{"summary_file_id": summaryFileID, "updated_at": m.UpdatedAt}}
		if _, err := s.mongoMeetings.UpdateOne(ctx, filter, update); err != nil {
			return mongoWriteError(err)
		}
	}
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
	m.Minutes = minutes
	m.MinutesFileID = minutesFileID
	m.UpdatedAt = time.Now().UTC()

	if s.mongoEnabled {
		ctx, cancel := s.mongoContext()
		defer cancel()
		filter := bson.M{"_id": meetingID}
		update := bson.M{"$set": bson.M{"minutes": minutes, "minutes_file_id": minutesFileID, "updated_at": m.UpdatedAt}}
		if _, err := s.mongoMeetings.UpdateOne(ctx, filter, update); err != nil {
			return mongoWriteError(err)
		}
	}
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
	m.Todos = append(m.Todos, todo)
	m.UpdatedAt = time.Now().UTC()

	if s.mongoEnabled {
		ctx, cancel := s.mongoContext()
		defer cancel()
		filter := bson.M{"_id": meetingID}
		update := bson.M{"$set": bson.M{"todos": m.Todos, "updated_at": m.UpdatedAt}}
		if _, err := s.mongoMeetings.UpdateOne(ctx, filter, update); err != nil {
			return nil, mongoWriteError(err)
		}
	}
	return m, nil
}
