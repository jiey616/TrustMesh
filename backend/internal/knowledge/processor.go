package knowledge

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"trustmesh/backend/internal/embedding"
	"trustmesh/backend/internal/model"
)

// ChunkStore defines the storage operations the processor needs.
type ChunkStore interface {
	SaveKnowledgeChunks(docID string, chunks []model.KnowledgeChunk) error
	UpdateKnowledgeDocStatus(docID, status string, chunkCount int) error
}

// Processor handles async document processing: parse → chunk → embed → store.
type Processor struct {
	storage  FileStorage
	embedder embedding.Client
	qdrant   *QdrantClient
	store    ChunkStore
	log      *zap.Logger
}

func NewProcessor(storage FileStorage, embedder embedding.Client, qdrant *QdrantClient, store ChunkStore, log *zap.Logger) *Processor {
	return &Processor{
		storage:  storage,
		embedder: embedder,
		qdrant:   qdrant,
		store:    store,
		log:      log,
	}
}

// ProcessDocument runs the full processing pipeline for a document.
// This should be called in a goroutine.
func (p *Processor) ProcessDocument(doc *model.KnowledgeDocument) {
	ctx := context.Background()

	if err := p.processDocument(ctx, doc); err != nil {
		if p.log != nil {
			p.log.Error("knowledge document processing failed",
				zap.String("doc_id", doc.ID),
				zap.Error(err))
		}
		_ = p.store.UpdateKnowledgeDocStatus(doc.ID, model.KnowledgeDocStatusFailed, 0)
		return
	}
}

func (p *Processor) processDocument(ctx context.Context, doc *model.KnowledgeDocument) error {
	// 1. Read file content
	reader, err := p.storage.Get(ctx, doc.SourceURI)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read content: %w", err)
	}

	text := string(content)
	if strings.TrimSpace(text) == "" {
		_ = p.store.UpdateKnowledgeDocStatus(doc.ID, model.KnowledgeDocStatusReady, 0)
		return nil
	}

	// 2. Chunk the text
	chunkResults := ChunkText(text, doc.MimeType, DefaultChunkSize, DefaultChunkOverlap)
	if len(chunkResults) == 0 {
		_ = p.store.UpdateKnowledgeDocStatus(doc.ID, model.KnowledgeDocStatusReady, 0)
		return nil
	}

	// 3. Build chunk models
	chunks := make([]model.KnowledgeChunk, len(chunkResults))
	texts := make([]string, len(chunkResults))
	for i, cr := range chunkResults {
		chunkID := fmt.Sprintf("%s_chunk_%d", doc.ID, i)
		chunks[i] = model.KnowledgeChunk{
			ID:         chunkID,
			DocumentID: doc.ID,
			UserID:     doc.UserID,
			ProjectID:  doc.ProjectID,
			ChunkIndex: i,
			Content:    cr.Content,
			TokenCount: cr.TokenCount,
			Metadata:   cr.Metadata,
			CreatedAt:  doc.CreatedAt,
		}
		texts[i] = cr.Content
	}

	// 4. Generate embeddings in batches (skipped when no embedding client is
	// configured — document still gets chunked for text search).
	var allEmbeddings [][]float32
	if p.embedder != nil {
		allEmbeddings = make([][]float32, len(texts))
		const batchSize = 20
		for i := 0; i < len(texts); i += batchSize {
			end := i + batchSize
			if end > len(texts) {
				end = len(texts)
			}
			batch := texts[i:end]
			embeddings, err := p.embedder.Embed(ctx, batch)
			if err != nil {
				return fmt.Errorf("embed batch %d: %w", i/batchSize, err)
			}
			copy(allEmbeddings[i:end], embeddings)
		}
	}

	// 5. Save chunks to MongoDB
	if err := p.store.SaveKnowledgeChunks(doc.ID, chunks); err != nil {
		return fmt.Errorf("save chunks: %w", err)
	}

	// 6. Upsert vectors to Qdrant (Qdrant requires UUID or integer IDs).
	// Skipped when embedding/qdrant is not configured (text-only mode).
	if p.embedder != nil && p.qdrant != nil {
		points := make([]QdrantPoint, len(chunks))
		for i, chunk := range chunks {
			payload := map[string]any{
				"chunk_id":    chunk.ID,
				"document_id": doc.ID,
				"user_id":     doc.UserID,
			}
			if doc.ProjectID != nil {
				payload["project_id"] = *doc.ProjectID
			}
			points[i] = QdrantPoint{
				ID:      uuid.New().String(),
				Vector:  allEmbeddings[i],
				Payload: payload,
			}
		}
		if err := p.qdrant.UpsertPoints(ctx, points); err != nil {
			return fmt.Errorf("upsert vectors: %w", err)
		}
	}

	// 7. Update document status
	if err := p.store.UpdateKnowledgeDocStatus(doc.ID, model.KnowledgeDocStatusReady, len(chunks)); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	if p.log != nil {
		p.log.Info("knowledge document processed",
			zap.String("doc_id", doc.ID),
			zap.Int("chunks", len(chunks)))
	}
	return nil
}
