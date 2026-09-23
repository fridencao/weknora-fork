package embedding

import "sync/atomic"

// batchEmbedSizeOverride carries the system_settings value for
// embedding.batch_size. Zero means "nothing pushed yet".
//
// Precedence preserved from the env-only implementation:
//
//	model row GetBatchEmbedSize()  >  DB (this override)  >  ENV  >  5
//
// The model row stays on top because it is a per-model tuning decision
// made next to the model definition, not a deployment-wide default.
var batchEmbedSizeOverride atomic.Int64

// SetBatchEmbedSize pushes the resolved default batch size. Non-positive
// clears the override.
func SetBatchEmbedSize(n int) {
	if n <= 0 {
		batchEmbedSizeOverride.Store(0)
		return
	}
	batchEmbedSizeOverride.Store(int64(n))
}

// ClearBatchEmbedSizeOverride drops the pushed value. Used by tests that
// assert the env-only path.
func ClearBatchEmbedSizeOverride() { batchEmbedSizeOverride.Store(0) }
