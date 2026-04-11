package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/nonchord/kcompass/internal/backend"
	"github.com/nonchord/kcompass/internal/cli"
)

// --- Non-interactive (all flags) tests ---

func TestOperatorAddCommandMode(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--name", "nonchord-staging",
		"--description", "Staging cluster",
		"--command", "tailscale configure kubeconfig nonchord-staging",
	)
	require.NoError(t, err)
	rec := assertSingleClusterYAML(t, out, "nonchord-staging")
	require.NotNil(t, rec.Kubeconfig.Command)
	assert.Equal(t,
		[]string{"tailscale", "configure", "kubeconfig", "nonchord-staging"},
		rec.Kubeconfig.Command)
	assert.Empty(t, rec.Kubeconfig.Inline)
}

func TestOperatorAddInlineMode(t *testing.T) {
	tmp := t.TempDir()
	kubeconfig := filepath.Join(tmp, "kc.yaml")
	const blob = `apiVersion: v1
kind: Config
clusters:
  - name: dev
    cluster:
      server: https://127.0.0.1:6443
contexts:
  - name: dev
    context:
      cluster: dev
      user: dev
current-context: dev
users:
  - name: dev
    user:
      token: t
`
	require.NoError(t, os.WriteFile(kubeconfig, []byte(blob), 0o600))

	cfgPath := filepath.Join(tmp, "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--name", "dev-laptop",
		"--kubeconfig", kubeconfig,
	)
	require.NoError(t, err)
	rec := assertSingleClusterYAML(t, out, "dev-laptop")
	assert.Equal(t, blob, rec.Kubeconfig.Inline)
	assert.Empty(t, rec.Kubeconfig.Command)
}

func TestOperatorAddCommandSplitsOnWhitespace(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--name", "prod-gke",
		"--command", "  gcloud  container  clusters  get-credentials  prod  ",
	)
	require.NoError(t, err)
	rec := assertSingleClusterYAML(t, out, "prod-gke")
	assert.Equal(t,
		[]string{"gcloud", "container", "clusters", "get-credentials", "prod"},
		rec.Kubeconfig.Command)
}

func TestOperatorAddMissingNameErrors(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	_, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--command", "tailscale configure kubeconfig x",
	)
	assert.Error(t, err)
}

func TestOperatorAddMissingCredSourceErrors(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	_, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--name", "x",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--command")
}

func TestOperatorAddBothModesErrors(t *testing.T) {
	tmp := t.TempDir()
	kubeconfig := filepath.Join(tmp, "kc.yaml")
	require.NoError(t, os.WriteFile(kubeconfig, []byte("apiVersion: v1\nkind: Config\n"), 0o600))

	cfgPath := filepath.Join(tmp, "config.yaml")
	_, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--name", "x",
		"--command", "echo hi",
		"--kubeconfig", kubeconfig,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestOperatorAddEmptyKubeconfigFileErrors(t *testing.T) {
	tmp := t.TempDir()
	kubeconfig := filepath.Join(tmp, "kc.yaml")
	require.NoError(t, os.WriteFile(kubeconfig, []byte{}, 0o600))

	cfgPath := filepath.Join(tmp, "config.yaml")
	_, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--name", "x",
		"--kubeconfig", kubeconfig,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestOperatorAddOutputToFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	outFile := filepath.Join(tmp, "clusters.yaml")

	_, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--name", "prod",
		"--command", "tailscale configure kubeconfig prod",
		"--output", outFile,
	)
	require.NoError(t, err)

	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	rec := assertSingleClusterYAML(t, string(data), "prod")
	assert.Equal(t, []string{"tailscale", "configure", "kubeconfig", "prod"}, rec.Kubeconfig.Command)
}

func TestOperatorAddAppendsToExistingFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	outFile := filepath.Join(tmp, "clusters.yaml")

	_, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--name", "prod",
		"--command", "tailscale configure kubeconfig prod",
		"--output", outFile,
	)
	require.NoError(t, err)

	_, err = executeWithConfig(t, cfgPath, "operator", "add",
		"--name", "staging",
		"--command", "tailscale configure kubeconfig staging",
		"--output", outFile,
	)
	require.NoError(t, err)

	data, err := os.ReadFile(outFile)
	require.NoError(t, err)

	type clusterFile struct {
		Clusters []backend.ClusterRecord `yaml:"clusters"`
	}
	var cf clusterFile
	require.NoError(t, yaml.Unmarshal(data, &cf))
	require.Len(t, cf.Clusters, 2)
	assert.Equal(t, "prod", cf.Clusters[0].Name)
	assert.Equal(t, "staging", cf.Clusters[1].Name)
}

func TestOperatorAddOutputToFileShowsConfirmation(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	outFile := filepath.Join(tmp, "clusters.yaml")

	out, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--name", "prod",
		"--command", "tailscale configure kubeconfig prod",
		"--output", outFile,
	)
	require.NoError(t, err)
	assert.Contains(t, out, "prod")
	assert.Contains(t, out, outFile)
}

// --- Prompt unit tests ---

func TestPromptReturnsInput(t *testing.T) {
	in := strings.NewReader("my-cluster\n")
	var out strings.Builder
	val, err := cli.Prompt(&out, in, "Cluster name", "")
	require.NoError(t, err)
	assert.Equal(t, "my-cluster", val)
	assert.Contains(t, out.String(), "Cluster name")
}

func TestPromptUsesDefault(t *testing.T) {
	in := strings.NewReader("\n")
	var out strings.Builder
	val, err := cli.Prompt(&out, in, "Cluster name", "default-name")
	require.NoError(t, err)
	assert.Equal(t, "default-name", val)
	assert.Contains(t, out.String(), "[default-name]")
}

func TestPromptEOFReturnsDefault(t *testing.T) {
	in := strings.NewReader("")
	var out strings.Builder
	val, err := cli.Prompt(&out, in, "Cluster name", "fallback")
	require.NoError(t, err)
	assert.Equal(t, "fallback", val)
}

func TestPromptTrimsWhitespace(t *testing.T) {
	in := strings.NewReader("  prod  \n")
	var out strings.Builder
	val, err := cli.Prompt(&out, in, "Name", "")
	require.NoError(t, err)
	assert.Equal(t, "prod", val)
}

func TestPromptNoDefaultOmitsBrackets(t *testing.T) {
	in := strings.NewReader("x\n")
	var out strings.Builder
	_, err := cli.Prompt(&out, in, "Name", "")
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "[")
}

// --- helpers ---

func assertSingleClusterYAML(t *testing.T, yamlStr, name string) backend.ClusterRecord {
	t.Helper()
	type clusterFile struct {
		Clusters []backend.ClusterRecord `yaml:"clusters"`
	}
	var cf clusterFile
	require.NoError(t, yaml.Unmarshal([]byte(yamlStr), &cf), "output is not valid YAML")
	require.NotEmpty(t, cf.Clusters)
	for _, c := range cf.Clusters {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("cluster %q not found in output", name)
	return backend.ClusterRecord{}
}
