package docparser

import (
	"strings"
	"sync/atomic"
)

// imageHostKeepURLs carries the system_settings value for
// image_host.keep_url. nil means "nothing pushed yet" — fall back to the
// IMAGE_HOST_KEEP_URL env var.
//
// isWhitelistedImageHost is called from package-level fetch helpers that
// have a context but no settings service in scope, so the resolved value
// is pushed here by systemSettingService (same shape as the vlm /
// storageallowlist bridges).
//
// Entries are already normalised (lower-cased, trimmed, non-empty) by
// SetImageHostKeepURLs.
var imageHostKeepURLs atomic.Pointer[[]string]

// SetImageHostKeepURLs pushes the whitelist. An empty list clears the
// override, restoring env/default resolution.
func SetImageHostKeepURLs(hosts []string) {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			out = append(out, h)
		}
	}
	if len(out) == 0 {
		imageHostKeepURLs.Store(nil)
		return
	}
	imageHostKeepURLs.Store(&out)
}

// ClearImageHostKeepURLOverride drops the pushed value. Used by tests
// that assert the env-only path.
func ClearImageHostKeepURLOverride() { imageHostKeepURLs.Store(nil) }
