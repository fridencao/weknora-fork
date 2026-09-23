package handler

// ADR-008 决策 4（文档列表页图谱徽标）的代理层测试。
//
// handler 本体只是「校验访问 → 收敛 id → 转发」的胶水，真正的逻辑与安全边界
// 在两个纯函数里：ownedGraphDocIDs（不信任客户端 id）与 postStarkbGraph（降级
// 语义）。所以这里直接打这两个，不搭 middleware 的 grant 上下文。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// stubKnowledgeService 只实现 GetKnowledgeByID；其余方法走嵌入的 nil 接口，
// 一旦被调到就 panic —— 归属校验不该碰别的能力。
type stubKnowledgeService struct {
	interfaces.KnowledgeService
	get func(ctx context.Context, id string) (*types.Knowledge, error)
}

func (s *stubKnowledgeService) GetKnowledgeByID(ctx context.Context, id string) (*types.Knowledge, error) {
	return s.get(ctx, id)
}

func kbHandlerWithKnowledge(get func(context.Context, string) (*types.Knowledge, error)) *KnowledgeBaseHandler {
	return &KnowledgeBaseHandler{knowledgeService: &stubKnowledgeService{get: get}}
}

// TestOwnedGraphDocIDs_DropsForeignKBAndForeignTenant 是这条链路上最重要的断言。
//
// starkb-api 的 graph_doc_state 以 knowledge_id 为唯一键、查询不按租户过滤，
// 所以「这个 id 到底属不属于调用者」只能在这里判。放行一个外来 id，就等于把
// 别人的图谱状态和 last_error 交出去。
func TestOwnedGraphDocIDs_DropsForeignKBAndForeignTenant(t *testing.T) {
	kb := &types.KnowledgeBase{ID: "kb-1", TenantID: 100}
	h := kbHandlerWithKnowledge(func(_ context.Context, id string) (*types.Knowledge, error) {
		switch id {
		case "mine":
			return &types.Knowledge{ID: "mine", KnowledgeBaseID: "kb-1", TenantID: 100}, nil
		case "other-kb":
			return &types.Knowledge{ID: "other-kb", KnowledgeBaseID: "kb-2", TenantID: 100}, nil
		default:
			// 跨租户时 GetKnowledgeByID 按租户查不到 → NotFound。
			return nil, errors.New("knowledge not found")
		}
	})

	got := ownedGraphDocIDs(context.Background(), h, kb,
		[]string{"mine", "other-kb", "foreign-tenant"})
	if len(got) != 1 || got[0] != "mine" {
		t.Fatalf("只应保留本 KB 的文档，得到 %v", got)
	}
}

func TestOwnedGraphDocIDs_DedupsInput(t *testing.T) {
	kb := &types.KnowledgeBase{ID: "kb-1"}
	calls := 0
	h := kbHandlerWithKnowledge(func(_ context.Context, id string) (*types.Knowledge, error) {
		calls++
		return &types.Knowledge{ID: id, KnowledgeBaseID: "kb-1"}, nil
	})
	got := ownedGraphDocIDs(context.Background(), h, kb, []string{"a", "a", "b", "a"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("应去重且保序，得到 %v", got)
	}
	if calls != 2 {
		t.Fatalf("重复 id 不该重复点查，实际调用 %d 次", calls)
	}
}

func TestOwnedGraphDocIDs_BlankAndEmptyInput(t *testing.T) {
	h := kbHandlerWithKnowledge(func(_ context.Context, _ string) (*types.Knowledge, error) {
		t.Fatal("空输入不该触发查询")
		return nil, nil
	})
	if got := ownedGraphDocIDs(context.Background(), h, &types.KnowledgeBase{ID: "kb-1"}, nil); got != nil {
		t.Fatalf("nil 输入应回 nil，得到 %v", got)
	}
	if got := ownedGraphDocIDs(context.Background(), h, &types.KnowledgeBase{ID: "kb-1"}, []string{"", ""}); len(got) != 0 {
		t.Fatalf("全空串应回空集合，得到 %v", got)
	}
}

// 上限保护：别让这个接口被当成全量扫描用。
func TestOwnedGraphDocIDs_CapsRequestedCount(t *testing.T) {
	kb := &types.KnowledgeBase{ID: "kb-1"}
	lookups := 0
	h := kbHandlerWithKnowledge(func(_ context.Context, id string) (*types.Knowledge, error) {
		lookups++
		return &types.Knowledge{ID: id, KnowledgeBaseID: "kb-1"}, nil
	})
	ids := make([]string, graphDocStatusMaxIDs+120)
	for i := range ids {
		ids[i] = string(rune('a'+i%26)) + "-" + itoa(i)
	}
	got := ownedGraphDocIDs(context.Background(), h, kb, ids)
	if len(got) != graphDocStatusMaxIDs {
		t.Fatalf("应截断到 %d，得到 %d", graphDocStatusMaxIDs, len(got))
	}
	if lookups > graphDocStatusMaxIDs {
		t.Fatalf("点查次数不该超过上限，实际 %d", lookups)
	}
}

// 测试装配下 knowledgeService 可能为 nil；此时按原样放行（与旧行为一致），
// 而不是 fail-open 成「查全库」。生产装配下该字段始终非 nil。
func TestOwnedGraphDocIDs_NilServicePassesThrough(t *testing.T) {
	h := &KnowledgeBaseHandler{}
	got := ownedGraphDocIDs(context.Background(), h, &types.KnowledgeBase{ID: "kb-1"}, []string{"a", "b"})
	if len(got) != 2 {
		t.Fatalf("nil service 应原样透传，得到 %v", got)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// ---------------- postStarkbGraph 的降级语义 ----------------

func TestPostStarkbGraph_Unconfigured(t *testing.T) {
	t.Setenv("STARKB_API_URL", "")
	if _, reason := postStarkbGraph(context.Background(), "/graph/doc-status", nil); reason != "STARKB_API_URL 未配置" {
		t.Fatalf("未配置时应给出明确原因，得到 %q", reason)
	}
}

func TestPostStarkbGraph_Unreachable(t *testing.T) {
	// 127.0.0.1:1 上不会有服务在听。
	t.Setenv("STARKB_API_URL", "http://127.0.0.1:1")
	if _, reason := postStarkbGraph(context.Background(), "/graph/doc-status", nil); reason != "starkb-api 不可达" {
		t.Fatalf("不可达时应降级，得到 %q", reason)
	}
}

func TestPostStarkbGraph_Non200Degrades(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("STARKB_API_URL", srv.URL)
	if _, reason := postStarkbGraph(context.Background(), "/graph/doc-status", nil); reason != "starkb-api 返回异常" {
		t.Fatalf("非 200 应降级，得到 %q", reason)
	}
}

func TestPostStarkbGraph_PostsJSONToPath(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type 应为 application/json，得到 %q", ct)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"states":{"a":"ready"},"details":{}}`))
	}))
	defer srv.Close()
	t.Setenv("STARKB_API_URL", srv.URL)

	out, reason := postStarkbGraph(context.Background(), "/graph/doc-status",
		map[string]any{"knowledge_ids": []string{"a"}})
	if reason != "" {
		t.Fatalf("不该降级，得到 %q", reason)
	}
	if gotPath != "/graph/doc-status" {
		t.Fatalf("路径应为 /graph/doc-status，得到 %q", gotPath)
	}
	if ids, ok := gotBody["knowledge_ids"].([]any); !ok || len(ids) != 1 || ids[0] != "a" {
		t.Fatalf("请求体应原样带上 knowledge_ids，得到 %v", gotBody)
	}
	states, ok := out["states"].(map[string]any)
	if !ok || states["a"] != "ready" {
		t.Fatalf("应解回 states，得到 %v", out)
	}
}

func TestPostStarkbGraph_MalformedJSONDegrades(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	t.Setenv("STARKB_API_URL", srv.URL)
	if _, reason := postStarkbGraph(context.Background(), "/graph/doc-status", nil); reason != "响应解析失败" {
		t.Fatalf("坏响应应降级，得到 %q", reason)
	}
}
