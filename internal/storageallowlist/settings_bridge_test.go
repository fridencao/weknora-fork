package storageallowlist

import (
	"sort"
	"testing"
)

func allowedNames() []string {
	m := AllowedMap()
	out := make([]string, 0, len(m))
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// TestStorageAllowListBridgePrecedence pins pushed > ENV, and that a
// pushed list is itself filtered against the supported providers.
func TestStorageAllowListBridgePrecedence(t *testing.T) {
	t.Cleanup(ClearStorageAllowListOverride)
	ClearStorageAllowListOverride()

	// Unset ENV means "everything allowed".
	t.Setenv(AllowListEnv, "")
	if got := len(allowedNames()); got != len(supported) {
		t.Errorf("unset: want all %d providers allowed, got %d", len(supported), got)
	}

	t.Setenv(AllowListEnv, "local")
	if got := allowedNames(); len(got) != 1 || got[0] != "local" {
		t.Errorf("env=local: want [local], got %v", got)
	}

	// Pushed beats ENV, and unknown providers are dropped.
	SetStorageAllowList([]string{"minio", "not-a-provider"})
	if got := allowedNames(); len(got) != 1 || got[0] != "minio" {
		t.Errorf("pushed=[minio,not-a-provider]: want [minio], got %v", got)
	}

	ClearStorageAllowListOverride()
	if got := allowedNames(); len(got) != 1 || got[0] != "local" {
		t.Errorf("after clear: want env [local], got %v", got)
	}
}

// TestSetStorageAllowListEmptyClears pins that an empty push restores
// env resolution rather than allowing nothing.
func TestSetStorageAllowListEmptyClears(t *testing.T) {
	t.Cleanup(ClearStorageAllowListOverride)
	t.Setenv(AllowListEnv, "local")

	SetStorageAllowList(nil)
	SetStorageAllowList([]string{"  ", ""})
	if got := allowedNames(); len(got) != 1 || got[0] != "local" {
		t.Errorf("empty push must clear, want env [local], got %v", got)
	}
}
