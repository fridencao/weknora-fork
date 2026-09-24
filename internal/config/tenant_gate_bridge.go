package config

import "sync/atomic"

// 两个租户安全门禁的运行期覆盖位：system_settings（tenant.enable_rbac /
// tenant.enable_cross_tenant_access）解析结果由此推送给 config 包，
// 使中间件与 handler 的读取点无需重启即可拿到新值。
//
// 形状与 sandbox / approval / vlm 的桥接一致：目标包暴露 atomic 覆盖位
// 与 SetXxx，由 systemSettingService 在 preload 与 dispatchSideEffects
// 推送。读取优先级：推送值 > config.yaml > 内置默认。
//
// 为什么读的是「已推送」标志而不是 nil 语义：两个门禁只有 true/false
// 两个合法值，用 pushed 标志 + 值两位即可区分「未推送」与「推送了
// false」——后者恰恰是运维关掉门禁的常见操作，绝不能丢。

var (
	rbacEnforcedPushed atomic.Bool
	rbacEnforcedValue  atomic.Bool

	crossTenantPushed atomic.Bool
	crossTenantValue  atomic.Bool
)

// SetTenantRBACEnforcedOverride pushes the resolved tenant.enable_rbac.
func SetTenantRBACEnforcedOverride(v bool) {
	rbacEnforcedValue.Store(v)
	rbacEnforcedPushed.Store(true)
}

// ClearTenantRBACEnforcedOverride drops the pushed value, restoring
// config.yaml / built-in resolution. Used by tests.
func ClearTenantRBACEnforcedOverride() { rbacEnforcedPushed.Store(false) }

// SetTenantCrossTenantAccessOverride pushes the resolved
// tenant.enable_cross_tenant_access.
func SetTenantCrossTenantAccessOverride(v bool) {
	crossTenantValue.Store(v)
	crossTenantPushed.Store(true)
}

// ClearTenantCrossTenantAccessOverride drops the pushed value. Used by tests.
func ClearTenantCrossTenantAccessOverride() { crossTenantPushed.Store(false) }

// IsRBACEnforced 读取优先级：system_settings 推送值 > config.yaml > 内置
// 默认 true。nil 接收者的容错语义保留（未推送且 Tenant 未配置 → true，
// 失败关闭）。
//
// 推送位是包级状态，覆盖 nil 与非 nil 接收者一视同仁——这正是把桥接放
// 在包级而非实例字段的原因：中间件里的 cfg 与 handler 里的 configInfo
// 可能是不同时构造的 *Config 实例，实例字段做不到全局一致。
func (t *TenantConfig) IsRBACEnforced() bool {
	if rbacEnforcedPushed.Load() {
		return rbacEnforcedValue.Load()
	}
	if t == nil || t.EnableRBAC == nil {
		return true
	}
	return *t.EnableRBAC
}

// EffectiveEnableCrossTenantAccess 同上：推送值 > config.yaml > false。
func (t *TenantConfig) EffectiveEnableCrossTenantAccess() bool {
	if crossTenantPushed.Load() {
		return crossTenantValue.Load()
	}
	return t != nil && t.EnableCrossTenantAccess
}
