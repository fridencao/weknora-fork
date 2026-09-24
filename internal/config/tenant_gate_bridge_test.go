package config

import "testing"

// TestTenantGateBridgePrecedence pins pushed > config.yaml > built-in,
// including the two semantics that make this bridge safety-critical:
//   - pushing FALSE (operator turns a gate off) must stick — the pushed
//     flag, not a nil/zero-value distinction, carries the state;
//   - a nil TenantConfig must keep failing closed (RBAC=true, cross=false)
//     when nothing is pushed.
func TestTenantGateBridgePrecedence(t *testing.T) {
	t.Cleanup(func() {
		ClearTenantRBACEnforcedOverride()
		ClearTenantCrossTenantAccessOverride()
	})
	ClearTenantRBACEnforcedOverride()
	ClearTenantCrossTenantAccessOverride()

	// Built-in defaults, no YAML, no push.
	var nilTenant *TenantConfig
	if !nilTenant.IsRBACEnforced() {
		t.Error("nil TenantConfig: RBAC must fail closed to true")
	}
	if nilTenant.EffectiveEnableCrossTenantAccess() {
		t.Error("nil TenantConfig: cross-tenant must default to false")
	}

	// YAML tier.
	yamlOff := &TenantConfig{EnableCrossTenantAccess: true}
	if !yamlOff.EffectiveEnableCrossTenantAccess() {
		t.Error("YAML true should be visible before any push")
	}

	// Pushed tier beats YAML — including pushing false over YAML true.
	SetTenantRBACEnforcedOverride(false)
	SetTenantCrossTenantAccessOverride(false)
	if nilTenant.IsRBACEnforced() {
		t.Error("pushed false must beat built-in true")
	}
	if yamlOff.EffectiveEnableCrossTenantAccess() {
		t.Error("pushed false must beat YAML true (operator turned the gate off)")
	}

	// Clearing restores the YAML tier.
	ClearTenantCrossTenantAccessOverride()
	if !yamlOff.EffectiveEnableCrossTenantAccess() {
		t.Error("after clear, YAML true must be visible again")
	}
}
