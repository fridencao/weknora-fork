package service

// M6-1 WS1.2 · 图谱实体下钻单测。
//
// 真实 sqlite chunk 表 + httptest 假 starkb-api，覆盖三件容易出错的事：
//  1. 证据回跳口径（图谱 chunk key → 正文契约锚点 → WeKnora 子 chunk）；
//  2. 权限边界：共享图谱空间下返回的**别的 KB** 的证据必须被丢弃；
//  3. 降级：数据面不可用/实体不存在时返回 available:false 而不是 error。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const graphEntityTestTenant = uint64(7)

// graphEntityFixture 一套真实 sqlite 存储（chunk + knowledge）。
type graphEntityFixture struct {
	svc       *knowledgeBaseService
	db        *gorm.DB
	ctx       context.Context
	kbID      string
	otherKBID string
}

func newGraphEntityFixture(t *testing.T) *graphEntityFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "graph-entity.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&types.Knowledge{}, &types.Chunk{}))

	kbID, otherKBID := "kb-graph", "kb-other"
	for _, row := range []*types.Knowledge{
		{ID: "doc-a", TenantID: graphEntityTestTenant, KnowledgeBaseID: kbID, Title: "宁德时代年报"},
		{ID: "doc-b", TenantID: graphEntityTestTenant, KnowledgeBaseID: kbID, Title: "曾毓群访谈"},
		{ID: "doc-x", TenantID: graphEntityTestTenant, KnowledgeBaseID: otherKBID, Title: "别的库的文档"},
	} {
		require.NoError(t, db.Create(row).Error)
	}

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, graphEntityTestTenant)
	return &graphEntityFixture{
		svc: &knowledgeBaseService{
			kgRepo:    repository.NewKnowledgeRepository(db),
			chunkRepo: repository.NewChunkRepository(db),
		},
		db:        db,
		ctx:       ctx,
		kbID:      kbID,
		otherKBID: otherKBID,
	}
}

// addTextChunk 落一条带契约对齐 metadata 的正文子 chunk（回跳的定位依据）。
func (f *graphEntityFixture) addTextChunk(t *testing.T, id, docID string, blockIDs ...string) {
	t.Helper()
	blocks := make([]map[string]any, 0, len(blockIDs))
	for _, b := range blockIDs {
		blocks = append(blocks, map[string]any{"block_id": b, "coverage": 0.9})
	}
	meta, err := json.Marshal(map[string]any{"sbk_blocks": blocks})
	require.NoError(t, err)
	require.NoError(t, f.db.Create(&types.Chunk{
		ID:              id,
		TenantID:        graphEntityTestTenant,
		KnowledgeID:     docID,
		KnowledgeBaseID: f.kbID,
		ChunkType:       types.ChunkTypeText,
		ChunkIndex:      1,
		Content:         "宁德时代发布年报。",
		Metadata:        meta,
	}).Error)
}

// fakeGraphEntityAPI 起一个假 starkb-api，返回给定 payload。
func fakeGraphEntityAPI(t *testing.T, payload map[string]any) func() {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/graph/entity", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)
	return func() { t.Setenv("STARKB_API_URL", srv.URL) }
}

func graphEntityResponse(chunks ...map[string]any) map[string]any {
	return map[string]any{
		"available": true,
		"workspace": "default",
		"entity": map[string]any{
			"id": "宁德时代", "entity_type": "公司",
			"description": "动力电池企业", "degree": 12,
		},
		"neighbors": []map[string]any{{
			"id": "曾毓群", "entity_type": "人物", "relation_type": "创始人",
			"description": "创始人关系", "direction": "in",
		}},
		"chunks":         chunks,
		"chunk_total":    len(chunks),
		"neighbor_total": 1,
		"truncated":      false,
	}
}

func TestGraphEntityDetailResolvesEvidenceToWeKnoraChunks(t *testing.T) {
	f := newGraphEntityFixture(t)
	f.addTextChunk(t, "chunk-1", "doc-a", "p001-b002")
	setEnv := fakeGraphEntityAPI(t, graphEntityResponse(map[string]any{
		// LightRAG 真实形态：key 前段是 WeKnora doc id，正文里带契约锚点
		"key": "doc-a-chunk-000", "content": "年报摘录 <!--sbk:p001-b002--> 完",
	}))
	setEnv()

	out, err := f.svc.GraphEntityDetail(f.ctx, f.kbID, "宁德时代")
	require.NoError(t, err)
	require.Equal(t, true, out["available"])
	entity := out["entity"].(map[string]any)
	require.Equal(t, "动力电池企业", entity["description"])

	evidence := out["evidence"].([]map[string]any)
	require.Len(t, evidence, 1)
	require.Equal(t, "chunk-1", evidence[0]["chunk_id"])
	require.Equal(t, "doc-a", evidence[0]["knowledge_id"])
	require.Equal(t, "宁德时代年报", evidence[0]["title"], "标题回填供下钻面板展示")
}

func TestGraphEntityDetailDropsEvidenceFromOtherKnowledgeBases(t *testing.T) {
	// 共享图谱空间（starkb 默认 workspace）会返回别的 KB 的实体与证据；
	// 路由只校验了当前 KB 的读权限，所以过滤必须在这里做。
	f := newGraphEntityFixture(t)
	f.addTextChunk(t, "chunk-1", "doc-a", "p001-b002")
	setEnv := fakeGraphEntityAPI(t, graphEntityResponse(
		map[string]any{"key": "doc-a-chunk-000", "content": "<!--sbk:p001-b002-->"},
		map[string]any{"key": "doc-x-chunk-000", "content": "<!--sbk:p009-b009-->"},
	))
	setEnv()

	out, err := f.svc.GraphEntityDetail(f.ctx, f.kbID, "宁德时代")
	require.NoError(t, err)
	evidence := out["evidence"].([]map[string]any)
	require.Len(t, evidence, 1)
	require.Equal(t, "doc-a", evidence[0]["knowledge_id"])
	require.Equal(t, 1, out["dropped_chunks"], "越权证据必须被计入丢弃")
}

func TestGraphEntityDetailSkipsChunksWithoutContractAnchors(t *testing.T) {
	// 实测约 3% 的图谱 chunk 无锚点（纯表格/图片区域），无法定位到子 chunk。
	f := newGraphEntityFixture(t)
	f.addTextChunk(t, "chunk-1", "doc-a", "p001-b002")
	setEnv := fakeGraphEntityAPI(t, graphEntityResponse(
		map[string]any{"key": "doc-a-chunk-000", "content": "没有锚点的正文"},
	))
	setEnv()

	out, err := f.svc.GraphEntityDetail(f.ctx, f.kbID, "宁德时代")
	require.NoError(t, err)
	require.Empty(t, out["evidence"].([]map[string]any))
	require.Equal(t, 1, out["dropped_chunks"])
	// 证据点不开不代表面板空着：实体与邻居仍要返回
	require.Len(t, out["neighbors"].([]map[string]any), 1)
}

func TestGraphEntityDetailDegradesWhenDataPlaneMissing(t *testing.T) {
	f := newGraphEntityFixture(t)
	t.Setenv("STARKB_API_URL", "")

	out, err := f.svc.GraphEntityDetail(f.ctx, f.kbID, "宁德时代")
	require.NoError(t, err, "数据面缺失必须是降级而非报错")
	require.Equal(t, false, out["available"])
	require.NotEmpty(t, out["reason"])
}

func TestGraphEntityDetailPassesThroughUnknownEntity(t *testing.T) {
	f := newGraphEntityFixture(t)
	setEnv := fakeGraphEntityAPI(t, map[string]any{
		"available": false, "reason": "实体不存在: 不存在的东西",
	})
	setEnv()

	out, err := f.svc.GraphEntityDetail(f.ctx, f.kbID, "不存在的东西")
	require.NoError(t, err)
	require.Equal(t, false, out["available"])
	require.Contains(t, out["reason"], "实体不存在")
}

func TestGraphEntityDetailRejectsBlankInput(t *testing.T) {
	f := newGraphEntityFixture(t)
	out, err := f.svc.GraphEntityDetail(f.ctx, f.kbID, "   ")
	require.NoError(t, err)
	require.Equal(t, false, out["available"])
	require.Contains(t, out["reason"], "缺少参数")
}

func TestSnippetTruncatesByRune(t *testing.T) {
	require.Equal(t, "短", snippet("短", 10))
	long := snippet("宁德时代动力电池", 4)
	require.Equal(t, "宁德时代…", long)
	require.Equal(t, 5, len([]rune(long)), "截断后仍应是合法 UTF-8（不会切出半个汉字）")
}
