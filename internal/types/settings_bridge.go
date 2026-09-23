package types

import (
	"strings"
	"sync/atomic"
)

// languageOverride carries the system_settings value for language.default.
// nil means "nothing pushed yet" — fall back to the WEKNORA_LANGUAGE env
// var, then to the zh-CN default.
//
// EnvLanguage/DefaultLanguage are called from prompt construction, task
// payload persistence, and middleware — none of which carry a settings
// service — so the value is pushed here.
//
// A pointer (not a bare string) so "not pushed" is distinguishable from
// "pushed an empty string", which means "clear the override and go back
// to the env/default".
var languageOverride atomic.Pointer[string]

// SetDefaultLanguage pushes the resolved default locale. An empty or
// whitespace-only value clears the override.
func SetDefaultLanguage(locale string) {
	trimmed := strings.TrimSpace(locale)
	if trimmed == "" {
		languageOverride.Store(nil)
		return
	}
	languageOverride.Store(&trimmed)
}

// ClearDefaultLanguageOverride drops the pushed value. Used by tests that
// assert the env-only path.
func ClearDefaultLanguageOverride() { languageOverride.Store(nil) }
