package handler

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"trustmesh/backend/internal/agentfile"
	"trustmesh/backend/internal/clawsynapse"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/project"
	"trustmesh/backend/internal/protocol"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"

	"github.com/gin-gonic/gin"
)

const defaultMeetingTimeout = 30 * time.Minute

type MeetingHandler struct {
	store       *store.Store
	clawClient  *clawsynapse.Client
	fileStorage project.FileStorage
	externalURL string
	jwtSecret   []byte
	downloadTTL time.Duration

	meetingTimers   map[string]*time.Timer
	meetingTimerMu  sync.Mutex
	timeoutDuration time.Duration
}

func NewMeetingHandler(s *store.Store, clawClient *clawsynapse.Client, fileStorage project.FileStorage, externalURL string, jwtSecret []byte, downloadTTL time.Duration) *MeetingHandler {
	return &MeetingHandler{
		store:           s,
		clawClient:      clawClient,
		fileStorage:     fileStorage,
		externalURL:     externalURL,
		jwtSecret:       jwtSecret,
		downloadTTL:     downloadTTL,
		meetingTimers:   make(map[string]*time.Timer),
		timeoutDuration: defaultMeetingTimeout,
	}
}

type createMeetingRequest struct {
	Title        string                     `json:"title"`
	Agenda       string                     `json:"agenda"`
	HostAgentID  string                     `json:"host_agent_id"`
	Participants []model.MeetingParticipant `json:"participants"`
	AgendaItems  []model.MeetingAgendaItem  `json:"agenda_items"`
	FileIDs      []string                   `json:"file_ids"`
}

func (h *MeetingHandler) Create(c *gin.Context) {
	sc, ok := currentScope(c)
	userID := sc.UserID
	if !ok {
		return
	}
	projectID := c.Param("projectId")

	var req createMeetingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid request body"))
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		transport.WriteError(c, transport.Validation("会议标题不能为空", nil))
		return
	}

	meeting := &model.Meeting{
		ProjectID:    projectID,
		Title:        strings.TrimSpace(req.Title),
		Agenda:       strings.TrimSpace(req.Agenda),
		HostAgentID:  req.HostAgentID,
		Participants: req.Participants,
		AgendaItems:  req.AgendaItems,
		FileIDs:      req.FileIDs,
		AttachedFiles: resolveMeetingAttachedFiles(h.store, projectID, req.FileIDs),
	}

	// Assign default IDs to agenda items
	for i := range meeting.AgendaItems {
		item := &meeting.AgendaItems[i]
		if item.ID == "" {
			item.ID = fmt.Sprintf("agenda_%d", i+1)
		}
		if item.Order == 0 {
			item.Order = i + 1
		}
	}

	result, appErr := h.store.CreateMeeting(sc, meeting)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Notify PM agent (host) — PM will decide which executors to involve
	if h.clawClient != nil {
		h.notifyPM(context.Background(), result, result.Agenda, userID)
	}

	// Start meeting timeout watchdog — auto-conclude if PM goes silent.
	h.ensureMeetingTimeout(result.ID)

	transport.WriteData(c, http.StatusCreated, result)
}

func (h *MeetingHandler) Get(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	meetingID := c.Param("id")

	meeting, appErr := h.store.GetMeeting(sc, meetingID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, meeting)
}

func (h *MeetingHandler) List(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	projectID := c.Param("projectId")

	items := h.store.ListMeetings(sc, projectID)
	transport.WriteList(c, items, len(items))
}

type sendMessageRequest struct {
	Content string `json:"content"`
}

func (h *MeetingHandler) SendMessage(c *gin.Context) {
	sc, ok := currentScope(c)
	userID := sc.UserID
	if !ok {
		return
	}
	meetingID := c.Param("id")

	meeting, appErr := h.store.GetMeeting(sc, meetingID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid request body"))
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		transport.WriteError(c, transport.Validation("消息内容不能为空", nil))
		return
	}

	msg := &model.MeetingMessage{
		MeetingID:  meetingID,
		SenderType: "user",
		SenderID:   userID,
		SenderName: "我",
		Content:    strings.TrimSpace(req.Content),
	}

	result, appErr := h.store.AddMeetingMessage(sc, msg)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	if meeting.Status == model.MeetingWaiting {
		_ = h.store.UpdateMeetingStatus(sc, meetingID, model.MeetingInProgress)
	}

	// Forward to PM agent (host) — PM will moderate and involve executors
	if h.clawClient != nil && meeting.Status != model.MeetingCompleted {
		h.notifyPM(context.Background(), meeting, req.Content, userID)
	}

	transport.WriteData(c, http.StatusCreated, result)
}

func (h *MeetingHandler) ListMessages(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	meetingID := c.Param("id")

	items, appErr := h.store.ListMeetingMessages(sc, meetingID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteList(c, items, len(items))
}

func (h *MeetingHandler) Start(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	meetingID := c.Param("id")

	appErr := h.store.UpdateMeetingStatus(sc, meetingID, model.MeetingInProgress)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, gin.H{"status": "in_progress"})
}

func (h *MeetingHandler) End(c *gin.Context) {
	sc, ok := currentScope(c)
	userID := sc.UserID
	if !ok {
		return
	}
	meetingID := c.Param("id")

	// Stop timeout watchdog — user is ending the meeting explicitly.
	h.stopMeetingTimeout(meetingID)

	// Conclude meeting
	_ = h.store.UpdateMeetingStatus(sc, meetingID, model.MeetingCompleted)

	// Ensure meeting minutes exist. The host (PM agent) is expected to upload
	// minutes via chat.message (metadata.action="minutes"). If it didn't (e.g.
	// the meeting was ended without a host-generated summary), auto-generate a
	// transcript-based minutes document so the requirement is always satisfied.
	if h.fileStorage != nil {
		meeting, _ := h.store.GetMeeting(sc, meetingID)
		if meeting != nil && meeting.MinutesFileID == "" {
			if fileID, genErr := h.generateAndSaveMinutes(sc, meeting); genErr == nil && fileID != "" {
				h.appendSystemMeetingNote(sc, meeting, "📄 已自动生成会议纪要并上传至项目文件管理（主持人未主动上传）。")
			}
		}
	}

	// Send [会议已结束] signal — skill instructs agents to ACK and stop
	if h.clawClient != nil {
		meeting, _ := h.store.GetMeeting(sc, meetingID)
		if meeting != nil {
			h.broadcastMeetingEnd(context.Background(), meeting, userID)
		}
	}

	transport.WriteData(c, http.StatusOK, gin.H{"status": "completed"})
}

// AddTodo adds a meeting todo (called after summary generated).
func (h *MeetingHandler) AddTodo(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	meetingID := c.Param("id")

	var req model.MeetingTodoItem
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid request body"))
		return
	}

	meeting, appErr := h.store.AddMeetingTodo(sc, meetingID, req)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusCreated, meeting)
}

// ——— Message helpers ———

// resolveAllNodeIDs returns node IDs of all meeting participants that have a known agent.
func (h *MeetingHandler) resolveAllNodeIDs(meeting *model.Meeting, userID string) []string {
	agents := h.store.ListAgents(store.Scope{UserID: userID})
	agentByID := make(map[string]string, len(agents))
	for _, a := range agents {
		if a.NodeID != "" {
			agentByID[a.ID] = a.NodeID
		}
	}

	seen := make(map[string]bool)
	var ids []string

	// PM agent (host)
	if meeting.HostAgentID != "" {
		if nodeID, ok := agentByID[meeting.HostAgentID]; ok && !seen[nodeID] {
			ids = append(ids, nodeID)
			seen[nodeID] = true
		}
	}

	// Participant agents
	for _, p := range meeting.Participants {
		if nodeID, ok := agentByID[p.AgentID]; ok && !seen[nodeID] {
			ids = append(ids, nodeID)
			seen[nodeID] = true
		}
	}

	return ids
}

// resolvePMNodeID returns the PM agent's node ID for a meeting.
func (h *MeetingHandler) resolvePMNodeID(meeting *model.Meeting, userID string) string {
	agents := h.store.ListAgents(store.Scope{UserID: userID})
	for _, a := range agents {
		if a.ID == meeting.HostAgentID && a.NodeID != "" {
			return a.NodeID
		}
		// Fallback: find any PM-role agent
		if meeting.HostAgentID == "" && a.Role == "pm" && a.NodeID != "" {
			return a.NodeID
		}
	}
	return ""
}

// notifyPM sends a user's meeting message to the PM agent (host) only.
// PM will pre-distribute agenda items to executors and orchestrate round-robin.
func (h *MeetingHandler) notifyPM(ctx context.Context, meeting *model.Meeting, content, userID string) {
	nodeID := h.resolvePMNodeID(meeting, userID)
	if nodeID == "" {
		return
	}

	agents := h.store.ListAgents(store.Scope{UserID: userID})
	agentByID := make(map[string]string, len(agents))
	agentNodeIDs := make(map[string]string, len(agents))
	agentStructByID := make(map[string]model.Agent, len(agents))
	for _, a := range agents {
		agentByID[a.ID] = a.Name
		agentNodeIDs[a.ID] = a.NodeID
		agentStructByID[a.ID] = a
	}

	// Effective agenda items: if the meeting has no structured agenda_items but a
	// free-text agenda exists, synthesize items from it so the host never deadlocks
	// on an empty agenda (it would otherwise stop after the prep/ready handshake).
	effectiveRefs := make([]protocol.MeetingAgendaItemRef, 0, len(meeting.AgendaItems))
	for _, item := range meeting.AgendaItems {
		ref := protocol.MeetingAgendaItemRef{ID: item.ID, Order: item.Order, Description: item.Description}
		for _, as := range item.Assignees {
			ref.Assignees = append(ref.Assignees, protocol.MeetingAssigneeRef{
				AgentID: as.AgentID,
				Name:    agentByID[as.AgentID],
				NodeID:  agentNodeIDs[as.AgentID],
				Weight:  as.Weight,
			})
		}
		effectiveRefs = append(effectiveRefs, ref)
	}
	if len(effectiveRefs) == 0 && strings.TrimSpace(meeting.Agenda) != "" {
		effectiveRefs = synthesizeAgendaItems(meeting.Agenda)
	}

	var sb strings.Builder
	sb.WriteString("请使用 /tm-meeting-host skill 主持本次会议。\n\n")
	sb.WriteString("请按结构化会议流程处理本次会议。\n\n")
	sb.WriteString(fmtMeetingField("会议标题", meeting.Title))
	sb.WriteString(fmtMeetingField("议题", meeting.Agenda))

	// List agenda items with multi-agent assignees
	if len(effectiveRefs) > 0 {
		sb.WriteString("\n**议程安排：**\n")
		for _, item := range effectiveRefs {
			sb.WriteString(fmt.Sprintf("%d. **%s**", item.Order, item.Description))
			if len(item.Assignees) > 0 {
				sb.WriteString(" → 参与: ")
				names := make([]string, 0, len(item.Assignees))
				for _, as := range item.Assignees {
					name := as.Name
					if name == "" {
						name = as.AgentID
					}
					names = append(names, fmt.Sprintf("%s(权重%d)", name, as.Weight))
				}
				sb.WriteString(strings.Join(names, ", "))
			}
			sb.WriteString("\n")
		}
		if len(meeting.AgendaItems) == 0 {
			sb.WriteString("（注：以上议题由会议议程文本自动生成）\n")
		}
		sb.WriteString("\n")
	}

	// Include ONLY the selected meeting participants so the host never assumes
	// other project agents are in the meeting. (Bug: the full roster was sent
	// before, so the host announced "all agents participate".)
	sb.WriteString(fmt.Sprintf("## 参会数字员工清单（仅以下 %d 位；未列出的数字员工不在本次会议中、不会收到任何会议消息）\n\n", len(meeting.Participants)))
	for _, p := range meeting.Participants {
		if p.AgentID == meeting.HostAgentID {
			continue
		}
		a, ok := agentStructByID[p.AgentID]
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("- **%s** (ID: `%s`, 节点: `%s`, 角色: %s)\n", a.Name, a.ID, a.NodeID, a.Role))
		if len(a.Capabilities) > 0 {
			sb.WriteString(fmt.Sprintf("  能力: %s\n", strings.Join(a.Capabilities, ", ")))
		}
	}
	if len(meeting.Participants) == 0 {
		sb.WriteString("（本次会议未指定参会数字员工）\n")
	}
	sb.WriteString("\n")

	// Include reference files if any — with download URLs (same mechanism as tasks)
	var attachedFileRefs []protocol.TaskAttachedFileRef
	if len(meeting.AttachedFiles) > 0 {
		enriched := h.enrichMeetingFiles(meeting.AttachedFiles)
		attachedFileRefs = enriched
		sb.WriteString("## 参考文件下载链接\n\n")
		sb.WriteString("以下文件可通过 curl -L 下载：\n\n")
		for _, f := range enriched {
			name := f.FileName
			if name == "" {
				name = f.ID
			}
			if f.DownloadUrl != "" {
				sb.WriteString(fmt.Sprintf("- **%s**\n  下载链接：%s\n", name, f.DownloadUrl))
			} else {
				sb.WriteString(fmt.Sprintf("- `%s`（无下载链接）\n", name))
			}
		}
		sb.WriteString("\n")
	} else if len(meeting.FileIDs) > 0 {
		// Fallback for meetings created before AttachedFiles was added — no download URLs available
		sb.WriteString("## 参考文件\n\n")
		for _, fid := range meeting.FileIDs {
			sb.WriteString(fmt.Sprintf("- `%s`\n", fid))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("用户消息：")
	sb.WriteString(content)
	sb.WriteString("\n\n")

	sb.WriteString("## 结构化会议流程\n\n")
	sb.WriteString("1. **会前准备**：`meeting.chat` (role=host) + `@数字员工名称` 点名\n")
	sb.WriteString("2. **首轮发言**：按权重从高到低**逐个主动 @ 点名每个 数字员工**；前一个 数字员工 发言完成后必须立即 @ 下一个，**严禁被动等待其余 数字员工 自动发言**（会议里只有被 @ 的 数字员工 才会被触发发言，不 @ 就不会有人说话）\n")
	sb.WriteString("3. **交叉评论**：所有 数字员工 首轮后，让各 数字员工 评论彼此观点\n")
	sb.WriteString("4. **追问澄清**：未解决的问题点名要求补充\n")
	sb.WriteString("5. **纪要第一步**：纪要草案带 `@数字员工名称` 发给所有 数字员工 确认\n")
	sb.WriteString("6. **纪要第二步**：全部确认后，生成最终 Markdown 纪要，**必须用 `meeting.control`（`action=\"minutes\"`，`content` 为完整纪要正文）把纪要上传到项目文件管理**，再用普通 `meeting.chat`（不带 `@数字员工名称`）+ `ui_blocks` 确认按钮发给用户\n")
	sb.WriteString("7. **兜底**：若主持人未上传纪要，系统会在会议关闭时自动生成一份基于发言记录的纪要；以主持人上传的纪要为准\n\n")
	sb.WriteString("⚠️ 规则：所有给数字员工的消息必须加 `@名称`，且**每次必须且只能 @ 一个数字员工**（避免多人同时被触发、消息交错混乱）；最终给用户的确认不要加 @；冲突由你裁决\n")
	sb.WriteString("⚠️ **语言要求**：本次会议全程必须使用简体中文。你主持发言、点名指令（含 `context_brief`）、纪要草案与最终纪要都必须是简体中文；点名 数字员工 时请在 `context_brief` 中明确要求「请用简体中文回复」，任何 数字员工 若用英文或其他语言回复，请立即点名要求其改用简体中文重述。\n")
	sb.WriteString("\n## 结构化路由与上下文（重要）\n\n")
	sb.WriteString("- **精准路由**：点名时在消息里设置 `target` 字段为被点名 数字员工 的 agent_id（同时保留 @名称 便于阅读），后端会据此精准投递，避免误触发其他 数字员工 抢答。\n")
	sb.WriteString("- **上下文摘要（context_brief）**：每次给 数字员工 的点名消息请附带 `context_brief` 字段（≤200字），提炼前序讨论要点与分歧点；数字员工 是无状态的，只看该摘要理解局势，不要把完整聊天记录转发给它。\n")
	sb.WriteString("- **阶段标识（phase）**：用 `phase` 字段标注当前阶段——`init`(广播议程,无需回复) / `prep`(点名报到) / `speak`(首轮发言) / `review`(交叉评审,≤2轮后强制裁决) / `clarify`(追问) / `summary`(广播本议题结论,无需回复) / `confirm`(纪要确认)。`init` 与 `summary` 用 `target:\"all\"` 广播。\n")
	sb.WriteString("- **防死锁**：交叉评审最多 2 轮，第 2 轮后仍有分歧必须强制裁决并记录“保留意见”；点名后若某 数字员工 超时未回复，标注“⏰ 超时”并继续推进，不要空等。\n")
	sb.WriteString("- ⚠️ **报到即推进**：prep 阶段只要收到某 数字员工 回复「已就绪」或任何明确就绪表态，即视为该 数字员工 完成报到，**立即进入 speak 阶段**；**严禁等待 数字员工 下载/阅读参考文件的二次确认**——下载阅读是 数字员工 私下异步动作，绝不作为推进会议的前置条件，也不要因为某 数字员工 迟迟不发「下载完成」回执而停在原地空等。\n")
	sb.WriteString("\n## 技能要求\n\n")
	sb.WriteString("- **你（主持人）必须加载 `/tm-meeting-host` 技能**来处理 `meeting.instruction` 消息和主持会议流程\n")
	sb.WriteString("- **点名时无需在可见消息里写「请加载技能」**：后端在把你的 `meeting.chat` 投递给被点名 数字员工 时会自动附带「请加载 `/tm-meeting-participant` 技能」提示词，前端不会显示该提示。你只需在内容里用 `@名称` + `target` 字段点名即可。\n")
	sb.WriteString("- **纪要上传必须使用 `/tm-meeting-host` 中描述的 `meeting.control` (action=minutes) 协议**\n")

	// Build structured payload for the new meeting protocol
	// IMPORTANT: only the agents the user actually selected as participants,
	// NOT the entire project roster. This is what the host uses to decide who
	// is in the meeting — sending all agents made it announce "everyone joins".
	participants := make([]protocol.MeetingParticipantRef, 0, len(meeting.Participants))
	for _, p := range meeting.Participants {
		if p.AgentID == meeting.HostAgentID {
			continue
		}
		a, ok := agentStructByID[p.AgentID]
		if !ok {
			continue
		}
		participants = append(participants, protocol.MeetingParticipantRef{
			AgentID:      a.ID,
			Name:         a.Name,
			NodeID:       a.NodeID,
			Role:         a.Role,
			Capabilities: a.Capabilities,
		})
	}

	msg := protocol.MeetingInstructionPayload{
		SchemaVersion:  "1.0",
		MeetingID:      meeting.ID,
		ProjectID:      meeting.ProjectID,
		Content:        sb.String(),
		UserContent:    content,
		MeetingTitle:   meeting.Title,
		MeetingAgenda:  meeting.Agenda,
		AgendaItems:    effectiveRefs,
		Participants:   participants,
		AttachedFiles:  attachedFileRefs,
	}
	_, _ = h.clawClient.Publish(ctx, nodeID, "meeting.instruction", msg, meeting.ID, nil)
}

// broadcastMeetingEnd sends [会议已结束] signal to all participants.
// The skill instructs agents to ACK and stop — no further communication.
func (h *MeetingHandler) broadcastMeetingEnd(ctx context.Context, meeting *model.Meeting, userID string) {
	nodeIDs := h.resolveAllNodeIDs(meeting, userID)
	if len(nodeIDs) == 0 {
		return
	}

	payload := protocol.MeetingEndPayload{
		SchemaVersion: "1.0",
		MeetingID:     meeting.ID,
		ProjectID:     meeting.ProjectID,
		Title:         meeting.Title,
		Message:       fmt.Sprintf("[会议已结束] 会议「%s」已被主持人关闭。请勿回复任何业务内容。", meeting.Title),
	}

	for _, nodeID := range nodeIDs {
		_, _ = h.clawClient.Publish(ctx, nodeID, "meeting.end", payload, meeting.ID, nil)
	}
}

func fmtMeetingField(label, value string) string {
	return fmt.Sprintf("- **%s**：%s\n", label, value)
}

// synthesizeAgendaItems derives structured agenda items from a free-text agenda
// when the meeting was created without any structured agenda_items. This prevents
// the host from deadlocking after the prep/ready handshake (it would otherwise have
// no topics to drive). Splitting prefers sentence/semicolon/newline and numbered
// markers over commas, so a single-paragraph agenda stays one topic.
func synthesizeAgendaItems(agenda string) []protocol.MeetingAgendaItemRef {
	agenda = strings.TrimSpace(agenda)
	if agenda == "" {
		return nil
	}
	parts := agendaSplitRe.Split(agenda, -1)
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "。；;,.，、 \t\n")
		if len([]rune(p)) < 4 {
			continue
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		clean = []string{agenda}
	}
	refs := make([]protocol.MeetingAgendaItemRef, 0, len(clean))
	for i, desc := range clean {
		refs = append(refs, protocol.MeetingAgendaItemRef{
			ID:          fmt.Sprintf("syn_%d", i+1),
			Order:       i + 1,
			Description: desc,
		})
	}
	return refs
}

var agendaSplitRe = regexp.MustCompile(`[。\n;；]+`)

// generateAndSaveMinutes builds a transcript-based markdown minutes document and
// stores it in project file management. Used as a fallback when the host (PM
// agent) did not upload minutes via chat.message (metadata.action="minutes").
// Returns the created project file ID (empty on failure).
func (h *MeetingHandler) generateAndSaveMinutes(sc store.Scope, meeting *model.Meeting) (string, error) {
	return h.store.GenerateMeetingMinutesFile(sc, meeting)
}

// appendSystemMeetingNote records a system note in the meeting transcript.
func (h *MeetingHandler) appendSystemMeetingNote(sc store.Scope, meeting *model.Meeting, text string) {
	msg := &model.MeetingMessage{
		MeetingID:  meeting.ID,
		SenderType: "system",
		SenderID:   "system",
		SenderName: "系统",
		Content:    text,
	}
	_, _ = h.store.AddMeetingMessage(sc, msg)
}

// buildMeetingMinutesFileName produces a safe file name for meeting minutes.
func buildMeetingMinutesFileName(title string) string {
	base := strings.TrimSpace(title)
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, "\\", "_")
	base = strings.ReplaceAll(base, ":", "_")
	base = strings.ReplaceAll(base, "*", "_")
	base = strings.ReplaceAll(base, "?", "_")
	base = strings.ReplaceAll(base, "\"", "_")
	base = strings.ReplaceAll(base, "<", "_")
	base = strings.ReplaceAll(base, ">", "_")
	base = strings.ReplaceAll(base, "|", "_")
	if base == "" {
		base = "会议纪要"
	}
	return fmt.Sprintf("%s_会议纪要.md", base)
}

func resolveMeetingAttachedFiles(s interface {
	GetProjectFileByID(fileID string) (*model.ProjectFile, *transport.AppError)
}, projectID string, fileIDs []string) []model.MeetingAttachedFile {
	if len(fileIDs) == 0 {
		return nil
	}
	out := make([]model.MeetingAttachedFile, 0, len(fileIDs))
	for _, fid := range fileIDs {
		f, err := s.GetProjectFileByID(fid)
		if err != nil || f == nil {
			out = append(out, model.MeetingAttachedFile{ID: fid})
			continue
		}
		if f.ProjectID != projectID {
			continue
		}
		out = append(out, model.MeetingAttachedFile{
			ID:       f.ID,
			FileName: f.FileName,
			FileSize: f.FileSize,
			MimeType: f.MimeType,
			Source:   f.Source,
		})
	}
	return out
}

// enrichMeetingFiles converts meeting attached files to protocol refs with download URLs.
// Uses the same token-based download mechanism as tasks.
func (h *MeetingHandler) enrichMeetingFiles(files []model.MeetingAttachedFile) []protocol.TaskAttachedFileRef {
	// Convert MeetingAttachedFile → TaskAttachedFile (identical structure)
	taskFiles := make([]model.TaskAttachedFile, len(files))
	for i, f := range files {
		taskFiles[i] = model.TaskAttachedFile{
			ID:       f.ID,
			FileName: f.FileName,
			FileSize: f.FileSize,
			MimeType: f.MimeType,
			Source:   f.Source,
		}
	}
	return agentfile.EnrichWithDownloadURLs(taskFiles, h.externalURL, h.jwtSecret, h.downloadTTL)
}

// ——— Meeting timeout watchdog ———

// ensureMeetingTimeout starts a background timer that will auto-conclude the
// meeting if no activity (meeting.chat messages) is detected within the timeout
// duration. This prevents meetings from hanging forever when the PM agent goes
// silent. Idempotent — calling it again for the same meeting is a no-op.
func (h *MeetingHandler) ensureMeetingTimeout(meetingID string) {
	h.ensureMeetingTimeoutWith(meetingID, h.timeoutDuration)
}

func (h *MeetingHandler) ensureMeetingTimeoutWith(meetingID string, d time.Duration) {
	h.meetingTimerMu.Lock()
	defer h.meetingTimerMu.Unlock()

	if _, exists := h.meetingTimers[meetingID]; exists {
		return
	}

	h.meetingTimers[meetingID] = time.AfterFunc(d, func() {
		h.timeoutMeeting(meetingID)
	})
}

// OnMeetingActivity resets the timeout watchdog for the given meeting. Called
// by the webhook handler whenever a meeting.chat message is received from an
// agent, indicating that the meeting is still active.
func (h *MeetingHandler) OnMeetingActivity(meetingID string) {
	h.meetingTimerMu.Lock()
	defer h.meetingTimerMu.Unlock()

	t, exists := h.meetingTimers[meetingID]
	if !exists {
		return
	}
	t.Reset(h.timeoutDuration)
}

// stopMeetingTimeout cancels the watchdog for a meeting (called when the user
// explicitly ends the meeting).
func (h *MeetingHandler) stopMeetingTimeout(meetingID string) {
	h.meetingTimerMu.Lock()
	defer h.meetingTimerMu.Unlock()

	if t, exists := h.meetingTimers[meetingID]; exists {
		t.Stop()
		delete(h.meetingTimers, meetingID)
	}
}

// timeoutMeeting is invoked by the watchdog timer when a meeting has been
// inactive for too long. It auto-completes the meeting, generates fallback
// minutes, and broadcasts meeting.end to all participants.
func (h *MeetingHandler) timeoutMeeting(meetingID string) {
	h.meetingTimerMu.Lock()
	delete(h.meetingTimers, meetingID)
	h.meetingTimerMu.Unlock()

	_ = h.store.UpdateMeetingStatus(store.SystemScope(), meetingID, model.MeetingCompleted)

	meeting, _ := h.store.GetMeeting(store.SystemScope(), meetingID)
	if meeting == nil {
		return
	}

	if meeting.MinutesFileID == "" && h.fileStorage != nil {
		if fileID, err := h.generateAndSaveMinutes(store.SystemScope(), meeting); err == nil && fileID != "" {
			h.appendSystemMeetingNote(store.SystemScope(), meeting, "⏰ 会议超时自动收尾，已自动生成会议纪要并上传至项目文件管理。")
		}
	}

	if h.clawClient != nil {
		h.broadcastMeetingEnd(context.Background(), meeting, "")
	}
}

// RecoverTimeouts restarts the inactivity watchdog for every meeting that is
// still in_progress when the backend (re)starts. The watchdog timers live only
// in memory, so a process restart would otherwise silently drop them and leave
// meetings hanging forever. For each in_progress meeting we resume the timer
// based on its last activity, preserving the original timeout window (so a
// meeting that was about to time out still does, and one with time left keeps
// its remaining budget).
func (h *MeetingHandler) RecoverTimeouts(ctx context.Context) {
	meetings := h.store.ListMeetingsByStatus(store.SystemScope(), model.MeetingInProgress)
	for _, m := range meetings {
		last := m.UpdatedAt
		if last.IsZero() {
			last = m.CreatedAt
		}
		remaining := h.timeoutDuration - time.Since(last)
		if remaining <= 0 {
			go h.timeoutMeeting(m.ID)
			continue
		}
		h.ensureMeetingTimeoutWith(m.ID, remaining)
	}
}
