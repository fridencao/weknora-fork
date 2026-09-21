package chatpipeline

// WS3.3 · 溯源完备性门禁（数据侧强绑定）。
// 在 INTO_CHAT_MESSAGE 前检查检索结果：带 sbk_blocks（L4 指针，来自契约归一层
// 对齐服务）的 chunk 才具备溯源资格；可配置"强制溯源"时剥离无指针 chunk，
// 否则仅在日志与 metadata 打标。答案文本级论断拦截（NLP 侧）属 M3 范围。

import (
	"context"
	"encoding/json"
	"os"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// PluginProvenanceGate 溯源完备性门禁插件。
type PluginProvenanceGate struct {
	require bool // true=剥离无 L4 指针的 chunk（强绑定）
}

// NewPluginProvenanceGate 创建门禁插件（container.Invoke 接线）。
// 开关：STARKB_REQUIRE_PROVENANCE=true 启用强绑定剥离。
func NewPluginProvenanceGate(
	eventManager *EventManager,
) *PluginProvenanceGate {
	p := &PluginProvenanceGate{
		require: os.Getenv("STARKB_REQUIRE_PROVENANCE") == "true",
	}
	eventManager.Register(p)
	return p
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
	withPointer := 0
	kept := chatManage.SearchResult[:0]
	for _, r := range chatManage.SearchResult {
		if p.hasL4Pointer(r) {
			withPointer++
			kept = append(kept, r)
		} else if !p.require {
			kept = append(kept, r)
		}
	}
	chatManage.SearchResult = kept
	coverage := 0
	if total > 0 {
		coverage = withPointer * 100 / total
	}
	logger.Infof(ctx, "provenance gate: 溯源覆盖 %d/%d=%d%%（require=%v，剥离 %d）",
		withPointer, total, coverage, p.require, total-len(kept))
	return next()
}

