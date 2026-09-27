package service

import (
	"context"
	"math"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func TestFuseOrDeduplicate_KeywordOnlyRescalesUnboundedBM25(t *testing.T) {
	t.Parallel()

	got := fuseOrDeduplicate(context.Background(), nil, []*types.IndexWithScore{
		{ChunkID: "strong", Score: 16.1239},
		{ChunkID: "mid", Score: 8.06195},
		{ChunkID: "weak", Score: 4.030975},
	}, nil, nil)

	require.Len(t, got, 3)
	require.Equal(t, "strong", got[0].ChunkID)
	require.InDelta(t, 1.0, got[0].Score, 1e-9)
	require.InDelta(t, 0.5, got[1].Score, 1e-9)
	require.InDelta(t, 0.25, got[2].Score, 1e-9)

	// 0.3*base stays below the composite clamp, so model score can still discriminate.
	require.Less(t, 0.3*got[0].Score, 1.0)
	require.Less(t, 0.3*got[1].Score, 0.3*got[0].Score)
}

func TestFuseOrDeduplicate_KeywordOnlyLeavesUnitIntervalScores(t *testing.T) {
	t.Parallel()

	flat := fuseOrDeduplicate(context.Background(), nil, []*types.IndexWithScore{
		{ChunkID: "a", Score: 1.0},
		{ChunkID: "b", Score: 1.0},
	}, nil, nil)
	require.Len(t, flat, 2)
	require.InDelta(t, 1.0, flat[0].Score, 1e-9)
	require.InDelta(t, 1.0, flat[1].Score, 1e-9)

	bounded := fuseOrDeduplicate(context.Background(), nil, []*types.IndexWithScore{
		{ChunkID: "high", Score: 0.8},
		{ChunkID: "low", Score: 0.4},
	}, nil, nil)
	require.Equal(t, "high", bounded[0].ChunkID)
	require.InDelta(t, 0.8, bounded[0].Score, 1e-9)
	require.InDelta(t, 0.4, bounded[1].Score, 1e-9)
}

func TestFuseOrDeduplicate_VectorOnlyKeepsEmbeddingScores(t *testing.T) {
	t.Parallel()

	got := fuseOrDeduplicate(context.Background(), []*types.IndexWithScore{
		{ChunkID: "near", Score: 0.91},
		{ChunkID: "far", Score: 0.22},
	}, nil, nil, nil)

	require.Equal(t, "near", got[0].ChunkID)
	require.InDelta(t, 0.91, got[0].Score, 1e-9)
	require.InDelta(t, 0.22, got[1].Score, 1e-9)
}

func TestFuseOrDeduplicate_HybridUsesRRFNotRawBM25(t *testing.T) {
	t.Parallel()

	got := fuseOrDeduplicate(context.Background(),
		[]*types.IndexWithScore{{ChunkID: "vec", Score: 0.9}},
		[]*types.IndexWithScore{{ChunkID: "kw", Score: 16.1239}},
		nil, nil,
	)

	require.Len(t, got, 2)
	for _, hit := range got {
		require.Greater(t, hit.Score, 0.0)
		require.Less(t, hit.Score, 1.0)
		require.NotEqual(t, 16.1239, hit.Score)
	}
}

func TestRescaleUnboundedScores_IgnoresNonFiniteWhenFindingMax(t *testing.T) {
	t.Parallel()

	hits := []*types.IndexWithScore{
		{ChunkID: "nan", Score: math.NaN()},
		{ChunkID: "top", Score: 10},
		{ChunkID: "low", Score: 5},
		nil,
	}
	rescaleUnboundedScores(hits)
	require.Equal(t, 0.0, hits[0].Score)
	require.InDelta(t, 1.0, hits[1].Score, 1e-9)
	require.InDelta(t, 0.5, hits[2].Score, 1e-9)
}

func TestFuseOrDeduplicate_ThreeWayRRFBoostsGraphEndorsedChunks(t *testing.T) {
	t.Parallel()

	vec := []*types.IndexWithScore{
		{ChunkID: "a", Score: 0.9},
		{ChunkID: "b", Score: 0.8},
		{ChunkID: "c", Score: 0.7},
	}
	kw := []*types.IndexWithScore{
		{ChunkID: "b", Score: 1.0},
		{ChunkID: "d", Score: 0.5},
	}
	// 图谱通道独立背书 chunk "c"（向量第 3、关键词未命中）。
	graph := []*types.IndexWithScore{{ChunkID: "c", Score: 1.0}}

	three := fuseOrDeduplicate(context.Background(), vec, kw, graph, nil)
	two := fuseOrDeduplicate(context.Background(), vec, kw, nil, nil)

	require.Len(t, three, 4)
	// "c" 在三通道下应排到向量第 2 名 "b" 之前或显著缩小差距（图权重加分）。
	rankIn := func(ids []string, id string) int {
		for i, v := range ids {
			if v == id {
				return i
			}
		}
		return -1
	}
	threeIDs := make([]string, 0, len(three))
	twoIDs := make([]string, 0, len(two))
	for _, r := range three {
		threeIDs = append(threeIDs, r.ChunkID)
	}
	for _, r := range two {
		twoIDs = append(twoIDs, r.ChunkID)
	}
	require.Less(t, rankIn(threeIDs, "c"), rankIn(twoIDs, "c"),
		"graph endorsement should improve chunk c's rank")
}

func TestFuseOrDeduplicate_TagsActualChannelsPerHit(t *testing.T) {
	t.Parallel()

	vec := []*types.IndexWithScore{
		{ChunkID: "v-only", Score: 0.9},
		{ChunkID: "v+k", Score: 0.8},
		{ChunkID: "v+g", Score: 0.7},
	}
	kw := []*types.IndexWithScore{{ChunkID: "v+k", Score: 1.0}}
	graph := []*types.IndexWithScore{{ChunkID: "v+g", Score: 1.0}}

	got := fuseOrDeduplicate(context.Background(), vec, kw, graph, nil)
	channels := make(map[string][]types.RetrieverType, len(got))
	for _, r := range got {
		channels[r.ChunkID] = r.Channels
	}

	// M5-3：标签反映**实际参与**的通道，按 vector→keywords→graph 顺序。
	require.Equal(t, []types.RetrieverType{types.VectorRetrieverType}, channels["v-only"])
	require.Equal(t, []types.RetrieverType{
		types.VectorRetrieverType, types.KeywordsRetrieverType}, channels["v+k"])
	require.Equal(t, []types.RetrieverType{
		types.VectorRetrieverType, types.GraphRetrieverType}, channels["v+g"])
}

func TestFuseOrDeduplicate_KeepsGraphRecallChannelTag(t *testing.T) {
	t.Parallel()

	// graphRecallForSearch 产出的结果自带 graph 标签；融合不得把它覆盖成
	// 首通道标签（正是 M5-3 要修的显示层缺口）。
	graph := []*types.IndexWithScore{{
		ChunkID:  "graph-only",
		Score:    1.0,
		Channels: []types.RetrieverType{types.GraphRetrieverType},
	}}

	got := fuseOrDeduplicate(context.Background(), nil,
		[]*types.IndexWithScore{{ChunkID: "kw", Score: 1.0}}, graph, nil)

	byID := map[string][]types.RetrieverType{}
	for _, r := range got {
		byID[r.ChunkID] = r.Channels
	}
	require.Equal(t, []types.RetrieverType{types.GraphRetrieverType}, byID["graph-only"])
}

func TestRetrievalConfig_GraphWeightDefault(t *testing.T) {
	t.Parallel()

	require.InDelta(t, 0.2, (*types.RetrievalConfig)(nil).GetEffectiveRRFGraphWeight(), 1e-9)
	require.InDelta(t, 0.35, (&types.RetrievalConfig{RRFGraphWeight: 0.35}).GetEffectiveRRFGraphWeight(), 1e-9)
}

func TestRetrievalConfig_GraphChannelEnabled(t *testing.T) {
	t.Parallel()

	require.True(t, (*types.RetrievalConfig)(nil).GetGraphChannelEnabled(true))
	require.False(t, (*types.RetrievalConfig)(nil).GetGraphChannelEnabled(false))
	off := false
	require.False(t, (&types.RetrievalConfig{GraphChannelEnabled: &off}).GetGraphChannelEnabled(true),
		"UI 显式 false 应覆盖 env 默认 true")
	on := true
	require.True(t, (&types.RetrievalConfig{GraphChannelEnabled: &on}).GetGraphChannelEnabled(false),
		"UI 显式 true 应覆盖 env 默认 false")
}

// ---- M6-3（docs/16 RW1）：图谱独有结果 top-k 保底名额 ----

func TestEnsureGraphOnlySlots_PromotesGraphOnlyIntoTail(t *testing.T) {
	t.Parallel()

	fused := []*types.IndexWithScore{
		{ChunkID: "v1", Score: 0.02, Channels: []types.RetrieverType{types.VectorRetrieverType}},
		{ChunkID: "v2", Score: 0.018, Channels: []types.RetrieverType{types.VectorRetrieverType}},
		{ChunkID: "vk", Score: 0.016, Channels: []types.RetrieverType{types.VectorRetrieverType, types.KeywordsRetrieverType}},
		{ChunkID: "k1", Score: 0.010, Channels: []types.RetrieverType{types.KeywordsRetrieverType}},
		{ChunkID: "g1", Score: 0.004, Channels: []types.RetrieverType{types.GraphRetrieverType}},
		{ChunkID: "g2", Score: 0.003, Channels: []types.RetrieverType{types.GraphRetrieverType}},
	}

	out := ensureGraphOnlySlots(context.Background(), fused, 4, 2)
	require.Len(t, out, 4, "保底名额替换队尾，不扩容 top-k")
	got := map[string]bool{}
	for _, r := range out {
		got[r.ChunkID] = true
	}
	require.True(t, got["g1"] && got["g2"], "两个 graph-only 都应进 top-k")
	require.False(t, got["k1"], "队尾最弱的 keyword-only 被替换")
}

func TestEnsureGraphOnlySlots_RespectsExistingGraphOnly(t *testing.T) {
	t.Parallel()

	fused := []*types.IndexWithScore{
		{ChunkID: "g1", Score: 0.9, Channels: []types.RetrieverType{types.GraphRetrieverType}},
		{ChunkID: "g2", Score: 0.8, Channels: []types.RetrieverType{types.GraphRetrieverType}},
		{ChunkID: "v1", Score: 0.7, Channels: []types.RetrieverType{types.VectorRetrieverType}},
		{ChunkID: "v2", Score: 0.6, Channels: []types.RetrieverType{types.VectorRetrieverType}},
	}
	out := ensureGraphOnlySlots(context.Background(), fused, 4, 2)
	require.Len(t, out, 4)
	require.Equal(t, "g1", out[0].ChunkID, "已满足名额时不动排序")
}

func TestEnsureGraphOnlySlots_OffOrNoCandidates(t *testing.T) {
	t.Parallel()

	fused := []*types.IndexWithScore{
		{ChunkID: "v1", Score: 0.9, Channels: []types.RetrieverType{types.VectorRetrieverType}},
		{ChunkID: "v2", Score: 0.8, Channels: []types.RetrieverType{types.VectorRetrieverType}},
	}
	// slots=0（功能关）直接截断
	out := ensureGraphOnlySlots(context.Background(), fused, 1, 0)
	require.Len(t, out, 1)
	require.Equal(t, "v1", out[0].ChunkID)

	// 无 graph-only 候补时不扩容不替换
	withGraph := []*types.IndexWithScore{
		{ChunkID: "vg", Score: 0.9, Channels: []types.RetrieverType{types.VectorRetrieverType, types.GraphRetrieverType}},
		{ChunkID: "v2", Score: 0.8, Channels: []types.RetrieverType{types.VectorRetrieverType}},
	}
	out2 := ensureGraphOnlySlots(context.Background(), withGraph, 2, 2)
	require.Len(t, out2, 2)
	require.Equal(t, "vg", out2[0].ChunkID, "vg 是多通道命中不算 graph-only")
}

func TestRetrievalConfig_GraphSlotsResolution(t *testing.T) {
	// t.Setenv 与 t.Parallel 互斥（env 是进程级状态），本组不并行
	t.Run("off by default", func(t *testing.T) {
		require.Equal(t, 0, (*types.RetrievalConfig)(nil).GetEffectiveRRFGraphSlots())
		require.Equal(t, 0, (&types.RetrievalConfig{}).GetEffectiveRRFGraphSlots())
	})

	t.Run("explicit setting wins over env", func(t *testing.T) {
		t.Setenv("STARKB_RRF_GRAPH_MIN_SLOTS", "1")
		n := 3
		cfg := &types.RetrievalConfig{RRFGraphMinSlots: &n}
		require.Equal(t, 3, cfg.GetEffectiveRRFGraphSlots())
	})

	t.Run("env fallback", func(t *testing.T) {
		t.Setenv("STARKB_RRF_GRAPH_MIN_SLOTS", "2")
		require.Equal(t, 2, (&types.RetrievalConfig{}).GetEffectiveRRFGraphSlots())
		t.Setenv("STARKB_RRF_GRAPH_MIN_SLOTS", "not-a-number")
		require.Equal(t, 0, (&types.RetrievalConfig{}).GetEffectiveRRFGraphSlots())
	})
}
