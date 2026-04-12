package agent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	chromem "github.com/philippgille/chromem-go"

	"github.com/chickenzord/zlaw/internal/config"
	"github.com/chickenzord/zlaw/internal/llm"
	"github.com/chickenzord/zlaw/internal/llm/auth"
)

const (
	semanticCollectionName = "memories"
	semanticHashKey        = "content_hash"
)

// SemanticMemoryStore wraps MarkdownFileStore with a chromem-go vector index for
// semantic (embedding-based) similarity search. The markdown files remain the
// canonical source of truth; the vector index under <baseDir>/.index/ is a
// regenerable cache.
//
// Implements MemoryStore — drop-in replacement for MarkdownFileStore.
type SemanticMemoryStore struct {
	markdown *MarkdownFileStore
	db       *chromem.DB
	coll     *chromem.Collection
	logger   *slog.Logger
}

// NewSemanticMemoryStore creates a SemanticMemoryStore rooted at baseDir.
// The vector index is persisted to <baseDir>/.index/.
//
// On startup it scans all markdown files and embeds any that are missing from
// the index or whose content has changed (content-hash diffing). Unchanged
// entries are left as-is to avoid redundant API calls.
//
// embedFunc is called for each document that needs (re-)embedding and for every
// search query. Use NewEmbeddingFunc to build one from agent config.
func NewSemanticMemoryStore(ctx context.Context, baseDir string, embedFunc chromem.EmbeddingFunc, logger *slog.Logger) (*SemanticMemoryStore, error) {
	indexDir := filepath.Join(baseDir, ".index")

	db, err := chromem.NewPersistentDB(indexDir, false)
	if err != nil {
		return nil, fmt.Errorf("semantic memory: open index at %s: %w", indexDir, err)
	}

	coll, err := db.GetOrCreateCollection(semanticCollectionName, nil, embedFunc)
	if err != nil {
		return nil, fmt.Errorf("semantic memory: create collection: %w", err)
	}

	if logger == nil {
		logger = slog.Default()
	}
	s := &SemanticMemoryStore{
		markdown: NewMarkdownFileStore(baseDir),
		db:       db,
		coll:     coll,
		logger:   logger,
	}

	if err := s.rebuildIndex(ctx); err != nil {
		return nil, fmt.Errorf("semantic memory: rebuild index: %w", err)
	}

	return s, nil
}

// Save writes the memory to disk and upserts its embedding in the vector index.
func (s *SemanticMemoryStore) Save(m Memory) error {
	if err := s.markdown.Save(m); err != nil {
		return err
	}
	// Load the saved version to capture the timestamps set by Save.
	saved, err := s.markdown.Load(m.ID)
	if err != nil {
		return err
	}
	ctx := context.Background()
	return s.upsertIndex(ctx, saved)
}

// Delete removes the memory from disk and from the vector index.
func (s *SemanticMemoryStore) Delete(id string) error {
	if err := s.markdown.Delete(id); err != nil {
		return err
	}
	_ = s.coll.Delete(context.Background(), nil, nil, id)
	return nil
}

// List returns all memories. Delegates to the markdown store.
func (s *SemanticMemoryStore) List() ([]Memory, error) {
	return s.markdown.List()
}

// Search returns up to 10 memories ranked by semantic similarity to the query.
// When keywords is empty it returns all memories. Falls back to the markdown
// store on index errors.
func (s *SemanticMemoryStore) Search(keywords []string) ([]Memory, error) {
	if len(keywords) == 0 {
		return s.markdown.List()
	}

	count := s.coll.Count()
	if count == 0 {
		return nil, nil
	}

	n := count
	if n > 10 {
		n = 10
	}

	ctx := context.Background()
	query := strings.Join(keywords, " ")

	results, err := s.coll.Query(ctx, query, n, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}

	s.logger.Debug("semantic search", "query", query, "index_size", count, "results", len(results))

	memories := make([]Memory, 0, len(results))
	for _, r := range results {
		m, err := s.markdown.Load(r.ID)
		if err != nil {
			// Stale index entry — file was deleted without updating the index.
			continue
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// rebuildIndex ensures every markdown file has an up-to-date index entry.
// It skips files whose content hash matches the stored entry.
func (s *SemanticMemoryStore) rebuildIndex(ctx context.Context) error {
	memories, err := s.markdown.List()
	if err != nil {
		return err
	}

	for _, m := range memories {
		existing, err := s.coll.GetByID(ctx, m.ID)
		if err == nil && existing.Metadata[semanticHashKey] == contentHash(m.Content) {
			continue // already indexed and unchanged
		}
		if err := s.upsertIndex(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// upsertIndex removes any existing entry for m.ID and adds a fresh one.
// chromem-go auto-embeds the content using the collection's EmbeddingFunc.
func (s *SemanticMemoryStore) upsertIndex(ctx context.Context, m Memory) error {
	_ = s.coll.Delete(ctx, nil, nil, m.ID) // remove stale entry; ignore error
	return s.coll.AddDocument(ctx, chromem.Document{
		ID:      m.ID,
		Content: m.Content,
		Metadata: map[string]string{
			semanticHashKey: contentHash(m.Content),
		},
	})
}

// contentHash returns a hex SHA-256 of text for change detection.
func contentHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h)
}

// NewEmbeddingFunc builds a chromem.EmbeddingFunc from the given embedder
// config using the same preset and credentials infrastructure as the LLM client.
// The auth token is resolved once at startup — suitable for static API keys.
func NewEmbeddingFunc(cfg config.EmbedderConfig, credPath string) (chromem.EmbeddingFunc, error) {
	if cfg.Backend == "" {
		return nil, fmt.Errorf("embedder: backend is required")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("embedder: model is required")
	}

	preset, err := llm.LookupPreset(cfg.Backend)
	if err != nil {
		return nil, fmt.Errorf("embedder: %w", err)
	}

	baseURL := preset.BaseURL
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	// Strip any path suffix from the preset URL — embedding endpoints are
	// always at /v1/embeddings relative to the API root.
	baseURL = strings.TrimRight(baseURL, "/")

	if credPath == "" {
		credPath = auth.DefaultCredentialsPath()
	}
	src, err := auth.NewTokenSourceFromStore(credPath, cfg.AuthProfile)
	if err != nil {
		return nil, fmt.Errorf("embedder: load auth profile %q: %w", cfg.AuthProfile, err)
	}
	token, err := src.Token(context.Background())
	if err != nil {
		return nil, fmt.Errorf("embedder: get token: %w", err)
	}

	return chromem.NewEmbeddingFuncOpenAICompat(baseURL, token, cfg.Model, nil), nil
}
