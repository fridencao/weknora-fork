package approval

// 系统设置桥接（配置治理批一）。
//
// Gate 是容器启动期构造的单例，构造时把审批超时与 fail-open 策略固化进结构体
// 字段；而这两项现在归 system_settings 管（DB > ENV > 默认），需要能热更新。
// 沿用 internal/sandbox/docker_enabled.go 的既有先例：包级 atomic 覆盖值 +
// 由 systemSettingService 在 preload / Update / Reset / pubsub 重载时推送。
//
// 覆盖值为空（未推送）时回落到构造期从 ENV / 默认解析出的字段值，保证
// 启动窗口与只 Setenv 的单测行为不变。

import (
	"sync/atomic"
	"time"
)

var (
	// toolApprovalTimeoutOverride 秒；0 表示"未推送"。
	toolApprovalTimeoutOverride atomic.Int64
	// toolApprovalFailOpenOverride nil 表示"未推送"。
	toolApprovalFailOpenOverride atomic.Pointer[bool]
)

// SetToolApprovalTimeout 记录解析后的审批等待时长（秒）。<=0 视为未设置。
func SetToolApprovalTimeout(seconds int) {
	toolApprovalTimeoutOverride.Store(int64(seconds))
}

// SetToolApprovalFailOpen 记录解析后的放行策略（true=审批不可用时放行）。
func SetToolApprovalFailOpen(failOpen bool) {
	v := failOpen
	toolApprovalFailOpenOverride.Store(&v)
}

// ClearToolApprovalOverrides 恢复"仅构造期解析"的行为，避免测试间串味。
func ClearToolApprovalOverrides() {
	toolApprovalTimeoutOverride.Store(0)
	toolApprovalFailOpenOverride.Store(nil)
}

// effectiveTimeout 返回当前生效的审批等待时长：系统设置推送值优先，
// 否则用构造期解析的字段值。
func (g *Gate) effectiveTimeout() time.Duration {
	if sec := toolApprovalTimeoutOverride.Load(); sec > 0 {
		return time.Duration(sec) * time.Second
	}
	return g.timeout
}

// effectiveFailClose 返回当前生效的"失败即拒绝"标志：系统设置推送值优先
// （failOpen=true → failClose=false），否则用构造期解析的字段值。
func (g *Gate) effectiveFailClose() bool {
	if v := toolApprovalFailOpenOverride.Load(); v != nil {
		return !*v
	}
	return g.failClose
}
