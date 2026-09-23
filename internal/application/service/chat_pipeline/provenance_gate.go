package chatpipeline

// WS3.3 · 溯源完备性门禁（数据侧强绑定）。
// 在 INTO_CHAT_MESSAGE 前检查检索结果：带 sbk_blocks（L4 指针，来自契约归一层
// 对齐服务）的 chunk 才具备溯源资格；可配置"强制溯源"时剥离无指针 chunk，
// 否则仅在日志与 metadata 打标。答案文本级论断拦截（NLP 侧）属 M3 范围。

import (
	"context"
	"encoding/json"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// PluginProvenanceGate 溯源完备性门禁插件。
type PluginProvenanceGate struct {
	// settings 提供 starkb.require_provenance 开关（DB > ENV > 默认），
	// 每次请求实时解析，改后立即生效。
	settings interfaces.SystemSettingService
}

// NewPluginProvenanceGate 创建门禁插件（container.Invoke 接线）。
// 开关：starkb.require_provenance 系统设置启用强绑定剥离。
func NewPluginProvenanceGate(
	eventManager *EventManager,
	settings interfaces.SystemSettingService,
) *PluginProvenanceGate {
	p := &PluginProvenanceGate{settings: settings}
	eventManager.Register(p)
	return p
}

// requireProvenance 解析强绑定开关（DB > ENV > 默认 false）。
func (p *PluginProvenanceGate) requireProvenance(ctx context.Context) bool {
	if p.settings == nil {
		return false
	}
	return p.settings.GetBool(ctx,
		types.SettingKeyStarkbRequireProvenance, types.SettingEnvStarkbRequireProvenance, false)
}

// ActivationEvents 在消息装配前触发（检索结果已定型）。
func (p *PluginProvenanceGate) ActivationEvents() []types.EventType {
	return []types.EventType{types.INTO_CHAT_MESSAGE}
}

func (p *PluginProvenanceGate) hasL4Pointer(r *types.SearchResult) bool {
	if r == nil || r.ChunkMetadata == nil {
		return false
	}
	raw, err := json.Marshal(r.ChunkMetadata)
	if err != nil {
		return false
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return false
	}
	blocks, ok := meta["sbk_blocks"]
	if !ok {
		return false
	}
	arr, ok := blocks.([]any)
	return ok && len(arr) > 0
}

// OnEvent 统计溯源覆盖率；强绑定模式下剥离无指针 chunk。
func (p *PluginProvenanceGate) OnEvent(
	ctx context.Context,
	eventType types.EventType,
	chatManage *types.ChatManage,
	next func() *PluginError,
) *PluginError {
	total := len(chatManage.SearchResult)
	if total == 0 {
		return next()
	}
	require := p.requireProvenance(ctx)
	withPointer := 0
	kept := chatManage.SearchResult[:0]
	for _, r := range chatManage.SearchResult {
		if p.hasL4Pointer(r) {
			withPointer++
			kept = append(kept, r)
		} else if !require {
			kept = append(kept, r)
		}
	}
	chatManage.SearchResult = kept
	coverage := 0
	if total > 0 {
		coverage = withPointer * 100 / total
	}
	logger.Infof(ctx, "provenance gate: 溯源覆盖 %d/%d=%d%%（require=%v，剥离 %d）",
		withPointer, total, coverage, require, total-len(kept))
	return next()
}
