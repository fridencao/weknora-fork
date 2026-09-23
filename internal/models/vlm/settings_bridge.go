package vlm

import (
	"sync/atomic"
	"time"
)

// vlmHTTPTimeoutOverride carries the system_settings value for
// vlm.http_timeout_s. Zero means "nothing pushed yet" — fall back to the
// env var, then to defaultTimeout.
//
// Why a package-level atomic rather than a parameter: vlmHTTPTimeout is
// called while constructing the OpenAI-compatible client, which has
// neither a context nor a settings service. Same shape as
// sandbox.SetDockerBackendEnabled and approval.SetToolApprovalTimeout.
//
// Zero is a safe sentinel: a zero timeout would mean "no timeout at all"
// in http.Client terms, which we never want, so it cannot be a legitimate
// pushed value.
var vlmHTTPTimeoutOverride atomic.Int64 // nanoseconds

// SetVLMHTTPTimeout pushes the resolved VLM HTTP timeout. A non-positive
// duration clears the override, restoring env/default resolution.
func SetVLMHTTPTimeout(d time.Duration) {
	if d <= 0 {
		vlmHTTPTimeoutOverride.Store(0)
		return
	}
	vlmHTTPTimeoutOverride.Store(int64(d))
}

// ClearVLMHTTPTimeoutOverride drops the pushed value, restoring env
// resolution. Used by tests that assert the env-only path.
func ClearVLMHTTPTimeoutOverride() { vlmHTTPTimeoutOverride.Store(0) }
