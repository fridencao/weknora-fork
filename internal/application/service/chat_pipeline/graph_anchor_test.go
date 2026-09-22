package chatpipeline

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

// A0 回归（docs/08）：图谱证据回跳。
//
// 修复前真实数据表现：LightRAG 产出 `{docID}-chunk-NNN`（46 字符），
// 旧解析器要求 73 字符 → 恒失败 → 图谱通道静默召回 0 条。

const (
	realDocID  = "12a7052c-307b-4850-8789-21e67b0e8316"
	realKey    = realDocID + "-chunk-000"
	otherDocID = "ae9e5920-4a23-4956-841b-4d82a918f47a"
)

// ---- 解析器 ----

func TestParseLightragChunkKey_RealFormat(t *testing.T) {
	t.Parallel()

	docID, key, err := ParseLightragChunkKey(realKey)
	require.NoError(t, err, "真实 LightRAG key 必须能解析（旧实现按 73 字符强校验，此处恒失败）")
	require.Equal(t, realDocID, docID)
	require.Equal(t, realKey, key)

	// 三位以上序号、非 UUID 文档名同样适用
	docID, _, err = ParseLightragChunkKey("my-doc-chunk-1234")
	require.NoError(t, err)
	require.Equal(t, "my-doc", docID)
}

func TestParseLightragChunkKey_LegacyAndInvalid(t *testing.T) {
	t.Parallel()

	// docs/02 §7 定长设计仍兼容（历史数据/单测构造）
	legacy := realDocID + "-" + otherDocID
	require.Len(t, legacy, LightragChunkKeyLen)
	docID, _, err := ParseLightragChunkKey(legacy)
	require.NoError(t, err)
	require.Equal(t, realDocID, docID)

	for _, bad := range []string{"", "no-suffix", "doc-chunk-", "doc-chunk-abc", "doc-chunk"} {
		_, _, err := ParseLightragChunkKey(bad)
		require.Error(t, err, "非法 key 应报错: %q", bad)
	}
}

// ---- 锚点提取 ----

func TestParseGraphChunkAnchors_OrderedDeduped(t *testing.T) {
	t.Parallel()

	content := "<!--sbk:p001-b001-->正文<!--sbk:p001-b002-->更多" +
		"<!--sbk:p001-b001--><!--sbk:p002-b001-->"
	require.Equal(t, []string{"p001-b001", "p001-b002", "p002-b001"},
		ParseGraphChunkAnchors(content), "锚点须保序去重")

	require.Nil(t, ParseGraphChunkAnchors(""))
	require.Nil(t, ParseGraphChunkAnchors("无锚点正文"))
}

// ---- 证据收集 ----

func TestCollectGraphEvidence_FiltersAndDedupes(t *testing.T) {
	t.Parallel()

	data := &LightragQueryData{}
	data.Data.Chunks = []struct {
		ChunkID  string `json:"chunk_id"`
		Content  string `json:"content"`
		FilePath string `json:"file_path"`
	}{
		{ChunkID: realKey, Content: "<!--sbk:p001-b001-->"},
		{ChunkID: otherDocID + "-chunk-003", Content: "<!--sbk:p002-b001-->"}, // KB 范围外
		{ChunkID: realKey, Content: "<!--sbk:p001-b001-->"},                  // 重复 key
		{ChunkID: "garbage", Content: "<!--sbk:p003-b001-->"},                // 键非法
	}

	refs := CollectGraphEvidence(data, map[string]struct{}{realDocID: {}})
	require.Len(t, refs, 1, "仅保留 KB 范围内且键合法的证据")
	require.Equal(t, realKey, refs[0].Key)
	require.Equal(t, realDocID, refs[0].DocID)
	require.Equal(t, []string{"p001-b001"}, refs[0].BlockIDs)

	require.Nil(t, CollectGraphEvidence(data, nil), "空范围应短路")
	require.Nil(t, CollectGraphEvidence(nil, map[string]struct{}{realDocID: {}}))
}

// ---- 回跳解析 ----

// stubChunkRepo 只实现按知识文档列举子 chunk，其余方法嵌 nil（不触发）。
type stubChunkRepo struct {
	interfaces.ChunkRepository
	byDoc map[string][]*types.Chunk
	errOn map[string]bool
	calls int
}

func (s *stubChunkRepo) ListChunksByKnowledgeIDAndTypes(
	_ context.Context, _ uint64, knowledgeID string, _ []types.ChunkType,
) ([]*types.Chunk, error) {
	s.calls++
	if s.errOn[knowledgeID] {
		return nil, errors.New("db down")
	}
	return s.byDoc[knowledgeID], nil
}

func mkChunk(id, docID string, index int, blocks ...[2]any) *types.Chunk {
	parts := ""
	for i, b := range blocks {
		if i > 0 {
			parts += ","
		}
		parts += fmt.Sprintf(`{"block_id":%q,"coverage":%v,"role":"primary"}`, b[0], b[1])
	}
	return &types.Chunk{
		ID:          id,
		KnowledgeID: docID,
		ChunkIndex:  index,
		ChunkType:   types.ChunkTypeText,
		Content:     "content-" + id,
		Metadata:    types.JSON([]byte(`{"sbk_blocks":[` + parts + `]}`)),
	}
}

func TestResolveGraphEvidence_RanksByCoverage(t *testing.T) {
	t.Parallel()

	// 锚点顺序与 coverage 顺序故意相反：应按 coverage 降序选代表
	c1 := mkChunk("c1", realDocID, 1, [2]any{"p001-b001", 0.9})
	c2 := mkChunk("c2", realDocID, 2, [2]any{"p001-b002", 0.4})
	repo := &stubChunkRepo{byDoc: map[string][]*types.Chunk{realDocID: {c1, c2}}}

	refs := []GraphEvidenceRef{{
		Key: realKey, DocID: realDocID, BlockIDs: []string{"p001-b002", "p001-b001"},
	}}
	got, err := ResolveGraphEvidence(context.Background(), repo, 1, refs, 2, 20)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "c1", got[0].ID, "coverage 高者优先")
	require.Equal(t, "c2", got[1].ID)
}

func TestResolveGraphEvidence_RoundRobinsAndCaps(t *testing.T) {
	t.Parallel()

	chunks := []*types.Chunk{
		mkChunk("c1", realDocID, 1, [2]any{"b1", 0.9}),
		mkChunk("c2", realDocID, 2, [2]any{"b2", 0.8}),
		mkChunk("c3", realDocID, 3, [2]any{"b3", 0.7}),
		mkChunk("c4", realDocID, 4, [2]any{"b4", 0.6}),
	}
	repo := &stubChunkRepo{byDoc: map[string][]*types.Chunk{realDocID: chunks}}
	refs := []GraphEvidenceRef{
		{Key: "k1", DocID: realDocID, BlockIDs: []string{"b1", "b2"}},
		{Key: "k2", DocID: realDocID, BlockIDs: []string{"b3"}},
		{Key: "k3", DocID: realDocID, BlockIDs: []string{"b4"}},
	}

	got, err := ResolveGraphEvidence(context.Background(), repo, 1, refs, 2, 20)
	require.NoError(t, err)
	require.Equal(t, []string{"c1", "c3", "c4", "c2"}, ids(got),
		"轮转取用：先保证每个图谱命中贡献 1 条，再取第 2 条")
	require.Equal(t, 1, repo.calls, "同一文档只查一次全量子 chunk")

	// 整体配额截断
	got, err = ResolveGraphEvidence(context.Background(), repo, 1, refs, 2, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"c1", "c3"}, ids(got))
}

func TestResolveGraphEvidence_DedupesAcrossHits(t *testing.T) {
	t.Parallel()

	shared := mkChunk("shared", realDocID, 1, [2]any{"b1", 0.9})
	only2 := mkChunk("only2", realDocID, 2, [2]any{"b2", 0.8})
	repo := &stubChunkRepo{byDoc: map[string][]*types.Chunk{realDocID: {shared, only2}}}
	refs := []GraphEvidenceRef{
		{Key: "k1", DocID: realDocID, BlockIDs: []string{"b1"}},
		{Key: "k2", DocID: realDocID, BlockIDs: []string{"b1", "b2"}},
	}

	got, err := ResolveGraphEvidence(context.Background(), repo, 1, refs, 2, 20)
	require.NoError(t, err)
	require.Equal(t, []string{"shared", "only2"}, ids(got))
}

func TestResolveGraphEvidence_NoAnchorOrError(t *testing.T) {
	t.Parallel()

	repo := &stubChunkRepo{byDoc: map[string][]*types.Chunk{
		realDocID: {mkChunk("c1", realDocID, 1, [2]any{"b1", 0.9})},
	}}

	// 无锚点（实测 173 条图谱 chunk 中 5 条，纯表格/图片区域）→ 跳过而非报错
	got, err := ResolveGraphEvidence(context.Background(), repo, 1,
		[]GraphEvidenceRef{{Key: realKey, DocID: realDocID}}, 2, 20)
	require.NoError(t, err)
	require.Empty(t, got)

	// 锚点存在但库里没有对应子 chunk → 空结果，不报错
	got, err = ResolveGraphEvidence(context.Background(), repo, 1,
		[]GraphEvidenceRef{{Key: realKey, DocID: realDocID, BlockIDs: []string{"nope"}}}, 2, 20)
	require.NoError(t, err)
	require.Empty(t, got)

	// 单篇查询失败 → 降级跳过，不向上抛
	errRepo := &stubChunkRepo{
		byDoc: map[string][]*types.Chunk{},
		errOn: map[string]bool{realDocID: true},
	}
	got, err = ResolveGraphEvidence(context.Background(), errRepo, 1,
		[]GraphEvidenceRef{{Key: realKey, DocID: realDocID, BlockIDs: []string{"b1"}}}, 2, 20)
	require.NoError(t, err)
	require.Empty(t, got)

	// 空输入短路
	got, err = ResolveGraphEvidence(context.Background(), repo, 1, nil, 2, 20)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestResolveGraphEvidence_DefaultQuota(t *testing.T) {
	t.Parallel()

	chunks := []*types.Chunk{
		mkChunk("c1", realDocID, 1, [2]any{"b1", 0.9}),
		mkChunk("c2", realDocID, 2, [2]any{"b2", 0.8}),
		mkChunk("c3", realDocID, 3, [2]any{"b3", 0.7}),
	}
	repo := &stubChunkRepo{byDoc: map[string][]*types.Chunk{realDocID: chunks}}
	refs := []GraphEvidenceRef{
		{Key: "k1", DocID: realDocID, BlockIDs: []string{"b1", "b2", "b3"}},
	}

	// perHit/total <= 0 时回落默认（2 / 20）
	got, err := ResolveGraphEvidence(context.Background(), repo, 1, refs, 0, 0)
	require.NoError(t, err)
	require.Len(t, got, DefaultGraphChunksPerHit)
}

func ids(chunks []*types.Chunk) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, c.ID)
	}
	return out
}
