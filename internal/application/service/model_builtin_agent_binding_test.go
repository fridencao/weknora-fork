package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

// stubAgentRepoForModelBind records create/update calls so the test can assert
// on what CreateModel wrote to the agent store.
type stubAgentRepoForModelBind struct {
	interfaces.CustomAgentRepository
	existing  *types.CustomAgent
	created   []*types.CustomAgent
	updated   []*types.CustomAgent
	getCalled bool
}

func (s *stubAgentRepoForModelBind) GetAgentByID(context.Context, string, uint64) (*types.CustomAgent, error) {
	s.getCalled = true
	if s.existing == nil {
		return nil, repository.ErrCustomAgentNotFound
	}
	return s.existing, nil
}

func (s *stubAgentRepoForModelBind) CreateAgent(_ context.Context, agent *types.CustomAgent) error {
	s.created = append(s.created, agent)
	return nil
}

func (s *stubAgentRepoForModelBind) UpdateAgent(_ context.Context, agent *types.CustomAgent) error {
	s.updated = append(s.updated, agent)
	return nil
}

// builtinEntriesForModelBind registers a single built-in agent whose YAML
// defaults leave model_id empty — the shipped state of config/builtin_agents.yaml.
func builtinEntriesForModelBind(t *testing.T) {
	t.Helper()
	restore := types.OverrideBuiltinAgentEntriesForTest(map[string]*types.BuiltinAgentEntry{
		types.BuiltinQuickAnswerID: {
			ID:        types.BuiltinQuickAnswerID,
			IsBuiltin: true,
			Config:    types.CustomAgentConfig{AgentMode: "quick-answer"},
		},
	})
	t.Cleanup(restore)
}

func TestCreateModelBindsChatModelToUnconfiguredBuiltinAgent(t *testing.T) {
	builtinEntriesForModelBind(t)
	agentRepo := &stubAgentRepoForModelBind{}
	svc := NewModelService(&stubModelRepoForDelete{}, nil, agentRepo, nil, nil, nil)

	model := &types.Model{
		ID: "chat-1", TenantID: 7, Name: "chat-model",
		Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote,
	}
	require.NoError(t, svc.CreateModel(builtinModelContext(false), model))

	require.Len(t, agentRepo.created, 1,
		"创建 KnowledgeQA 模型应给未配置的内置智能体写配置记录")
	bound := agentRepo.created[0]
	require.Equal(t, types.BuiltinQuickAnswerID, bound.ID)
	require.Equal(t, uint64(7), bound.TenantID)
	require.True(t, bound.IsBuiltin)
	require.Equal(t, "chat-1", bound.Config.ModelID,
		"内置智能体的空 model_id 应绑定刚创建的对话模型")
}

func TestCreateModelKeepsExplicitlyConfiguredBuiltinAgentModel(t *testing.T) {
	builtinEntriesForModelBind(t)
	existing := &types.CustomAgent{
		ID: types.BuiltinQuickAnswerID, TenantID: 7, IsBuiltin: true,
		Config:    types.CustomAgentConfig{ModelID: "user-picked", AgentMode: "quick-answer"},
		UpdatedAt: time.Now(),
	}
	agentRepo := &stubAgentRepoForModelBind{existing: existing}
	svc := NewModelService(&stubModelRepoForDelete{}, nil, agentRepo, nil, nil, nil)

	model := &types.Model{
		ID: "chat-2", TenantID: 7,
		Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote,
	}
	require.NoError(t, svc.CreateModel(builtinModelContext(false), model))

	require.Empty(t, agentRepo.created, "已有记录不应再建一条")
	require.Empty(t, agentRepo.updated, "用户显式选过的 model_id 不能被覆盖")
	require.Equal(t, "user-picked", existing.Config.ModelID)
}

func TestCreateModelSkipsBindingForNonChatModels(t *testing.T) {
	builtinEntriesForModelBind(t)
	agentRepo := &stubAgentRepoForModelBind{}
	svc := NewModelService(&stubModelRepoForDelete{}, nil, agentRepo, nil, nil, nil)

	model := &types.Model{
		ID: "emb-1", TenantID: 7,
		Type: types.ModelTypeEmbedding, Source: types.ModelSourceRemote,
	}
	require.NoError(t, svc.CreateModel(builtinModelContext(false), model))

	require.False(t, agentRepo.getCalled, "非对话模型不应触碰智能体配置")
	require.Empty(t, agentRepo.created)
	require.Empty(t, agentRepo.updated)
}
