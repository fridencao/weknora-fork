package config

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// RetrievalDefaults 是检索参数的**唯一缺省来源**（ADR-008 决策 2）：
// 租户「检索设置」退休后，对话路径与检索 API 路径都从这里取缺省值。
func TestRetrievalDefaultsMapsConversationValues(t *testing.T) {
	conv := &ConversationConfig{
		EmbeddingTopK:       30,
		VectorThreshold:     0.2,
		KeywordThreshold:    0.35,
		RerankTopK:          30,
		RerankThreshold:     0.3,
		RRFK:                80,
		RRFVectorWeight:     0.6,
		RRFKeywordWeight:    0.4,
		RRFGraphWeight:      0.25,
		GraphChannelEnabled: boolPtr(true),
	}

	rc := conv.RetrievalDefaults()

	require.Equal(t, 30, rc.EmbeddingTopK)
	require.Equal(t, 0.2, rc.VectorThreshold)
	require.Equal(t, 0.35, rc.KeywordThreshold)
	require.Equal(t, 30, rc.RerankTopK)
	require.Equal(t, 0.3, rc.RerankThreshold)
	require.Equal(t, 80, rc.GetEffectiveRRFK())
	require.Equal(t, 0.6, rc.RRFVectorWeight)
	require.Equal(t, 0.4, rc.RRFKeywordWeight)
	require.Equal(t, 0.25, rc.GetEffectiveRRFGraphWeight())
	require.NotNil(t, rc.GraphChannelEnabled)
	require.True(t, *rc.GraphChannelEnabled)
}

// 未配置的字段必须落到内置默认，而不是 0——否则"不写 YAML"会悄悄把检索
// 参数打成 0（例如 RerankTopK=0 会截断掉全部结果）。
func TestRetrievalDefaultsFallBackToBuiltins(t *testing.T) {
	rc := (&ConversationConfig{}).RetrievalDefaults()

	require.Equal(t, types.DefaultRetrievalTopK, rc.GetEffectiveEmbeddingTopK())
	require.Equal(t, 0.15, rc.GetEffectiveVectorThreshold())
	require.Equal(t, 0.3, rc.GetEffectiveKeywordThreshold())
	require.Equal(t, 10, rc.GetEffectiveRerankTopK())
	require.Equal(t, 60, rc.GetEffectiveRRFK())
	vector, keyword := rc.GetEffectiveRRFWeights()
	require.Equal(t, 0.7, vector)
	require.Equal(t, 0.3, keyword)
	require.Equal(t, 0.2, rc.GetEffectiveRRFGraphWeight())
	require.Nil(t, rc.GraphChannelEnabled, "未配置时应为 nil，交由调用方回落环境变量")
}

// nil 安全：服务层不必自己判空 cfg / cfg.Conversation。
func TestRetrievalDefaultsNilSafe(t *testing.T) {
	var conv *ConversationConfig
	require.NotNil(t, conv.RetrievalDefaults())

	var cfg *Config
	require.NotNil(t, cfg.RetrievalDefaults())

	require.Equal(t, types.DefaultRetrievalTopK, (&Config{}).RetrievalDefaults().GetEffectiveEmbeddingTopK())
}

func boolPtr(v bool) *bool { return &v }
