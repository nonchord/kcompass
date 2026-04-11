package cli_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperatorDNSGitURL(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "dns", "git@github.com:company/clusters")
	require.NoError(t, err)
	assert.Contains(t, out, `"v=kc1; backend=git@github.com:company/clusters"`)
	// Section headers are always present regardless of detected domains.
	assert.Contains(t, out, "Corporate DNS")
	assert.Contains(t, out, "Tailscale")
	assert.Contains(t, out, "Netbird")
	// At least one kcompass. hostname is shown.
	assert.Contains(t, out, "kcompass.")
}

func TestOperatorDNSHTTPSURL(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "dns", "https://github.com/company/clusters")
	require.NoError(t, err)
	assert.Contains(t, out, `"v=kc1; backend=https://github.com/company/clusters"`)
}

func TestOperatorDNSShowsExample(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "dns", "git@github.com:company/clusters")
	require.NoError(t, err)
	assert.Contains(t, out, "300 IN TXT")
	// Example line always contains a kcompass. hostname (real or placeholder).
	assert.Contains(t, out, "kcompass.")
}

func TestOperatorDNSRequiresURL(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	_, err := executeWithConfig(t, cfgPath, "operator", "dns")
	assert.Error(t, err)
}

func TestOperatorDNSVerifyFlagAccepted(t *testing.T) {
	// --verify should run without error even when no TXT records exist.
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "dns", "--verify", "git@github.com:company/clusters")
	require.NoError(t, err)
	// Either "Verifying" header or "No network domains detected" should appear.
	assert.Contains(t, out, "Verifying TXT records")
}

// TestOperatorDNSHostnameFlag checks that --hostname overrides the detected
// domain list and implies --verify. The fake hostname won't resolve, so we
// only assert that it shows up in the verify output labelled "Explicit".
func TestOperatorDNSHostnameFlag(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "dns",
		"--hostname", "kcompass.verify-test.invalid",
		"git@github.com:company/clusters")
	require.NoError(t, err)
	assert.Contains(t, out, "Verifying TXT records")
	assert.Contains(t, out, "Explicit")
	assert.Contains(t, out, "kcompass.verify-test.invalid")
}

// TestOperatorDNSHostnameRepeatable verifies that --hostname can be passed
// multiple times and all entries are present in the verify output.
func TestOperatorDNSHostnameRepeatable(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "dns",
		"--hostname", "kcompass.one.invalid",
		"--hostname", "kcompass.two.invalid",
		"git@github.com:company/clusters")
	require.NoError(t, err)
	assert.Contains(t, out, "kcompass.one.invalid")
	assert.Contains(t, out, "kcompass.two.invalid")
}

// TestOperatorDNSHostnameDedup verifies that duplicate --hostname entries
// are queried at most once. The verify output should contain the hostname
// exactly once. Same dedup code path covers the auto-detected case where
// the tailnet magic DNS suffix and a DNS search domain happen to agree.
func TestOperatorDNSHostnameDedup(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "dns",
		"--hostname", "kcompass.dup.invalid",
		"--hostname", "kcompass.dup.invalid",
		"git@github.com:company/clusters")
	require.NoError(t, err)
	// Exactly one verify line mentions the hostname (printDNSRecords output
	// prints the template above, which does NOT include explicit hostnames).
	assert.Equal(t, 1, strings.Count(out, "kcompass.dup.invalid"))
}

func TestInitOutputHintsDNS(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "init", "--skip-verify", "git@github.com:company/clusters")
	require.NoError(t, err)
	assert.Contains(t, out, "Backend registered")
	assert.Contains(t, out, "kcompass operator dns")
}
