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

func (s *Store) CreateMeeting(userID string, m *model.Meeting) (*model.Meeting, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m.ID = uuid.NewString()
	m.CreatedAt = time.Now().UTC()
	m.UpdatedAt = m.CreatedAt
	m.CreatorID = userID
	m.OrgID = s.personalOrgOfUnsafe(userID)
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

func (s *Store) GetMeeting(_ string, meetingID string) (*model.Meeting, *transport.AppError) {
	s.mu.RLock()
	m, ok := s.meetings[meetingID]
	s.mu.RUnlock()
	if ok {
		return m, nil
	}

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
			s.mu.Unlock()
			return &mm, nil
		}
	}
	return nil, transport.NotFound("meeting not found")
}

func (s *Store) ListMeetings(_ string, projectID string) []*model.Meeting {
	s.mu.RLock()
	defer s.mu.RUnlock()

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
func (s *Store) ListMeetingsByStatus(_ string, status model.MeetingStatus) []*model.Meeting {
	if s.mongoEnabled {
		ctx, cancel := s.mongoContext()
		defer cancel()
		cur, err := s.mongoMeetings.Find(ctx, bson.M{"status": status})
		if err == nil {
			var out []*model.Meeting
			if cur.All(ctx, &out) == nil {
				s.mu.Lock()
				for _, m := range out {
					if _, exists := s.meetings[m.ID]; !exists {
						s.meetings[m.ID] = m
						s.projectMeetings[m.ProjectID] = append(s.projectMeetings[m.ProjectID], m.ID)
					}
				}
				s.mu.Unlock()
				return out
			}
		}
	}
	return nil
}

func (s *Store) UpdateMeetingStatus(userID string, meetingID string, status model.MeetingStatus) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.meetings[meetingID]
	if !ok {
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
		targetUser := userID
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

func (s *Store) AddMeetingMessage(msg *model.MeetingMessage) (*model.MeetingMessage, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	// Mark the participant as joined once they actually speak. Participants are
	// created in "invited" state; this keeps the participant roster accurate
	// without relying on a separate join handshake.
	if msg.SenderType == "agent" && msg.SenderID != "" && meeting != nil {
		for i := range meeting.Participants {
			if meeting.Participants[i].AgentID == msg.SenderID && meeting.Participants[i].Status == "invited" {
				meeting.Participants[i].Status = "joined"
				meeting.UpdatedAt = msg.CreatedAt
				if s.mongoEnabled {
					ctx, cancel := s.mongoContext()
					upd := bson.M{"$set": bson.M{"participants": meeting.Participants, "updated_at": meeting.UpdatedAt}}
					if _, e := s.mongoMeetings.UpdateOne(ctx, bson.M{"_id": meeting.ID}, upd); e != nil {
						s.log.Warn("failed to update participant status", zap.String("meeting_id", meeting.ID), zap.Error(e))
					}
					cancel()
				}
				break
			}
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

func (s *Store) ListMeetingMessages(_ string, meetingID string) []*model.MeetingMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	return out
}

// ─── Meeting Summary / Todos / Agenda ───

func (s *Store) UpdateMeetingSummary(_ string, meetingID, summaryFileID string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.meetings[meetingID]
	if !ok {
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
func (s *Store) UpdateMeetingMinutes(_ string, meetingID, minutes, minutesFileID string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.meetings[meetingID]
	if !ok {
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
func (s *Store) GenerateMeetingMinutesFile(userID string, meeting *model.Meeting) (string, error) {
	if s.fileStorage == nil {
		return "", fmt.Errorf("file storage not configured")
	}
	if meeting.MinutesFileID != "" {
		return meeting.MinutesFileID, nil
	}

	messages := s.ListMeetingMessages(userID, meeting.ID)

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

	// Resolve the project owner for the ownership check inside SaveProjectFile.
	// Callers such as the timeout monitor may pass an empty userID; fall back to
	// the project owner (or meeting creator) so minutes are always attributable.
	ownerID := userID
	if ownerID == "" {
		if p, ok := s.projects[meeting.ProjectID]; ok && p.UserID != "" {
			ownerID = p.UserID
		} else if meeting.CreatorID != "" {
			ownerID = meeting.CreatorID
		}
	}

	pf, appErr := s.SaveProjectFile(ownerID, meeting.ProjectID, pf)
	if appErr != nil {
		return "", fmt.Errorf("save project file: %w", appErr)
	}
	localPath, err := s.fileStorage.Save(context.Background(), meeting.ProjectID, pf.ID, fileName, strings.NewReader(markdown))
	if err != nil {
		return "", fmt.Errorf("store minutes file: %w", err)
	}
	_ = s.SetProjectFileLocalPath(pf.ID, localPath)
	_ = s.UpdateMeetingMinutes("", meeting.ID, markdown, pf.ID)
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

func (s *Store) AddMeetingTodo(_ string, meetingID string, todo model.MeetingTodoItem) (*model.Meeting, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.meetings[meetingID]
	if !ok {
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
