package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type stubChunkRepo struct {
	interfaces.ChunkRepository
	updated []*types.Chunk
}

func (s *stubChunkRepo) UpdateChunks(_ context.Context, chunks []*types.Chunk) error {
	s.updated = append(s.updated, chunks...)
	return nil
}

func TestAlignProvenanceOnIngest_WritesBackMetadata(t *testing.T) {
	var gotBody alignProvenanceRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/align/knowledge", r.URL.Path)
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"contract_dir": "/data/processed/报告-033996f2",
			"total":        2, "failed": 0,
			"results": []map[string]any{
				{"chunk_id": "c1", "metadata": map[string]any{
					"sbk_blocks": []any{map[string]any{"block_id": "p001-b001", "coverage": 1.0, "role": "primary"}},
					"sbk_pages":  []int{1}, "sbk_method": "interval"}},
				{"chunk_id": "c2", "metadata": map[string]any{
					"sbk_blocks": []any{}, "sbk_pages": []any{}, "sbk_method": "failed"}},
			},
		})
	}))
	defer srv.Close()
	t.Setenv("STARKB_ALIGN_ON_INGEST", "true")
	t.Setenv("STARKB_API_URL", srv.URL)

	repo := &stubChunkRepo{}
	knowledge := &types.Knowledge{ID: "k1", FileName: "报告.pdf"}
	chunks := []*types.Chunk{
		{ID: "c1", Content: "金融智能体应用", StartAt: 0, EndAt: 7},
		{ID: "c2", Content: "另一段内容", StartAt: 8, EndAt: 13},
	}

	AlignProvenanceOnIngest(context.Background(), repo, 1, knowledge, chunks)

	require.Len(t, repo.updated, 2)
	require.Equal(t, "报告.pdf", gotBody.FileName)
	require.Len(t, gotBody.Chunks, 2)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(repo.updated[0].Metadata, &meta))
	require.Contains(t, meta, "sbk_blocks")
	require.Contains(t, meta, "sbk_method")
}

func TestAlignProvenanceOnIngest_DisabledAndGuards(t *testing.T) {
	t.Setenv("STARKB_ALIGN_ON_INGEST", "false")
	t.Setenv("STARKB_API_URL", "http://unused")
	repo := &stubChunkRepo{}
	// 开关关闭：不请求、不写回
	AlignProvenanceOnIngest(context.Background(), repo, 1,
		&types.Knowledge{ID: "k1", FileName: "a.pdf"},
		[]*types.Chunk{{ID: "c1", Content: "x", StartAt: 0, EndAt: 1}})
	require.Empty(t, repo.updated)

	// 无文件名：no-op
	t.Setenv("STARKB_ALIGN_ON_INGEST", "true")
	AlignProvenanceOnIngest(context.Background(), repo, 1,
		&types.Knowledge{ID: "k1"}, []*types.Chunk{{ID: "c1", Content: "x"}})
	require.Empty(t, repo.updated)
}
