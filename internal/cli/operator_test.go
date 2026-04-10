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
	assert.Contains(t, out, "kcompass.<search-domain>")
	assert.Contains(t, out, "kcompass.<tailnet-magic-dns-suffix>")
	assert.Contains(t, out, "kcompass.<management-server-domain>")
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
	assert.Contains(t, out, "kcompass.internal.company.com")
}

func TestOperatorDNSRequiresURL(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	_, err := executeWithConfig(t, cfgPath, "operator", "dns")
	assert.Error(t, err)
}

func TestInitOutputHintsDNS(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "init", "git@github.com:company/clusters")
	require.NoError(t, err)
	assert.Contains(t, out, "Backend registered")
	assert.Contains(t, out, "kcompass operator dns")
}
