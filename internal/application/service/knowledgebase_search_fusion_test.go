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
