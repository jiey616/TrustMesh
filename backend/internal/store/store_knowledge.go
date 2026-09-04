package store

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode"

	"go.mongodb.org/mongo-driver/v2/bson"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// CreateKnowledgeDocument creates a new knowledge document record.
func (s *Store) CreateKnowledgeDocument(sc Scope, doc *model.KnowledgeDocument) (*model.KnowledgeDocument, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc.ID = "kb_" + newID()
	doc.UserID = sc.UserID
	doc.OrgID = s.resolveOwnerOrgUnsafe(sc)
	doc.Status = model.KnowledgeDocStatusProcessing
	now := time.Now().UTC()
	doc.CreatedAt = now
	doc.UpdatedAt = now
	if doc.Tags == nil {
		doc.Tags = []string{}
	}
	if doc.Metadata == nil {
		doc.Metadata = map[string]any{}
	}

	s.knowledgeDocs[doc.ID] = doc
	s.userKnowledgeDocs[sc.UserID] = append(s.userKnowledgeDocs[sc.UserID], doc.ID)

	if err := s.persistKnowledgeDocUnsafe(doc); err != nil {
		return nil, mongoWriteError(err)
	}
	return doc, nil
}

// GetKnowledgeDocument returns a document by ID, checking ownership.
func (s *Store) GetKnowledgeDocument(sc Scope, docID string) (*model.KnowledgeDocument, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, ok := s.knowledgeDocs[docID]
	if !ok {
		return nil, transport.NotFound("knowledge document not found")
	}
	if !visibleToScope(sc, doc.OrgID, doc.UserID) {
		return nil, transport.Forbidden("access denied")
	}
	return doc, nil
}

// ListKnowledgeDocuments returns documents for a user with optional filters.
func (s *Store) ListKnowledgeDocuments(sc Scope, projectID, status, tag string) []*model.KnowledgeDocument {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 带租户上下文时，user 分区索引只覆盖作者本人的文档，
	// 必须全量扫描按归属裁决，否则同租户成员上传的文档列不出来。
	if sc.HasOrg() {
		result := make([]*model.KnowledgeDocument, 0)
		for _, doc := range s.knowledgeDocs {
			if !visibleToScope(sc, doc.OrgID, doc.UserID) {
				continue
			}
			if !knowledgeDocMatchesFilters(doc, projectID, status, tag) {
				continue
			}
			result = append(result, doc)
		}
		sort.Slice(result, func(i, j int) bool {
			return result[i].CreatedAt.Before(result[j].CreatedAt)
		})
		return result
	}

	// 无租户上下文：走 user 分区索引，与改造前完全一致
	docIDs := s.userKnowledgeDocs[sc.UserID]
	var result []*model.KnowledgeDocument
	for _, id := range docIDs {
		doc, ok := s.knowledgeDocs[id]
		if !ok {
			continue
		}
		if projectID != "" {
			if doc.ProjectID == nil || *doc.ProjectID != projectID {
				continue
			}
		}
		if status != "" && doc.Status != status {
			continue
		}
		if tag != "" && !containsTag(doc.Tags, tag) {
			continue
		}
		result = append(result, doc)
	}
	return result
}

// UpdateKnowledgeDocument updates document metadata.
func (s *Store) UpdateKnowledgeDocument(sc Scope, docID string, title, description *string, tags []string) (*model.KnowledgeDocument, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, ok := s.knowledgeDocs[docID]
	if !ok {
		return nil, transport.NotFound("knowledge document not found")
	}
	if !visibleToScope(sc, doc.OrgID, doc.UserID) {
		return nil, transport.Forbidden("access denied")
	}

	if title != nil {
		doc.Title = *title
	}
	if description != nil {
		doc.Description = *description
	}
	if tags != nil {
		doc.Tags = tags
	}
	doc.UpdatedAt = time.Now().UTC()

	if err := s.persistKnowledgeDocUnsafe(doc); err != nil {
		return nil, mongoWriteError(err)
	}
	return doc, nil
}

// DeleteKnowledgeDocument removes a document and its chunks from the store.
func (s *Store) DeleteKnowledgeDocument(sc Scope, docID string) (*model.KnowledgeDocument, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, ok := s.knowledgeDocs[docID]
	if !ok {
		return nil, transport.NotFound("knowledge document not found")
	}
	if !visibleToScope(sc, doc.OrgID, doc.UserID) {
		return nil, transport.Forbidden("access denied")
	}

	delete(s.knowledgeDocs, docID)
	ids := s.userKnowledgeDocs[sc.UserID]
	for i, id := range ids {
		if id == docID {
			s.userKnowledgeDocs[sc.UserID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}

	_ = s.deleteKnowledgeDocUnsafe(docID)
	_ = s.deleteKnowledgeChunksUnsafe(docID)
	return doc, nil
}

// SaveKnowledgeChunks saves chunks to MongoDB (called by processor).
func (s *Store) SaveKnowledgeChunks(docID string, chunks []model.KnowledgeChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_ = s.deleteKnowledgeChunksUnsafe(docID)
	return s.persistKnowledgeChunksUnsafe(chunks)
}

// SetKnowledgeDocSourceURI sets the source URI and persists to MongoDB.
func (s *Store) SetKnowledgeDocSourceURI(docID, uri string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, ok := s.knowledgeDocs[docID]
	if !ok {
		return transport.NotFound("knowledge document not found")
	}
	doc.SourceURI = uri
	if err := s.persistKnowledgeDocUnsafe(doc); err != nil {
		return mongoWriteError(err)
	}
	return nil
}

// UpdateKnowledgeDocStatus updates document status and chunk count.
func (s *Store) UpdateKnowledgeDocStatus(docID, status string, chunkCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, ok := s.knowledgeDocs[docID]
	if !ok {
		return nil
	}
	doc.Status = status
	doc.ChunkCount = chunkCount
	doc.UpdatedAt = time.Now().UTC()
	return s.persistKnowledgeDocUnsafe(doc)
}

// GetKnowledgeChunksByIDs fetches chunks from MongoDB by their IDs.
func (s *Store) GetKnowledgeChunksByIDs(chunkIDs []string) ([]model.KnowledgeChunk, error) {
	if !s.mongoEnabled || s.mongoKnowledgeChunks == nil || len(chunkIDs) == 0 {
		return nil, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()

	cursor, err := s.mongoKnowledgeChunks.Find(ctx, bson.M{"_id": bson.M{"$in": chunkIDs}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var chunks []model.KnowledgeChunk
	if err := cursor.All(ctx, &chunks); err != nil {
		return nil, err
	}
	return chunks, nil
}

// GetKnowledgeChunksByDocID returns all chunks for a document.
func (s *Store) GetKnowledgeChunksByDocID(docID string) ([]model.KnowledgeChunk, error) {
	if !s.mongoEnabled || s.mongoKnowledgeChunks == nil {
		return nil, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()

	cursor, err := s.mongoKnowledgeChunks.Find(ctx, bson.M{"document_id": docID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var chunks []model.KnowledgeChunk
	if err := cursor.All(ctx, &chunks); err != nil {
		return nil, err
	}
	return chunks, nil
}

// ResolveKnowledgeDocOwnerByAgentNode finds the user who owns an agent by node ID.
func (s *Store) ResolveKnowledgeDocOwnerByAgentNode(nodeID string) (string, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agentID, ok := s.agentByNode[strings.TrimSpace(nodeID)]
	if !ok {
		return "", transport.NotFound("agent not found for node")
	}
	agent, ok := s.agents[agentID]
	if !ok {
		return "", transport.NotFound("agent not found")
	}
	return agent.UserID, nil
}

// ValidateProjectOwnership checks that a project belongs to a user.
func (s *Store) ValidateProjectOwnership(sc Scope, projectID string) *transport.AppError {
	s.mu.RLock()
	defer s.mu.RUnlock()

	project, ok := s.projects[projectID]
	if !ok {
		return transport.NotFound("project not found")
	}
	if !visibleToScope(sc, project.OrgID, project.UserID) {
		return transport.Forbidden("access denied to project")
	}
	return nil
}

// GetKnowledgeDocTitle returns the title of a knowledge document.
func (s *Store) GetKnowledgeDocTitle(docID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if doc, ok := s.knowledgeDocs[docID]; ok {
		return doc.Title
	}
	return ""
}

// SearchKnowledgeChunks does a text search fallback (when no vector search).
func (s *Store) SearchKnowledgeChunks(ctx context.Context, sc Scope, projectID *string, query string, limit int) ([]model.KnowledgeChunk, error) {
	if !s.mongoEnabled || s.mongoKnowledgeChunks == nil {
		return nil, nil
	}
	mctx, cancel := s.mongoContext()
	defer cancel()
	_ = ctx

	// 带租户上下文按 org_id 检索；否则按 user_id，与改造前完全一致。
	filter := bson.M{"user_id": sc.UserID}
	if sc.HasOrg() {
		filter = bson.M{"org_id": sc.OrgID}
	}
	if projectID != nil {
		filter["$or"] = bson.A{
			bson.M{"project_id": *projectID},
			bson.M{"project_id": nil},
		}
	}

	cursor, err := s.mongoKnowledgeChunks.Find(mctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(mctx)

	var chunks []model.KnowledgeChunk
	if err := cursor.All(mctx, &chunks); err != nil {
		return nil, err
	}

	// Keyword matching fallback: split the query into 3-4 char keywords and
	// match chunks that contain any of them, ranked by hit count. This makes
	// natural-language queries (not just verbatim substrings) usable when no
	// embedding backend is configured.
	queryLower := strings.ToLower(query)
	keywords := extractTextKeywords(queryLower)
	type scored struct {
		chunk model.KnowledgeChunk
		hits  int
	}
	var matched []scored
	for _, chunk := range chunks {
		content := strings.ToLower(chunk.Content)
		if strings.Contains(content, queryLower) {
			// full substring hit: perfect match
			matched = append(matched, scored{chunk: chunk, hits: 1000})
			continue
		}
		hits := 0
		for _, kw := range keywords {
			if strings.Contains(content, kw) {
				hits++
			}
		}
		if hits > 0 {
			matched = append(matched, scored{chunk: chunk, hits: hits})
		}
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].hits > matched[j].hits })
	out := make([]model.KnowledgeChunk, 0, len(matched))
	for i, m := range matched {
		if i >= limit {
			break
		}
		out = append(out, m.chunk)
	}
	return out, nil
}

// textStopwords are function words/particles that add no retrieval value as
// 3-4 char keywords. Kept deliberately small; over-filtering hurts recall.
var textStopwords = map[string]bool{
	"怎么": true, "什么": true, "如何": true, "为什么": true, "是否": true,
	"一个": true, "这个": true, "那个": true, "哪个": true, "可以": true,
	"需要": true, "应该": true, "进行": true, "以及": true, "或者": true,
	"就是": true, "我们": true, "你们": true, "他们": true, "对于": true,
	"有关": true, "关于": true, "没有": true, "不是": true, "如果": true,
	"然后": true, "但是": true, "因为": true, "所以": true, "请": true,
	"让我": true, "帮我": true, "说一下": true, "告诉我": true,
}

// extractTextKeywords derives search keywords from a Chinese query by sliding
// 3-4 char windows (longer first), dropping stopwords and duplicates, and
// capping the list to avoid a keyword explosion on long queries.
func extractTextKeywords(query string) []string {
	var b strings.Builder
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	runes := []rune(b.String())
	if len(runes) == 0 {
		return nil
	}
	if len(runes) <= 4 {
		return []string{string(runes)}
	}

	seen := make(map[string]bool, 24)
	var words []string
	for size := 4; size >= 3; size-- {
		for i := 0; i+size <= len(runes); i++ {
			w := string(runes[i : i+size])
			if textStopwords[w] || seen[w] {
				continue
			}
			seen[w] = true
			words = append(words, w)
			if len(words) >= 16 {
				break
			}
		}
		if len(words) >= 16 {
			break
		}
	}
	if len(words) == 0 && len(runes) >= 3 {
		return []string{string(runes[:3])}
	}
	return words
}

func containsTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}

// knowledgeDocMatchesFilters 是 ListKnowledgeDocuments 的过滤条件，
// 抽出来供无租户上下文（user 分区索引）与带租户上下文（全量扫描）两条路径复用。
func knowledgeDocMatchesFilters(doc *model.KnowledgeDocument, projectID, status, tag string) bool {
	if projectID != "" {
		if doc.ProjectID == nil || *doc.ProjectID != projectID {
			return false
		}
	}
	if status != "" && doc.Status != status {
		return false
	}
	if tag != "" && !containsTag(doc.Tags, tag) {
		return false
	}
	return true
}
