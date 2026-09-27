package types

import (
	"database/sql/driver"
	"encoding/json"
	"os"
	"strconv"
)

// RetrievalConfig holds the global retrieval/search configuration for a tenant.
// This replaces the retrieval-related fields previously scattered in ConversationConfig
// and ChatHistoryConfig. Both knowledge search and message search share these parameters.
//
// Stored as a JSONB column on the tenants table, managed via the settings UI
// at /tenants/kv/retrieval-config.
type RetrievalConfig struct {
	// EmbeddingTopK is the maximum number of chunks returned by vector search (default: 50)
	EmbeddingTopK int `json:"embedding_top_k"`
	// VectorThreshold is the minimum vector similarity score (0-1, default: 0.15)
	VectorThreshold float64 `json:"vector_threshold"`
	// KeywordThreshold is the minimum keyword match score (0-1, default: 0.3)
	KeywordThreshold float64 `json:"keyword_threshold"`
	// RerankTopK is the maximum number of results after reranking (default: 10)
	RerankTopK int `json:"rerank_top_k"`
	// RerankThreshold is the minimum rerank score (-10 to 10, default: 0.2)
	RerankThreshold float64 `json:"rerank_threshold"`
	// RerankModelID is the ID of the rerank model to use (required for search)
	RerankModelID string `json:"rerank_model_id"`

	// RRFK is the smoothing constant of Reciprocal Rank Fusion. Larger values
	// flatten the curve, reducing the bias towards top-1 results.
	// Default: 60. Sensible range: 30..100 depending on corpus size.
	RRFK int `json:"rrf_k,omitempty"`
	// RRFVectorWeight is the weight applied to the vector retriever inside RRF.
	// RRFVectorWeight + RRFKeywordWeight should usually sum to 1.0 but the math
	// works for any positive weights. Default: 0.7.
	RRFVectorWeight float64 `json:"rrf_vector_weight,omitempty"`
	// RRFKeywordWeight is the keyword counterpart. Default: 0.3.
	RRFKeywordWeight float64 `json:"rrf_keyword_weight,omitempty"`
	// RRFGraphWeight is the LightRAG graph channel weight (M3 three-way RRF,
	// docs/07 G3). The graph channel is additive — vector/keyword keep their
	// defaults so the graph acts as a supplement, not a dilution. Default: 0.2.
	RRFGraphWeight float64 `json:"rrf_graph_weight,omitempty"`
	// RRFGraphMinSlots guarantees this many graph-only chunks inside the final
	// top-k (M6-3, docs/16 RW1). RRF weight 0.2 rarely lifts graph-only chunks
	// past vector/keyword hits, so the graph's unique evidence usually never
	// reaches the answer context. The guarantee swaps the weakest tail entries
	// for the highest-ranked graph-only chunks beyond the cut — total count
	// stays at top-k. Pointer so "unset" (nil) stays distinct from 0.
	// Resolution order: explicit value > STARKB_RRF_GRAPH_MIN_SLOTS env > 0 (off).
	RRFGraphMinSlots *int `json:"rrf_graph_min_slots,omitempty"`
	// GraphChannelEnabled toggles the LightRAG graph recall channel (docs/07
	// G3). Nil means "not configured in the UI": the deployment default
	// (GRAPH_CHANNEL_ENABLED env) applies, so existing deployments keep their
	// behavior. Settings UI sets it explicitly per tenant.
	GraphChannelEnabled *bool `json:"graph_channel_enabled,omitempty"`
}

// DefaultRetrievalTopK is the retrieval depth used when a caller supplies no
// usable TopK / MatchCount. HybridSearch also floors its over-retrieval pool
// at this value, so the fallback and the pool it draws from stay in step by
// construction rather than by two independently maintained literals.
const DefaultRetrievalTopK = 50

// GetEffectiveEmbeddingTopK returns EmbeddingTopK with a fallback default.
func (c *RetrievalConfig) GetEffectiveEmbeddingTopK() int {
	if c == nil || c.EmbeddingTopK <= 0 {
		return DefaultRetrievalTopK
	}
	return c.EmbeddingTopK
}

// GetEffectiveVectorThreshold returns VectorThreshold with a fallback default.
func (c *RetrievalConfig) GetEffectiveVectorThreshold() float64 {
	if c == nil || c.VectorThreshold <= 0 {
		return 0.15
	}
	return c.VectorThreshold
}

// GetEffectiveKeywordThreshold returns KeywordThreshold with a fallback default.
func (c *RetrievalConfig) GetEffectiveKeywordThreshold() float64 {
	if c == nil || c.KeywordThreshold <= 0 {
		return 0.3
	}
	return c.KeywordThreshold
}

// GetEffectiveRerankTopK returns RerankTopK with a fallback default.
func (c *RetrievalConfig) GetEffectiveRerankTopK() int {
	if c == nil || c.RerankTopK <= 0 {
		return 10
	}
	return c.RerankTopK
}

// GetEffectiveRerankThreshold returns RerankThreshold with a fallback default.
func (c *RetrievalConfig) GetEffectiveRerankThreshold() float64 {
	if c == nil {
		return 0.2
	}
	return c.RerankThreshold
}

// GetEffectiveRRFK returns the RRF smoothing constant with a fallback default.
func (c *RetrievalConfig) GetEffectiveRRFK() int {
	if c == nil || c.RRFK <= 0 {
		return 60
	}
	return c.RRFK
}

// GetEffectiveRRFWeights returns vector / keyword weights with sensible defaults.
// When neither weight is set explicitly, returns 0.7 / 0.3.
func (c *RetrievalConfig) GetEffectiveRRFWeights() (vector, keyword float64) {
	if c == nil || (c.RRFVectorWeight == 0 && c.RRFKeywordWeight == 0) {
		return 0.7, 0.3
	}
	v := c.RRFVectorWeight
	k := c.RRFKeywordWeight
	if v <= 0 {
		v = 0.7
	}
	if k <= 0 {
		k = 0.3
	}
	return v, k
}

// GetEffectiveRRFGraphWeight returns the graph channel weight (default 0.2).
// A zero/negative value falls back to the default, never to "disabled" —
// disabling is the graph channel's own switch (GraphChannelEnabled), which
// simply yields an empty graph candidate set.
func (c *RetrievalConfig) GetEffectiveRRFGraphWeight() float64 {
	if c == nil || c.RRFGraphWeight <= 0 {
		return 0.2
	}
	return c.RRFGraphWeight
}

// GetEffectiveRRFGraphSlots returns the graph-only guaranteed-slot count
// (M6-3). Resolution order: explicit tenant setting > deployment env
// STARKB_RRF_GRAPH_MIN_SLOTS > 0 (feature off). Returning 0 keeps the legacy
// pure-RRF ordering, so existing deployments see zero behavior change until
// the operator opts in.
func (c *RetrievalConfig) GetEffectiveRRFGraphSlots() int {
	if c != nil && c.RRFGraphMinSlots != nil && *c.RRFGraphMinSlots > 0 {
		return *c.RRFGraphMinSlots
	}
	if v := os.Getenv("STARKB_RRF_GRAPH_MIN_SLOTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// GetGraphChannelEnabled resolves the graph channel switch: an explicit UI
// setting wins; otherwise the deployment env default applies.
func (c *RetrievalConfig) GetGraphChannelEnabled(envDefault bool) bool {
	if c != nil && c.GraphChannelEnabled != nil {
		return *c.GraphChannelEnabled
	}
	return envDefault
}

// Value implements the driver.Valuer interface for database serialization
func (c RetrievalConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface for database deserialization
func (c *RetrievalConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}
