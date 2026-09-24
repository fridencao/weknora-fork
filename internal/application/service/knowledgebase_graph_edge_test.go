package service

// M6-1 WS1.2 · 图谱边下钻单测。
//
// 重点是「边只给自己那条关系的证据」与共享口径不退化：权限过滤必须与实体路径一致
// （两条路径共用 buildGraphEvidenceRefs，但这个测试盯着的是行为而非实现）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func fakeGraphEdgeAPI(t *testing.T, payload map[string]any) func() {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/graph/edge", r.URL.Path)
		// 端点参数必须原样透传，否则会拿错关系
		require.Equal(t, "曾毓群", r.URL.Query().Get("source"))
		require.Equal(t, "宁德时代", r.URL.Query().Get("target"))
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)
	return func() { t.Setenv("STARKB_API_URL", srv.URL) }
}

func graphEdgeResponse(chunks ...map[string]any) map[string]any {
	return map[string]any{
		"available": true,
		"workspace": "default",
		"source":    "曾毓群",
		"target":    "宁德时代",
		"relations": []map[string]any{{
			"source": "曾毓群", "target": "宁德时代", "direction": "out",
			"relation_type": "创始人", "relation_types": []string{"创始人"},
			"description": "创始关系", "descriptions": []string{"创始关系"},
		}},
		"chunks":      chunks,
		"chunk_total": len(chunks),
		"truncated":   false,
	}
}

func TestGraphEdgeDetailResolvesOnlyThatEdgesEvidence(t *testing.T) {
	f := newGraphEntityFixture(t)
	f.addTextChunk(t, "chunk-1", "doc-a", "p001-b002")
	setEnv := fakeGraphEdgeAPI(t, graphEdgeResponse(map[string]any{
		"key": "doc-a-chunk-000", "content": "创始人访谈 <!--sbk:p001-b002--> 完",
	}))
	setEnv()

	out, err := f.svc.GraphEdgeDetail(f.ctx, f.kbID, "曾毓群", "宁德时代")
	require.NoError(t, err)
	require.Equal(t, true, out["available"])
	require.Equal(t, "曾毓群", out["source"])
	require.Equal(t, "宁德时代", out["target"])

	relations := out["relations"].([]map[string]any)
	require.Len(t, relations, 1)
	require.Equal(t, "创始人", relations[0]["relation_type"])
	require.Equal(t, "out", relations[0]["direction"])

	evidence := out["evidence"].([]map[string]any)
	require.Len(t, evidence, 1)
	require.Equal(t, "chunk-1", evidence[0]["chunk_id"])
	require.Equal(t, "宁德时代年报", evidence[0]["title"])
}

func TestGraphEdgeDetailDropsEvidenceFromOtherKnowledgeBases(t *testing.T) {
	// 与实体路径同一条安全口径：共享图谱空间下的越权证据必须被丢弃
	f := newGraphEntityFixture(t)
	f.addTextChunk(t, "chunk-1", "doc-a", "p001-b002")
	setEnv := fakeGraphEdgeAPI(t, graphEdgeResponse(
		map[string]any{"key": "doc-a-chunk-000", "content": "<!--sbk:p001-b002-->"},
		map[string]any{"key": "doc-x-chunk-000", "content": "<!--sbk:p009-b009-->"},
	))
	setEnv()

	out, err := f.svc.GraphEdgeDetail(f.ctx, f.kbID, "曾毓群", "宁德时代")
	require.NoError(t, err)
	evidence := out["evidence"].([]map[string]any)
	require.Len(t, evidence, 1)
	require.Equal(t, "doc-a", evidence[0]["knowledge_id"])
	require.Equal(t, 1, out["dropped_chunks"])
}

func TestGraphEdgeDetailDegradesWhenRelationOrPlaneMissing(t *testing.T) {
	f := newGraphEntityFixture(t)

	t.Setenv("STARKB_API_URL", "")
	out, err := f.svc.GraphEdgeDetail(f.ctx, f.kbID, "曾毓群", "宁德时代")
	require.NoError(t, err)
	require.Equal(t, false, out["available"])

	setEnv := fakeGraphEdgeAPI(t, map[string]any{
		"available": false, "reason": "关系不存在: 曾毓群 → 宁德时代",
	})
	setEnv()
	out, err = f.svc.GraphEdgeDetail(f.ctx, f.kbID, "曾毓群", "宁德时代")
	require.NoError(t, err)
	require.Equal(t, false, out["available"])
	require.Contains(t, out["reason"], "关系不存在")
	// 降级形状里 relations/evidence 必须是空数组而不是 null（前端直接 .length）
	require.NotNil(t, out["relations"])
	require.NotNil(t, out["evidence"])
}

func TestGraphEdgeDetailRejectsBlankEndpoints(t *testing.T) {
	f := newGraphEntityFixture(t)
	out, err := f.svc.GraphEdgeDetail(f.ctx, f.kbID, "  ", "宁德时代")
	require.NoError(t, err)
	require.Equal(t, false, out["available"])
	require.Contains(t, out["reason"], "缺少参数")
}
