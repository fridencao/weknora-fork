package docparser

import "testing"

// TestImageHostBridgePrecedence pins pushed > ENV and that normalisation
// (lower-case, trimmed, empty entries dropped) happens at push time.
func TestImageHostBridgePrecedence(t *testing.T) {
	t.Cleanup(ClearImageHostKeepURLOverride)
	ClearImageHostKeepURLOverride()

	t.Setenv("IMAGE_HOST_KEEP_URL", "env-host.example")
	if isWhitelistedImageHost("http://env-host.example/a.png") != true {
		t.Fatal("env tier should whitelist env-host.example")
	}

	// Pushed tier beats ENV; entries are normalised at push time.
	SetImageHostKeepURLs([]string{"  MinerU.Internal:8080 ", "", "ENV-HOST.EXAMPLE"})
	cases := []struct {
		url  string
		want bool
	}{
		{"http://mineru.internal:8080/img.png", true}, // host+port, normalised
		// 既有匹配规则：带端口的条目只匹配同端口请求。
		{"http://mineru.internal/other.png", false},      // entry had :8080
		{"http://env-host.example/x.png", true},          // pushed beats env
		{"http://evil.example/a.png", false},             // not listed
		{"http://mineru.internal.evil.com/a.png", false}, // suffix must not match
	}
	for _, c := range cases {
		if got := isWhitelistedImageHost(c.url); got != c.want {
			t.Errorf("isWhitelistedImageHost(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

// TestSetImageHostKeepURLsEmptyClears pins that an empty push restores
// env resolution rather than whitelisting nothing forever.
func TestSetImageHostKeepURLsEmptyClears(t *testing.T) {
	t.Cleanup(ClearImageHostKeepURLOverride)
	t.Setenv("IMAGE_HOST_KEEP_URL", "")

	SetImageHostKeepURLs([]string{"a.example"})
	ClearImageHostKeepURLOverride()
	if isWhitelistedImageHost("http://a.example/x.png") {
		t.Fatal("after clear, env is empty so nothing should match")
	}
}
