package types

import "testing"

// TestDefaultLanguageBridgePrecedence pins pushed > ENV > built-in.
func TestDefaultLanguageBridgePrecedence(t *testing.T) {
	t.Cleanup(ClearDefaultLanguageOverride)
	ClearDefaultLanguageOverride()

	t.Setenv("WEKNORA_LANGUAGE", "")
	if got := DefaultLanguage(); got != "zh-CN" {
		t.Errorf("default: want zh-CN, got %q", got)
	}

	t.Setenv("WEKNORA_LANGUAGE", "en-US")
	if got := DefaultLanguage(); got != "en-US" {
		t.Errorf("env=en-US: want en-US, got %q", got)
	}

	SetDefaultLanguage("ja-JP")
	if got := DefaultLanguage(); got != "ja-JP" {
		t.Errorf("pushed=ja-JP should beat env=en-US: want ja-JP, got %q", got)
	}

	// Empty push clears rather than pinning an empty locale — an empty
	// locale would interpolate into prompts as "Write in .".
	SetDefaultLanguage("   ")
	if got := DefaultLanguage(); got != "en-US" {
		t.Errorf("empty push must clear: want env en-US, got %q", got)
	}

	ClearDefaultLanguageOverride()
	if got := EnvLanguage(); got != "en-US" {
		t.Errorf("after clear: want env en-US, got %q", got)
	}
}
