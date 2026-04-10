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

func TestOperatorAddGKEAllFlags(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--provider", "gke",
		"--name", "prod",
		"--description", "Production cluster",
		"--project", "my-project",
		"--region", "us-east1",
		"--cluster-id", "prod",
	)
	require.NoError(t, err)
	assertValidClusterYAML(t, out, "prod", "gke", "gcloud")
	assert.Contains(t, out, "my-project")
	assert.Contains(t, out, "us-east1")
}

func TestOperatorAddEKSAllFlags(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--provider", "eks",
		"--name", "staging",
		"--account-id", "123456789012",
		"--region", "us-west-2",
		"--cluster-name", "staging",
	)
	require.NoError(t, err)
	assertValidClusterYAML(t, out, "staging", "eks", "aws")
	assert.Contains(t, out, "123456789012")
}

func TestOperatorAddGenericAllFlags(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--provider", "generic",
		"--name", "homelab",
		"--server", "https://192.168.1.100:6443",
	)
	require.NoError(t, err)
	assertValidClusterYAML(t, out, "homelab", "generic", "static")
	assert.Contains(t, out, "192.168.1.100")
}

func TestOperatorAddClusterIDDefaultsToName(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--provider", "gke",
		"--name", "prod",
		"--project", "my-project",
		"--region", "us-east1",
		// no --cluster-id: should default to name
	)
	require.NoError(t, err)
	assert.Contains(t, out, "cluster_id: prod")
}

func TestOperatorAddEKSClusterNameDefaultsToName(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--provider", "eks",
		"--name", "staging",
		"--account-id", "123456789012",
		"--region", "us-west-2",
		// no --cluster-name: should default to name
	)
	require.NoError(t, err)
	assert.Contains(t, out, "cluster_name: staging")
}

func TestOperatorAddMissingProviderErrors(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	_, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--name", "prod",
		"--project", "my-project",
		"--region", "us-east1",
	)
	assert.Error(t, err)
}

func TestOperatorAddMissingNameErrors(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	_, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--provider", "gke",
		"--project", "my-project",
		"--region", "us-east1",
	)
	assert.Error(t, err)
}

func TestOperatorAddUnknownProviderErrors(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	_, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--provider", "aks",
		"--name", "prod",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

func TestOperatorAddGKEMissingProjectErrors(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	_, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--provider", "gke",
		"--name", "prod",
		"--region", "us-east1",
		// missing --project
	)
	assert.Error(t, err)
}

func TestOperatorAddOutputToFile(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	outFile := filepath.Join(t.TempDir(), "clusters.yaml")

	_, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--provider", "gke",
		"--name", "prod",
		"--project", "my-project",
		"--region", "us-east1",
		"--output", outFile,
	)
	require.NoError(t, err)

	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assertValidClusterYAML(t, string(data), "prod", "gke", "gcloud")
}

func TestOperatorAddAppendsToExistingFile(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	outFile := filepath.Join(t.TempDir(), "clusters.yaml")

	// First cluster.
	_, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--provider", "gke", "--name", "prod",
		"--project", "my-project", "--region", "us-east1",
		"--output", outFile,
	)
	require.NoError(t, err)

	// Second cluster.
	_, err = executeWithConfig(t, cfgPath, "operator", "add",
		"--provider", "eks", "--name", "staging",
		"--account-id", "123456789012", "--region", "us-west-2",
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
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	outFile := filepath.Join(t.TempDir(), "clusters.yaml")

	out, err := executeWithConfig(t, cfgPath, "operator", "add",
		"--provider", "gke", "--name", "prod",
		"--project", "my-project", "--region", "us-east1",
		"--output", outFile,
	)
	require.NoError(t, err)
	assert.Contains(t, out, "prod")
	assert.Contains(t, out, outFile)
}

// --- Prompt (interactive path) unit tests ---

func TestPromptReturnsInput(t *testing.T) {
	in := strings.NewReader("my-cluster\n")
	var out strings.Builder
	val, err := cli.Prompt(&out, in, "Cluster name", "")
	require.NoError(t, err)
	assert.Equal(t, "my-cluster", val)
	assert.Contains(t, out.String(), "Cluster name")
}

func TestPromptUsesDefault(t *testing.T) {
	in := strings.NewReader("\n") // user just presses Enter
	var out strings.Builder
	val, err := cli.Prompt(&out, in, "Cluster name", "default-name")
	require.NoError(t, err)
	assert.Equal(t, "default-name", val)
	assert.Contains(t, out.String(), "[default-name]")
}

func TestPromptEOFReturnsDefault(t *testing.T) {
	in := strings.NewReader("") // immediate EOF
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

func assertValidClusterYAML(t *testing.T, yamlStr, name, provider, auth string) {
	t.Helper()
	type clusterFile struct {
		Clusters []backend.ClusterRecord `yaml:"clusters"`
	}
	var cf clusterFile
	require.NoError(t, yaml.Unmarshal([]byte(yamlStr), &cf), "output is not valid YAML")
	require.NotEmpty(t, cf.Clusters, "expected at least one cluster in output")
	found := false
	for _, c := range cf.Clusters {
		if c.Name == name {
			assert.Equal(t, provider, c.Provider)
			assert.Equal(t, auth, c.Auth)
			found = true
		}
	}
	assert.True(t, found, "cluster %q not found in output", name)
}
