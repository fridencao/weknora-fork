package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// TestAuditRetentionEffectiveDays pins the resolution order for the
// per-sweep retention window:
//
//	DB > ENV > config.yaml tier (ctor def) > 0 semantics preserved.
func TestAuditRetentionEffectiveDays(t *testing.T) {
	// ENV-only settings: DB tier empty, so ENV wins over the ctor def.
	t.Setenv(types.SettingEnvAuditRetentionDays, "30")
	r := &AuditLogRetentionRunner{retentionDays: 90, settings: newEnvOnlySettings()}
	if got := r.effectiveRetentionDays(context.Background()); got != 30 {
		t.Errorf("env=30 should beat ctor def=90, got %d", got)
	}

	// No ENV, no DB: fall back to the config.yaml tier.
	t.Setenv(types.SettingEnvAuditRetentionDays, "")
	if got := r.effectiveRetentionDays(context.Background()); got != 90 {
		t.Errorf("fallback: want ctor def 90, got %d", got)
	}

	// 0 stays 0 — "disable the purge" is a supported compliance posture.
	r2 := &AuditLogRetentionRunner{retentionDays: 0, settings: newEnvOnlySettings()}
	if got := r2.effectiveRetentionDays(context.Background()); got != 0 {
		t.Errorf("cfg 0 (disabled) must stay 0, got %d", got)
	}

	// nil settings (legacy constructor path in old tests) must not panic.
	r3 := &AuditLogRetentionRunner{retentionDays: 45}
	if got := r3.effectiveRetentionDays(context.Background()); got != 45 {
		t.Errorf("nil settings: want ctor def 45, got %d", got)
	}
}
