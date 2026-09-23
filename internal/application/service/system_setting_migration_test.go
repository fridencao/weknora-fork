package service

// 配置治理批一的防漂移守卫。
//
// 键名常量放在 internal/types（因为 chat_pipeline 不能反向导入 service，
// 会成环），注册表放在本包。两处一旦不同步，消费方就会读一个注册表里
// 不存在的键——GetXxx 会静默返回调用方默认值，界面上也看不到这一项，
// 排查成本很高。这个测试把该错误变成编译后立刻可见的失败。

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// migratedSettingKeys 是批一迁入 system_settings 的全部键。
// 新增迁移项时必须同时改这里——这是刻意的，避免"改了消费方忘了注册表"。
var migratedSettingKeys = []string{
	types.SettingKeyFusionReliabilityWeight,
	types.SettingKeyGraphChannelEnabled,
	types.SettingKeyGraphChannelTopK,
	types.SettingKeyGraphChannelChunksPerHit,
	types.SettingKeyGraphChannelTimeoutS,
	types.SettingKeyStarkbClaimGate,
	types.SettingKeyStarkbRequireProvenance,
	types.SettingKeyStarkbAlignOnIngest,
	types.SettingKeyStarkbGraphOnIngest,
	types.SettingKeyStarkbGraphCleanupOnDelete,
	types.SettingKeyAgentLLMTimeout,
	types.SettingKeyAgentToolApprovalTimeout,
	types.SettingKeyAgentToolApprovalFailOpen,
	types.SettingKeyDocreaderCallTimeout,
	types.SettingKeyDocumentProcessTimeout,
	types.SettingKeyTenantEnableRBAC,
	types.SettingKeyTenantEnableCrossTenantAccess,
	types.SettingKeyHousekeepingEnabled,
	types.SettingKeyRetrievalMultiStoreTimeoutS,
	types.SettingKeyTenantInvitationTTL,
	types.SettingKeyChatAttachmentTTLHours,
	types.SettingKeyChatAttachmentOCRMaxPages,
	types.SettingKeyChatAttachmentOCRConcurrency,
	types.SettingKeyChatAttachmentWaitTimeoutS,
}

// TestMigratedSettingsRegistered 每个迁移键都必须在注册表里，
// 且注册的 EnvName 与其常量配套的旧环境变量名一致。
func TestMigratedSettingsRegistered(t *testing.T) {
	envPairs := map[string]string{
		types.SettingKeyFusionReliabilityWeight:       types.SettingEnvFusionReliabilityWeight,
		types.SettingKeyGraphChannelEnabled:           types.SettingEnvGraphChannelEnabled,
		types.SettingKeyGraphChannelTopK:              types.SettingEnvGraphChannelTopK,
		types.SettingKeyGraphChannelChunksPerHit:      types.SettingEnvGraphChannelChunksPerHit,
		types.SettingKeyGraphChannelTimeoutS:          types.SettingEnvGraphChannelTimeoutS,
		types.SettingKeyStarkbClaimGate:               types.SettingEnvStarkbClaimGate,
		types.SettingKeyStarkbRequireProvenance:       types.SettingEnvStarkbRequireProvenance,
		types.SettingKeyStarkbAlignOnIngest:           types.SettingEnvStarkbAlignOnIngest,
		types.SettingKeyStarkbGraphOnIngest:           types.SettingEnvStarkbGraphOnIngest,
		types.SettingKeyStarkbGraphCleanupOnDelete:    types.SettingEnvStarkbGraphCleanupOnDelete,
		types.SettingKeyAgentLLMTimeout:               types.SettingEnvAgentLLMTimeout,
		types.SettingKeyAgentToolApprovalTimeout:      types.SettingEnvAgentToolApprovalTimeout,
		types.SettingKeyAgentToolApprovalFailOpen:     types.SettingEnvAgentToolApprovalFailOpen,
		types.SettingKeyDocreaderCallTimeout:          types.SettingEnvDocreaderCallTimeout,
		types.SettingKeyDocumentProcessTimeout:        types.SettingEnvDocumentProcessTimeout,
		types.SettingKeyTenantEnableRBAC:              types.SettingEnvTenantEnableRBAC,
		types.SettingKeyTenantEnableCrossTenantAccess: types.SettingEnvTenantEnableCrossTenantAccess,
		types.SettingKeyHousekeepingEnabled:           types.SettingEnvHousekeepingEnabled,
		types.SettingKeyRetrievalMultiStoreTimeoutS:   types.SettingEnvRetrievalMultiStoreTimeoutS,
		types.SettingKeyTenantInvitationTTL:           types.SettingEnvTenantInvitationTTL,
		types.SettingKeyChatAttachmentTTLHours:        types.SettingEnvChatAttachmentTTLHours,
		types.SettingKeyChatAttachmentOCRMaxPages:     types.SettingEnvChatAttachmentOCRMaxPages,
		types.SettingKeyChatAttachmentOCRConcurrency:  types.SettingEnvChatAttachmentOCRConcurrency,
		types.SettingKeyChatAttachmentWaitTimeoutS:    types.SettingEnvChatAttachmentWaitTimeoutS,
	}

	for _, key := range migratedSettingKeys {
		spec, ok := registry[key]
		require.Truef(t, ok, "迁移键 %q 未注册到 registry——消费方会读到调用方默认值而静默失效", key)
		require.Equalf(t, envPairs[key], spec.EnvName,
			"迁移键 %q 的 EnvName 与常量不匹配（常量表=%q，注册表=%q）", key, envPairs[key], spec.EnvName)
		require.NotEmptyf(t, spec.Description, "迁移键 %q 缺 Description，界面无法说明用途", key)
		require.NotEmptyf(t, spec.Category, "迁移键 %q 缺 Category，界面无法归类", key)
	}
}

// TestReliabilityWeightFloorLocked 校验 ADR-005 的 ≥0.25 下限锁定：
// 迁移前该权重直读环境变量、可被写成任意值绕过锁定；现在必须由
// registry 校验拦住。
func TestReliabilityWeightFloorLocked(t *testing.T) {
	spec, ok := registry[types.SettingKeyFusionReliabilityWeight]
	require.True(t, ok)
	require.Equal(t, "float", spec.Type, "可靠度权重必须是 float，否则前端会渲染成文本框")
	require.Equal(t, types.SettingDefaultReliabilityWeight, spec.Default)

	// 低于下限必须被拒
	require.Error(t, validateRegistryEntry(types.SettingKeyFusionReliabilityWeight, 0.1))
	require.Error(t, validateRegistryEntry(types.SettingKeyFusionReliabilityWeight, 0.24))
	// 下限本身与区间内必须通过
	require.NoError(t, validateRegistryEntry(types.SettingKeyFusionReliabilityWeight, 0.25))
	require.NoError(t, validateRegistryEntry(types.SettingKeyFusionReliabilityWeight, 0.5))
	require.NoError(t, validateRegistryEntry(types.SettingKeyFusionReliabilityWeight, 1))
	// 超过 1 无意义
	require.Error(t, validateRegistryEntry(types.SettingKeyFusionReliabilityWeight, 1.5))
}
