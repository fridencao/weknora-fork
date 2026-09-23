package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

// ADR-008 决策 3.4（off→on 边沿）/ 3.6（删除清理）的 Go 侧钩子。
//
// M4 R1 已实现 3.3/3.4/3.5/3.7 主干（graph_doc_state + 补齐 worker + 端点 + 对账 CLI），
// 这里锁住它没覆盖的两条边：存量 KB 的开关边沿、以及删除侧。

type capturedPost struct {
	path string
	body map[string]any
}

// graphHookServer 起一个假 starkb-api，收集收到的请求。
func graphHookServer(t *testing.T) (*httptest.Server, *[]capturedPost) {
	t.Helper()
	var mu sync.Mutex
	var got []capturedPost
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		got = append(got, capturedPost{path: r.URL.Path, body: body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"marked":2,"skipped_ready":0,"pending":1}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func tenantCtx(id uint64) context.Context {
	return context.WithValue(context.Background(), types.TenantInfoContextKey,
		&types.Tenant{ID: id})
}

type stubLifecycleRepo struct {
	interfaces.KnowledgeRepository
	docs []*types.Knowledge
	err  error
}

func (s *stubLifecycleRepo) ListKnowledgeByKnowledgeBaseID(
	_ context.Context, _ uint64, _ string,
) ([]*types.Knowledge, error) {
	return s.docs, s.err
}

func kbWithAutoBuild(on bool) *types.KnowledgeBase {
	return &types.KnowledgeBase{ID: "kb-1", GraphConfig: &types.GraphConfig{AutoBuild: on}}
}

// ---- 决策 3.4：off→on 边沿 ----

func TestGraphAutoBuildTurnedOn(t *testing.T) {
	cases := []struct {
		name  string
		wasOn bool
		kb    *types.KnowledgeBase
		want  bool
	}{
		{"关→开 = 边沿", false, kbWithAutoBuild(true), true},
		{"开→开 不是边沿（否则每次保存都全库重扫）", true, kbWithAutoBuild(true), false},
		{"开→关 不是边沿", true, kbWithAutoBuild(false), false},
		{"关→关 不是边沿", false, kbWithAutoBuild(false), false},
		{"GraphConfig 为 nil（未配置）不算开启", false,
			&types.KnowledgeBase{ID: "kb-1"}, false},
		{"kb 为 nil 不 panic", false, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, graphAutoBuildTurnedOn(tc.wasOn, tc.kb))
		})
	}
}

func TestGraphBackfillOnEnablePostsExistingDocs(t *testing.T) {
	srv, got := graphHookServer(t)
	t.Setenv("STARKB_API_URL", srv.URL)
	t.Setenv("STARKB_GRAPH_WORKSPACE_MODE", "")

	repo := &stubLifecycleRepo{docs: []*types.Knowledge{
		{ID: "doc-a", FileName: "a.pdf"},
		{ID: "doc-b", FileName: "b.pdf"},
		{ID: "pasted", FileName: ""}, // 无契约语义，从未进过图谱
	}}
	GraphBackfillOnEnable(context.Background(), repo, 10011, "kb-1")

	require.Len(t, *got, 1)
	require.Equal(t, "/graph/backfill", (*got)[0].path, "应复用 M4 已有的补齐端点")
	require.Equal(t, "10011", (*got)[0].body["tenant_id"])
	require.Equal(t, "kb-1", (*got)[0].body["kb_id"])
	require.ElementsMatch(t, []any{"doc-a", "doc-b"}, (*got)[0].body["doc_ids"])
}

func TestGraphBackfillOnEnableNoopWhenNoDocs(t *testing.T) {
	srv, got := graphHookServer(t)
	t.Setenv("STARKB_API_URL", srv.URL)
	GraphBackfillOnEnable(context.Background(), &stubLifecycleRepo{}, 1, "kb-1")
	require.Empty(t, *got)
}

func TestGraphBackfillOnEnableNoopWithoutAPIURL(t *testing.T) {
	_, got := graphHookServer(t)
	t.Setenv("STARKB_API_URL", "")
	repo := &stubLifecycleRepo{docs: []*types.Knowledge{{ID: "a", FileName: "a.pdf"}}}
	GraphBackfillOnEnable(context.Background(), repo, 1, "kb-1")
	require.Empty(t, *got)
}

func TestGraphBackfillOnEnableNoopWithoutKBID(t *testing.T) {
	srv, got := graphHookServer(t)
	t.Setenv("STARKB_API_URL", srv.URL)
	repo := &stubLifecycleRepo{docs: []*types.Knowledge{{ID: "a", FileName: "a.pdf"}}}
	GraphBackfillOnEnable(context.Background(), repo, 1, "")
	require.Empty(t, *got)
}

// ---- 决策 3.6：删除清理 ----

// graphCleanupSvc 构造挂了 ENV-only 设置服务的 knowledgeService。
// 删除清理开关迁到 system_settings 后（DB > ENV > 默认），这些用例仍用
// t.Setenv 驱动，需要 DB 层为空的解析器才能保持旧语义。
func graphCleanupSvc() *knowledgeService {
	return &knowledgeService{settings: newEnvOnlySettings()}
}

func TestGraphCleanupOnDeletePostsTombstone(t *testing.T) {
	srv, got := graphHookServer(t)
	t.Setenv("STARKB_API_URL", srv.URL)
	t.Setenv("STARKB_GRAPH_WORKSPACE_MODE", "")

	graphCleanupSvc().GraphCleanupOnDelete(tenantCtx(10011), []*types.Knowledge{
		{ID: "doc-a", KnowledgeBaseID: "kb-1", FileName: "a.pdf"},
		{ID: "doc-b", KnowledgeBaseID: "kb-1", FileName: "b.pdf"},
	})

	require.Len(t, *got, 1)
	require.Equal(t, "/graph/docs/delete", (*got)[0].path)
	require.Equal(t, "10011", (*got)[0].body["tenant_id"])
	require.Equal(t, "kb-1", (*got)[0].body["kb_id"])
	require.ElementsMatch(t, []any{"doc-a", "doc-b"}, (*got)[0].body["doc_ids"])
}

func TestGraphCleanupOnDeleteGroupsByKnowledgeBase(t *testing.T) {
	srv, got := graphHookServer(t)
	t.Setenv("STARKB_API_URL", srv.URL)

	graphCleanupSvc().GraphCleanupOnDelete(tenantCtx(1), []*types.Knowledge{
		{ID: "a", KnowledgeBaseID: "kb-1", FileName: "a.pdf"},
		{ID: "b", KnowledgeBaseID: "kb-2", FileName: "b.pdf"},
	})

	require.Len(t, *got, 2, "workspace 是 KB 维度的，一次请求只能对应一个 KB")
	kbs := map[string]bool{}
	for _, c := range *got {
		kbs[c.body["kb_id"].(string)] = true
	}
	require.True(t, kbs["kb-1"] && kbs["kb-2"])
}

func TestGraphCleanupOnDeleteSkipsDocsWithoutContractSemantics(t *testing.T) {
	srv, got := graphHookServer(t)
	t.Setenv("STARKB_API_URL", srv.URL)

	graphCleanupSvc().GraphCleanupOnDelete(tenantCtx(1), []*types.Knowledge{
		{ID: "pasted", KnowledgeBaseID: "kb-1", FileName: ""},
	})

	require.Empty(t, *got)
}

func TestGraphCleanupOnDeleteDisabled(t *testing.T) {
	srv, got := graphHookServer(t)
	t.Setenv("STARKB_API_URL", srv.URL)
	t.Setenv("STARKB_GRAPH_CLEANUP_ON_DELETE", "false")

	graphCleanupSvc().GraphCleanupOnDelete(tenantCtx(1), []*types.Knowledge{
		{ID: "a", KnowledgeBaseID: "kb-1", FileName: "a.pdf"},
	})

	require.Empty(t, *got)
}

func TestGraphCleanupOnDeleteNoopWithoutAPIURL(t *testing.T) {
	_, got := graphHookServer(t)
	t.Setenv("STARKB_API_URL", "")

	graphCleanupSvc().GraphCleanupOnDelete(tenantCtx(1), []*types.Knowledge{
		{ID: "a", KnowledgeBaseID: "kb-1", FileName: "a.pdf"},
	})

	require.Empty(t, *got)
}

func TestGraphCleanupOnDeleteNoopWithoutTenant(t *testing.T) {
	srv, got := graphHookServer(t)
	t.Setenv("STARKB_API_URL", srv.URL)

	graphCleanupSvc().GraphCleanupOnDelete(context.Background(), []*types.Knowledge{
		{ID: "a", KnowledgeBaseID: "kb-1", FileName: "a.pdf"},
	})

	require.Empty(t, *got, "无租户上下文时不能瞎猜 tenant_id")
}

// ---- workspace 口径（与 M4 的 graphWorkspaceForKB 一致） ----

func TestGraphWorkspaceForKBID(t *testing.T) {
	t.Setenv("STARKB_GRAPH_WORKSPACE_MODE", "")
	require.Equal(t, "", graphWorkspaceForKBID("kb-1"), "shared 模式 = 全局空间")

	t.Setenv("STARKB_GRAPH_WORKSPACE_MODE", "kb")
	require.Equal(t, "kb-1", graphWorkspaceForKBID("kb-1"))
	require.Equal(t, "", graphWorkspaceForKBID(""))
}
