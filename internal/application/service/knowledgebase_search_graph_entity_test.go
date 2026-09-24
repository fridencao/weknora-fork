package service

// M6-1 WS1.3 · 图谱召回实体名收集单测。
//
// 实体名会被前端拿去深链图谱浏览器（?node=），所以空名与重复必须在源头清掉；
// 截断保序意味着「LightRAG 相关性最高的 N 个」，顺序不能乱。

import (
	"testing"

	chatpipeline "github.com/Tencent/WeKnora/internal/application/service/chat_pipeline"
	"github.com/stretchr/testify/require"
)

func graphQueryWithEntities(names ...string) *chatpipeline.LightragQueryData {
	data := &chatpipeline.LightragQueryData{}
	for _, n := range names {
		data.Data.Entities = append(data.Data.Entities, struct {
			EntityName  string `json:"entity_name"`
			Description string `json:"description"`
			SourceID    string `json:"source_id"`
		}{EntityName: n})
	}
	return data
}

func TestCollectGraphEntityNamesKeepsOrderAndDedupes(t *testing.T) {
	out := CollectGraphEntityNames(
		graphQueryWithEntities("宁德时代", "曾毓群", "宁德时代", "  ", "曾毓群"), 8)
	require.Equal(t, []string{"宁德时代", "曾毓群"}, out,
		"空名丢弃、重复去重、保 LightRAG 相关性顺序")
}

func TestCollectGraphEntityNamesTruncatesByRelevance(t *testing.T) {
	out := CollectGraphEntityNames(
		graphQueryWithEntities("甲", "乙", "丙", "丁"), 2)
	require.Equal(t, []string{"甲", "乙"}, out, "截断保留的是最相关的，不是随机的")
}

func TestCollectGraphEntityNamesHandlesEmptyAndNil(t *testing.T) {
	require.Nil(t, CollectGraphEntityNames(nil, 8))
	require.Nil(t, CollectGraphEntityNames(graphQueryWithEntities(), 8))
	require.Nil(t, CollectGraphEntityNames(graphQueryWithEntities("甲"), 0))
}
