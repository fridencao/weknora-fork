package storageallowlist

import (
	"strings"
	"sync/atomic"
)

// allowListOverride carries the system_settings value for
// storage.allow_list. nil means "nothing pushed yet" — fall back to the
// STORAGE_ALLOW_LIST env var.
//
// AllowedMap/IsAllowed are called from provider construction and the
// admin API, neither of which has a settings service in scope.
//
// Pointer so "not pushed" differs from "pushed empty", the latter being a
// deliberate "allow everything" (which is also what an unset env means,
// but the distinction keeps the pushed state explicit).
var allowListOverride atomic.Pointer[[]string]

// SetStorageAllowList pushes the resolved provider allow-list. An empty
// slice clears the override.
func SetStorageAllowList(providers []string) {
	if len(providers) == 0 {
		allowListOverride.Store(nil)
		return
	}
	out := make([]string, 0, len(providers))
	for _, p := range providers {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		allowListOverride.Store(nil)
		return
	}
	allowListOverride.Store(&out)
}

// ClearStorageAllowListOverride drops the pushed value. Used by tests that
// assert the env-only path.
func ClearStorageAllowListOverride() { allowListOverride.Store(nil) }
