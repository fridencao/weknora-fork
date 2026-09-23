package embedding

import "testing"

// fakeBatchModel lets the model-row branch be exercised without a real
// embedder.
type fakeBatchModel struct {
	Embedder
	size int
}

func (f *fakeBatchModel) GetBatchEmbedSize() int { return f.size }

func TestResolveBatchEmbedSizePrecedence(t *testing.T) {
	t.Cleanup(ClearBatchEmbedSizeOverride)
	ClearBatchEmbedSizeOverride()

	// Default when nothing is set.
	t.Setenv("BATCH_EMBED_SIZE", "")
	if got := resolveBatchEmbedSize(&fakeBatchModel{}); got != defaultBatchEmbedSize {
		t.Errorf("default: want %d, got %d", defaultBatchEmbedSize, got)
	}

	// ENV tier.
	t.Setenv("BATCH_EMBED_SIZE", "9")
	if got := resolveBatchEmbedSize(&fakeBatchModel{}); got != 9 {
		t.Errorf("env=9: want 9, got %d", got)
	}

	// Pushed tier beats ENV.
	SetBatchEmbedSize(12)
	if got := resolveBatchEmbedSize(&fakeBatchModel{}); got != 12 {
		t.Errorf("pushed=12 should beat env=9: want 12, got %d", got)
	}

	// Model row beats everything — it is a per-model tuning decision.
	if got := resolveBatchEmbedSize(&fakeBatchModel{size: 3}); got != 3 {
		t.Errorf("model row=3 should beat pushed=12: want 3, got %d", got)
	}

	// Clearing restores ENV.
	ClearBatchEmbedSizeOverride()
	if got := resolveBatchEmbedSize(&fakeBatchModel{}); got != 9 {
		t.Errorf("after clear: want env 9, got %d", got)
	}
}
