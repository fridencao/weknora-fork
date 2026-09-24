package main

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// boolOnlySettings is a SystemSettingService that answers only GetBool.
// The embedded nil interface makes any other method call panic loudly,
// which is the point: applyTenantSettingOverrides must not reach beyond
// the 3-tier bool resolver.
type boolOnlySettings struct {
	interfaces.SystemSettingService
	bools map[string]bool
}

func (s *boolOnlySettings) GetBool(_ context.Context, key, _ string, def bool) bool {
	if v, ok := s.bools[key]; ok {
		return v
	}
	return def
}

func boolPtr(b bool) *bool { return &b }

// cfgWithTenant builds a config whose tenant section mirrors what
// LoadConfig would have produced from config.yaml + env before the
// bootstrap hook runs.
func cfgWithTenant(rbac *bool, crossTenant bool) *config.Config {
	return &config.Config{
		Tenant: &config.TenantConfig{
			EnableRBAC:              rbac,
			EnableCrossTenantAccess: crossTenant,
		},
	}
}

// TestTenantOverrideDBBeatsConfigYAML is the core guarantee: a value
// saved in system_settings wins over what config.yaml / env resolved to.
func TestTenantOverrideDBBeatsConfigYAML(t *testing.T) {
	// config.yaml opted OUT of RBAC enforcement and left cross-tenant off.
	cfg := cfgWithTenant(boolPtr(false), false)
	svc := &boolOnlySettings{bools: map[string]bool{
		types.SettingKeyTenantEnableRBAC:              true,
		types.SettingKeyTenantEnableCrossTenantAccess: true,
	}}

	t.Cleanup(func() {
		config.ClearTenantRBACEnforcedOverride()
		config.ClearTenantCrossTenantAccessOverride()
	})
	applyTenantSettingOverrides(context.Background(), cfg, svc)

	if !cfg.Tenant.IsRBACEnforced() {
		t.Errorf("DB enable_rbac=true should beat config.yaml false, got %v",
			cfg.Tenant.IsRBACEnforced())
	}
	if !cfg.Tenant.EnableCrossTenantAccess {
		t.Error("DB enable_cross_tenant_access=true should beat config.yaml false")
	}
}

// TestTenantOverrideFallsBackToConfigYAML covers the no-DB-row case: the
// value LoadConfig resolved must survive untouched, so deployments that
// configure these through config.yaml keep working.
func TestTenantOverrideFallsBackToConfigYAML(t *testing.T) {
	cfg := cfgWithTenant(boolPtr(false), true)
	svc := &boolOnlySettings{bools: map[string]bool{}}

	t.Cleanup(func() {
		config.ClearTenantRBACEnforcedOverride()
		config.ClearTenantCrossTenantAccessOverride()
	})
	applyTenantSettingOverrides(context.Background(), cfg, svc)

	if cfg.Tenant.IsRBACEnforced() {
		t.Error("empty DB must keep config.yaml enable_rbac=false")
	}
	if !cfg.Tenant.EnableCrossTenantAccess {
		t.Error("empty DB must keep config.yaml enable_cross_tenant_access=true")
	}
}

// TestTenantOverrideUnsetRBACDefaultsTrue pins the built-in default.
// This is the value the registry advertises to the UI, and it is easy to
// get wrong: EnableRBAC is a *bool whose nil means "enforce".
func TestTenantOverrideUnsetRBACDefaultsTrue(t *testing.T) {
	cfg := cfgWithTenant(nil, false)
	svc := &boolOnlySettings{bools: map[string]bool{}}

	t.Cleanup(func() {
		config.ClearTenantRBACEnforcedOverride()
		config.ClearTenantCrossTenantAccessOverride()
	})
	applyTenantSettingOverrides(context.Background(), cfg, svc)

	if !cfg.Tenant.IsRBACEnforced() {
		t.Error("nil EnableRBAC (unset in config.yaml) must resolve to the default true")
	}
}

// TestTenantOverrideDBFalseBeatsConfigYAMLTrue is the security-relevant
// direction: the DB must be able to TIGHTEN as well as loosen. A
// cross-tenant deployment that wants to shut the door from the UI must
// not be silently overridden by a stale config.yaml.
func TestTenantOverrideDBFalseBeatsConfigYAMLTrue(t *testing.T) {
	cfg := cfgWithTenant(boolPtr(true), true)
	svc := &boolOnlySettings{bools: map[string]bool{
		types.SettingKeyTenantEnableCrossTenantAccess: false,
	}}

	t.Cleanup(func() {
		config.ClearTenantRBACEnforcedOverride()
		config.ClearTenantCrossTenantAccessOverride()
	})
	applyTenantSettingOverrides(context.Background(), cfg, svc)

	if cfg.Tenant.EnableCrossTenantAccess {
		t.Error("DB enable_cross_tenant_access=false must beat config.yaml true")
	}
}

// TestTenantOverrideNilConfigDoesNotPanic guards the defensive nil check:
// runStartupBootstrap is best-effort and must never brick startup.
func TestTenantOverrideNilConfigDoesNotPanic(t *testing.T) {
	svc := &boolOnlySettings{bools: map[string]bool{}}

	applyTenantSettingOverrides(context.Background(), nil, svc)
	applyTenantSettingOverrides(context.Background(), &config.Config{}, svc)
}
