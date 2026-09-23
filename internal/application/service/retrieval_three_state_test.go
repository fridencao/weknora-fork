package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// 检索参数的三态覆盖（ADR-008 决策 2）。
//
// 改造前：智能体检索字段是值类型，覆盖判断是 `if x > 0`，而 EnsureDefaults 会把
// 它们 materialize 成非 0（EmbeddingTopK=10 / VectorThreshold=0.5 / RerankTopK=5…），
// 于是智能体**永远**覆盖上层，且无法表达"显式设为 0/关闭"；RerankThreshold 又是
// 无条件覆盖，同一段代码两种语义。
//
// 改造后：nil = 继承部署缺省（config.RetrievalDefaults），显式值 = 覆盖。
// 存量智能体已存的非 0 值按显式值解释，行为与改造前一致，无需数据迁移。

// deploymentConversation 模拟一份部署配置（值刻意区别于内置默认，
// 以便区分"取到了部署值"与"取到了内置兜底值"）。
func deploymentConversation() *config.ConversationConfig {
	return &config.ConversationConfig{
		EmbeddingTopK:    30,
		VectorThreshold:  0.2,
		KeywordThreshold: 0.35,
		RerankTopK:       30,
		RerankThreshold:  0.3,
	}
}

// newChatManageFromDeployment 复刻 KnowledgeQA 的初始化方式：先铺部署缺省，
// 再由 applyAgentOverridesToChatManage 做智能体覆盖。
func newChatManageFromDeployment(t *testing.T, svc *sessionService) *types.ChatManage {
	t.Helper()
	rc := svc.cfg.RetrievalDefaults()
	return &types.ChatManage{
		PipelineRequest: types.PipelineRequest{
			EmbeddingTopK:       rc.GetEffectiveEmbeddingTopK(),
			VectorThreshold:     rc.GetEffectiveVectorThreshold(),
			KeywordThreshold:    rc.GetEffectiveKeywordThreshold(),
			RerankTopK:          rc.GetEffectiveRerankTopK(),
			RerankThreshold:     rc.GetEffectiveRerankThreshold(),
			GraphChannelEnabled: rc.GraphChannelEnabled,
		},
	}
}

func newOverrideTestService() *sessionService {
	return &sessionService{
		cfg:                   &config.Config{Conversation: deploymentConversation()},
		webSearchProviderRepo: &sharedAgentWebSearchRepo{},
	}
}

func TestRetrievalOverridesInheritDeploymentWhenNil(t *testing.T) {
	svc := newOverrideTestService()
	cm := newChatManageFromDeployment(t, svc)

	// 全部字段留 nil 的智能体：不应改变任何检索参数。
	svc.applyAgentOverridesToChatManage(t.Context(), &types.CustomAgent{
		Config: types.CustomAgentConfig{},
	}, cm)

	require.Equal(t, 30, cm.EmbeddingTopK, "EmbeddingTopK 应继承部署缺省")
	require.Equal(t, 0.2, cm.VectorThreshold, "VectorThreshold 应继承部署缺省")
	require.Equal(t, 0.35, cm.KeywordThreshold, "KeywordThreshold 应继承部署缺省")
	require.Equal(t, 30, cm.RerankTopK, "RerankTopK 应继承部署缺省")
	require.Equal(t, 0.3, cm.RerankThreshold, "RerankThreshold 应继承部署缺省")
	require.Nil(t, cm.GraphChannelEnabled, "图谱通道开关未设置时应保持 nil（继承部署默认）")
}

func TestRetrievalOverridesApplyWhenExplicit(t *testing.T) {
	svc := newOverrideTestService()
	cm := newChatManageFromDeployment(t, svc)

	svc.applyAgentOverridesToChatManage(t.Context(), &types.CustomAgent{
		Config: types.CustomAgentConfig{
			EmbeddingTopK:       ptr(80),
			VectorThreshold:     ptr(0.6),
			KeywordThreshold:    ptr(0.7),
			RerankTopK:          ptr(15),
			RerankThreshold:     ptr(-1.5),
			GraphChannelEnabled: ptr(true),
		},
	}, cm)

	require.Equal(t, 80, cm.EmbeddingTopK)
	require.Equal(t, 0.6, cm.VectorThreshold)
	require.Equal(t, 0.7, cm.KeywordThreshold)
	require.Equal(t, 15, cm.RerankTopK)
	require.Equal(t, -1.5, cm.RerankThreshold)
	require.NotNil(t, cm.GraphChannelEnabled)
	require.True(t, *cm.GraphChannelEnabled)
}

// RerankThreshold=0 是合法阈值（不过滤），必须能被显式表达——
// 这正是 `if x > 0` 语义做不到、而指针三态能做到的核心场景。
func TestRetrievalOverrideCanExpressExplicitZero(t *testing.T) {
	svc := newOverrideTestService()
	cm := newChatManageFromDeployment(t, svc)
	require.Equal(t, 0.3, cm.RerankThreshold, "前置条件：部署缺省非 0")

	svc.applyAgentOverridesToChatManage(t.Context(), &types.CustomAgent{
		Config: types.CustomAgentConfig{RerankThreshold: ptr(0.0)},
	}, cm)

	require.Equal(t, 0.0, cm.RerankThreshold,
		"显式 0 必须被当作覆盖值，而不是「未设置」")
}

// 图谱通道开关（读侧）的三态：显式 false 必须生效，不能被当作"未设置"。
func TestGraphChannelOverrideCanBeExplicitlyDisabled(t *testing.T) {
	svc := newOverrideTestService()
	cm := newChatManageFromDeployment(t, svc)

	svc.applyAgentOverridesToChatManage(t.Context(), &types.CustomAgent{
		Config: types.CustomAgentConfig{GraphChannelEnabled: ptr(false)},
	}, cm)

	require.NotNil(t, cm.GraphChannelEnabled)
	require.False(t, *cm.GraphChannelEnabled, "显式关闭图谱通道必须生效")
}

// EnsureDefaults 不得再把检索字段 materialize 成具体数值，否则"继承"永远不生效。
func TestEnsureDefaultsDoesNotMaterializeRetrievalFields(t *testing.T) {
	agent := &types.CustomAgent{Config: types.CustomAgentConfig{}}
	agent.EnsureDefaults()

	require.Nil(t, agent.Config.EmbeddingTopK, "EnsureDefaults 不应填充 EmbeddingTopK")
	require.Nil(t, agent.Config.VectorThreshold, "EnsureDefaults 不应填充 VectorThreshold")
	require.Nil(t, agent.Config.KeywordThreshold, "EnsureDefaults 不应填充 KeywordThreshold")
	require.Nil(t, agent.Config.RerankTopK, "EnsureDefaults 不应填充 RerankTopK")
	require.Nil(t, agent.Config.RerankThreshold, "EnsureDefaults 不应填充 RerankThreshold")
	require.Nil(t, agent.Config.GraphChannelEnabled, "EnsureDefaults 不应填充 GraphChannelEnabled")
}

// 存量智能体（已存非 0 值）按显式值解释：行为与改造前一致，无需迁移。
func TestLegacyAgentValuesStillOverride(t *testing.T) {
	svc := newOverrideTestService()
	cm := newChatManageFromDeployment(t, svc)

	// 模拟改造前被 EnsureDefaults 写成 10 / 0.5 / 5 的存量智能体。
	svc.applyAgentOverridesToChatManage(t.Context(), &types.CustomAgent{
		Config: types.CustomAgentConfig{
			EmbeddingTopK:    ptr(10),
			VectorThreshold:  ptr(0.5),
			RerankTopK:       ptr(5),
			KeywordThreshold: ptr(0.3),
		},
	}, cm)

	require.Equal(t, 10, cm.EmbeddingTopK)
	require.Equal(t, 0.5, cm.VectorThreshold)
	require.Equal(t, 5, cm.RerankTopK)
	require.Equal(t, 0.3, cm.KeywordThreshold)
}
