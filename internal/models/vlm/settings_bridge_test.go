package vlm

import (
	"testing"
	"time"
)

// TestVLMHTTPTimeoutBridgePrecedence pins the resolution order:
// pushed setting > ENV > built-in default.
func TestVLMHTTPTimeoutBridgePrecedence(t *testing.T) {
	t.Cleanup(ClearVLMHTTPTimeoutOverride)
	ClearVLMHTTPTimeoutOverride()

	// Default: nothing pushed, no env.
	t.Setenv("VLM_HTTP_TIMEOUT_SECONDS", "")
	if got := vlmHTTPTimeout(); got != defaultTimeout {
		t.Errorf("default: want %v, got %v", defaultTimeout, got)
	}

	// ENV tier.
	t.Setenv("VLM_HTTP_TIMEOUT_SECONDS", "42")
	if got := vlmHTTPTimeout(); got != 42*time.Second {
		t.Errorf("env=42: want 42s, got %v", got)
	}

	// Pushed (system_settings) tier must beat ENV.
	SetVLMHTTPTimeout(7 * time.Second)
	if got := vlmHTTPTimeout(); got != 7*time.Second {
		t.Errorf("pushed=7s should beat env=42: want 7s, got %v", got)
	}

	// Clearing restores ENV.
	ClearVLMHTTPTimeoutOverride()
	if got := vlmHTTPTimeout(); got != 42*time.Second {
		t.Errorf("after clear: want env 42s, got %v", got)
	}
}

// TestSetVLMHTTPTimeoutRejectsNonPositive guards the zero sentinel: a
// zero timeout would mean "no timeout at all", so it must clear instead.
func TestSetVLMHTTPTimeoutRejectsNonPositive(t *testing.T) {
	t.Cleanup(ClearVLMHTTPTimeoutOverride)
	t.Setenv("VLM_HTTP_TIMEOUT_SECONDS", "")

	SetVLMHTTPTimeout(0)
	if got := vlmHTTPTimeout(); got != defaultTimeout {
		t.Errorf("push 0 must not set a zero timeout: want default, got %v", got)
	}
	SetVLMHTTPTimeout(-5 * time.Second)
	if got := vlmHTTPTimeout(); got != defaultTimeout {
		t.Errorf("push negative must clear: want default, got %v", got)
	}
}
