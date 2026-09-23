package embedding

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/Tencent/WeKnora/internal/models/utils"
	"github.com/panjf2000/ants/v2"
)

type batchEmbedder struct {
	pool *ants.Pool
}

func NewBatchEmbedder(pool *ants.Pool) EmbedderPooler {
	return &batchEmbedder{pool: pool}
}

type textEmbedding struct {
	text    string
	results []float32
}

// defaultBatchEmbedSize is the last-resort batch size when neither the
// model row, the pushed setting, nor the env var supplies one.
const defaultBatchEmbedSize = 5

// resolveBatchEmbedSize applies the documented precedence:
//
//	model row GetBatchEmbedSize()  >  pushed setting  >  ENV  >  default
//
// The model row stays on top: it is a per-model tuning decision recorded
// next to the model definition, not a deployment-wide default. The pushed
// value is the system_settings tier; ENV is the middle tier of the 3-tier
// resolver, kept so an emergency override still works.
func resolveBatchEmbedSize(model Embedder) int {
	if p, ok := model.(interface{ GetBatchEmbedSize() int }); ok {
		if v := p.GetBatchEmbedSize(); v > 0 {
			return v
		}
	}
	if n := batchEmbedSizeOverride.Load(); n > 0 {
		return int(n)
	}
	if s := os.Getenv("BATCH_EMBED_SIZE"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return defaultBatchEmbedSize
}

func (e *batchEmbedder) BatchEmbedWithPool(ctx context.Context, model Embedder, texts []string) ([][]float32, error) {
	// Create goroutine pool for concurrent processing of document chunks
	var wg sync.WaitGroup
	var mu sync.Mutex  // For synchronizing access to error
	var firstErr error // Record the first error that occurs
	batchSize := resolveBatchEmbedSize(model)
	textEmbeddings := utils.MapSlice(texts, func(text string) *textEmbedding {
		return &textEmbedding{text: text}
	})

	// Function to process each document chunk
	processChunk := func(texts []*textEmbedding) func() {
		return func() {
			defer wg.Done()
			// If an error has already occurred, don't continue processing
			if firstErr != nil {
				return
			}
			// Embed text
			embedding, err := model.BatchEmbed(ctx, utils.MapSlice(texts, func(text *textEmbedding) string {
				return text.text
			}))
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			if len(embedding) != len(texts) {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("embedding model returned %d embeddings for %d inputs", len(embedding), len(texts))
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			for i, text := range texts {
				if text == nil {
					continue
				}
				text.results = embedding[i]
			}
			mu.Unlock()
		}
	}

	// Submit all tasks to the goroutine pool
	for _, texts := range utils.ChunkSlice(textEmbeddings, batchSize) {
		wg.Add(1)
		err := e.pool.Submit(processChunk(texts))
		if err != nil {
			return nil, err
		}
	}

	// Wait for all tasks to complete
	wg.Wait()

	// Check if any errors occurred
	if firstErr != nil {
		return nil, firstErr
	}

	results := utils.MapSlice(textEmbeddings, func(text *textEmbedding) []float32 {
		return text.results
	})
	return results, nil
}
