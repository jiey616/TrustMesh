package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"trustmesh/backend/internal/embedding"
	"trustmesh/backend/internal/knowledge"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

type KnowledgeHandler struct {
	store     *store.Store
	storage   knowledge.FileStorage
	processor *knowledge.Processor
	embedder  embedding.Client
	qdrant    *knowledge.QdrantClient
	log       *zap.Logger
}

func NewKnowledgeHandler(
	s *store.Store,
	storage knowledge.FileStorage,
	processor *knowledge.Processor,
	embedder embedding.Client,
	qdrant *knowledge.QdrantClient,
	log *zap.Logger,
) *KnowledgeHandler {
	return &KnowledgeHandler{
		store:     s,
		storage:   storage,
		processor: processor,
		embedder:  embedder,
		qdrant:    qdrant,
		log:       log,
	}
}

func (h *KnowledgeHandler) Upload(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	userID := sc.UserID

	// Apply upload limits: max file size 100MB + file type whitelist
	applyUploadLimit(c)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "file is required"))
		return
	}
	defer file.Close()

	// Validate file extension against whitelist
	if !validateFileExtension(header.Filename) {
		transport.WriteError(c, transport.BadRequest("UNSUPPORTED_FILE_TYPE",
			"不支持的文件类型。允许的格式：文档、图片、压缩包、代码文件等。"))
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		title = header.Filename
	}

	doc := &model.KnowledgeDocument{
		Title:       title,
		Description: strings.TrimSpace(c.PostForm("description")),
		DocType:     strings.TrimSpace(c.PostForm("doc_type")),
		MimeType:    header.Header.Get("Content-Type"),
		FileSize:    header.Size,
	}
	if doc.DocType == "" {
		doc.DocType = "document"
	}
	if doc.MimeType == "" {
		doc.MimeType = "text/plain"
	}

	if projectID := strings.TrimSpace(c.PostForm("project_id")); projectID != "" {
		if appErr := h.store.ValidateProjectOwnership(sc, projectID); appErr != nil {
			transport.WriteError(c, appErr)
			return
		}
		doc.ProjectID = &projectID
	}

	if tagsStr := strings.TrimSpace(c.PostForm("tags")); tagsStr != "" {
		var tags []string
		for _, t := range strings.Split(tagsStr, ",") {
			if tag := strings.TrimSpace(t); tag != "" {
				tags = append(tags, tag)
			}
		}
		doc.Tags = tags
	}

	// Create document record first to get ID
	doc, appErr := h.store.CreateKnowledgeDocument(sc, doc)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Save file
	uri, err := h.storage.Save(c.Request.Context(), userID, doc.ID, header.Filename, file)
	if err != nil {
		h.log.Error("failed to save knowledge file", zap.String("doc_id", doc.ID), zap.Error(err))
		_ = h.store.UpdateKnowledgeDocStatus(doc.ID, model.KnowledgeDocStatusFailed, 0)
		transport.WriteError(c, transport.NewError(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save file"))
		return
	}

	// Update source URI and persist to MongoDB
	if appErr := h.store.SetKnowledgeDocSourceURI(doc.ID, uri); appErr != nil {
		h.log.Error("failed to persist source URI", zap.String("doc_id", doc.ID), zap.Error(appErr))
	}
	doc.SourceURI = uri

	// Start async processing
	if h.processor != nil {
		go h.processor.ProcessDocument(doc)
	} else {
		_ = h.store.UpdateKnowledgeDocStatus(doc.ID, model.KnowledgeDocStatusReady, 0)
	}

	transport.WriteData(c, http.StatusCreated, doc)
}

func (h *KnowledgeHandler) List(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	projectID := strings.TrimSpace(c.Query("project_id"))
	status := strings.TrimSpace(c.Query("status"))
	tag := strings.TrimSpace(c.Query("tag"))

	docs := h.store.ListKnowledgeDocuments(sc, projectID, status, tag)
	if docs == nil {
		docs = []*model.KnowledgeDocument{}
	}
	transport.WriteList(c, docs, len(docs))
}

func (h *KnowledgeHandler) Get(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	doc, appErr := h.store.GetKnowledgeDocument(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, doc)
}

type updateKnowledgeDocRequest struct {
	Title       *string  `json:"title"`
	Description *string  `json:"description"`
	Tags        []string `json:"tags"`
}

func (h *KnowledgeHandler) Update(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var req updateKnowledgeDocRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}

	doc, appErr := h.store.UpdateKnowledgeDocument(sc, c.Param("id"), req.Title, req.Description, req.Tags)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, doc)
}

func (h *KnowledgeHandler) Delete(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	doc, appErr := h.store.DeleteKnowledgeDocument(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Clean up file storage and Qdrant vectors
	if h.storage != nil && doc.SourceURI != "" {
		_ = h.storage.Delete(c.Request.Context(), doc.SourceURI)
	}
	if h.qdrant != nil {
		_ = h.qdrant.DeleteByDocumentID(c.Request.Context(), doc.ID)
	}

	transport.WriteData(c, http.StatusOK, doc)
}

func (h *KnowledgeHandler) ListChunks(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	docID := c.Param("id")
	if _, appErr := h.store.GetKnowledgeDocument(sc, docID); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	chunks, err := h.store.GetKnowledgeChunksByDocID(docID)
	if err != nil {
		transport.WriteError(c, transport.NewError(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch chunks"))
		return
	}
	if chunks == nil {
		chunks = []model.KnowledgeChunk{}
	}
	transport.WriteList(c, chunks, len(chunks))
}

func (h *KnowledgeHandler) Reprocess(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	doc, appErr := h.store.GetKnowledgeDocument(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	_ = h.store.UpdateKnowledgeDocStatus(doc.ID, model.KnowledgeDocStatusProcessing, 0)

	if h.qdrant != nil {
		_ = h.qdrant.DeleteByDocumentID(c.Request.Context(), doc.ID)
	}

	if h.processor != nil {
		go h.processor.ProcessDocument(doc)
	}

	transport.WriteData(c, http.StatusOK, gin.H{"status": "processing"})
}

type searchRequest struct {
	Query     string  `json:"query"`
	ProjectID string  `json:"project_id"`
	TopK      int     `json:"top_k"`
	MinScore  float64 `json:"min_score"`
}

type searchResultItem struct {
	ChunkID       string         `json:"chunk_id"`
	DocumentID    string         `json:"document_id"`
	DocumentTitle string         `json:"document_title"`
	Content       string         `json:"content"`
	Score         float64        `json:"score"`
	ChunkIndex    int            `json:"chunk_index"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

func (h *KnowledgeHandler) Search(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var req searchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "query is required"))
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if req.MinScore <= 0 {
		req.MinScore = 0.7
	}

	results, err := h.vectorSearch(c, sc, req)
	if err != nil {
		h.log.Warn("vector search failed, falling back to text search", zap.Error(err))
		results, err = h.textSearch(c, sc, req)
		if err != nil {
			transport.WriteError(c, transport.NewError(http.StatusInternalServerError, "INTERNAL_ERROR", "search failed"))
			return
		}
	}

	if results == nil {
		results = []searchResultItem{}
	}
	transport.WriteList(c, results, len(results))
}

func (h *KnowledgeHandler) vectorSearch(c *gin.Context, sc store.Scope, req searchRequest) ([]searchResultItem, error) {
	if h.embedder == nil || h.qdrant == nil {
		return nil, errors.New("vector search unavailable: embedding not configured")
	}

	embeddings, err := h.embedder.Embed(c.Request.Context(), []string{req.Query})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return nil, nil
	}

	// Build filter：带租户上下文按 org_id 过滤，否则按 user_id（与改造前一致）
	filterKey, filterValue := "user_id", sc.UserID
	if sc.HasOrg() {
		filterKey, filterValue = "org_id", sc.OrgID
	}
	mustConditions := []knowledge.QdrantCondition{
		{Key: filterKey, Match: map[string]any{"value": filterValue}},
	}

	var filter *knowledge.QdrantFilter
	if req.ProjectID != "" {
		// Search project-specific + user global docs
		filter = &knowledge.QdrantFilter{
			Must: mustConditions,
			Should: []knowledge.QdrantCondition{
				{Key: "project_id", Match: map[string]any{"value": req.ProjectID}},
			},
		}
	} else {
		filter = &knowledge.QdrantFilter{
			Must: mustConditions,
		}
	}

	hits, err := h.qdrant.Search(c.Request.Context(), embeddings[0], filter, req.TopK)
	if err != nil {
		return nil, err
	}

	var results []searchResultItem
	for _, hit := range hits {
		if hit.Score < req.MinScore {
			continue
		}
		chunkID, _ := hit.Payload["chunk_id"].(string)
		docID, _ := hit.Payload["document_id"].(string)

		chunks, err := h.store.GetKnowledgeChunksByIDs([]string{chunkID})
		if err != nil || len(chunks) == 0 {
			continue
		}
		chunk := chunks[0]

		results = append(results, searchResultItem{
			ChunkID:       chunkID,
			DocumentID:    docID,
			DocumentTitle: h.store.GetKnowledgeDocTitle(docID),
			Content:       chunk.Content,
			Score:         hit.Score,
			ChunkIndex:    chunk.ChunkIndex,
			Metadata:      chunk.Metadata,
		})
	}
	return results, nil
}

func (h *KnowledgeHandler) textSearch(c *gin.Context, sc store.Scope, req searchRequest) ([]searchResultItem, error) {
	var projectID *string
	if req.ProjectID != "" {
		projectID = &req.ProjectID
	}

	chunks, err := h.store.SearchKnowledgeChunks(c.Request.Context(), sc, projectID, req.Query, req.TopK)
	if err != nil {
		return nil, err
	}

	var results []searchResultItem
	for _, chunk := range chunks {
		results = append(results, searchResultItem{
			ChunkID:       chunk.ID,
			DocumentID:    chunk.DocumentID,
			DocumentTitle: h.store.GetKnowledgeDocTitle(chunk.DocumentID),
			Content:       chunk.Content,
			Score:         1.0,
			ChunkIndex:    chunk.ChunkIndex,
			Metadata:      chunk.Metadata,
		})
	}
	return results, nil
}
