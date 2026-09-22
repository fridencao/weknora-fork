package chatpipeline

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

// A0 端到端实测（真实 LightRAG + 真实 Postgres）。默认跳过。
//
//	STARKB_GRAPH_LIVE=1 \
//	STARKB_GRAPH_LIVE_DSN='postgres://postgres:<pw>@127.0.0.1:5432/WeKnora?sslmode=disable' \
//	STARKB_GRAPH_LIVE_KB=75ff06a0-844a-458d-9a82-8674ba3c3803 \
//	go test ./internal/application/service/chat_pipeline/ -run TestLiveGraphAnchorResolve -v
//
// 验证目标：修复前该链路的真实表现是「图谱证据全部解析失败 → 召回 0 条」。
func TestLiveGraphAnchorResolve(t *testing.T) {
	if os.Getenv("STARKB_GRAPH_LIVE") == "" {
		t.Skip("set STARKB_GRAPH_LIVE=1 to run the live LightRAG/Postgres check")
	}

	dsn := os.Getenv("STARKB_GRAPH_LIVE_DSN")
	require.NotEmpty(t, dsn, "STARKB_GRAPH_LIVE_DSN required")
	kbID := os.Getenv("STARKB_GRAPH_LIVE_KB")
	require.NotEmpty(t, kbID, "STARKB_GRAPH_LIVE_KB required")
	query := os.Getenv("STARKB_GRAPH_LIVE_QUERY")
	if query == "" {
		query = "人工智能对组织与就业的影响"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.PingContext(ctx))

	// 租户从 KB 反查（chunk 查询按 tenant_id 过滤，硬编码会静默查空）
	var tenantID uint64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT tenant_id FROM knowledges WHERE knowledge_base_id = $1 LIMIT 1`, kbID).Scan(&tenantID))
	t.Logf("租户: %d", tenantID)

	// KB 范围 → 允许回跳的文档 ID 集合
	allowed := map[string]struct{}{}
	rows, err := db.QueryContext(ctx, `SELECT id FROM knowledges WHERE knowledge_base_id = $1`, kbID)
	require.NoError(t, err)
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		allowed[id] = struct{}{}
	}
	require.NoError(t, rows.Err())
	rows.Close()
	require.NotEmpty(t, allowed, "KB 范围内无文档")
	t.Logf("KB %s → %d 篇文档", kbID, len(allowed))

	// 真实 LightRAG 查询
	client := NewLightragClientFromEnv()
	require.NotEmpty(t, client.BaseURL(), "LIGHT_RAG_BASE_URL required")
	data, err := client.QueryData(ctx, query, 20)
	require.NoError(t, err)
	t.Logf("LightRAG 证据 chunk: %d 条（查询 %q）", len(data.Data.Chunks), query)
	require.NotEmpty(t, data.Data.Chunks, "LightRAG 未返回证据 chunk")

	// 键解析 + KB 过滤
	refs := CollectGraphEvidence(data, allowed)
	t.Logf("可回跳证据: %d 条（修复前此处恒为 0）", len(refs))
	require.NotEmpty(t, refs, "修复后仍无可回跳证据")

	withAnchor := 0
	for _, r := range refs {
		if len(r.BlockIDs) > 0 {
			withAnchor++
		}
	}
	t.Logf("其中带契约锚点: %d 条", withAnchor)
	require.NotZero(t, withAnchor, "无任何证据带契约锚点，回跳不可用")

	// 锚点回跳 → WeKnora 子 chunk
	repo := &liveChunkRepo{db: db}
	chunks, err := ResolveGraphEvidence(ctx, repo, tenantID, refs, DefaultGraphChunksPerHit, 20)
	require.NoError(t, err)
	t.Logf("回跳得到 WeKnora 子 chunk: %d 条（修复前恒为 0）", len(chunks))
	require.NotEmpty(t, chunks, "锚点回跳未命中任何 chunk")

	for i, c := range chunks {
		if i >= 5 {
			break
		}
		t.Logf("  [%d] id=%s knowledge=%s idx=%d type=%s len=%d",
			i, c.ID, c.KnowledgeID, c.ChunkIndex, c.ChunkType, len(c.Content))
	}
	// 回跳结果必须落在 KB 范围内（权限口径）
	for _, c := range chunks {
		_, ok := allowed[c.KnowledgeID]
		require.True(t, ok, "回跳越权: %s 不在 KB 范围内", c.KnowledgeID)
	}
}

// liveChunkRepo 直连 Postgres 的最小 ChunkRepository 适配（仅实现回跳所需方法）。
type liveChunkRepo struct {
	// 嵌入接口：其余方法不触发（一旦误用即 nil panic，不会被静默忽略）
	interfaces.ChunkRepository
	db *sql.DB
}

func (r *liveChunkRepo) ListChunksByKnowledgeIDAndTypes(
	ctx context.Context, tenantID uint64, knowledgeID string, chunkTypes []types.ChunkType,
) ([]*types.Chunk, error) {
	names := make([]string, 0, len(chunkTypes))
	for _, ct := range chunkTypes {
		names = append(names, string(ct))
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, knowledge_id, chunk_index, chunk_type, content,
		       COALESCE(metadata, '{}'::jsonb), COALESCE(parent_chunk_id, ''),
		       COALESCE(start_at, 0), COALESCE(end_at, 0)
		FROM chunks
		WHERE tenant_id = $1 AND knowledge_id = $2 AND chunk_type = ANY($3)
		ORDER BY chunk_index`, tenantID, knowledgeID, names)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*types.Chunk
	for rows.Next() {
		c := &types.Chunk{}
		var meta []byte
		if err := rows.Scan(&c.ID, &c.KnowledgeID, &c.ChunkIndex, &c.ChunkType,
			&c.Content, &meta, &c.ParentChunkID, &c.StartAt, &c.EndAt); err != nil {
			return nil, err
		}
		c.Metadata = types.JSON(meta)
		out = append(out, c)
	}
	return out, rows.Err()
}
