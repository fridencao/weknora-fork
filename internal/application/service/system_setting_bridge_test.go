package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/models/vlm"
	"github.com/Tencent/WeKnora/internal/storageallowlist"
	"github.com/Tencent/WeKnora/internal/types"
)

// TestApplyDeepPackageBridgesPushesResolvedValues verifies the plumbing:
// values resolved by the 3-tier resolver actually land in the consumer
// packages' override slots.
//
// Precedence within each bridge is covered by that package's own test;
// this one proves the push happens at all — the failure mode it guards
// against is "registry entry added, consumer converted, but nobody ever
// calls the setter", which leaves the UI value inert with no error.
//
// Two of the four bridges expose an exported read path, so they can be
// asserted from here. vlm and embedding keep their resolvers unexported;
// their precedence tests cover the same shared code path.
func TestApplyDeepPackageBridgesPushesResolvedValues(t *testing.T) {
	t.Setenv("WEKNORA_LANGUAGE", "en-US")
	t.Setenv("STORAGE_ALLOW_LIST", "local,minio")

	t.Cleanup(func() {
		types.ClearDefaultLanguageOverride()
		storageallowlist.ClearStorageAllowListOverride()
		// 桥接位是包级 atomic，值会跨测试存留。若不清理，这里推过的
		// 默认值会压住后续测试设置的 env（如 BATCH_EMBED_SIZE），
		// 造成顺序相关的假失败——实测发生过一次。
		vlm.ClearVLMHTTPTimeoutOverride()
		embedding.ClearBatchEmbedSizeOverride()
	})

	// ENV-only settings: the DB tier is empty, so the resolver reads ENV.
	svc := newEnvOnlySettings().(*systemSettingService)
	svc.applyDeepPackageBridges(context.Background())

	if got := types.DefaultLanguage(); got != "en-US" {
		t.Errorf("language bridge: want en-US, got %q", got)
	}
	if m := storageallowlist.AllowedMap(); !m["local"] || !m["minio"] || m["s3"] {
		t.Errorf("storage allow-list bridge: want {local,minio} only, got %v", m)
	}
}
