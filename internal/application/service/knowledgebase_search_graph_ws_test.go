package service

// M6-4 WS4.1 · 图谱空间查询的单测补齐（此前零覆盖）。
//
// graphQueryMerged 是 kb 模式切档（WS4.3）的核心路径：多 KB 并行查图 → 去重合并。
// 它当前唯一的保护是"能编译"，切档等于让一段没测过的代码进入生产查询热路径——
// 本文件用 httptest 假 LightRAG 把三条关键语义钉死：
//   1. shared 模式直通（不发 X-Workspace 头，单次请求）；
//   2. kb 模式逐 KB 带头并行查询，chunk 按 chunk_id 去重，实体/关系拼接；
//   3. 单空间失败只降级不传染（docs/03 §4"不可达的图直接不查"的查询侧对偶）。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	chatpipeline "github.com/Tencent/WeKnora/internal/application/service/chat_pipeline"
	"github.com/stretchr/testify/require"
)

type lightragCall struct {
	workspace string
	query     string
	mode      string
	topK      int
}

type fakeLightrag struct {
	mu    sync.Mutex
	calls []lightragCall
	// workspace → 响应载荷；不在表里的 workspace 返回 500（模拟单空间故障）
	respByWS map[string]any
}

func (f *fakeLightrag) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/query/data", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		var body struct {
			Query string `json:"query"`
			Mode  string `json:"mode"`
			TopK  int    `json:"top_k"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		f.mu.Lock()
		f.calls = append(f.calls, lightragCall{
			workspace: r.Header.Get("X-Workspace"),
			query:     body.Query,
			mode:      body.Mode,
			topK:      body.TopK,
		})
		resp, ok := f.respByWS[r.Header.Get("X-Workspace")].(map[string]any)
		f.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// newFakeLightrag 起假服务并把 LIGHT_RAG_BASE_URL 指过去（客户端从 env 构造，
// 字段不导出，无法直接注入——env 是唯一合法入口）。
func newFakeLightrag(t *testing.T, respByWS map[string]any) (*fakeLightrag, *chatpipeline.LightragClient) {
	t.Helper()
	f := &fakeLightrag{respByWS: respByWS}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("LIGHT_RAG_BASE_URL", srv.URL)
	t.Setenv("LIGHT_RAG_API_KEY", "")
	return f, chatpipeline.NewLightragClientFromEnv()
}

func lightragResp(chunks []map[string]any, entities []map[string]any) map[string]any {
	return map[string]any{
		"status": "success",
		"data": map[string]any{
			"chunks":   chunks,
			"entities": entities,
		},
	}
}

func chunk(id string) map[string]any {
	return map[string]any{"chunk_id": id, "content": "正文-" + id}
}

func TestQueryDataInWorkspaceRoutesViaHeader(t *testing.T) {
	f, client := newFakeLightrag(t, map[string]any{
		"":     lightragResp(nil, nil),
		"kb-a": lightragResp(nil, nil),
	})

	_, err := client.QueryDataInWorkspace(context.Background(), "q", 5, "")
	require.NoError(t, err)
	_, err = client.QueryDataInWorkspace(context.Background(), "q", 5, "kb-a")
	require.NoError(t, err)

	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.calls, 2)
	require.Equal(t, "", f.calls[0].workspace, "shared 模式不带 X-Workspace 头")
	require.Equal(t, "kb-a", f.calls[1].workspace)
	require.Equal(t, "mix", f.calls[0].mode, "mix 模式（KG+向量证据）是查询契约")
	require.Equal(t, 5, f.calls[0].topK)
}

func TestGraphQueryMergedSharedModeIsSinglePassthrough(t *testing.T) {
	f, client := newFakeLightrag(t, map[string]any{
		"": lightragResp([]map[string]any{chunk("c1")}, nil),
	})
	s := &knowledgeBaseService{}
	out, err := s.graphQueryMerged(context.Background(), client,
		[]string{"kb-a", "kb-b"}, "q", 10)
	require.NoError(t, err)
	require.Len(t, out.Data.Chunks, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.calls, 1, "shared 模式只有一张图：单次请求，无并行")
	require.Equal(t, "", f.calls[0].workspace)
}

func TestGraphQueryMergedKbModeParallelAndDedup(t *testing.T) {
	t.Setenv("STARKB_GRAPH_WORKSPACE_MODE", "kb")
	f, client := newFakeLightrag(t, map[string]any{
		// 同一 chunk 被两个空间召回（跨 KB 共享证据）→ 必须只保留一份
		"kb-a": lightragResp([]map[string]any{chunk("c1")}, []map[string]any{{"entity_name": "甲"}}),
		"kb-b": lightragResp([]map[string]any{chunk("c1"), chunk("c2")}, []map[string]any{{"entity_name": "乙"}}),
	})
	s := &knowledgeBaseService{}
	out, err := s.graphQueryMerged(context.Background(), client,
		[]string{"kb-a", "kb-b"}, "q", 10)
	require.NoError(t, err)

	ids := make([]string, 0, len(out.Data.Chunks))
	for _, c := range out.Data.Chunks {
		ids = append(ids, c.ChunkID)
	}
	require.ElementsMatch(t, []string{"c1", "c2"}, ids, "按 chunk_id 去重")

	names := make([]string, 0, len(out.Data.Entities))
	for _, e := range out.Data.Entities {
		names = append(names, e.EntityName)
	}
	require.ElementsMatch(t, []string{"甲", "乙"}, names, "实体跨空间拼接")

	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.calls, 2)
	wsSeen := map[string]bool{}
	for _, c := range f.calls {
		wsSeen[c.workspace] = true
		require.Equal(t, "q", c.query)
	}
	require.True(t, wsSeen["kb-a"] && wsSeen["kb-b"], "每个空间各自带头查询")
}

func TestGraphQueryMergedToleratesPartialFailure(t *testing.T) {
	t.Setenv("STARKB_GRAPH_WORKSPACE_MODE", "kb")
	f, client := newFakeLightrag(t, map[string]any{
		// kb-broken 不在表里 → 500
		"kb-ok": lightragResp([]map[string]any{chunk("c9")}, nil),
	})
	s := &knowledgeBaseService{}
	out, err := s.graphQueryMerged(context.Background(), client,
		[]string{"kb-broken", "kb-ok"}, "q", 10)
	require.NoError(t, err, "单空间故障是降级不是错误（不阻断其它空间结果）")
	require.Len(t, out.Data.Chunks, 1)
	require.Equal(t, "c9", out.Data.Chunks[0].ChunkID)
	f.mu.Lock()
	require.Len(t, f.calls, 2, "失败的空间也确实被尝试过（不是被静默跳过）")
	f.mu.Unlock()
}

func TestGraphQueryMergedEmptyKBListIsNoop(t *testing.T) {
	t.Setenv("STARKB_GRAPH_WORKSPACE_MODE", "kb")
	f, client := newFakeLightrag(t, map[string]any{})
	s := &knowledgeBaseService{}
	out, err := s.graphQueryMerged(context.Background(), client,
		nil, "q", 10)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Empty(t, out.Data.Chunks)
	f.mu.Lock()
	require.Empty(t, f.calls, "无 KB 不应发起任何请求")
	f.mu.Unlock()
}
