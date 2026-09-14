package clawsynapse

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/embedding"
	"trustmesh/backend/internal/knowledge"
	"trustmesh/backend/internal/project"
	"trustmesh/backend/internal/store"
)

// stubEmbedder is a minimal embedding.Client used to prove that the embedder
// dependency reaches the handler through WebhookDeps. It never performs I/O.
type stubEmbedder struct{ dim int }

func (s stubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, s.dim)
	}
	return out, nil
}

func (s stubEmbedder) Dimension() int { return s.dim }

var _ embedding.Client = stubEmbedder{}

// TestWebhookHandlerConfigViaDI verifies that every configuration field bound
// through WebhookDeps is actually installed on the constructed handler, and
// that omitted (nil) optional dependencies degrade gracefully. This locks in
// the construction-time dependency injection that replaced the runtime setters
// (T0.10).
func TestWebhookHandlerConfigViaDI(t *testing.T) {
	st := store.New()
	logger := zap.NewNop()
	cli := NewClient("http://127.0.0.1:1", time.Second)
	embedder := stubEmbedder{dim: 16}
	qdrant := knowledge.NewQdrantClient("http://127.0.0.1:6333", 16)
	pfs := project.NewLocalFileStorage(t.TempDir())

	var notifiedID string
	onMeeting := func(meetingID string) { notifiedID = meetingID }

	h := NewWebhookHandler(WebhookDeps{
		Store:              st,
		Client:             cli,
		Log:                logger,
		Embedder:           embedder,
		Qdrant:             qdrant,
		ProjectFileStorage: pfs,
		ExternalURL:        "https://tm.example.com",
		JWTSecret:          []byte("di-secret"),
		DownloadTTL:        15 * time.Minute,
		OnMeetingActivity:  onMeeting,
	})

	if h.store != st {
		t.Fatalf("store not injected: got %p want %p", h.store, st)
	}
	if h.client != cli {
		t.Fatalf("client not injected: got %p want %p", h.client, cli)
	}
	if h.log != logger {
		t.Fatalf("log not injected")
	}
	if h.embedder == nil || h.embedder.Dimension() != 16 {
		t.Fatalf("embedder not injected: %#v", h.embedder)
	}
	if h.qdrant != qdrant {
		t.Fatalf("qdrant not injected: got %p want %p", h.qdrant, qdrant)
	}
	if h.projectFileStorage != pfs {
		t.Fatalf("projectFileStorage not injected: got %#v want %#v", h.projectFileStorage, pfs)
	}
	if h.externalURL != "https://tm.example.com" {
		t.Fatalf("externalURL not injected: got %q", h.externalURL)
	}
	if string(h.jwtSecret) != "di-secret" {
		t.Fatalf("jwtSecret not injected: got %q", string(h.jwtSecret))
	}
	if h.downloadTTL != 15*time.Minute {
		t.Fatalf("downloadTTL not injected: got %v", h.downloadTTL)
	}
	if h.onMeetingActivity == nil {
		t.Fatalf("onMeetingActivity not injected")
	}
	h.onMeetingActivity("m-42")
	if notifiedID != "m-42" {
		t.Fatalf("onMeetingActivity not wired: got %q", notifiedID)
	}

	// Optional dependencies omitted → zero values, no panics.
	minimal := NewWebhookHandler(WebhookDeps{Store: st})
	if minimal.externalURL != "" || minimal.jwtSecret != nil || minimal.downloadTTL != 0 {
		t.Fatalf("omitted agent-file config should stay zero-valued: %#v", minimal)
	}
	if minimal.embedder != nil || minimal.qdrant != nil || minimal.projectFileStorage != nil {
		t.Fatalf("omitted optional deps should stay nil")
	}
	if minimal.onMeetingActivity != nil {
		t.Fatalf("omitted notifier should stay nil")
	}
}

// TestWebhookHandlerNoConfigRace drives many goroutines that concurrently read
// every configuration field (and the accessors that consume them) exactly as
// request handlers do while the server is serving traffic. Because config is
// bound once at construction and never mutated afterwards, this must be clean
// under `go test -race`. Before T0.10 the equivalent access pattern raced
// against the runtime setters that mutated the same fields after construction.
func TestWebhookHandlerNoConfigRace(t *testing.T) {
	st := store.New()
	h := NewWebhookHandler(WebhookDeps{
		Store:              st,
		Client:             NewClient("http://127.0.0.1:1", time.Second),
		Log:                zap.NewNop(),
		Embedder:           stubEmbedder{dim: 8},
		Qdrant:             knowledge.NewQdrantClient("http://127.0.0.1:6333", 8),
		ProjectFileStorage: project.NewLocalFileStorage(t.TempDir()),
		ExternalURL:        "https://tm.example.com",
		JWTSecret:          []byte("race-secret"),
		DownloadTTL:        15 * time.Minute,
		OnMeetingActivity:  func(string) {},
	})

	const goroutines = 64
	const iterations = 300

	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				// Hash-free reads of every injected field keep the field usage
				// real to the race detector without depending on formatting.
				_ = h.store
				_ = h.client
				_ = h.log
				_ = h.embedder
				_ = h.qdrant
				_ = h.projectFileStorage
				_ = h.externalURL
				_ = h.jwtSecret
				_ = h.downloadTTL
				_ = h.onMeetingActivity
				// Accessors that consume the config fields (no I/O on the hot path).
				_ = h.enrichAttachedFiles(nil)
				_ = h.artifactDownloadURL("t-race")
				_ = h.artifactDownloadURL("")
				if h.onMeetingActivity != nil {
					h.onMeetingActivity("m-race")
				}
			}
		}()
	}
	close(start)
	wg.Wait()
}
