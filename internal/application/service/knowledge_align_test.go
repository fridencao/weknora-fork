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
	updated []*types.Chunk // UpdateChunks（批量，不写 metadata 列）
	saved   []*types.Chunk // UpdateChunk（整行 Save，含 metadata）
}

func (s *stubChunkRepo) UpdateChunks(_ context.Context, chunks []*types.Chunk) error {
	s.updated = append(s.updated, chunks...)
	return nil
}

func (s *stubChunkRepo) UpdateChunk(_ context.Context, chunk *types.Chunk) error {
	s.saved = append(s.saved, chunk)
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

	svc := &KnowledgePostProcessService{settings: newEnvOnlySettings()}
	svc.AlignProvenanceOnIngest(context.Background(), repo, 1, knowledge, chunks)
	// 回归（2026-09-26 冒烟实证）：UpdateChunks 批量 SQL 不含 metadata 列，
	// 锚点写回曾是 no-op（全靠 starkb-api 5 分钟一轮的 anchor_reconcile 兜底，
	// 入库后检索窗口内图谱通道 0 命中）。对齐必须走 UpdateChunk（整行 Save）。
	require.Len(t, repo.saved, 2)
	require.Empty(t, repo.updated, "对齐不得走 UpdateChunks（不写 metadata）")
	require.Equal(t, "报告.pdf", gotBody.FileName)
	require.Len(t, gotBody.Chunks, 2)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(repo.saved[0].Metadata, &meta))
	require.Contains(t, meta, "sbk_blocks")
	require.Contains(t, meta, "sbk_method")
}

func TestAlignProvenanceOnIngest_DisabledAndGuards(t *testing.T) {
	t.Setenv("STARKB_ALIGN_ON_INGEST", "false")
	t.Setenv("STARKB_API_URL", "http://unused")
	repo := &stubChunkRepo{}
	svc := &KnowledgePostProcessService{settings: newEnvOnlySettings()}
	// 开关关闭：不请求、不写回
	svc.AlignProvenanceOnIngest(context.Background(), repo, 1,
		&types.Knowledge{ID: "k1", FileName: "a.pdf"},
		[]*types.Chunk{{ID: "c1", Content: "x", StartAt: 0, EndAt: 1}})
	require.Empty(t, repo.updated)

	// 无文件名：no-op
	t.Setenv("STARKB_ALIGN_ON_INGEST", "true")
	svc.AlignProvenanceOnIngest(context.Background(), repo, 1,
		&types.Knowledge{ID: "k1"}, []*types.Chunk{{ID: "c1", Content: "x"}})
	require.Empty(t, repo.updated)
}
