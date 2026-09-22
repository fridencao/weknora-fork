package chatpipeline

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// 图谱召回通道（读侧）开关的优先级（ADR-008 决策 2 / 3.1）：
//
//	chatManage.GraphChannelEnabled（智能体三态）> 部署默认（GRAPH_CHANNEL_ENABLED）
//
// 改造前这个开关是在插件内部直接读租户上下文（TenantInfoFromContext.RetrievalConfig）
// 的，而同一请求里的 rerank 阈值来自部署 YAML——两个源。现在与其它检索参数
// 一起走 chatManage，因此这里要锁住"谁优先"。
func TestGraphChannelEnabledPrecedence(t *testing.T) {
	t.Parallel()

	boolPtr := func(v bool) *bool { return &v }

	cases := []struct {
		name       string
		chatManage *types.ChatManage
		envDefault bool
		want       bool
	}{
		{"未设置时用部署默认(true)", &types.ChatManage{}, true, true},
		{"未设置时用部署默认(false)", &types.ChatManage{}, false, false},
		{"显式开启覆盖部署默认(false)", &types.ChatManage{
			PipelineRequest: types.PipelineRequest{GraphChannelEnabled: boolPtr(true)},
		}, false, true},
		{"显式关闭覆盖部署默认(true)", &types.ChatManage{
			PipelineRequest: types.PipelineRequest{GraphChannelEnabled: boolPtr(false)},
		}, true, false},
		{"nil chatManage 回落部署默认", nil, true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, graphChannelEnabled(tc.chatManage, tc.envDefault))
		})
	}
}
