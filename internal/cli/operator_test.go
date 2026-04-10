package cli_test

import (
	"path/filepath"
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

func TestInitOutputHintsDNS(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "init", "git@github.com:company/clusters")
	require.NoError(t, err)
	assert.Contains(t, out, "Backend registered")
	assert.Contains(t, out, "kcompass operator dns")
}
